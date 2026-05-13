package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/spf13/cobra"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/backup"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/migrate"
	maint "github.com/drzero42/nexorious-go/internal/middleware"
	"github.com/drzero42/nexorious-go/internal/scheduler"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
	psnsvc "github.com/drzero42/nexorious-go/internal/services/psn"
	steamsvc "github.com/drzero42/nexorious-go/internal/services/steam"
	"github.com/drzero42/nexorious-go/internal/worker"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

// newServeCmd returns the `serve` subcommand. Bare `./nexorious` also routes
// here via the root command's RunE for backwards compatibility.
func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server (default action)",
		Long:  "Start the Echo HTTP server, worker pool, and scheduler. This is the default action when no subcommand is supplied.",
		RunE:  runServe,
	}
}

// runServe contains the historical main() body. It loads .env, opens the
// database, wires the migrator/worker/scheduler/HTTP server and blocks until
// SIGINT/SIGTERM triggers a graceful shutdown.
func runServe(cmd *cobra.Command, _ []string) error {
	// -------------------------------------------------------------------------
	// .env file loading
	// -------------------------------------------------------------------------
	configFile, _ := cmd.Root().PersistentFlags().GetString("config")
	if configFile != "" {
		if err := godotenv.Load(configFile); err != nil {
			return fmt.Errorf("load env file %q: %w", configFile, err)
		}
	} else {
		if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load .env: %w", err)
		}
	}

	// -------------------------------------------------------------------------
	// Config
	// -------------------------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	})))

	// -------------------------------------------------------------------------
	// Database
	// -------------------------------------------------------------------------
	ctx := context.Background()

	resolvedDatabaseURL := cfg.DatabaseURL
	db := openBunDB(resolvedDatabaseURL)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	defer func() { _ = db.Close() }()

	// Tool detection for backup/restore
	backup.CheckTools()
	if backup.PgDumpAvailable() {
		slog.Info("pg_dump available — backups enabled")
	} else {
		slog.Warn("pg_dump not found — backup creation disabled")
	}
	if backup.PsqlAvailable() {
		slog.Info("psql available — restore enabled")
	} else {
		slog.Warn("psql not found — restore disabled")
	}

	// Backup service
	backupSvc := backup.NewService(db, resolvedDatabaseURL, cfg.BackupPath, cfg.StoragePath, version)

	// -------------------------------------------------------------------------
	// Migrator
	// -------------------------------------------------------------------------
	migrator := migrate.NewMigrator(db)

	// initAppState runs determineState + InitNeedsSetup.
	// Called once at startup (if DB is reachable) and as StartDBProbe's onRecovery.
	initAppState := func(ctx context.Context) error {
		if err := migrator.DetermineStateForTest(); err != nil {
			return fmt.Errorf("initAppState: determineState: %w", err)
		}
		if migrator.State() == migrate.AppStateReady {
			if err := migrator.InitNeedsSetup(ctx, db); err != nil {
				return fmt.Errorf("initAppState: InitNeedsSetup: %w", err)
			}
		}
		return nil
	}

	// -------------------------------------------------------------------------
	// Single startup ping — no retry (StartDBProbe handles retries).
	// -------------------------------------------------------------------------
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	pingErr := db.PingContext(pingCtx)
	pingCancel()
	if pingErr == nil {
		slog.Info("database connected")
		if err := initAppState(ctx); err != nil {
			slog.Error("initAppState failed — starting in DBUnavailable state", "err", err)
		}
	} else {
		slog.Warn("database not reachable at startup — starting in DBUnavailable state", "err", pingErr)
	}

	// -------------------------------------------------------------------------
	// IGDB client (optional)
	// -------------------------------------------------------------------------
	igdbClient := igdb.NewClient(cfg)

	if !igdbClient.Configured() {
		slog.Warn("IGDB credentials not configured — game search, import, and metadata features will be unavailable")
	} else {
		validateCtx, validateCancel := context.WithTimeout(ctx, 10*time.Second)
		err := igdbClient.ValidateCredentials(validateCtx)
		validateCancel()
		if err != nil {
			if igdb.IsAuthError(err) {
				slog.Warn("IGDB credentials are invalid — disabling IGDB features", "err", err)
				igdbClient = igdb.NewClient(&config.Config{}) // unconfigured client
			} else {
				slog.Warn("IGDB credential probe failed (network/transient) — IGDB client kept", "err", err)
			}
		} else {
			slog.Info("IGDB credentials validated successfully")
		}
	}

	// -------------------------------------------------------------------------
	// Worker pool — created early so the Echo server can reference it.
	// -------------------------------------------------------------------------
	pool := worker.NewPool(db)
	pool.Register("import_item", tasks.NewImportItemHandler(db, igdbClient, cfg.StoragePath))
	pool.Register("export_json", tasks.NewExportJSONHandler(db, cfg.StoragePath))
	pool.Register("export_csv", tasks.NewExportCSVHandler(db, cfg.StoragePath))
	pool.Register("dispatch_sync", tasks.NewDispatchSyncHandler(db, steamsvc.NewClient(), psnsvc.NewClient()))
	pool.Register("process_sync_item", tasks.NewProcessSyncItemHandler(db, igdbClient))
	pool.Register("metadata_refresh_dispatch",
		tasks.NewMetadataRefreshDispatchHandler(db, pool, igdbClient))
	pool.Register("metadata_refresh_item",
		tasks.NewMetadataRefreshItemHandler(db, igdbClient, cfg.StoragePath))

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	var sched *scheduler.Scheduler

	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	restoreCallbacks := &api.RestoreCallbacks{
		SetMaintenance: maint.SetMaintenanceMode,
		ShutdownPool:   func() { pool.Shutdown() },
		StopScheduler: func() {
			if sched != nil {
				sched.Stop()
			}
		},
		CloseDB: func() error {
			// psql terminates PostgreSQL connections via pg_terminate_backend before restore;
			// keeping *bun.DB open lets all handlers auto-reconnect after restore completes.
			return nil
		},
		ReconnectDB: func() (*bun.DB, error) {
			// Existing *bun.DB auto-reconnects; no need to create a new instance.
			backupSvc.SetDB(db)
			return db, nil
		},
		RebuildServices: func(newDB *bun.DB) error {
			newPool := worker.NewPool(newDB)
			newPool.Register("import_item", tasks.NewImportItemHandler(newDB, igdbClient, cfg.StoragePath))
			newPool.Register("export_json", tasks.NewExportJSONHandler(newDB, cfg.StoragePath))
			newPool.Register("export_csv", tasks.NewExportCSVHandler(newDB, cfg.StoragePath))
			newPool.Register("dispatch_sync", tasks.NewDispatchSyncHandler(newDB, steamsvc.NewClient(), psnsvc.NewClient()))
			newPool.Register("process_sync_item", tasks.NewProcessSyncItemHandler(newDB, igdbClient))
			newPool.Register("metadata_refresh_dispatch",
				tasks.NewMetadataRefreshDispatchHandler(newDB, newPool, igdbClient))
			newPool.Register("metadata_refresh_item",
				tasks.NewMetadataRefreshItemHandler(newDB, igdbClient, cfg.StoragePath))
			newPool.Start(shutdownCtx, cfg.WorkerCount)
			pool = newPool

			newSched := scheduler.NewScheduler(newDB, newPool, backupSvc, cfg)
			sched = newSched
			if err := newSched.Start(shutdownCtx); err != nil {
				slog.Error("failed to restart scheduler after restore", "err", err)
				slog.Info("worker pool restarted after restore; scheduler did not start")
				return fmt.Errorf("restart scheduler after restore: %w", err)
			}

			slog.Info("workers and scheduler restarted after restore")
			return nil
		},
		ReinitMigrator: func(db *bun.DB) error {
			if err := migrator.DetermineStateForTest(); err != nil {
				return err
			}
			return migrator.InitNeedsSetup(context.Background(), db)
		},
		SetAppState: func(state string) {
			if state == "db_unavailable" {
				migrator.SetStateForTest(migrate.AppStateDBUnavailable)
			}
		},
		RebuildBackupJob: func(ctx context.Context, cron, retentionMode string, retentionValue int) {
			if sched != nil {
				sched.RebuildBackupJob(ctx, cron, retentionMode, retentionValue)
			}
		},
	}

	e := api.New(cfg, migrator, db, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, pool)

	// StartDBProbe — polls every 5s, calls initAppState on recovery.
	migrator.StartDBProbe(shutdownCtx, db, initAppState)

	// Worker/scheduler gate — starts after Ready && !NeedsSetup.
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if migrator.State() == migrate.AppStateReady && !migrator.NeedsSetup() {
				pool.Start(ctx, cfg.WorkerCount)
				sched = scheduler.NewScheduler(db, pool, backupSvc, cfg)
				if err := sched.Start(ctx); err != nil {
					slog.Error("failed to start scheduler", "err", err)
				}
				slog.Info("app ready — workers and scheduler started")
				return
			}
			time.Sleep(2 * time.Second)
		}
	}(shutdownCtx)

	addr := fmt.Sprintf(":%d", cfg.Port)
	sc := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: 10 * time.Second,
		HideBanner:      true,
		HidePort:        true,
	}

	slog.Info("nexorious starting", "addr", addr, "version", version, "commit", commit)
	if err := sc.Start(shutdownCtx, e); err != nil {
		slog.Info("server stopped", "err", err)
	}

	// Graceful shutdown sequence.
	if sched != nil {
		sched.Stop()
	}
	pool.Shutdown()

	slog.Info("shutdown complete")
	return nil
}

// parseSlogLevel maps a LOG_LEVEL string to a slog.Level.
func parseSlogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

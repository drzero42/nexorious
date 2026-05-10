package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/internal/scheduler"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
	"github.com/drzero42/nexorious-go/internal/worker"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	// -------------------------------------------------------------------------
	// CLI flags
	// -------------------------------------------------------------------------
	var (
		showVersion bool
		configFile  string
		migrateOnly bool
	)
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.BoolVar(&showVersion, "v", false, "Print version and exit (shorthand)")
	flag.StringVar(&configFile, "config", "", "Path to .env file (default: .env in working directory)")
	flag.BoolVar(&migrateOnly, "migrate-only", false, "Run pending migrations then exit (for initContainers)")
	flag.Parse()

	if showVersion {
		fmt.Printf("nexorious %s (%s)\n", version, commit)
		os.Exit(0)
	}

	// -------------------------------------------------------------------------
	// .env file loading
	// -------------------------------------------------------------------------
	if configFile != "" {
		if err := godotenv.Load(configFile); err != nil {
			log.Fatalf("failed to load env file %q: %v", configFile, err)
		}
	} else {
		if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Fatalf("failed to load .env: %v", err)
		}
	}

	// -------------------------------------------------------------------------
	// Config
	// -------------------------------------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	})))

	// -------------------------------------------------------------------------
	// Database
	// -------------------------------------------------------------------------
	ctx := context.Background()

	resolvedDatabaseURL := cfg.DatabaseURL
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(resolvedDatabaseURL)))
	db := bun.NewDB(sqldb, pgdialect.New())
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	defer func() { _ = db.Close() }()

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
	// --migrate-only mode
	// -------------------------------------------------------------------------
	if migrateOnly {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := db.PingContext(pingCtx)
			cancel()
			if err == nil {
				break
			}
			slog.Warn("migrate-only: waiting for database", "err", err)
			time.Sleep(2 * time.Second)
		}

		pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
		if err := db.PingContext(pingCtx); err != nil {
			pingCancel()
			slog.Error("migrate-only: database unreachable after 30s", "err", err)
			_ = db.Close()
			os.Exit(1)
		}
		pingCancel()

		if err := migrator.DetermineStateForTest(); err != nil {
			slog.Error("migrate-only: determineState failed", "err", err)
			_ = db.Close()
			os.Exit(1)
		}
		if migrator.State() == migrate.AppStateReady {
			slog.Info("migrate-only: no pending migrations")
			_ = db.Close()
			os.Exit(0)
		}

		migrator.SetLogWriter(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Writer())
		if err := migrator.RunMigrations(ctx); err != nil {
			slog.Error("migrate-only: migrations failed", "err", err)
			_ = db.Close()
			os.Exit(1)
		}
		slog.Info("migrate-only: migrations complete")
		_ = db.Close()
		os.Exit(0)
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
	pool.Register("import_item", tasks.NewImportItemHandler(db))
	pool.Register("export_json", tasks.NewExportJSONHandler(db, cfg.StoragePath))
	pool.Register("export_csv", tasks.NewExportCSVHandler(db, cfg.StoragePath))
	// pool.Register("process_sync_item", syncHandler)
	// pool.Register("metadata_refresh_process", metadataHandler)

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	e := api.New(cfg, migrator, db, resolvedDatabaseURL, igdbClient, pool)

	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// StartDBProbe — polls every 5s, calls initAppState on recovery.
	migrator.StartDBProbe(shutdownCtx, db, initAppState)

	// Worker/scheduler gate — starts after Ready && !NeedsSetup.
	var sched *scheduler.Scheduler

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if migrator.State() == migrate.AppStateReady && !migrator.NeedsSetup() {
				pool.Start(ctx, cfg.WorkerCount)
				sched = scheduler.NewScheduler(db, pool)
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

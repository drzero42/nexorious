package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/spf13/cobra"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/backup"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	maint "github.com/drzero42/nexorious/internal/middleware"
	"github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/scheduler"
	epicsvc "github.com/drzero42/nexorious/internal/services/epic"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	"github.com/drzero42/nexorious/internal/services/igdb"
	psnsvc "github.com/drzero42/nexorious/internal/services/psn"
	steamsvc "github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// newServeCmd returns the `serve` subcommand. Bare `./nexorious` also routes
// here via the root command's RunE for backwards compatibility.
func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server (default action)",
		Long:  "Start the Echo HTTP server, River worker client, and scheduler. This is the default action when no subcommand is supplied.",
		RunE:  runServe,
	}
}

// runServe contains the historical main() body. It loads .env, opens the
// database, wires the migrator/River client/HTTP server and blocks until
// SIGINT/SIGTERM triggers a graceful shutdown.
func runServe(cmd *cobra.Command, _ []string) error {
	cfg, err := loadEnvAndConfig(cmd)
	if err != nil {
		return err
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	})))

	encrypter, err := crypto.NewEncrypter(cfg.DBEncryptionKey)
	if err != nil {
		slog.Error("invalid DB_ENCRYPTION_KEY", "err", err)
		os.Exit(1)
	}

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
		if err := migrator.DetermineState(); err != nil {
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
	var igdbLimiter ratelimit.Limiter
	if cfg.RateLimiterBackend == "postgres" {
		igdbLimiter = ratelimit.NewPostgres(db, "igdb", cfg.IGDBRequestsPerSecond, float64(cfg.IGDBBurstCapacity))
	} else {
		igdbLimiter = ratelimit.NewLocal(cfg.IGDBRequestsPerSecond, cfg.IGDBBurstCapacity)
	}

	igdbClient := igdb.NewClient(cfg, igdbLimiter)

	if !igdbClient.Configured() {
		slog.Warn("IGDB credentials not configured — game search, import, and metadata features will be unavailable")
	} else {
		validateCtx, validateCancel := context.WithTimeout(ctx, 10*time.Second)
		err := igdbClient.ValidateCredentials(validateCtx)
		validateCancel()
		if err != nil {
			if igdb.IsAuthError(err) {
				slog.Warn("IGDB credentials are invalid — disabling IGDB features", "err", err)
				igdbClient = igdb.NewInvalidCredentialsClient(igdbLimiter)
			} else {
				slog.Warn("IGDB credential probe failed (network/transient) — IGDB client kept", "err", err)
			}
		} else {
			slog.Info("IGDB credentials validated successfully")
		}
	}

	// -------------------------------------------------------------------------
	// pgxPool for River
	// -------------------------------------------------------------------------
	pgxPool, err := openPgxPool(ctx, resolvedDatabaseURL)
	if err != nil {
		return fmt.Errorf("pgxpool: %w", err)
	}
	defer pgxPool.Close()

	// -------------------------------------------------------------------------
	// River workers
	// -------------------------------------------------------------------------
	staleThreshold, err := time.ParseDuration(cfg.StaleJobThreshold)
	if err != nil {
		slog.Warn("invalid STALE_JOB_THRESHOLD, defaulting to 4h", "value", cfg.StaleJobThreshold)
		staleThreshold = 4 * time.Hour
	}

	epicClient := epicsvc.NewClient(cfg.LegendaryWorkDir)
	dispatchSyncWorker := &tasks.DispatchSyncWorker{
		DB:      db,
		Adapter: buildAdapterFactory(db, encrypter, epicClient),
	}
	metaDispatchWorker := &tasks.MetadataRefreshDispatchWorker{
		DB:         db,
		IGDBClient: igdbClient,
	}
	checkPendingSyncsWorker := &scheduler.CheckPendingSyncsWorker{DB: db}
	rescueOrphanedWorker := &scheduler.RescueOrphanedPendingItemsWorker{DB: db}
	igdbMatchWorker := &tasks.IGDBMatchWorker{DB: db, IGDBClient: igdbClient}
	userGameWorker := &tasks.UserGameWorker{DB: db, IGDBClient: igdbClient}

	workers := river.NewWorkers()
	river.AddWorker(workers, &tasks.ImportItemWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &tasks.ExportJSONWorker{DB: db, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &tasks.ExportCSVWorker{DB: db, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, dispatchSyncWorker)
	river.AddWorker(workers, igdbMatchWorker)
	river.AddWorker(workers, userGameWorker)
	river.AddWorker(workers, metaDispatchWorker)
	river.AddWorker(workers, &tasks.MetadataRefreshItemWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &tasks.MetadataFetchWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &scheduler.CleanupOldJobsWorker{DB: db})
	river.AddWorker(workers, &scheduler.CleanupExportsWorker{DB: db})
	river.AddWorker(workers, &scheduler.CleanupUnreferencedGamesWorker{DB: db})
	river.AddWorker(workers, &scheduler.CleanupExpiredSessionsWorker{DB: db})
	river.AddWorker(workers, &scheduler.CleanupStaleJobsWorker{DB: db})
	river.AddWorker(workers, &scheduler.CleanupSyncChangesWorker{DB: db})
	river.AddWorker(workers, checkPendingSyncsWorker)
	river.AddWorker(workers, rescueOrphanedWorker)
	river.AddWorker(workers, &scheduler.CheckScheduledBackupWorker{DB: db, BackupSvc: backupSvc})
	river.AddWorker(workers, &notify.NotifyWorker{DB: db, Encrypter: encrypter, Sender: notify.NewShoutrrrSender()})
	river.AddWorker(workers, &notify.PruneEventsWorker{DB: db})

	riverClient, err := river.NewClient(riverpgxv5.New(pgxPool), &river.Config{
		Workers:      workers,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
		PeriodicJobs: scheduler.BuildPeriodicJobs(cfg, staleThreshold),
	})
	if err != nil {
		return fmt.Errorf("river.NewClient: %w", err)
	}
	notify.SetRiverClient(riverClient)

	// Wire River client into workers that submit sub-jobs.
	dispatchSyncWorker.RiverClient = riverClient
	metaDispatchWorker.RiverClient = riverClient
	checkPendingSyncsWorker.RiverClient = riverClient
	rescueOrphanedWorker.RiverClient = riverClient
	igdbMatchWorker.RiverClient = riverClient
	userGameWorker.RiverClient = riverClient

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	restoreCallbacks := &api.RestoreCallbacks{
		SetMaintenance: maint.SetMaintenanceMode,
		ShutdownPool:   func() {},
		StopScheduler:  func() {},
		CloseDB: func() error {
			return nil
		},
		ReconnectDB: func() (*bun.DB, error) {
			backupSvc.SetDB(db)
			return db, nil
		},
		RebuildServices: func(newDB *bun.DB) error {
			_ = riverClient.Stop(context.Background()) //nolint:errcheck // best-effort stop during DB rebuild; nowhere to surface
			pgxPool.Close()

			newPgxPool, err := openPgxPool(context.Background(), resolvedDatabaseURL)
			if err != nil {
				return fmt.Errorf("RebuildServices: pgxpool: %w", err)
			}

			newEpicClient := epicsvc.NewClient(cfg.LegendaryWorkDir)
			newDispatchSync := &tasks.DispatchSyncWorker{
				DB:      newDB,
				Adapter: buildAdapterFactory(newDB, encrypter, newEpicClient),
			}
			newMetaDispatch := &tasks.MetadataRefreshDispatchWorker{
				DB:         newDB,
				IGDBClient: igdbClient,
			}
			newCheckSyncs := &scheduler.CheckPendingSyncsWorker{DB: newDB}
			newRescueOrphaned := &scheduler.RescueOrphanedPendingItemsWorker{DB: newDB}
			newIGDBMatch := &tasks.IGDBMatchWorker{DB: newDB, IGDBClient: igdbClient}
			newUserGame := &tasks.UserGameWorker{DB: newDB, IGDBClient: igdbClient}

			newWorkers := river.NewWorkers()
			river.AddWorker(newWorkers, &tasks.ImportItemWorker{DB: newDB, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
			river.AddWorker(newWorkers, &tasks.ExportJSONWorker{DB: newDB, StoragePath: cfg.StoragePath})
			river.AddWorker(newWorkers, &tasks.ExportCSVWorker{DB: newDB, StoragePath: cfg.StoragePath})
			river.AddWorker(newWorkers, newDispatchSync)
			river.AddWorker(newWorkers, newIGDBMatch)
			river.AddWorker(newWorkers, newUserGame)
			river.AddWorker(newWorkers, newMetaDispatch)
			river.AddWorker(newWorkers, &tasks.MetadataRefreshItemWorker{DB: newDB, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
			river.AddWorker(newWorkers, &tasks.MetadataFetchWorker{DB: newDB, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
			river.AddWorker(newWorkers, &scheduler.CleanupOldJobsWorker{DB: newDB})
			river.AddWorker(newWorkers, &scheduler.CleanupExportsWorker{DB: newDB})
			river.AddWorker(newWorkers, &scheduler.CleanupUnreferencedGamesWorker{DB: newDB})
			river.AddWorker(newWorkers, &scheduler.CleanupExpiredSessionsWorker{DB: newDB})
			river.AddWorker(newWorkers, &scheduler.CleanupStaleJobsWorker{DB: newDB})
			river.AddWorker(newWorkers, &scheduler.CleanupSyncChangesWorker{DB: newDB})
			river.AddWorker(newWorkers, newCheckSyncs)
			river.AddWorker(newWorkers, newRescueOrphaned)
			river.AddWorker(newWorkers, &scheduler.CheckScheduledBackupWorker{DB: newDB, BackupSvc: backupSvc})
			river.AddWorker(newWorkers, &notify.NotifyWorker{DB: newDB, Encrypter: encrypter, Sender: notify.NewShoutrrrSender()})
			river.AddWorker(newWorkers, &notify.PruneEventsWorker{DB: newDB})

			newClient, err := river.NewClient(riverpgxv5.New(newPgxPool), &river.Config{
				Workers:      newWorkers,
				Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
				PeriodicJobs: scheduler.BuildPeriodicJobs(cfg, staleThreshold),
			})
			if err != nil {
				return fmt.Errorf("RebuildServices: river.NewClient: %w", err)
			}

			newDispatchSync.RiverClient = newClient
			newMetaDispatch.RiverClient = newClient
			newCheckSyncs.RiverClient = newClient
			newRescueOrphaned.RiverClient = newClient
			newIGDBMatch.RiverClient = newClient
			newUserGame.RiverClient = newClient

			if err := newClient.Start(shutdownCtx); err != nil {
				return fmt.Errorf("RebuildServices: River start: %w", err)
			}

			riverClient = newClient
			notify.SetRiverClient(riverClient)
			pgxPool = newPgxPool
			slog.Info("services rebuilt after restore")
			return nil
		},
		ReinitMigrator: func(db *bun.DB) error {
			if err := migrator.DetermineState(); err != nil {
				return err
			}
			return migrator.InitNeedsSetup(context.Background(), db)
		},
		SetAppState: func(state string) {
			if state == "db_unavailable" {
				migrator.SetStateForTest(migrate.AppStateDBUnavailable)
			}
		},
		RebuildBackupJob: func(_ context.Context, _, _ string, _ int) {},
	}

	e := api.New(encrypter, cfg, migrator, db, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, version, commit, riverClient)

	// StartDBProbe — polls every 5s, calls initAppState on recovery.
	migrator.StartDBProbe(shutdownCtx, db, initAppState)

	// River start gate — waits for Ready && !NeedsSetup.
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if migrator.State() == migrate.AppStateReady && !migrator.NeedsSetup() {
				reconcileOrphanedDispatchJobs(context.Background(), db)
				if err := riverClient.Start(ctx); err != nil {
					slog.Error("failed to start River client", "err", err)
				}
				slog.Info("app ready — River client started")
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
	if err := riverClient.Stop(shutdownCtx); err != nil {
		slog.Warn("River client stop", "err", err)
	}

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

// reconcileOrphanedDispatchJobs rescues dispatch_sync River jobs that are
// stuck in 'running' state because the process that claimed them is no longer
// heartbeating. Called once at startup before riverClient.Start so River picks
// them up for retry within seconds.
func reconcileOrphanedDispatchJobs(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(`
		UPDATE river_job
		   SET state = 'retryable',
		       scheduled_at = now(),
		       errors = array_append(errors, jsonb_build_object(
		         'at', now(),
		         'error', 'rescued at startup: client no longer heartbeating'
		       ))
		 WHERE kind = 'dispatch_sync'
		   AND state = 'running'
		   AND attempt < max_attempts
		   AND NOT EXISTS (
		     SELECT 1 FROM river_client rc
		      WHERE rc.id = ANY(river_job.attempted_by)
		        AND rc.updated_at > now() - interval '30 seconds'
		   )`,
	).Exec(ctx)
	if err != nil {
		slog.Error("startup: reconcile orphaned dispatch_sync failed", "err", err)
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows > 0 {
		slog.Info("startup: rescued orphaned dispatch_sync jobs", "count", rows)
	}
}

func buildAdapterFactory(
	db *bun.DB,
	encrypter *crypto.Encrypter,
	epicClient *epicsvc.Client,
) func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
	return func(ctx context.Context, storefront string, cfg models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		switch storefront {
		case "steam":
			if cfg.StorefrontCredentials == nil {
				return nil, tasks.ErrCredentials
			}
			plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
			if err != nil {
				slog.Warn("adapter factory: steam decrypt failed", "user_id", cfg.UserID, "err", err)
				return nil, tasks.ErrCredentials
			}
			var creds struct {
				WebAPIKey string `json:"web_api_key"`
				SteamID   string `json:"steam_id"`
			}
			if err := json.Unmarshal(plain, &creds); err != nil {
				return nil, tasks.ErrCredentials
			}
			return steamsvc.NewAdapter(steamsvc.NewClient(), creds.WebAPIKey, creds.SteamID), nil

		case "psn":
			if cfg.StorefrontCredentials == nil {
				return nil, tasks.ErrCredentials
			}
			plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
			if err != nil {
				slog.Warn("adapter factory: psn decrypt failed", "user_id", cfg.UserID, "err", err)
				return nil, tasks.ErrCredentials
			}
			var creds struct {
				NPSSOToken string `json:"npsso_token"`
			}
			if err := json.Unmarshal(plain, &creds); err != nil {
				return nil, tasks.ErrCredentials
			}
			return psnsvc.NewAdapter(psnsvc.NewClient(), creds.NPSSOToken), nil

		case "gog":
			if cfg.StorefrontCredentials == nil {
				return nil, tasks.ErrCredentials
			}
			plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
			if err != nil {
				slog.Warn("adapter factory: gog decrypt failed", "user_id", cfg.UserID, "err", err)
				return nil, tasks.ErrCredentials
			}
			var creds struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				UserID       string `json:"user_id"`
				Username     string `json:"username"`
			}
			if err := json.Unmarshal(plain, &creds); err != nil {
				return nil, tasks.ErrCredentials
			}
			onNewTokens := func(accessToken, refreshToken string) error {
				creds.AccessToken = accessToken
				creds.RefreshToken = refreshToken
				newCredsJSON, merr := json.Marshal(creds)
				if merr != nil {
					return merr
				}
				enc, encErr := encrypter.Encrypt(newCredsJSON)
				if encErr != nil {
					return encErr
				}
				_, dbErr := db.NewRaw(
					`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
					enc, cfg.UserID,
				).Exec(context.Background())
				return dbErr
			}
			return gogsvc.NewAdapter(gogsvc.NewClient(), creds.RefreshToken, onNewTokens), nil

		case "epic":
			if cfg.StorefrontCredentials == nil {
				return nil, tasks.ErrCredentials
			}
			plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
			if err != nil {
				slog.Warn("adapter factory: epic decrypt failed", "user_id", cfg.UserID, "err", err)
				return nil, tasks.ErrCredentials
			}
			var snapshot map[string]string
			if err := json.Unmarshal(plain, &snapshot); err != nil {
				return nil, tasks.ErrCredentials
			}
			onSnapshot := func(s map[string]string) error {
				newJSON, _ := json.Marshal(s) //nolint:errcheck // marshaling a map[string]string cannot fail
				enc, encErr := encrypter.Encrypt(newJSON)
				if encErr != nil {
					return encErr
				}
				_, dbErr := db.NewRaw(
					`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
					enc, cfg.UserID,
				).Exec(context.Background())
				return dbErr
			}
			return epicsvc.NewAdapter(epicClient, cfg.UserID, snapshot, onSnapshot), nil

		default:
			return nil, fmt.Errorf("unknown storefront: %s", storefront)
		}
	}
}

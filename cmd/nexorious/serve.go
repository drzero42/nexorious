package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/riverqueue/river"
	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/riverqueue/rivercontrib/otelriver"
	"github.com/spf13/cobra"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/extra/bunotel"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/backup"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/crypto"
	nexdb "github.com/drzero42/nexorious/internal/db"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	maint "github.com/drzero42/nexorious/internal/middleware"
	"github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/observability"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/scheduler"
	amazongamessvc "github.com/drzero42/nexorious/internal/services/amazongames"
	epicgamesstoresvc "github.com/drzero42/nexorious/internal/services/epicgamesstore"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	"github.com/drzero42/nexorious/internal/services/igdb"
	playstationstoresvc "github.com/drzero42/nexorious/internal/services/playstationstore"
	"github.com/drzero42/nexorious/internal/services/storelink"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
	"github.com/drzero42/nexorious/internal/storefront"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// riverDrainTimeout bounds how long graceful shutdown waits for in-flight River
// jobs to finish their current unit of work before escalating to a hard cancel.
// Sized to stay within the Kubernetes default terminationGracePeriodSeconds (30s)
// alongside the HTTP server's own graceful timeout and the hard-cancel fallback.
const riverDrainTimeout = 20 * time.Second

// newServeCmd returns the `serve` subcommand, the explicit way to start the
// HTTP server. A bare `./nexorious` prints the help overview instead.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		Long:  "Start the Echo HTTP server, River worker client, and scheduler.",
		RunE:  runServe,
	}
	cmd.Flags().Bool("migrate", false,
		"Run pending database migrations before serving; abort startup if they fail")
	return cmd
}

// runServe contains the historical main() body. It loads .env, opens the
// database, wires the migrator/River client/HTTP server and blocks until
// SIGINT/SIGTERM triggers a graceful shutdown.
func runServe(cmd *cobra.Command, _ []string) error {
	cfg, err := loadEnvAndConfig(cmd)
	if err != nil {
		return err
	}

	var appHandler slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	})
	if cfg.OTELExporterOTLPEndpoint != "" {
		// Tracing enabled: bind trace_id/span_id from the active span onto
		// every log line. Chained only when tracing is on so the off path
		// stays zero-overhead.
		appHandler = observability.NewTraceContextHandler(appHandler)
	}
	slog.SetDefault(slog.New(logging.NewContextHandler(appHandler)))

	// -------------------------------------------------------------------------
	// Observability (metrics + opt-in tracing) — must precede DB + River wiring
	// so the bunotel hook and otelriver middleware can bind to the providers.
	// -------------------------------------------------------------------------
	obs, err := observability.Init(cfg, version)
	if err != nil {
		slog.Error("observability init failed", logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
		os.Exit(1)
	}

	encrypter, err := crypto.NewEncrypter(cfg.DBEncryptionKey)
	if err != nil {
		slog.Error("invalid DB_ENCRYPTION_KEY", logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
		os.Exit(1)
	}

	// -------------------------------------------------------------------------
	// Database
	// -------------------------------------------------------------------------
	ctx := context.Background()

	resolvedDatabaseURL := cfg.DatabaseURL
	db := openBunDB(resolvedDatabaseURL)
	db = db.WithQueryHook(bunotel.NewQueryHook(
		bunotel.WithMeterProvider(obs.MeterProvider),
		bunotel.WithTracerProvider(obs.TracerProvider),
	))
	// Counts failed queries into nexorious_db_errors_total — bunotel records
	// timing but no error signal (#913). No-op when metrics are disabled.
	db = db.WithQueryHook(observability.NewDBErrorHook())
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(nexdb.MaxIdleConns)
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
	// NeedsAdopt and MigrationRefused states are left in place; Gate 2 serves the /migrate page.
	initAppState := func(ctx context.Context) error {
		if err := migrator.DetermineState(); err != nil {
			return fmt.Errorf("initAppState: determineState: %w", err)
		}
		switch migrator.State() {
		case migrate.AppStateReady:
			if err := migrator.InitNeedsSetup(ctx, db); err != nil {
				return fmt.Errorf("initAppState: InitNeedsSetup: %w", err)
			}
		case migrate.AppStateRestoreFailed:
			logRestoreFailed(migrator)
		}
		return nil
	}

	// -------------------------------------------------------------------------
	// Startup DB connection.
	// With --migrate (issue #941): wait for the DB (retrying like the `migrate`
	// subcommand), apply pending migrations, and abort startup on any failure
	// rather than serving in a broken state.
	// Without it: single ping, no retry (StartDBProbe handles retries), and
	// pending migrations leave the app in NeedsMigration for the /migrate UI.
	// -------------------------------------------------------------------------
	migrateOnStart, _ := cmd.Flags().GetBool("migrate") //nolint:errcheck // "migrate" flag is always registered; cannot error
	if migrateOnStart {
		// SIGINT/SIGTERM must be able to cancel the wait/migration phase.
		migCtx, migStop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		err := runStartupMigrations(migCtx, db, migrator, 30*time.Second)
		migStop()
		if err != nil {
			return err
		}
		slog.Info("database connected")
		if err := initAppState(ctx); err != nil {
			return fmt.Errorf("serve --migrate: init app state: %w", err)
		}
	} else {
		pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
		pingErr := db.PingContext(pingCtx)
		pingCancel()
		if pingErr == nil {
			slog.Info("database connected")
			warnIfPrivilegedDBRole(ctx, db)
			if err := initAppState(ctx); err != nil {
				slog.Error("initAppState failed — starting in DBUnavailable state", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
			}
		} else {
			slog.Warn("database not reachable at startup — starting in DBUnavailable state", logging.KeyErr, pingErr, logging.Cat(logging.CategoryDB))
		}
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
		slog.Warn("IGDB credentials not configured — game search, import, and metadata features will be unavailable", logging.Cat(logging.CategoryConfig))
	} else {
		validateCtx, validateCancel := context.WithTimeout(ctx, 10*time.Second)
		err := igdbClient.ValidateCredentials(validateCtx)
		validateCancel()
		if err != nil {
			if igdb.IsAuthError(err) {
				slog.Warn("IGDB credentials are invalid — disabling IGDB features", logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
				igdbClient = igdb.NewInvalidCredentialsClient(igdbLimiter)
			} else {
				slog.Warn("IGDB credential probe failed (network/transient) — IGDB client kept", logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
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
	// Job lifecycle module (#1265): the one owner of Job and item status for
	// the kinds registered with it. It inserts River jobs in its own
	// transactions through an insert-only client on Bun's *sql.DB (ADR 0001),
	// built with the same logger and OpenTelemetry plugin as the working one.
	txRiverClient, err := river.NewClient(riverdatabasesql.New(db.DB), newInsertOnlyRiverConfig(obs))
	if err != nil {
		return fmt.Errorf("river.NewClient (insert-only): %w", err)
	}
	notify.SetTxRiverClient(txRiverClient)
	lifecycle := joblifecycle.New(db, txRiverClient)
	tasks.RegisterLifecycleKinds(lifecycle)

	epicClient := epicgamesstoresvc.NewClient(cfg.LegendaryWorkDir)
	// A sign-in or sync interrupted by a crash leaves its session directory,
	// with the user's Epic credentials, behind (#1387).
	if n := epicClient.RemoveStaleSessions(ctx, epicgamesstoresvc.StaleSessionAge); n > 0 {
		slog.InfoContext(ctx, "epic: removed session directories left by an earlier crash", "count", n)
	}
	dispatchSyncWorker := &tasks.DispatchSyncWorker{
		DB: db,
		LC: lifecycle,
		Adapter: storefront.NewConnections(db, encrypter, storefront.Clients{
			Epic:   epicClient,
			Amazon: amazongamessvc.NewClient(),
			GOG:    gogsvc.NewClient(),
		}).AdapterFactory(),
	}
	metaDispatchWorker := &tasks.MetadataRefreshDispatchWorker{
		DB:         db,
		LC:         lifecycle,
		IGDBClient: igdbClient,
		MinAge:     cfg.MetadataRefreshMinAgeDuration(),
	}
	igdbMatchWorker := &tasks.IGDBMatchWorker{DB: db, LC: lifecycle, IGDBClient: igdbClient}
	userGameWorker := &tasks.UserGameWorker{DB: db, LC: lifecycle, IGDBClient: igdbClient}
	importMatchWorker := &tasks.ImportMatchWorker{DB: db, LC: lifecycle, IGDBClient: igdbClient}
	storeLinkDispatchWorker := &tasks.StoreLinkRefreshDispatchWorker{DB: db, LC: lifecycle}
	storeLinkItemWorker := &tasks.StoreLinkRefreshItemWorker{DB: db, LC: lifecycle, ResolverFor: buildStoreLinkResolverFactory(db, encrypter)}

	updateState := updatecheck.NewState()
	updateClient := updatecheck.NewClient()

	workers := river.NewWorkers()
	river.AddWorker(workers, &tasks.ImportItemWorker{DB: db, LC: lifecycle, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &tasks.ExportJSONWorker{DB: db, LC: lifecycle, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &tasks.ExportCSVWorker{DB: db, LC: lifecycle, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &joblifecycle.EffectsWorker{Module: lifecycle})
	river.AddWorker(workers, &joblifecycle.RecoveryWorker{Module: lifecycle})
	river.AddWorker(workers, dispatchSyncWorker)
	river.AddWorker(workers, igdbMatchWorker)
	river.AddWorker(workers, userGameWorker)
	river.AddWorker(workers, importMatchWorker)
	river.AddWorker(workers, &tasks.ImportFinalizeWorker{DB: db, LC: lifecycle, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, metaDispatchWorker)
	river.AddWorker(workers, storeLinkDispatchWorker)
	river.AddWorker(workers, storeLinkItemWorker)
	river.AddWorker(workers, &tasks.MetadataRefreshItemWorker{DB: db, LC: lifecycle, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &tasks.MetadataFetchWorker{DB: db, IGDBClient: igdbClient, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &scheduler.CleanupOldJobsWorker{DB: db})
	river.AddWorker(workers, &scheduler.CleanupExportsWorker{DB: db, StoragePath: cfg.StoragePath})
	river.AddWorker(workers, &scheduler.CleanupUnreferencedGamesWorker{DB: db})
	river.AddWorker(workers, &scheduler.CleanupExpiredSessionsWorker{DB: db})
	river.AddWorker(workers, &scheduler.CleanupSyncChangesWorker{DB: db})
	river.AddWorker(workers, &scheduler.CheckPendingSyncsWorker{DB: db, LC: lifecycle})
	river.AddWorker(workers, &scheduler.CheckScheduledBackupWorker{DB: db, BackupSvc: backupSvc})
	river.AddWorker(workers, &notify.NotifyWorker{DB: db, Encrypter: encrypter, Sender: notify.NewShoutrrrSender()})
	river.AddWorker(workers, &notify.PruneEventsWorker{DB: db})
	river.AddWorker(workers, &scheduler.CheckForUpdatesWorker{
		DB:             db,
		State:          updateState,
		Client:         updateClient,
		RunningVersion: version,
		Enabled:        cfg.UpdateCheckEnabled,
	})

	// Quiet job kinds: routine periodic-maintenance jobs and high-fan-out per-item
	// workers whose successful completion logs at Debug (failures still Warn) so
	// they don't drown out user-initiated, top-level job outcomes.
	quietJobKinds := scheduler.MaintenanceJobKinds()
	quietJobKinds = append(quietJobKinds, tasks.PerItemJobKinds()...)
	quietJobKinds = append(quietJobKinds, notify.PruneEventsArgs{}.Kind())

	riverClient, err := river.NewClient(riverpgxv5.New(pgxPool), newRiverConfig(cfg, obs, workers, quietJobKinds))
	if err != nil {
		slog.ErrorContext(ctx, "serve: river client init failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return fmt.Errorf("river.NewClient: %w", err)
	}
	notify.SetRiverClient(riverClient)

	// The write stage inserts the immediate metadata fetch, which is not Job
	// work, through the working client.
	userGameWorker.RiverClient = riverClient

	// -------------------------------------------------------------------------
	// HTTP server
	// -------------------------------------------------------------------------
	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// A channel of their own, checked just before a restart after a restore:
	// a process asked to stop must not exec a new image instead.
	termSignals := make(chan os.Signal, 1)
	signal.Notify(termSignals, syscall.SIGINT, syscall.SIGTERM)

	// River runs on a context independent of the OS signal. If River were started
	// on shutdownCtx, SIGTERM would hard-cancel in-flight jobs immediately (River
	// cancels running-job contexts when the Start context is cancelled). Instead we
	// keep River's lifecycle separate and drain it explicitly at the shutdown site.
	riverCtx, riverCancel := context.WithCancel(context.Background())
	defer riverCancel()

	// Restore compatibility window: reject backups newer than this binary
	// (ceiling) and older than the v0.17.1 adopt stepping stone (floor).
	// A pre-v0.17.1 backup is un-adoptable; restoring it would strand the
	// instance in the refuse state, so it is rejected before any DB mutation.
	backupSvc.SetMigrationWindow(migrate.LatestMigration(), migrate.MinRestorableMigration)

	e := api.New(encrypter, cfg, migrator, db, resolvedDatabaseURL, igdbClient, backupSvc, version, commit, updateState,
		api.WithRiverClient(riverClient), api.WithLifecycle(lifecycle))

	// StartDBProbe — polls every 5s, calls initAppState on recovery.
	stopProbe := migrator.StartDBProbe(shutdownCtx, db, initAppState)

	// River start gate — the one place River is started. It waits for Ready &&
	// !NeedsSetup and watches gateCtx, so it stops polling if the process is
	// shutting down or a restore switches it off, but starts River on riverCtx
	// so SIGTERM doesn't hard-cancel jobs the moment they begin.
	gate := riverStartGate{
		ready:       func() bool { return migrator.State() == migrate.AppStateReady && !migrator.NeedsSetup() },
		runRecovery: func() { recoverLifecycleAtStartup(context.Background(), lifecycle) },
		start:       func() error { return riverClient.Start(riverCtx) },
		poll:        2 * time.Second,
		minBackoff:  2 * time.Second,
		maxBackoff:  time.Minute,
	}
	gateCtx, stopGate := context.WithCancel(shutdownCtx)
	defer stopGate()
	gateDone := make(chan struct{})
	go func() {
		defer close(gateDone)
		gate.run(gateCtx)
	}()

	// A restore ends by restarting the process (#1341): normal boot is then the
	// only place that migrates, starts River and hands out the client.
	restartRequested := make(chan struct{}, 1)
	backupSvc.SetRestoreHooks(backup.RestoreHooks{
		SetMaintenance: func() { maint.SetMaintenanceMode(true) },
		StopBackground: func(ctx context.Context) error {
			return stopBackground(ctx, stopGate, gateDone, stopProbe, riverClient.StopAndCancel)
		},
		// The restore terminated every backend, including this pool's idle
		// connections. Drop them before anything queries the restored DB, or
		// each fails its next query with 57P01 (#1284).
		DropIdleConns: func() { nexdb.DropIdleConns(db) },
		// Bring the restored database up to date in place. A restored
		// fully-migrated v0.17.1 dump comes back AppStateNeedsAdopt;
		// ReinitAfterRestore adopts it (and runs any catch-up migrations). An
		// un-adoptable backup can't reach here — the restore floor gate
		// rejects it before applying.
		ReinitMigrator: func(ctx context.Context) error { return migrator.ReinitAfterRestore(ctx, db) },
		Restart: func() {
			select {
			case restartRequested <- struct{}{}:
			default:
			}
		},
	})

	if cfg.PprofEnabled {
		startPprofServer(cfg.PprofAddr)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	sc := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: 10 * time.Second,
		HideBanner:      true,
		HidePort:        true,
	}

	// The server stops on a termination signal or when a restore asks for a
	// restart. Stopping it gracefully is what flushes the restore's response.
	serveCtx, stopServing := context.WithCancel(shutdownCtx)
	defer stopServing()
	var restarting atomic.Bool
	go func() {
		select {
		case <-restartRequested:
			restarting.Store(true)
			slog.Info("restore finished — restarting the server")
			stopServing()
		case <-serveCtx.Done():
		}
	}()

	slog.Info("nexorious starting", "addr", addr, "version", version, "commit", commit)
	if err := sc.Start(serveCtx, e); err != nil {
		slog.Info("server stopped", logging.KeyErr, err)
	}

	if restarting.Load() {
		// Skip the River drain and do not close the database handle or the
		// pgx pool: closing a pool waits for every connection to come back,
		// and a River client that did not stop still holds one. Every
		// descriptor is close-on-exec, so the kernel closes them. The restore
		// removed its temporary directories before its response was written.
		obsShutdownCtx, obsCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := obs.Shutdown(obsShutdownCtx); err != nil {
			slog.Warn("observability shutdown", logging.KeyErr, err)
		}
		obsCancel()
		// os.Exit, not return: returning would run the deferred pool closes.
		os.Exit(restartSelf(termSignals, syscall.Exec))
	}

	// Graceful shutdown: drain in-flight River jobs. River was started on riverCtx
	// (independent of the OS signal), so SIGTERM has not cancelled running jobs —
	// Stop lets each in-flight job finish its current unit of work, bounded by
	// riverDrainTimeout. If the drain budget is exceeded, escalate to StopAndCancel
	// so a stuck job can't hold shutdown past the Kubernetes grace period.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), riverDrainTimeout)
	defer drainCancel()
	if err := riverClient.Stop(drainCtx); err != nil {
		slog.Warn("River graceful drain exceeded budget — cancelling in-flight jobs", logging.KeyErr, err)
		hardCtx, hardCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer hardCancel()
		if err := riverClient.StopAndCancel(hardCtx); err != nil {
			slog.Warn("River hard stop", logging.KeyErr, err)
		}
	}

	// Flush and stop the providers last (tracer first, then meter) so in-flight
	// spans and job/DB metrics from the River drain above are exported.
	obsShutdownCtx, obsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer obsCancel()
	if err := obs.Shutdown(obsShutdownCtx); err != nil {
		slog.Warn("observability shutdown", logging.KeyErr, err)
	}

	slog.Info("shutdown complete")
	return nil
}

// newRiverConfig builds the River client config.
//
// otelriver goes in Plugins, not Middleware: River installs Middleware entries
// only as middleware, so otelriver's MetricEmit hook (producer fetch metrics)
// would never run. River places Plugins ahead of Middleware in the chain, so
// otelriver still wraps the logging middleware and job logs keep the span's
// trace context.
func newRiverConfig(cfg *config.Config, obs *observability.Providers, workers *river.Workers, quietJobKinds []string) *river.Config {
	return &river.Config{
		Logger:  logging.RiverLogger(),
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault:         {MaxWorkers: cfg.WorkerCount},
			tasks.QueueMetadataRefresh: {MaxWorkers: cfg.MetadataRefreshWorkers},
		},
		PeriodicJobs: scheduler.BuildPeriodicJobs(cfg),
		Plugins: []rivertype.Plugin{
			otelriver.NewMiddleware(&otelriver.MiddlewareConfig{
				MeterProvider:  obs.MeterProvider,
				TracerProvider: obs.TracerProvider,
				DurationUnit:   "s",
			}),
		},
		Middleware: []rivertype.Middleware{
			logging.NewWorkerMiddleware(quietJobKinds...),
		},
		ErrorHandler: &logging.WorkerErrorHandler{},
		// Above the longest timeout of any Job lifecycle River kind (a sync
		// dispatch, 2 hours), so River's rescuer never acts at the same
		// moment as a timeout (docs/job-lifecycle.md).
		RescueStuckJobsAfter: riverRescueStuckJobsAfter,
	}
}

// riverRescueStuckJobsAfter is how long a River job may run before River's
// rescuer retries or discards it.
const riverRescueStuckJobsAfter = 3 * time.Hour

// newInsertOnlyRiverConfig is the config of the insert-only River client the
// Job lifecycle module inserts through: no queues and no workers, but the same
// logger and OpenTelemetry plugin as the working client, so its log lines and
// trace links match.
func newInsertOnlyRiverConfig(obs *observability.Providers) *river.Config {
	return &river.Config{
		Logger: logging.RiverLogger(),
		Plugins: []rivertype.Plugin{
			otelriver.NewMiddleware(&otelriver.MiddlewareConfig{
				MeterProvider:  obs.MeterProvider,
				TracerProvider: obs.TracerProvider,
				DurationUnit:   "s",
			}),
		},
	}
}

// recoverLifecycleAtStartup runs before River starts: it reclaims the River
// jobs a dead process left running, then applies the recovery rule. A failing
// step is logged and River is started anyway. Both steps tolerate being run
// again before a start succeeds. A restart never notifies, so no maintenance
// event is emitted here.
func recoverLifecycleAtStartup(ctx context.Context, lc *joblifecycle.Module) {
	n, err := lc.ReclaimRunning(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "startup: reclaim running river jobs failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	} else if n > 0 {
		slog.InfoContext(ctx, "startup: reclaimed river jobs left running", "count", n)
	}
	res, err := lc.Recover(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "startup: job recovery failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	if res.Failed+res.Requeued > 0 {
		slog.InfoContext(ctx, "startup: job recovery", "failed", res.Failed, "requeued", res.Requeued)
	}
}

// runStartupMigrations implements the `serve --migrate` startup path: wait for
// the database (retrying like the `migrate` subcommand), then apply any
// pending application and River migrations. A non-nil error means startup must
// abort — serving with pending or half-applied migrations is never safe.
func runStartupMigrations(ctx context.Context, db *bun.DB, migrator *migrate.Migrator, waitTimeout time.Duration) error {
	if err := waitForDB(ctx, db, waitTimeout); err != nil {
		return fmt.Errorf("serve --migrate: %w", err)
	}
	if err := migrator.DetermineState(); err != nil {
		return fmt.Errorf("serve --migrate: determine state: %w", err)
	}
	switch migrator.State() {
	case migrate.AppStateReady:
		slog.Info("serve --migrate: no pending migrations")
		return nil
	case migrate.AppStateMigrationRefused:
		return fmt.Errorf("serve --migrate: refused — database is not a clean v0.17.1 or baseline install; " +
			"upgrade to v0.17.1 first then v0.90.0+, or export → fresh install → import")
	case migrate.AppStateRestoreFailed:
		// Never migrate a database a restore may have left half-loaded, but do
		// start: the server serves the restore-failed page (#1341).
		slog.Error("serve --migrate: a backup restore did not finish — not migrating", logging.Cat(logging.CategoryDB))
		return nil
	}
	// AppStateNeedsMigration and AppStateNeedsAdopt proceed.
	migrator.SetLogWriter(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Writer())
	if err := migrator.RunMigrations(ctx); err != nil {
		return fmt.Errorf("serve --migrate: run migrations: %w", err)
	}
	slog.Info("serve --migrate: migrations complete")
	return nil
}

// logRestoreFailed logs the restore-failed state with what the marker names.
func logRestoreFailed(migrator *migrate.Migrator) {
	var backupID, preRestore string
	if m := migrator.RestoreMarker(); m != nil {
		backupID, preRestore = m.BackupID, m.PreRestoreBackupID
	}
	slog.Error("a backup restore did not finish — not migrating and not starting background work; "+
		"see \"Backups and restore\" in the admin guide",
		"backup_id", backupID, "pre_restore_backup_id", preRestore, logging.Cat(logging.CategoryDB))
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

// buildStoreLinkResolverFactory returns a factory the store-link enrichment item
// worker uses to build a per-storefront resolver. Steam/GOG/Epic resolution is
// credential-free (public endpoints / a copied appid); only PSN needs the user's
// decrypted NPSSO token.
func buildStoreLinkResolverFactory(db *bun.DB, encrypter *crypto.Encrypter) func(context.Context, string, string) (storelink.Resolver, error) {
	return func(ctx context.Context, sf, userID string) (storelink.Resolver, error) {
		switch sf {
		case "steam":
			return storelink.NewSteamResolver(), nil
		case "gog":
			return storelink.NewGOGResolver(nil, ""), nil
		case "epic-games-store":
			return storelink.NewEpicResolver(nil, ""), nil
		case "playstation-store":
			var encCreds string
			if err := db.NewRaw(
				`SELECT storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = 'playstation-store'`, userID,
			).Scan(ctx, &encCreds); err != nil {
				return nil, fmt.Errorf("load psn creds: %w", err)
			}
			plain, err := encrypter.Decrypt(encCreds)
			if err != nil {
				return nil, fmt.Errorf("decrypt psn creds: %w", err)
			}
			var creds storefront.PlayStationStoreCredentials
			if err := json.Unmarshal(plain, &creds); err != nil {
				return nil, fmt.Errorf("parse psn creds: %w", err)
			}
			return storelink.NewPSNResolver(playstationstoresvc.NewClient(), creds.NPSSOToken), nil
		default:
			return nil, fmt.Errorf("unknown storefront: %s", sf)
		}
	}
}

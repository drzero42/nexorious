package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// emitMaint emits an admin.maintenance.{completed,failed} event.
func emitMaint(ctx context.Context, db *bun.DB, failed bool, p notify.MaintPayload) {
	typ := notify.TypeAdminMaintCompleted
	if failed {
		typ = notify.TypeAdminMaintFailed
	}
	notify.Emit(ctx, db, notify.EmitParams{Type: typ, Scope: notify.ScopeAdmin, Payload: p})
}

// ── CleanupOldJobs ─────────────────────────────────────────────────────────────

type CleanupOldJobsArgs struct{}

func (CleanupOldJobsArgs) Kind() string { return "cleanup_old_jobs" }

type CleanupOldJobsWorker struct {
	river.WorkerDefaults[CleanupOldJobsArgs]
	DB *bun.DB
}

func (w *CleanupOldJobsWorker) Work(ctx context.Context, _ *river.Job[CleanupOldJobsArgs]) error {
	CleanupOldJobs(ctx, w.DB)
	return nil
}

// ── CleanupExports ─────────────────────────────────────────────────────────────

type CleanupExportsArgs struct{}

func (CleanupExportsArgs) Kind() string { return "cleanup_exports" }

type CleanupExportsWorker struct {
	river.WorkerDefaults[CleanupExportsArgs]
	DB          *bun.DB
	StoragePath string
}

func (w *CleanupExportsWorker) Work(ctx context.Context, _ *river.Job[CleanupExportsArgs]) error {
	CleanupExports(ctx, w.DB, w.StoragePath)
	return nil
}

// ── CleanupUnreferencedGames ──────────────────────────────────────────────────

type CleanupUnreferencedGamesArgs struct{}

func (CleanupUnreferencedGamesArgs) Kind() string { return "cleanup_unreferenced_games" }

type CleanupUnreferencedGamesWorker struct {
	river.WorkerDefaults[CleanupUnreferencedGamesArgs]
	DB *bun.DB
}

func (w *CleanupUnreferencedGamesWorker) Work(ctx context.Context, _ *river.Job[CleanupUnreferencedGamesArgs]) error {
	CleanupUnreferencedGames(ctx, w.DB)
	return nil
}

// ── CleanupExpiredSessions ────────────────────────────────────────────────────

type CleanupExpiredSessionsArgs struct{}

func (CleanupExpiredSessionsArgs) Kind() string { return "cleanup_expired_sessions" }

type CleanupExpiredSessionsWorker struct {
	river.WorkerDefaults[CleanupExpiredSessionsArgs]
	DB *bun.DB
}

func (w *CleanupExpiredSessionsWorker) Work(ctx context.Context, _ *river.Job[CleanupExpiredSessionsArgs]) error {
	CleanupExpiredSessions(ctx, w.DB)
	return nil
}

// ── RescueOrphanedPendingItems ────────────────────────────────────────────────

type RescueOrphanedPendingItemsArgs struct{}

func (RescueOrphanedPendingItemsArgs) Kind() string { return "rescue_orphaned_pending_items" }

type RescueOrphanedPendingItemsWorker struct {
	river.WorkerDefaults[RescueOrphanedPendingItemsArgs]
	DB          *bun.DB
	RiverClient *river.Client[pgx.Tx]
}

func (w *RescueOrphanedPendingItemsWorker) Work(ctx context.Context, _ *river.Job[RescueOrphanedPendingItemsArgs]) error {
	RescueOrphanedPendingItems(ctx, w.DB, w.RiverClient, time.Hour)
	return nil
}

// ── CleanupStaleJobs ──────────────────────────────────────────────────────────

type CleanupStaleJobsArgs struct {
	Threshold string `json:"threshold"`
}

func (CleanupStaleJobsArgs) Kind() string { return "cleanup_stale_jobs" }

type CleanupStaleJobsWorker struct {
	river.WorkerDefaults[CleanupStaleJobsArgs]
	DB *bun.DB
}

func (w *CleanupStaleJobsWorker) Work(ctx context.Context, job *river.Job[CleanupStaleJobsArgs]) error {
	d, err := time.ParseDuration(job.Args.Threshold)
	if err != nil {
		slog.WarnContext(ctx, "cleanup_stale_jobs: invalid threshold, defaulting to 4h", "threshold", job.Args.Threshold, logging.KeyErr, err)
		d = 4 * time.Hour
	}
	CleanupStaleJobs(ctx, w.DB, d)
	return nil
}

// ── CheckPendingSyncs ─────────────────────────────────────────────────────────

type CheckPendingSyncsArgs struct{}

func (CheckPendingSyncsArgs) Kind() string { return "check_pending_syncs" }

type CheckPendingSyncsWorker struct {
	river.WorkerDefaults[CheckPendingSyncsArgs]
	DB          *bun.DB
	RiverClient *river.Client[pgx.Tx]
}

func (w *CheckPendingSyncsWorker) Work(ctx context.Context, _ *river.Job[CheckPendingSyncsArgs]) error {
	var configs []models.UserSyncConfig
	if err := w.DB.NewSelect().Model(&configs).Where("frequency != 'manual'").Scan(ctx); err != nil {
		slog.ErrorContext(ctx, "CheckPendingSyncs: query configs", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		return nil
	}

	now := time.Now().UTC()
	intervals := map[string]time.Duration{
		"hourly": time.Hour,
		"daily":  24 * time.Hour,
		"weekly": 7 * 24 * time.Hour,
	}

	for _, cfg := range configs {
		interval, known := intervals[cfg.Frequency]
		needsSync := false
		if cfg.LastSyncedAt == nil {
			needsSync = true
		} else if known {
			needsSync = now.Sub(*cfg.LastSyncedAt) >= interval
		}

		if !needsSync {
			continue
		}
		if !known {
			interval = 24 * time.Hour
		}
		if wait, backingOff := syncBackoff(ctx, w.DB, cfg, interval, now); backingOff {
			slog.DebugContext(ctx, "CheckPendingSyncs: store is failing; backing off", logging.KeyUserID, cfg.UserID, logging.KeySource, cfg.Storefront, "retry_in", wait.Round(time.Second))
			continue
		}

		// Guard + insert run in one transaction fronted by a transaction-scoped
		// advisory lock on the (sync, storefront, user) dedup key (same key the
		// manual-trigger handler uses), so a scheduler tick and a concurrent
		// manual trigger can't both pass the guard and create duplicate active
		// sync jobs for the same user+storefront.
		jobID := uuid.NewString()
		var inserted bool
		txErr := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if e := tasks.AcquireJobDedupLock(ctx, tx, "sync", cfg.Storefront, cfg.UserID); e != nil {
				return e
			}
			var existingID string
			e := tx.NewRaw(
				`SELECT id FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = ? AND status IN ('pending', 'processing') LIMIT 1`,
				cfg.UserID, cfg.Storefront,
			).Scan(ctx, &existingID)
			if e == nil {
				return nil // already running; leave inserted=false
			}
			if !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			if _, e := tx.NewRaw(
				`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at) VALUES (?, ?, 'sync', ?, 'pending', 'low', now())`,
				jobID, cfg.UserID, cfg.Storefront,
			).Exec(ctx); e != nil {
				return e
			}
			inserted = true
			return nil
		})
		if txErr != nil {
			slog.ErrorContext(ctx, "CheckPendingSyncs: create job", logging.Cat(logging.CategoryDB), logging.KeyErr, txErr, logging.KeyUserID, cfg.UserID, logging.KeySource, cfg.Storefront)
			continue
		}
		if !inserted {
			continue // already running
		}

		if _, err := w.RiverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID:      jobID,
			UserID:     cfg.UserID,
			Storefront: cfg.Storefront,
		}, nil); err != nil {
			slog.ErrorContext(ctx, "CheckPendingSyncs: enqueue dispatch failed", logging.KeyErr, err, logging.KeyJobID, jobID, logging.KeyUserID, cfg.UserID, logging.Cat(logging.CategoryDB))
		}
	}
	return nil
}

// ── CleanupSyncChanges ────────────────────────────────────────────────────────

type CleanupSyncChangesArgs struct {
	RetentionDays int `json:"retention_days"`
}

func (CleanupSyncChangesArgs) Kind() string { return "cleanup_changes" }

type CleanupSyncChangesWorker struct {
	river.WorkerDefaults[CleanupSyncChangesArgs]
	DB *bun.DB
}

func (w *CleanupSyncChangesWorker) Work(ctx context.Context, job *river.Job[CleanupSyncChangesArgs]) error {
	CleanupSyncChanges(ctx, w.DB, job.Args.RetentionDays)
	return nil
}

// CleanupSyncChanges deletes changes rows older than retentionDays days.
func CleanupSyncChanges(ctx context.Context, db *bun.DB, retentionDays int) {
	result, err := db.NewRaw(
		`DELETE FROM changes WHERE created_at < now() - make_interval(days => ?)`,
		retentionDays,
	).Exec(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: failed to delete old changes", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows > 0 {
		slog.InfoContext(ctx, "cleanup: deleted old changes", "count", rows)
	}
}

// ── BuildPeriodicJobs ─────────────────────────────────────────────────────────

// mustCron parses a standard cron expression and panics on error.
// Used only at startup with hard-coded expressions.
func mustCron(expr string) cron.Schedule {
	s, err := cron.ParseStandard(expr)
	if err != nil {
		panic("scheduler: invalid cron expression " + expr + ": " + err.Error())
	}
	return s
}

// MaintenanceJobKinds returns the River job kinds for periodic, system-scheduled
// maintenance jobs. Their routine successful completions are logged at Debug by
// logging.WorkerMiddleware so they don't drown out user-initiated job outcomes.
func MaintenanceJobKinds() []string {
	return []string{
		CheckForUpdatesArgs{}.Kind(),
		CheckScheduledBackupArgs{}.Kind(),
		CleanupOldJobsArgs{}.Kind(),
		CleanupExportsArgs{}.Kind(),
		CleanupUnreferencedGamesArgs{}.Kind(),
		CleanupExpiredSessionsArgs{}.Kind(),
		RescueOrphanedPendingItemsArgs{}.Kind(),
		CleanupStaleJobsArgs{}.Kind(),
		CheckPendingSyncsArgs{}.Kind(),
		CleanupSyncChangesArgs{}.Kind(),
		joblifecycle.RecoveryArgs{}.Kind(),
	}
}

// BuildPeriodicJobs returns the list of River periodic jobs for the scheduler.
// staleThreshold is passed from config (already parsed).
func BuildPeriodicJobs(cfg *config.Config, staleThreshold time.Duration) []*river.PeriodicJob {
	interval, err := time.ParseDuration(cfg.MetadataRefreshInterval)
	if err != nil {
		slog.Warn("scheduler: invalid METADATA_REFRESH_INTERVAL, defaulting to 24h",
			"value", cfg.MetadataRefreshInterval, logging.KeyErr, err)
		interval = 24 * time.Hour
	}

	jobs := []*river.PeriodicJob{
		river.NewPeriodicJob(
			mustCron("0 3 * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupOldJobsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			mustCron("0 4 * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupExportsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			mustCron("0 5 * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupUnreferencedGamesArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			mustCron("*/30 * * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CleanupExpiredSessionsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			mustCron("0 * * * *"),
			func() (river.JobArgs, *river.InsertOpts) {
				return CleanupStaleJobsArgs{Threshold: staleThreshold.String()}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			mustCron("*/15 * * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CheckPendingSyncsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			mustCron("*/30 * * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return RescueOrphanedPendingItemsArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		// The Job lifecycle's recovery rule, for the kinds on the module
		// (startup runs it once more before River starts).
		river.NewPeriodicJob(
			mustCron("*/5 * * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return joblifecycle.RecoveryArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			mustCron("0 2 * * *"),
			func() (river.JobArgs, *river.InsertOpts) {
				return CleanupSyncChangesArgs{RetentionDays: cfg.SyncHistoryRetentionDays}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(interval),
			func() (river.JobArgs, *river.InsertOpts) { return tasks.MetadataRefreshDispatchArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(time.Minute),
			func() (river.JobArgs, *river.InsertOpts) { return CheckScheduledBackupArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			mustCron("0 5 * * *"),
			func() (river.JobArgs, *river.InsertOpts) {
				return notify.PruneEventsArgs{RetentionDays: cfg.NotifyEventsRetentionDays}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
	}

	if cfg.UpdateCheckEnabled {
		// Unlike the other periodic jobs, RunOnStart is true so the sidebar
		// notice appears shortly after boot; the per-release dedup key keeps
		// the admin notification at once per release regardless.
		jobs = append(jobs, river.NewPeriodicJob(
			mustCron("0 */6 * * *"),
			func() (river.JobArgs, *river.InsertOpts) { return CheckForUpdatesArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true},
		))
	}

	return jobs
}

// ── Standalone worker functions ────────────────────────────────────────────────

// CleanupOldJobs deletes terminal jobs older than 30 days and their items (CASCADE).
func CleanupOldJobs(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(
		`DELETE FROM jobs
		 WHERE status IN ('completed', 'failed', 'cancelled')
		   AND completed_at < now() - interval '30 days'`,
	).Exec(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: failed to delete old jobs", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows > 0 {
		slog.InfoContext(ctx, "cleanup: deleted old jobs", "count", rows)
	}
}

// exportOrphanAge is how old a file in the exports directory that no Job
// refers to must be before the sweep removes it: older than any export can run.
const exportOrphanAge = 24 * time.Hour

// CleanupExports deletes expired export files (>24h) and clears their
// file_path, then removes files in the exports directory that no Job refers
// to and that are older than a day: a partial file left by a kill, or the file
// of a run whose transition was refused after its writer could no longer tell
// whether it had committed.
func CleanupExports(ctx context.Context, db *bun.DB, storagePath string) {
	cleanupExpiredExports(ctx, db)
	if storagePath != "" {
		sweepExportsDir(ctx, db, filepath.Join(storagePath, "exports"))
	}
}

func cleanupExpiredExports(ctx context.Context, db *bun.DB) {
	var jobs []struct {
		ID       string  `bun:"id"`
		FilePath *string `bun:"file_path"`
	}
	err := db.NewRaw(`
		SELECT id, file_path FROM jobs
		WHERE job_type = 'export' AND status = 'completed'
		  AND file_path IS NOT NULL AND completed_at < now() - interval '24 hours'`,
	).Scan(ctx, &jobs)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: failed to query expired exports", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	for _, j := range jobs {
		if j.FilePath != nil {
			if err := os.Remove(*j.FilePath); err != nil && !os.IsNotExist(err) {
				slog.WarnContext(ctx, "cleanup: failed to remove export file", "path", *j.FilePath, logging.KeyErr, err)
			}
		}
	}

	ids := make([]string, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	if _, err := db.NewRaw(
		`UPDATE jobs SET file_path = NULL WHERE id IN (?)`,
		bun.List(ids),
	).Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "cleanup: clear expired export file_paths failed", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
	}
	slog.InfoContext(ctx, "cleanup: cleaned expired exports", "count", len(jobs))
}

func sweepExportsDir(ctx context.Context, db *bun.DB, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.WarnContext(ctx, "cleanup: read exports directory", "path", dir, logging.KeyErr, err)
		}
		return
	}
	var referenced []string
	if err := db.NewRaw(`SELECT file_path FROM jobs WHERE job_type = 'export' AND file_path IS NOT NULL`).Scan(ctx, &referenced); err != nil {
		slog.ErrorContext(ctx, "cleanup: query export file paths", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		return
	}
	keep := make(map[string]bool, len(referenced))
	for _, p := range referenced {
		keep[filepath.Clean(p)] = true
	}
	removed := 0
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if keep[path] {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < exportOrphanAge {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.WarnContext(ctx, "cleanup: remove unreferenced export file", "path", path, logging.KeyErr, err)
			continue
		}
		removed++
	}
	if removed > 0 {
		slog.InfoContext(ctx, "cleanup: removed unreferenced export files", "count", removed)
	}
}

// CleanupUnreferencedGames deletes games with no user_games rows.
func CleanupUnreferencedGames(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(
		`DELETE FROM games
		 WHERE id NOT IN (SELECT DISTINCT game_id FROM user_games)`,
	).Exec(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: failed to delete unreferenced games", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows > 0 {
		slog.InfoContext(ctx, "cleanup: deleted unreferenced games", "count", rows)
	}
}

// CleanupExpiredSessions deletes expired user_sessions rows.
func CleanupExpiredSessions(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(
		`DELETE FROM user_sessions WHERE expires_at < now()`,
	).Exec(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup: failed to delete expired sessions", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows > 0 {
		slog.InfoContext(ctx, "cleanup: deleted expired sessions", "count", rows)
	}
}

// syncRetryBase is the wait after a store's first failed sync. Each further
// failure doubles it, up to the store's sync interval (#1344).
const syncRetryBase = 15 * time.Minute

// syncRetryDelay is how long to wait after the newest of failures consecutive
// failed syncs: syncRetryBase doubled per earlier failure, capped at interval.
func syncRetryDelay(failures int, interval time.Duration) time.Duration {
	d := syncRetryBase
	for i := 1; i < failures && d < interval; i++ {
		d *= 2
	}
	return min(d, interval)
}

// syncBackoff reports whether a store whose syncs have been failing should
// wait before its next scheduled sync, and for how long. The failures counted
// are the store's failed syncs since its last successful one, manual or
// scheduled, credential failures included. A failed lookup never holds a sync
// back.
func syncBackoff(ctx context.Context, db *bun.DB, cfg models.UserSyncConfig, interval time.Duration, now time.Time) (time.Duration, bool) {
	var row struct {
		Failures   int        `bun:"failures"`
		LastFailed *time.Time `bun:"last_failed"`
	}
	if err := db.NewRaw(
		`SELECT COUNT(*) AS failures, MAX(completed_at) AS last_failed FROM jobs
		 WHERE user_id = ? AND source = ? AND job_type = 'sync' AND status = 'failed'
		   AND completed_at IS NOT NULL
		   AND (?::timestamptz IS NULL OR completed_at > ?::timestamptz)`,
		cfg.UserID, cfg.Storefront, cfg.LastSyncedAt, cfg.LastSyncedAt,
	).Scan(ctx, &row); err != nil {
		slog.WarnContext(ctx, "CheckPendingSyncs: count failed syncs", logging.Cat(logging.CategoryDB), logging.KeyErr, err, logging.KeyUserID, cfg.UserID, logging.KeySource, cfg.Storefront)
		return 0, false
	}
	if row.Failures == 0 || row.LastFailed == nil {
		return 0, false
	}
	wait := row.LastFailed.Add(syncRetryDelay(row.Failures, interval)).Sub(now)
	return wait, wait > 0
}

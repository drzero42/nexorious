package scheduler

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

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
	DB *bun.DB
}

func (w *CleanupExportsWorker) Work(ctx context.Context, _ *river.Job[CleanupExportsArgs]) error {
	CleanupExports(ctx, w.DB)
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
		slog.Warn("cleanup_stale_jobs: invalid threshold, defaulting to 4h", "threshold", job.Args.Threshold, "err", err)
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
		slog.Error("CheckPendingSyncs: query configs", "err", err)
		return nil
	}

	now := time.Now().UTC()
	intervals := map[string]float64{
		"hourly": 3600,
		"daily":  86400,
		"weekly": 604800,
	}

	for _, cfg := range configs {
		if cfg.Storefront == "epic" {
			continue
		}

		needsSync := false
		if cfg.LastSyncedAt == nil {
			needsSync = true
		} else if threshold, ok := intervals[cfg.Frequency]; ok {
			needsSync = now.Sub(*cfg.LastSyncedAt).Seconds() >= threshold
		}

		if !needsSync {
			continue
		}

		var existingID string
		err := w.DB.NewRaw(
			`SELECT id FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = ? AND status IN ('pending', 'processing') LIMIT 1`,
			cfg.UserID, cfg.Storefront,
		).Scan(ctx, &existingID)
		if err == nil {
			continue // already running
		}

		jobID := uuid.NewString()
		_, err = w.DB.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at) VALUES (?, ?, 'sync', ?, 'pending', 'low', now())`,
			jobID, cfg.UserID, cfg.Storefront,
		).Exec(ctx)
		if err != nil {
			slog.Error("CheckPendingSyncs: insert job", "err", err)
			continue
		}

		_, _ = w.RiverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID:      jobID,
			UserID:     cfg.UserID,
			Storefront: cfg.Storefront,
		}, nil)
	}
	return nil
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

// BuildPeriodicJobs returns the list of River periodic jobs for the scheduler.
// staleThreshold is passed from config (already parsed).
func BuildPeriodicJobs(cfg *config.Config, staleThreshold time.Duration) []*river.PeriodicJob {
	interval, err := time.ParseDuration(cfg.MetadataRefreshInterval)
	if err != nil {
		slog.Warn("scheduler: invalid METADATA_REFRESH_INTERVAL, defaulting to 24h",
			"value", cfg.MetadataRefreshInterval, "err", err)
		interval = 24 * time.Hour
	}

	return []*river.PeriodicJob{
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
			river.PeriodicInterval(interval),
			func() (river.JobArgs, *river.InsertOpts) { return tasks.MetadataRefreshDispatchArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(time.Minute),
			func() (river.JobArgs, *river.InsertOpts) { return CheckScheduledBackupArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: false},
		),
	}
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
		slog.Error("cleanup: failed to delete old jobs", "err", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("cleanup: deleted old jobs", "count", rows)
	}
}

// CleanupExports deletes expired export files (>24h) and clears their file_path.
func CleanupExports(ctx context.Context, db *bun.DB) {
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
		slog.Error("cleanup: failed to query expired exports", "err", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	for _, j := range jobs {
		if j.FilePath != nil {
			if err := os.Remove(*j.FilePath); err != nil && !os.IsNotExist(err) {
				slog.Warn("cleanup: failed to remove export file", "path", *j.FilePath, "err", err)
			}
		}
	}

	ids := make([]string, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	_, _ = db.NewRaw(
		`UPDATE jobs SET file_path = NULL WHERE id IN (?)`,
		bun.List(ids),
	).Exec(ctx)
	slog.Info("cleanup: cleaned expired exports", "count", len(jobs))
}

// CleanupUnreferencedGames deletes games with no user_games rows.
func CleanupUnreferencedGames(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(
		`DELETE FROM games
		 WHERE id NOT IN (SELECT DISTINCT game_id FROM user_games)`,
	).Exec(ctx)
	if err != nil {
		slog.Error("cleanup: failed to delete unreferenced games", "err", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("cleanup: deleted unreferenced games", "count", rows)
	}
}

// CleanupExpiredSessions deletes expired user_sessions rows.
func CleanupExpiredSessions(ctx context.Context, db *bun.DB) {
	result, err := db.NewRaw(
		`DELETE FROM user_sessions WHERE expires_at < now()`,
	).Exec(ctx)
	if err != nil {
		slog.Error("cleanup: failed to delete expired sessions", "err", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("cleanup: deleted expired sessions", "count", rows)
	}
}

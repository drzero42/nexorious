package scheduler

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/backup"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker"
)

type Scheduler struct {
	db                      *bun.DB
	pool                    *worker.Pool
	backupSvc               *backup.Service
	metadataRefreshInterval time.Duration
	staleJobThreshold       time.Duration
	scheduler               gocron.Scheduler
	backupJob               gocron.Job
}

func NewScheduler(db *bun.DB, pool *worker.Pool, backupSvc *backup.Service, cfg *config.Config) *Scheduler {
	interval, err := time.ParseDuration(cfg.MetadataRefreshInterval)
	if err != nil {
		slog.Warn("scheduler: invalid METADATA_REFRESH_INTERVAL, defaulting to 24h",
			"value", cfg.MetadataRefreshInterval, "err", err)
		interval = 24 * time.Hour
	}
	staleThreshold, err := time.ParseDuration(cfg.StaleJobThreshold)
	if err != nil {
		slog.Warn("scheduler: invalid STALE_JOB_THRESHOLD, defaulting to 4h",
			"value", cfg.StaleJobThreshold, "err", err)
		staleThreshold = 4 * time.Hour
	}
	return &Scheduler{
		db:                      db,
		pool:                    pool,
		backupSvc:               backupSvc,
		metadataRefreshInterval: interval,
		staleJobThreshold:       staleThreshold,
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	sched, err := gocron.NewScheduler()
	if err != nil {
		return err
	}
	s.scheduler = sched

	// Cleanup old job results — daily at 3:00 AM UTC.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("0 3 * * *", false),
		gocron.NewTask(func() {
			CleanupOldJobs(ctx, s.db)
		}),
	)

	// Cleanup exports — daily at 4:00 AM UTC.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("0 4 * * *", false),
		gocron.NewTask(func() {
			CleanupExports(ctx, s.db)
		}),
	)

	// Cleanup unreferenced games — daily at 5:00 AM UTC.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("0 5 * * *", false),
		gocron.NewTask(func() {
			CleanupUnreferencedGames(ctx, s.db)
		}),
	)

	// Cleanup expired sessions — every 30 minutes.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("*/30 * * * *", false),
		gocron.NewTask(func() {
			CleanupExpiredSessions(ctx, s.db)
		}),
	)

	// Check pending syncs — every 15 minutes.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("*/15 * * * *", false),
		gocron.NewTask(func() {
			CheckPendingSyncs(ctx, s.db, s.pool)
		}),
	)

	// Cleanup stale metadata_refresh jobs — hourly.
	_, _ = s.scheduler.NewJob(
		gocron.CronJob("0 * * * *", false),
		gocron.NewTask(func() {
			CleanupStaleJobs(ctx, s.db, s.staleJobThreshold)
		}),
	)

	// Metadata refresh dispatch — configurable interval.
	_, _ = s.scheduler.NewJob(
		gocron.DurationJob(s.metadataRefreshInterval),
		gocron.NewTask(func() {
			_ = s.pool.Submit(ctx, "metadata_refresh_dispatch", nil, 1)
		}),
	)

	// Scheduled backup job — reads config from DB.
	if s.backupSvc != nil && backup.PgDumpAvailable() {
		var cfg models.BackupConfig
		if err := s.db.NewSelect().Model(&cfg).Where("id = 1").Scan(ctx); err != nil {
			slog.Warn("scheduler: could not read backup_config", "err", err)
		} else if cfg.ScheduleCron != "" {
			s.registerBackupJob(ctx, cfg.ScheduleCron, cfg.RetentionMode, cfg.RetentionValue)
		}
	} else if s.backupSvc != nil {
		slog.Warn("scheduler: pg_dump not available — skipping scheduled backup job")
	}

	s.scheduler.Start()
	slog.Info("scheduler started")
	return nil
}

func (s *Scheduler) registerBackupJob(_ context.Context, cron, retentionMode string, retentionValue int) {
	job, err := s.scheduler.NewJob(
		gocron.CronJob(cron, false),
		gocron.NewTask(func() {
			id, err := s.backupSvc.CreateBackup("scheduled")
			if err != nil {
				slog.Error("scheduled backup failed", "err", err)
				return
			}
			slog.Info("scheduled backup created", "id", id)
			if err := s.backupSvc.ApplyRetention(retentionMode, retentionValue); err != nil {
				slog.Warn("scheduled backup retention cleanup failed", "err", err)
			}
		}),
	)
	if err != nil {
		slog.Error("scheduler: failed to register backup job", "err", err)
		return
	}
	s.backupJob = job
	slog.Info("scheduler: backup job registered", "cron", cron)
}

// RebuildBackupJob removes the existing backup job and registers a new one with updated config.
// Call this after the backup config is updated via the API.
func (s *Scheduler) RebuildBackupJob(ctx context.Context, cron, retentionMode string, retentionValue int) {
	if s.backupJob != nil {
		_ = s.scheduler.RemoveJob(s.backupJob.ID())
		s.backupJob = nil
	}
	if cron != "" && backup.PgDumpAvailable() {
		s.registerBackupJob(ctx, cron, retentionMode, retentionValue)
	}
}

func (s *Scheduler) Stop() {
	if s.scheduler != nil {
		_ = s.scheduler.Shutdown()
		slog.Info("scheduler stopped")
	}
}

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

// CheckPendingSyncs dispatches sync jobs for overdue non-manual sync configs.
func CheckPendingSyncs(ctx context.Context, db *bun.DB, pool *worker.Pool) {
	var configs []models.UserSyncConfig
	if err := db.NewSelect().Model(&configs).Where("frequency != 'manual'").Scan(ctx); err != nil {
		slog.Error("CheckPendingSyncs: query configs", "err", err)
		return
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
		err := db.NewRaw(
			`SELECT id FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = ? AND status IN ('pending', 'processing') LIMIT 1`,
			cfg.UserID, cfg.Storefront,
		).Scan(ctx, &existingID)
		if err == nil {
			continue // already running
		}

		jobID := uuid.NewString()
		_, err = db.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at) VALUES (?, ?, 'sync', ?, 'pending', 'low', now())`,
			jobID, cfg.UserID, cfg.Storefront,
		).Exec(ctx)
		if err != nil {
			slog.Error("CheckPendingSyncs: insert job", "err", err)
			continue
		}

		_ = pool.Submit(ctx, "dispatch_sync", map[string]string{
			"job_id": jobID, "user_id": cfg.UserID, "storefront": cfg.Storefront,
		}, 1)
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

package scheduler

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-co-op/gocron/v2"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/worker"
)

type Scheduler struct {
	db        *bun.DB
	pool      *worker.Pool
	scheduler gocron.Scheduler
}

func NewScheduler(db *bun.DB, pool *worker.Pool) *Scheduler {
	return &Scheduler{db: db, pool: pool}
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

	s.scheduler.Start()
	slog.Info("scheduler started")
	return nil
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

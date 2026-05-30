package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/backup"
	"github.com/drzero42/nexorious/internal/db/models"
)

type CheckScheduledBackupArgs struct{}

func (CheckScheduledBackupArgs) Kind() string { return "check_scheduled_backup" }

type CheckScheduledBackupWorker struct {
	river.WorkerDefaults[CheckScheduledBackupArgs]
	DB        *bun.DB
	BackupSvc *backup.Service
}

func (w *CheckScheduledBackupWorker) Work(ctx context.Context, _ *river.Job[CheckScheduledBackupArgs]) error {
	if !backup.PgDumpAvailable() {
		return nil
	}
	var cfg models.BackupConfig
	if err := w.DB.NewSelect().Model(&cfg).Where("id = 1").Scan(ctx); err != nil || cfg.ScheduleCron == "" {
		return nil
	}
	sched, err := cron.ParseStandard(cfg.ScheduleCron)
	if err != nil {
		slog.Warn("check_scheduled_backup: invalid cron expression", "cron", cfg.ScheduleCron, "err", err)
		return nil
	}
	now := time.Now().UTC()
	if cfg.LastBackupAt != nil && now.Before(sched.Next(*cfg.LastBackupAt)) {
		return nil
	}
	id, err := w.BackupSvc.CreateBackup("scheduled")
	if err != nil {
		slog.Error("scheduled backup failed", "err", err)
		return nil
	}
	slog.Info("scheduled backup created", "id", id)
	if err := w.BackupSvc.ApplyRetention(cfg.RetentionMode, cfg.RetentionValue); err != nil {
		slog.Warn("scheduled backup retention cleanup failed", "err", err)
	}
	if _, err := w.DB.NewRaw(
		`UPDATE backup_config SET last_backup_at = now(), updated_at = now() WHERE id = 1`,
	).Exec(context.Background()); err != nil {
		slog.Error("check_scheduled_backup: update last_backup_at failed", "err", err)
	}
	return nil
}

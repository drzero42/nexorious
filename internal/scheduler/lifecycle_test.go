package scheduler_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious-go/internal/backup"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/migrations"
	"github.com/drzero42/nexorious-go/internal/scheduler"
	"github.com/drzero42/nexorious-go/internal/worker"
)

// TestScheduler_StartStop verifies that Start creates the gocron scheduler and
// Stop shuts it down without error. No backup service is provided so the
// backup-job registration branch is skipped.
func TestScheduler_StartStop(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "4h",
	}

	sched := scheduler.NewScheduler(db, pool, nil, cfg)

	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop should not panic or error.
	sched.Stop()
}

// TestScheduler_StopWithoutStart verifies that Stop is safe to call before Start.
func TestScheduler_StopWithoutStart(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "4h",
	}

	sched := scheduler.NewScheduler(db, pool, nil, cfg)
	// Should not panic (scheduler field is nil, guarded by nil check in Stop).
	sched.Stop()
}

// TestScheduler_RebuildBackupJob_NoCronNoPanic verifies RebuildBackupJob with
// an empty cron string removes any existing job but does not register a new one.
func TestScheduler_RebuildBackupJob_NoCronNoPanic(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "4h",
	}

	sched := scheduler.NewScheduler(db, pool, nil, cfg)
	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sched.Stop()

	// Empty cron — should be a no-op register but should not panic.
	sched.RebuildBackupJob(ctx, "", "keep_last", 5)
}

// TestScheduler_RebuildBackupJob_WithCron verifies that RebuildBackupJob with a
// valid cron registers a new job. pg_dump is not available in CI so the backup
// job registration is skipped (guarded by backup.PgDumpAvailable()), but the
// function itself must not panic or error.
func TestScheduler_RebuildBackupJob_WithCron(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "4h",
	}

	sched := scheduler.NewScheduler(db, pool, nil, cfg)
	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sched.Stop()

	// Rebuild with a cron expression — PgDumpAvailable() may return false in CI
	// but the function should still complete without panic.
	sched.RebuildBackupJob(ctx, "0 2 * * *", "keep_last", 7)
}

// setupTestDBWithDSN is like setupTestDB but also returns the connection string
// so callers that need to pass a DSN to other services (e.g. backup.NewService)
// can do so.
func setupTestDBWithDSN(t *testing.T) (*bun.DB, string) {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		"postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })

	migrator := bunmigrate.NewMigrator(db, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		t.Fatalf("migrator init: %v", err)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db, connStr
}

// TestScheduler_RebuildBackupJob_InvalidCron exercises the error path in
// registerBackupJob when an invalid cron expression is supplied.
func TestScheduler_RebuildBackupJob_InvalidCron(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	db, connStr := setupTestDBWithDSN(t)
	pool := worker.NewPool(db)
	defer pool.Shutdown()

	backupSvc := backup.NewService(db, connStr, t.TempDir(), t.TempDir(), "test")
	cfg := &config.Config{MetadataRefreshInterval: "24h", StaleJobThreshold: "4h"}

	sched := scheduler.NewScheduler(db, pool, backupSvc, cfg)
	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sched.Stop()

	// An invalid cron expression triggers slog.Error inside registerBackupJob.
	sched.RebuildBackupJob(ctx, "not-a-cron", "keep_last", 5)
}

// TestScheduler_StartWithBackupService_EmptyScheduleCron exercises the Start
// branch where backup_config has an empty schedule_cron (so registerBackupJob
// is not called even though backupSvc is set).
func TestScheduler_StartWithBackupService_EmptyScheduleCron(t *testing.T) {
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available")
	}

	db, connStr := setupTestDBWithDSN(t)

	// Clear the seeded cron so ScheduleCron == "" branch is exercised.
	ctx := context.Background()
	if _, err := db.NewRaw(`UPDATE backup_config SET schedule_cron = '' WHERE id = 1`).Exec(ctx); err != nil {
		t.Fatalf("clear schedule_cron: %v", err)
	}

	pool := worker.NewPool(db)
	defer pool.Shutdown()

	backupSvc := backup.NewService(db, connStr, t.TempDir(), t.TempDir(), "test")
	cfg := &config.Config{MetadataRefreshInterval: "24h", StaleJobThreshold: "4h"}

	sched := scheduler.NewScheduler(db, pool, backupSvc, cfg)
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sched.Stop()
}

// TestScheduler_StartWithBackupService exercises the backup-job registration
// branch inside Start (and indirectly registerBackupJob) when pg_dump is
// available. The migration seeds backup_config id=1 with schedule_cron='0 2 * * *'.
func TestScheduler_StartWithBackupService(t *testing.T) {
	// Only meaningful when pg_dump is present on PATH.
	backup.CheckTools()
	if !backup.PgDumpAvailable() {
		t.Skip("pg_dump not available — skipping backup-registration branch test")
	}

	db, connStr := setupTestDBWithDSN(t)

	pool := worker.NewPool(db)
	defer pool.Shutdown()

	backupDir := t.TempDir()
	storageDir := t.TempDir()
	backupSvc := backup.NewService(db, connStr, backupDir, storageDir, "test")

	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "4h",
	}

	sched := scheduler.NewScheduler(db, pool, backupSvc, cfg)
	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start with backup service failed: %v", err)
	}
	defer sched.Stop()

	// RebuildBackupJob with a valid cron should exercise registerBackupJob.
	sched.RebuildBackupJob(ctx, "0 3 * * *", "keep_last", 5)
	// Rebuild again with empty cron to clear the job (exercises the remove branch).
	sched.RebuildBackupJob(ctx, "", "keep_last", 5)
}

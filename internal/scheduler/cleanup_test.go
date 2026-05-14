package scheduler_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/scheduler"
	"github.com/drzero42/nexorious-go/internal/worker"
)

// badDB returns a bun.DB connected to a non-existent host, so all queries fail.
func badDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb := sql.OpenDB(pgdriver.NewConnector(
		pgdriver.WithDSN("postgres://test:test@127.0.0.1:1/nonexistent?sslmode=disable"),
		pgdriver.WithTimeout(0), // immediate timeout
	))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// CleanupExports
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// CleanupOldJobs
// ---------------------------------------------------------------------------

func TestCleanupOldJobs_DeletesExpiredJobs(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Insert a job that is >30 days old and completed — should be deleted.
	_, err := db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, completed_at)
		 VALUES ('old-job-1', ?, 'export', 'manual', 'completed', 'low', now() - interval '31 days')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert old job: %v", err)
	}

	// Insert a recent job — should remain.
	_, err = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, completed_at)
		 VALUES ('recent-job-1', ?, 'export', 'manual', 'completed', 'low', now() - interval '1 day')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert recent job: %v", err)
	}

	scheduler.CleanupOldJobs(ctx, db)

	// Old job should be gone.
	var count int
	if err := db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE id = 'old-job-1'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check old job: %v", err)
	}
	if count != 0 {
		t.Errorf("expected old job deleted, got count=%d", count)
	}

	// Recent job should remain.
	if err := db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE id = 'recent-job-1'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check recent job: %v", err)
	}
	if count != 1 {
		t.Errorf("expected recent job to remain, got count=%d", count)
	}
}

func TestCleanupOldJobs_NoOldJobs_NoPanic(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	scheduler.CleanupOldJobs(ctx, db)
}

// TestCleanupOldJobs_DBError exercises the err != nil branch via a bad DB.
func TestCleanupOldJobs_DBError(t *testing.T) {
	db := badDB(t)
	// Should not panic; slog.Error is called internally.
	scheduler.CleanupOldJobs(context.Background(), db)
}

// ---------------------------------------------------------------------------
// CleanupExpiredSessions
// ---------------------------------------------------------------------------

func TestCleanupExpiredSessions_DeletesExpiredSessions(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Insert an expired session.
	_, err := db.NewRaw(
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at, created_at)
		 VALUES (?, ?, 'abc123hash', 'abc123refresh', now() - interval '1 hour', now() - interval '2 hours')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	// Insert a valid session.
	_, err = db.NewRaw(
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at, created_at)
		 VALUES (?, ?, 'def456hash', 'def456refresh', now() + interval '1 hour', now() - interval '10 minutes')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert valid session: %v", err)
	}

	scheduler.CleanupExpiredSessions(ctx, db)

	var count int
	if err := db.NewRaw(`SELECT COUNT(*) FROM user_sessions WHERE token_hash = 'abc123hash'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check expired session: %v", err)
	}
	if count != 0 {
		t.Errorf("expected expired session deleted, got count=%d", count)
	}

	if err := db.NewRaw(`SELECT COUNT(*) FROM user_sessions WHERE token_hash = 'def456hash'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check valid session: %v", err)
	}
	if count != 1 {
		t.Errorf("expected valid session to remain, got count=%d", count)
	}
}

func TestCleanupExpiredSessions_NoSessions_NoPanic(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	scheduler.CleanupExpiredSessions(ctx, db)
}

// TestCleanupExpiredSessions_DBError exercises the err != nil branch.
func TestCleanupExpiredSessions_DBError(t *testing.T) {
	scheduler.CleanupExpiredSessions(context.Background(), badDB(t))
}

// ---------------------------------------------------------------------------
// CleanupExports
// ---------------------------------------------------------------------------

func TestCleanupExports_RemovesExpiredFilesAndClearsPath(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Create a real temp file to verify it gets deleted.
	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()

	// Expired export job (>24h old).
	_, err = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, file_path, completed_at)
		 VALUES ('job-exp-export', ?, 'export', 'manual', 'completed', 'low', ?, now() - interval '25 hours')`,
		userID, tmpPath,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert expired export: %v", err)
	}

	// Recent export job (not yet expired).
	_, err = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, file_path, completed_at)
		 VALUES ('job-fresh-export', ?, 'export', 'manual', 'completed', 'low', '/tmp/fresh.json', now() - interval '1 hour')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert fresh export: %v", err)
	}

	scheduler.CleanupExports(ctx, db)

	// Temp file should have been removed.
	if _, statErr := os.Stat(tmpPath); !os.IsNotExist(statErr) {
		t.Errorf("expected temp file to be removed, but it still exists")
	}

	// file_path for expired job should be NULLed.
	var filePath *string
	if err := db.NewRaw(`SELECT file_path FROM jobs WHERE id = 'job-exp-export'`).Scan(ctx, &filePath); err != nil {
		t.Fatalf("read file_path: %v", err)
	}
	if filePath != nil {
		t.Errorf("expected file_path=NULL after cleanup, got %v", *filePath)
	}

	// Recent job file_path should be unchanged.
	if err := db.NewRaw(`SELECT file_path FROM jobs WHERE id = 'job-fresh-export'`).Scan(ctx, &filePath); err != nil {
		t.Fatalf("read fresh file_path: %v", err)
	}
	if filePath == nil {
		t.Error("expected fresh export file_path to remain intact")
	}
}

func TestCleanupExports_NoExpiredJobs_NothingHappens(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	// No rows at all — should not error/panic.
	scheduler.CleanupExports(ctx, db)
}

func TestCleanupExports_MissingFile_NoError(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Point to a file that does not exist.
	_, err := db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, file_path, completed_at)
		 VALUES ('job-missing-file', ?, 'export', 'manual', 'completed', 'low', '/tmp/does-not-exist-xyz.json', now() - interval '25 hours')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	// Should not panic; IsNotExist is handled gracefully.
	scheduler.CleanupExports(ctx, db)
}

// TestCleanupExports_DBError exercises the err != nil query failure branch.
func TestCleanupExports_DBError(t *testing.T) {
	scheduler.CleanupExports(context.Background(), badDB(t))
}

// ---------------------------------------------------------------------------
// CleanupUnreferencedGames
// ---------------------------------------------------------------------------

func TestCleanupUnreferencedGames_DeletesOrphans(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// games.id is IGDB ID (plain INTEGER, not auto-generated).
	const orphanID = 99901
	const referencedID = 99902

	// Insert two games with explicit IDs.
	if _, err := db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Orphan Game', now(), now())`,
		orphanID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert orphan game: %v", err)
	}
	if _, err := db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Referenced Game', now(), now())`,
		referencedID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert referenced game: %v", err)
	}

	// Link only the second game to a user.
	if _, err := db.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		uuid.NewString(), userID, referencedID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	scheduler.CleanupUnreferencedGames(ctx, db)

	// Orphan game should be gone.
	var exists bool
	if err := db.NewRaw(`SELECT EXISTS(SELECT 1 FROM games WHERE id = ?)`, orphanID).Scan(ctx, &exists); err != nil {
		t.Fatalf("check orphan game: %v", err)
	}
	if exists {
		t.Error("expected orphan game to be deleted")
	}

	// Referenced game should still exist.
	if err := db.NewRaw(`SELECT EXISTS(SELECT 1 FROM games WHERE id = ?)`, referencedID).Scan(ctx, &exists); err != nil {
		t.Fatalf("check referenced game: %v", err)
	}
	if !exists {
		t.Error("expected referenced game to remain")
	}
}

func TestCleanupUnreferencedGames_NoGames_NoPanic(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	scheduler.CleanupUnreferencedGames(ctx, db)
}

// TestCleanupUnreferencedGames_DBError exercises the err != nil branch.
func TestCleanupUnreferencedGames_DBError(t *testing.T) {
	scheduler.CleanupUnreferencedGames(context.Background(), badDB(t))
}

// ---------------------------------------------------------------------------
// CheckPendingSyncs
// ---------------------------------------------------------------------------


func TestCheckPendingSyncs_OverdueSyncDispatched(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Insert an overdue daily sync config.
	configID := uuid.NewString()
	lastSynced := time.Now().UTC().Add(-2 * 24 * time.Hour) // 2 days ago
	_, err := db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add, last_synced_at)
		 VALUES (?, ?, 'steam', 'daily', false, ?)`,
		configID, userID, lastSynced,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	pool := worker.NewPool(db)
	defer pool.Shutdown()

	scheduler.CheckPendingSyncs(ctx, db, pool)

	// A pending sync job should have been inserted.
	var count int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = 'steam' AND status = 'pending'`,
		userID,
	).Scan(ctx, &count); err != nil {
		t.Fatalf("count sync jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending sync job, got %d", count)
	}
}

func TestCheckPendingSyncs_NotOverdue_NotDispatched(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Synced 1 hour ago — not overdue for daily frequency.
	configID := uuid.NewString()
	lastSynced := time.Now().UTC().Add(-1 * time.Hour)
	_, err := db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add, last_synced_at)
		 VALUES (?, ?, 'steam', 'daily', false, ?)`,
		configID, userID, lastSynced,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	pool := worker.NewPool(db)
	defer pool.Shutdown()

	scheduler.CheckPendingSyncs(ctx, db, pool)

	var count int
	if err := db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync'`, userID).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 sync jobs for non-overdue sync, got %d", count)
	}
}

func TestCheckPendingSyncs_NeverSynced_Dispatched(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// last_synced_at is NULL (never synced) — should always dispatch.
	configID := uuid.NewString()
	_, err := db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add)
		 VALUES (?, ?, 'steam', 'weekly', false)`,
		configID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	pool := worker.NewPool(db)
	defer pool.Shutdown()

	scheduler.CheckPendingSyncs(ctx, db, pool)

	var count int
	if err := db.NewRaw(
		`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync' AND status = 'pending'`,
		userID,
	).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending sync job for never-synced config, got %d", count)
	}
}

func TestCheckPendingSyncs_EpicSkipped(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Epic storefront — should always be skipped.
	configID := uuid.NewString()
	_, err := db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add)
		 VALUES (?, ?, 'epic', 'daily', false)`,
		configID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	pool := worker.NewPool(db)
	defer pool.Shutdown()

	scheduler.CheckPendingSyncs(ctx, db, pool)

	var count int
	if err := db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync'`, userID).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 sync jobs for epic storefront, got %d", count)
	}
}

func TestCheckPendingSyncs_AlreadyRunning_NotDuplicated(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// Insert an overdue config.
	configID := uuid.NewString()
	lastSynced := time.Now().UTC().Add(-2 * 24 * time.Hour)
	_, err := db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add, last_synced_at)
		 VALUES (?, ?, 'steam', 'daily', false, ?)`,
		configID, userID, lastSynced,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	// Pre-insert a processing sync job — should prevent a second dispatch.
	_, err = db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert running job: %v", err)
	}

	pool := worker.NewPool(db)
	defer pool.Shutdown()

	scheduler.CheckPendingSyncs(ctx, db, pool)

	var count int
	if err := db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync'`, userID).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 sync job (no duplicate), got %d", count)
	}
}

func TestCheckPendingSyncs_ManualFrequency_Skipped(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, db)

	// manual frequency — should never be auto-dispatched.
	configID := uuid.NewString()
	_, err := db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, auto_add)
		 VALUES (?, ?, 'steam', 'manual', false)`,
		configID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	pool := worker.NewPool(db)
	defer pool.Shutdown()

	scheduler.CheckPendingSyncs(ctx, db, pool)

	var count int
	if err := db.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync'`, userID).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 sync jobs for manual frequency, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// NewScheduler — guard branches for invalid config durations
// ---------------------------------------------------------------------------

func TestNewScheduler_InvalidMetadataRefreshInterval_DefaultsTo24h(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "not-a-duration",
		StaleJobThreshold:       "4h",
	}

	// Should not panic; falls back to 24h.
	sched := scheduler.NewScheduler(db, pool, nil, cfg)
	if sched == nil {
		t.Fatal("expected non-nil scheduler")
	}
}

func TestNewScheduler_InvalidStaleJobThreshold_DefaultsTo4h(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "not-a-duration",
	}

	sched := scheduler.NewScheduler(db, pool, nil, cfg)
	if sched == nil {
		t.Fatal("expected non-nil scheduler")
	}
}

func TestNewScheduler_ValidConfig(t *testing.T) {
	db := setupTestDB(t)
	pool := worker.NewPool(db)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "12h",
		StaleJobThreshold:       "2h",
	}

	sched := scheduler.NewScheduler(db, pool, nil, cfg)
	if sched == nil {
		t.Fatal("expected non-nil scheduler")
	}
}


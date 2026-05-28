package scheduler_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/scheduler"
)

// ---------------------------------------------------------------------------
// CleanupOldJobs
// ---------------------------------------------------------------------------

func TestCleanupOldJobs_DeletesExpiredJobs(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Insert a job that is >30 days old and completed — should be deleted.
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, completed_at)
		 VALUES ('old-job-1', ?, 'export', 'manual', 'completed', 'low', now() - interval '31 days')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert old job: %v", err)
	}

	// Insert a recent job — should remain.
	_, err = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, completed_at)
		 VALUES ('recent-job-1', ?, 'export', 'manual', 'completed', 'low', now() - interval '1 day')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert recent job: %v", err)
	}

	scheduler.CleanupOldJobs(ctx, testDB)

	// Old job should be gone.
	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE id = 'old-job-1'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check old job: %v", err)
	}
	if count != 0 {
		t.Errorf("expected old job deleted, got count=%d", count)
	}

	// Recent job should remain.
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE id = 'recent-job-1'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check recent job: %v", err)
	}
	if count != 1 {
		t.Errorf("expected recent job to remain, got count=%d", count)
	}
}

// ---------------------------------------------------------------------------
// CleanupExpiredSessions
// ---------------------------------------------------------------------------

func TestCleanupExpiredSessions_DeletesExpiredSessions(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Insert an expired session.
	_, err := testDB.NewRaw(
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at, created_at)
		 VALUES (?, ?, 'abc123hash', 'abc123refresh', now() - interval '1 hour', now() - interval '2 hours')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	// Insert a valid session.
	_, err = testDB.NewRaw(
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, expires_at, created_at)
		 VALUES (?, ?, 'def456hash', 'def456refresh', now() + interval '1 hour', now() - interval '10 minutes')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert valid session: %v", err)
	}

	scheduler.CleanupExpiredSessions(ctx, testDB)

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_sessions WHERE token_hash = 'abc123hash'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check expired session: %v", err)
	}
	if count != 0 {
		t.Errorf("expected expired session deleted, got count=%d", count)
	}

	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_sessions WHERE token_hash = 'def456hash'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check valid session: %v", err)
	}
	if count != 1 {
		t.Errorf("expected valid session to remain, got count=%d", count)
	}
}

// ---------------------------------------------------------------------------
// CleanupExports
// ---------------------------------------------------------------------------

func TestCleanupExports_RemovesExpiredFilesAndClearsPath(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Create a real temp file to verify it gets deleted.
	tmpFile, err := os.CreateTemp(t.TempDir(), "export-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()

	// Expired export job (>24h old).
	_, err = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, file_path, completed_at)
		 VALUES ('job-exp-export', ?, 'export', 'manual', 'completed', 'low', ?, now() - interval '25 hours')`,
		userID, tmpPath,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert expired export: %v", err)
	}

	// Recent export job (not yet expired).
	_, err = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, file_path, completed_at)
		 VALUES ('job-fresh-export', ?, 'export', 'manual', 'completed', 'low', '/tmp/fresh.json', now() - interval '1 hour')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert fresh export: %v", err)
	}

	scheduler.CleanupExports(ctx, testDB)

	// Temp file should have been removed.
	if _, statErr := os.Stat(tmpPath); !os.IsNotExist(statErr) {
		t.Errorf("expected temp file to be removed, but it still exists")
	}

	// file_path for expired job should be NULLed.
	var filePath *string
	if err := testDB.NewRaw(`SELECT file_path FROM jobs WHERE id = 'job-exp-export'`).Scan(ctx, &filePath); err != nil {
		t.Fatalf("read file_path: %v", err)
	}
	if filePath != nil {
		t.Errorf("expected file_path=NULL after cleanup, got %v", *filePath)
	}

	// Recent job file_path should be unchanged.
	if err := testDB.NewRaw(`SELECT file_path FROM jobs WHERE id = 'job-fresh-export'`).Scan(ctx, &filePath); err != nil {
		t.Fatalf("read fresh file_path: %v", err)
	}
	if filePath == nil {
		t.Error("expected fresh export file_path to remain intact")
	}
}

// ---------------------------------------------------------------------------
// CleanupUnreferencedGames
// ---------------------------------------------------------------------------

func TestCleanupUnreferencedGames_DeletesOrphans(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// games.id is IGDB ID (plain INTEGER, not auto-generated).
	const orphanID = 99901
	const referencedID = 99902

	// Insert two games with explicit IDs.
	if _, err := testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Orphan Game', now(), now())`,
		orphanID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert orphan game: %v", err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Referenced Game', now(), now())`,
		referencedID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert referenced game: %v", err)
	}

	// Link only the second game to a user.
	if _, err := testDB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now())`,
		uuid.NewString(), userID, referencedID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	scheduler.CleanupUnreferencedGames(ctx, testDB)

	// Orphan game should be gone.
	var exists bool
	if err := testDB.NewRaw(`SELECT EXISTS(SELECT 1 FROM games WHERE id = ?)`, orphanID).Scan(ctx, &exists); err != nil {
		t.Fatalf("check orphan game: %v", err)
	}
	if exists {
		t.Error("expected orphan game to be deleted")
	}

	// Referenced game should still exist.
	if err := testDB.NewRaw(`SELECT EXISTS(SELECT 1 FROM games WHERE id = ?)`, referencedID).Scan(ctx, &exists); err != nil {
		t.Fatalf("check referenced game: %v", err)
	}
	if !exists {
		t.Error("expected referenced game to remain")
	}
}

// ---------------------------------------------------------------------------
// CheckPendingSyncs
// ---------------------------------------------------------------------------

func TestCheckPendingSyncs_OverdueSyncDispatched(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Insert an overdue daily sync config.
	configID := uuid.NewString()
	lastSynced := time.Now().UTC().Add(-2 * 24 * time.Hour) // 2 days ago
	_, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, last_synced_at)
		 VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, lastSynced,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	w := &scheduler.CheckPendingSyncsWorker{DB: testDB, RiverClient: newTestRiverClient(t)}
	_ = w.Work(ctx, &river.Job[scheduler.CheckPendingSyncsArgs]{})

	// A pending sync job should have been inserted.
	var count int
	if err := testDB.NewRaw(
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
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Synced 1 hour ago — not overdue for daily frequency.
	configID := uuid.NewString()
	lastSynced := time.Now().UTC().Add(-1 * time.Hour)
	_, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, last_synced_at)
		 VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, lastSynced,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	w := &scheduler.CheckPendingSyncsWorker{DB: testDB, RiverClient: newTestRiverClient(t)}
	_ = w.Work(ctx, &river.Job[scheduler.CheckPendingSyncsArgs]{})

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync'`, userID).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 sync jobs for non-overdue sync, got %d", count)
	}
}

func TestCheckPendingSyncs_NeverSynced_Dispatched(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// last_synced_at is NULL (never synced) — should always dispatch.
	configID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency)
		 VALUES (?, ?, 'steam', 'weekly')`,
		configID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	w := &scheduler.CheckPendingSyncsWorker{DB: testDB, RiverClient: newTestRiverClient(t)}
	_ = w.Work(ctx, &river.Job[scheduler.CheckPendingSyncsArgs]{})

	var count int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync' AND status = 'pending'`,
		userID,
	).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pending sync job for never-synced config, got %d", count)
	}
}

func TestCheckPendingSyncs_AlreadyRunning_NotDuplicated(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Insert an overdue config.
	configID := uuid.NewString()
	lastSynced := time.Now().UTC().Add(-2 * 24 * time.Hour)
	_, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, last_synced_at)
		 VALUES (?, ?, 'steam', 'daily', ?)`,
		configID, userID, lastSynced,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	// Pre-insert a processing sync job — should prevent a second dispatch.
	_, err = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert running job: %v", err)
	}

	w := &scheduler.CheckPendingSyncsWorker{DB: testDB, RiverClient: newTestRiverClient(t)}
	_ = w.Work(ctx, &river.Job[scheduler.CheckPendingSyncsArgs]{})

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync'`, userID).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 sync job (no duplicate), got %d", count)
	}
}

func TestCheckPendingSyncs_ManualFrequency_Skipped(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// manual frequency — should never be auto-dispatched.
	configID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency)
		 VALUES (?, ?, 'steam', 'manual')`,
		configID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert sync config: %v", err)
	}

	w := &scheduler.CheckPendingSyncsWorker{DB: testDB, RiverClient: newTestRiverClient(t)}
	_ = w.Work(ctx, &river.Job[scheduler.CheckPendingSyncsArgs]{})

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync'`, userID).Scan(ctx, &count); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 sync jobs for manual frequency, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// BuildPeriodicJobs — guard branches for invalid config durations
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// CleanupSyncChanges
// ---------------------------------------------------------------------------

func TestCleanupSyncChanges_DeletesOldRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)

	// Insert a job so sync_changes can reference it.
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at)
         VALUES (?, ?, 'sync', 'steam', 'completed', 'low', now())`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	// 100-day-old row — should be deleted when retention=90.
	_, err = testDB.NewRaw(
		`INSERT INTO sync_changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'Old Game', now() - interval '100 days')`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert old sync_change: %v", err)
	}
	// 50-day-old row — should remain.
	_, err = testDB.NewRaw(
		`INSERT INTO sync_changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'Mid Game', now() - interval '50 days')`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert mid sync_change: %v", err)
	}
	// 1-day-old row — should remain.
	_, err = testDB.NewRaw(
		`INSERT INTO sync_changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'New Game', now() - interval '1 day')`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert new sync_change: %v", err)
	}

	scheduler.CleanupSyncChanges(ctx, testDB, 90)

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM sync_changes`).Scan(ctx, &count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows remaining, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// BuildPeriodicJobs — guard branches for invalid config durations
// ---------------------------------------------------------------------------

func TestBuildPeriodicJobs_InvalidMetadataRefreshInterval_DefaultsTo24h(t *testing.T) {
	cfg := &config.Config{
		MetadataRefreshInterval: "not-a-duration",
		StaleJobThreshold:       "4h",
	}
	// Should not panic; falls back to 24h.
	jobs := scheduler.BuildPeriodicJobs(cfg, 4*time.Hour)
	if len(jobs) == 0 {
		t.Fatal("expected non-empty periodic jobs slice")
	}
}

func TestBuildPeriodicJobs_InvalidStaleJobThreshold_DefaultsTo4h(t *testing.T) {
	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "not-a-duration",
	}
	jobs := scheduler.BuildPeriodicJobs(cfg, 4*time.Hour)
	if len(jobs) == 0 {
		t.Fatal("expected non-empty periodic jobs slice")
	}
}

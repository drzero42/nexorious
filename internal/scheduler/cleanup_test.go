package scheduler_test

import (
	"context"
	"os"
	"path/filepath"
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

	// Insert an active job with no completed_at — should never be deleted.
	_, err = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority)
		 VALUES ('active-job-1', ?, 'import', 'manual', 'processing', 'low')`,
		userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert active job: %v", err)
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

	// Active job (no completed_at) should survive cleanup.
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM jobs WHERE id = 'active-job-1'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check active job: %v", err)
	}
	if count != 1 {
		t.Errorf("expected active job to remain, got count=%d", count)
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
		`INSERT INTO user_sessions (id, user_id, session_id_hash, expires_at, created_at)
		 VALUES (?, ?, 'abc123hash', now() - interval '1 hour', now() - interval '2 hours')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	// Insert a valid session.
	_, err = testDB.NewRaw(
		`INSERT INTO user_sessions (id, user_id, session_id_hash, expires_at, created_at)
		 VALUES (?, ?, 'def456hash', now() + interval '1 hour', now() - interval '10 minutes')`,
		uuid.NewString(), userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert valid session: %v", err)
	}

	scheduler.CleanupExpiredSessions(ctx, testDB)

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_sessions WHERE session_id_hash = 'abc123hash'`).Scan(ctx, &count); err != nil {
		t.Fatalf("check expired session: %v", err)
	}
	if count != 0 {
		t.Errorf("expected expired session deleted, got count=%d", count)
	}

	if err := testDB.NewRaw(`SELECT COUNT(*) FROM user_sessions WHERE session_id_hash = 'def456hash'`).Scan(ctx, &count); err != nil {
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

	scheduler.CleanupExports(ctx, testDB, "")

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

// Files in the exports directory that no Job refers to and that are older than
// a day are removed (a partial file left by a kill, or a run whose transition
// was refused); a referenced file and a recent one are kept.
func TestCleanupExports_SweepsUnreferencedFiles(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := insertUser(t, ctx, nil)
	storage := t.TempDir()
	dir := filepath.Join(storage, "exports")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	mk := func(name string, mtime time.Time) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		return p
	}
	orphan := mk("orphan_1.json", old)
	recent := mk("recent_1.json", time.Now())
	referenced := mk("kept_1.json", old)
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, file_path, completed_at)
		 VALUES ('job-kept', ?, 'export', 'nexorious', 'completed', 'normal', ?, now())`,
		userID, referenced,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	scheduler.CleanupExports(ctx, testDB, storage)

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("unreferenced old file kept")
	}
	for _, p := range []string{recent, referenced} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed: %v", filepath.Base(p), err)
		}
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

// TestCheckPendingSyncs_BacksOffAfterFailures checks the retry delay after
// failed syncs (#1344): 15 minutes after the first failure, doubling per
// failure, capped at the sync interval. Failures from before the last
// successful sync do not count.
func TestCheckPendingSyncs_BacksOffAfterFailures(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		frequency  string
		lastSynced time.Duration   // ago
		failedAgo  []time.Duration // completed_at of each failed sync, ago
		wantSync   bool
	}{
		{"first failure, inside 15 min", "daily", 48 * time.Hour, []time.Duration{10 * time.Minute}, false},
		{"first failure, past 15 min", "daily", 48 * time.Hour, []time.Duration{20 * time.Minute}, true},
		{"third failure, inside 1 h", "daily", 48 * time.Hour, []time.Duration{3 * time.Hour, 2 * time.Hour, 50 * time.Minute}, false},
		{"third failure, past 1 h", "daily", 48 * time.Hour, []time.Duration{3 * time.Hour, 2 * time.Hour, 70 * time.Minute}, true},
		{"many failures, daily cap not reached", "daily", 72 * time.Hour, repeatAgo(10, 23*time.Hour), false},
		{"many failures, hourly cap reached", "hourly", 72 * time.Hour, repeatAgo(10, 61*time.Minute), true},
		{"failures before the last success", "daily", 48 * time.Hour, []time.Duration{50 * time.Hour, 49 * time.Hour}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncateAllTables(t)
			userID := insertUser(t, ctx, nil)
			now := time.Now().UTC()
			if _, err := testDB.NewRaw(
				`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, last_synced_at)
				 VALUES (?, ?, 'steam', ?, ?)`,
				uuid.NewString(), userID, tc.frequency, now.Add(-tc.lastSynced),
			).Exec(ctx); err != nil {
				t.Fatalf("insert sync config: %v", err)
			}
			for _, ago := range tc.failedAgo {
				if _, err := testDB.NewRaw(
					`INSERT INTO jobs (id, user_id, job_type, source, status, priority, error_message, created_at, completed_at)
					 VALUES (?, ?, 'sync', 'steam', 'failed', 'low', 'storefront unavailable', ?, ?)`,
					uuid.NewString(), userID, now.Add(-ago-time.Minute), now.Add(-ago),
				).Exec(ctx); err != nil {
					t.Fatalf("insert failed job: %v", err)
				}
			}

			w := &scheduler.CheckPendingSyncsWorker{DB: testDB, RiverClient: newTestRiverClient(t)}
			_ = w.Work(ctx, &river.Job[scheduler.CheckPendingSyncsArgs]{})

			var pending int
			if err := testDB.NewRaw(
				`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync' AND status = 'pending'`, userID,
			).Scan(ctx, &pending); err != nil {
				t.Fatalf("count pending jobs: %v", err)
			}
			if got := pending == 1; got != tc.wantSync {
				t.Errorf("sync started = %v, want %v", got, tc.wantSync)
			}
		})
	}
}

// repeatAgo returns n completion offsets, the newest newest ago and each
// earlier one an hour before the next.
func repeatAgo(n int, newest time.Duration) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = newest + time.Duration(n-1-i)*time.Hour
	}
	return out
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

	// Insert a job so changes can reference it.
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
		`INSERT INTO changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'Old Game', now() - interval '100 days')`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert old change: %v", err)
	}
	// 50-day-old row — should remain.
	_, err = testDB.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'Mid Game', now() - interval '50 days')`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert mid change: %v", err)
	}
	// 1-day-old row — should remain.
	_, err = testDB.NewRaw(
		`INSERT INTO changes (id, job_id, user_id, change_type, title, created_at)
         VALUES (?, ?, ?, 'added', 'New Game', now() - interval '1 day')`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert new change: %v", err)
	}

	scheduler.CleanupSyncChanges(ctx, testDB, 90)

	var count int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM changes`).Scan(ctx, &count); err != nil {
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

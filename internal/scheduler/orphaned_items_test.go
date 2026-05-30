package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/scheduler"
)

func insertSyncJob(t *testing.T, ctx context.Context, userID string) string {
	t.Helper()
	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'gog', 'processing', 'high')`,
		jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insertSyncJob: %v", err)
	}
	return jobID
}

func insertPendingItem(t *testing.T, ctx context.Context, jobID, userID string, age time.Duration) string {
	t.Helper()
	itemID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, 'Test Game', '{}', 'pending', '{}', '[]', now() - (?::text || ' seconds')::interval)`,
		itemID, jobID, userID, uuid.NewString(), int64(age.Seconds()),
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insertPendingItem: %v", err)
	}
	return itemID
}

func countRiverJobsForItem(t *testing.T, ctx context.Context, itemID string) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE args->>'job_item_id' = ?`, itemID,
	).Scan(ctx, &n); err != nil {
		t.Fatalf("countRiverJobsForItem: %v", err)
	}
	return n
}

func TestRescueOrphanedPendingItems_ReenqueuesOrphanedItem(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)

	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJob(t, ctx, userID)
	itemID := insertPendingItem(t, ctx, jobID, userID, 2*time.Hour)

	scheduler.RescueOrphanedPendingItems(ctx, testDB, rc, time.Hour)

	if n := countRiverJobsForItem(t, ctx, itemID); n != 1 {
		t.Fatalf("expected 1 river_job for orphaned item, got %d", n)
	}
}

func TestRescueOrphanedPendingItems_FreshItemLeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)

	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJob(t, ctx, userID)
	itemID := insertPendingItem(t, ctx, jobID, userID, 5*time.Minute)

	scheduler.RescueOrphanedPendingItems(ctx, testDB, rc, time.Hour)

	if n := countRiverJobsForItem(t, ctx, itemID); n != 0 {
		t.Fatalf("expected no river_job for fresh item, got %d", n)
	}
}

func TestRescueOrphanedPendingItems_ItemWithActiveRiverJobLeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)

	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJob(t, ctx, userID)
	itemID := insertPendingItem(t, ctx, jobID, userID, 2*time.Hour)

	// Insert an active River job for the item.
	_, err := testDB.NewRaw(
		`INSERT INTO river_job (kind, args, max_attempts, queue, state, scheduled_at, priority)
		 VALUES ('process_sync_item', ('{"job_item_id": "' || ? || '"}')::jsonb, 5, 'default', 'available', now(), 1)`,
		itemID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert river_job: %v", err)
	}

	scheduler.RescueOrphanedPendingItems(ctx, testDB, rc, time.Hour)

	if n := countRiverJobsForItem(t, ctx, itemID); n != 1 {
		t.Fatalf("expected exactly 1 river_job (the existing one), got %d", n)
	}
}

func TestRescueOrphanedPendingItems_ItemWithCompletedRiverJobReenqueued(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)

	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJob(t, ctx, userID)
	itemID := insertPendingItem(t, ctx, jobID, userID, 2*time.Hour)

	// Insert a completed River job for the item (exactly the bug we hit).
	_, err := testDB.NewRaw(
		`INSERT INTO river_job (kind, args, max_attempts, queue, state, scheduled_at, priority, finalized_at)
		 VALUES ('process_sync_item', ('{"job_item_id": "' || ? || '"}')::jsonb, 5, 'default', 'completed', now(), 1, now())`,
		itemID,
	).Exec(ctx)
	if err != nil {
		t.Fatalf("insert completed river_job: %v", err)
	}

	scheduler.RescueOrphanedPendingItems(ctx, testDB, rc, time.Hour)

	// Should now have 2: the old completed one + the new available one.
	if n := countRiverJobsForItem(t, ctx, itemID); n != 2 {
		t.Fatalf("expected 2 river_jobs (old completed + new available), got %d", n)
	}
}

func TestRescueOrphanedPendingItems_TerminalJobItemLeftAlone(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)

	userID := insertUser(t, ctx, nil)
	// A completed parent job.
	jobID := uuid.NewString()
	_, _ = testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority) VALUES (?, ?, 'sync', 'gog', 'completed', 'high')`,
		jobID, userID,
	).Exec(ctx)
	itemID := insertPendingItem(t, ctx, jobID, userID, 2*time.Hour)

	scheduler.RescueOrphanedPendingItems(ctx, testDB, rc, time.Hour)

	if n := countRiverJobsForItem(t, ctx, itemID); n != 0 {
		t.Fatalf("expected no river_job for item under completed job, got %d", n)
	}
}

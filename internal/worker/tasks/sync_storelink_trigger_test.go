package tasks_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// TestSyncCompletion_EnqueuesStoreLinkRefresh verifies that finalizing a sync
// job enqueues a scoped, incremental store-link enrichment dispatch for that
// job's storefront.
func TestSyncCompletion_EnqueuesStoreLinkRefresh(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)

	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, dispatch_complete, created_at)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'low', 1, true, now())`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'x', 'x', '{}', 'completed', '{}', '[]', now())`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	rc := newTestMetadataRiverClient(t)
	tasks.SyncCheckJobCompletion(ctx, testDB, rc, jobID)

	var n int
	if err := testDB.NewRaw(
		`SELECT count(*) FROM river_job WHERE kind = 'store_link_refresh_dispatch'`,
	).Scan(ctx, &n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 store_link_refresh_dispatch river job, got %d", n)
	}
}

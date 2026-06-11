package tasks_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/worker/tasks"
)

func TestSyncJobItemStatusCounts(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	const jobID = "job-metrics-1"
	const userID = "user-metrics-1"
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority)
		 VALUES (?, ?, 'sync', 'steam', 'processing', 'high')`,
		jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	seed := func(key, status string) {
		if _, err := testDB.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
			 VALUES (?, ?, ?, ?, ?, '{}', ?, '{}', '[]')`,
			uuid.NewString(), jobID, userID, key, key, status,
		).Exec(ctx); err != nil {
			t.Fatalf("seed item %s: %v", key, err)
		}
	}
	seed("a", "completed")
	seed("b", "completed")
	seed("c", "failed")
	seed("d", "skipped")
	seed("e", "skipped")
	seed("f", "skipped")

	completed, failed, skipped, ok := tasks.SyncJobItemStatusCountsForTest(ctx, testDB, jobID)
	if !ok {
		t.Fatal("syncJobItemStatusCounts ok = false; want true")
	}
	if completed != 2 || failed != 1 || skipped != 3 {
		t.Errorf("counts = (completed=%d failed=%d skipped=%d); want (2 1 3)", completed, failed, skipped)
	}
}

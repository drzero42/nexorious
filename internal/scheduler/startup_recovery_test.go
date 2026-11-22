package scheduler_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/scheduler"
)

// insertSyncJobState inserts a gog sync Job with the given status and
// dispatch_complete value.
func insertSyncJobState(t *testing.T, ctx context.Context, userID, status string, dispatchComplete bool) string {
	t.Helper()
	jobID := uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, dispatch_complete)
		 VALUES (?, ?, 'sync', 'gog', ?, 'high', ?)`,
		jobID, userID, status, dispatchComplete,
	).Exec(ctx); err != nil {
		t.Fatalf("insertSyncJobState: %v", err)
	}
	return jobID
}

// insertRiverJob inserts a river_job row of the given kind and state whose args
// carry {argKey: argValue}, and returns its id.
func insertRiverJob(t *testing.T, ctx context.Context, kind, state, argKey, argValue string, attempt, maxAttempts int) int64 {
	t.Helper()
	var id int64
	if err := testDB.NewRaw(
		`INSERT INTO river_job (kind, args, attempt, max_attempts, queue, state, scheduled_at, priority, finalized_at, attempted_at)
		 VALUES (?, jsonb_build_object(?::text, ?::text), ?, ?, 'default', ?::river_job_state, now(), 1,
		         CASE WHEN ? IN ('completed', 'discarded', 'cancelled') THEN now() END,
		         CASE WHEN ? = 'running' THEN now() END)
		 RETURNING id`,
		kind, argKey, argValue, attempt, maxAttempts, state, state, state,
	).Scan(ctx, &id); err != nil {
		t.Fatalf("insertRiverJob: %v", err)
	}
	return id
}

func countDispatches(t *testing.T, ctx context.Context, jobID, state string) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE kind = 'dispatch_sync' AND args->>'job_id' = ? AND state = ?::river_job_state`,
		jobID, state,
	).Scan(ctx, &n); err != nil {
		t.Fatalf("countDispatches: %v", err)
	}
	return n
}

// A clean shutdown cancels a running dispatch, which River records as
// completed while the Job stays 'processing' with dispatch_complete=false.
// Startup recovery re-enqueues the dispatch for exactly those Jobs.
func TestRecoverInterruptedSyncs_ReenqueuesUnfinishedDispatch(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)
	userID := insertUser(t, ctx, nil)

	interrupted := insertSyncJobState(t, ctx, userID, "processing", false)
	insertRiverJob(t, ctx, "dispatch_sync", "completed", "job_id", interrupted, 1, 3)

	stillQueued := insertSyncJobState(t, ctx, userID, "processing", false)
	insertRiverJob(t, ctx, "dispatch_sync", "available", "job_id", stillQueued, 0, 3)

	dispatched := insertSyncJobState(t, ctx, userID, "processing", true)
	// A pending_review item keeps this Job open through the completion check.
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'k', 'Game', '{}', 'pending_review', '{}', '[]')`,
		uuid.NewString(), dispatched, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert pending_review item: %v", err)
	}
	failed := insertSyncJobState(t, ctx, userID, "failed", false)
	cancelled := insertSyncJobState(t, ctx, userID, "cancelled", false)

	scheduler.RecoverInterruptedSyncs(ctx, testDB, rc)

	if n := countDispatches(t, ctx, interrupted, "available"); n != 1 {
		t.Errorf("interrupted Job: want 1 new dispatch, got %d", n)
	}
	var args struct {
		UserID     string `bun:"user_id"`
		Storefront string `bun:"storefront"`
	}
	if err := testDB.NewRaw(
		`SELECT args->>'user_id' AS user_id, args->>'storefront' AS storefront
		 FROM river_job WHERE kind = 'dispatch_sync' AND args->>'job_id' = ? AND state = 'available'`, interrupted,
	).Scan(ctx, &args); err != nil {
		t.Fatalf("load new dispatch: %v", err)
	}
	if args.UserID != userID || args.Storefront != "gog" {
		t.Errorf("new dispatch args: want user %q / gog, got %q / %q", userID, args.UserID, args.Storefront)
	}
	if n := countDispatches(t, ctx, stillQueued, "available"); n != 1 {
		t.Errorf("Job with a queued dispatch: want its 1 dispatch untouched, got %d", n)
	}
	for name, id := range map[string]string{"dispatch_complete=true": dispatched, "failed": failed, "cancelled": cancelled} {
		if n := countDispatches(t, ctx, id, "available"); n != 0 {
			t.Errorf("%s Job: want no dispatch, got %d", name, n)
		}
	}

	// A second pass (e.g. after a restore) must not stack another dispatch.
	scheduler.RecoverInterruptedSyncs(ctx, testDB, rc)
	if n := countDispatches(t, ctx, interrupted, "available"); n != 1 {
		t.Errorf("second pass: want still 1 dispatch, got %d", n)
	}
}

// A hard kill leaves River jobs 'running'. With a single instance they belong
// to a dead process, so startup releases the sync kinds, and only those.
func TestRecoverInterruptedSyncs_ReleasesRunningSyncRiverJobs(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)
	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJobState(t, ctx, userID, "processing", false)
	itemA := insertPendingItem(t, ctx, jobID, userID, 0)
	itemB := insertPendingItem(t, ctx, jobID, userID, 0)

	released := map[string]int64{
		"dispatch_sync":   insertRiverJob(t, ctx, "dispatch_sync", "running", "job_id", jobID, 1, 3),
		"igdb_match":      insertRiverJob(t, ctx, "igdb_match", "running", "job_item_id", itemA, 1, 5),
		"user_game_write": insertRiverJob(t, ctx, "user_game_write", "running", "job_item_id", itemB, 2, 5),
	}
	otherKind := insertRiverJob(t, ctx, "export_json", "running", "job_id", uuid.NewString(), 1, 3)

	scheduler.RecoverInterruptedSyncs(ctx, testDB, rc)

	state := func(id int64) string {
		var s string
		if err := testDB.NewRaw(`SELECT state::text FROM river_job WHERE id = ?`, id).Scan(ctx, &s); err != nil {
			t.Fatalf("load river_job state: %v", err)
		}
		return s
	}
	for kind, id := range released {
		if s := state(id); s != "retryable" {
			t.Errorf("%s: want retryable, got %q", kind, s)
		}
	}
	if s := state(otherKind); s != "running" {
		t.Errorf("non-sync kind: want running (untouched), got %q", s)
	}
	// The released dispatch counts as live: no second dispatch is stacked on it.
	var n int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE kind = 'dispatch_sync' AND args->>'job_id' = ?`, jobID,
	).Scan(ctx, &n); err != nil {
		t.Fatalf("count dispatches: %v", err)
	}
	if n != 1 {
		t.Errorf("want 1 dispatch river_job for the released Job, got %d", n)
	}
}

// An item the previous process left 'pending' with no live River job is
// re-queued at once, whatever its age, and without a maintenance event.
func TestRecoverInterruptedSyncs_RequeuesFreshStrandedItemWithoutEvent(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)
	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJobState(t, ctx, userID, "processing", true)
	itemID := insertPendingItem(t, ctx, jobID, userID, 0) // created just now
	// The cancelled worker returned nil, so River recorded its job completed.
	insertRiverJob(t, ctx, "igdb_match", "completed", "job_item_id", itemID, 1, 5)

	scheduler.RecoverInterruptedSyncs(ctx, testDB, rc)

	var live int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE args->>'job_item_id' = ? AND state = 'available'`, itemID,
	).Scan(ctx, &live); err != nil {
		t.Fatalf("count river jobs: %v", err)
	}
	if live != 1 {
		t.Errorf("want 1 new river_job for the stranded item, got %d", live)
	}
	var events int
	if err := testDB.NewRaw(`SELECT COUNT(*) FROM events`).Scan(ctx, &events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 0 {
		t.Errorf("a restart must not emit events, got %d", events)
	}
}

// A Job whose last item settled just before the shutdown never got its
// completion check. Startup finalizes it and emits sync.completed once.
func TestRecoverInterruptedSyncs_FinalizesSettledJob(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)
	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJobState(t, ctx, userID, "processing", true)
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates)
		 VALUES (?, ?, ?, 'k', 'Game', '{}', 'completed', '{}', '[]')`,
		uuid.NewString(), jobID, userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert completed item: %v", err)
	}

	scheduler.RecoverInterruptedSyncs(ctx, testDB, rc)
	scheduler.RecoverInterruptedSyncs(ctx, testDB, rc)

	var status string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if status != "completed" {
		t.Errorf("job status: want completed, got %q", status)
	}
	var events int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, jobID+":sync.completed",
	).Scan(ctx, &events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 1 {
		t.Errorf("sync.completed events: want 1, got %d", events)
	}
}

// A dispatch hard-killed on its last attempt is dead (#1304): its River job is
// discarded and the Job failed, so it no longer blocks a new sync and is not
// re-dispatched.
func TestRecoverInterruptedSyncs_FailsJobWhoseDispatchHasNoAttemptsLeft(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)
	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJobState(t, ctx, userID, "processing", false)
	itemID := insertPendingItem(t, ctx, jobID, userID, 0)
	insertRiverJob(t, ctx, "dispatch_sync", "running", "job_id", jobID, 3, 3)

	scheduler.RecoverInterruptedSyncs(ctx, testDB, rc)

	if n := countDispatches(t, ctx, jobID, "discarded"); n != 1 {
		t.Errorf("want the exhausted dispatch discarded, got %d discarded", n)
	}
	var total int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM river_job WHERE kind = 'dispatch_sync' AND args->>'job_id' = ?`, jobID,
	).Scan(ctx, &total); err != nil {
		t.Fatalf("count dispatches: %v", err)
	}
	if total != 1 {
		t.Errorf("want no new dispatch for the failed Job, got %d dispatch river_jobs", total)
	}

	var status, itemStatus string
	var errMsg *string
	if err := testDB.NewRaw(`SELECT status, error_message FROM jobs WHERE id = ?`, jobID).Scan(ctx, &status, &errMsg); err != nil {
		t.Fatalf("load job: %v", err)
	}
	if status != "failed" || errMsg == nil || *errMsg == "" {
		t.Errorf("want job failed with a message, got status=%q error_message=%v", status, errMsg)
	}
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(ctx, &itemStatus); err != nil {
		t.Fatalf("load item: %v", err)
	}
	if itemStatus != "cancelled" {
		t.Errorf("want pending item cancelled, got %q", itemStatus)
	}
	var events int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM events WHERE dedup_key = ?`, jobID+":sync.failed",
	).Scan(ctx, &events); err != nil {
		t.Fatalf("count sync.failed events: %v", err)
	}
	if events != 1 {
		t.Errorf("want 1 sync.failed event, got %d", events)
	}
}

// An item whose worker was hard-killed on its last attempt is failed rather
// than re-queued (#1304): re-queueing would hand an item that crashes the
// process fresh attempts on every restart.
func TestRecoverInterruptedSyncs_FailsItemWhoseWorkerHasNoAttemptsLeft(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	rc := newTestRiverClient(t)
	userID := insertUser(t, ctx, nil)
	jobID := insertSyncJobState(t, ctx, userID, "processing", false)
	items := map[string]string{
		"igdb_match":      insertPendingItem(t, ctx, jobID, userID, 0),
		"user_game_write": insertPendingItem(t, ctx, jobID, userID, 0),
	}
	riverIDs := map[string]int64{}
	for kind, itemID := range items {
		riverIDs[kind] = insertRiverJob(t, ctx, kind, "running", "job_item_id", itemID, 5, 5)
	}

	scheduler.RecoverInterruptedSyncs(ctx, testDB, rc)

	for kind, itemID := range items {
		var state, itemStatus string
		var errMsg *string
		if err := testDB.NewRaw(`SELECT state::text FROM river_job WHERE id = ?`, riverIDs[kind]).Scan(ctx, &state); err != nil {
			t.Fatalf("load river_job: %v", err)
		}
		if state != "discarded" {
			t.Errorf("%s: want river job discarded, got %q", kind, state)
		}
		if err := testDB.NewRaw(`SELECT status, error_message FROM job_items WHERE id = ?`, itemID).Scan(ctx, &itemStatus, &errMsg); err != nil {
			t.Fatalf("load item: %v", err)
		}
		if itemStatus != "failed" || errMsg == nil || *errMsg == "" {
			t.Errorf("%s: want item failed with a message, got status=%q error_message=%v", kind, itemStatus, errMsg)
		}
		var requeued int
		if err := testDB.NewRaw(
			`SELECT COUNT(*) FROM river_job WHERE args->>'job_item_id' = ? AND id <> ?`, itemID, riverIDs[kind],
		).Scan(ctx, &requeued); err != nil {
			t.Fatalf("count requeued: %v", err)
		}
		if requeued != 0 {
			t.Errorf("%s: want item not re-queued, got %d new river jobs", kind, requeued)
		}
	}
}

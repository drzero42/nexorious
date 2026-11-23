package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// hookAdapter yields its batches, then runs afterFetch (still inside
// GetLibrary) and returns err. Tests use afterFetch to interrupt or cancel the
// sync at the moment the library fetch ends.
type hookAdapter struct {
	batches    [][]tasks.ExternalGameEntry
	afterFetch func()
	err        error
	calls      int
}

func (a *hookAdapter) GetLibrary(_ context.Context, _ int, onBatch func([]tasks.ExternalGameEntry) error) error {
	a.calls++
	for _, batch := range a.batches {
		if err := onBatch(batch); err != nil {
			return err
		}
	}
	if a.afterFetch != nil {
		a.afterFetch()
	}
	return a.err
}

// seedGOGSyncJob inserts a user, a gog sync Job in the given status and a gog
// sync config, and returns the user and Job ids.
func seedGOGSyncJob(t *testing.T, status string) (userID, jobID string) {
	t.Helper()
	ctx := context.Background()
	userID, jobID = uuid.NewString(), uuid.NewString()
	insertTestUser(t, testDB, userID)
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items) VALUES (?, ?, 'sync', 'gog', ?, 'low', 0)`,
		jobID, userID, status,
	).Exec(ctx); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := testDB.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency) VALUES (?, ?, 'gog', 'daily')`,
		uuid.NewString(), userID,
	).Exec(ctx); err != nil {
		t.Fatalf("insert sync config: %v", err)
	}
	return userID, jobID
}

func runDispatch(t *testing.T, ctx context.Context, w *tasks.DispatchSyncWorker, jobID, userID string) {
	t.Helper()
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

// runInterrupted runs a dispatch whose ctx is cancelled on the way: the
// worker records nothing and returns the cancellation, for River to run the
// same River job again.
func runInterrupted(t *testing.T, ctx context.Context, w *tasks.DispatchSyncWorker, jobID, userID string) {
	t.Helper()
	if err := w.Work(ctx, syncDispatchJob(t, tasks.DispatchSyncArgs{JobID: jobID, UserID: userID, Storefront: "gog"})); !errors.Is(err, context.Canceled) {
		t.Fatalf("Work: want context.Canceled, got %v", err)
	}
}

func loadJobState(t *testing.T, jobID string) (status string, dispatchComplete bool) {
	t.Helper()
	if err := testDB.NewRaw(
		`SELECT status, dispatch_complete FROM jobs WHERE id = ?`, jobID,
	).Scan(context.Background(), &status, &dispatchComplete); err != nil {
		t.Fatalf("load job: %v", err)
	}
	return status, dispatchComplete
}

func countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(query, args...).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

// A shutdown that cancels the dispatch's ctx as the fetch ends must leave the
// Job resumable: not marked dispatched, not failed, last_synced_at untouched,
// and no game swept as removed (#1302).
func TestDispatchSync_InterruptedAfterFetch_NotMarkedComplete(t *testing.T) {
	truncateAllTables(t)
	userID, jobID := seedGOGSyncJob(t, "pending")
	// Known game the fetch does not return: an unguarded sweep would remove it.
	insertTestExternalGame(t, userID, "gog", "gone", "Gone Game", "pc-windows")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := &hookAdapter{
		batches:    [][]tasks.ExternalGameEntry{{{ExternalID: "1", Title: "Game One", Platforms: []string{"pc-windows"}}}},
		afterFetch: cancel,
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), LC: newTestLC(t)}
	runInterrupted(t, ctx, w, jobID, userID)

	status, dispatchComplete := loadJobState(t, jobID)
	if status != "processing" {
		t.Errorf("job status: want processing, got %q", status)
	}
	if dispatchComplete {
		t.Error("dispatch_complete must stay false for an interrupted dispatch")
	}
	var lastSynced *time.Time
	if err := testDB.NewRaw(
		`SELECT last_synced_at FROM user_sync_configs WHERE user_id = ? AND storefront = 'gog'`, userID,
	).Scan(context.Background(), &lastSynced); err != nil {
		t.Fatalf("load sync config: %v", err)
	}
	if lastSynced != nil {
		t.Errorf("last_synced_at must be unchanged, got %v", lastSynced)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM external_games WHERE user_id = ? AND external_id = 'gone' AND is_available = true`, userID); n != 1 {
		t.Error("an interrupted dispatch must not sweep games as removed")
	}
	if n := countRows(t, `SELECT COUNT(*) FROM events WHERE dedup_key = ?`, jobID+":sync.failed"); n != 0 {
		t.Error("an interrupt must not emit sync.failed")
	}
}

// The same holds when the interrupted fetch returns an error: it is an
// interrupt, not a failed sync.
func TestDispatchSync_InterruptedFetchError_NotFailed(t *testing.T) {
	truncateAllTables(t)
	userID, jobID := seedGOGSyncJob(t, "pending")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := &hookAdapter{afterFetch: cancel, err: context.Canceled}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), LC: newTestLC(t)}
	runInterrupted(t, ctx, w, jobID, userID)

	if status, dispatchComplete := loadJobState(t, jobID); status != "processing" || dispatchComplete {
		t.Errorf("want processing with dispatch_complete=false, got %q / %v", status, dispatchComplete)
	}
}

// The startup recovery re-runs an interrupted dispatch for the same Job. The
// second run must not duplicate job_items or 'skipped' change rows, and the Job
// must complete.
func TestDispatchSync_RerunAfterInterrupt_NoDuplicates(t *testing.T) {
	truncateAllTables(t)
	userID, jobID := seedGOGSyncJob(t, "pending")
	bg := context.Background()
	if _, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription)
		 VALUES (?, ?, 'gog', '2', 'Skipped Game', true, true, false)`,
		uuid.NewString(), userID,
	).Exec(bg); err != nil {
		t.Fatalf("insert skipped game: %v", err)
	}
	batches := [][]tasks.ExternalGameEntry{{
		{ExternalID: "1", Title: "Game One", Platforms: []string{"pc-windows"}},
		{ExternalID: "2", Title: "Skipped Game", Platforms: []string{"pc-windows"}},
	}}
	lc := newTestLC(t)

	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	first := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&hookAdapter{batches: batches, afterFetch: cancel}), LC: lc}
	runInterrupted(t, ctx, first, jobID, userID)

	second := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(&hookAdapter{batches: batches}), LC: newTestLC(t)}
	runDispatch(t, bg, second, jobID, userID)

	if n := countRows(t, `SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID); n != 1 {
		t.Errorf("job_items: want 1, got %d", n)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'skipped'`, jobID); n != 1 {
		t.Errorf("'skipped' changes: want 1, got %d", n)
	}
	if status, dispatchComplete := loadJobState(t, jobID); status != "processing" || !dispatchComplete {
		t.Fatalf("after re-run: want processing with dispatch_complete=true, got %q / %v", status, dispatchComplete)
	}

	if n := countRows(t, `SELECT COUNT(*) FROM river_job WHERE kind = 'igdb_match'`); n != 1 {
		t.Errorf("igdb_match River jobs: want 1 (the re-run gives the item no second one), got %d", n)
	}

	// Settle the one item; the Job then completes.
	var itemID string
	if err := testDB.NewRaw(`SELECT id FROM job_items WHERE job_id = ?`, jobID).Scan(bg, &itemID); err != nil {
		t.Fatal(err)
	}
	if err := lc.FinishItem(bg, itemID, itemOwner(t, itemID), joblifecycle.ItemResult{Status: models.JobItemStatusCompleted}); err != nil {
		t.Fatalf("settle item: %v", err)
	}
	if status, _ := loadJobState(t, jobID); status != "completed" {
		t.Errorf("job status: want completed, got %q", status)
	}
}

// Cancel leaves a queued dispatch in River. When it runs afterwards it must not
// turn the cancelled Job back into a running sync, and must fetch nothing.
func TestDispatchSync_CancelledJob_StaysCancelledAndFetchesNothing(t *testing.T) {
	truncateAllTables(t)
	userID, jobID := seedGOGSyncJob(t, models.JobStatusCancelled)

	adapter := &hookAdapter{batches: [][]tasks.ExternalGameEntry{{{ExternalID: "1", Title: "Game One"}}}}
	built := 0
	factory := func(context.Context, string, models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
		built++
		return adapter, nil
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: factory, LC: newTestLC(t)}
	runDispatch(t, context.Background(), w, jobID, userID)

	if status, _ := loadJobState(t, jobID); status != models.JobStatusCancelled {
		t.Errorf("job status: want cancelled, got %q", status)
	}
	if built != 0 || adapter.calls != 0 {
		t.Errorf("nothing may be fetched for a cancelled Job (adapter built %d, fetched %d)", built, adapter.calls)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM external_games WHERE user_id = ?`, userID); n != 0 {
		t.Errorf("external_games: want 0, got %d", n)
	}
}

// A dispatch that was already running when the user cancelled, and then fails,
// must not turn 'cancelled' into 'failed' or announce a failed sync.
func TestDispatchSync_FailsAfterCancel_StaysCancelledNoNotification(t *testing.T) {
	truncateAllTables(t)
	userID, jobID := seedGOGSyncJob(t, "pending")

	lc := newTestLC(t)
	adapter := &hookAdapter{
		afterFetch: func() {
			if err := lc.Cancel(context.Background(), jobID); err != nil {
				t.Errorf("cancel job: %v", err)
			}
		},
		err: errors.New("upstream exploded"),
	}
	w := &tasks.DispatchSyncWorker{DB: testDB, Adapter: adapterFactory(adapter), LC: lc}
	runDispatch(t, context.Background(), w, jobID, userID)
	runSyncEffects(t, lc)

	if status, _ := loadJobState(t, jobID); status != models.JobStatusCancelled {
		t.Errorf("job status: want cancelled, got %q", status)
	}
	if n := countRows(t, `SELECT COUNT(*) FROM events WHERE dedup_key = ?`, jobID+":sync.failed"); n != 0 {
		t.Error("a dispatch failing after a cancel must not emit sync.failed")
	}
}

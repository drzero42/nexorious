package api_test

// Sync handlers on the Job lifecycle module (#1268): storefront retry, Skip,
// reset, and Retry of an old sync Job.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// failItemOf fails an item through its recorded owner.
func failItemOf(t *testing.T, lc *joblifecycle.Module, itemID string) {
	t.Helper()
	if err := lc.FinishItem(context.Background(), itemID, itemOwnerID(t, itemID), joblifecycle.ItemResult{
		Status: "failed", Message: "boom",
	}); err != nil {
		t.Fatalf("fail item: %v", err)
	}
}

// completedSyncWithFailedItem returns a completed sync Job whose item for eg failed.
func completedSyncWithFailedItem(t *testing.T, f *rmFixture, lc *joblifecycle.Module, eg string) (jobID, itemID string) {
	t.Helper()
	job := newSyncJob(t, lc, f.userID, rmStore)
	itemID = job.add(t, f.userID, eg)[0]
	job.done(t)
	failItemOf(t, lc, itemID)
	if s := f.str(`SELECT status FROM jobs WHERE id = ?`, job.ID); s != "completed" {
		t.Fatalf("setup: job = %s, want completed", s)
	}
	return job.ID, itemID
}

func (f *rmFixture) retryStorefront() *httptest.ResponseRecorder {
	f.t.Helper()
	return postJSONAuth(f.t, f.e, "/api/sync/"+rmStore+"/external-games/retry-failed", nil, f.token)
}

// Storefront retry retries a failed item of a completed Job (reopening it)
// and leaves one of a failed Job alone; the view offers no retry for the
// latter.
func TestStorefrontRetry_OnlyJobsThatCanReopen(t *testing.T) {
	f := newRM(t, "sf-retry-kinds")
	f.game("a", "", 0, false, "playstation-4")
	f.game("b", "", 0, false, "playstation-4")
	lc := newTestLifecycle(t)

	jobA, itemA := completedSyncWithFailedItem(t, f, lc, "a")

	failedJob := newSyncJob(t, lc, f.userID, rmStore)
	itemB := failedJob.add(t, f.userID, "b")[0]
	failItemOf(t, lc, itemB)
	if err := lc.Fail(context.Background(), failedJob.ID, joblifecycle.Failure{Message: "fetch failed", CallerRiverJobID: failedJob.Dispatch}); err != nil {
		t.Fatal(err)
	}

	list := f.list()
	if list["a"].FailedJobItemID == nil || *list["a"].FailedJobItemID != itemA {
		t.Errorf("a: failed_job_item_id = %v, want %s", list["a"].FailedJobItemID, itemA)
	}
	if list["b"].SyncStatus != "failed" || list["b"].FailedJobItemID != nil {
		t.Errorf("b = %+v, want failed with no failed_job_item_id", list["b"])
	}

	wantCode(t, f.retryStorefront(), http.StatusNoContent)

	if s := f.itemStatus(itemA); s != "pending" || itemOwnerID(t, itemA) == 0 {
		t.Errorf("a's item: want pending with a River job, got %s", s)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = ?`, jobA); s != "processing" {
		t.Errorf("a's job: want reopened, got %s", s)
	}
	if s := f.itemStatus(itemB); s != "failed" {
		t.Errorf("b's item: want failed (its Job is failed), got %s", s)
	}
}

// While another sync of the storefront is active an old Job cannot reopen:
// the storefront retry leaves its items failed and still answers 204, and
// Retry of the Job itself is refused.
func TestStorefrontRetry_AnotherSyncActive(t *testing.T) {
	f := newRM(t, "sf-retry-active")
	f.game("a", "", 0, false, "playstation-4")
	lc := newTestLifecycle(t)
	oldJob, item := completedSyncWithFailedItem(t, f, lc, "a")
	newSyncJob(t, lc, f.userID, rmStore) // the active sync

	wantCode(t, f.retryStorefront(), http.StatusNoContent)
	if s := f.itemStatus(item); s != "failed" {
		t.Errorf("item: want failed, got %s", s)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = ?`, oldJob); s != "completed" {
		t.Errorf("old job: want completed, got %s", s)
	}

	wantCode(t, postJSONAuth(t, f.e, "/api/jobs/"+oldJob+"/retry-failed", nil, f.token), http.StatusConflict)
	if s := f.itemStatus(item); s != "failed" {
		t.Errorf("item after job retry: want failed, got %s", s)
	}
}

func (f *rmFixture) skip(id string) *httptest.ResponseRecorder {
	f.t.Helper()
	return postJSONAuth(f.t, f.e, "/api/sync/ignored/"+id, nil, f.token)
}

// A Skip that finds no unfinished item of the game in an active Job sets only
// the flags: no item changes and no diff row.
func TestSkip_NoUnfinishedItem_SetsOnlyTheFlags(t *testing.T) {
	f := newRM(t, "skip-flags")
	f.game("eg", "", 100, false, "playstation-4")
	f.game("ch", "eg", 100, false, "playstation-5")
	f.game("other", "", 0, false, "playstation-4")
	lc := newTestLifecycle(t)
	job := newSyncJob(t, lc, f.userID, rmStore)
	ids := job.add(t, f.userID, "eg", "other")
	job.done(t)
	if err := lc.FinishItem(context.Background(), ids[0], itemOwnerID(t, ids[0]), joblifecycle.ItemResult{Status: "completed"}); err != nil {
		t.Fatal(err)
	}

	wantCode(t, f.skip("eg"), http.StatusNoContent)

	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE id IN ('eg', 'ch') AND is_skipped`); n != 2 {
		t.Errorf("skipped family members: want 2, got %d", n)
	}
	if s := f.itemStatus(ids[0]); s != "completed" {
		t.Errorf("finished item: want completed, got %s", s)
	}
	if s := f.itemStatus(ids[1]); s != "pending" {
		t.Errorf("another game's item: want pending, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes`); n != 0 {
		t.Errorf("changes: want 0, got %d", n)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = ?`, job.ID); s != "processing" {
		t.Errorf("job: want processing, got %s", s)
	}
}

// Skip of a game whose item is pending skips it and its waiting child's item,
// cancels the item's River job, and the cancelled worker, run anyway, writes
// nothing.
func TestSkip_PendingItem_CancelsItsRiverJob(t *testing.T) {
	f := newRM(t, "skip-pending")
	f.game("eg", "", 100, false, "playstation-4")
	f.game("ch", "eg", 0, false, "playstation-5")
	lc := newTestLifecycle(t)
	job := newSyncJob(t, lc, f.userID, rmStore)
	ids := job.add(t, f.userID, "eg", "ch")
	job.done(t)
	parent, child := ids[0], ids[1]
	if waiting, err := lc.Wait(context.Background(), child, itemOwnerID(t, child)); err != nil || !waiting {
		t.Fatalf("child wait = %v %v", waiting, err)
	}
	parentOwner := itemOwnerID(t, parent)

	wantCode(t, f.skip("eg"), http.StatusNoContent)

	for _, id := range []string{parent, child} {
		if s := f.itemStatus(id); s != "skipped" {
			t.Errorf("item %s: want skipped, got %s", id, s)
		}
	}
	if s := riverJobState(t, testDB, parentOwner); s != "cancelled" {
		t.Errorf("parent's River job: want cancelled, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes WHERE job_id = ? AND change_type = 'skipped' AND external_game_id = 'eg'`, job.ID); n != 1 {
		t.Errorf("parent's skipped row: want 1, got %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM changes WHERE external_game_id = 'ch'`); n != 0 {
		t.Errorf("child's changes: want 0, got %d", n)
	}
	if s := f.str(`SELECT status FROM jobs WHERE id = ?`, job.ID); s != "completed" {
		t.Errorf("job: want completed, got %s", s)
	}

	w := &tasks.UserGameWorker{DB: testDB, LC: lc}
	if err := w.Work(context.Background(), &river.Job[tasks.UserGameArgs]{
		JobRow: &rivertype.JobRow{ID: parentOwner, Attempt: 1, MaxAttempts: 5},
		Args:   tasks.UserGameArgs{JobItemID: parent},
	}); err != nil {
		t.Fatalf("late worker: %v", err)
	}
	if n := f.count(`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, f.userID); n != 0 {
		t.Errorf("the cancelled worker wrote %d user games", n)
	}
}

// Skip and a user_game_write for the same game both lock the Job row first,
// so they never deadlock; the item ends completed or skipped.
func TestSkip_ConcurrentWrite_NoDeadlock(t *testing.T) {
	for range 5 {
		f := newRM(t, "skip-race")
		f.game("eg", "", 100, false, "playstation-4")
		lc := newTestLifecycle(t)
		job := newSyncJob(t, lc, f.userID, rmStore)
		item := job.add(t, f.userID, "eg")[0]
		job.done(t)
		owner := itemOwnerID(t, item)

		w := &tasks.UserGameWorker{DB: testDB, LC: lc}
		var wg sync.WaitGroup
		var rec *httptest.ResponseRecorder
		var werr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			werr = w.Work(context.Background(), &river.Job[tasks.UserGameArgs]{
				JobRow: &rivertype.JobRow{ID: owner, Attempt: 1, MaxAttempts: 5},
				Args:   tasks.UserGameArgs{JobItemID: item},
			})
		}()
		go func() { defer wg.Done(); rec = f.skip("eg") }()
		wg.Wait()
		if werr != nil {
			t.Fatalf("worker: %v", werr)
		}
		wantCode(t, rec, http.StatusNoContent)
		if s := f.itemStatus(item); s != "completed" && s != "skipped" {
			t.Fatalf("item: want completed or skipped, got %s", s)
		}
	}
}

// Reset during a running sync cancels it through the module (no unfinished
// item, no queued River job) and deletes the storefront's games.
func TestResetSyncData_DuringASync(t *testing.T) {
	f := newRM(t, "reset-during")
	f.game("eg", "", 0, false, "playstation-4")
	lc := newTestLifecycle(t)
	job := newSyncJob(t, lc, f.userID, rmStore)
	job.add(t, f.userID, "eg")

	wantCode(t, deleteAuth(t, f.e, "/api/sync/"+rmStore+"/data", f.token), http.StatusNoContent)

	if s := f.str(`SELECT status FROM jobs WHERE id = ?`, job.ID); s != "cancelled" {
		t.Errorf("job: want cancelled, got %s", s)
	}
	if n := f.count(`SELECT COUNT(*) FROM job_items WHERE job_id = ? AND status IN ('pending', 'pending_review')`, job.ID); n != 0 {
		t.Errorf("unfinished items: %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM river_job WHERE state IN ('available', 'scheduled', 'retryable', 'pending')`); n != 0 {
		t.Errorf("queued River jobs left: %d", n)
	}
	if n := f.count(`SELECT COUNT(*) FROM external_games WHERE user_id = ?`, f.userID); n != 0 {
		t.Errorf("external games left: %d", n)
	}
}

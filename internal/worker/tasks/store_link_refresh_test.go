package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/services/storelink"
	"github.com/drzero42/nexorious/internal/usergame"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// seedStoreLinkJobItem creates a store_link_refresh Job for the user's
// storefront with one item, through the lifecycle module, and returns the Job,
// the item and the item's owning River job.
func seedStoreLinkJobItem(t *testing.T, lc *joblifecycle.Module, userID, storefront string) (jobID, itemID string, rid int64) {
	t.Helper()
	itemID = uuid.NewString()
	c, err := lc.CreateWithItems(context.Background(), joblifecycle.NewJob{
		UserID: userID, JobType: models.JobTypeStoreLinkRefresh, Source: storefront, Priority: models.JobPriorityLow, TotalItems: 1,
	}, []joblifecycle.NewItem{{
		ID: itemID, UserID: userID, ItemKey: storefront, SourceTitle: storefront,
		SourceMetadata: json.RawMessage(`{"storefront":"` + storefront + `","force":false}`),
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.JobID, itemID, itemOwner(t, itemID)
}

// cancelOnceResolver cancels the supplied context on its first Resolve call,
// then returns no link. Used to exercise the mid-loop cancellation path.
type cancelOnceResolver struct {
	cancel context.CancelFunc
	done   bool
}

func (c *cancelOnceResolver) Resolve(_ context.Context, _ string, _ map[string]string) (string, error) {
	if !c.done {
		c.done = true
		c.cancel()
	}
	return "", nil
}

func seedExternalGameSL(t *testing.T, userID, storefront, externalID string, storeLink *string) {
	t.Helper()
	_, err := testDB.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, store_link, is_available, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, true, now(), now())`,
		uuid.NewString(), userID, storefront, externalID, "T-"+externalID, storeLink,
	).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestStoreLinkRefreshDispatch_IncrementalSelectsOnlyNullRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	filled := "440"
	seedExternalGameSL(t, userID, "steam", "10", nil)
	seedExternalGameSL(t, userID, "steam", "20", &filled)

	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB}
	groups, total, err := w.SelectGroups(ctx, tasks.StoreLinkRefreshDispatchArgs{Force: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || total != 1 {
		t.Fatalf("incremental: groups=%v total=%d, want 1 group / 1 row", groups, total)
	}
}

func TestStoreLinkRefreshDispatch_ForceSelectsAllRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	filled := "440"
	seedExternalGameSL(t, userID, "steam", "10", nil)
	seedExternalGameSL(t, userID, "steam", "20", &filled)

	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB}
	groups, total, err := w.SelectGroups(ctx, tasks.StoreLinkRefreshDispatchArgs{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || total != 2 {
		t.Fatalf("force: groups=%v total=%d, want 1 group / 2 rows", groups, total)
	}
}

func TestStoreLinkRefreshDispatch_ScopeFiltersStorefront(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	seedExternalGameSL(t, userID, "steam", "10", nil)
	seedExternalGameSL(t, userID, "gog", "99", nil)

	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB}
	groups, _, err := w.SelectGroups(ctx, tasks.StoreLinkRefreshDispatchArgs{UserID: userID, Storefront: "gog"})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Storefront != "gog" {
		t.Fatalf("scoped: groups=%v, want only gog", groups)
	}
}

type fakeResolver struct{ links map[string]string }

func (f fakeResolver) Resolve(_ context.Context, externalID string, _ map[string]string) (string, error) {
	return f.links[externalID], nil
}

func TestStoreLinkRefreshItem_ResolvesGroupRows(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	seedExternalGameSL(t, userID, "steam", "10", nil)
	seedExternalGameSL(t, userID, "steam", "20", nil)

	lc := newTestLC(t)
	jobID, itemID, rid := seedStoreLinkJobItem(t, lc, userID, "steam")

	w := &tasks.StoreLinkRefreshItemWorker{
		DB: testDB,
		LC: lc,
		ResolverFor: func(_ context.Context, _, _ string) (storelink.Resolver, error) {
			return fakeResolver{links: map[string]string{"10": "10", "20": "20"}}, nil
		},
	}
	if err := w.Work(ctx, riverJob(rid, 1, tasks.StoreLinkRefreshItemArgs{JobItemID: itemID})); err != nil {
		t.Fatal(err)
	}
	if s := jobStatus(t, jobID); s != models.JobStatusCompleted {
		t.Errorf("job = %s, want completed", s)
	}
	var n int
	if err := testDB.NewRaw(
		`SELECT count(*) FROM external_games WHERE user_id = ? AND storefront = 'steam' AND store_link IS NOT NULL`, userID,
	).Scan(ctx, &n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("resolved rows = %d, want 2", n)
	}
}

func TestStoreLinkRefreshItem_ResolverErrorMarksFailed(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	seedExternalGameSL(t, userID, "steam", "10", nil)
	lc := newTestLC(t)
	_, itemID, rid := seedStoreLinkJobItem(t, lc, userID, "steam")

	w := &tasks.StoreLinkRefreshItemWorker{
		DB: testDB,
		LC: lc,
		ResolverFor: func(_ context.Context, _, _ string) (storelink.Resolver, error) {
			return nil, errors.New("creds boom")
		},
	}
	if err := w.Work(ctx, riverJob(rid, 1, tasks.StoreLinkRefreshItemArgs{JobItemID: itemID})); err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if got, _ := itemStatus(t, itemID); got != "failed" {
		t.Fatalf("item status = %q, want failed", got)
	}
}

// An item interrupted by a cancelled context (a shutdown) records nothing: it
// returns the context's error, so River puts it back, and stays pending.
func TestStoreLinkRefreshItem_InterruptedIsNotFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	// Two rows so the loop runs a second iteration after the context is cancelled.
	seedExternalGameSL(t, userID, "steam", "10", nil)
	seedExternalGameSL(t, userID, "steam", "20", nil)
	lc := newTestLC(t)
	_, itemID, rid := seedStoreLinkJobItem(t, lc, userID, "steam")

	w := &tasks.StoreLinkRefreshItemWorker{
		DB: testDB,
		LC: lc,
		ResolverFor: func(_ context.Context, _, _ string) (storelink.Resolver, error) {
			return &cancelOnceResolver{cancel: cancel}, nil
		},
	}
	if err := w.Work(ctx, riverJob(rid, 1, tasks.StoreLinkRefreshItemArgs{JobItemID: itemID})); !errors.Is(err, context.Canceled) {
		t.Fatalf("Work = %v, want context.Canceled", err)
	}
	if got, _ := itemStatus(t, itemID); got != models.JobItemStatusPending {
		t.Fatalf("item status = %q, want pending", got)
	}
	if got := itemOwner(t, itemID); got != rid {
		t.Fatalf("item owner changed: %d", got)
	}
}

// A store-link item whose River job is gone (the case the old reaper failed)
// is given a new River job by recovery, not failed.
func TestStoreLinkRefreshItem_RecoveryRequeuesLostItem(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	lc := newTestLC(t)
	_, itemID, rid := seedStoreLinkJobItem(t, lc, userID, "playstation-store")
	if _, err := testDB.ExecContext(ctx, `DELETE FROM river_job WHERE id = ?`, rid); err != nil {
		t.Fatal(err)
	}
	res, err := lc.Recover(ctx)
	if err != nil || res.Requeued != 1 {
		t.Fatalf("recover = %+v %v", res, err)
	}
	if got, _ := itemStatus(t, itemID); got != models.JobItemStatusPending {
		t.Fatalf("item = %s, want pending", got)
	}
	if got := itemOwner(t, itemID); got == rid {
		t.Fatal("item not given a new river job")
	}
}

// The refresh after a sync runs before any Job exists: it creates its Job with
// all items, and nothing when there is nothing to resolve.
func TestStoreLinkRefreshDispatch_AfterSync(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	lc := newTestLC(t)
	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB, LC: lc}
	args := tasks.StoreLinkRefreshDispatchArgs{UserID: userID, Storefront: "steam"}

	if err := w.Work(ctx, riverJob(1, 1, args)); err != nil {
		t.Fatal(err)
	}
	var jobs int
	_ = testDB.NewRaw(`SELECT count(*) FROM jobs`).Scan(ctx, &jobs)
	if jobs != 0 {
		t.Fatalf("nothing to resolve created %d jobs", jobs)
	}

	seedExternalGameSL(t, userID, "steam", "10", nil)
	if err := w.Work(ctx, riverJob(2, 1, args)); err != nil {
		t.Fatal(err)
	}
	var job models.Job
	if err := testDB.NewSelect().Model(&job).Where("job_type = ?", models.JobTypeStoreLinkRefresh).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if job.Source != "steam" || job.UserID != userID || job.Status != models.JobStatusProcessing || job.TotalItems != 1 {
		t.Fatalf("job = %+v", job)
	}
	// An equivalent Job is active: a second dispatch creates nothing.
	if err := w.Work(ctx, riverJob(3, 1, args)); err != nil {
		t.Fatal(err)
	}
	_ = testDB.NewRaw(`SELECT count(*) FROM jobs`).Scan(ctx, &jobs)
	if jobs != 1 {
		t.Fatalf("jobs = %d, want 1", jobs)
	}
}

func TestStoreLinkRefreshItem_TimeoutIsGenerous(t *testing.T) {
	// A per-(user, storefront) group can make hundreds of rate-limited calls, so
	// the item worker must override River's 1-minute default. Guard against a
	// regression that drops it back.
	w := &tasks.StoreLinkRefreshItemWorker{}
	if got := w.Timeout(nil); got < 10*time.Minute {
		t.Fatalf("item Timeout = %v, want >= 10m (must exceed River's 1m default)", got)
	}
}

// Clearing a user's library cancels the queued River jobs of that user's
// items. In an admin-triggered refresh the Job is the admin's and survives, so
// its item is left pending with a cancelled owner: recovery queues it again.
func TestStoreLinkRefreshItem_CancelledByClearLibraryIsRequeued(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	adminID := insertMetaRefreshAdminUser(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	lc := newTestLC(t)
	itemID := uuid.NewString()
	if _, err := lc.CreateWithItems(ctx, joblifecycle.NewJob{
		UserID: adminID, JobType: models.JobTypeStoreLinkRefresh, Source: models.JobSourceSystem, Priority: models.JobPriorityLow, TotalItems: 1,
	}, []joblifecycle.NewItem{{ID: itemID, UserID: userID, ItemKey: "steam", SourceTitle: "steam",
		SourceMetadata: json.RawMessage(`{"storefront":"steam","force":true}`)}}, nil); err != nil {
		t.Fatal(err)
	}
	owner := itemOwner(t, itemID)

	if _, err := usergame.ClearLibrary(ctx, testDB, userID); err != nil {
		t.Fatal(err)
	}
	var state string
	_ = testDB.NewRaw(`SELECT state FROM river_job WHERE id = ?`, owner).Scan(ctx, &state)
	if state != "cancelled" {
		t.Fatalf("ClearLibrary left the item's river job %s", state)
	}

	res, err := lc.Recover(ctx)
	if err != nil || res.Requeued != 1 {
		t.Fatalf("recover = %+v %v", res, err)
	}
	if got := itemOwner(t, itemID); got == owner {
		t.Fatal("item not given a new river job")
	}
}

// A dispatch that runs before any Job exists returns its errors to River and
// tells the admin once, on its last attempt only.
func TestStoreLinkRefreshDispatch_MaintFailedOnLastAttemptOnly(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB, LC: newTestLC(t)}
	args := tasks.StoreLinkRefreshDispatchArgs{UserID: userID, Storefront: "steam"}

	// Make the dispatch's query fail.
	if _, err := testDB.ExecContext(ctx, `ALTER TABLE external_games RENAME TO external_games_away`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := testDB.ExecContext(context.Background(), `ALTER TABLE external_games_away RENAME TO external_games`); err != nil {
			t.Errorf("restore table: %v", err)
		}
	})

	for attempt := 1; attempt <= 5; attempt++ {
		if err := w.Work(ctx, riverJob(77, attempt, args)); err == nil {
			t.Fatalf("attempt %d returned nil: River would not retry", attempt)
		}
		want := 0
		if attempt == 5 {
			want = 1
		}
		if n := countEvents(t, notify.TypeAdminMaintFailed); n != want {
			t.Fatalf("after attempt %d: maint_failed events = %d, want %d", attempt, n, want)
		}
	}
}

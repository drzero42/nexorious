package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/storelink"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// itemStatus returns the current status of a job_item.
func itemStatus(t *testing.T, itemID string) string {
	t.Helper()
	var s string
	if err := testDB.NewRaw(`SELECT status FROM job_items WHERE id = ?`, itemID).Scan(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// jobStatus returns the current status of a job.
func jobStatus(t *testing.T, jobID string) string {
	t.Helper()
	var s string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// seedStoreLinkJobItem inserts a store_link_refresh job + one pending item for
// the given storefront, with the item created `ageSeconds` in the past.
func seedStoreLinkJobItem(t *testing.T, userID, storefront string, ageSeconds int) (jobID, itemID string) {
	t.Helper()
	ctx := context.Background()
	jobID = uuid.NewString()
	if _, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, ?, ?, 'processing', 'low', 1, now())`,
		jobID, userID, models.JobTypeStoreLinkRefresh, storefront,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	itemID = uuid.NewString()
	meta := `{"storefront":"` + storefront + `","force":false}`
	if _, err := testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now() - (?::text || ' seconds')::interval)`,
		itemID, jobID, userID, storefront, storefront, meta, ageSeconds,
	).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	return jobID, itemID
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
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, store_link, is_available, is_subscription, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, true, false, now(), now())`,
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

	jobID := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, ?, 'steam', 'processing', 'low', 2, now())`,
		jobID, userID, models.JobTypeStoreLinkRefresh,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	itemID := uuid.NewString()
	_, err = testDB.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, 'steam', 'steam', '{"storefront":"steam","force":false}', 'pending', '{}', '[]', now())`,
		itemID, jobID, userID,
	).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}

	w := &tasks.StoreLinkRefreshItemWorker{
		DB: testDB,
		ResolverFor: func(_ context.Context, _, _ string) (storelink.Resolver, error) {
			return fakeResolver{links: map[string]string{"10": "10", "20": "20"}}, nil
		},
	}
	if err := w.ProcessItem(ctx, itemID); err != nil {
		t.Fatal(err)
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
	_, itemID := seedStoreLinkJobItem(t, userID, "steam", 0)

	w := &tasks.StoreLinkRefreshItemWorker{
		DB: testDB,
		ResolverFor: func(_ context.Context, _, _ string) (storelink.Resolver, error) {
			return nil, errors.New("creds boom")
		},
	}
	if err := w.ProcessItem(ctx, itemID); err != nil {
		t.Fatalf("ProcessItem returned error: %v", err)
	}
	if got := itemStatus(t, itemID); got != "failed" {
		t.Fatalf("item status = %q, want failed", got)
	}
}

func TestStoreLinkRefreshItem_CancelledMidLoopMarksFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	truncateAllTables(t)
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	// Two rows so the loop runs a second iteration after the context is cancelled.
	seedExternalGameSL(t, userID, "steam", "10", nil)
	seedExternalGameSL(t, userID, "steam", "20", nil)
	_, itemID := seedStoreLinkJobItem(t, userID, "steam", 0)

	w := &tasks.StoreLinkRefreshItemWorker{
		DB: testDB,
		ResolverFor: func(_ context.Context, _, _ string) (storelink.Resolver, error) {
			return &cancelOnceResolver{cancel: cancel}, nil
		},
	}
	// Returns nil (finalize-as-failed, not River retry); the item must end failed,
	// never left orphaned in pending.
	if err := w.ProcessItem(ctx, itemID); err != nil {
		t.Fatalf("ProcessItem returned error: %v", err)
	}
	if got := itemStatus(t, itemID); got != "failed" {
		t.Fatalf("item status = %q, want failed", got)
	}
}

func TestStoreLinkRefreshDispatch_ReapsOrphanedItem(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := uuid.NewString()
	insertTestUser(t, testDB, userID)
	// No external_games rows → SelectGroups is empty, so Work only performs the
	// reap and then returns without creating a new job. The orphaned item is old
	// enough to be eligible and has no River job backing it.
	jobID, itemID := seedStoreLinkJobItem(t, userID, "playstation-store", 600)

	w := &tasks.StoreLinkRefreshDispatchWorker{DB: testDB}
	if err := w.Work(ctx, &river.Job[tasks.StoreLinkRefreshDispatchArgs]{
		Args: tasks.StoreLinkRefreshDispatchArgs{Force: true},
	}); err != nil {
		t.Fatalf("dispatch Work returned error: %v", err)
	}
	if got := itemStatus(t, itemID); got != "failed" {
		t.Fatalf("orphaned item status = %q, want failed", got)
	}
	if got := jobStatus(t, jobID); got != "completed" {
		t.Fatalf("wedged job status = %q, want completed", got)
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

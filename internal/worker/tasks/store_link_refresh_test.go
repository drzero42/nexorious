package tasks_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/storelink"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

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

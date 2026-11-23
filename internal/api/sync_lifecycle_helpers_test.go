package api_test

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"

	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// newTestLifecycle returns a lifecycle module with the registered kinds,
// inserting River jobs through an insert-only client on the shared test
// database, as the server does.
func newTestLifecycle(t *testing.T) *joblifecycle.Module {
	t.Helper()
	rc, err := river.NewClient(riverdatabasesql.New(testDB.DB), &river.Config{})
	if err != nil {
		t.Fatalf("insert-only river client: %v", err)
	}
	lc := joblifecycle.New(testDB, rc)
	tasks.RegisterLifecycleKinds(lc)
	return lc
}

// syncJob is a sync Job made by the module, with its dispatch started.
type syncJob struct {
	ID       string
	Dispatch int64 // the dispatch owner's River job id
	lc       *joblifecycle.Module
}

// newSyncJob creates a sync Job for user and storefront through the module
// and starts its dispatch.
func newSyncJob(t *testing.T, lc *joblifecycle.Module, userID, storefront string) *syncJob {
	t.Helper()
	ctx := context.Background()
	c, err := lc.CreateWithDispatch(ctx, joblifecycle.NewJob{UserID: userID, JobType: "sync", Source: storefront, Priority: "high"})
	if err != nil || !c.Created {
		t.Fatalf("create sync job: %+v %v", c, err)
	}
	var owner int64
	if err := testDB.NewRaw(`SELECT river_job_id FROM jobs WHERE id = ?`, c.JobID).Scan(ctx, &owner); err != nil {
		t.Fatalf("dispatch owner: %v", err)
	}
	if _, err := lc.StartDispatch(ctx, c.JobID, owner); err != nil {
		t.Fatalf("start dispatch: %v", err)
	}
	return &syncJob{ID: c.JobID, Dispatch: owner, lc: lc}
}

// add adds one item per external game (item_key = its external_id) and
// returns their ids in order.
func (j *syncJob) add(t *testing.T, userID string, egIDs ...string) []string {
	t.Helper()
	ctx := context.Background()
	items := make([]joblifecycle.NewItem, 0, len(egIDs))
	for _, eg := range egIDs {
		var key string
		if err := testDB.NewRaw(`SELECT external_id FROM external_games WHERE id = ?`, eg).Scan(ctx, &key); err != nil {
			t.Fatalf("external game %s: %v", eg, err)
		}
		id := eg
		items = append(items, joblifecycle.NewItem{UserID: userID, ItemKey: key, SourceTitle: key, ExternalGameID: &id})
	}
	if _, err := j.lc.AddNewItems(ctx, j.ID, j.Dispatch, items); err != nil {
		t.Fatalf("add items: %v", err)
	}
	out := make([]string, len(egIDs))
	for i, eg := range egIDs {
		if err := testDB.NewRaw(`SELECT id FROM job_items WHERE job_id = ? AND external_game_id = ?`, j.ID, eg).Scan(ctx, &out[i]); err != nil {
			t.Fatalf("item of %s: %v", eg, err)
		}
	}
	return out
}

// done marks the dispatch done.
func (j *syncJob) done(t *testing.T) {
	t.Helper()
	if err := j.lc.MarkDispatched(context.Background(), j.ID, j.Dispatch, joblifecycle.Dispatched{}); err != nil {
		t.Fatalf("mark dispatched: %v", err)
	}
}

// owner returns an item's recorded owner (0 when none).
func itemOwnerID(t *testing.T, itemID string) int64 {
	t.Helper()
	var id *int64
	if err := testDB.NewRaw(`SELECT river_job_id FROM job_items WHERE id = ?`, itemID).Scan(context.Background(), &id); err != nil {
		t.Fatalf("item owner: %v", err)
	}
	if id == nil {
		return 0
	}
	return *id
}

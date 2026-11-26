package joblifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

func newItem(user, key string) NewItem {
	return NewItem{UserID: user, ItemKey: key, SourceTitle: key}
}

// startDispatch creates a Job with a dispatch and starts it; it returns the
// Job id and the dispatch River job id.
func startDispatch(t *testing.T, m *Module, user string) (string, int64) {
	t.Helper()
	ctx := context.Background()
	c, err := m.CreateWithDispatch(ctx, newJob(user))
	if err != nil {
		t.Fatal(err)
	}
	rid := *loadJob(t, c.JobID).RiverJobID
	if _, err := m.StartDispatch(ctx, c.JobID, rid); err != nil {
		t.Fatal(err)
	}
	return c.JobID, rid
}

func itemByKey(t *testing.T, jobID, key string) models.JobItem {
	t.Helper()
	for _, it := range loadItems(t, jobID) {
		if it.ItemKey == key {
			return it
		}
	}
	t.Fatalf("no item %q", key)
	return models.JobItem{}
}

// SkipItems skips an item that has an owner: the owner's River job is
// cancelled, a later write by the cancelled owner is refused, and an ended Job
// is refused before the selector runs.
func TestSkipItemsCancelsOwner(t *testing.T) {
	m, rec := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, rid := startDispatch(t, m, user)
	if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{Items: []NewItem{newItem(user, "A"), newItem(user, "B")}}); err != nil {
		t.Fatal(err)
	}
	a := itemByKey(t, jobID, "A")

	ownWrote := false
	n, err := m.SkipItems(ctx, jobID, func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error) {
		ownWrote = job.ID == jobID
		return []string{a.ID}, nil
	})
	if err != nil || n != 1 || !ownWrote {
		t.Fatalf("SkipItems = %d %v (own %v)", n, err, ownWrote)
	}
	if got := itemByKey(t, jobID, "A"); got.Status != models.JobItemStatusSkipped || got.RiverJobID != nil {
		t.Fatalf("item = %+v", got)
	}
	if s := riverState(t, *a.RiverJobID); s != riverCancelled {
		t.Errorf("owner's river job = %s, want cancelled", s)
	}
	if err := m.EndItem(ctx, a.ID, *a.RiverJobID, ItemResult{Status: models.JobItemStatusCompleted}); !errors.Is(err, ErrNotOwner) {
		t.Errorf("the cancelled owner's write = %v, want ErrNotOwner", err)
	}
	if got := itemByKey(t, jobID, "B"); got.Status != models.JobItemStatusPending {
		t.Errorf("unselected item = %s, want pending", got.Status)
	}
	if len(rec.itemCalls) != 1 || rec.itemCalls[0] != models.JobItemStatusSkipped {
		t.Errorf("observer = %v", rec.itemCalls)
	}

	// An ended Job refuses before the selector runs.
	if err := m.Cancel(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	_, err = m.SkipItems(ctx, jobID, func(context.Context, bun.Tx, *models.Job) ([]string, error) {
		t.Fatal("selector ran for an ended Job")
		return nil, nil
	})
	if !errors.Is(err, ErrJobNotActive) {
		t.Fatalf("SkipItems on a cancelled Job = %v", err)
	}
}

// CompleteItems completes one item in Pending review and one pending item
// with an owner: the owner is cancelled, no item work is queued, and the Job
// completes.
func TestCompleteItemsInReviewAndOwned(t *testing.T) {
	m, rec := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, rid := startDispatch(t, m, user)
	if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{Items: []NewItem{newItem(user, "R"), newItem(user, "P")}}); err != nil {
		t.Fatal(err)
	}
	review, owned := itemByKey(t, jobID, "R"), itemByKey(t, jobID, "P")
	if err := m.AwaitReview(ctx, review.ID, *review.RiverJobID, Review{}); err != nil {
		t.Fatal(err)
	}

	before := countRiver(t, "fake_item")
	n, err := m.CompleteItems(ctx, jobID, func(context.Context, bun.Tx, *models.Job) ([]string, error) {
		return []string{review.ID, owned.ID}, nil
	})
	if err != nil || n != 2 {
		t.Fatalf("CompleteItems = %d %v", n, err)
	}
	for _, it := range loadItems(t, jobID) {
		if it.Status != models.JobItemStatusCompleted || it.RiverJobID != nil {
			t.Errorf("item %s = %s", it.ItemKey, it.Status)
		}
	}
	if s := riverState(t, *owned.RiverJobID); s != riverCancelled {
		t.Errorf("owner's river job = %s, want cancelled", s)
	}
	if j := loadJob(t, jobID); j.Status != models.JobStatusCompleted {
		t.Fatalf("job = %s, want completed", j.Status)
	}
	if c := countRiver(t, "fake_item"); c != before {
		t.Errorf("CompleteItems queued item work: %d river jobs, want %d", c, before)
	}
	if len(rec.jobCalls) != 1 || rec.jobCalls[0] != models.JobStatusCompleted {
		t.Errorf("job observer = %v", rec.jobCalls)
	}
}

func TestAwaitReviewDivertsToNextStage(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	jobID, its := mustCreateWithItems(t, m, insertUser(t), 1)
	err := m.AwaitReview(ctx, its[0].ID, *its[0].RiverJobID, Review{
		Divert: func(context.Context, bun.Tx) (river.JobArgs, error) {
			return fakeItemArgs{JobItemID: its[0].ID}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := loadItems(t, jobID)[0]
	if got.Status != models.JobItemStatusPending || got.RiverJobID == nil || *got.RiverJobID == *its[0].RiverJobID {
		t.Fatalf("item = %+v, want pending with a new owner", got)
	}
}

func TestReclaimRunningDeletesRemovedPeriodicKinds(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	for _, kind := range append([]string{"cleanup_old_jobs"}, removedRiverKinds...) {
		if _, err := testDB.ExecContext(ctx,
			`INSERT INTO river_job (kind, args, max_attempts, state) VALUES (?, '{}', 1, 'available')`, kind); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.ReclaimRunning(ctx); err != nil {
		t.Fatal(err)
	}
	for _, kind := range removedRiverKinds {
		if c := countRiver(t, kind); c != 0 {
			t.Errorf("%s rows left: %d", kind, c)
		}
	}
	if c := countRiver(t, "cleanup_old_jobs"); c != 1 {
		t.Errorf("another kind's row was deleted")
	}
}

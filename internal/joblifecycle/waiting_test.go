package joblifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// fakeWaitsOn is the waiting hook of the fake kind: an item whose
// source_metadata names waits_on waits on the item of the Job with that
// item_key (nil when there is none).
func fakeWaitsOn(ctx context.Context, tx bun.Tx, q WaitQuery) ([]WaitEdge, error) {
	sql := `SELECT ji.id AS item_id, pi.id AS awaited_item_id
		FROM job_items ji
		LEFT JOIN job_items pi ON pi.job_id = ji.job_id AND pi.item_key = ji.source_metadata->>'waits_on'
		WHERE ji.job_id = ? AND ji.source_metadata->>'waits_on' IS NOT NULL`
	args := []any{q.JobID}
	if len(q.Items) > 0 {
		sql += ` AND ji.id IN (?)`
		args = append(args, bun.List(q.Items))
	}
	if len(q.Awaited) > 0 {
		sql += ` AND pi.id IN (?)`
		args = append(args, bun.List(q.Awaited))
	}
	var out []WaitEdge
	if err := tx.NewRaw(sql, args...).Scan(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func newWaitingModule(t *testing.T) (*Module, *recorder) {
	t.Helper()
	m, rec := newTestModule(t)
	m.kinds[0].WaitsOn = fakeWaitsOn
	return m, rec
}

func parentItem(user, key string) NewItem {
	return NewItem{UserID: user, ItemKey: key, SourceTitle: key}
}

func childItem(user, key, parentKey string) NewItem {
	meta, _ := json.Marshal(map[string]string{"waits_on": parentKey})
	return NewItem{UserID: user, ItemKey: key, SourceTitle: key, SourceMetadata: meta}
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

func mustWait(t *testing.T, m *Module, it models.JobItem, want bool) {
	t.Helper()
	got, err := m.Wait(context.Background(), it.ID, *it.RiverJobID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got != want {
		t.Fatalf("Wait = %v, want %v", got, want)
	}
}

// The whole path of a waiting child: it may wait for a parent that a later
// batch adds, recovery leaves it alone, a dispatch run again adds nothing
// twice, and the parent ending queues it.
func TestWaitForParentInLaterBatch(t *testing.T) {
	m, _ := newWaitingModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, rid := startDispatch(t, m, user)

	if n, err := m.AddNewItems(ctx, jobID, rid, []NewItem{childItem(user, "C", "P")}); err != nil || n != 1 {
		t.Fatalf("AddNewItems = %d %v", n, err)
	}
	child := itemByKey(t, jobID, "C")
	mustWait(t, m, child, true)
	if got := itemByKey(t, jobID, "C"); got.RiverJobID != nil || got.Status != models.JobItemStatusPending {
		t.Fatalf("waiting child = %+v", got)
	}
	// A long dispatch: recovery must leave the waiting child alone.
	if res, err := m.Recover(ctx); err != nil || res.Requeued+res.Failed != 0 {
		t.Fatalf("recover = %+v %v", res, err)
	}
	if got := itemByKey(t, jobID, "C"); got.RiverJobID != nil {
		t.Fatalf("recovery queued a waiting child")
	}

	// The dispatch is run again and sees the child again: only the parent is new.
	n, err := m.AddNewItems(ctx, jobID, rid, []NewItem{childItem(user, "C", "P"), parentItem(user, "P"), parentItem(user, "P")})
	if err != nil || n != 1 {
		t.Fatalf("AddNewItems again = %d %v", n, err)
	}
	if c := countRiver(t, "fake_item"); c != 2 {
		t.Fatalf("river jobs = %d, want 2 (child's first and the parent's)", c)
	}
	if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{}); err != nil {
		t.Fatal(err)
	}
	if got := itemByKey(t, jobID, "C"); got.RiverJobID != nil {
		t.Fatalf("marking the dispatch done queued a child whose parent is open")
	}

	end(t, m, itemByKey(t, jobID, "P"), models.JobItemStatusCompleted)
	child = itemByKey(t, jobID, "C")
	if child.RiverJobID == nil || riverState(t, *child.RiverJobID) != riverAvailable {
		t.Fatalf("parent ending did not queue the child: %+v", child)
	}
	if j := loadJob(t, jobID); j.Status != models.JobStatusProcessing {
		t.Fatalf("job = %s, want processing", j.Status)
	}
	end(t, m, child, models.JobItemStatusCompleted)
	if j := loadJob(t, jobID); j.Status != models.JobStatusCompleted {
		t.Fatalf("job = %s, want completed", j.Status)
	}
}

func TestCannotWait(t *testing.T) {
	ctx := context.Background()

	t.Run("awaited item ended, dispatch running", func(t *testing.T) {
		m, _ := newWaitingModule(t)
		user := insertUser(t)
		jobID, rid := startDispatch(t, m, user)
		if _, err := m.AddNewItems(ctx, jobID, rid, []NewItem{parentItem(user, "P"), childItem(user, "C", "P")}); err != nil {
			t.Fatal(err)
		}
		end(t, m, itemByKey(t, jobID, "P"), models.JobItemStatusFailed)
		child := itemByKey(t, jobID, "C")
		mustWait(t, m, child, false)
		if got := itemByKey(t, jobID, "C"); got.RiverJobID == nil || *got.RiverJobID != *child.RiverJobID {
			t.Fatalf("a refused wait changed the owner")
		}
	})

	t.Run("no awaited item, dispatch done", func(t *testing.T) {
		m, _ := newWaitingModule(t)
		user := insertUser(t)
		jobID, rid := startDispatch(t, m, user)
		if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{Items: []NewItem{childItem(user, "C", "P")}}); err != nil {
			t.Fatal(err)
		}
		mustWait(t, m, itemByKey(t, jobID, "C"), false)
	})

	t.Run("item the kind does not name", func(t *testing.T) {
		m, _ := newWaitingModule(t)
		user := insertUser(t)
		jobID, rid := startDispatch(t, m, user)
		if _, err := m.AddNewItems(ctx, jobID, rid, []NewItem{parentItem(user, "P")}); err != nil {
			t.Fatal(err)
		}
		mustWait(t, m, itemByKey(t, jobID, "P"), false)
	})
}

// A child that waits for a parent the dispatch never adds is queued when the
// dispatch is marked done, and so is an item without an owner the kind no
// longer names as waiting (a game promoted during the sweep).
func TestMarkDispatchedQueuesItemsThatCannotWait(t *testing.T) {
	m, _ := newWaitingModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, rid := startDispatch(t, m, user)
	if _, err := m.AddNewItems(ctx, jobID, rid, []NewItem{childItem(user, "C", "P"), childItem(user, "D", "P")}); err != nil {
		t.Fatal(err)
	}
	mustWait(t, m, itemByKey(t, jobID, "C"), true)
	mustWait(t, m, itemByKey(t, jobID, "D"), true)
	// D is promoted: the kind no longer names it as waiting.
	if _, err := testDB.ExecContext(ctx, `UPDATE job_items SET source_metadata = '{}' WHERE job_id = ? AND item_key = 'D'`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"C", "D"} {
		if got := itemByKey(t, jobID, key); got.RiverJobID == nil {
			t.Errorf("%s was not queued when the dispatch was marked done", key)
		}
	}
	if j := loadJob(t, jobID); j.Status != models.JobStatusProcessing {
		t.Fatalf("job = %s, want processing", j.Status)
	}
}

// A parent ending in the same instant its child asks to wait never leaves
// the child waiting on an ended item: either the wait is refused, or the
// parent's ending queues the child.
func TestParentEndsWhileChildAsksToWait(t *testing.T) {
	for range 10 {
		m, _ := newWaitingModule(t)
		ctx := context.Background()
		user := insertUser(t)
		jobID, rid := startDispatch(t, m, user)
		if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{Items: []NewItem{parentItem(user, "P"), childItem(user, "C", "P")}}); err != nil {
			t.Fatal(err)
		}
		parent, child := itemByKey(t, jobID, "P"), itemByKey(t, jobID, "C")
		var wg sync.WaitGroup
		var waitErr, endErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, waitErr = m.Wait(ctx, child.ID, *child.RiverJobID)
		}()
		go func() {
			defer wg.Done()
			endErr = m.EndItem(ctx, parent.ID, *parent.RiverJobID, ItemResult{Status: models.JobItemStatusCompleted})
		}()
		wg.Wait()
		if waitErr != nil || endErr != nil {
			t.Fatalf("wait %v, end %v", waitErr, endErr)
		}
		if got := itemByKey(t, jobID, "C"); got.Status != models.JobItemStatusPending || got.RiverJobID == nil {
			t.Fatalf("child left without an owner: %+v", got)
		}
	}
}

// Skip of an owned pending item cancels its River job, so its worker writes
// nothing, and queues the items that waited on it.
func TestSkipItemsCancelsOwnerAndQueuesWaiters(t *testing.T) {
	m, rec := newWaitingModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, rid := startDispatch(t, m, user)
	if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{Items: []NewItem{parentItem(user, "P"), childItem(user, "C", "P")}}); err != nil {
		t.Fatal(err)
	}
	parent, child := itemByKey(t, jobID, "P"), itemByKey(t, jobID, "C")
	mustWait(t, m, child, true)

	ownWrote := false
	n, err := m.SkipItems(ctx, jobID, func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error) {
		ownWrote = job.ID == jobID
		return []string{parent.ID}, nil
	})
	if err != nil || n != 1 || !ownWrote {
		t.Fatalf("SkipItems = %d %v (own %v)", n, err, ownWrote)
	}
	if got := itemByKey(t, jobID, "P"); got.Status != models.JobItemStatusSkipped || got.RiverJobID != nil {
		t.Fatalf("parent = %+v", got)
	}
	if s := riverState(t, *parent.RiverJobID); s != riverCancelled {
		t.Errorf("parent's river job = %s, want cancelled", s)
	}
	if err := m.EndItem(ctx, parent.ID, *parent.RiverJobID, ItemResult{Status: models.JobItemStatusCompleted}); !errors.Is(err, ErrNotOwner) {
		t.Errorf("the cancelled owner's write = %v, want ErrNotOwner", err)
	}
	if got := itemByKey(t, jobID, "C"); got.RiverJobID == nil {
		t.Fatalf("skipping the parent did not queue the waiting child")
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

// A parent in Pending review with a waiting child: the Job waits on the user
// (one awaiting-review effect), and a rematch completes both.
func TestCompleteByRematchWithWaitingChild(t *testing.T) {
	m, rec := newWaitingModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, rid := startDispatch(t, m, user)
	if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{Items: []NewItem{parentItem(user, "P"), childItem(user, "C", "P")}}); err != nil {
		t.Fatal(err)
	}
	parent, child := itemByKey(t, jobID, "P"), itemByKey(t, jobID, "C")
	if err := m.AwaitReview(ctx, parent.ID, *parent.RiverJobID, Review{}); err != nil {
		t.Fatal(err)
	}
	mustWait(t, m, child, true)
	if c := countRiver(t, EffectsArgs{}.Kind()); c != 1 {
		t.Fatalf("awaiting-review effects = %d, want 1", c)
	}

	before := countRiver(t, "fake_item")
	n, err := m.CompleteByRematch(ctx, jobID, func(context.Context, bun.Tx, *models.Job) ([]string, error) {
		return []string{parent.ID, child.ID}, nil
	})
	if err != nil || n != 2 {
		t.Fatalf("CompleteByRematch = %d %v", n, err)
	}
	for _, it := range loadItems(t, jobID) {
		if it.Status != models.JobItemStatusCompleted || it.RiverJobID != nil {
			t.Errorf("item %s = %s", it.ItemKey, it.Status)
		}
	}
	if j := loadJob(t, jobID); j.Status != models.JobStatusCompleted {
		t.Fatalf("job = %s, want completed", j.Status)
	}
	if c := countRiver(t, "fake_item"); c != before {
		t.Errorf("rematch queued item work: %d river jobs, want %d", c, before)
	}
	if len(rec.jobCalls) != 1 || rec.jobCalls[0] != models.JobStatusCompleted {
		t.Errorf("job observer = %v", rec.jobCalls)
	}
}

// Retrying a failed parent also retries the failed children that waited on
// it, and only those.
func TestRetryItemRetriesItsFailedWaiters(t *testing.T) {
	m, _ := newWaitingModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, rid := startDispatch(t, m, user)
	if err := m.MarkDispatched(ctx, jobID, rid, Dispatched{Items: []NewItem{
		parentItem(user, "P"), childItem(user, "C", "P"), parentItem(user, "Q"),
	}}); err != nil {
		t.Fatal(err)
	}
	parent := itemByKey(t, jobID, "P")
	mustWait(t, m, itemByKey(t, jobID, "C"), true)
	end(t, m, parent, models.JobItemStatusFailed)
	end(t, m, itemByKey(t, jobID, "C"), models.JobItemStatusFailed)
	end(t, m, itemByKey(t, jobID, "Q"), models.JobItemStatusFailed)
	if j := loadJob(t, jobID); j.Status != models.JobStatusCompleted {
		t.Fatalf("job = %s", j.Status)
	}

	n, err := m.Retry(ctx, jobID, []string{parent.ID})
	if err != nil || n != 2 {
		t.Fatalf("Retry = %d %v, want 2 (the parent and its child)", n, err)
	}
	for key, want := range map[string]string{"P": models.JobItemStatusPending, "C": models.JobItemStatusPending, "Q": models.JobItemStatusFailed} {
		got := itemByKey(t, jobID, key)
		if got.Status != want {
			t.Errorf("%s = %s, want %s", key, got.Status, want)
		}
		if want == models.JobItemStatusPending && got.RiverJobID == nil {
			t.Errorf("%s retried without a river job", key)
		}
	}
	// The child's worker then asks to wait on the retried parent.
	mustWait(t, m, itemByKey(t, jobID, "C"), true)
}

func TestAwaitReviewDivertsToNextStage(t *testing.T) {
	m, _ := newWaitingModule(t)
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

func TestSettleDeletesRemovedPeriodicKinds(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	for _, kind := range append([]string{"cleanup_old_jobs"}, removedRiverKinds...) {
		if _, err := testDB.ExecContext(ctx,
			`INSERT INTO river_job (kind, args, max_attempts, state) VALUES (?, '{}', 1, 'available')`, kind); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Settle(ctx); err != nil {
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

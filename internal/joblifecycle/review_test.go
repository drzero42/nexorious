package joblifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

type fakeNextArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (fakeNextArgs) Kind() string { return "fake_next" }

func countEffects(t *testing.T, outcome Outcome) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM river_job WHERE kind = ? AND args->>'outcome' = ?`,
		EffectsArgs{}.Kind(), string(outcome)).Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNextStageMovesOwnership(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 1)
	old := *its[0].RiverJobID

	if err := m.NextStage(ctx, its[0].ID, old+999, Stage{Args: fakeNextArgs{JobItemID: its[0].ID}}); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("non-owner: err = %v, want ErrNotOwner", err)
	}

	resolved, conf := 42, 0.9
	ownRan := false
	err := m.NextStage(ctx, its[0].ID, old, Stage{
		Args:           fakeNextArgs{JobItemID: its[0].ID},
		ResolvedIGDBID: &resolved, MatchConfidence: &conf,
		Own: func(context.Context, bun.Tx) error { ownRan = true; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	it := loadItems(t, jobID)[0]
	if it.Status != models.JobItemStatusPending || it.RiverJobID == nil || *it.RiverJobID == old {
		t.Fatalf("item = %+v, want pending with a new owner", it)
	}
	if it.ResolvedIGDBID == nil || *it.ResolvedIGDBID != 42 || it.MatchConfidence == nil || *it.MatchConfidence != 0.9 || it.ResolvedAt != nil {
		t.Errorf("fields = %v %v %v", it.ResolvedIGDBID, it.MatchConfidence, it.ResolvedAt)
	}
	if !ownRan {
		t.Error("own write did not run")
	}
	if n := countRiver(t, "fake_next"); n != 1 {
		t.Errorf("next-stage river jobs = %d, want 1", n)
	}
	// The old owner is refused from now on.
	if err := m.EndItem(ctx, it.ID, old, ItemResult{Status: models.JobItemStatusCompleted}); !errors.Is(err, ErrNotOwner) {
		t.Errorf("old owner end: err = %v, want ErrNotOwner", err)
	}
	if loadJob(t, jobID).Status != models.JobStatusProcessing {
		t.Error("job ended during a hand-over")
	}
}

func TestReviewResolveAndSkip(t *testing.T) {
	m, rec := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 3)

	// Item 0 completes; items 1 and 2 go to review. Still working until the
	// last owned item leaves pending, then waiting on the user.
	end(t, m, its[0], models.JobItemStatusCompleted)
	conf := 0.4
	if err := m.AwaitReview(ctx, its[1].ID, *its[1].RiverJobID, Review{Candidates: json.RawMessage(`[{"igdb_id":1}]`), MatchConfidence: &conf}); err != nil {
		t.Fatal(err)
	}
	if n := countEffects(t, OutcomeAwaitingReview); n != 0 {
		t.Fatalf("awaiting-review effect queued while an item is still owned: %d", n)
	}
	if err := m.AwaitReview(ctx, its[2].ID, *its[2].RiverJobID, Review{}); err != nil {
		t.Fatal(err)
	}
	if n := countEffects(t, OutcomeAwaitingReview); n != 1 {
		t.Fatalf("awaiting-review effects = %d, want 1", n)
	}
	if s := loadJob(t, jobID).Status; s != models.JobStatusProcessing {
		t.Fatalf("job = %s, want processing (waiting on the user)", s)
	}
	got := loadItems(t, jobID)
	if got[1].Status != models.JobItemStatusPendingReview || got[1].RiverJobID != nil || string(got[1].IGDBCandidates) != `[{"igdb_id": 1}]` {
		t.Fatalf("review item = %+v (candidates %s)", got[1], got[1].IGDBCandidates)
	}

	// Resolve and Skip refuse an item that is not in review.
	if err := m.Resolve(ctx, its[0].ID, 7); !errors.Is(err, ErrNotInReview) {
		t.Errorf("resolve completed item: err = %v, want ErrNotInReview", err)
	}
	if err := m.Skip(ctx, its[0].ID); !errors.Is(err, ErrNotInReview) {
		t.Errorf("skip completed item: err = %v, want ErrNotInReview", err)
	}

	// Resolve item 1: pending with an owner, chosen game stored.
	if err := m.Resolve(ctx, its[1].ID, 7); err != nil {
		t.Fatal(err)
	}
	r := loadItems(t, jobID)[1]
	if r.Status != models.JobItemStatusPending || r.RiverJobID == nil || r.ResolvedIGDBID == nil || *r.ResolvedIGDBID != 7 || r.ResolvedAt == nil {
		t.Fatalf("resolved item = %+v", r)
	}
	if riverState(t, *r.RiverJobID) != riverAvailable {
		t.Error("resolve did not queue a river job")
	}
	end(t, m, r, models.JobItemStatusCompleted)
	// A worker ending an item while another item is still in review queues the
	// awaiting-review effect again (it is not a user action); the effect is
	// idempotent through its dedup key.
	if s := loadJob(t, jobID).Status; s != models.JobStatusProcessing {
		t.Fatalf("job = %s, want processing", s)
	}
	before := countEffects(t, OutcomeAwaitingReview)

	// Skip of the last item in review completes the Job; as a user action it
	// does not queue the awaiting-review effect.
	if err := m.Skip(ctx, its[2].ID); err != nil {
		t.Fatal(err)
	}
	if n := countEffects(t, OutcomeAwaitingReview); n != before {
		t.Errorf("skip queued an awaiting-review effect")
	}
	j := loadJob(t, jobID)
	if j.Status != models.JobStatusCompleted {
		t.Fatalf("job = %s, want completed", j.Status)
	}
	if countEffects(t, OutcomeCompleted) != 1 {
		t.Error("completed effect not queued")
	}
	if s := loadItems(t, jobID)[2]; s.Status != models.JobItemStatusSkipped || s.ProcessedAt == nil {
		t.Errorf("skipped item = %+v", s)
	}
	if len(rec.jobCalls) != 1 {
		t.Errorf("job observer calls = %v", rec.jobCalls)
	}

	// Job not active.
	if err := m.Resolve(ctx, its[2].ID, 7); !errors.Is(err, ErrJobNotActive) {
		t.Errorf("resolve in an ended job: err = %v, want ErrJobNotActive", err)
	}
}

func TestReviewRefusedInCancelledJob(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	jobID, its := mustCreateWithItems(t, m, insertUser(t), 2)
	if err := m.AwaitReview(ctx, its[0].ID, *its[0].RiverJobID, Review{}); err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if err := m.Skip(ctx, its[0].ID); !errors.Is(err, ErrJobNotActive) {
		t.Errorf("skip: err = %v, want ErrJobNotActive", err)
	}
	if err := m.AwaitReview(ctx, its[1].ID, *its[1].RiverJobID, Review{}); !errors.Is(err, ErrJobNotActive) {
		t.Errorf("await review: err = %v, want ErrJobNotActive", err)
	}
}

func TestCreateWithCompletedItemAndOrder(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	base := time.Now().UTC().Truncate(time.Microsecond)
	nj := newJob(user)
	nj.TotalItems = 1
	c, err := m.CreateWithItems(ctx, nj, []NewItem{
		{UserID: user, ItemKey: "data", SourceTitle: "data", Completed: true, CreatedAt: base},
		{UserID: user, ItemKey: "b", SourceTitle: "b", CreatedAt: base.Add(time.Microsecond)},
		{UserID: user, ItemKey: "a", SourceTitle: "a", CreatedAt: base.Add(2 * time.Microsecond)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := loadItems(t, c.JobID)
	if got[0].ItemKey != "data" || got[1].ItemKey != "b" || got[2].ItemKey != "a" {
		t.Fatalf("order = %s %s %s", got[0].ItemKey, got[1].ItemKey, got[2].ItemKey)
	}
	if got[0].Status != models.JobItemStatusCompleted || got[0].RiverJobID != nil {
		t.Fatalf("data item = %+v", got[0])
	}
	if got[1].RiverJobID == nil || got[2].RiverJobID == nil {
		t.Fatal("pending items without owners")
	}
	if n := countRiver(t, "fake_item"); n != 2 {
		t.Errorf("item river jobs = %d, want 2", n)
	}
	if j := loadJob(t, c.JobID); j.TotalItems != 1 || j.Status != models.JobStatusProcessing {
		t.Errorf("job = %+v", j)
	}

	// A Job whose only item is created ended completes at once.
	nj2 := newJob(insertUser(t))
	c2, err := m.CreateWithItems(ctx, nj2, []NewItem{{UserID: nj2.UserID, ItemKey: "data", Completed: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := loadJob(t, c2.JobID).Status; s != models.JobStatusCompleted {
		t.Errorf("job of ended items = %s, want completed", s)
	}
}

func TestKindForPrefersNamedSource(t *testing.T) {
	m := New(testDB, testRC)
	args := func(*models.Job, *models.JobItem) (river.JobArgs, error) { return nil, nil }
	key := func(*models.Job) (DupKey, bool) { return DupKey{}, false }
	all := &Kind{JobType: "imp", ItemArgs: args, DuplicateKey: key}
	named := &Kind{JobType: "imp", Sources: []string{"special"}, ItemArgs: args, DuplicateKey: key}
	m.Register(all)
	m.Register(named)
	if m.kindFor("imp", "special") != named {
		t.Error("named source registered after the catch-all lost")
	}
	if m.kindFor("imp", "removed-source") != all {
		t.Error("an unlisted source did not fall to the catch-all")
	}
	if m.kindFor("other", "special") != nil {
		t.Error("matched another job type")
	}
}

func TestRecoveryFailsJobWhoseDispatchCannotRunAgain(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	m := New(testDB, testRC)
	m.Register(&Kind{
		JobType:             "nodispatch",
		ItemRiverKinds:      []string{fakeItemArgs{}.Kind()},
		DispatchLostMessage: "start it again, please",
		ItemArgs: func(_ *models.Job, it *models.JobItem) (river.JobArgs, error) {
			return fakeItemArgs{JobItemID: it.ID}, nil
		},
		DuplicateKey: func(*models.Job) (DupKey, bool) { return DupKey{}, false },
	})
	user := insertUser(t)
	// A half-created Job from an older version: dispatch not done, items
	// pending without owners.
	nj := NewJob{UserID: user, JobType: "nodispatch", Source: "x"}
	c, err := m.CreateWithItems(ctx, nj, items(user, 2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx, `UPDATE jobs SET dispatch_complete = false WHERE id = ?`, c.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx, `UPDATE job_items SET river_job_id = NULL WHERE job_id = ?`, c.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM river_job`); err != nil {
		t.Fatal(err)
	}

	res, err := m.Recover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	j := loadJob(t, c.JobID)
	if j.Status != models.JobStatusFailed || j.ErrorMessage == nil || *j.ErrorMessage != "start it again, please" {
		t.Fatalf("job = %+v", j)
	}
	if res.Requeued != 0 || countRiver(t, "fake_item") != 0 {
		t.Errorf("recovery queued items of a Job it failed: %+v", res)
	}
	for _, it := range loadItems(t, c.JobID) {
		if it.Status != models.JobItemStatusCancelled {
			t.Errorf("item = %s, want cancelled", it.Status)
		}
	}
}

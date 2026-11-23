package joblifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// ── A fake kind ──────────────────────────────────────────────────────────────

type fakeDispatchArgs struct {
	JobID string `json:"job_id"`
}

func (fakeDispatchArgs) Kind() string { return "fake_dispatch" }

type fakeItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (fakeItemArgs) Kind() string { return "fake_item" }

type recorder struct {
	mu          sync.Mutex
	itemCalls   []string
	jobCalls    []string
	effects     []Effect
	effectsFail error
}

func (r *recorder) ItemFinished(_ context.Context, _ models.Job, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.itemCalls = append(r.itemCalls, status)
}

func (r *recorder) JobFinished(_ context.Context, _ models.Job, outcome string, _ int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobCalls = append(r.jobCalls, outcome)
}

func newTestModule(t *testing.T) (*Module, *recorder) {
	t.Helper()
	truncateAllTables(t)
	rec := &recorder{}
	m := New(testDB, testRC)
	m.Register(&Kind{
		JobType:            "fake",
		DispatchRiverKinds: []string{fakeDispatchArgs{}.Kind()},
		ItemRiverKinds:     []string{fakeItemArgs{}.Kind()},
		ItemArgs: func(_ *models.Job, it *models.JobItem) (river.JobArgs, error) {
			return fakeItemArgs{JobItemID: it.ID}, nil
		},
		DispatchArgs: func(j *models.Job) (river.JobArgs, error) { return fakeDispatchArgs{JobID: j.ID}, nil },
		DuplicateKey: func(j *models.Job) (DupKey, bool) { return DupKey{Source: j.Source, UserID: j.UserID}, true },
		Effects: func(_ context.Context, _ *bun.DB, e Effect) error {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			if rec.effectsFail != nil {
				return rec.effectsFail
			}
			rec.effects = append(rec.effects, e)
			return nil
		},
		HasEffects: func(Outcome) bool { return true },
		Observer:   rec,
	})
	return m, rec
}

func insertUser(t *testing.T) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := testDB.ExecContext(context.Background(),
		`INSERT INTO users (id, username, password_hash) VALUES (?, ?, 'x')`, id, "u-"+id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newJob(user string) NewJob {
	return NewJob{UserID: user, JobType: "fake", Source: "src", Priority: models.JobPriorityLow}
}

func items(user string, n int) []NewItem {
	out := make([]NewItem, n)
	for i := range out {
		out[i] = NewItem{UserID: user, ItemKey: uuid.NewString(), SourceTitle: "t"}
	}
	return out
}

func loadJob(t *testing.T, id string) models.Job {
	t.Helper()
	var j models.Job
	if err := testDB.NewSelect().Model(&j).Where("id = ?", id).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return j
}

func loadItems(t *testing.T, jobID string) []models.JobItem {
	t.Helper()
	var out []models.JobItem
	if err := testDB.NewSelect().Model(&out).Where("job_id = ?", jobID).Order("created_at", "id").Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return out
}

func riverState(t *testing.T, id int64) string {
	t.Helper()
	var s string
	if err := testDB.NewRaw(`SELECT state FROM river_job WHERE id = ?`, id).Scan(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func setRiverState(t *testing.T, id int64, state string) {
	t.Helper()
	fin := "NULL"
	if state == riverCompleted || state == riverCancelled || state == riverDiscarded {
		fin = "now()"
	}
	if _, err := testDB.ExecContext(context.Background(),
		`UPDATE river_job SET state = ?::river_job_state, finalized_at = `+fin+` WHERE id = ?`, state, id); err != nil {
		t.Fatal(err)
	}
}

func countRiver(t *testing.T, kind string, states ...string) int {
	t.Helper()
	q := `SELECT count(*) FROM river_job WHERE kind = ?`
	args := []any{kind}
	if len(states) > 0 {
		q += ` AND state::text IN (?)`
		args = append(args, bun.List(states))
	}
	var n int
	if err := testDB.NewRaw(q, args...).Scan(context.Background(), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func mustCreateWithItems(t *testing.T, m *Module, user string, n int) (string, []models.JobItem) {
	t.Helper()
	c, err := m.CreateWithItems(context.Background(), newJob(user), items(user, n), nil)
	if err != nil || !c.Created {
		t.Fatalf("CreateWithItems: %+v, %v", c, err)
	}
	return c.JobID, loadItems(t, c.JobID)
}

func finish(t *testing.T, m *Module, it models.JobItem, status string) {
	t.Helper()
	if err := m.FinishItem(context.Background(), it.ID, *it.RiverJobID, ItemResult{Status: status, Message: "boom"}); err != nil {
		t.Fatalf("FinishItem: %v", err)
	}
}

// ── Creation and dispatch ────────────────────────────────────────────────────

func TestCreateWithDispatchRecordsOwnerAndDedups(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)

	c, err := m.CreateWithDispatch(ctx, newJob(user))
	if err != nil || !c.Created {
		t.Fatalf("create: %+v %v", c, err)
	}
	j := loadJob(t, c.JobID)
	if j.Status != models.JobStatusPending || j.DispatchComplete || j.RiverJobID == nil {
		t.Fatalf("job = %+v", j)
	}
	if riverState(t, *j.RiverJobID) != riverAvailable {
		t.Errorf("dispatch river job not queued")
	}

	dup, err := m.CreateWithDispatch(ctx, newJob(user))
	if err != nil || dup.Created || dup.JobID != c.JobID {
		t.Fatalf("duplicate create = %+v, %v; want existing id", dup, err)
	}
	if n := countRiver(t, "fake_dispatch"); n != 1 {
		t.Errorf("duplicate inserted a river job: %d", n)
	}
}

func TestDispatchOwnership(t *testing.T) {
	m, rec := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	c, _ := m.CreateWithDispatch(ctx, newJob(user))
	owner := *loadJob(t, c.JobID).RiverJobID

	if _, err := m.StartDispatch(ctx, c.JobID, owner+999); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("second dispatch river job: err = %v, want ErrNotOwner", err)
	}
	j, err := m.StartDispatch(ctx, c.JobID, owner)
	if err != nil || j.Status != models.JobStatusProcessing || j.StartedAt == nil {
		t.Fatalf("start: %+v %v", j, err)
	}
	// Run again (a River retry): continues.
	if _, err := m.StartDispatch(ctx, c.JobID, owner); err != nil {
		t.Fatalf("re-run start: %v", err)
	}
	if err := m.MarkDispatched(ctx, c.JobID, owner, Dispatched{Items: items(user, 2)}); err != nil {
		t.Fatal(err)
	}
	j2 := loadJob(t, c.JobID)
	if !j2.DispatchComplete || j2.RiverJobID != nil || j2.Status != models.JobStatusProcessing {
		t.Fatalf("after mark dispatched: %+v", j2)
	}
	if _, err := m.StartDispatch(ctx, c.JobID, owner); !errors.Is(err, ErrDispatchDone) {
		t.Fatalf("start after done: %v, want ErrDispatchDone", err)
	}
	for _, it := range loadItems(t, c.JobID) {
		finish(t, m, it, models.JobItemStatusCompleted)
	}
	if s := loadJob(t, c.JobID).Status; s != models.JobStatusCompleted {
		t.Fatalf("job = %s, want completed", s)
	}
	if len(rec.jobCalls) != 1 || len(rec.itemCalls) != 2 {
		t.Errorf("observer: jobs %v items %v", rec.jobCalls, rec.itemCalls)
	}
}

func TestMarkDispatchedWithNoItemsCompletes(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	c, _ := m.CreateWithDispatch(ctx, newJob(insertUser(t)))
	owner := *loadJob(t, c.JobID).RiverJobID
	if _, err := m.StartDispatch(ctx, c.JobID, owner); err != nil {
		t.Fatal(err)
	}
	path := "/x"
	if err := m.MarkDispatched(ctx, c.JobID, owner, Dispatched{FilePath: &path}); err != nil {
		t.Fatal(err)
	}
	j := loadJob(t, c.JobID)
	if j.Status != models.JobStatusCompleted || j.FilePath == nil || *j.FilePath != path {
		t.Fatalf("job = %+v", j)
	}
	if n := countRiver(t, EffectsArgs{}.Kind()); n != 1 {
		t.Errorf("effects jobs = %d, want 1", n)
	}
}

// A rolled-back transition leaves no River job.
func TestRolledBackCreateLeavesNoRiverJob(t *testing.T) {
	m, _ := newTestModule(t)
	user := insertUser(t)
	boom := errors.New("own write failed")
	_, err := m.CreateWithItems(context.Background(), newJob(user), items(user, 3), func(context.Context, bun.Tx) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var jobs int
	if err := testDB.NewRaw(`SELECT count(*) FROM jobs`).Scan(context.Background(), &jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || countRiver(t, "fake_item") != 0 {
		t.Errorf("rolled-back create left %d jobs, %d river jobs", jobs, countRiver(t, "fake_item"))
	}
}

// ── Items ────────────────────────────────────────────────────────────────────

func TestItemOwnership(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 2)
	it := its[0]
	owner := *it.RiverJobID

	if err := m.Owns(ctx, it.ID, owner); err != nil {
		t.Fatalf("owns: %v", err)
	}
	if err := m.FinishItem(ctx, it.ID, owner+999, ItemResult{Status: models.JobItemStatusCompleted}); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("other river job: %v, want ErrNotOwner", err)
	}
	// A River retry of the owner is the same River job id: accepted.
	finish(t, m, it, models.JobItemStatusCompleted)
	// The owner, run again after finishing: refused.
	if err := m.FinishItem(ctx, it.ID, owner, ItemResult{Status: models.JobItemStatusCompleted}); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("owner after finish: %v, want ErrNotOwner", err)
	}
	if err := m.Owns(ctx, it.ID, owner); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("owns after finish: %v", err)
	}
	if err := m.FinishItem(ctx, uuid.NewString(), owner, ItemResult{Status: models.JobItemStatusCompleted}); !errors.Is(err, ErrGone) {
		t.Fatalf("missing item: %v, want ErrGone", err)
	}
	if s := loadJob(t, jobID).Status; s != models.JobStatusProcessing {
		t.Fatalf("job finished early: %s", s)
	}
}

func TestFinishItemOwnWriteAndScrub(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	_, its := mustCreateWithItems(t, m, user, 2)

	err := m.FinishItem(ctx, its[0].ID, *its[0].RiverJobID, ItemResult{
		Status: models.JobItemStatusCompleted,
		Own: func(context.Context, bun.Tx) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.FinishItem(ctx, its[1].ID, *its[1].RiverJobID, ItemResult{
		Status: models.JobItemStatusFailed, Message: "GET https://api.example/x?key=secret failed",
	}); err != nil {
		t.Fatal(err)
	}
	got := loadItems(t, its[0].JobID)
	if string(got[0].Result) != `{"ok": true}` || got[0].RiverJobID != nil || got[0].ProcessedAt == nil {
		t.Errorf("completed item = %+v", got[0])
	}
	if got[1].ErrorMessage == nil || *got[1].ErrorMessage == "GET https://api.example/x?key=secret failed" {
		t.Errorf("error message not scrubbed: %v", got[1].ErrorMessage)
	}
}

func TestConcurrentFinishCompletesOnce(t *testing.T) {
	for range 5 {
		m, rec := newTestModule(t)
		user := insertUser(t)
		jobID, its := mustCreateWithItems(t, m, user, 2)
		var wg sync.WaitGroup
		for _, it := range its {
			wg.Go(func() { finish(t, m, it, models.JobItemStatusCompleted) })
		}
		wg.Wait()
		if s := loadJob(t, jobID).Status; s != models.JobStatusCompleted {
			t.Fatalf("job = %s", s)
		}
		if n := countRiver(t, EffectsArgs{}.Kind()); n != 1 {
			t.Fatalf("effects jobs = %d, want 1", n)
		}
		if len(rec.jobCalls) != 1 {
			t.Fatalf("JobFinished calls = %d", len(rec.jobCalls))
		}
	}
}

func TestCancelledContextRecordsNothing(t *testing.T) {
	m, _ := newTestModule(t)
	user := insertUser(t)
	_, its := mustCreateWithItems(t, m, user, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.FinishItem(ctx, its[0].ID, *its[0].RiverJobID, ItemResult{Status: models.JobItemStatusCompleted}); err == nil {
		t.Fatal("finish on cancelled context succeeded")
	}
	if s := loadItems(t, its[0].JobID)[0].Status; s != models.JobItemStatusPending {
		t.Fatalf("item = %s, want pending", s)
	}
}

// ── Cancel and Fail ──────────────────────────────────────────────────────────

func TestCancelLeavesNothingUnfinished(t *testing.T) {
	m, rec := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 3)
	// One item's River job is running: it can only be signalled.
	running := *its[0].RiverJobID
	setRiverState(t, running, riverRunning)
	// One is in review (no owner).
	if _, err := testDB.ExecContext(ctx, `UPDATE job_items SET status = 'pending_review', river_job_id = NULL WHERE id = ?`, its[2].ID); err != nil {
		t.Fatal(err)
	}

	if err := m.Cancel(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	j := loadJob(t, jobID)
	if j.Status != models.JobStatusCancelled || j.CompletedAt == nil {
		t.Fatalf("job = %+v", j)
	}
	for _, it := range loadItems(t, jobID) {
		if it.Status != models.JobItemStatusCancelled || it.RiverJobID != nil {
			t.Errorf("item %s = %s owner %v", it.ID, it.Status, it.RiverJobID)
		}
	}
	if riverState(t, *its[1].RiverJobID) != riverCancelled {
		t.Errorf("queued river job not cancelled")
	}
	var signalled bool
	if err := testDB.NewRaw(`SELECT metadata->>'cancel_attempted_at' IS NOT NULL FROM river_job WHERE id = ?`, running).Scan(ctx, &signalled); err != nil {
		t.Fatal(err)
	}
	if !signalled || riverState(t, running) != riverRunning {
		t.Errorf("running river job not signalled (signalled=%v)", signalled)
	}
	// A write attempted by the running River job afterwards is refused.
	if err := m.FinishItem(ctx, its[0].ID, running, ItemResult{Status: models.JobItemStatusCompleted}); !errors.Is(err, ErrJobNotActive) {
		t.Fatalf("late write: %v, want ErrJobNotActive", err)
	}
	if err := m.Cancel(ctx, jobID); !errors.Is(err, ErrJobNotActive) {
		t.Fatalf("second cancel: %v", err)
	}
	if n := countRiver(t, EffectsArgs{}.Kind()); n != 0 {
		t.Errorf("cancel queued %d effects jobs", n)
	}
	if len(rec.itemCalls) != 0 {
		t.Errorf("cancelled items counted: %v", rec.itemCalls)
	}
}

func TestFailByDispatchOwner(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	c, _ := m.CreateWithDispatch(ctx, newJob(user))
	owner := *loadJob(t, c.JobID).RiverJobID
	if _, err := m.StartDispatch(ctx, c.JobID, owner); err != nil {
		t.Fatal(err)
	}
	if err := m.AddItems(ctx, c.JobID, owner, items(user, 2)); err != nil {
		t.Fatal(err)
	}
	var ownWrote bool
	err := m.Fail(ctx, c.JobID, Failure{
		Message: "dispatch broke", CallerRiverJobID: owner, KindData: json.RawMessage(`{"k":1}`),
		// Own's result replaces KindData: the effects learn what the write found.
		Own: func(context.Context, bun.Tx) (json.RawMessage, error) {
			ownWrote = true
			return json.RawMessage(`{"k":2}`), nil
		},
	})
	if err != nil || !ownWrote {
		t.Fatalf("fail: %v own=%v", err, ownWrote)
	}
	j := loadJob(t, c.JobID)
	if j.Status != models.JobStatusFailed || j.ErrorMessage == nil || *j.ErrorMessage != "dispatch broke" {
		t.Fatalf("job = %+v", j)
	}
	if riverState(t, owner) != riverAvailable {
		t.Errorf("caller's own river job was cancelled")
	}
	if n := countRiver(t, "fake_item", riverAvailable); n != 0 {
		t.Errorf("%d item river jobs still queued", n)
	}
	var data string
	if err := testDB.NewRaw(`SELECT args->>'kind_data' FROM river_job WHERE kind = ?`, EffectsArgs{}.Kind()).Scan(ctx, &data); err != nil {
		t.Fatal(err)
	}
	if data != `{"k": 2}` {
		t.Errorf("kind data = %q", data)
	}
	if err := m.Fail(ctx, c.JobID, Failure{Message: "x", CallerRiverJobID: owner}); !errors.Is(err, ErrJobNotActive) {
		t.Errorf("second fail: %v", err)
	}
}

// ── Retry ────────────────────────────────────────────────────────────────────

func TestRetryReopensCompletedJob(t *testing.T) {
	m, rec := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 2)
	finish(t, m, its[0], models.JobItemStatusCompleted)
	finish(t, m, its[1], models.JobItemStatusFailed)
	first := loadJob(t, jobID)
	if first.Status != models.JobStatusCompleted {
		t.Fatalf("job = %s", first.Status)
	}

	n, err := m.Retry(ctx, jobID, nil)
	if err != nil || n != 1 {
		t.Fatalf("retry: %d %v", n, err)
	}
	j := loadJob(t, jobID)
	if j.Status != models.JobStatusProcessing || j.CompletedAt == nil {
		t.Fatalf("reopened job = %+v", j)
	}
	retried := loadItems(t, jobID)[1]
	if retried.Status != models.JobItemStatusPending || retried.ErrorMessage != nil || retried.RiverJobID == nil {
		t.Fatalf("retried item = %+v", retried)
	}
	finish(t, m, retried, models.JobItemStatusCompleted)
	if s := loadJob(t, jobID).Status; s != models.JobStatusCompleted {
		t.Fatalf("job = %s", s)
	}
	if len(rec.jobCalls) != 1 {
		t.Errorf("observer JobFinished = %d, want 1 (not again on the second completion)", len(rec.jobCalls))
	}
	var firstFinishes []bool
	if err := testDB.NewRaw(`SELECT (args->>'first_finish')::bool FROM river_job WHERE kind = ? ORDER BY id`, EffectsArgs{}.Kind()).Scan(ctx, &firstFinishes); err != nil {
		t.Fatal(err)
	}
	if len(firstFinishes) != 2 || !firstFinishes[0] || firstFinishes[1] {
		t.Errorf("effects first_finish = %v, want [true false]", firstFinishes)
	}

	// No failed items: nothing changes.
	if n, err := m.Retry(ctx, jobID, nil); err != nil || n != 0 {
		t.Fatalf("empty retry: %d %v", n, err)
	}
	if s := loadJob(t, jobID).Status; s != models.JobStatusCompleted {
		t.Fatalf("empty retry reopened the job: %s", s)
	}
}

func TestRetryRefusals(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 1)
	finish(t, m, its[0], models.JobItemStatusFailed)

	// Another active Job with the same duplicate key blocks the reopen.
	if _, err := m.CreateWithDispatch(ctx, newJob(user)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Retry(ctx, jobID, nil); !errors.Is(err, ErrDuplicateActive) {
		t.Fatalf("retry with duplicate active: %v", err)
	}
	if s := loadItems(t, jobID)[0].Status; s != models.JobItemStatusFailed {
		t.Fatalf("refused retry changed the item: %s", s)
	}

	// A cancelled Job is never reopened.
	other, _ := mustCreateWithItems(t, m, insertUser(t), 1)
	if err := m.Cancel(ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Retry(ctx, other, nil); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("retry cancelled job: %v", err)
	}
	// An item that is not failed.
	third, its3 := mustCreateWithItems(t, m, insertUser(t), 1)
	if _, err := m.Retry(ctx, third, []string{its3[0].ID}); !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("retry pending item: %v", err)
	}
}

// ── Completion and effects ───────────────────────────────────────────────────

func TestAwaitingReviewQueuesEffectOnce(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 2)
	// A stage 1 transition never produces pending_review: the test puts one
	// item there directly.
	if _, err := testDB.ExecContext(ctx, `UPDATE job_items SET status = 'pending_review', river_job_id = NULL WHERE id = ?`, its[0].ID); err != nil {
		t.Fatal(err)
	}
	finish(t, m, its[1], models.JobItemStatusCompleted)
	if s := loadJob(t, jobID).Status; s != models.JobStatusProcessing {
		t.Fatalf("job = %s, want processing (waiting on the user)", s)
	}
	var outcomes []string
	if err := testDB.NewRaw(`SELECT args->>'outcome' FROM river_job WHERE kind = ?`, EffectsArgs{}.Kind()).Scan(ctx, &outcomes); err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0] != string(OutcomeAwaitingReview) {
		t.Fatalf("effects = %v", outcomes)
	}
	// A user action leaving it waiting does not queue it again.
	st := &txState{}
	err := testDB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		job, err := lockJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		return m.evaluate(ctx, tx, st, m.kinds[0], job, true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countRiver(t, EffectsArgs{}.Kind()); n != 1 {
		t.Errorf("effects after user action = %d, want 1", n)
	}
}

func TestEffectsWorker(t *testing.T) {
	m, rec := newTestModule(t)
	ctx := context.Background()
	w := &EffectsWorker{Module: m}
	jobID, its := mustCreateWithItems(t, m, insertUser(t), 1)
	finish(t, m, its[0], models.JobItemStatusCompleted)

	job := func(attempt int) *river.Job[EffectsArgs] {
		return &river.Job[EffectsArgs]{
			JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 5},
			Args:   EffectsArgs{JobID: jobID, Outcome: OutcomeCompleted, FirstFinish: true},
		}
	}
	rec.effectsFail = errors.New("smtp down")
	if err := w.Work(ctx, job(1)); err == nil {
		t.Fatal("failing effect returned nil: River would not retry it")
	}
	rec.effectsFail = nil
	if err := w.Work(ctx, job(2)); err != nil {
		t.Fatal(err)
	}
	if len(rec.effects) != 1 || rec.effects[0].Job.ID != jobID || !rec.effects[0].FirstFinish {
		t.Fatalf("effects = %+v", rec.effects)
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := w.Work(ctx, job(1)); err != nil || len(rec.effects) != 1 {
		t.Fatalf("effect for a deleted job: err=%v effects=%d", err, len(rec.effects))
	}
}

// ── Recovery ─────────────────────────────────────────────────────────────────

func TestRecoveryItems(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 5)
	// 0 live (left alone), 1 discarded (failed), 2 completed without a
	// transition (failed), 3 cancelled (re-queued), 4 row gone (re-queued).
	setRiverState(t, *its[1].RiverJobID, riverDiscarded)
	setRiverState(t, *its[2].RiverJobID, riverCompleted)
	setRiverState(t, *its[3].RiverJobID, riverCancelled)
	if _, err := testDB.ExecContext(ctx, `DELETE FROM river_job WHERE id = ?`, *its[4].RiverJobID); err != nil {
		t.Fatal(err)
	}

	res, err := m.Recover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 2 || res.Requeued != 2 {
		t.Fatalf("result = %+v, want 2 failed, 2 re-queued", res)
	}
	got := loadItems(t, jobID)
	want := []string{models.JobItemStatusPending, models.JobItemStatusFailed, models.JobItemStatusFailed, models.JobItemStatusPending, models.JobItemStatusPending}
	for i, it := range got {
		if it.Status != want[i] {
			t.Errorf("item %d = %s, want %s", i, it.Status, want[i])
		}
	}
	if *got[0].RiverJobID != *its[0].RiverJobID {
		t.Errorf("live owner replaced")
	}
	for _, i := range []int{3, 4} {
		if got[i].RiverJobID == nil || *got[i].RiverJobID == *its[i].RiverJobID || riverState(t, *got[i].RiverJobID) != riverAvailable {
			t.Errorf("item %d not given a new river job", i)
		}
	}
	// Running it again changes nothing: nothing re-runs work that ended by itself.
	res, err = m.Recover(ctx)
	if err != nil || res.Failed+res.Requeued != 0 {
		t.Fatalf("second run = %+v, %v", res, err)
	}
}

func TestRecoveryAdoptsRiverJobFromBeforeTheUpgrade(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 1)
	// The row as an older version left it: no recorded owner, but a queued
	// River job names it in its args.
	if _, err := testDB.ExecContext(ctx, `UPDATE job_items SET river_job_id = NULL WHERE id = ?`, its[0].ID); err != nil {
		t.Fatal(err)
	}
	res, err := m.Recover(ctx)
	if err != nil || res.Requeued != 0 {
		t.Fatalf("recover = %+v %v", res, err)
	}
	if got := loadItems(t, jobID)[0]; got.RiverJobID == nil || *got.RiverJobID != *its[0].RiverJobID {
		t.Fatalf("queued river job not adopted: %v", got.RiverJobID)
	}
	if n := countRiver(t, "fake_item"); n != 1 {
		t.Errorf("river jobs = %d, want 1", n)
	}
}

func TestRecoveryDispatch(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	mk := func() (string, int64) {
		c, err := m.CreateWithDispatch(ctx, newJob(insertUser(t)))
		if err != nil {
			t.Fatal(err)
		}
		return c.JobID, *loadJob(t, c.JobID).RiverJobID
	}
	discarded, d1 := mk()
	setRiverState(t, d1, riverDiscarded)
	noDone, d2 := mk()
	setRiverState(t, d2, riverCompleted)
	lost, d3 := mk()
	if _, err := testDB.ExecContext(ctx, `DELETE FROM river_job WHERE id = ?`, d3); err != nil {
		t.Fatal(err)
	}
	unrecorded, d4 := mk()
	if _, err := testDB.ExecContext(ctx, `UPDATE jobs SET river_job_id = NULL WHERE id = ?`, unrecorded); err != nil {
		t.Fatal(err)
	}
	live, d5 := mk()

	res, err := m.Recover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 2 || res.Requeued != 1 {
		t.Fatalf("result = %+v", res)
	}
	for id, want := range map[string]string{discarded: "failed", noDone: "failed", lost: "pending", unrecorded: "pending", live: "pending"} {
		if s := loadJob(t, id).Status; s != want {
			t.Errorf("job %s = %s, want %s", id, s, want)
		}
	}
	if j := loadJob(t, lost); j.RiverJobID == nil || *j.RiverJobID == d3 {
		t.Errorf("lost dispatch not inserted again")
	}
	if j := loadJob(t, unrecorded); j.RiverJobID == nil || *j.RiverJobID != d4 {
		t.Errorf("unrecorded dispatch not adopted")
	}
	if j := loadJob(t, live); *j.RiverJobID != d5 {
		t.Errorf("live dispatch touched")
	}
}

func TestRecoveryCompletesFinishedJob(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	jobID, its := mustCreateWithItems(t, m, user, 1)
	// A processing Job whose items all finished without completing it.
	if _, err := testDB.ExecContext(ctx, `UPDATE job_items SET status = 'completed', river_job_id = NULL WHERE id = ?`, its[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if s := loadJob(t, jobID).Status; s != models.JobStatusCompleted {
		t.Fatalf("job = %s", s)
	}
}

// Recovery leaves Jobs of kinds not on the module alone.
func TestRecoveryLeavesOtherKindsAlone(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, dispatch_complete) VALUES ('sync-1', ?, 'sync', 'steam', 'processing', 'high', false)`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, status) VALUES ('si-1', 'sync-1', ?, 'k', 't', 'pending')`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO river_job (kind, args, state, attempt, max_attempts, attempted_at, queue, priority) VALUES ('igdb_match', '{"job_item_id":"si-1"}', 'running', 1, 1, now(), 'default', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Settle(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := m.Recover(ctx)
	if err != nil || res.Failed+res.Requeued != 0 {
		t.Fatalf("recover = %+v %v", res, err)
	}
	if s := loadJob(t, "sync-1").Status; s != models.JobStatusProcessing {
		t.Errorf("sync job = %s", s)
	}
	if n := countRiver(t, "igdb_match", riverRunning); n != 1 {
		t.Errorf("sync river job settled")
	}
}

func TestSettle(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)
	_, its := mustCreateWithItems(t, m, user, 3)
	// 0: attempts left; 1: none left; 2: signalled to stop.
	for i, q := range []string{
		`UPDATE river_job SET state = 'running', attempt = 1, max_attempts = 5, attempted_at = now() WHERE id = ?`,
		`UPDATE river_job SET state = 'running', attempt = 5, max_attempts = 5, attempted_at = now() WHERE id = ?`,
		`UPDATE river_job SET state = 'running', attempt = 1, max_attempts = 5, attempted_at = now(), metadata = '{"cancel_attempted_at":"2026-01-01T00:00:00Z"}' WHERE id = ?`,
	} {
		if _, err := testDB.ExecContext(ctx, q, *its[i].RiverJobID); err != nil {
			t.Fatal(err)
		}
	}
	n, err := m.Settle(ctx)
	if err != nil || n != 3 {
		t.Fatalf("settle = %d %v", n, err)
	}
	for i, want := range []string{riverRetryable, riverDiscarded, riverCancelled} {
		if s := riverState(t, *its[i].RiverJobID); s != want {
			t.Errorf("river job %d = %s, want %s", i, s, want)
		}
	}
	// Then the rule: the discarded one fails its item, the cancelled one is
	// re-queued, the retryable one is left to River.
	res, err := m.Recover(ctx)
	if err != nil || res.Failed != 1 || res.Requeued != 1 {
		t.Fatalf("recover = %+v %v", res, err)
	}
	// Both steps again: nothing changes.
	if n, _ := m.Settle(ctx); n != 0 {
		t.Errorf("second settle moved %d", n)
	}
	if res, _ := m.Recover(ctx); res.Failed+res.Requeued != 0 {
		t.Errorf("second recover = %+v", res)
	}
}

// A pending item from before the upgrade (no recorded owner) whose River job
// is gone is given a new one.
func TestRecoveryQueuesUnownedItemWithoutRiverJob(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	jobID, its := mustCreateWithItems(t, m, insertUser(t), 1)
	if _, err := testDB.ExecContext(ctx, `UPDATE job_items SET river_job_id = NULL WHERE id = ?`, its[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM river_job WHERE id = ?`, *its[0].RiverJobID); err != nil {
		t.Fatal(err)
	}
	res, err := m.Recover(ctx)
	if err != nil || res.Requeued != 1 {
		t.Fatalf("recover = %+v %v", res, err)
	}
	got := loadItems(t, jobID)[0]
	if got.RiverJobID == nil || riverState(t, *got.RiverJobID) != riverAvailable {
		t.Fatalf("item not given a river job: %v", got.RiverJobID)
	}
}

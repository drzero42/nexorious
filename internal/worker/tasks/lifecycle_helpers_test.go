package tasks_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/riverqueue/river"
	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"

	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// newTestLC returns a lifecycle module with the registered kinds, inserting
// River jobs through an insert-only client on the shared test database.
func newTestLC(t *testing.T) *joblifecycle.Module {
	t.Helper()
	rc, err := river.NewClient(riverdatabasesql.New(testDB.DB), &river.Config{})
	if err != nil {
		t.Fatalf("insert-only river client: %v", err)
	}
	lc := joblifecycle.New(testDB, rc)
	tasks.RegisterLifecycleKinds(lc)
	return lc
}

// itemOwner returns the River job recorded as the owner of a pending item.
func itemOwner(t *testing.T, itemID string) int64 {
	t.Helper()
	var id *int64
	if err := testDB.NewRaw(`SELECT river_job_id FROM job_items WHERE id = ?`, itemID).Scan(context.Background(), &id); err != nil {
		t.Fatalf("item owner: %v", err)
	}
	if id == nil {
		t.Fatalf("item %s has no owner", itemID)
	}
	return *id
}

// dispatchOwner returns the River job recorded as the owner of a Job's dispatch.
func dispatchOwner(t *testing.T, jobID string) int64 {
	t.Helper()
	var id *int64
	if err := testDB.NewRaw(`SELECT river_job_id FROM jobs WHERE id = ?`, jobID).Scan(context.Background(), &id); err != nil {
		t.Fatalf("dispatch owner: %v", err)
	}
	if id == nil {
		t.Fatalf("job %s has no dispatch owner", jobID)
	}
	return *id
}

// riverJob builds the River job a worker is run with: riverJobID is the River
// job's id (the recorded owner, for lifecycle work), attempt of 5.
func riverJob[T river.JobArgs](riverJobID int64, attempt int, args T) *river.Job[T] {
	return &river.Job[T]{JobRow: &rivertype.JobRow{ID: riverJobID, Attempt: attempt, MaxAttempts: 5}, Args: args}
}

func jobStatus(t *testing.T, jobID string) string {
	t.Helper()
	var s string
	if err := testDB.NewRaw(`SELECT status FROM jobs WHERE id = ?`, jobID).Scan(context.Background(), &s); err != nil {
		t.Fatalf("job status: %v", err)
	}
	return s
}

func itemStatus(t *testing.T, itemID string) (status string, msg *string) {
	t.Helper()
	if err := testDB.NewRaw(`SELECT status, error_message FROM job_items WHERE id = ?`, itemID).Scan(context.Background(), &status, &msg); err != nil {
		t.Fatalf("item status: %v", err)
	}
	return status, msg
}

func countEvents(t *testing.T, typ string) int {
	t.Helper()
	var n int
	if err := testDB.NewRaw(`SELECT count(*) FROM events WHERE type = ?`, typ).Scan(context.Background(), &n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

// fakeOwnerSeq hands out River job ids for items a test inserts directly: the
// lifecycle only compares the recorded owner with the running River job's id,
// so no river_job row is needed.
var fakeOwnerSeq atomic.Int64

func init() { fakeOwnerSeq.Store(1 << 40) }

func nextFakeOwner() int64 { return fakeOwnerSeq.Add(1) }

// ownedJob builds the River job a worker runs an item with, as its recorded
// owner.
func ownedJob[T river.JobArgs](t *testing.T, itemID string, args T) *river.Job[T] {
	t.Helper()
	return riverJob(itemOwner(t, itemID), 1, args)
}

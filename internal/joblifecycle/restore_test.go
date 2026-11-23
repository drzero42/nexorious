package joblifecycle

import (
	"context"
	"testing"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// After a restore every active Job is failed with its unfinished items
// cancelled, with no owner left recorded and no finishing effects queued;
// finished Jobs and their items are left alone.
func TestFailAllAfterRestore(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	user := insertUser(t)

	activeID, its := mustCreateWithItems(t, m, user, 3)
	if err := m.AwaitReview(ctx, its[1].ID, *its[1].RiverJobID, Review{}); err != nil {
		t.Fatal(err)
	}
	finish(t, m, its[2], models.JobItemStatusCompleted)

	dispatched, err := m.CreateWithDispatch(ctx, NewJob{UserID: user, JobType: "fake", Source: "other"})
	if err != nil {
		t.Fatal(err)
	}
	doneID, done := mustCreateWithItems(t, m, insertUser(t), 1)
	finish(t, m, done[0], models.JobItemStatusFailed)
	effectsBefore := countRiver(t, EffectsArgs{}.Kind())

	if err := testDB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return FailAllAfterRestore(ctx, tx, "interrupted by a restore")
	}); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{activeID, dispatched.JobID} {
		j := loadJob(t, id)
		if j.Status != models.JobStatusFailed || j.CompletedAt == nil || j.RiverJobID != nil ||
			j.ErrorMessage == nil || *j.ErrorMessage != "interrupted by a restore" {
			t.Errorf("job %s = %+v, want failed with the restore message and no owner", id, j)
		}
	}
	want := []string{models.JobItemStatusCancelled, models.JobItemStatusCancelled, models.JobItemStatusCompleted}
	for i, it := range loadItems(t, activeID) {
		if it.Status != want[i] || it.ProcessedAt == nil || it.RiverJobID != nil {
			t.Errorf("item %d = %s (processed_at %v, owner %v), want %s, finished, no owner", i, it.Status, it.ProcessedAt, it.RiverJobID, want[i])
		}
	}
	if j := loadJob(t, doneID); j.Status != models.JobStatusCompleted {
		t.Errorf("finished job changed: %s", j.Status)
	}
	if c := countRiver(t, EffectsArgs{}.Kind()); c != effectsBefore {
		t.Errorf("restore queued finishing effects: %d, want %d", c, effectsBefore)
	}
}

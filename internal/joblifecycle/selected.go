package joblifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// Selector runs inside the module's transaction, after the Job row is locked
// and found active. It does the caller's own writes and returns the items the
// operation acts on.
type Selector func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error)

// SkipItems is Skip of a game (sync): the items sel returns that are pending
// or in Pending review in this Job become skipped, an owner is cancelled, and
// completion is evaluated. It returns how many items it skipped; a Job that
// is not active is refused (ErrJobNotActive) before sel runs.
func (m *Module) SkipItems(ctx context.Context, jobID string, sel Selector) (int, error) {
	return m.endSelected(ctx, "skip_items", jobID, models.JobItemStatusSkipped, sel)
}

// CompleteItems completes the items sel returns that are pending or in
// Pending review in this Job, after sel did the library writes: an owner is
// cancelled, and completion is evaluated. Sync rematch uses it, and so does
// sync's settle (a confident match completes the same-title games waiting in
// Pending review). It returns how many it completed; a Job that is not active
// is refused (ErrJobNotActive) before sel runs.
func (m *Module) CompleteItems(ctx context.Context, jobID string, sel Selector) (int, error) {
	return m.endSelected(ctx, "complete_items", jobID, models.JobItemStatusCompleted, sel)
}

func (m *Module) endSelected(ctx context.Context, op, jobID, status string, sel Selector) (int, error) {
	var n int
	err := m.run(ctx, op, func(ctx context.Context, tx bun.Tx, st *txState) error {
		n = 0
		job, err := lockJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		k, err := m.kindOf(job)
		if err != nil {
			return err
		}
		if !job.IsActive() {
			return ErrJobNotActive
		}
		ids, err := sel(ctx, tx, job)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		var items []models.JobItem
		if err := tx.NewSelect().Model(&items).
			Where("job_id = ? AND id IN (?) AND status IN ('pending', 'pending_review')", job.ID, bun.List(ids)).
			Order("created_at", "id").
			Scan(ctx); err != nil {
			return fmt.Errorf("load items: %w", err)
		}
		if len(items) == 0 {
			return nil
		}
		done := make([]string, len(items))
		for i, it := range items {
			done[i] = it.ID
			if it.RiverJobID != nil {
				if err := m.cancelRiverJob(ctx, tx, *it.RiverJobID); err != nil {
					return err
				}
			}
		}
		if _, err := tx.NewRaw(
			`UPDATE job_items SET status = ?, processed_at = ?, river_job_id = NULL
			 WHERE id IN (?) AND status IN ('pending', 'pending_review')`,
			status, time.Now().UTC(), bun.List(done),
		).Exec(ctx); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		if k.Observer != nil {
			jobCopy := *job
			for range done {
				st.onCommit(func(ctx context.Context) { k.Observer.ItemEnded(ctx, jobCopy, status) })
			}
		}
		n = len(done)
		return m.evaluate(ctx, tx, st, k, job, true)
	})
	return n, err
}

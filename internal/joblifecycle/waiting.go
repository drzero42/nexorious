package joblifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// This file holds the rules for items that wait on another item
// (docs/job-lifecycle.md, "Items that wait on another item"). Only sync has
// such items: a Child game's item waits on its parent's. The module knows
// nothing about games; the kind's WaitsOn hook says which item waits on which,
// and the module applies every status and owner condition itself.

// WaitQuery narrows what a kind's WaitsOn hook returns. JobID is always set.
// Items keeps only those waiter items; Awaited keeps only edges whose awaited
// item is one of these. Empty means no narrowing.
type WaitQuery struct {
	JobID   string
	Items   []string
	Awaited []string
}

// WaitEdge says that ItemID waits on AwaitedItemID, an item of the same Job,
// or on an item that does not exist (nil). The hook returns an edge for every
// item, whatever its status, that may wait on another.
type WaitEdge struct {
	ItemID        string  `bun:"item_id"`
	AwaitedItemID *string `bun:"awaited_item_id"`
}

// waitable reports which of itemIDs may wait now: the kind names an item each
// waits on, and that item is unfinished in the Job, or it does not exist and
// the dispatch is not done. The Job row is locked.
func (m *Module) waitable(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job, itemIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if k.WaitsOn == nil || len(itemIDs) == 0 {
		return out, nil
	}
	edges, err := k.WaitsOn(ctx, tx, WaitQuery{JobID: job.ID, Items: itemIDs})
	if err != nil {
		return nil, fmt.Errorf("waits on: %w", err)
	}
	var awaited []string
	for _, e := range edges {
		if e.AwaitedItemID != nil {
			awaited = append(awaited, *e.AwaitedItemID)
		}
	}
	status := map[string]string{}
	if len(awaited) > 0 {
		var rows []struct {
			ID     string `bun:"id"`
			Status string `bun:"status"`
		}
		if err := tx.NewRaw(`SELECT id, status FROM job_items WHERE job_id = ? AND id IN (?)`, job.ID, bun.List(awaited)).Scan(ctx, &rows); err != nil {
			return nil, fmt.Errorf("read awaited items: %w", err)
		}
		for _, r := range rows {
			status[r.ID] = r.Status
		}
	}
	for _, e := range edges {
		if e.AwaitedItemID == nil {
			out[e.ItemID] = !job.DispatchComplete
			continue
		}
		if *e.AwaitedItemID == e.ItemID {
			continue
		}
		switch status[*e.AwaitedItemID] {
		case models.JobItemStatusPending, models.JobItemStatusPendingReview, models.JobItemStatusProcessing:
			out[e.ItemID] = true
		}
	}
	return out, nil
}

// Wait is asked by an item's owner that wants to wait on another item. When
// waiting is allowed the owner is cleared and completion evaluated, and Wait
// reports true; otherwise nothing changes and it reports false, and the
// caller decides at once.
func (m *Module) Wait(ctx context.Context, itemID string, riverJobID int64) (bool, error) {
	var waiting bool
	err := m.run(ctx, "wait", func(ctx context.Context, tx bun.Tx, st *txState) error {
		waiting = false
		job, k, item, err := m.lockItem(ctx, tx, itemID, riverJobID)
		if err != nil {
			return err
		}
		ok, err := m.waitable(ctx, tx, k, job, []string{item.ID})
		if err != nil {
			return err
		}
		if !ok[item.ID] {
			return nil
		}
		if _, err := tx.NewRaw(
			`UPDATE job_items SET river_job_id = NULL WHERE id = ? AND status = 'pending'`, item.ID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("wait: %w", err)
		}
		waiting = true
		return m.evaluate(ctx, tx, st, k, job, false)
	})
	return waiting, err
}

// releaseWaiters queues the pending items without an owner that wait on one
// of finished, items that have just finished in this transaction.
func (m *Module) releaseWaiters(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job, finished []string) error {
	if k.WaitsOn == nil || len(finished) == 0 {
		return nil
	}
	edges, err := k.WaitsOn(ctx, tx, WaitQuery{JobID: job.ID, Awaited: finished})
	if err != nil {
		return fmt.Errorf("waits on: %w", err)
	}
	ids := make([]string, 0, len(edges))
	for _, e := range edges {
		ids = append(ids, e.ItemID)
	}
	return m.queueOwnerless(ctx, tx, k, job, ids)
}

// queueOwnerless gives a River job to each of ids that is a pending item of
// the Job without an owner.
func (m *Module) queueOwnerless(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var items []models.JobItem
	if err := tx.NewSelect().Model(&items).
		Where("job_id = ? AND id IN (?) AND status = 'pending' AND river_job_id IS NULL", job.ID, bun.List(ids)).
		Order("created_at", "id").
		Scan(ctx); err != nil {
		return fmt.Errorf("load waiting items: %w", err)
	}
	for i := range items {
		if err := m.queueItem(ctx, tx, k, job, &items[i]); err != nil {
			return err
		}
	}
	return nil
}

// queueUnwaitable queues every pending item of the Job without an owner that
// may not wait (any more). Used when the dispatch is marked done.
func (m *Module) queueUnwaitable(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job) error {
	var ids []string
	if err := tx.NewRaw(
		`SELECT id FROM job_items WHERE job_id = ? AND status = 'pending' AND river_job_id IS NULL`, job.ID,
	).Scan(ctx, &ids); err != nil {
		return fmt.Errorf("load items without owner: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	ok, err := m.waitable(ctx, tx, k, job, ids)
	if err != nil {
		return err
	}
	queue := ids[:0]
	for _, id := range ids {
		if !ok[id] {
			queue = append(queue, id)
		}
	}
	return m.queueOwnerless(ctx, tx, k, job, queue)
}

// Selector runs inside the module's transaction, after the Job row is locked
// and found active. It does the caller's own writes and returns the items the
// operation acts on.
type Selector func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error)

// SkipItems is Skip of a game (sync): the items sel returns that are pending
// or in Pending review in this Job become skipped, an owner is cancelled, the
// items that waited on them are queued, and completion is evaluated. It
// returns how many items it skipped; a Job that is not active is refused
// (ErrJobNotActive) before sel runs.
func (m *Module) SkipItems(ctx context.Context, jobID string, sel Selector) (int, error) {
	return m.finishSelected(ctx, "skip_items", jobID, models.JobItemStatusSkipped, sel)
}

// CompleteByRematch completes the items sel returns that are pending or in
// Pending review in this Job, after sel did the library writes: an owner is
// cancelled, the items that waited on them are queued, and completion is
// evaluated. It returns how many it completed; a Job that is not active is
// refused (ErrJobNotActive) before sel runs.
func (m *Module) CompleteByRematch(ctx context.Context, jobID string, sel Selector) (int, error) {
	return m.finishSelected(ctx, "complete_by_rematch", jobID, models.JobItemStatusCompleted, sel)
}

func (m *Module) finishSelected(ctx context.Context, op, jobID, status string, sel Selector) (int, error) {
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
				st.onCommit(func(ctx context.Context) { k.Observer.ItemFinished(ctx, jobCopy, status) })
			}
		}
		if err := m.releaseWaiters(ctx, tx, k, job, done); err != nil {
			return err
		}
		n = len(done)
		return m.evaluate(ctx, tx, st, k, job, true)
	})
	return n, err
}

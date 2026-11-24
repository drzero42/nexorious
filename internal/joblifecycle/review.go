package joblifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// Stage is how an item's owner hands it to its next stage.
type Stage struct {
	// Args runs the next stage; the River job inserted for it becomes the
	// item's owner.
	Args river.JobArgs
	// ResolvedIGDBID and MatchConfidence are stored on the item when set.
	ResolvedIGDBID  *int
	MatchConfidence *float64
	Own             OwnWrite
}

// NextStage hands an item its caller owns to its next stage (pending →
// pending): the caller's own writes and the item fields are written, and the
// River job for the next stage is inserted and recorded as the new owner.
func (m *Module) NextStage(ctx context.Context, itemID string, riverJobID int64, s Stage) error {
	if s.Args == nil {
		return errors.New("joblifecycle: NextStage without args")
	}
	return m.run(ctx, "next_stage", func(ctx context.Context, tx bun.Tx, _ *txState) error {
		_, _, item, err := m.lockItem(ctx, tx, itemID, riverJobID)
		if err != nil {
			return err
		}
		if s.Own != nil {
			if err := s.Own(ctx, tx); err != nil {
				return err
			}
		}
		rid, err := m.insertOne(ctx, tx, s.Args)
		if err != nil {
			return err
		}
		q := tx.NewUpdate().Table("job_items").
			Set("river_job_id = ?", rid).
			Where("id = ? AND status = 'pending'", item.ID)
		if s.ResolvedIGDBID != nil {
			q = q.Set("resolved_igdb_id = ?", *s.ResolvedIGDBID)
		}
		if s.MatchConfidence != nil {
			q = q.Set("match_confidence = ?", *s.MatchConfidence)
		}
		if _, err := q.Exec(ctx); err != nil {
			return fmt.Errorf("next stage: %w", err)
		}
		return nil
	})
}

// Review is what an item's owner stores when it sends the item to Pending
// review.
type Review struct {
	Candidates      json.RawMessage // nil keeps what the item has
	MatchConfidence *float64
	// Divert, when set, runs first in the transaction. When it returns args,
	// the item is handed to that next stage instead of Pending review (the
	// work was done meanwhile, for instance a sync game matched by a rematch).
	Divert func(ctx context.Context, tx bun.Tx) (river.JobArgs, error)
}

// AwaitReview sends an item its caller owns to Pending review (pending →
// pending_review): candidates and confidence are stored, the owner cleared and
// completion evaluated.
func (m *Module) AwaitReview(ctx context.Context, itemID string, riverJobID int64, r Review) error {
	return m.run(ctx, "await_review", func(ctx context.Context, tx bun.Tx, st *txState) error {
		job, k, item, err := m.lockItem(ctx, tx, itemID, riverJobID)
		if err != nil {
			return err
		}
		if r.Divert != nil {
			args, err := r.Divert(ctx, tx)
			if err != nil {
				return err
			}
			if args != nil {
				rid, err := m.insertOne(ctx, tx, args)
				if err != nil {
					return err
				}
				if _, err := tx.NewRaw(
					`UPDATE job_items SET river_job_id = ? WHERE id = ? AND status = 'pending'`, rid, item.ID,
				).Exec(ctx); err != nil {
					return fmt.Errorf("next stage: %w", err)
				}
				return nil
			}
		}
		q := tx.NewUpdate().Table("job_items").
			Set("status = 'pending_review'").
			Set("river_job_id = NULL").
			Set("match_confidence = ?", r.MatchConfidence).
			Where("id = ? AND status = 'pending'", item.ID)
		if r.Candidates != nil {
			q = q.Set("igdb_candidates = ?", r.Candidates)
		}
		if _, err := q.Exec(ctx); err != nil {
			return fmt.Errorf("await review: %w", err)
		}
		return m.evaluate(ctx, tx, st, k, job, false)
	})
}

// lockReviewItem locks the Job of an item in Pending review and returns both.
// It refuses an item whose Job is not active (ErrJobNotActive) and one that is
// not in Pending review (ErrNotInReview). Authorisation is the caller's.
func (m *Module) lockReviewItem(ctx context.Context, tx bun.Tx, itemID string) (*models.Job, *Kind, *models.JobItem, error) {
	var jobID string
	err := tx.NewRaw(`SELECT job_id FROM job_items WHERE id = ?`, itemID).Scan(ctx, &jobID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, ErrGone
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load item: %w", err)
	}
	job, err := lockJob(ctx, tx, jobID)
	if err != nil {
		return nil, nil, nil, err
	}
	k, err := m.kindOf(job)
	if err != nil {
		return nil, nil, nil, err
	}
	var item models.JobItem
	if err := tx.NewSelect().Model(&item).Where("id = ?", itemID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, ErrGone
		}
		return nil, nil, nil, fmt.Errorf("load item: %w", err)
	}
	if !job.IsActive() {
		return nil, nil, nil, ErrJobNotActive
	}
	if item.Status != models.JobItemStatusPendingReview {
		return nil, nil, nil, ErrNotInReview
	}
	return job, k, &item, nil
}

// Resolve records the IGDB game the user chose for an item in Pending review
// and hands it to the kind's next step (pending_review → pending). The kind's
// ItemArgs decides that step from the item, which now has resolved_igdb_id.
func (m *Module) Resolve(ctx context.Context, itemID string, igdbID int) error {
	return m.run(ctx, "resolve", func(ctx context.Context, tx bun.Tx, _ *txState) error {
		job, k, item, err := m.lockReviewItem(ctx, tx, itemID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if _, err := tx.NewRaw(
			`UPDATE job_items SET status = 'pending', resolved_igdb_id = ?, resolved_at = ?
			 WHERE id = ? AND status = 'pending_review'`, igdbID, now, item.ID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("resolve: %w", err)
		}
		item.Status, item.ResolvedIGDBID, item.ResolvedAt = models.JobItemStatusPending, &igdbID, &now
		return m.queueItem(ctx, tx, k, job, item)
	})
}

// Skip ends an item in Pending review as skipped and evaluates completion.
func (m *Module) Skip(ctx context.Context, itemID string) error {
	return m.run(ctx, "skip", func(ctx context.Context, tx bun.Tx, st *txState) error {
		job, k, item, err := m.lockReviewItem(ctx, tx, itemID)
		if err != nil {
			return err
		}
		if _, err := tx.NewRaw(
			`UPDATE job_items SET status = 'skipped', processed_at = ?
			 WHERE id = ? AND status = 'pending_review'`, time.Now().UTC(), item.ID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("skip: %w", err)
		}
		if k.Observer != nil {
			jobCopy := *job
			st.onCommit(func(ctx context.Context) { k.Observer.ItemEnded(ctx, jobCopy, models.JobItemStatusSkipped) })
		}
		if err := m.releaseWaiters(ctx, tx, k, job, []string{item.ID}); err != nil {
			return err
		}
		return m.evaluate(ctx, tx, st, k, job, true)
	})
}

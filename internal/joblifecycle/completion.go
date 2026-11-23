package joblifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
)

// evaluate applies the completion rule (docs/job-lifecycle.md, "Completion")
// to a locked, active Job whose dispatch is done. userAction is true for
// Resolve, Skip, Retry and rematch, which do not queue the awaiting-review
// effect again.
func (m *Module) evaluate(ctx context.Context, tx bun.Tx, st *txState, k *Kind, job *models.Job, userAction bool) error {
	if !job.DispatchComplete || job.Status != models.JobStatusProcessing {
		return nil
	}
	var c struct {
		Owned   int `bun:"owned"`
		Pending int `bun:"pending"`
		Review  int `bun:"review"`
	}
	if err := tx.NewRaw(
		`SELECT count(*) FILTER (WHERE status = 'pending' AND river_job_id IS NOT NULL) AS owned,
		        count(*) FILTER (WHERE status IN ('pending', 'processing')) AS pending,
		        count(*) FILTER (WHERE status = 'pending_review') AS review
		 FROM job_items WHERE job_id = ?`, job.ID,
	).Scan(ctx, &c); err != nil {
		return fmt.Errorf("evaluate completion: %w", err)
	}
	switch {
	case c.Owned > 0:
		return nil
	case c.Review > 0:
		if !userAction && k.hasEffects(OutcomeAwaitingReview) {
			return m.queueEffects(ctx, tx, job.ID, OutcomeAwaitingReview, false, nil)
		}
		return nil
	case c.Pending > 0:
		// Only pending items without an owner and nothing in review: cannot
		// arise for a kind whose items do not wait. Recovery queues them.
		return nil
	}
	first := job.CompletedAt == nil
	now := time.Now().UTC()
	if _, err := tx.NewRaw(
		`UPDATE jobs SET status = 'completed', completed_at = ? WHERE id = ? AND status = 'processing'`,
		now, job.ID,
	).Exec(ctx); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	job.Status, job.CompletedAt = models.JobStatusCompleted, &now
	if k.hasEffects(OutcomeCompleted) {
		if err := m.queueEffects(ctx, tx, job.ID, OutcomeCompleted, first, nil); err != nil {
			return err
		}
	}
	if first {
		return m.observeJob(ctx, tx, st, k, job, models.JobStatusCompleted)
	}
	return nil
}

// observeJob queues the observer's per-Job call for after the commit.
func (m *Module) observeJob(ctx context.Context, tx bun.Tx, st *txState, k *Kind, job *models.Job, outcome string) error {
	if k.Observer == nil {
		return nil
	}
	var failed int
	if err := tx.NewRaw(`SELECT count(*) FROM job_items WHERE job_id = ? AND status = 'failed'`, job.ID).Scan(ctx, &failed); err != nil {
		return fmt.Errorf("count failed items: %w", err)
	}
	jobCopy := *job
	st.onCommit(func(ctx context.Context) { k.Observer.JobFinished(ctx, jobCopy, outcome, failed) })
	return nil
}

// EffectsArgs is the River job that runs a Job's finishing effects.
type EffectsArgs struct {
	JobID       string          `json:"job_id"`
	Outcome     Outcome         `json:"outcome"`
	FirstFinish bool            `json:"first_finish"`
	KindData    json.RawMessage `json:"kind_data,omitempty"`
}

func (EffectsArgs) Kind() string { return "job_finishing_effects" }

// InsertOpts: several attempts, so a failing effect is retried; priority 1, so
// it waits only behind sync work queued earlier.
func (EffectsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 1}
}

func (m *Module) queueEffects(ctx context.Context, tx bun.Tx, jobID string, o Outcome, first bool, kindData json.RawMessage) error {
	_, err := m.insertOne(ctx, tx, EffectsArgs{JobID: jobID, Outcome: o, FirstFinish: first, KindData: kindData})
	return err
}

// EffectsWorker runs finishing effects.
type EffectsWorker struct {
	river.WorkerDefaults[EffectsArgs]
	Module *Module
}

// Timeout bounds the effects; they are a handful of writes and notifications.
func (w *EffectsWorker) Timeout(*river.Job[EffectsArgs]) time.Duration { return 5 * time.Minute }

func (w *EffectsWorker) Work(ctx context.Context, rj *river.Job[EffectsArgs]) error {
	m := w.Module
	ctx = logging.WithJobID(ctx, rj.Args.JobID)
	var job models.Job
	if err := m.db.NewSelect().Model(&job).Where("id = ?", rj.Args.JobID).Scan(ctx); err != nil {
		if isNoRows(err) {
			return nil
		}
		return fmt.Errorf("load job: %w", err)
	}
	k := m.kindFor(job.JobType, job.Source)
	if k == nil || k.Effects == nil {
		return nil
	}
	if rj.Args.Outcome == OutcomeAwaitingReview && !job.IsActive() {
		return nil
	}
	err := k.Effects(ctx, m.db, Effect{Job: &job, Outcome: rj.Args.Outcome, FirstFinish: rj.Args.FirstFinish, KindData: rj.Args.KindData})
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if rj.Attempt >= rj.MaxAttempts {
		slog.ErrorContext(ctx, "job lifecycle: finishing effects failed", logging.KeyOutcome, string(rj.Args.Outcome), logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	return err
}

package joblifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
)

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// Generic messages for work recovery fails: the real error, when there was
// one, is in the River job's errors.
const (
	msgItemGaveUp     = "the worker gave up after its last attempt"
	msgItemNoResult   = "the worker stopped without a result"
	msgDispatchGaveUp = "interrupted: the dispatch gave up after its last attempt"
	msgDispatchNoDone = "interrupted: the dispatch stopped without finishing"
	msgDispatchLost   = "interrupted; start it again"
)

// RecoveryResult counts what one recovery run did.
type RecoveryResult struct {
	Failed   int // items and Jobs failed
	Requeued int // River jobs inserted again
}

// removedRiverKinds are periodic River kinds of older versions that no
// longer have a worker (the orphan rescue and the stale sweep, which the
// recovery rule replaced). A row queued before the upgrade would sit in the
// queue for ever; Settle deletes them.
var removedRiverKinds = []string{"rescue_orphaned_pending_items", "cleanup_stale_jobs"}

// Settle moves River jobs a dead process left running, of every River kind
// the lifecycle uses, to a state River acts on: cancelled if they had been
// signalled to stop, retryable if they have attempts left, discarded if not.
// It also deletes River jobs of removed periodic kinds. Run it at startup
// only, before River starts: then nothing is running.
func (m *Module) Settle(ctx context.Context) (int, error) {
	if _, err := m.db.NewRaw(`DELETE FROM river_job WHERE kind IN (?)`, bun.List(removedRiverKinds)).Exec(ctx); err != nil {
		return 0, fmt.Errorf("delete river jobs of removed kinds: %w", err)
	}
	res, err := m.db.NewRaw(`
		UPDATE river_job SET
		  state = CASE
		    WHEN metadata->>'cancel_attempted_at' IS NOT NULL THEN 'cancelled'::river_job_state
		    WHEN attempt < max_attempts THEN 'retryable'::river_job_state
		    ELSE 'discarded'::river_job_state END,
		  finalized_at = CASE
		    WHEN metadata->>'cancel_attempted_at' IS NOT NULL OR attempt >= max_attempts THEN now()
		    ELSE NULL END,
		  scheduled_at = now()
		WHERE state = 'running' AND kind IN (?)`,
		bun.List(m.RiverKinds()),
	).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("settle running river jobs: %w", err)
	}
	n, _ := res.RowsAffected() //nolint:errcheck // advisory count
	return int(n), nil
}

// Recover applies the recovery rule (docs/job-lifecycle.md, "Recovery") to
// every active Job of a kind on the module, one Job per transaction. It has no
// time thresholds and never re-runs work that ended by itself.
func (m *Module) Recover(ctx context.Context) (RecoveryResult, error) {
	var total RecoveryResult
	types := m.JobTypes()
	if len(types) == 0 {
		return total, nil
	}
	var jobs []struct {
		ID      string `bun:"id"`
		JobType string `bun:"job_type"`
		Source  string `bun:"source"`
	}
	if err := m.db.NewRaw(
		`SELECT id, job_type, source FROM jobs WHERE status IN ('pending', 'processing') AND job_type IN (?) ORDER BY created_at`,
		bun.List(types),
	).Scan(ctx, &jobs); err != nil {
		return total, fmt.Errorf("list active jobs: %w", err)
	}
	var firstErr error
	for _, j := range jobs {
		if !m.Handles(j.JobType, j.Source) {
			continue
		}
		var r RecoveryResult
		err := m.run(ctx, "recover", func(ctx context.Context, tx bun.Tx, st *txState) error {
			r = RecoveryResult{}
			return m.recoverJob(ctx, tx, st, j.ID, &r)
		})
		if err != nil && !IsRefusal(err) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		total.Failed += r.Failed
		total.Requeued += r.Requeued
	}
	return total, firstErr
}

func (m *Module) recoverJob(ctx context.Context, tx bun.Tx, st *txState, jobID string, r *RecoveryResult) error {
	job, err := lockJob(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if !job.IsActive() {
		return nil
	}
	k, err := m.kindOf(job)
	if err != nil {
		return err
	}
	ctx = logging.WithJobID(ctx, job.ID)

	// An unfinished dispatch first: when it decides the Job fails, its items
	// are cancelled with it and need nothing queued.
	if !job.DispatchComplete {
		failMsg, err := m.recoverDispatch(ctx, tx, k, job, r)
		if err != nil {
			return err
		}
		if failMsg != "" {
			slog.WarnContext(ctx, "job recovery: failed interrupted work", logging.KeyJobType, job.JobType, "reason", failMsg)
			r.Failed++
			return m.fail(ctx, tx, st, k, job, failMsg, nil, 0)
		}
	}

	var items []models.JobItem
	if err := tx.NewSelect().Model(&items).Where("job_id = ? AND status = 'pending'", job.ID).Order("created_at", "id").Scan(ctx); err != nil {
		return fmt.Errorf("load pending items: %w", err)
	}
	owners := make([]int64, 0, len(items))
	for _, it := range items {
		if it.RiverJobID != nil {
			owners = append(owners, *it.RiverJobID)
		}
	}
	states, err := riverStates(ctx, tx, owners)
	if err != nil {
		return err
	}
	var ownerless []string
	for _, it := range items {
		if it.RiverJobID == nil {
			ownerless = append(ownerless, it.ID)
		}
	}
	mayWait, err := m.waitable(ctx, tx, k, job, ownerless)
	if err != nil {
		return err
	}

	for i := range items {
		it := &items[i]
		if it.RiverJobID == nil {
			if mayWait[it.ID] {
				continue // waiting on another item, which is allowed (invariant 3)
			}
			rid, err := findLive(ctx, tx, k.ItemRiverKinds, "job_item_id", it.ID)
			if err != nil {
				return err
			}
			if rid != 0 {
				if _, err := tx.NewRaw(`UPDATE job_items SET river_job_id = ? WHERE id = ?`, rid, it.ID).Exec(ctx); err != nil {
					return fmt.Errorf("adopt river job: %w", err)
				}
				continue
			}
			if err := m.queueItem(ctx, tx, k, job, it); err != nil {
				return err
			}
			r.Requeued++
			continue
		}
		switch states[*it.RiverJobID] {
		case riverAvailable, riverScheduled, riverRetryable, riverPending, riverRunning:
			continue
		case riverDiscarded:
			if err := m.recoveryFailItem(ctx, tx, st, k, job, it, msgItemGaveUp); err != nil {
				return err
			}
			r.Failed++
		case riverCompleted:
			slog.ErrorContext(ctx, "job lifecycle: worker returned without a result", logging.KeyJobItemID, it.ID, logging.KeyRiverJobID, *it.RiverJobID, logging.Cat(logging.CategoryValidation))
			if err := m.recoveryFailItem(ctx, tx, st, k, job, it, msgItemNoResult); err != nil {
				return err
			}
			r.Failed++
		default: // cancelled, or the row is gone
			if err := m.queueItem(ctx, tx, k, job, it); err != nil {
				return err
			}
			r.Requeued++
		}
	}
	return m.evaluate(ctx, tx, st, k, job, false)
}

// recoverDispatch applies the recovery rule to a Job whose dispatch is not
// done: it adopts a live dispatch River job, leaves a live owner alone, or
// inserts the dispatch again. It returns the message to fail the Job with
// when the dispatch ended without finishing or cannot be run again, else "".
func (m *Module) recoverDispatch(ctx context.Context, tx bun.Tx, k *Kind, job *models.Job, r *RecoveryResult) (string, error) {
	if job.RiverJobID == nil {
		rid, err := findLive(ctx, tx, k.DispatchRiverKinds, "job_id", job.ID)
		if err != nil {
			return "", err
		}
		if rid != 0 {
			if _, err := tx.NewRaw(`UPDATE jobs SET river_job_id = ? WHERE id = ?`, rid, job.ID).Exec(ctx); err != nil {
				return "", fmt.Errorf("adopt dispatch: %w", err)
			}
			return "", nil
		}
	}
	state := ""
	if job.RiverJobID != nil {
		states, err := riverStates(ctx, tx, []int64{*job.RiverJobID})
		if err != nil {
			return "", err
		}
		state = states[*job.RiverJobID]
	}
	switch state {
	case riverAvailable, riverScheduled, riverRetryable, riverPending, riverRunning:
		return "", nil
	case riverDiscarded:
		return msgDispatchGaveUp, nil
	case riverCompleted:
		slog.ErrorContext(ctx, "job lifecycle: dispatch returned without marking it done", logging.KeyRiverJobID, *job.RiverJobID, logging.Cat(logging.CategoryValidation))
		return msgDispatchNoDone, nil
	}
	// Cancelled, gone, or never recorded.
	if k.DispatchArgs == nil {
		if k.DispatchLostMessage != "" {
			return k.DispatchLostMessage, nil
		}
		return msgDispatchLost, nil
	}
	args, err := k.DispatchArgs(job)
	if err != nil {
		return "", fmt.Errorf("dispatch args: %w", err)
	}
	rid, err := m.insertOne(ctx, tx, args)
	if err != nil {
		return "", err
	}
	if _, err := tx.NewRaw(`UPDATE jobs SET river_job_id = ? WHERE id = ?`, rid, job.ID).Exec(ctx); err != nil {
		return "", fmt.Errorf("record dispatch owner: %w", err)
	}
	job.RiverJobID = &rid
	r.Requeued++
	return "", nil
}

func (m *Module) recoveryFailItem(ctx context.Context, tx bun.Tx, st *txState, k *Kind, job *models.Job, it *models.JobItem, msg string) error {
	slog.WarnContext(ctx, "job recovery: failed interrupted work", logging.KeyJobType, job.JobType, logging.KeyJobItemID, it.ID, "reason", msg)
	return m.finishItem(ctx, tx, st, k, job, it, models.JobItemStatusFailed, msg, nil)
}

// RecoveryArgs is the periodic River job that runs the recovery rule.
type RecoveryArgs struct{}

func (RecoveryArgs) Kind() string { return "job_lifecycle_recovery" }

func (RecoveryArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

// RecoveryWorker runs the rule periodically. When it fails or re-queues
// something it emits one admin maintenance event.
type RecoveryWorker struct {
	river.WorkerDefaults[RecoveryArgs]
	Module *Module
}

func (w *RecoveryWorker) Timeout(*river.Job[RecoveryArgs]) time.Duration { return 10 * time.Minute }

func (w *RecoveryWorker) Work(ctx context.Context, _ *river.Job[RecoveryArgs]) error {
	res, err := w.Module.Recover(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		notify.Emit(ctx, w.Module.db, notify.EmitParams{
			Type: notify.TypeAdminMaintFailed, Scope: notify.ScopeAdmin,
			Payload: notify.MaintPayload{Action: "job_recovery", Error: err.Error(), Rescued: res.Requeued, Failed: res.Failed},
		})
		return nil
	}
	if res.Failed+res.Requeued > 0 {
		notify.Emit(ctx, w.Module.db, notify.EmitParams{
			Type: notify.TypeAdminMaintCompleted, Scope: notify.ScopeAdmin,
			Payload: notify.MaintPayload{Action: "job_recovery", Rescued: res.Requeued, Failed: res.Failed},
		})
	}
	return nil
}

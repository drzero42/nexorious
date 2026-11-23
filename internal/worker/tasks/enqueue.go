package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
)

// ErrNilRiverClient is returned by EnqueueOrFail when called with a nil River
// client. The corresponding job_item is also marked 'failed' so it does not sit
// stranded in 'pending' without a backing River job.
var ErrNilRiverClient = errors.New("nil river client")

// EnqueueOrFail inserts a River job for the given job_item. If the insert
// cannot be performed — because the River client is nil or River returns an
// error — the job_item is moved to status='failed' with a diagnostic
// error_message so it does not get stuck in 'pending' forever.
//
// Why: job_items.status='pending' and the existence of a non-terminal river_job
// row must stay in lockstep. Every silent rc.Insert failure breaks that
// invariant and produces a job_item the workers will never touch again. The
// scheduled RescueOrphanedPendingItems job is a slow safety net (≥30 min,
// ≥1 h item age, parent job must still be 'processing'); EnqueueOrFail closes
// the gap synchronously by failing loudly at the call site instead.
//
// The job_item is marked failed only on Insert failure — on success the caller
// keeps the item in its current state (typically 'pending').
func EnqueueOrFail(
	ctx context.Context,
	db *bun.DB,
	rc *river.Client[pgx.Tx],
	jobItemID string,
	args river.JobArgs,
) error {
	if rc == nil {
		markEnqueueFailed(ctx, db, jobItemID, "river client unavailable")
		return ErrNilRiverClient
	}
	if _, err := rc.Insert(ctx, args, nil); err != nil {
		markEnqueueFailed(ctx, db, jobItemID, fmt.Sprintf("river enqueue failed: %v", err))
		return fmt.Errorf("river insert %s: %w", args.Kind(), err)
	}
	return nil
}

// ArgsForJobType returns the River JobArgs that run a job_item of the given
// job_type, for the kinds not yet on internal/joblifecycle (a kind on the
// module supplies its own). Only sync is left; used by the HTTP retry handlers
// and the orphan rescuer.
func ArgsForJobType(jobType, jobItemID string) (river.JobArgs, error) {
	if jobType == models.JobTypeSync {
		return IGDBMatchArgs{JobItemID: jobItemID}, nil
	}
	return nil, fmt.Errorf("unknown job_type %q", jobType)
}

func markEnqueueFailed(ctx context.Context, db *bun.DB, jobItemID, msg string) {
	msg = logging.ScrubURLQueries(msg) // never persist URL query credentials (#937)
	now := time.Now().UTC()
	if _, err := db.NewRaw(
		`UPDATE job_items SET status = ?, error_message = ?, processed_at = ?
		 WHERE id = ? AND status = ?`,
		models.JobItemStatusFailed, msg, now, jobItemID, models.JobItemStatusPending,
	).Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "EnqueueOrFail: mark item failed",
			logging.KeyJobItemID, jobItemID, "msg", msg, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
}

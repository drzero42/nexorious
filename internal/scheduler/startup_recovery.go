package scheduler

import (
	"context"
	"log/slog"

	pgx "github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// RecoverInterruptedSyncs resumes sync work the previous process left
// unfinished (#1302). It must run before the River client starts.
//
// It relies on single instance being the only supported deployment: when this
// runs nothing else is working, so every River job still 'running' belongs to
// a dead process and every unfinished sync Job is left over from it. Do not
// call it while workers are running.
//
// In order:
//  1. Settle 'running' River jobs of the sync kinds that have no attempts
//     left (#1304): discard them, fail the Job of a dispatch and fail the item
//     of an item job. They are not retried, so work that kills the process
//     every time cannot loop forever.
//  2. Release the remaining 'running' River jobs of the sync kinds to
//     'retryable' (a hard kill leaves them 'running').
//  3. Re-enqueue the dispatch of every active sync Job whose dispatch did not
//     finish and that has no live dispatch_sync River job (a clean shutdown
//     cancels the dispatch, which River then records as completed).
//  4. Re-queue pending items that have no live River job, whatever their age.
//  5. Run the completion check for sync Jobs whose dispatch finished, so a Job
//     whose last item settled just before the shutdown is finalized.
//
// Every step is best-effort: a failure is logged and the remaining steps
// still run. The hourly orphan rescue and the stale-job sweep remain as the
// safety nets behind it.
func RecoverInterruptedSyncs(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx]) {
	settleExhaustedSyncRiverJobs(ctx, db)
	releaseRunningSyncRiverJobs(ctx, db)
	reenqueueUnfinishedDispatches(ctx, db, rc)
	// No maintenance event: a restart must not produce a notification.
	rescueOrphanedPendingItems(ctx, db, rc, 0, false)
	completeSettledSyncJobs(ctx, db, rc)
}

// syncRiverKinds are the River job kinds that make up a sync.
func syncRiverKinds() []string {
	return []string{
		tasks.DispatchSyncArgs{}.Kind(),
		tasks.IGDBMatchArgs{}.Kind(),
		tasks.UserGameArgs{}.Kind(),
	}
}

// interruptedSyncMessage is the error recorded on a sync Job or item whose
// River job was hard-killed on its last attempt.
const interruptedSyncMessage = "interrupted by repeated server restarts"

// settleExhaustedSyncRiverJobs discards the 'running' sync River jobs that
// have no attempts left and fails what they were working on: the Job for a
// dispatch (pending items cancelled, sync.failed emitted), the item for an
// item job. River's rescuer never touches such a dispatch (it has no timeout),
// so without this the Job blocks new syncs until the stale-job sweep.
//
// If failing the Job errors after its River job is discarded, the next step
// re-enqueues the dispatch with fresh attempts.
func settleExhaustedSyncRiverJobs(ctx context.Context, db *bun.DB) {
	var dead []struct {
		Kind      string `bun:"kind"`
		JobID     string `bun:"job_id"`
		JobItemID string `bun:"job_item_id"`
	}
	if err := db.NewRaw(`
		WITH dead AS (
		  UPDATE river_job
		     SET state = 'discarded',
		         finalized_at = now(),
		         errors = array_append(errors, jsonb_build_object(
		           'at', now(),
		           'error', 'discarded at startup: previous process no longer running and no attempts left'
		         ))
		   WHERE kind IN (?)
		     AND state = 'running'
		     AND attempt >= max_attempts
		  RETURNING kind, args
		)
		SELECT kind,
		       COALESCE(args->>'job_id', '') AS job_id,
		       COALESCE(args->>'job_item_id', '') AS job_item_id
		FROM dead`,
		bun.List(syncRiverKinds()),
	).Scan(ctx, &dead); err != nil {
		slog.ErrorContext(ctx, "startup recovery: discard exhausted sync river jobs failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	for _, d := range dead {
		if d.Kind == (tasks.DispatchSyncArgs{}).Kind() {
			slog.WarnContext(ctx, "startup recovery: failing sync whose dispatch was killed on its last attempt", logging.KeyJobID, d.JobID)
			tasks.FailSyncJob(ctx, db, d.JobID, "sync "+interruptedSyncMessage)
			continue
		}
		slog.WarnContext(ctx, "startup recovery: failing sync item whose worker was killed on its last attempt", logging.KeyJobItemID, d.JobItemID, "kind", d.Kind)
		if _, err := db.NewRaw(
			`UPDATE job_items SET status = 'failed', error_message = ?, processed_at = now()
			 WHERE id = ? AND status = 'pending'`,
			interruptedSyncMessage, d.JobItemID,
		).Exec(ctx); err != nil {
			slog.ErrorContext(ctx, "startup recovery: fail interrupted sync item failed", logging.KeyErr, err, logging.KeyJobItemID, d.JobItemID, logging.Cat(logging.CategoryDB))
		}
	}
}

func releaseRunningSyncRiverJobs(ctx context.Context, db *bun.DB) {
	kinds := syncRiverKinds()
	result, err := db.NewRaw(`
		UPDATE river_job
		   SET state = 'retryable',
		       scheduled_at = now(),
		       errors = array_append(errors, jsonb_build_object(
		         'at', now(),
		         'error', 'rescued at startup: previous process no longer running'
		       ))
		 WHERE kind IN (?)
		   AND state = 'running'
		   AND attempt < max_attempts`,
		bun.List(kinds),
	).Exec(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "startup recovery: release running sync river jobs failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // advisory RowsAffected
	if rows > 0 {
		slog.InfoContext(ctx, "startup recovery: released running sync river jobs", "count", rows)
	}
}

func reenqueueUnfinishedDispatches(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx]) {
	var jobs []struct {
		ID     string `bun:"id"`
		UserID string `bun:"user_id"`
		Source string `bun:"source"`
	}
	if err := db.NewRaw(`
		SELECT j.id, j.user_id, j.source
		FROM jobs j
		WHERE j.job_type = 'sync'
		  AND j.status IN ('pending', 'processing')
		  AND j.dispatch_complete = false
		  AND NOT EXISTS (
		    SELECT 1 FROM river_job rj
		    WHERE rj.kind = ?
		      AND rj.args->>'job_id' = j.id
		      AND rj.state NOT IN ('completed', 'discarded', 'cancelled')
		  )`,
		tasks.DispatchSyncArgs{}.Kind(),
	).Scan(ctx, &jobs); err != nil {
		slog.ErrorContext(ctx, "startup recovery: query unfinished dispatches failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	for _, j := range jobs {
		if _, err := rc.Insert(ctx, tasks.DispatchSyncArgs{JobID: j.ID, UserID: j.UserID, Storefront: j.Source}, nil); err != nil {
			slog.ErrorContext(ctx, "startup recovery: re-enqueue dispatch failed", logging.KeyErr, err, logging.KeyJobID, j.ID, logging.KeySource, j.Source, logging.Cat(logging.CategoryDB))
			continue
		}
		slog.InfoContext(ctx, "startup recovery: re-enqueued interrupted sync dispatch", logging.KeyJobID, j.ID, logging.KeySource, j.Source)
	}
}

func completeSettledSyncJobs(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx]) {
	var jobIDs []string
	if err := db.NewRaw(`
		SELECT id FROM jobs
		WHERE job_type = 'sync' AND status = 'processing' AND dispatch_complete = true`,
	).Scan(ctx, &jobIDs); err != nil {
		slog.ErrorContext(ctx, "startup recovery: query dispatched sync jobs failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	for _, id := range jobIDs {
		tasks.SyncCheckJobCompletion(ctx, db, rc, id)
	}
}

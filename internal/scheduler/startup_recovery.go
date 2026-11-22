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
//  1. Release 'running' River jobs of the sync kinds to 'retryable' (a hard
//     kill leaves them 'running'; a job on its last attempt is left alone,
//     see #1304).
//  2. Re-enqueue the dispatch of every active sync Job whose dispatch did not
//     finish and that has no live dispatch_sync River job (a clean shutdown
//     cancels the dispatch, which River then records as completed).
//  3. Re-queue pending items that have no live River job, whatever their age.
//  4. Run the completion check for sync Jobs whose dispatch finished, so a Job
//     whose last item settled just before the shutdown is finalized.
//
// Every step is best-effort: a failure is logged and the remaining steps
// still run. The hourly orphan rescue and the stale-job sweep remain as the
// safety nets behind it.
func RecoverInterruptedSyncs(ctx context.Context, db *bun.DB, rc *river.Client[pgx.Tx]) {
	releaseRunningSyncRiverJobs(ctx, db)
	reenqueueUnfinishedDispatches(ctx, db, rc)
	// No maintenance event: a restart must not produce a notification.
	rescueOrphanedPendingItems(ctx, db, rc, 0, false)
	completeSettledSyncJobs(ctx, db, rc)
}

func releaseRunningSyncRiverJobs(ctx context.Context, db *bun.DB) {
	kinds := []string{
		tasks.DispatchSyncArgs{}.Kind(),
		tasks.IGDBMatchArgs{}.Kind(),
		tasks.UserGameArgs{}.Kind(),
	}
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

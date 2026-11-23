package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
)

// CleanupStaleJobs marks sync jobs that are stuck with an orphaned dispatch
// (dispatch_complete=false after all River retries are exhausted) and no
// remaining unfinished items as failed. This releases duplicate-run guards and
// unblocks scheduled syncs after a crash. dispatch_complete=true jobs are never
// touched. The kinds on internal/joblifecycle (the two refreshes, export) are
// its recovery rule's, which has no age threshold.
//
// A job is stale when ALL of:
//   - job_type = 'sync' and dispatch_complete = false
//   - status IN ('pending', 'processing')
//   - created_at < now() - threshold
//   - no associated job_items rows are unfinished
//
// Action: UPDATE jobs SET status='failed', error_message='stale_job_cleaned_up'.
func CleanupStaleJobs(ctx context.Context, db *bun.DB, threshold time.Duration) {
	syncResult, err := db.NewRaw(
		`UPDATE jobs
		   SET status = 'failed',
		       error_message = 'stale_job_cleaned_up',
		       completed_at = now()
		 WHERE job_type = 'sync'
		   AND status IN ('pending', 'processing')
		   AND dispatch_complete = false
		   AND created_at < now() - (? || ' seconds')::interval
		   AND NOT EXISTS (
		     SELECT 1 FROM job_items
		      WHERE job_items.job_id = jobs.id
		        AND job_items.status NOT IN ('completed', 'failed', 'skipped', 'cancelled')
		   )`,
		int64(threshold.Seconds()),
	).Exec(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup_stale_jobs: sync cleanup failed", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		emitMaint(ctx, db, true, notify.MaintPayload{Action: "stale_jobs_cleanup", Error: err.Error()})
		return
	}
	syncRows, _ := syncResult.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if syncRows > 0 {
		slog.WarnContext(ctx, "cleanup_stale_jobs: marked stale sync jobs failed", "count", syncRows)
		emitMaint(ctx, db, false, notify.MaintPayload{Action: "stale_jobs_cleanup", Count: int(syncRows)})
	}
}

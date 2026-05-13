package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"
)

// CleanupStaleJobs marks metadata_refresh jobs that are stuck in pending or
// processing with no remaining unfinished items as failed. This releases the
// duplicate-run guard in metadata_refresh_dispatch after a crash during
// dispatch.
//
// A job is stale when ALL of:
//   - job_type = 'metadata_refresh'
//   - status IN ('pending', 'processing')
//   - created_at < now() - threshold
//   - no associated job_items rows are in pending/processing/pending_review
//     (i.e. items are either all terminal or never existed)
//
// Action: UPDATE jobs SET status='failed', error_message='stale_job_cleaned_up'.
func CleanupStaleJobs(ctx context.Context, db *bun.DB, threshold time.Duration) {
	result, err := db.NewRaw(
		`UPDATE jobs
		   SET status = 'failed',
		       error_message = 'stale_job_cleaned_up',
		       completed_at = now()
		 WHERE job_type = 'metadata_refresh'
		   AND status IN ('pending', 'processing')
		   AND created_at < now() - (? || ' seconds')::interval
		   AND NOT EXISTS (
		     SELECT 1 FROM job_items
		      WHERE job_items.job_id = jobs.id
		        AND job_items.status NOT IN ('completed', 'failed', 'skipped')
		   )`,
		int64(threshold.Seconds()),
	).Exec(ctx)
	if err != nil {
		slog.Error("cleanup_stale_jobs: failed", "err", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		slog.Info("cleanup_stale_jobs: marked stale jobs failed", "count", rows)
	}
}

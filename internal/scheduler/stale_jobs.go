package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
)

// maintenanceRefreshJobTypes are the system maintenance refresh job types that
// share a lifecycle — a handler or the scheduler creates the jobs row, a dispatch
// worker populates it — and the same stale signature, so CleanupStaleJobs reaps
// them with one rule.
var maintenanceRefreshJobTypes = []string{
	models.JobTypeMetadataRefresh,
	models.JobTypeStoreLinkRefresh,
}

// CleanupStaleJobs marks jobs that are stuck in pending or processing with no
// remaining unfinished items as failed. This releases duplicate-run guards and
// unblocks scheduled syncs after a crash.
//
// A job is stale when ALL of:
//   - job_type is in the handled set (see below)
//   - status IN ('pending', 'processing')
//   - created_at < now() - threshold
//   - no associated job_items rows are in pending/processing/pending_review
//     (i.e. items are either all terminal or never existed)
//
// Handled job types:
//   - metadata_refresh, store_link_refresh: the maintenance refresh types; guards
//     against a stuck dispatch row (incl. one created before any job_items) after a crash
//   - sync: guards against orphaned dispatch (dispatch_complete=false) after
//     all River retries are exhausted; dispatch_complete=true jobs are never touched
//
// Action: UPDATE jobs SET status='failed', error_message='stale_job_cleaned_up'.
func CleanupStaleJobs(ctx context.Context, db *bun.DB, threshold time.Duration) {
	result, err := db.NewRaw(
		`UPDATE jobs
		   SET status = 'failed',
		       error_message = 'stale_job_cleaned_up',
		       completed_at = now()
		 WHERE job_type IN (?)
		   AND status IN ('pending', 'processing')
		   AND created_at < now() - (? || ' seconds')::interval
		   AND NOT EXISTS (
		     SELECT 1 FROM job_items
		      WHERE job_items.job_id = jobs.id
		        AND job_items.status NOT IN ('completed', 'failed', 'skipped')
		   )`,
		bun.List(maintenanceRefreshJobTypes), int64(threshold.Seconds()),
	).Exec(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "cleanup_stale_jobs: failed", logging.Cat(logging.CategoryDB), logging.KeyErr, err)
		emitMaint(ctx, db, true, notify.MaintPayload{Action: "stale_jobs_cleanup", Error: err.Error()})
		return
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows > 0 {
		slog.WarnContext(ctx, "cleanup_stale_jobs: marked stale jobs failed", "count", rows)
	}

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
	}

	if rows+syncRows > 0 {
		emitMaint(ctx, db, false, notify.MaintPayload{Action: "stale_jobs_cleanup", Count: int(rows + syncRows)})
	}
}

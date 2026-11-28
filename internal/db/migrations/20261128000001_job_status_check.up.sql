-- Reject a jobs.status or job_items.status that is not a models.JobStatus or
-- models.JobItemStatus value (#1513). A row with any other status is never
-- recovered, never counted toward completion and never cleaned up, so fail
-- such rows first: older installs may still hold job_items with the status
-- 'igdb_failed' (added in #35, removed in #610 without a data migration).
--
-- A status added to the Go types must be added here too, by a new migration;
-- TestJobStatusCheckMatchesModels fails until it is.

-- A Job the migration fails gets completed_at, an error message, and its open
-- items cancelled (docs/job-lifecycle.md, "Rows from older versions").
WITH failed_jobs AS (
    UPDATE jobs
       SET status = 'failed',
           completed_at = COALESCE(completed_at, now()),
           error_message = COALESCE(error_message, 'unknown status ''' || status || ''', failed by an upgrade')
     WHERE status NOT IN ('pending', 'processing', 'completed', 'failed', 'cancelled')
    RETURNING id
)
UPDATE job_items
   SET status = 'cancelled', processed_at = COALESCE(processed_at, now())
 WHERE job_id IN (SELECT id FROM failed_jobs)
   AND status IN ('pending', 'processing', 'pending_review');

-- An item with an unknown status becomes failed, so it can be retried.
UPDATE job_items
   SET status = 'failed',
       processed_at = COALESCE(processed_at, now()),
       error_message = COALESCE(error_message, 'unknown status ''' || status || ''', failed by an upgrade; retry it')
 WHERE status NOT IN ('pending', 'processing', 'completed', 'pending_review', 'skipped', 'failed', 'cancelled');

ALTER TABLE jobs ADD CONSTRAINT jobs_status_check
    CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled'));

ALTER TABLE job_items ADD CONSTRAINT job_items_status_check
    CHECK (status IN ('pending', 'processing', 'completed', 'pending_review', 'skipped', 'failed', 'cancelled'));

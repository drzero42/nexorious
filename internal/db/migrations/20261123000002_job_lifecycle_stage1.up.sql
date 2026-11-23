-- Job lifecycle stage 1 (#1266): record the River job that owns a pending item
-- or an unfinished dispatch, and put the rows of the three kinds that move onto
-- internal/joblifecycle (metadata_refresh, store_link_refresh, export) into a
-- state its rules understand (docs/job-lifecycle.md, "Rows from older
-- versions"). river_job is not touched: Bun migrations run before River's.

ALTER TABLE job_items ADD COLUMN river_job_id bigint;
ALTER TABLE jobs ADD COLUMN river_job_id bigint;

-- Jobs that never finished their dispatch, whatever dispatch_complete says (it
-- defaults to true): every active export, and an active refresh still pending
-- with no items. Fail the ones more than a day old first...
UPDATE jobs
   SET status = 'failed',
       completed_at = now(),
       error_message = 'interrupted; start it again',
       dispatch_complete = false
 WHERE status IN ('pending', 'processing')
   AND created_at < now() - interval '1 day'
   AND (job_type = 'export'
        OR (job_type IN ('metadata_refresh', 'store_link_refresh')
            AND status = 'pending'
            AND NOT EXISTS (SELECT 1 FROM job_items ji WHERE ji.job_id = jobs.id)));

-- ...and let recovery run the rest.
UPDATE jobs
   SET dispatch_complete = false
 WHERE status IN ('pending', 'processing')
   AND (job_type = 'export'
        OR (job_type IN ('metadata_refresh', 'store_link_refresh')
            AND status = 'pending'
            AND NOT EXISTS (SELECT 1 FROM job_items ji WHERE ji.job_id = jobs.id)));

-- Unfinished items of a failed or cancelled Job are cancelled.
UPDATE job_items ji
   SET status = 'cancelled', processed_at = now()
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.job_type IN ('metadata_refresh', 'store_link_refresh', 'export')
   AND j.status IN ('failed', 'cancelled')
   AND ji.status IN ('pending', 'processing', 'pending_review');

-- Unfinished items of a completed Job become failed, so they can be retried.
UPDATE job_items ji
   SET status = 'failed', processed_at = now(),
       error_message = 'retry interrupted by an upgrade; retry again'
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.job_type IN ('metadata_refresh', 'store_link_refresh', 'export')
   AND j.status = 'completed'
   AND ji.status IN ('pending', 'processing', 'pending_review');

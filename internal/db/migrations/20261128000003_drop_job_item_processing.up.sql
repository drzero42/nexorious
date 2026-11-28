-- Remove the Job item status 'processing' (#1516). Only the old import resolve
-- handler wrote it, and the stage 2 migration (20261123000003) moved every
-- such item, so this should change no row. Any item left is moved by the same
-- rules (docs/job-lifecycle.md, "Rows from older versions").

-- An item of an active Job goes back to a worker. Its River job is kept:
-- recovery adopts it, or refuses it when it runs and queues another.
UPDATE job_items ji
   SET status = 'pending'
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.status IN ('pending', 'processing')
   AND ji.status = 'processing';

-- An item of a completed Job becomes failed, so it can be retried.
UPDATE job_items ji
   SET status = 'failed', processed_at = COALESCE(ji.processed_at, now()), river_job_id = NULL,
       error_message = 'interrupted by an upgrade; retry it'
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.status = 'completed'
   AND ji.status = 'processing';

-- An item of a failed or cancelled Job is cancelled with it.
UPDATE job_items ji
   SET status = 'cancelled', processed_at = COALESCE(ji.processed_at, now()), river_job_id = NULL
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.status IN ('failed', 'cancelled')
   AND ji.status = 'processing';

ALTER TABLE job_items DROP CONSTRAINT job_items_status_check;
ALTER TABLE job_items ADD CONSTRAINT job_items_status_check
    CHECK (status IN ('pending', 'completed', 'pending_review', 'skipped', 'failed', 'cancelled'));

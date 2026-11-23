-- Job lifecycle stage 3 (#1268): put the rows of sync, which moves onto
-- internal/joblifecycle, into a state its rules understand
-- (docs/job-lifecycle.md, "Rows from older versions"). The rules that fail
-- Jobs run before the rules for items. river_job is not touched: Bun
-- migrations run before River's. Every item failed or cancelled here gets
-- processed_at, which the storefront view reads a failed item against.
--
-- Sync has no 'processing' items: only the old import resolve handler wrote
-- that status. Waiting children left by the old code are 'pending' with no
-- recorded owner; recovery leaves one that may still wait and queues the
-- others.

-- 1. A failed sync the old "retry failed" reopened: its dispatch never
--    finished and must not be run now, possibly beside a newer sync.
UPDATE jobs
   SET status = 'failed',
       completed_at = now(),
       error_message = 'sync did not finish; start a new one'
 WHERE job_type = 'sync'
   AND status IN ('pending', 'processing')
   AND NOT dispatch_complete
   AND completed_at IS NOT NULL;

-- 2. A sync created more than a day ago whose dispatch never started.
UPDATE jobs
   SET status = 'failed',
       completed_at = now(),
       error_message = 'sync never started; start a new one'
 WHERE job_type = 'sync'
   AND status = 'pending'
   AND created_at < now() - interval '1 day';

-- 3. Both old creators inserted without dispatch_complete, whose default is
--    true; only the dispatch's first write set it false. Without this the
--    Job could never be dispatched or completed.
UPDATE jobs
   SET dispatch_complete = false
 WHERE job_type = 'sync'
   AND status = 'pending';

-- 4. Unfinished items of a failed or cancelled sync: old Cancel left all of
--    them, the old failure cancelled only pending ones.
UPDATE job_items ji
   SET status = 'cancelled', processed_at = now(), river_job_id = NULL
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.job_type = 'sync'
   AND j.status IN ('failed', 'cancelled')
   AND ji.status IN ('pending', 'pending_review', 'processing');

-- 5. Unfinished items of a completed sync: an old single-item retry or an
--    old rematch still in flight, or an item the old retry sent to review in
--    a finished Job. As failed they can be retried, which reopens the Job.
UPDATE job_items ji
   SET status = 'failed', processed_at = now(), river_job_id = NULL,
       error_message = 'interrupted by an upgrade; retry it'
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.job_type = 'sync'
   AND j.status = 'completed'
   AND ji.status IN ('pending', 'pending_review', 'processing');

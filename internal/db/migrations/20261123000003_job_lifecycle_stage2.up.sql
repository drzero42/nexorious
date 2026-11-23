-- Job lifecycle stage 2 (#1267): put the rows of the two import kinds, which
-- move onto internal/joblifecycle, into a state its rules understand
-- (docs/job-lifecycle.md, "Rows from older versions"). Selects on
-- job_type = 'import' only: export Jobs use the same source values.
-- river_job is not touched: Bun migrations run before River's.
--
-- An active generic import with dispatch_complete = false (an upload handler
-- that died part-way) is left to recovery, which fails it through the module
-- so that import.failed is sent and its River jobs are cancelled.

-- 1. An active nexorious import with no item but the pools item: every record
--    was malformed, and nothing would ever complete it.
UPDATE jobs
   SET status = 'failed',
       completed_at = now(),
       error_message = 'the file had no usable game records'
 WHERE job_type = 'import'
   AND source = 'nexorious'
   AND status IN ('pending', 'processing')
   AND NOT EXISTS (SELECT 1 FROM job_items ji
                    WHERE ji.job_id = jobs.id AND ji.item_key <> '__pools__');

-- 2. Older versions kept an active nexorious import 'pending' until it
--    completed; the module only completes a 'processing' Job. Recovery then
--    completes the ones whose items have all finished.
UPDATE jobs
   SET status = 'processing'
 WHERE job_type = 'import'
   AND source = 'nexorious'
   AND status = 'pending';

-- 3. No import wrote started_at.
UPDATE jobs
   SET started_at = COALESCE(started_at, created_at)
 WHERE job_type = 'import'
   AND status IN ('pending', 'processing');

-- 4. Unfinished items of a failed or cancelled import: old Cancel left them.
UPDATE job_items ji
   SET status = 'cancelled', processed_at = now(), river_job_id = NULL
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.job_type = 'import'
   AND j.status IN ('failed', 'cancelled')
   AND ji.status IN ('pending', 'pending_review', 'processing');

-- 5. Unfinished items of a completed import: the old single-item retry and
--    resolve worked inside a completed Job. As failed they can be retried,
--    which reopens the Job; a resolved one then goes straight to finalize.
UPDATE job_items ji
   SET status = 'failed', processed_at = now(), river_job_id = NULL,
       error_message = 'interrupted by an upgrade; retry it'
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.job_type = 'import'
   AND j.status = 'completed'
   AND ji.status IN ('pending', 'pending_review', 'processing');

-- 6. Only the old resolve handler wrote 'processing', and an item whose
--    enqueue then failed was stranded. As pending, recovery gives it a River
--    job; with resolved_igdb_id set it goes to finalize.
UPDATE job_items ji
   SET status = 'pending'
  FROM jobs j
 WHERE j.id = ji.job_id
   AND j.job_type = 'import'
   AND j.status IN ('pending', 'processing')
   AND ji.status = 'processing';

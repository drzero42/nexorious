-- Rows the up step failed keep their new status: the old one is gone.
ALTER TABLE job_items DROP CONSTRAINT IF EXISTS job_items_status_check;
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_status_check;

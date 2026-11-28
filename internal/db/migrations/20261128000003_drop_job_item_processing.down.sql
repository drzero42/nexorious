-- Items the up step moved keep their new status.
ALTER TABLE job_items DROP CONSTRAINT IF EXISTS job_items_status_check;
ALTER TABLE job_items ADD CONSTRAINT job_items_status_check
    CHECK (status IN ('pending', 'processing', 'completed', 'pending_review', 'skipped', 'failed', 'cancelled'));

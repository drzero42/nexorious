-- jobs.auto_retry_done has not been set since the sync rewrite and nothing
-- reads it (#1337, #1398).
ALTER TABLE jobs DROP COLUMN auto_retry_done;

-- Import job items inserted before #1319/#1333 were stored with Go's zero time
-- (0001-01-01) because the model's created_at tag lacked nullzero, so the
-- column default never applied (#1323). Backfill them from the parent Job,
-- skipping Jobs that were themselves stored with the zero time.
UPDATE job_items ji
   SET created_at = j.created_at
  FROM jobs j
 WHERE ji.job_id = j.id
   AND ji.created_at < '1970-01-01'
   AND j.created_at >= '1970-01-01';

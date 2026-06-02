ALTER TABLE sync_changes RENAME TO changes;
ALTER INDEX sync_changes_job_id_idx     RENAME TO changes_job_id_idx;
ALTER INDEX sync_changes_user_id_idx    RENAME TO changes_user_id_idx;
ALTER INDEX sync_changes_created_at_idx RENAME TO changes_created_at_idx;

ALTER INDEX changes_created_at_idx RENAME TO sync_changes_created_at_idx;
ALTER INDEX changes_user_id_idx    RENAME TO sync_changes_user_id_idx;
ALTER INDEX changes_job_id_idx     RENAME TO sync_changes_job_id_idx;
ALTER TABLE changes RENAME TO sync_changes;

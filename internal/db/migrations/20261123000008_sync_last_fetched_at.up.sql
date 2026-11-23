-- last_fetched_at is when the store's library was last fetched successfully.
-- Unlike last_synced_at, Reset sync data leaves it alone, so the outage rule
-- that sync.failed and sync.recovered share keeps its boundary (#1405).
ALTER TABLE user_sync_configs ADD COLUMN last_fetched_at timestamptz;

UPDATE user_sync_configs SET last_fetched_at = last_synced_at;

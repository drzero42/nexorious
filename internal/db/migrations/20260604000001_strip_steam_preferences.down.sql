-- No-op: the stripped preferences.steam blobs cannot be restored (the mirrored
-- credentials are gone and were never the source of truth — the encrypted
-- user_sync_configs blob is). There is nothing to reverse.
SELECT 1;

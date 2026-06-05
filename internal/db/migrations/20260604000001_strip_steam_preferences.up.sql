-- Strip the legacy `steam` key from users.preferences. Older builds of the
-- Steam connection card mirrored {steam_id, web_api_key, is_verified, username}
-- into preferences.steam in addition to the encrypted user_sync_configs blob.
-- Nothing reads it back, and it stored the web_api_key secret in plaintext
-- preferences, so remove it. `preferences` is TEXT, so cast to jsonb, drop the
-- key, and cast back. Idempotent: the WHERE clause skips rows without the key.

UPDATE users
SET preferences = (preferences::jsonb - 'steam')::text
WHERE preferences::jsonb ? 'steam';

-- Add a nullable user_game_id to the changes table so Recent Activity can link
-- a change row to its resulting library entry (/games/$id). Captured at write
-- time in the sync/import workers (external_game_id is NULL for imports, so a
-- query-time join can't resolve imports — only the write site has the id).
-- ON DELETE SET NULL: if the user later deletes the game, the entry gracefully
-- degrades to plain text. No backfill: Recent Activity is recent-only and the
-- changes table is pruned by the cleanup_changes retention job, so old rows
-- simply render as plain text. No index (matches the un-indexed external_game_id
-- FK; the table is bounded by retention, so SET-NULL scans stay cheap).
ALTER TABLE changes
    ADD COLUMN user_game_id TEXT REFERENCES user_games(id) ON DELETE SET NULL;

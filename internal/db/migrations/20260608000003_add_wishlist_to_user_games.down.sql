DROP INDEX IF EXISTS user_games_wishlisted_idx;
ALTER TABLE user_games DROP COLUMN IF EXISTS is_wishlisted;

ALTER TABLE user_game_platforms
    DROP COLUMN IF EXISTS store_game_id,
    DROP COLUMN IF EXISTS store_url;

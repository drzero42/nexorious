ALTER TABLE user_game_platforms
    ADD COLUMN IF NOT EXISTS original_platform_name   TEXT,
    ADD COLUMN IF NOT EXISTS original_storefront_name TEXT;

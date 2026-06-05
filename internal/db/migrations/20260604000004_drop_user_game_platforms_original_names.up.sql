ALTER TABLE user_game_platforms
    DROP COLUMN IF EXISTS original_platform_name,
    DROP COLUMN IF EXISTS original_storefront_name;

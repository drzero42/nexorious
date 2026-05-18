-- Widen the unique constraint on external_games to allow one row per
-- (user, storefront, product, platform). Required for GOG games that
-- are available on both pc-windows and pc-linux.
ALTER TABLE external_games
    DROP CONSTRAINT external_games_user_id_storefront_external_id_key;

ALTER TABLE external_games
    ADD CONSTRAINT external_games_user_id_storefront_external_id_raw_platform_key
    UNIQUE NULLS NOT DISTINCT (user_id, storefront, external_id, raw_platform);

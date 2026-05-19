ALTER TABLE external_games
    DROP CONSTRAINT external_games_user_id_storefront_external_id_raw_platform_key;

ALTER TABLE external_games
    ADD CONSTRAINT external_games_user_id_storefront_external_id_key
    UNIQUE (user_id, storefront, external_id);

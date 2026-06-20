ALTER TABLE external_games
    DROP COLUMN IF EXISTS source_metadata,
    DROP COLUMN IF EXISTS store_link;

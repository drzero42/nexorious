-- Add store_link and source_metadata to external_games (#831).
-- store_link holds a per-store identifier (Steam appid / GOG slug / Epic slug /
-- PSN concept id), written by the enrichment worker.
-- source_metadata is a generic jsonb capture bag used by sync to stash
-- store-specific data (e.g. Epic's namespace). Both columns are nullable.
ALTER TABLE external_games
    ADD COLUMN store_link      TEXT,
    ADD COLUMN source_metadata JSONB;

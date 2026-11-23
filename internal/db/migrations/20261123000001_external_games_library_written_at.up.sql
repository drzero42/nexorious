-- #1338: when the library write last ran for an external game. The storefront
-- view counts a failed or in-review item only while it is newer than this, so
-- a rematch or a later write supersedes an old item without touching it.
-- Set only by the library write; external_games.updated_at is also bumped by
-- skip, unskip, the store-link refresh and the removed-game sweep. No backfill.
ALTER TABLE public.external_games ADD COLUMN library_written_at timestamptz;

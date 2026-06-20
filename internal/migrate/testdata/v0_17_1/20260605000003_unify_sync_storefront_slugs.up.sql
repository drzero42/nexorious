-- Unify sync-source slugs with storefronts.name (issue #850).
-- Renames the two divergent sync slugs (epic -> epic-games-store, psn ->
-- playstation-store) in the three columns that store the UNRESOLVED slug, plus
-- any in-flight dispatch_sync River job args. user_game_platforms.storefront is
-- NOT touched: it already stores the resolved catalog name.
--
-- Safe against the unique constraints on user_sync_configs (user_id, storefront)
-- and external_games (user_id, storefront, external_id): the target values
-- 'playstation-store'/'epic-games-store' were never valid sync slugs before, so
-- no colliding row can pre-exist.

UPDATE external_games   SET storefront = 'playstation-store' WHERE storefront = 'psn';
UPDATE external_games   SET storefront = 'epic-games-store'  WHERE storefront = 'epic';

UPDATE user_sync_configs SET storefront = 'playstation-store' WHERE storefront = 'psn';
UPDATE user_sync_configs SET storefront = 'epic-games-store'  WHERE storefront = 'epic';

UPDATE jobs SET source = 'playstation-store' WHERE source = 'psn';
UPDATE jobs SET source = 'epic-games-store'  WHERE source = 'epic';

-- River manages its own schema and may not exist yet on a fresh install when
-- this migration runs; guard with to_regclass so fresh installs don't fail.
DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    UPDATE river_job
       SET args = jsonb_set(args, '{storefront}', '"playstation-store"'::jsonb)
     WHERE kind = 'dispatch_sync' AND args->>'storefront' = 'psn';
    UPDATE river_job
       SET args = jsonb_set(args, '{storefront}', '"epic-games-store"'::jsonb)
     WHERE kind = 'dispatch_sync' AND args->>'storefront' = 'epic';
  END IF;
END $$;

-- Reverse of 20260605000003 up. Safe: post-up, the only 'playstation-store' /
-- 'epic-games-store' values in these three columns originated from the sync
-- slug, so reverting them to 'psn'/'epic' is the exact inverse.

UPDATE external_games   SET storefront = 'psn'  WHERE storefront = 'playstation-store';
UPDATE external_games   SET storefront = 'epic' WHERE storefront = 'epic-games-store';

UPDATE user_sync_configs SET storefront = 'psn'  WHERE storefront = 'playstation-store';
UPDATE user_sync_configs SET storefront = 'epic' WHERE storefront = 'epic-games-store';

UPDATE jobs SET source = 'psn'  WHERE source = 'playstation-store';
UPDATE jobs SET source = 'epic' WHERE source = 'epic-games-store';

DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    UPDATE river_job
       SET args = jsonb_set(args, '{storefront}', '"psn"'::jsonb)
     WHERE kind = 'dispatch_sync' AND args->>'storefront' = 'playstation-store';
    UPDATE river_job
       SET args = jsonb_set(args, '{storefront}', '"epic"'::jsonb)
     WHERE kind = 'dispatch_sync' AND args->>'storefront' = 'epic-games-store';
  END IF;
END $$;

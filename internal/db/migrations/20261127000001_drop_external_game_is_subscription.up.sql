-- A storefront game's subscription state is ownership_status = 'subscription'
-- (#1495). Before dropping the duplicate flag, give any game that has no
-- ownership_status the one the flag implied, as the library write did.
UPDATE external_games
SET ownership_status = CASE WHEN is_subscription THEN 'subscription' ELSE 'owned' END
WHERE ownership_status IS NULL;

ALTER TABLE external_games DROP COLUMN IF EXISTS is_subscription;

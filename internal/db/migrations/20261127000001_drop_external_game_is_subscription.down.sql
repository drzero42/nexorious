ALTER TABLE external_games ADD COLUMN IF NOT EXISTS is_subscription boolean DEFAULT false NOT NULL;
UPDATE external_games SET is_subscription = (ownership_status IS NOT DISTINCT FROM 'subscription');

UPDATE user_games SET play_status = 'not_started' WHERE play_status IS NULL;
ALTER TABLE user_games ALTER COLUMN play_status SET NOT NULL;
ALTER TABLE user_games ALTER COLUMN play_status SET DEFAULT 'not_started';

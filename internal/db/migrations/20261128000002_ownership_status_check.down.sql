-- Values the up step cleared stay NULL: the old ones are gone.
ALTER TABLE external_games DROP CONSTRAINT IF EXISTS external_games_ownership_status_check;
ALTER TABLE user_game_platforms DROP CONSTRAINT IF EXISTS user_game_platforms_ownership_status_check;

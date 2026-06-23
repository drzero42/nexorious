ALTER TABLE user_game_platforms
    DROP COLUMN achievements_unlocked,
    DROP COLUMN achievements_total;

ALTER TABLE external_game_platforms
    DROP COLUMN achievements_unlocked,
    DROP COLUMN achievements_total;

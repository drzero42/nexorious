ALTER TABLE external_game_platforms
    ADD COLUMN achievements_unlocked integer,
    ADD COLUMN achievements_total    integer;

ALTER TABLE user_game_platforms
    ADD COLUMN achievements_unlocked integer,
    ADD COLUMN achievements_total    integer;

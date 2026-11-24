ALTER TABLE external_games DROP CONSTRAINT external_games_resolved_igdb_id_fkey;
ALTER TABLE external_games
    ADD CONSTRAINT external_games_resolved_igdb_id_fkey FOREIGN KEY (resolved_igdb_id) REFERENCES games(id) ON DELETE SET NULL;

ALTER TABLE user_games DROP CONSTRAINT user_games_game_id_fkey;
ALTER TABLE user_games
    ADD CONSTRAINT user_games_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;

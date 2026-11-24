-- Only the unreferenced-games cleanup deletes games rows. With SET NULL and
-- CASCADE, a cleanup that raced a sync silently cleared the External game's
-- match or deleted the library entry the sync had just written (#1423).
-- NO ACTION makes such a delete fail instead; the cleanup retries next run.
ALTER TABLE external_games DROP CONSTRAINT external_games_resolved_igdb_id_fkey;
ALTER TABLE external_games
    ADD CONSTRAINT external_games_resolved_igdb_id_fkey FOREIGN KEY (resolved_igdb_id) REFERENCES games(id);

ALTER TABLE user_games DROP CONSTRAINT user_games_game_id_fkey;
ALTER TABLE user_games
    ADD CONSTRAINT user_games_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);

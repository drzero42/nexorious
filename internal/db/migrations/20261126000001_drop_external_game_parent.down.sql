-- Brings the column back empty: the families are not restored.
ALTER TABLE external_games ADD COLUMN IF NOT EXISTS parent_id text;
ALTER TABLE external_games
    ADD CONSTRAINT external_games_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES external_games(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS external_games_parent_id_idx ON external_games USING btree (parent_id) WHERE (parent_id IS NOT NULL);

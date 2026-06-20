ALTER TABLE external_games
    ADD COLUMN parent_id TEXT REFERENCES external_games(id) ON DELETE SET NULL;

CREATE INDEX external_games_parent_id_idx
    ON external_games (parent_id)
    WHERE parent_id IS NOT NULL;

-- Backfill existing sibling pairs.
-- The oldest row per (user_id, storefront, title) group is the parent.
-- All later rows get parent_id set to the oldest row's id.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY user_id, storefront, title
                              ORDER BY created_at ASC) AS rn,
           FIRST_VALUE(id) OVER (PARTITION BY user_id, storefront, title
                                 ORDER BY created_at ASC) AS parent_candidate_id
    FROM external_games
)
UPDATE external_games
SET parent_id  = ranked.parent_candidate_id,
    updated_at = now()
FROM ranked
WHERE external_games.id = ranked.id
  AND ranked.rn > 1;

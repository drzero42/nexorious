-- Derived editions (#1449, #1450): drop the stored parent link between External
-- games. Every Child game already carries its own match and skip, except one
-- adopted while its parent was skipped: it took the skip but never the match
-- (a skipped game gets no Job item). Give it its skipped parent's match, so the
-- two stay editions of each other once the link is gone.
UPDATE external_games c
SET resolved_igdb_id = p.resolved_igdb_id
FROM external_games p
WHERE c.parent_id = p.id
  AND c.is_skipped AND p.is_skipped
  AND c.resolved_igdb_id IS NULL AND p.resolved_igdb_id IS NOT NULL;

DROP INDEX IF EXISTS external_games_parent_id_idx;
ALTER TABLE external_games DROP CONSTRAINT IF EXISTS external_games_parent_id_fkey;
ALTER TABLE external_games DROP COLUMN IF EXISTS parent_id;

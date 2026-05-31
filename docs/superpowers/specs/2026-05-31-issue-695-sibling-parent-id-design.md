# Issue #695 — Establish sibling relationship at Stage 1 via parent_id

## Background

PSN assigns separate title IDs to the PS4 and PS5 versions of the same game, producing two
`external_games` rows for the same logical game. The sync pipeline currently handles these
implicitly:

- **Stage 2** searches for another row with the same `(user_id, storefront, title)` that already
  has `resolved_igdb_id` set and inherits it.
- **HandleRematchExternalGame** runs a title search to find unresolved siblings and cascades the
  match to them when the user resolves a `pending_review` item.

This has several problems:

- The sibling relationship is discovered by title search at runtime rather than recorded when the
  data arrives.
- If both siblings land in `pending_review` simultaneously, both appear as separate "Needs Review"
  entries — the user sees two items for the same game.
- Skip does not cascade to siblings at all.
- The title search is fragile: any title normalisation difference breaks it.

This spec replaces the implicit runtime discovery with an explicit `parent_id` FK established
during Stage 1.

---

## Schema

### Migration: `20260531000002_external_games_parent_id`

**Up:**

```sql
ALTER TABLE external_games
    ADD COLUMN parent_id TEXT REFERENCES external_games(id) ON DELETE SET NULL;

CREATE INDEX external_games_parent_id_idx
    ON external_games (parent_id)
    WHERE parent_id IS NOT NULL;
```

**Backfill existing sibling pairs** (in the same up migration):

The oldest row per `(user_id, storefront, title)` group is the parent. All later rows get
`parent_id` set to the oldest row's id.

```sql
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
```

**Down:**

```sql
ALTER TABLE external_games DROP COLUMN parent_id;
```

### Model (`internal/db/models/models.go`)

Add one field to `ExternalGame`:

```go
ParentID *string `bun:"parent_id" json:"parent_id,omitempty"`
```

---

## Stage 1 — upsertExternalGame

After the upsert, if the row was a fresh insert (`xmax = 0`), look for a parent candidate and set
`parent_id` on the new row.

**Key invariant:** only rows with `parent_id IS NULL` are eligible as parent candidates. This
ensures a flat tree (no chaining) even when N > 2 siblings arrive.

```go
var row struct {
    ID        string `bun:"id"`
    IsSkipped bool   `bun:"is_skipped"`
    IsNew     bool   `bun:"is_new"`
}
// INSERT ... ON CONFLICT ... RETURNING id, is_skipped, (xmax = 0) AS is_new

if row.IsNew {
    var parentID string
    if err := db.NewRaw(`
        SELECT id FROM external_games
        WHERE user_id = ? AND storefront = ? AND title = ?
          AND id != ? AND parent_id IS NULL
        LIMIT 1`,
        p.UserID, p.Storefront, e.Title, row.ID,
    ).Scan(ctx, &parentID); err == nil && parentID != "" {
        if _, err := db.NewRaw(`
            UPDATE external_games SET parent_id = ? WHERE id = ? AND parent_id IS NULL`,
            parentID, row.ID,
        ).Exec(ctx); err != nil {
            slog.Error("dispatch_sync: set parent_id failed", "err", err, "external_game_id", row.ID)
        }
    }
}
```

**Job item and Stage 2 enqueue:** unchanged — Stage 1 still inserts a `job_items` row and enqueues
Stage 2 for every external game including children. This is necessary for job completion tracking
and for Stage 3's sibling trigger to find a `pending` item to re-enqueue.

---

## Stage 2 — IGDBMatchWorker

Replace the title-based sibling check (lines 402–424 of `internal/worker/tasks/sync.go`) with a
`parent_id` lookup. The replacement runs after the "already resolved" fast-path and before the
IGDB search.

```go
// Child check: if this row has a parent, inherit or wait.
if eg.ParentID != nil {
    var parent models.ExternalGame
    if err := w.DB.NewSelect().Model(&parent).
        Where("id = ?", *eg.ParentID).
        Scan(ctx); err == nil && parent.ResolvedIGDBID != nil {
        // Parent resolved — inherit and proceed to Stage 3.
        igdbID := *parent.ResolvedIGDBID
        // INSERT INTO games ... ON CONFLICT DO NOTHING
        // UPDATE external_games SET resolved_igdb_id = ? WHERE id = ?
        return w.enqueueUserGame(ctx, item.ID, item.JobID)
    }
    // Parent not yet resolved — leave job_item in pending and return nil.
    // Stage 3 of the parent will re-enqueue Stage 2 for this child.
    slog.Debug("igdb_match: parent unresolved, waiting",
        "item_id", p.JobItemID, "parent_id", *eg.ParentID)
    return nil
}
// Existing IGDB search for non-child rows follows...
```

The title-based sibling block is **deleted entirely**.

---

## Stage 3 — UserGameWorker (sibling trigger)

After `syncMarkItemCompleted` (on the resolved path only — not the skip early-return), check for
unresolved children and re-enqueue Stage 2 for each:

```go
var childItems []struct {
    JobItemID      string `bun:"job_item_id"`
    ExternalGameID string `bun:"external_game_id"`
}
if err := w.DB.NewRaw(`
    SELECT ji.id AS job_item_id, eg.id AS external_game_id
    FROM external_games eg
    JOIN job_items ji ON ji.external_game_id = eg.id
    WHERE eg.parent_id = ?
      AND eg.resolved_igdb_id IS NULL
      AND NOT eg.is_skipped
      AND ji.status = 'pending'
    ORDER BY ji.created_at DESC`,
    eg.ID,
).Scan(ctx, &childItems); err == nil {
    for _, child := range childItems {
        if _, err := w.RiverClient.Insert(ctx, IGDBMatchArgs{JobItemID: child.JobItemID}, nil); err != nil {
            slog.Error("user_game_write: enqueue sibling Stage 2",
                "err", err, "child_eg_id", child.ExternalGameID, "job_item_id", child.JobItemID)
        }
    }
}
```

This handles the late-arriving sibling case: a child added to the library after its parent was
already matched will have Stage 2 triggered here, which fast-paths (parent is resolved) directly
to Stage 3.

---

## API Handlers

### HandleListExternalGames

Add `AND eg.parent_id IS NULL` to the WHERE clause. Children never appear in any UI list.

Expand the `platforms_csv` subquery to include platforms from children, so the parent entry shows
the full platform picture:

```sql
(SELECT string_agg(DISTINCT egp.platform, ',' ORDER BY egp.platform)
 FROM external_game_platforms egp
 WHERE egp.external_game_id = eg.id
    OR egp.external_game_id IN (
        SELECT id FROM external_games WHERE parent_id = eg.id
    )
) AS platforms_csv
```

### HandlePendingReviewCount

Two changes:

1. Add `JOIN external_games eg ON eg.id = ji.external_game_id` and filter `AND eg.parent_id IS
   NULL`. Children can never reach `pending_review` in the new model; this guards against existing
   data that will be backfilled by the migration.

2. Replace `COUNT(DISTINCT ji.source_title)` with `COUNT(*)`. The `DISTINCT source_title` dedup
   was a workaround for the implicit sibling problem. With `parent_id IS NULL` filtering it is
   structurally impossible for two `pending_review` items for the same logical game to coexist, so
   the workaround is no longer needed.

```sql
SELECT j.source, COUNT(*) AS count
FROM job_items ji
JOIN jobs j ON ji.job_id = j.id
JOIN external_games eg ON eg.id = ji.external_game_id
WHERE ji.user_id = ? AND ji.status = 'pending_review'
  AND eg.parent_id IS NULL
GROUP BY j.source
```

### HandleSkipGame / HandleUnskipGame

After updating the parent row, cascade to children:

```sql
UPDATE external_games SET is_skipped = ?, updated_at = now() WHERE parent_id = ?
```

**On skip:** also update each child's most recent `pending` or `pending_review` job_item to
`skipped` and call `SyncCheckJobCompletion` for each affected job — consistent with how the
parent's own job_item is handled.

**On unskip:** only update `is_skipped = false` on child external_game rows; job_items are not
modified, consistent with the current `HandleUnskipGame` behavior for the parent itself.

### HandleRematchExternalGame

Replace the title-based sibling query:

```sql
-- Before
SELECT id, external_id, title FROM external_games
WHERE user_id = ? AND storefront = ? AND title = ?
  AND id != ? AND is_skipped = false

-- After
SELECT id, external_id, title FROM external_games
WHERE parent_id = ? AND is_skipped = false
```

The rest of the cascade (set `resolved_igdb_id`, find/create job_item, enqueue Stage 3) is
unchanged.

---

## Testing

### Tests to update

- **`TestIGDBMatchWorker_SiblingResolution`** — rewrite to use `parent_id` instead of inserting a
  pre-resolved sibling with matching title. Add a second case: child with unresolved parent returns
  nil and leaves job_item in `pending`.
- **`TestPendingReviewCount_Deduplicates`** — verify that the `DISTINCT source_title` removal
  doesn't break the count; adjust test setup since dedup now comes from `parent_id IS NULL`
  filtering rather than title dedup.
- **`TestSkipGame_MarksJobItemSkippedAndCompletesJob`** — add a child row and verify the cascade
  skips it too.

### New tests

- **`TestIGDBMatchWorker_ChildInheritsFromResolvedParent`** — parent has `resolved_igdb_id`, child
  Stage 2 runs, child inherits and enqueues Stage 3.
- **`TestIGDBMatchWorker_ChildWaitsForUnresolvedParent`** — parent has no `resolved_igdb_id`,
  child Stage 2 returns nil, job_item remains `pending`.
- **`TestUserGameWorker_SiblingTrigger`** — Stage 3 for parent finds a pending child job_item and
  enqueues Stage 2 for it.
- **`TestHandleRematchExternalGame_CascadesToChildren`** — rematch on parent cascades
  `resolved_igdb_id` and enqueues Stage 3 for child via `parent_id`, not title search.
- **`TestPendingReviewCount_ExcludesChildren`** — child with `parent_id` set and a `pending_review`
  job_item is not counted.

---

## docs/sync.md updates

- **Data Model table** — add `parent_id` to the `external_games` row description: nullable FK to
  `external_games.id`; set when Stage 1 detects a same-title sibling.
- **Siblings section** — replace the two-place pull/push description with the three-place model:
  Stage 1 establishes the relationship, Stage 2 inherits from a resolved parent or waits, Stage 3
  triggers Stage 2 for unresolved children after the parent is written. Remove all mention of
  title search.
- **User Interactions section** — note that siblings never appear in review lists; resolving or
  skipping a parent cascades to all children automatically.

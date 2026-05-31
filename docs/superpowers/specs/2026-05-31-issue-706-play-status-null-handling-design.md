# Design: `play_status` NULL handling — issue #706

## Problem

`play_status` on `user_games` is `TEXT` — nullable with no default. Games added via sync arrive with `play_status = NULL` because the sync worker omits the column on insert. Games added manually also get NULL when the caller omits the field.

The status filter in the library uses `WHERE ug.play_status = ?`, which does not match NULL rows in SQL. The result: selecting any specific status (Not Started, In Progress, etc.) returns 0 games, while "All Statuses" works fine because it adds no WHERE clause at all.

## Root cause

Three gaps in the current implementation:

1. **Schema**: `play_status TEXT` — no `NOT NULL`, no `DEFAULT`. Any insert that omits the column stores NULL.
2. **Sync worker**: `INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)` — `play_status` intentionally omitted, leaving it NULL.
3. **No playtime inference**: Sync has access to `hours_played` per platform but never uses it to set an initial play status.

## Design

### 1 — DB migration

New migration file: `internal/db/migrations/20260531<timestamp>_play_status_not_null.up.sql` (timestamp assigned at implementation time per the `YYYYMMDDHHmmss` naming convention)

```sql
UPDATE user_games SET play_status = 'not_started' WHERE play_status IS NULL;
ALTER TABLE user_games ALTER COLUMN play_status SET NOT NULL;
ALTER TABLE user_games ALTER COLUMN play_status SET DEFAULT 'not_started';
```

Corresponding `.down.sql`:

```sql
ALTER TABLE user_games ALTER COLUMN play_status DROP DEFAULT;
ALTER TABLE user_games ALTER COLUMN play_status DROP NOT NULL;
```

Keep `TEXT` (not a PostgreSQL enum type) for consistency with `ownership_status` and to avoid painful `ALTER TYPE` migrations if values are ever added or renamed. The `user_games_play_status_idx` index already exists — no change needed.

### 2 — Sync worker (`UserGameWorker`, Stage 3)

The upsert at the top of Stage 3 is extended to also `RETURNING play_status`:

```sql
INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = now()
RETURNING id, (xmax = 0) AS is_new, play_status
```

The `isNewRow` struct gains a `PlayStatus *string` field to receive it.

After the platform loop, the worker computes total incoming hours from `egPlatforms` and conditionally updates `play_status`:

| Scenario | Behaviour |
|---|---|
| New row, total hours = 0 | DB default (`'not_started'`) applies; no explicit set required |
| New row, total hours > 0 | `UPDATE user_games SET play_status = 'in_progress' WHERE id = ?` |
| Existing row, `play_status = 'not_started'`, total hours > 0 | `UPDATE user_games SET play_status = 'in_progress' WHERE id = ?` |
| Existing row, `play_status ≠ 'not_started'` | Leave untouched — user has made an explicit choice |

"Total hours" is the sum of `egp.HoursPlayed` over all `egPlatforms` rows for this sync run. Sync can only auto-transition `not_started → in_progress`. Every other status is treated as user-set and never overwritten.

### 3 — Manually added games

No code change needed. After the migration, `POST /api/user-games` that omits `play_status` gets `'not_started'` from the DB default automatically. The API handler and filter code (`internal/filter/criteria.go`) are untouched — once NULLs are eliminated, `WHERE ug.play_status = ?` works correctly for all rows.

### 4 — Documentation (`docs/sync.md`)

Add a **Play Status** subsection inside the Stage 3 section, covering:

- Default value is `'not_started'`
- Sync infers initial play status from total incoming hours: hours > 0 → `in_progress`, otherwise the DB default applies
- The "hands off" rule: sync only auto-promotes `not_started → in_progress`; any other status the user has set is never touched
- Manually added games default to `'not_started'` unless the caller provides a value

## Out of scope

- Changing `play_status` from `TEXT` to a PostgreSQL native enum.
- Any status transitions beyond `not_started → in_progress`.
- Changing the Go model field from `*string` to `string` (consistent with the existing `ownership_status` pattern).

## Tasks

- [ ] Migration: backfill NULLs and add `NOT NULL DEFAULT 'not_started'` to `user_games.play_status`
- [ ] Sync worker: extend upsert to `RETURNING play_status`; after platform loop, set `play_status = 'in_progress'` on new rows when total hours > 0
- [ ] Sync worker: update `play_status` to `'in_progress'` on existing `not_started` rows when total hours > 0
- [ ] `docs/sync.md`: add Play Status subsection in Stage 3

# Data Model Sync Alignment Design

**Date:** 2026-05-24
**Branch:** issue-608-normalise-external-games
**Scope:** Align the single initial migration and Go model structs with the canonical sync spec in `docs/sync.md`. No worker or service code is changed in this pass; those will be updated in a follow-up session once the schema is stable.

> **Note:** Tests will fail after this change because worker and API code still references removed fields. That is expected and intentional — a separate code-update session will bring the callers in line with the new schema.

---

## Problem

`docs/sync.md` is the source of truth for how the sync system works. The data model deviates from it in six ways:

1. **`external_games.playtime_hours`** — exists in the schema; the spec says playtime belongs at the `external_game_platforms` level, not the game level.
2. **`external_game_platforms.hours_played`** — missing; the spec explicitly stores per-platform playtime here.
3. **`user_games.hours_played`** — exists as a stored column; the spec says it should be a calculated sum of `user_game_platforms.hours_played` (see issue #613). Removing it now while resetting dev databases avoids a separate migration later.
4. **`user_sync_configs.epic_legendary_state`** — exists as a JSONB column; the spec says Epic stores its session state in `storefront_credentials` (encrypted), not in a separate field.
5. **`job_items.external_game_id`** — missing FK to `external_games`; the spec defines one JobItem per ExternalGame for sync jobs. Non-sync jobs leave this null.
6. **`sync_changes` table** — completely absent; the spec defines it as the backing store for the Sync History UI.

Additionally, `JobItemStatusIGDBFailed` is a Go constant with no basis in the spec. The spec routes all IGDB failures to `pending_review` (recoverable) or `failed` (permanent). The constant is removed and `JobItemStatusCancelled` is added (it exists in the spec but was missing from the constants).

---

## Approach

**Schema-first clean break.** Edit the single migration file (`20260503000001_initial.up.sql`) and the Go model structs directly. The dev database is being cleared, so no incremental migration compatibility is required.

---

## Changes

### `20260503000001_initial.up.sql`

#### `external_games` — remove `playtime_hours`

```sql
-- before
playtime_hours INTEGER NOT NULL DEFAULT 0,

-- after: column removed entirely
```

#### `external_game_platforms` — add `hours_played`

```sql
-- after
CREATE TABLE external_game_platforms (
    id               TEXT PRIMARY KEY,
    external_game_id TEXT NOT NULL REFERENCES external_games(id) ON DELETE CASCADE,
    platform         TEXT NOT NULL,
    hours_played     NUMERIC(10,2) NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(external_game_id, platform)
);
```

#### `user_games` — remove `hours_played`

```sql
-- before
hours_played    NUMERIC(10,2),

-- after: column removed entirely
```

#### `user_sync_configs` — remove `epic_legendary_state`

```sql
-- before
epic_legendary_state JSONB,

-- after: column removed entirely
```

#### `job_items` — add `external_game_id`

```sql
-- add nullable FK to external_games
external_game_id    TEXT REFERENCES external_games(id) ON DELETE SET NULL,
```

Add a supporting index:

```sql
CREATE INDEX job_items_external_game_id_idx ON job_items (external_game_id);
```

#### New table: `sync_changes`

```sql
CREATE TABLE sync_changes (
    id               TEXT PRIMARY KEY,
    job_id           TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    external_game_id TEXT REFERENCES external_games(id) ON DELETE SET NULL,
    change_type      TEXT NOT NULL,   -- 'added', 'removed', 'status_changed'
    title            TEXT NOT NULL,
    old_status       TEXT,
    new_status       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sync_changes_job_id_idx  ON sync_changes (job_id);
CREATE INDEX sync_changes_user_id_idx ON sync_changes (user_id);
CREATE INDEX sync_changes_created_at_idx ON sync_changes (created_at);
```

---

### Go model structs

#### `internal/db/models/models.go`

- `ExternalGame`: remove `PlaytimeHours int` field
- `ExternalGamePlatform`: add `HoursPlayed float64` field (`bun:"hours_played,notnull"`)
- `UserGame`: remove `HoursPlayed *float64` field
- `UserSyncConfig`: remove `EpicLegendaryState json.RawMessage` field
- Add new `SyncChange` struct

**`SyncChange` struct:**

```go
type SyncChange struct {
    bun.BaseModel `bun:"table:sync_changes"`

    ID             string    `bun:"id,pk"              json:"id"`
    JobID          string    `bun:"job_id,notnull"     json:"job_id"`
    UserID         string    `bun:"user_id,notnull"    json:"user_id"`
    ExternalGameID *string   `bun:"external_game_id"   json:"external_game_id"`
    ChangeType     string    `bun:"change_type,notnull" json:"change_type"`
    Title          string    `bun:"title,notnull"      json:"title"`
    OldStatus      *string   `bun:"old_status"         json:"old_status"`
    NewStatus      *string   `bun:"new_status"         json:"new_status"`
    CreatedAt      time.Time `bun:"created_at,notnull" json:"created_at"`
}
```

#### `internal/db/models/jobs.go`

- Remove: `JobItemStatusIGDBFailed = "igdb_failed"`
- Add: `JobItemStatusCancelled = "cancelled"`

#### `internal/db/models/jobs.go` — `JobItem` struct

- Add: `ExternalGameID *string` field (`bun:"external_game_id"`)

#### `internal/db/models/models.go` — imports

Removing `UserSyncConfig.EpicLegendaryState` (the only `json.RawMessage` field in `models.go`) means the `encoding/json` import is no longer needed in that file and should be dropped.

---

## What is NOT in scope

- Updating worker code (`internal/worker/tasks/sync.go`) — the code still references removed fields; that is the next session's job.
- Updating API handlers (`internal/api/sync.go`, `internal/api/jobs.go`) — same reason.
- Updating service adapters (`services/steam`, `services/psn`, `services/gog`) — these pass `PlaytimeHours` in `GameEntry` structs; the adapter interface itself doesn't change in this pass.
- The `hours_played` refactor for `user_games` beyond removing the column (issue #613 is resolved by the column removal; the calculated-sum read path is a query change, not a schema change).
- Running tests — tests will fail until the code-update session completes.

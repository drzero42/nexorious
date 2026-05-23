# Design: Normalise external_games — separate platform associations into own table (issue #608)

## Problem

`external_games` uses one row per `(user_id, storefront, external_id, raw_platform)`. A game available on multiple platforms (e.g. Counter-Strike 2 on Windows + Linux, a GOG game on Windows + Linux) produces multiple rows. This causes:

- **Counts**: `COUNT(*) FROM external_games` returns platform entries, not games — making sync counters misleading.
- **Semantics**: `is_skipped`, `is_available`, `title`, and the unavailability sweep all logically apply to a game, not a platform entry.
- **job_items bloat**: one job_item per platform row inflates progress counters (806 items for a 457-game library).
- **Worker complexity**: every storefront sync case fans out one upsert per platform; `ProcessSyncItemWorker` handles them as independent items.

## Solution

Normalise to two tables:

- `external_games` — one row per `(user_id, storefront, external_id)`. Game-level fields only.
- `external_game_platforms` — one row per `(external_game_id, platform)`. Holds the resolved canonical platform slug.

Platform slugs are resolved to canonical form at **insertion time** (in `DispatchSyncWorker`) using `platformresolution.RawPlatformToSlug()`. The `external_game_platforms.platform` column maps directly to `platforms.name`.

## Migration approach

Pre-1.0, no production data exists. All five existing migration pairs are deleted. A single replacement `20260503000001_initial.up.sql` contains the full schema including the normalised `external_games` and new `external_game_platforms`. Dev databases are reset.

## Schema

```sql
-- external_games: unique on (user_id, storefront, external_id); no platform column
CREATE TABLE external_games (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storefront       TEXT NOT NULL,
    external_id      TEXT NOT NULL,
    title            TEXT NOT NULL,
    resolved_igdb_id INTEGER REFERENCES games(id) ON DELETE SET NULL,
    is_skipped       BOOLEAN NOT NULL DEFAULT false,
    is_available     BOOLEAN NOT NULL DEFAULT true,
    is_subscription  BOOLEAN NOT NULL DEFAULT false,
    playtime_hours   INTEGER NOT NULL DEFAULT 0,
    ownership_status TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, storefront, external_id)
);

-- external_game_platforms: one row per resolved canonical platform per game
CREATE TABLE external_game_platforms (
    id               TEXT PRIMARY KEY,
    external_game_id TEXT NOT NULL REFERENCES external_games(id) ON DELETE CASCADE,
    platform         TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(external_game_id, platform)
);
```

`external_game_platforms` has no `updated_at` — platform membership rows are immutable once created; only additions and removals happen.

## Models

`ExternalGame` loses `RawPlatform`. A `Platforms` has-many relation is declared but not eagerly loaded — only `ProcessSyncItemWorker` loads it explicitly.

```go
type ExternalGame struct {
    bun.BaseModel `bun:"table:external_games"`
    ID              string    `bun:"id,pk"`
    UserID          string    `bun:"user_id,notnull"`
    Storefront      string    `bun:"storefront,notnull"`
    ExternalID      string    `bun:"external_id,notnull"`
    Title           string    `bun:"title,notnull"`
    ResolvedIGDBID  *int32    `bun:"resolved_igdb_id"`
    IsSkipped       bool      `bun:"is_skipped,notnull"`
    IsAvailable     bool      `bun:"is_available,notnull"`
    IsSubscription  bool      `bun:"is_subscription,notnull"`
    PlaytimeHours   int       `bun:"playtime_hours,notnull"`
    OwnershipStatus *string   `bun:"ownership_status"`
    CreatedAt       time.Time `bun:"created_at,notnull"`
    UpdatedAt       time.Time `bun:"updated_at,notnull"`

    Platforms []ExternalGamePlatform `bun:"rel:has-many,join:id=external_game_id" json:"-"`
}

type ExternalGamePlatform struct {
    bun.BaseModel `bun:"table:external_game_platforms"`
    ID             string    `bun:"id,pk"`
    ExternalGameID string    `bun:"external_game_id,notnull"`
    Platform       string    `bun:"platform,notnull"`
    CreatedAt      time.Time `bun:"created_at,notnull"`
}
```

## DispatchSyncWorker

### Per-storefront pattern

For every game in every storefront:

1. Upsert one `external_games` row (`ON CONFLICT (user_id, storefront, external_id) DO UPDATE`).
2. Resolve each storefront platform identifier to a canonical slug via `platformresolution.RawPlatformToSlug()`. If resolution fails, log an error and fall back to the storefront default (see below). This guarantees every `external_games` row always has at least one platform row.
3. Upsert each resolved platform into `external_game_platforms` (`ON CONFLICT DO NOTHING`).
4. Remove any `external_game_platforms` rows for that game that are no longer present upstream (platform removed from the storefront).
5. Dispatch one `job_item` per `external_games` row, `item_key = external_id`.

**Default platforms by storefront (fallback when resolution fails or no platform data):**

| Storefront | Default |
|---|---|
| steam | `pc-windows` |
| psn | `playstation-4` |
| epic | `pc-windows` |
| gog | `pc-windows` |

### Steam

- The `existing` map (`map[string][]string`, used to skip `GetAppDetailsPlatforms`) is removed. `GetAppDetailsPlatforms` is called for every game on every sync to keep platform data current (e.g., a developer adding Linux support after the initial sync).
- The existing appdetails fallback logic (error → `pc-windows`; no platforms returned → `pc-windows`) is retained.
- After resolving current platforms for a game, delete any `external_game_platforms` rows for that game not in the current set.
- IGDB resolution caching is unchanged: `ProcessSyncItemWorker` skips IGDB search if `resolved_igdb_id` is already set. This is the meaningful performance cache.
- `item_key = external_id` (was `external_id + ":" + raw_platform`).

### PSN

- Each PSN `TitleID` maps to exactly one platform (PS4 or PS5). One `external_games` row per TitleID, one `external_game_platforms` row per game. No platform reconciliation needed.
- `item_key = external_id` (unchanged from current pattern).
- PSN's cross-SKU model (separate TitleIDs for PS4 and PS5 versions of the same game) is handled at resolution time — see cross-SKU inheritance below. Counts for PSN will reflect SKUs, not underlying game titles. This is acceptable given PSN's data model.

### Epic

- Always resolves to `pc-windows`. Upsert platform row. No reconciliation needed.
- `item_key = external_id` (unchanged).

### GOG

- GOG library returns one entry per platform per game (same external_id can appear once for Windows, once for Linux in different batches). Accumulate a `seenPlatforms map[string]map[string]struct{}` (external_id → set of canonical platform slugs) across all batches during the stream.
- After the stream completes, reconcile: for each external_id seen, delete `external_game_platforms` rows not in the seen set.
- `item_key = external_id` (was `external_id + ":" + raw_platform`).

### job_item source_metadata

`raw_platform` is removed. Metadata is now:

```json
{"external_game_id": "...", "playtime_hours": N}
```

### Step 5 — unavailability sweep

Unchanged. Queries `external_games` by `external_id`; marks `is_available = false` on the game row. Platform rows are permanent membership records; unavailability is a game-level flag.

## ProcessSyncItemWorker

### Removed: step 7 (platform/storefront resolution)

`platformresolution.RawPlatformToSlug()` is no longer called here — platform slugs are already canonical in `external_game_platforms`. `StorefrontToCollectionSlug(eg.Storefront)` is still called once per item (storefront slug is game-level, not per-platform).

### New step 7 — iterate platform rows

After IGDB resolution and finding/creating the `user_game`, load all `external_game_platforms` rows for the game:

```go
var platforms []models.ExternalGamePlatform
w.DB.NewSelect().Model(&platforms).Where("external_game_id = ?", eg.ID).Scan(ctx)

if len(platforms) == 0 {
    syncMarkItemFailed(ctx, w.DB, &item, "external game has no platform rows")
    syncCheckJobCompletion(...)
    return nil
}

storefrontSlug, ok := platformresolution.StorefrontToCollectionSlug(eg.Storefront)
if !ok {
    syncMarkItemFailed(...)
    return nil
}

for _, egp := range platforms {
    // find-or-create user_game_platform using egp.Platform directly
    // ownership-rank guard applied per platform row
}
```

Since all platform slugs are resolved at sync time, there are no per-platform resolution failures here. Empty `platforms` is a bug (log + fail the item).

### Cross-SKU IGDB inheritance (steps 3.5 and 3.6)

Unchanged. These operate on `external_games` rows matched by `(user_id, storefront, title)` with a different `id`. The normalization does not affect PSN's TitleID model — PS4 and PS5 SKUs remain separate `external_games` rows, correctly resolving to the same IGDB game and thus the same `user_game` with two `user_game_platforms`.

## API handlers

No functional changes required:

- `HandleListExternalGames` — query joins `user_game_platforms ON ugp.external_game_id = eg.id`; unchanged. Now returns one row per game (correct).
- `HandleSkipExternalGame` / `HandleUnskipExternalGame` — operate on `external_games.id`; unchanged.
- `HandleResetSyncData` — deletes `external_games WHERE user_id = ? AND storefront = ?`; `external_game_platforms` cascade-deletes automatically.
- `HandleRematchExternalGame` — operates on `external_games.resolved_igdb_id`; unchanged.
- `job_items.go` handlers — operate on `external_games.resolved_igdb_id` and `is_skipped`; unchanged.

## Tests

### Rewritten

- **`TestDispatchSync_Steam_MultiPlatform_WindowsAndLinux`** — asserts 1 `external_games` row + 2 `external_game_platforms` rows (`pc-windows`, `pc-linux`); 1 `job_item` keyed `"730"`.
- **`TestDispatchSync_Steam_CacheHit_SkipsAppDetails`** — removed. The cache no longer exists. Replaced by `TestDispatchSync_Steam_PlatformUpdate_AddsNewPlatform`: pre-seeds an `external_games` row with one `external_game_platforms` row (`pc-windows`); second sync returns `{Windows: true, Linux: true}`; asserts a second `external_game_platforms` row (`pc-linux`) is added.
- **`TestDispatchSync_Steam_AppDetailsFailure_FallsBackToWindows`** — asserts 1 `external_game_platforms` row with `platform = 'pc-windows'`; item_key is `"888"`.

### Updated

All tests that INSERT directly into `external_games` drop the `raw_platform` column. Tests for `ProcessSyncItemWorker` seed an `external_game_platforms` row in addition to the `external_games` row.

## Files to change

| File | Change |
|---|---|
| `internal/db/migrations/` | Delete all 5 pairs; replace with single `20260503000001_initial.{up,down}.sql` |
| `internal/db/models/models.go` | Remove `RawPlatform` from `ExternalGame`; add `ExternalGamePlatform` |
| `internal/worker/tasks/sync.go` | All four storefront cases + `ProcessSyncItemWorker` |
| `internal/worker/tasks/sync_test.go` | Rewrite 3 tests; update all `external_games` INSERTs |
| `internal/api/sync.go` | No functional changes required |
| `slumber.yaml` | No new routes; no changes needed |

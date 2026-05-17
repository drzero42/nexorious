# Reset Sync Data — Design Spec

**Date:** 2026-05-17
**Status:** Approved

## Problem

After a sync there is no way to undo it for a given storefront. If matches went wrong, skip choices piled up incorrectly, or the sync produced bad data, the only option is to manually clean up row by row. A "reset" action is needed to wipe the slate and start fresh without disconnecting credentials.

## Scope

Reset is scoped to one user + one storefront at a time. It removes all synced data for that combination and cancels any in-progress sync job. Credentials and sync configuration are untouched.

## Tables Affected

| Table | Action |
|---|---|
| `external_games` | DELETE WHERE `user_id = ? AND storefront = ?` — primary action; wipes all IGDB resolutions and skip choices |
| `user_game_platforms` | DELETE WHERE `storefront = ?` and `user_game_id` owned by the user — must go first to drop the FK ref to `external_games` |
| `user_games` | **Not touched** — user library entries are preserved even if they become platform-less |
| `user_sync_configs` | `last_synced_at` reset to NULL; credentials and frequency unchanged |
| `jobs` / `job_items` | Historical records kept; active job cancelled (see below) |

## Backend

### New endpoint

`DELETE /api/sync/:storefront/data` — `HandleResetSyncData` on `SyncHandler`.

Registered in `RegisterRoutes` before `g.POST("/:storefront", ...)` (Echo v5 static-before-param ordering). Storefront validated against `validConfigStorefronts` (steam/psn/epic). Requires JWT. Returns 204. Idempotent — no data and no active job both succeed silently.

### Handler logic

**1. Cancel any active sync job.**

Query `jobs WHERE user_id = ? AND source = ? AND job_type = 'sync' AND status IN ('pending', 'processing')`. If found and not terminal:

- `UPDATE jobs SET status = 'cancelled', completed_at = now() WHERE id = ?`
- `UPDATE river_job SET state = 'cancelled', finalized_at = NOW() WHERE state IN ('available', 'scheduled', 'retryable', 'pending') AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`

**2. Delete data in a single transaction.**

```sql
-- (a) Drop platform rows first — FK to external_games
DELETE FROM user_game_platforms
WHERE storefront = ?
  AND user_game_id IN (SELECT id FROM user_games WHERE user_id = ?);

-- (b) Wipe all external game data for this user+storefront
DELETE FROM external_games
WHERE user_id = ? AND storefront = ?;

-- (c) Reset sync timestamp
UPDATE user_sync_configs
SET last_synced_at = NULL, updated_at = now()
WHERE user_id = ? AND storefront = ?;
```

### Fix: guard `syncCheckJobCompletion` against overwriting terminal status

Any `ProcessSyncItemWorker` already mid-flight when the reset runs will fail to load its now-deleted `external_game`, call `syncMarkItemFailed`, then `syncCheckJobCompletion`. Without a guard that function would flip the cancelled job back to `completed` or `completed_with_errors`.

Fix: add `AND status IN ('pending', 'processing')` to the two terminal `UPDATE jobs SET status = ...` statements at the end of `syncCheckJobCompletion`.

### Tests (`internal/api/sync_test.go`)

- Happy path, no active job: `external_games` and `user_game_platforms` deleted, `last_synced_at` is NULL, `user_games` untouched
- Happy path, active job present: job is cancelled before data is deleted
- Invalid storefront → 400
- Unauthorized → 401
- Already-empty state → 204 (idempotent)

## Frontend

### API (`ui/frontend/src/api/sync.ts`)

```ts
async function resetSyncData(platform: SyncPlatform): Promise<void>
// DELETE /api/sync/:platform/data — expects 204
```

### Hook (`ui/frontend/src/hooks/use-sync.ts`)

New `useResetSyncData` mutation. On success: invalidate external-games query and sync config/status queries for the platform. Exported from `hooks/index.ts`.

### UI (`ui/frontend/src/routes/_authenticated/sync/index.tsx`)

"Reset sync data" button added to the sync service card alongside the existing trigger-sync and disconnect buttons.

- Disabled while `isSyncing` (same guard as trigger button)
- Clicking opens a confirmation dialog:
  > *"This will remove all imported games and match history for [Storefront]. Your game library entries will not be deleted. This cannot be undone."*
- On confirm: calls mutation, shows success toast on completion

## What Is Not Reset

- Sync credentials (`storefront_credentials`)
- Sync frequency / `auto_add` config
- `user_games` rows (library entries survive)
- `jobs` / `job_items` history

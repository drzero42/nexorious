# Shared Sync Code Refactor Design

**Date:** 2026-05-24
**Branch:** issue-608-normalise-external-games
**Status:** Approved, ready for implementation

---

## Context

The data model and migrations were aligned with `docs/sync.md` in the preceding session. The Go code now fails to compile because the model alignment removed `ExternalGame.PlaytimeHours` and `UserGame.HoursPlayed`, and `sync.go` still references both. Beyond the compile errors, the current `sync.go` has two structural gaps vs. the spec:

- `ProcessSyncItemWorker` conflates Stage 2 (IGDB matching) and Stage 3 (user game write) into one River worker.
- `igdb_failed` is used as a custom retry mechanism; the spec says River's own retry handles transient IGDB failures.

Additionally, `sync_changes` writes are missing entirely, `job_items.external_game_id` is set via a JSON hack rather than the direct column, and pending items are never cancelled when a job fails mid-run.

This refactor fixes the compile errors and aligns the shared sync pipeline (Stages 1, 2, 3) with the spec without touching the per-storefront adapter implementations in `services/`.

---

## Scope

**In scope:**
- Split `ProcessSyncItemWorker` → `IGDBMatchWorker` (Stage 2) + `UserGameWorker` (Stage 3)
- Patch `DispatchSyncWorker` (Stage 1) for compile errors and missing spec behaviours
- Add `sync_changes` writes (Stage 1: removed; Stage 3: added, status_changed)
- Fix playtime: move from `external_games.playtime_hours` to `external_game_platforms.hours_played`
- Fix `job_items.external_game_id`: set directly, not via `source_metadata` JSON
- Remove `igdb_failed` status and its auto-retry logic; simplify `syncCheckJobCompletion`
- Cancel pending items when a job fails
- Fix `export.go` and `import_item.go` compile errors; add backward-compat fallback for old exports
- Update all call sites (API handlers, orphan rescuer, worker registration)

**Out of scope:**
- Per-storefront adapter implementations (Steam, PSN, GOG, Epic) — separate session
- Extracting the `StorefrontAdapter` interface — separate session with the adapters
- Removing the now-unused `jobs.auto_retry_done` column — would need a migration, deferred

---

## New Worker Types

### IGDBMatchWorker — Stage 2

River kind: `igdb_match`
Args: `IGDBMatchArgs{JobItemID string}`
MaxAttempts: 5

**Flow:**

1. Load `job_item`; load `external_game` via `job_item.external_game_id`
2. If `is_skipped` → enqueue `UserGameArgs`, return
3. Sibling check: find another `external_games` row for the same `(user_id, storefront, title)` with `resolved_igdb_id` set → inherit the ID, persist to `external_game.resolved_igdb_id` → enqueue `UserGameArgs`, return
4. If `external_game.resolved_igdb_id` already set → enqueue `UserGameArgs`, return
5. Search IGDB; score candidates with fuzzy matching
6. If best score ≥ 0.85 and margin over second-best > 0.01 → set `resolved_igdb_id` on `external_game`, ensure `games` row exists → enqueue `UserGameArgs`, return
7. Otherwise → store candidates in `job_item.igdb_candidates`, set `match_confidence`, mark item `pending_review`, call `syncCheckJobCompletion`, return

**Transient IGDB API failures:** return the error so River retries with exponential backoff. On the final attempt (`job.Attempt >= job.MaxAttempts`), fall through to step 7 (mark `pending_review`) instead of returning an error, so the item is not stranded.

### UserGameWorker — Stage 3

River kind: `user_game_write`
Args: `UserGameArgs{JobItemID string}`
MaxAttempts: 5

**Flow:**

1. Load `job_item`; load `external_game` via `job_item.external_game_id`
2. If `is_skipped` → update `external_game.updated_at`, mark item `skipped`, call `syncCheckJobCompletion`, return
3. **Manual resolution propagation:** if `external_game.resolved_igdb_id` is nil but `job_item.resolved_igdb_id` is set → write `resolved_igdb_id` to `external_game` (this is the manual-resolution path only; auto-resolve writes it in Stage 2)
4. Ensure `games` row exists for `resolved_igdb_id` (ON CONFLICT DO NOTHING)
5. Upsert `user_games` (conflict key: `user_id, game_id`). If the row was newly inserted, write an `added` entry to `sync_changes`
6. Load `external_game_platforms` rows for this `external_game`
7. Resolve storefront slug via `platformresolution.StorefrontToCollectionSlug`
8. For each `external_game_platforms` row:
   - Upsert `user_game_platforms` (conflict key: `user_game_id, platform, storefront`)
   - On INSERT: set all fields including `hours_played = egp.HoursPlayed`
   - On UPDATE (conflict): apply ownership rank guard — only upgrade, never downgrade. If ownership status changes, write a `status_changed` entry to `sync_changes`. Update `hours_played` only if `egp.HoursPlayed > stored`
9. Update `external_game.updated_at`
10. Mark item `completed`, call `syncCheckJobCompletion`

---

## DispatchSyncWorker Patches

The per-storefront switch cases are unchanged. Only targeted fixes:

### Playtime moves to `external_game_platforms`

Remove `playtime_hours` from all `external_games` INSERT/UPDATE SQL. Add `hours_played` to each `external_game_platforms` upsert:

```sql
INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
VALUES (?, ?, ?, ?, now())
ON CONFLICT (external_game_id, platform) DO UPDATE SET
    hours_played = GREATEST(EXCLUDED.hours_played, external_game_platforms.hours_played)
```

Per-storefront playtime assignment (handled inline within the existing switch cases):
- **Steam:** assign `og.PlaytimeHours` to the highest-priority platform (`pc-windows` → `mac` → `pc-linux`); all others get 0
- **PSN:** assign `e.PlaytimeHours` to the single platform for each entry
- **GOG / Epic:** always 0

### `job_items.external_game_id` set directly

Replace the `source_metadata` JSON payload (`{"external_game_id": "...", "playtime_hours": N}`) with:
- `external_game_id` set directly in the `job_items` INSERT column
- `source_metadata` set to `'{}'`

### Cancel pending items on failure

`failSyncJob` adds a second statement after marking the job failed:

```sql
UPDATE job_items SET status = 'cancelled' WHERE job_id = ? AND status = 'pending'
```

### `sync_changes` for removed games

In the availability sweep, for each game marked `is_available = false`, also insert:

```sql
INSERT INTO sync_changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
VALUES (?, ?, ?, ?, 'removed', ?, now())
```

### Stage 2 enqueue

All `ProcessSyncItemArgs{...}` enqueues become `IGDBMatchArgs{...}`.

---

## `syncCheckJobCompletion` Simplification

The `igdb_failed` auto-retry logic is removed entirely. New logic:

1. If any items are `pending` or `processing` → return (still active)
2. If any items are `pending_review` → return (awaiting user action)
3. If any items are `failed` → mark job `completed_with_errors`
4. Otherwise → mark job `completed`

`syncMarkItemIGDBFailed` is deleted.

`jobs.auto_retry_done` column is left in the DB but never written or read.

---

## Call-Site Updates

### `internal/worker/tasks/enqueue.go`

`ArgsForJobType` sync case returns `IGDBMatchArgs` — orphaned items always re-enter at Stage 2, which fast-paths to Stage 3 if already resolved.

### `internal/api/sync.go`

- `HandleUnskipGame`: enqueue `IGDBMatchArgs` (game needs matching)
- `HandleResolveItem`:
  - Fix `eg.PlaytimeHours` compile error (remove reference)
  - Fix job_item INSERT: set `external_game_id` directly, drop `source_metadata` payload
  - Enqueue `UserGameArgs` instead of `ProcessSyncItemArgs`
  - Note: this handler already writes `external_game.resolved_igdb_id` directly before enqueuing, so UserGameWorker's propagation step 3 will be a no-op (external_game already has the value). This is correct — the result is identical.
  - Note: the sibling push mechanic (per spec: resolve unresolved siblings at the time of user action) is not currently implemented and is out of scope for this refactor.

### `internal/scheduler/orphaned_items.go`

Sync case enqueues `IGDBMatchArgs`.

### `cmd/nexorious/serve.go`

Replace `ProcessSyncItemWorker` registration with `IGDBMatchWorker{DB, IGDBClient, RiverClient}` and `UserGameWorker{DB, RiverClient}` — in both the initial setup block and the hot-reload block.

---

## Export / Import Fixes

### `export.go`

- Remove `HoursPlayed: ug.HoursPlayed` from `exportGameJSON` construction. The `hours_played` field on the game-level export struct is computed as the sum of `p.HoursPlayed` across all platform rows.
- `exportStats.TotalHours` and the CSV hours column are similarly computed by summing platform rows.
- Per-platform `hours_played` in `exportPlatformJSON` is unchanged — it already reads from `user_game_platforms.hours_played`.

### `import_item.go`

- Remove `HoursPlayed: gd.HoursPlayed` from the `user_games` struct literal (column no longer exists).
- In the platform loop, add a fallback: if `pd.HoursPlayed` is nil but `gd.HoursPlayed` is non-nil and this is the first platform row, use `gd.HoursPlayed`. This handles old exports that recorded a game-level total without per-platform breakdown.

---

## Files Changed

| File | Change |
|---|---|
| `internal/worker/tasks/sync.go` | Add `IGDBMatchWorker` + `UserGameWorker`; patch `DispatchSyncWorker`; remove `ProcessSyncItemWorker`; remove `syncMarkItemIGDBFailed`; simplify `syncCheckJobCompletion` |
| `internal/worker/tasks/enqueue.go` | `ArgsForJobType` sync case → `IGDBMatchArgs` |
| `internal/worker/tasks/export.go` | Fix `ug.HoursPlayed` compile errors; compute hours from platforms |
| `internal/worker/tasks/import_item.go` | Remove `ug.HoursPlayed`; add game-level hours fallback |
| `internal/api/sync.go` | `HandleUnskipGame` → `IGDBMatchArgs`; `HandleResolveItem` → `UserGameArgs` |
| `internal/scheduler/orphaned_items.go` | Sync case → `IGDBMatchArgs` |
| `cmd/nexorious/serve.go` | Replace `ProcessSyncItemWorker` with `IGDBMatchWorker` + `UserGameWorker` |

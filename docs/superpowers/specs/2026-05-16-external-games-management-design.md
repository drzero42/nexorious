# External Games Management Design

**Date:** 2026-05-16
**Status:** Approved

## Overview

Each storefront sync page gains an "External Games" section showing all external game records for that platform. Users can see how their storefront library maps to IGDB, change or establish matches, and manage skipped games — without waiting for a new sync.

## Context

When a sync runs, the backend creates `external_games` rows representing the user's library on a given storefront. Most games are auto-matched to an IGDB ID. Games that don't match confidently land in `pending_review` for manual resolution. Games the user explicitly skips are marked `is_skipped = true`.

Future syncs recognise existing external games and skip re-processing them, so the user only needs to act on genuinely new games. Currently there is no way to browse the full set of external games for a storefront, re-evaluate a skip, or change a match that was made automatically or manually in a prior sync.

## Backend

### New endpoint: `GET /api/sync/:storefront/external-games`

Returns all external games for the authenticated user and the given storefront. Invalid storefront returns 400. Response is a flat array (no pagination — even large Steam libraries are acceptable to load at once).

Each item includes join data derived from `user_game_platform → user_game`:

```json
{
  "id": "...",
  "storefront": "steam",
  "external_id": "...",
  "title": "Half-Life 2: Episode One",
  "resolved_igdb_id": 1234,
  "is_skipped": false,
  "is_available": true,
  "is_subscription": false,
  "playtime_hours": 12,
  "has_user_game": true,
  "user_game_id": "...",
  "igdb_title": "Half-Life 2: Episode One",
  "user_game_other_platform_count": 2
}
```

`igdb_title` is null for unmatched games. `user_game_other_platform_count` is the count of *other* `user_game_platform` rows on the linked user_game (i.e. excluding the one tied to this external game); zero means rematching would orphan the user_game.

### New endpoint: `POST /api/sync/external-games/:id/rematch`

Changes the IGDB match for an external game and triggers immediate re-import. Requires the user to own the external game (404 otherwise).

Request body:
```json
{
  "igdb_id": 5678,
  "orphan_action": "keep" | "remove"
}
```

`orphan_action` is required when `user_game_other_platform_count === 0`; the backend returns 409 if it would orphan a user_game and `orphan_action` is absent.

Sequence:
1. Verify ownership.
2. If the external game currently has a `resolved_igdb_id` (i.e. it was previously matched), find the `user_game_platform` where `external_game_id = :id`. If found, delete it. If not found (e.g. a prior import failed partway), continue without error.
3. If a `user_game_platform` was deleted and the linked user_game now has zero platforms: apply `orphan_action` — delete the user_game if `"remove"`, leave it if `"keep"`.
4. Update `external_games`: set `resolved_igdb_id = igdb_id`, `is_skipped = false`.
5. Create a Job record (type sync, status processing) and a JobItem with `resolved_igdb_id` pre-set, then enqueue a `ProcessSyncItem` River task. Because `resolved_igdb_id` is already set, the worker skips IGDB search and goes straight to import.

Returns 204 on success.

### Modified: `HandleUnskipGame` (`DELETE /api/sync/ignored/:id`)

After setting `is_skipped = false`, creates a Job record + JobItem for the external game and enqueues a `ProcessSyncItem` River task **without** a pre-set `resolved_igdb_id`. The worker runs normal IGDB matching including the confidence check; a low-confidence result lands in `pending_review` as usual.

If River enqueue fails after the DB update, log the error but still return 204. The game is unskipped and will be picked up by the next full sync.

No changes to `HandleSkipGame`.

## Frontend

### API layer (`api/sync.ts`)

Two new functions:
- `getExternalGames(platform: SyncPlatform): Promise<ExternalGame[]>`
- `rematchExternalGame(id: string, igdbId: number, orphanAction?: 'keep' | 'remove'): Promise<void>`

Both exported from `api/index.ts`.

### Hooks (`use-sync.ts`)

- `useExternalGames(platform)` — TanStack Query, keyed under `syncKeys`
- `useRematchExternalGame()` — mutation, invalidates the external games query on success
- Existing `useSkipExternalGame` / `useUnskipExternalGame` mutations gain cache invalidation for the external games query

### Component: `ExternalGamesSection`

Added to `components/sync/`, exported from `components/sync/index.ts`.

Mounted in `$platform.tsx` below the Configuration card. Only rendered when the external games array is non-empty (i.e. at least one sync has completed).

Three always-visible collapsible subsections, each showing a count in the header:

**Matched** — `resolved_igdb_id != null && !is_skipped`

Table layout, one game per row, two title columns:

```
│ Storefront Title             │ IGDB Title              │ [Change Match] │
```

**Unmatched** — `resolved_igdb_id == null && !is_skipped`

Single-column list:
```
│ My Weird Game DLC                          [Find Match]  [Skip]  │
```

**Skipped** — `is_skipped == true`

Single-column list:
```
│ Some Launcher Tool                                    [Unskip]   │
```

### IGDB search dialog

Extract the existing IGDB game search UI from the job items `pending_review` flow into a shared `IGDBSearchDialog` component. Both the job items detail view and the new `ExternalGamesSection` use it. The dialog receives an `onSelect(igdbId: number, igdbTitle: string)` callback.

### Orphan warning

Before calling `rematchExternalGame`, if `user_game_other_platform_count === 0`, show a confirmation dialog:

> "This game's only storefront link will be removed. Keep it in your library (unlinked) or remove it from your collection?"

The dialog has two explicit buttons — "Keep" and "Remove" — and is not dismissable without a choice. The user's selection becomes `orphan_action` in the request.

## Error handling

| Scenario | Behaviour |
|---|---|
| Unauthenticated | 401 |
| Invalid storefront | 400 |
| External game not found / not owned | 404 |
| Rematch would orphan, `orphan_action` missing | 409 |
| DB / River failure | 500, generic message |
| Unskip River enqueue fails | Log error, return 204 |
| Frontend mutation failure | Toast error |
| Orphan dialog dismissed without choice | No-op, dialog stays open |

## Testing

New tests in `internal/api/sync_test.go` following the `TestIgnored_*` pattern:

**`TestListExternalGames_*`**
- Empty list when no external games exist
- Returns all states (matched, unmatched, skipped) for the requesting user
- Does not return other users' games
- Invalid storefront returns 400
- `igdb_title` and `user_game_other_platform_count` are populated correctly

**`TestRematchExternalGame_*`**
- 404 for unknown ID
- 404 for game owned by another user
- 409 when rematching would orphan and `orphan_action` is absent
- Successful rematch with `orphan_action: "keep"` — user_game_platform removed, user_game kept, external_game updated, River task enqueued
- Successful rematch with `orphan_action: "remove"` — user_game deleted

**`TestUnskipGame_*`**
- Extend existing test to assert a JobItem row and River task exist after unskip

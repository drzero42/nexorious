# Design: Immediate Metadata Fetch for Newly Synced Games

**Issue:** #622
**Date:** 2026-05-27

---

## Problem

When a sync run adds a new game to the library, the `games` row is created with only an IGDB ID, a title, and timestamps. Rich metadata — description, cover art, genres, release date, developer, publisher, rating — is not populated until the next `MetadataRefreshDispatch` run, which defaults to every 24 hours. A newly synced game can therefore appear in the library without cover art or description for up to a day.

---

## Solution Overview

At the end of Stage 3 (User Game Write), after the `games` row has been ensured and the user's library has been updated, detect whether the `games` row has no metadata yet (`description IS NULL`). If so, and if IGDB is configured, immediately enqueue a lightweight fire-and-forget metadata fetch job for that game. The game will have full metadata and cover art within seconds of the sync completing.

The existing periodic bulk refresh (`MetadataRefreshDispatch`) is unchanged. It remains the catch-all for games added before this fix, and for games whose immediate fetch exhausted all retries.

---

## New Worker: Immediate Metadata Fetch

A new River job kind — `metadata_fetch` — handles a single game with no `jobs`/`job_items` tracking layer. It is fire-and-forget: success is logged at debug level, failures are retried automatically by River (up to 3 attempts with backoff), and exhausted retries are logged at error level but otherwise silent to the user.

**Priority:** 2 — between sync workers (priority 1) and bulk metadata refresh (priority 3). The fetch runs promptly after the sync that triggers it without delaying in-flight sync jobs.

**IGDB guard:** if IGDB is not configured the job exits immediately and is not retried (there is nothing to do).

---

## Shared Fetch Logic

The core operation — call IGDB, update the `games` row, download cover art — is the same whether triggered by the new immediate fetch or by the existing bulk refresh. This logic is extracted from `MetadataRefreshItemWorker` into a shared package-level helper. Both workers call the helper; `MetadataRefreshItemWorker` adds the `job_items` tracking layer on top.

Cover art download is non-fatal in both paths: a failure is logged at warn level but does not fail the job or the job_item.

---

## Enqueue Point

The enqueue happens at the end of `UserGameWorker.Work()` (Stage 3), after the platform loop and before the item is marked completed. This single location covers both code paths:

- **Auto-resolve path:** `IGDBMatchWorker` creates the bare `games` row, then enqueues Stage 3. When Stage 3 runs, `description IS NULL` → fetch enqueued.
- **Manual-resolve path:** `UserGameWorker` creates the bare `games` row itself. After writing platforms, `description IS NULL` → fetch enqueued.

The check is conditional on IGDB being configured. Enqueue failure is non-fatal: logged at warn level, with the periodic bulk refresh as the safety net.

The `description IS NULL` condition is a secondary benefit for games added before this fix: those games are primarily covered by the periodic bulk refresh, but if they are re-synced before the bulk refresh runs they will also receive an immediate fetch at that point.

---

## What Does Not Change

- `MetadataRefreshDispatchWorker` and `MetadataRefreshItemWorker` — behaviour, schedule, and UI tracking are unchanged.
- The `jobs`/`job_items` schema — no new columns or tables.
- Database migrations — none required.
- The sync pipeline Stages 1 and 2 — no changes.

---

## Documentation Changes

### `docs/sync.md`

Two targeted edits:

1. **Stage 3 section** — add a step after the existing platform loop steps:

   > After writing all platform rows, if IGDB is configured and the `games` row has no description, an immediate metadata fetch is enqueued for that game. This ensures newly added games have cover art and full IGDB data within seconds rather than waiting for the next scheduled bulk refresh.

2. **Maintenance section** — replace the current paragraph (which only mentions `sync_changes` pruning) with a brief pointer:

   > Maintenance tasks that support the sync system — sync history pruning, orphaned item rescue, and stale job cleanup — are documented in [docs/maintenance.md](../maintenance.md).

### `docs/maintenance.md`

New file. Documents all periodic maintenance workers as a process reference — what the system does and why, not implementation details. Covers:

- **Sync history pruning** — removes `sync_changes` entries older than the configured retention period (default: 90 days) to keep the sync history table from growing unboundedly.
- **Job pruning** — removes completed, failed, and cancelled jobs (and their associated items) after 30 days.
- **Export cleanup** — removes export files from disk and clears their stored path after 24 hours.
- **Unreferenced game cleanup** — removes `games` catalogue entries that no longer have any user in their library. This can occur when a user removes all copies of a game or after a rematch that leaves an old IGDB entry with no references.
- **Session cleanup** — removes expired login sessions.
- **Stale job cleanup** — detects `metadata_refresh` jobs that are stuck in an active state with no remaining unfinished items (indicating a crash during dispatch) and marks them failed. This releases the duplicate-run guard so the next scheduled dispatch can proceed.
- **Orphaned item rescue** — detects `job_items` stuck in `pending` with no backing River job and re-enqueues them. This is a safety net for items whose River job was lost due to a crash or deployment. Only items older than one hour are considered, to avoid racing freshly-created items.
- **Metadata refresh** — periodically fetches fresh IGDB metadata for every game in the catalogue, ordered by staleness (least recently updated first). Covers description, cover art, genres, release date, developer, publisher, rating, platform names, game modes, themes, player perspectives, and HowLongToBeat times. Schedule is configurable via `METADATA_REFRESH_INTERVAL` (default: 24 hours). Can also be triggered manually by an admin. The immediate per-game fetch (triggered at the end of Stage 3) is the complement: it handles newly added games so they do not wait for the next scheduled window.
- **Scheduled backup** — checks every minute whether a user-configured backup schedule is due. When a backup is due, runs it and applies the configured retention policy.

`CheckPendingSyncs` (the scheduled sync trigger) is already documented in `docs/sync.md` under "Scheduled Sync" and is not duplicated in `docs/maintenance.md`.

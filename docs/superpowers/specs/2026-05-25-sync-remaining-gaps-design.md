# Sync Remaining Gaps Design

**Date:** 2026-05-25
**Branch:** issue-608-normalise-external-games

## Overview

After the previous gap-closure implementation (Tasks 1–11), three spec violations remain in
`internal/api/sync.go`. This document describes all required changes. The spec (`docs/sync.md`)
is authoritative.

---

## G1 — `HandleSkipGame`: mark job_item skipped and call `SyncCheckJobCompletion`

### Problem

`HandleSkipGame` sets `external_game.is_skipped = true` but does not update any `job_items` row.
If the game has a `pending_review` job_item, that item stays in `pending_review` forever.
`SyncCheckJobCompletion` blocks on `pending_review` items, so the job never transitions to
`completed` — it is stuck in `processing` indefinitely.

### Spec requirement

> "The user marks a game as ignored. `is_skipped` is set to `true` on the `external_game`
> **and the job_item is marked `skipped`**. No Stage 3 job is created."

### Fix

After setting `external_game.is_skipped = true`:

1. Query for the most recent `pending_review` or `pending` job_item for this `external_game_id`.
   If none exists (game pre-dates job tracking or items were pruned), skip silently.
2. Set that item's `status = 'skipped'` and `processed_at = now()`.
3. Call `tasks.SyncCheckJobCompletion(ctx, h.db, jobID)` so the job can complete immediately
   if this was the last blocking item.

No River worker fires after a skip action, so the HTTP handler must drive the completion check
directly.

---

## G2 — `HandleRematchExternalGame`: resolve sibling external games

### Problem

`HandleRematchExternalGame` updates only the specific `external_game.resolved_igdb_id` and
enqueues Stage 3 for the specific job_item. Siblings — other `external_games` rows for the same
`(user_id, storefront, title)` — are not resolved.

On PSN, each title ID (PS4 and PS5 variants of the same game) produces a separate
`external_game` row. Without the push mechanic the user must match each variant manually.

### Spec requirement

> "when the user resolves a `pending_review` item, any unresolved siblings with the same title
> are resolved with the same IGDB ID and a Stage 3 job is enqueued for each"

### Fix

After resolving the primary external_game and enqueuing Stage 3 for its job_item:

1. Query siblings: `external_games WHERE user_id = ? AND storefront = ? AND title = ?
   AND id != <primary> AND resolved_igdb_id IS NULL AND is_skipped = false`.
2. For each sibling:
   a. `UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`
   b. Find or create a job_item using the same fallback pattern already in the handler
      (look for `pending_review`, fall back to creating a minimal item on the most recent job).
   c. Enqueue `tasks.UserGameArgs{JobItemID: siblingItemID}`.

Siblings that are already resolved or skipped are excluded from the query — only unresolved,
non-skipped games are pushed.

---

## G3 — `HandleListExternalGames`: exclude in-flight games

### Problem

`HandleListExternalGames` returns all `external_games` for the storefront, including those with
`pending` or `processing` job_items. These games are being processed by Stage 2 or Stage 3.
They get `sync_status = 'unmatched'` in the CASE expression and are filtered out by the
frontend's four-group rendering — but they are still in the JSON payload.

### Spec requirement

> "Games that are currently in-flight (being processed by Stage 2 or Stage 3) do not appear
> in the External Games section — only the counts in the progress box reflect their existence
> until they settle into a stable state."

### Fix

Add to the WHERE clause of the `HandleListExternalGames` query:

```sql
AND NOT EXISTS (
    SELECT 1 FROM job_items ji
    WHERE ji.external_game_id = eg.id
      AND ji.status IN ('pending', 'processing')
)
```

This excludes any external game with an active job_item. Once Stage 2 or Stage 3 finishes
and the item settles into a stable state (`completed`, `skipped`, `pending_review`, `failed`),
the game appears in the section under the appropriate group.

---

## Out of Scope

- The `LEFT JOIN user_game_platforms ugp ON ugp.external_game_id = eg.id` in
  `HandleListExternalGames` can produce duplicate rows for Steam games with multiple platform
  entries. This is a pre-existing query bug unrelated to the three spec gaps above and is
  deferred to a separate fix.

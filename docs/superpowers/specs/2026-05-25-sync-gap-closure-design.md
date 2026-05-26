# Sync Gap Closure Design

**Date:** 2026-05-25
**Branch:** issue-608-normalise-external-games

## Overview

Audit of the sync system against `docs/sync.md` identified gaps across the backend (worker, API) and the React frontend. This document describes all required changes. The spec is authoritative; where code deviates the code must change.

---

## Section 1: Storefront / Platform Terminology Rename

### Problem

The frontend uses "platform" for the concept of a storefront (Steam, GOG, PSN, Epic). The spec consistently uses:

- **storefront** — Steam, GOG, PSN, Epic Games Store (a game store)
- **platform** — pc-windows, mac, playstation-4, playstation-5 (a gaming platform / OS)

`docs/sync.md` also contains one terminology error: line 379 says "The **platform** name and a connection status badge" when it means "storefront name".

### Changes

**`docs/sync.md`**

- Line 379: "The platform name and a connection status badge" → "The storefront name and a connection status badge."

**Frontend TypeScript** — rename the "which store" concept everywhere:

| Before | After |
|--------|-------|
| `SyncPlatform` enum | `SyncStorefront` |
| `SUPPORTED_SYNC_PLATFORMS` | `SUPPORTED_SYNC_STOREFRONTS` |
| `getPlatformDisplayInfo` | `getStorefrontDisplayInfo` |
| `platform` parameter / variable (when referring to a store) | `storefront` |
| Route file `$platform.tsx` | `$storefront.tsx` |
| URL segment `/sync/$platform` | `/sync/$storefront` |

The rename does **not** touch identifiers that correctly refer to gaming platforms (pc-windows, mac, etc.), such as `external_game_platforms`, `platforms: []string`, or `user_game_platforms`.

---

## Section 2: Credentials Error Connection State

### Problem

The API returns a `credentials_error` flag in sync status responses. Neither the hub card nor the detail page header surfaces a "Credentials Error" state — they only show two states instead of the spec's three.

### Changes

**Hub card (`sync-service-card.tsx`)**

Add a third connection badge variant. Priority order:
1. `credentials_error` set → destructive "Credentials Error" badge
2. connection configured → "Connected" badge
3. otherwise → "Not Configured" badge

**Detail page header (`$storefront.tsx`)**

Replace the current "Configured" / "Not Configured" badge with the full three-state badge: **Connected** / **Credentials Error** / **Not Configured**. Clicking the badge toggles the Connection & Settings section open/closed (verify existing toggle wiring; add if absent). The Connection & Settings section must default to expanded when state is "Credentials Error" or "Not Configured", and collapsed when "Connected".

---

## Section 3: Hub Page Sync Now Behaviour + Pending Review Badge

### Problem F2: Sync Now navigates away

After triggering sync, `sync/index.tsx` calls `navigate({ to: '/sync/$storefront' })`. The spec says the button triggers sync without navigating.

### Fix

Remove the `navigate` call after the mutation. The hub card already holds status state and will reflect the active job in-place.

### Problem F3: Pending review badge is not interactive

The pending review count badge on the hub card is a plain `<Badge>` with no click handler. The spec says clicking it navigates to that storefront's detail page, anchoring to the Needs Review section.

### Fix

- Wrap the badge in a `<Link to="/sync/$storefront" hash="needs-review">`.
- Add `id="needs-review"` to the Needs Review section on the detail page so the browser scrolls to it on arrival.

---

## Section 4: ExternalGamesSection — Four Groups

### Problem

The current component classifies external games on `resolved_igdb_id` and `is_skipped` only. This collapses the spec's "Needs Review" and "Failed" groups into a single "Unmatched" bucket, and provides no retry actions.

### Spec-required groups

| Group | Condition | Default | Actions |
|-------|-----------|---------|---------|
| Needs Review | `pending_review` job item exists for this game | Expanded | Pick IGDB match, Skip |
| Failed | Permanent Stage 2 or Stage 3 failure; no active job | Expanded | Retry (per game), Retry All |
| Matched | `resolved_igdb_id` set, Stage 3 complete | Collapsed | Change match |
| Skipped | `is_skipped = true` | Collapsed | Unskip |

### Changes

**API / type** — `GET /api/sync/:storefront/external-games` must return enough state to classify each game into one of the four groups. The `ExternalGame` type needs a `status` field (or equivalent flags: `has_pending_review`, `has_failed`) that the frontend can use without re-fetching job items.

**`external-games-section.tsx`**

- Replace the `unmatched` bucket with two separate buckets: `needsReview` and `failed`, using the new status field.
- Needs Review group: expanded by default; "Find Match" and "Skip" actions (same as current).
- Failed group: expanded by default; per-game "Retry" button and a "Retry All" button in the card header; wire to the existing retry endpoints.
- Rename "Unmatched" label → "Needs Review".
- Matched and Skipped groups: no change to behaviour; collapsed by default.

---

## Section 5: RecentActivity — Sync Changes Changelog

### Problem F8: Missing removed / status-changed entries

The component only shows job_item outcomes. The spec requires the changelog to also include games removed from the storefront and games whose ownership status changed — both sourced from the `sync_changes` table.

### Problem F9: No outcome badge

Job cards show only timestamp + item count. The spec requires a clear "Completed" / "Failed" outcome indicator.

### Changes

**API** — the recent jobs endpoint (or a companion) must include `sync_changes` rows (type `removed` and `status_changed`) grouped by job ID. Check whether the existing `GET /api/jobs/recent` response already carries this data; add it if not.

**`recent-activity.tsx`**

- Add two new `ItemsList` variants rendered within each `JobCard`:
  - **Removed** (type `removed`): games that became unavailable in this sync run.
  - **Status Changed** (type `status_changed`): games whose ownership status changed.
- Add a job outcome badge to the `JobCard` header: green "Completed" or red "Failed" based on job status, displayed alongside the timestamp.

---

## Section 6: Backend Code Fixes (B1–B4)

### B1 — `PlaytimeHours` type: `float64` → `int`

The spec records playtime in whole hours only. Change:
- `ExternalGameEntry.PlaytimeHours` in `storefrontadapter/storefrontadapter.go`: `float64` → `int`
- All four adapter mappings (Steam, PSN, GOG, Epic): cast/truncate to `int` when assigning
- Any worker code that reads `PlaytimeHours` and writes to `external_game_platforms.hours_played`

### B2 — Stage 1: enqueue Stage 2 after each batch

`DispatchSyncWorker` currently accumulates all Stage 2 jobs and enqueues them in one bulk call after all batches complete. The spec requires Stage 2 jobs to be enqueued after each batch. Move the enqueue call into (or immediately after) the per-batch callback.

### B3 — Remove `completed_with_errors`

`completed_with_errors` was removed from the design. A job either `completed` or `failed`. Find all usages in the codebase (worker, API handlers, frontend types, tests) and replace with the correct terminal status.

### B4 — `HandleRematchExternalGame`: direct Stage 3 enqueue

The handler currently creates a mini-job + job_item before enqueueing Stage 3. The spec requires enqueueing Stage 3 directly. Remove the mini-job creation and enqueue `UserGameArgs` directly, as the spec describes.

---

## Out of Scope

The following deviations were reviewed and accepted as-is (no code or spec change needed):

- None. All identified gaps require either a code fix or a spec fix.

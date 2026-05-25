# Sync UI Spec Compliance Design

**Date:** 2026-05-25
**Branch:** issue-608-normalise-external-games

## Overview

After the backend gap-closure work (G1–G3 in `2026-05-25-sync-remaining-gaps-design.md`), the
frontend UI diverges from `docs/sync.md` in five places. This document describes each gap and
the required changes. The spec is authoritative.

---

## Gap Map

| ID | Area | Backend change? | Frontend change? |
|----|------|-----------------|-----------------|
| G-F1 | Sync Hub cards overloaded | No | Yes |
| G-F2 | Connection & Settings restructure + auto_add removal | Yes (migration + API) | Yes |
| G-F3 | Progress Box missing counts | No | Yes |
| G-F4 | Sync History format wrong | Yes (API response) | Yes |
| G-F5 | External Games section order | No | Yes |

---

## G-F1 — Sync Hub cards are overloaded

### Problem

`SyncServiceCard` (used exclusively on the hub page) renders a sync-frequency selector, an
auto-add toggle, and a reset button. The spec says hub cards show only: platform name/icon,
connection status, last-synced timestamp, pending-review count badge, and a Sync Now button.

### Fix

Remove the following from `SyncServiceCard`:
- Sync frequency `<Select>`
- Auto-add `<Switch>` (removed everywhere per G-F2)
- Reset `<AlertDialog>` + trigger button

Keep: platform icon, name, status badge, last-synced line, pending-review badge, Sync Now
button, and the "View details" footer link.

Remove props no longer needed: `onUpdate`, `isUpdating`, `isResetting`, `onReset`.

**Files:** `ui/frontend/src/components/sync/sync-service-card.tsx`, `ui/frontend/src/routes/_authenticated/sync/index.tsx`

---

## G-F2 — Connection & Settings restructure + auto_add removal

### Problem

Two issues:

1. Sync frequency is in a standalone "Configuration" card *outside* the Connection & Settings
   collapsible. The spec says Connection & Settings contains: credentials, sync frequency,
   disconnect.

2. `auto_add` is not in the spec. It exists in the database, backend API, and frontend. It must
   be removed from all three.

### Fix

**Database migration:**
- Edit `internal/db/migrations/20260503000001_initial.up.sql`: remove the `auto_add` column line from the `user_sync_configs` `CREATE TABLE` statement (line 210: `auto_add BOOLEAN NOT NULL DEFAULT false`)
- Edit `internal/db/migrations/20260503000001_initial.down.sql`: no change needed (the down migration drops the table entirely)

**Backend:**
- Remove `auto_add` from `HandleGetSyncConfig`, `HandleListSyncConfigs`, and
  `HandleUpdateSyncConfig` in `internal/api/sync.go`
- Remove `AutoAdd` field from the sync config model/struct

**Frontend:**
- Remove `autoAdd` from `SyncConfig` interface (`types/sync.ts`)
- Remove `autoAdd` from `SyncConfigUpdateData` interface
- Remove `auto_add` from `transformSyncConfig` and `updateSyncConfig` in `api/sync.ts`
- In `$storefront.tsx`: delete the "Configuration" card; move the frequency `<Select>` into
  `CollapsibleContent`, after the per-storefront connection card, as a compact row
- Remove all `autoAdd` / `localAutoAdd` state and handlers from `$storefront.tsx`
- Remove `handleAutoAddChange` and related logic

**Structure of the updated collapsible in `$storefront.tsx`:**
```tsx
<CollapsibleContent className="space-y-4">
  {/* per-storefront credential card */}
  {storefront === SyncStorefront.STEAM && <SteamConnectionCard ... />}
  ...

  {/* Sync frequency — always shown when configured */}
  {config.isConfigured && (
    <div className="flex items-center justify-between px-1">
      <div>
        <div className="font-medium">Sync Frequency</div>
        <div className="text-sm text-muted-foreground">How often to automatically sync</div>
      </div>
      <Select value={effectiveFrequency} onValueChange={handleFrequencyChange} ...>
        ...
      </Select>
    </div>
  )}
</CollapsibleContent>
```

**Files:** `internal/db/migrations/20260503000001_initial.up.sql`, `internal/api/sync.go`, `internal/api/sync_test.go`, `internal/db/models/models.go`, `internal/scheduler/cleanup_test.go`, `ui/frontend/src/types/sync.ts`, `ui/frontend/src/api/sync.ts`, `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`

---

## G-F3 — Progress Box missing counts

### Problem

The spec says the progress box shows live counts for: **matched, needs review, skipped, failed,
still processing**. `JobProgressCard` currently shows: completed, failed, processing, pending,
igdb_failed. `pendingReview` and `skipped` are absent; "completed" is not labelled "matched".

### Fix

Update the stat grid in `JobProgressCard`:

| Spec label | Value |
|-----------|-------|
| Matched | `job.progress.completed` |
| Needs Review | `job.progress.pendingReview` |
| Skipped | `job.progress.skipped` |
| Failed | `job.progress.failed` |
| Processing | `job.progress.pending + job.progress.processing` |

"IGDB Error" remains as an optional extra column (not in spec, but useful and already present).

The `JobProgress` type already has `pendingReview` and `skipped` fields — this is a display-only
change.

**Files:** `ui/frontend/src/components/jobs/job-progress-card.tsx`

---

## G-F4 — Sync History format wrong

### Problem

`RecentActivity` reproduces the full per-game processing trace (completed/skipped/failed/igdb_failed
item lists). The spec says: "The history does not reproduce the full per-game processing trace —
it is a human-readable record of what changed in the user's library."

Additionally, the "Added to library" sync_changes category is not present in the API response or
the UI.

### Fix

**Backend — `GET /api/jobs/recent-jobs?source=<storefront>`:**

In `internal/api/jobs.go` (or wherever `HandleListRecentJobs` lives), update the response:

- Add `added_items []SyncChangeItem` — populated from `sync_changes WHERE job_id = ? AND change_type = 'added'`
- Remove `completed_items`, `skipped_items`, `failed_items`, `igdb_failed_items` arrays from the
  response body (the per-game lists)
- Keep the summary counts: `completed_count`, `skipped_count`, `failed_count`,
  `igdb_failed_count` (used for the one-line summary)

**Frontend — `types/jobs.ts`:**

```typescript
export interface RecentJobDetail {
  id: string;
  status: string;
  createdAt: string;
  completedAt: string | null;
  totalItems: number;
  completedCount: number;
  skippedCount: number;
  failedCount: number;
  igdbFailedCount: number;
  // per-game lists removed
  addedItems: SyncChangeItem[];       // NEW — from sync_changes added
  removedItems: SyncChangeItem[];
  statusChangedItems: SyncChangeItem[];
}
```

**Frontend — `components/sync/recent-activity.tsx`:**

- Delete `ItemsList` component and its four call sites
- Delete the per-job "Retry IGDB errors" button (belongs on the live progress box, not history)
- Add `SyncChangeList` entry for `addedItems` with a "Added to library" label
- Show a one-line summary string next to the timestamp badge:
  `"42 matched · 3 skipped · 1 failed"` (only include non-zero counts)

**Visual structure of a history entry:**
```
▶ 2026-05-25 14:32   ✅ Completed   42 matched · 3 skipped · 1 failed
    ▸ Added to library (12)
    ▸ Removed from storefront (2)
    ▸ Status changed (1)
```

**Files:** `internal/api/jobs.go`, `ui/frontend/src/types/jobs.ts`, `ui/frontend/src/components/sync/recent-activity.tsx`

---

## G-F5 — External Games section order

### Problem

`ExternalGamesSection` renders: Needs Review → Failed → **Skipped → Matched**. The spec table
order is: Needs Review → Failed → **Matched → Skipped**.

### Fix

In `ExternalGamesSection`, swap the Matched and Skipped `<Collapsible>` blocks so Matched
renders before Skipped.

**Files:** `ui/frontend/src/components/sync/external-games-section.tsx`

---

## Out of Scope

- **G-F6 (IGDB candidates in needs_review dialog):** Deferred. Requires a backend change to
  include `igdb_candidates` in the external games response for `needs_review` items, plus a
  frontend dialog update. Not addressed in this branch.
- **`auto_add` backend column removal from the Nix hash:** After the migration, update
  `vendorHash`/`npmDepsHash` in `nix/package.nix` / `nix/frontend.nix` if affected (Go vendor
  changes only affect `vendorHash`; this migration is SQL-only so no hash update is needed).
- **Duplicate rows in `HandleListExternalGames`** (Steam multi-platform): pre-existing bug noted
  in the previous design spec; deferred to a separate fix.

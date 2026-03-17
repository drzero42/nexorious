# Design: Remove "Coming Soon" Placeholder Messages

**Date:** 2026-03-17
**Priority:** High
**Scope:** Frontend only

## Problem

The admin Maintenance page contains a "Database Cleanup" card with two disabled buttons labelled "Coming Soon":
- **Orphaned Files** — Remove cover art not linked to any game
- **Expired Jobs** — Clean up job data older than 7 days

These placeholders provide no value to the user and clutter the UI with unfulfilled promises.

## Decision

Remove the entire "Database Cleanup" card from the Maintenance page. The actual cleanup functionality is tracked separately as a `Medium` priority roadmap item ("Maintenance job for orphaned file cleanup") and will be implemented in a future session with proper backend support.

## Change

**File:** `frontend/src/routes/_authenticated/admin/maintenance.tsx`

1. Delete the "Database Cleanup" `<Card>` block (~45 lines).
2. Remove the `lg:grid-cols-2` grid wrapper that previously held two cards side by side. Render the remaining "Seed Data" card full-width instead — a single card in a 2-column grid looks unintentional.
3. Remove the `Trash2` import from `lucide-react` — it is used only in the Database Cleanup card header and will become unused.
4. Update `MaintenancePageSkeleton`: remove the `lg:grid-cols-2` grid wrapper and the second `<Skeleton className="h-64" />` placeholder so the loading state stays consistent with the live layout.

## Out of Scope

- No backend changes.
- No API changes.
- No new tests required (the deleted UI has no test coverage).
- Implementation of orphaned file / expired job cleanup is a separate task.

## Alternatives Considered

- **Keep the card shell, remove only the rows:** Leaves an empty card — worse UX.
- **Hide with CSS/conditional:** Leaves dead code.

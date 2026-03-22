# List View Time to Beat — Design Spec

**Date:** 2026-03-22
**Status:** Approved

## Problem

The games library list view does not show Time to Beat (HowLongToBeat) estimates. The card view shows all three TTB values (Main / Extra / Completionist), but the list view table has no TTB column, making it impossible to compare completion times in list mode.

## Goal

Add a "Time to Beat" column to the list view that matches the card view's existing TTB display: `main / extra / completionist` (e.g. `10h / 20h / 30h`), shown only when at least one value is non-null.

## Design

### 1. Extract `formatTtb` to a shared utility

Create `frontend/src/lib/game-utils.ts` containing:

```ts
export function formatTtb(hours: number | null | undefined): string {
  return hours != null ? `${hours}h` : '—';
}
```

Update `frontend/src/components/games/game-card.tsx` to remove its local `formatTtb` definition and import from `@/lib/game-utils` instead.

### 2. Add Time to Beat column to list view

In `frontend/src/components/games/game-list.tsx`:

- Import `Timer` from `lucide-react` and `formatTtb` from `@/lib/game-utils`
- Add `<TableHead className="w-32">Time to Beat</TableHead>` between Hours and Rating columns
- Add a `<TableCell>` in the row renderer:
  - If all three TTB fields are null: render `<span className="text-sm text-muted-foreground">—</span>`
  - Otherwise: render `<div className="flex items-center gap-1 text-xs text-muted-foreground"><Timer className="h-3 w-3" /><span>{formatTtb(main)} / {formatTtb(extra)} / {formatTtb(completionist)}</span></div>`
- Update `GameListSkeleton` to add a cell for the new column, growing from 7 cells to 8: `<TableCell><Skeleton className="h-4 w-20" /></TableCell>`

### 3. Tests

`frontend/src/components/games/game-list.test.tsx` does not currently exist — it must be **created**. It should cover:
- Baseline: existing columns render correctly (title, status, hours, rating, platform)
- TTB data present: new column renders `Timer` icon and formatted values (e.g. `10h / 20h / 30h`)
- TTB data absent (all three null): new column renders `—`
- Skeleton: each skeleton row renders 8 cells

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/lib/game-utils.ts` | **New** — shared `formatTtb` utility |
| `frontend/src/components/games/game-card.tsx` | Remove local `formatTtb`, import from `@/lib/game-utils` |
| `frontend/src/components/games/game-list.tsx` | Add Time to Beat column (header, cell, skeleton) |
| `frontend/src/components/games/game-list.test.tsx` | Add/update TTB column tests |

## Out of Scope

- Backend changes (TTB data already in the `UserGame` type)
- Sorting by TTB in list view (separate feature)
- Detail page changes
- `getCoverUrl` duplication — this function is identically defined in both `game-card.tsx` and `game-list.tsx` (pre-existing issue). Consolidating it into `game-utils.ts` is out of scope for this change.

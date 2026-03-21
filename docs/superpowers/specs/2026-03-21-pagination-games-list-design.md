# Pagination for Games List — Design Spec

**Date:** 2026-03-21
**Status:** Approved

---

## Problem

When a filter is active on the My Games list, pagination controls are absent. Only the first page of results (hardcoded to 50) is shown, even though the total game count (e.g. 1060) is displayed at the top. Pagination should be visible and functional at all times, with and without filters.

---

## Context

The backend API (`GET /user-games/`) already supports server-side pagination via `page` and `per_page` query params and returns full pagination metadata: `total`, `page`, `per_page`, `pages`. The frontend API client (`buildUserGamesQueryParams`, `UserGamesListResponse`) already handles these fields. The existing shadcn/ui `Pagination` component exists at `frontend/src/components/ui/pagination.tsx`. The bug is entirely in the games list page: `page` is never sent to the API and no pagination UI is rendered.

The route uses `useSearch({ strict: false })` with a raw `as Record<string, string>` cast — no `validateSearch` schema is defined for this route and none will be added as part of this change.

---

## URL State

Two new URL search params are added alongside the existing `sort`, `order`, `view` params:

| Param | Type | Default | Omit when default? |
|-------|------|---------|--------|
| `page` | integer | `1` | Yes — omit `page` when it equals `1` (like `view` omits `'grid'`) |
| `perPage` | integer | `50` | Yes — omit `perPage` when it equals `50` |

Omitting default values keeps URLs clean (e.g. no `?page=1&perPage=50` on first load).

Both params are managed via `updateParams` → `navigate({ to: '/games', search: params, replace: true })`, consistent with existing sort/filter params.

---

## Component Design

### New: `GamesPagination`

**File:** `frontend/src/components/games/game-pagination.tsx`

**Props:**
```ts
interface GamesPaginationProps {
  page: number;
  perPage: number;
  totalPages: number;
  totalCount: number;
  onPageChange: (page: number) => void;
  onPerPageChange: (perPage: number) => void;
  showPerPageSelector: boolean;
}
```

**Behaviour:**
- Hidden entirely when `totalPages <= 1`.
- Renders prev/next buttons and numbered page buttons with ellipsis for large page counts, using the existing shadcn `Pagination`, `PaginationContent`, `PaginationItem`, `PaginationLink`, `PaginationPrevious`, `PaginationNext`, `PaginationEllipsis` components.
- When `showPerPageSelector` is `true`, renders a shadcn `<Select>` with options 25 / 50 / 100 / 500 before the page navigation controls. The `showPerPageSelector` prop exists because the per-page selector always appears at the top and never at the bottom — this is intentional and not expected to change.

**Placement in `GamesPageContent`:**

```
[Header: "Game Library" + count + Add Game button]
[GameFilters bar]
[GamesPagination — top, showPerPageSelector=true]
[BulkActions]
[GameGrid or GameList]
[GamesPagination — bottom, showPerPageSelector=false]
```

---

## Data Flow

### Reading URL params

```ts
// Read raw string values (data may still be loading — don't clamp against totalPages here)
const rawPage = parseInt(s['page'] ?? '1', 10);
const currentPage = isNaN(rawPage) || rawPage < 1 ? 1 : rawPage;
const currentPerPage = parsePerPage(s['perPage'] ?? '50'); // validates against [25,50,100,500], defaults to 50
```

Clamping against `totalPages` is **not** done at read time because `data` is undefined during the initial loading state. Clamping before data loads would reset the URL to page 1 immediately, losing the desired page. Instead, the out-of-range case is handled via an effect after data resolves (see Edge Cases).

### `queryParams` for game list vs. bulk-ID fetch

Two separate param objects are used:

```ts
// Used by useUserGames (paginated list)
const listQueryParams = useMemo(() => ({
  ...filterFields,
  page: currentPage,
  perPage: currentPerPage,
  sortBy,
  sortOrder,
}), [...]);

// Used by useUserGameIds (all matching IDs for bulk select — no pagination)
const idsQueryParams = useMemo(() => ({
  ...filterFields,
  sortBy,
  sortOrder,
  // No page/perPage — IDs endpoint returns all matching IDs
}), [...]);
```

This ensures "Select all" in bulk-actions fetches IDs across all pages, not just the current page.

### Filter reset on change

All handlers that modify the result set gain a `page: undefined` reset (which removes `page` from the URL, falling back to default 1):

- `handleFiltersChange` — resets page to default (removes from URL)
- `handleSortByChange` — resets page to default
- `handleSortOrderToggle` — resets page to default
- `handlePerPageChange` (new) — resets page to default, sets new perPage (omits perPage if 50)
- `handlePageChange` (new) — clears selection (see below), sets new page (omits if 1)

### Selection cleared on page change

`handlePageChange` clears `selectedIds` and resets `selectionMode` to `'manual'`, consistent with how `handleFiltersChange` clears selection. Items from a previous page should not remain selected when navigating to a different page.

### Handlers (new/updated)

```ts
const handlePageChange = useCallback((page: number) => {
  updateParams({ page: page === 1 ? undefined : String(page) });
  setSelectedIds(new Set());
  setSelectionMode('manual');
}, [updateParams]);

const handlePerPageChange = useCallback((perPage: number) => {
  updateParams({
    perPage: perPage === 50 ? undefined : String(perPage),
    page: undefined, // reset to page 1
  });
}, [updateParams]);
```

---

## Edge Cases

| Scenario | Handling |
|----------|----------|
| `page` in URL exceeds `totalPages` after data loads | `useEffect` watching `[data, currentPage]`: if `data` is defined and `currentPage > data.pages`, call `updateParams({ page: undefined })` to reset to page 1 |
| Non-numeric `page` in URL | `parseInt` returns `NaN`, falls back to `1` |
| `perPage` value not in `[25, 50, 100, 500]` | `parsePerPage` falls back to `50` |
| `totalPages <= 1` | Both pagination bars hidden entirely |
| Page navigation while items are selected | `handlePageChange` clears selection |
| `useUserGameIds` with pagination params | Uses `idsQueryParams` (no `page`/`perPage`) — returns all matching IDs |

---

## Testing

### New tests: `game-pagination.test.tsx`

- Renders page buttons, prev/next correctly
- Hides when `totalPages <= 1`
- Shows per-page selector only when `showPerPageSelector={true}`
- Fires `onPageChange` when a page button is clicked
- Fires `onPerPageChange` when per-page selector changes
- Renders ellipsis for large page counts

### Existing tests: `games/index.tsx` (if any)

- Update to account for `page` and `perPage` URL params

---

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/components/games/game-pagination.tsx` | New component |
| `frontend/src/components/games/index.ts` | Export `GamesPagination` |
| `frontend/src/routes/_authenticated/games/index.tsx` | Read `page`/`perPage` from URL, split into `listQueryParams`/`idsQueryParams`, render `GamesPagination` top and bottom, add reset-to-page-1 on filter/sort/page change, add out-of-range page reset effect |
| `frontend/src/components/games/game-pagination.test.tsx` | New tests |

No backend changes required.

---

## Out of Scope

- Configurable default page size (YAGNI)
- "Jump to page" number input
- Remembering per-page preference outside of URL (e.g. localStorage)
- `validateSearch` schema for the games route

# Pagination for Games List — Design Spec

**Date:** 2026-03-21
**Status:** Approved

---

## Problem

When a filter is active on the My Games list, pagination controls are absent. Only the first page of results (hardcoded to 50) is shown, even though the total game count (e.g. 1060) is displayed at the top. Pagination should be visible and functional at all times, with and without filters.

---

## Context

The backend API (`GET /user-games/`) already supports server-side pagination via `page` and `per_page` query params and returns full pagination metadata: `total`, `page`, `per_page`, `pages`. The frontend API client (`buildUserGamesQueryParams`, `UserGamesListResponse`) already handles these fields. The existing shadcn/ui `Pagination` component exists at `frontend/src/components/ui/pagination.tsx`. The bug is entirely in the games list page: `page` is never sent to the API and no pagination UI is rendered.

---

## URL State

Two new URL search params are added alongside the existing `sort`, `order`, `view` params:

| Param | Type | Default | Values |
|-------|------|---------|--------|
| `page` | integer | `1` | `1..totalPages` |
| `perPage` | integer | `50` | `25 \| 50 \| 100 \| 500` |

Both params are managed identically to existing sort/filter params (read via `useSearch`, written via `updateParams`, serialized as strings in the URL).

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
- When `showPerPageSelector` is `true`, renders a shadcn `<Select>` with options 25 / 50 / 100 / 500 before the page navigation controls.

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
const currentPage = clampPage(parseInt(s['page'] ?? '1', 10), 1, data?.pages ?? 1);
const currentPerPage = parsePerPage(s['perPage'] ?? '50'); // validates against [25,50,100,500], defaults to 50
```

Both are parsed/validated at read time. Invalid values fall back to defaults silently.

### `queryParams` additions

```ts
{
  // ...existing fields...
  page: currentPage,
  perPage: currentPerPage,
}
```

### Filter reset on change

All filter/sort change handlers that modify the result set gain a `page: '1'` reset:

- `handleFiltersChange` — resets page to 1
- `handleSortByChange` — resets page to 1
- `handleSortOrderToggle` — resets page to 1
- `handlePerPageChange` (new) — resets page to 1, sets new perPage

### Handlers (new/updated)

```ts
const handlePageChange = useCallback((page: number) => {
  updateParams({ page: String(page) });
}, [updateParams]);

const handlePerPageChange = useCallback((perPage: number) => {
  updateParams({ perPage: String(perPage), page: '1' });
}, [updateParams]);
```

---

## Edge Cases

| Scenario | Handling |
|----------|----------|
| `page` in URL exceeds `totalPages` after filters narrow results | Detect `items.length === 0 && currentPage > 1`, auto-reset to page 1 via `updateParams` |
| Non-numeric or out-of-range `page` in URL | Parsed with `parseInt`, clamped to `[1, totalPages]`, defaults to 1 |
| `perPage` value not in `[25, 50, 100, 500]` | Falls back to 50 |
| `totalPages <= 1` | Both pagination bars hidden |

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

- Update to account for `page` and `perPage` URL params being present

---

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/components/games/game-pagination.tsx` | New component |
| `frontend/src/components/games/index.ts` | Export `GamesPagination` |
| `frontend/src/routes/_authenticated/games/index.tsx` | Read `page`/`perPage` from URL, pass to `queryParams`, render `GamesPagination` top and bottom, add reset-to-page-1 on filter/sort change |
| `frontend/src/components/games/game-pagination.test.tsx` | New tests |

No backend changes required.

---

## Out of Scope

- Configurable default page size (YAGNI)
- "Jump to page" number input
- Remembering per-page preference outside of URL (e.g. localStorage)

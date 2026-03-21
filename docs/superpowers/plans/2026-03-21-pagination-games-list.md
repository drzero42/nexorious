# Pagination for Games List — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add pagination controls (prev/next/page numbers + per-page selector) to the My Games list so users can navigate beyond the first 50 results, with and without filters active.

**Architecture:** A new `GamesPagination` component wraps the existing shadcn `Pagination` components and a shadcn `Select` for per-page choice. `GamesPageContent` reads `page`/`perPage` from the URL, passes them to the API, and renders the component above and below the game grid. No backend changes required — the API already supports `page`/`per_page` params and returns `pages`/`total`.

**Tech Stack:** React 19, TypeScript, TanStack Router (`useSearch` + `navigate`), TanStack Query, Vitest, @testing-library/react, shadcn/ui Pagination + Select components.

**Spec:** `docs/superpowers/specs/2026-03-21-pagination-games-list-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `frontend/src/components/games/game-pagination.tsx` | **Create** | Pure UI component: page buttons, prev/next, per-page selector |
| `frontend/src/components/games/game-pagination.test.tsx` | **Create** | Tests for `GamesPagination` |
| `frontend/src/components/games/index.ts` | **Modify** | Export `GamesPagination` |
| `frontend/src/routes/_authenticated/games/index.tsx` | **Modify** | URL state, split query params, handlers, render pagination |

---

## Task 0: Create feature branch

- [ ] **Step 0.1: Create and switch to a feature branch**

```bash
git checkout -b feat/games-list-pagination
```

Expected: Branch `feat/games-list-pagination` created and checked out.

---

## Task 1: Write failing tests for `GamesPagination`

**Files:**
- Create: `frontend/src/components/games/game-pagination.test.tsx`

- [ ] **Step 1.1: Create the test file**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GamesPagination } from './game-pagination';

const defaultProps = {
  page: 1,
  perPage: 50,
  totalPages: 5,
  totalCount: 220,
  onPageChange: vi.fn(),
  onPerPageChange: vi.fn(),
  showPerPageSelector: false,
};

describe('GamesPagination', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('visibility', () => {
    it('renders nothing when totalPages is 1', () => {
      const { container } = render(
        <GamesPagination {...defaultProps} totalPages={1} totalCount={10} />
      );
      expect(container.firstChild).toBeNull();
    });

    it('renders nothing when totalPages is 0', () => {
      const { container } = render(
        <GamesPagination {...defaultProps} totalPages={0} totalCount={0} />
      );
      expect(container.firstChild).toBeNull();
    });

    it('renders when totalPages is 2', () => {
      render(<GamesPagination {...defaultProps} totalPages={2} />);
      expect(screen.getByRole('navigation', { name: /pagination/i })).toBeInTheDocument();
    });
  });

  describe('per-page selector', () => {
    it('does not render per-page selector when showPerPageSelector is false', () => {
      render(<GamesPagination {...defaultProps} showPerPageSelector={false} />);
      expect(screen.queryByText('Per page')).not.toBeInTheDocument();
    });

    it('renders per-page selector when showPerPageSelector is true', () => {
      render(<GamesPagination {...defaultProps} showPerPageSelector={true} perPage={50} />);
      expect(screen.getByText('Per page')).toBeInTheDocument();
    });

    it('shows all four per-page options', async () => {
      const user = userEvent.setup();
      render(<GamesPagination {...defaultProps} showPerPageSelector={true} perPage={50} />);
      await user.click(screen.getByRole('combobox'));
      expect(screen.getByRole('option', { name: '25' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: '50' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: '100' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: '500' })).toBeInTheDocument();
    });

    it('calls onPerPageChange with number when option is selected', async () => {
      const user = userEvent.setup();
      const onPerPageChange = vi.fn();
      render(
        <GamesPagination
          {...defaultProps}
          showPerPageSelector={true}
          perPage={50}
          onPerPageChange={onPerPageChange}
        />
      );
      await user.click(screen.getByRole('combobox'));
      await user.click(screen.getByRole('option', { name: '100' }));
      expect(onPerPageChange).toHaveBeenCalledWith(100);
      expect(onPerPageChange).toHaveBeenCalledTimes(1);
    });
  });

  describe('page navigation', () => {
    it('renders previous and next buttons', () => {
      render(<GamesPagination {...defaultProps} page={3} />);
      expect(screen.getByRole('link', { name: /previous/i })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: /next/i })).toBeInTheDocument();
    });

    it('previous button is aria-disabled on page 1', () => {
      render(<GamesPagination {...defaultProps} page={1} />);
      const prev = screen.getByRole('link', { name: /previous/i });
      expect(prev).toHaveAttribute('aria-disabled', 'true');
    });

    it('next button is aria-disabled on last page', () => {
      render(<GamesPagination {...defaultProps} page={5} totalPages={5} />);
      const next = screen.getByRole('link', { name: /next/i });
      expect(next).toHaveAttribute('aria-disabled', 'true');
    });

    it('clicking previous calls onPageChange with page - 1', async () => {
      const user = userEvent.setup();
      const onPageChange = vi.fn();
      render(
        <GamesPagination {...defaultProps} page={3} onPageChange={onPageChange} />
      );
      await user.click(screen.getByRole('link', { name: /previous/i }));
      expect(onPageChange).toHaveBeenCalledWith(2);
    });

    it('clicking next calls onPageChange with page + 1', async () => {
      const user = userEvent.setup();
      const onPageChange = vi.fn();
      render(
        <GamesPagination {...defaultProps} page={3} totalPages={5} onPageChange={onPageChange} />
      );
      await user.click(screen.getByRole('link', { name: /next/i }));
      expect(onPageChange).toHaveBeenCalledWith(4);
    });

    it('clicking a page number calls onPageChange with that page', async () => {
      const user = userEvent.setup();
      const onPageChange = vi.fn();
      render(
        <GamesPagination {...defaultProps} page={1} totalPages={5} onPageChange={onPageChange} />
      );
      await user.click(screen.getByRole('link', { name: '3' }));
      expect(onPageChange).toHaveBeenCalledWith(3);
    });

    it('active page link has aria-current="page"', () => {
      render(<GamesPagination {...defaultProps} page={3} totalPages={5} />);
      const activeLink = screen.getByRole('link', { name: '3' });
      expect(activeLink).toHaveAttribute('aria-current', 'page');
    });
  });

  describe('page range with small page counts', () => {
    it('renders all pages when totalPages is 7 or fewer', () => {
      render(<GamesPagination {...defaultProps} page={1} totalPages={7} />);
      for (let i = 1; i <= 7; i++) {
        expect(screen.getByRole('link', { name: String(i) })).toBeInTheDocument();
      }
    });
  });

  describe('page range with ellipsis', () => {
    it('renders ellipsis for large page counts', () => {
      render(<GamesPagination {...defaultProps} page={5} totalPages={20} />);
      expect(screen.getByRole('link', { name: '1' })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: '20' })).toBeInTheDocument();
      const ellipses = screen.getAllByText('More pages');
      expect(ellipses.length).toBeGreaterThanOrEqual(1);
    });

    it('always renders first and last page', () => {
      render(<GamesPagination {...defaultProps} page={10} totalPages={20} />);
      expect(screen.getByRole('link', { name: '1' })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: '20' })).toBeInTheDocument();
    });

    it('renders current page and neighbours', () => {
      render(<GamesPagination {...defaultProps} page={10} totalPages={20} />);
      expect(screen.getByRole('link', { name: '9' })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: '10' })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: '11' })).toBeInTheDocument();
    });
  });
});
```

- [ ] **Step 1.2: Run the tests and verify they fail**

```bash
cd frontend && npm run test game-pagination.test.tsx
```

Expected: All tests fail with `Cannot find module './game-pagination'`.

---

## Task 2: Implement `GamesPagination`

**Files:**
- Create: `frontend/src/components/games/game-pagination.tsx`

- [ ] **Step 2.1: Create the component**

> **Note on `href="#"`:** `PaginationLink`, `PaginationPrevious`, and `PaginationNext` all render as `<a>` tags. In JSDOM (used by Vitest), an `<a>` without `href` has role `generic`, not `link`. Adding `href="#"` gives them role `link` so tests can query them correctly. `e.preventDefault()` prevents browser navigation.

```tsx
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

const PER_PAGE_OPTIONS = [25, 50, 100, 500] as const;

export interface GamesPaginationProps {
  page: number;
  perPage: number;
  totalPages: number;
  totalCount: number;
  onPageChange: (page: number) => void;
  onPerPageChange: (perPage: number) => void;
  showPerPageSelector: boolean;
}

/**
 * Compute page numbers (and ellipsis placeholders) to display.
 * Always shows: first, last, current ± 1, with 'ellipsis' where gaps exist.
 * For 7 or fewer pages shows all pages without ellipsis.
 */
function getPageRange(current: number, total: number): (number | 'ellipsis')[] {
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }

  const pages: (number | 'ellipsis')[] = [1];

  if (current > 3) {
    pages.push('ellipsis');
  }

  const start = Math.max(2, current - 1);
  const end = Math.min(total - 1, current + 1);
  for (let i = start; i <= end; i++) {
    pages.push(i);
  }

  if (current < total - 2) {
    pages.push('ellipsis');
  }

  pages.push(total);
  return pages;
}

export function GamesPagination({
  page,
  perPage,
  totalPages,
  onPageChange,
  onPerPageChange,
  showPerPageSelector,
}: GamesPaginationProps) {
  if (totalPages <= 1) return null;

  const pageRange = getPageRange(page, totalPages);
  const isFirst = page <= 1;
  const isLast = page >= totalPages;

  return (
    <div className="flex items-center justify-between">
      {showPerPageSelector ? (
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">Per page</span>
          <Select
            value={String(perPage)}
            onValueChange={(v) => onPerPageChange(Number(v))}
          >
            <SelectTrigger className="w-20">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PER_PAGE_OPTIONS.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : (
        <div />
      )}

      <Pagination>
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (!isFirst) onPageChange(page - 1);
              }}
              aria-disabled={isFirst}
              className={isFirst ? 'pointer-events-none opacity-50' : 'cursor-pointer'}
            />
          </PaginationItem>

          {pageRange.map((p, i) =>
            p === 'ellipsis' ? (
              <PaginationItem key={`ellipsis-${i}`}>
                <PaginationEllipsis />
              </PaginationItem>
            ) : (
              <PaginationItem key={p}>
                <PaginationLink
                  href="#"
                  isActive={p === page}
                  onClick={(e) => {
                    e.preventDefault();
                    onPageChange(p);
                  }}
                  className="cursor-pointer"
                >
                  {p}
                </PaginationLink>
              </PaginationItem>
            )
          )}

          <PaginationItem>
            <PaginationNext
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (!isLast) onPageChange(page + 1);
              }}
              aria-disabled={isLast}
              className={isLast ? 'pointer-events-none opacity-50' : 'cursor-pointer'}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
}
```

- [ ] **Step 2.2: Run the tests and verify they pass**

```bash
cd frontend && npm run test game-pagination.test.tsx
```

Expected: All tests pass. If any fail, fix the component before continuing.

- [ ] **Step 2.3: Type check**

```bash
cd frontend && npm run check
```

Expected: Zero TypeScript errors.

- [ ] **Step 2.4: Commit**

```bash
cd frontend && git add src/components/games/game-pagination.tsx src/components/games/game-pagination.test.tsx
git commit -m "feat(frontend): add GamesPagination component"
```

---

## Task 3: Export `GamesPagination` from the games components barrel

**Files:**
- Modify: `frontend/src/components/games/index.ts`

Current content of `index.ts`:
```ts
export { BulkActions } from './bulk-actions';
export type { BulkActionsProps } from './bulk-actions';
export { GameCard } from './game-card';
export type { GameCardProps } from './game-card';
export { GameFilters } from './game-filters';
export type { GameFiltersProps } from './game-filters';
export { GameGrid } from './game-grid';
export type { GameGridProps } from './game-grid';
export { GameList } from './game-list';
export type { GameListProps } from './game-list';
export { GameEditForm } from './game-edit-form';
export type { GameEditFormProps } from './game-edit-form';
```

- [ ] **Step 3.1: Add the export (append to end of file)**

```ts
export { GamesPagination } from './game-pagination';
export type { GamesPaginationProps } from './game-pagination';
```

- [ ] **Step 3.2: Type check**

```bash
cd frontend && npm run check
```

Expected: Zero TypeScript errors.

- [ ] **Step 3.3: Commit**

```bash
cd frontend && git add src/components/games/index.ts
git commit -m "feat(frontend): export GamesPagination from games barrel"
```

---

## Task 4: Wire pagination into `GamesPageContent`

**Files:**
- Modify: `frontend/src/routes/_authenticated/games/index.tsx`

Read the current file before making any edits.

### Step 4.1 — Update imports

- [ ] **Step 4.1a: Add `useEffect` to the React import**

Change:
```tsx
import { Suspense, useMemo, useCallback, useState } from 'react';
```
To:
```tsx
import { Suspense, useMemo, useCallback, useState, useEffect } from 'react';
```

- [ ] **Step 4.1b: Add `GamesPagination` to the games components import**

Change:
```tsx
import {
  GameFilters,
  GameGrid,
  GameList,
  BulkActions,
} from '@/components/games';
```
To:
```tsx
import {
  GameFilters,
  GameGrid,
  GameList,
  BulkActions,
  GamesPagination,
} from '@/components/games';
```

### Step 4.2 — Add `parsePerPage` helper

- [ ] **Step 4.2: Add after the `SORT_OPTIONS` array (before `function GamesPageContent`)**

```tsx
const VALID_PER_PAGE = [25, 50, 100, 500] as const;
type PerPage = typeof VALID_PER_PAGE[number];

function parsePerPage(raw: string): PerPage {
  const n = parseInt(raw, 10);
  return (VALID_PER_PAGE as readonly number[]).includes(n) ? (n as PerPage) : 50;
}
```

### Step 4.3 — Read `page` and `perPage` from URL

- [ ] **Step 4.3: Add after the existing `sortBy`, `sortOrder`, `viewMode` reads**

Locate:
```tsx
const s = search as Record<string, string>;
const sortBy = (s['sort'] as SortField) ?? 'title';
const sortOrder = (s['order'] as SortOrder) ?? 'asc';
const viewMode = (s['view'] as 'grid' | 'list') ?? 'grid';
```

Add immediately after:
```tsx
const rawPage = parseInt(s['page'] ?? '1', 10);
const currentPage = isNaN(rawPage) || rawPage < 1 ? 1 : rawPage;
const currentPerPage = parsePerPage(s['perPage'] ?? '50');
```

### Step 4.4 — Replace `queryParams` with `filterFields`, `listQueryParams`, `idsQueryParams`

- [ ] **Step 4.4: Replace the single `queryParams` memo**

Remove:
```tsx
const queryParams = useMemo(
  () => ({
    search: filters.search || undefined,
    status: filters.status,
    ownershipStatus: filters.ownershipStatus,
    platform: filters.platforms.length > 0 ? filters.platforms : undefined,
    storefront: filters.storefronts.length > 0 ? filters.storefronts : undefined,
    genre: filters.genres.length > 0 ? filters.genres : undefined,
    gameMode: filters.gameModes.length > 0 ? filters.gameModes : undefined,
    theme: filters.themes.length > 0 ? filters.themes : undefined,
    playerPerspective: filters.playerPerspectives.length > 0 ? filters.playerPerspectives : undefined,
    tags: filters.tags.length > 0 ? filters.tags : undefined,
    perPage: 50,
    sortBy,
    sortOrder,
  }),
  [filters, sortBy, sortOrder]
);
```

Replace with:
```tsx
// Shared filter fields — no pagination params
const filterFields = useMemo(
  () => ({
    search: filters.search || undefined,
    status: filters.status,
    ownershipStatus: filters.ownershipStatus,
    platform: filters.platforms.length > 0 ? filters.platforms : undefined,
    storefront: filters.storefronts.length > 0 ? filters.storefronts : undefined,
    genre: filters.genres.length > 0 ? filters.genres : undefined,
    gameMode: filters.gameModes.length > 0 ? filters.gameModes : undefined,
    theme: filters.themes.length > 0 ? filters.themes : undefined,
    playerPerspective: filters.playerPerspectives.length > 0 ? filters.playerPerspectives : undefined,
    tags: filters.tags.length > 0 ? filters.tags : undefined,
  }),
  [filters]
);

// Passed to useUserGames — includes page + perPage
const listQueryParams = useMemo(
  () => ({
    ...filterFields,
    page: currentPage,
    perPage: currentPerPage,
    sortBy,
    sortOrder,
  }),
  [filterFields, currentPage, currentPerPage, sortBy, sortOrder]
);

// Passed to useUserGameIds — no page/perPage so "select all" spans all pages
const idsQueryParams = useMemo(
  () => ({
    ...filterFields,
    sortBy,
    sortOrder,
  }),
  [filterFields, sortBy, sortOrder]
);
```

### Step 4.5 — Update hook calls

- [ ] **Step 4.5: Update `useUserGames` and `useUserGameIds` to use the new param names**

Change:
```tsx
const { data, isLoading, refetch } = useUserGames(queryParams);
```
To:
```tsx
const { data, isLoading, refetch } = useUserGames(listQueryParams);
```

Change:
```tsx
const { refetch: fetchAllIds } = useUserGameIds(queryParams, { enabled: false });
```
To:
```tsx
const { refetch: fetchAllIds } = useUserGameIds(idsQueryParams, { enabled: false });
```

### Step 4.6 — Add out-of-range page reset effect

- [ ] **Step 4.6: Add after the `games`, `totalCount`, `visibleCount` declarations**

Locate:
```tsx
const games = useMemo(() => data?.items ?? [], [data?.items]);
const totalCount = data?.total ?? 0;
const visibleCount = games.length;
```

Add immediately after:
```tsx
// Reset to page 1 if the URL page exceeds available pages after data loads
useEffect(() => {
  if (data && data.pages > 0 && currentPage > data.pages) {
    updateParams({ page: undefined });
  }
}, [data, currentPage, updateParams]);
```

### Step 4.7 — Update filter/sort handlers to reset page

- [ ] **Step 4.7a: Add `page: undefined` to `handleFiltersChange`**

In `handleFiltersChange`, add `page: undefined` to the `updateParams` call:
```tsx
updateParams({
  q: newFilters.search || undefined,
  status: newFilters.status,
  ownership: newFilters.ownershipStatus,
  platform: newFilters.platforms,
  storefront: newFilters.storefronts,
  genre: newFilters.genres,
  gameMode: newFilters.gameModes,
  theme: newFilters.themes,
  playerPerspective: newFilters.playerPerspectives,
  tag: newFilters.tags,
  page: undefined, // reset to page 1 when filters change
});
```

- [ ] **Step 4.7b: Add `page: undefined` to `handleSortByChange`**

Change:
```tsx
updateParams({ sort: newSortBy, order: newOrder });
```
To:
```tsx
updateParams({ sort: newSortBy, order: newOrder, page: undefined });
```

- [ ] **Step 4.7c: Add `page: undefined` to `handleSortOrderToggle`**

Change:
```tsx
updateParams({ order: newOrder });
```
To:
```tsx
updateParams({ order: newOrder, page: undefined });
```

### Step 4.8 — Add new pagination handlers

- [ ] **Step 4.8: Add `handlePageChange` and `handlePerPageChange` after the existing handlers**

```tsx
const handlePageChange = useCallback((page: number) => {
  updateParams({ page: page === 1 ? undefined : String(page) });
  setSelectedIds(new Set());
  setSelectionMode('manual');
}, [updateParams]);

const handlePerPageChange = useCallback((perPage: number) => {
  updateParams({
    perPage: perPage === 50 ? undefined : String(perPage),
    page: undefined,
  });
}, [updateParams]);
```

### Step 4.9 — Render `GamesPagination` in the JSX

- [ ] **Step 4.9a: Add the top pagination bar after `<GameFilters ... />` and before `<BulkActions ... />`**

```tsx
{/* Top pagination bar — includes per-page selector */}
{data && (
  <GamesPagination
    page={currentPage}
    perPage={currentPerPage}
    totalPages={data.pages}
    totalCount={data.total}
    onPageChange={handlePageChange}
    onPerPageChange={handlePerPageChange}
    showPerPageSelector={true}
  />
)}
```

- [ ] **Step 4.9b: Add the bottom pagination bar after the `GameGrid`/`GameList` block**

```tsx
{/* Bottom pagination bar — page navigation only */}
{data && (
  <GamesPagination
    page={currentPage}
    perPage={currentPerPage}
    totalPages={data.pages}
    totalCount={data.total}
    onPageChange={handlePageChange}
    onPerPageChange={handlePerPageChange}
    showPerPageSelector={false}
  />
)}
```

- [ ] **Step 4.10: Type check**

```bash
cd frontend && npm run check
```

Expected: Zero TypeScript errors. Fix any errors before continuing.

- [ ] **Step 4.11: Run all frontend tests**

```bash
cd frontend && npm run test
```

Expected: All tests pass. Fix any failures before continuing.

- [ ] **Step 4.12: Commit**

```bash
cd frontend && git add src/routes/_authenticated/games/index.tsx
git commit -m "feat(frontend): wire pagination into GamesPageContent"
```

---

## Task 5: Final verification

- [ ] **Step 5.1: Run full test suite with coverage**

```bash
cd frontend && npm run test:coverage
```

Expected: All tests pass, coverage ≥ 70%. No regressions.

- [ ] **Step 5.2: Final type check**

```bash
cd frontend && npm run check
```

Expected: Zero errors.

---

## Task 6: Open pull request

- [ ] **Step 6.1: Push the feature branch**

```bash
git push -u origin feat/games-list-pagination
```

- [ ] **Step 6.2: Create the PR**

```bash
gh pr create \
  --title "feat: add pagination to games list" \
  --body "$(cat <<'EOF'
## Summary

- Adds `GamesPagination` component with prev/next, numbered pages (with ellipsis), and a per-page selector (25/50/100/500)
- Pagination controls appear at the top (with per-page selector) and bottom (navigation only) of the game grid/list
- `page` and `perPage` are stored in the URL; defaults (page=1, perPage=50) are omitted to keep URLs clean
- Filters, sort, and page changes all reset to page 1
- Page navigation clears bulk selection
- Fixes: \"Pagination hidden when filtering games list\" roadmap item

## Test plan

- [ ] Run `npm run test` — all tests pass
- [ ] Run `npm run check` — zero TypeScript errors
- [ ] Run `npm run test:coverage` — coverage ≥ 70%
- [ ] Manual: load the games list with 50+ games and verify pagination controls appear
- [ ] Manual: apply a filter and verify pagination controls remain visible
- [ ] Manual: navigate to page 2, then change a filter — verify return to page 1
- [ ] Manual: change per-page to 100 — verify URL updates and more games load
- [ ] Manual: navigate to a deep page, then manually type a page number beyond range in URL — verify auto-reset to page 1

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Reference

- Spec: `docs/superpowers/specs/2026-03-21-pagination-games-list-design.md`
- Shadcn Pagination: `frontend/src/components/ui/pagination.tsx`
- Shadcn Select usage example: `frontend/src/components/games/bulk-actions.tsx`
- Test pattern: `frontend/src/components/games/game-grid.test.tsx`
- Games API types: `frontend/src/api/games.ts` — `GetUserGamesParams`, `UserGamesListResponse`
- Games hooks: `frontend/src/hooks/use-games.ts` — `useUserGames`, `useUserGameIds`

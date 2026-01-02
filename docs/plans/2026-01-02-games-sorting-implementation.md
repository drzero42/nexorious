# Games Page Sorting Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add sorting controls to /games page with alphabetical (A→Z) as default, dropdown for sort field selection, direction toggle, and session persistence.

**Architecture:** Backend defaults change to `title`/`asc`. Frontend adds sort dropdown + direction toggle to filter bar. Sort state stored in sessionStorage and passed to existing API hook.

**Tech Stack:** FastAPI (Python), Next.js 16, React 19, shadcn/ui Select, lucide-react icons, TanStack Query

---

## Task 1: Backend - Change Default Sort Order

**Files:**
- Modify: `backend/app/api/user_games.py:120-121`

**Step 1: Update default values**

Change line 120-121 from:
```python
    sort_by: Optional[str] = Query(default="created_at", description="Sort field"),
    sort_order: Optional[str] = Query(default="desc", pattern="^(asc|desc)$", description="Sort order")
```

To:
```python
    sort_by: Optional[str] = Query(default="title", description="Sort field"),
    sort_order: Optional[str] = Query(default="asc", pattern="^(asc|desc)$", description="Sort order")
```

**Step 2: Run tests to check for failures**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/backend && uv run pytest app/tests/test_integration_user_games.py -v -k "sort" 2>&1 | tail -30`

Expected: Tests may fail if they rely on old default order. Fix if needed.

**Step 3: Commit**

```bash
git add backend/app/api/user_games.py
git commit -m "feat(api): change default sort to title ascending"
```

---

## Task 2: Frontend - Add Sort Types

**Files:**
- Modify: `frontend/src/app/(main)/games/page.tsx`

**Step 1: Add sort types and constants at top of file**

Add after the imports (before `export default function GamesPage()`):

```typescript
type SortField = 'title' | 'created_at' | 'time_to_beat' | 'rating' | 'release_date';
type SortOrder = 'asc' | 'desc';

interface SortOption {
  value: SortField;
  label: string;
  defaultOrder: SortOrder;
}

const SORT_OPTIONS: SortOption[] = [
  { value: 'title', label: 'Title', defaultOrder: 'asc' },
  { value: 'created_at', label: 'Date Added', defaultOrder: 'desc' },
  { value: 'time_to_beat', label: 'Time to Beat', defaultOrder: 'asc' },
  { value: 'rating', label: 'My Rating', defaultOrder: 'desc' },
  { value: 'release_date', label: 'Release Date', defaultOrder: 'desc' },
];

const SORT_STORAGE_KEY = 'games-sort-preference';

function loadSortPreference(): { sortBy: SortField; sortOrder: SortOrder } {
  if (typeof window === 'undefined') {
    return { sortBy: 'title', sortOrder: 'asc' };
  }
  try {
    const stored = sessionStorage.getItem(SORT_STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);
      return {
        sortBy: parsed.sortBy ?? 'title',
        sortOrder: parsed.sortOrder ?? 'asc',
      };
    }
  } catch {
    // Ignore parse errors
  }
  return { sortBy: 'title', sortOrder: 'asc' };
}

function saveSortPreference(sortBy: SortField, sortOrder: SortOrder): void {
  if (typeof window === 'undefined') return;
  sessionStorage.setItem(SORT_STORAGE_KEY, JSON.stringify({ sortBy, sortOrder }));
}
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run check`

Expected: PASS (types are defined but not yet used)

**Step 3: Commit**

```bash
git add frontend/src/app/\(main\)/games/page.tsx
git commit -m "feat(games): add sort types and session storage helpers"
```

---

## Task 3: Frontend - Add Sort State to Games Page

**Files:**
- Modify: `frontend/src/app/(main)/games/page.tsx`

**Step 1: Add sort state after existing useState calls**

Add after `const [selectionMode, setSelectionMode] = useState<SelectionMode>('manual');`:

```typescript
  // Sort state - initialize from sessionStorage
  const [sortBy, setSortBy] = useState<SortField>(() => loadSortPreference().sortBy);
  const [sortOrder, setSortOrder] = useState<SortOrder>(() => loadSortPreference().sortOrder);
```

**Step 2: Update queryParams to include sort**

Change the `queryParams` useMemo from:
```typescript
  const queryParams = useMemo(
    () => ({
      search: filters.search || undefined,
      status: filters.status,
      platformId: filters.platformId,
      perPage: 50,
    }),
    [filters]
  );
```

To:
```typescript
  const queryParams = useMemo(
    () => ({
      search: filters.search || undefined,
      status: filters.status,
      platformId: filters.platformId,
      perPage: 50,
      sortBy,
      sortOrder,
    }),
    [filters, sortBy, sortOrder]
  );
```

**Step 3: Add sort change handlers after handleFiltersChange**

Add after `handleFiltersChange`:

```typescript
  const handleSortByChange = useCallback((newSortBy: SortField) => {
    const option = SORT_OPTIONS.find((o) => o.value === newSortBy);
    const newOrder = option?.defaultOrder ?? 'asc';
    setSortBy(newSortBy);
    setSortOrder(newOrder);
    saveSortPreference(newSortBy, newOrder);
  }, []);

  const handleSortOrderToggle = useCallback(() => {
    const newOrder = sortOrder === 'asc' ? 'desc' : 'asc';
    setSortOrder(newOrder);
    saveSortPreference(sortBy, newOrder);
  }, [sortBy, sortOrder]);
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run check`

Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/app/\(main\)/games/page.tsx
git commit -m "feat(games): add sort state and handlers to games page"
```

---

## Task 4: Frontend - Update GameFilters Props

**Files:**
- Modify: `frontend/src/components/games/game-filters.tsx`

**Step 1: Update imports**

Change:
```typescript
import { Grid, List, X } from 'lucide-react';
```

To:
```typescript
import { ArrowDownAZ, ArrowUpAZ, ArrowDown, ArrowUp, Grid, List, X } from 'lucide-react';
```

**Step 2: Update GameFiltersProps interface**

Change from:
```typescript
export interface GameFiltersProps {
  filters: {
    search: string;
    status?: PlayStatus;
    platformId?: string;
  };
  onFiltersChange: (filters: GameFiltersProps['filters']) => void;
  viewMode: 'grid' | 'list';
  onViewModeChange: (mode: 'grid' | 'list') => void;
}
```

To:
```typescript
type SortField = 'title' | 'created_at' | 'time_to_beat' | 'rating' | 'release_date';
type SortOrder = 'asc' | 'desc';

interface SortOption {
  value: SortField;
  label: string;
}

const sortOptions: SortOption[] = [
  { value: 'title', label: 'Title' },
  { value: 'created_at', label: 'Date Added' },
  { value: 'time_to_beat', label: 'Time to Beat' },
  { value: 'rating', label: 'My Rating' },
  { value: 'release_date', label: 'Release Date' },
];

export interface GameFiltersProps {
  filters: {
    search: string;
    status?: PlayStatus;
    platformId?: string;
  };
  onFiltersChange: (filters: GameFiltersProps['filters']) => void;
  viewMode: 'grid' | 'list';
  onViewModeChange: (mode: 'grid' | 'list') => void;
  sortBy: SortField;
  sortOrder: SortOrder;
  onSortByChange: (sortBy: SortField) => void;
  onSortOrderToggle: () => void;
}
```

**Step 3: Update component destructuring**

Change:
```typescript
export function GameFilters({
  filters,
  onFiltersChange,
  viewMode,
  onViewModeChange,
}: GameFiltersProps) {
```

To:
```typescript
export function GameFilters({
  filters,
  onFiltersChange,
  viewMode,
  onViewModeChange,
  sortBy,
  sortOrder,
  onSortByChange,
  onSortOrderToggle,
}: GameFiltersProps) {
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run check`

Expected: FAIL (games page not passing new props yet)

**Step 5: Commit**

```bash
git add frontend/src/components/games/game-filters.tsx
git commit -m "feat(games): update GameFilters props for sorting"
```

---

## Task 5: Frontend - Add Sort UI to GameFilters

**Files:**
- Modify: `frontend/src/components/games/game-filters.tsx`

**Step 1: Add sort dropdown and toggle button**

In the return statement, add after the Platform filter Select (before `{/* Clear filters */}`):

```tsx
      {/* Sort dropdown */}
      <Select
        value={sortBy}
        onValueChange={(value) => onSortByChange(value as SortField)}
      >
        <SelectTrigger className="w-40">
          <SelectValue placeholder="Sort by" />
        </SelectTrigger>
        <SelectContent>
          {sortOptions.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Sort direction toggle */}
      <Button
        variant="outline"
        size="icon"
        onClick={onSortOrderToggle}
        title={sortOrder === 'asc' ? 'Ascending' : 'Descending'}
      >
        {sortBy === 'title' ? (
          sortOrder === 'asc' ? (
            <ArrowDownAZ className="h-4 w-4" />
          ) : (
            <ArrowUpAZ className="h-4 w-4" />
          )
        ) : sortOrder === 'asc' ? (
          <ArrowUp className="h-4 w-4" />
        ) : (
          <ArrowDown className="h-4 w-4" />
        )}
      </Button>
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run check`

Expected: FAIL (games page still not passing props)

**Step 3: Commit**

```bash
git add frontend/src/components/games/game-filters.tsx
git commit -m "feat(games): add sort dropdown and direction toggle to filters"
```

---

## Task 6: Frontend - Wire Up Sort Props in Games Page

**Files:**
- Modify: `frontend/src/app/(main)/games/page.tsx`

**Step 1: Update GameFilters usage**

Change:
```tsx
      <GameFilters
        filters={filters}
        onFiltersChange={handleFiltersChange}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
      />
```

To:
```tsx
      <GameFilters
        filters={filters}
        onFiltersChange={handleFiltersChange}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
        sortBy={sortBy}
        sortOrder={sortOrder}
        onSortByChange={handleSortByChange}
        onSortOrderToggle={handleSortOrderToggle}
      />
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run check`

Expected: PASS

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run test`

Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/app/\(main\)/games/page.tsx
git commit -m "feat(games): wire up sort props to GameFilters component"
```

---

## Task 7: Frontend - Add GameFilters Tests

**Files:**
- Create: `frontend/src/components/games/game-filters.test.tsx`

**Step 1: Create test file**

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { GameFilters } from './game-filters';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

// Mock the platforms hook
vi.mock('@/hooks', () => ({
  useAllPlatforms: () => ({
    data: [
      { name: 'pc', display_name: 'PC' },
      { name: 'ps5', display_name: 'PlayStation 5' },
    ],
  }),
}));

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
  );
}

describe('GameFilters', () => {
  const defaultProps = {
    filters: { search: '' },
    onFiltersChange: vi.fn(),
    viewMode: 'grid' as const,
    onViewModeChange: vi.fn(),
    sortBy: 'title' as const,
    sortOrder: 'asc' as const,
    onSortByChange: vi.fn(),
    onSortOrderToggle: vi.fn(),
  };

  it('renders sort dropdown with current value', () => {
    renderWithProviders(<GameFilters {...defaultProps} />);
    expect(screen.getByRole('combobox', { name: /sort/i })).toBeInTheDocument();
  });

  it('renders sort direction toggle button', () => {
    renderWithProviders(<GameFilters {...defaultProps} />);
    expect(screen.getByTitle('Ascending')).toBeInTheDocument();
  });

  it('calls onSortByChange when sort option is selected', async () => {
    const onSortByChange = vi.fn();
    renderWithProviders(
      <GameFilters {...defaultProps} onSortByChange={onSortByChange} />
    );

    const trigger = screen.getByRole('combobox', { name: /sort/i });
    await userEvent.click(trigger);

    const dateOption = screen.getByRole('option', { name: 'Date Added' });
    await userEvent.click(dateOption);

    expect(onSortByChange).toHaveBeenCalledWith('created_at');
  });

  it('calls onSortOrderToggle when direction button is clicked', async () => {
    const onSortOrderToggle = vi.fn();
    renderWithProviders(
      <GameFilters {...defaultProps} onSortOrderToggle={onSortOrderToggle} />
    );

    await userEvent.click(screen.getByTitle('Ascending'));
    expect(onSortOrderToggle).toHaveBeenCalled();
  });

  it('shows descending title when sortOrder is desc', () => {
    renderWithProviders(
      <GameFilters {...defaultProps} sortOrder="desc" />
    );
    expect(screen.getByTitle('Descending')).toBeInTheDocument();
  });
});
```

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run test -- src/components/games/game-filters.test.tsx`

Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/components/games/game-filters.test.tsx
git commit -m "test(games): add GameFilters sorting tests"
```

---

## Task 8: Verify Full Integration

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/backend && uv run pytest -q 2>&1 | tail -5`

Expected: All tests pass

**Step 2: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run test 2>&1 | tail -10`

Expected: All tests pass

**Step 3: Run type checks**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/games-sorting/frontend && npm run check`

Expected: PASS

**Step 4: Final commit if any fixes were needed**

```bash
git status
# If clean, no action needed
```

---

## Summary of Changes

| File | Change |
|------|--------|
| `backend/app/api/user_games.py` | Default sort: `title` asc |
| `frontend/src/app/(main)/games/page.tsx` | Sort state, handlers, sessionStorage |
| `frontend/src/components/games/game-filters.tsx` | Sort dropdown + direction toggle |
| `frontend/src/components/games/game-filters.test.tsx` | New test file |

Total: ~150 lines added across 4 files.

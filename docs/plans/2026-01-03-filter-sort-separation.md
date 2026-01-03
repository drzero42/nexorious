# Filter/Sort UI Separation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reorganize the /games page filter and sort controls from a single row into two labeled rows for better UX.

**Architecture:** Restructure the `GameFilters` component's JSX to use a vertical flex container with two child rows. Row 1 contains sort controls + view toggle. Row 2 contains filter controls + clear button. Each row has a left-aligned label.

**Tech Stack:** React, Tailwind CSS, existing shadcn/ui components

---

## Task 1: Update Tests for Two-Row Layout

**Files:**
- Modify: `frontend/src/components/games/game-filters.test.tsx`

**Step 1: Add test for "Sort by:" label**

Add this test to the `sort controls` describe block (around line 951):

```tsx
it('renders "Sort by:" label', () => {
  render(<GameFilters {...defaultProps} />);

  expect(screen.getByText('Sort by:')).toBeInTheDocument();
});
```

**Step 2: Add test for "Filters:" label**

Add this test to the top of the component tests (after line 154, before `describe('search input')`):

```tsx
describe('layout', () => {
  it('renders "Filters:" label', () => {
    render(<GameFilters {...defaultProps} />);

    expect(screen.getByText('Filters:')).toBeInTheDocument();
  });

  it('renders "Sort by:" label', () => {
    render(<GameFilters {...defaultProps} />);

    expect(screen.getByText('Sort by:')).toBeInTheDocument();
  });

  it('renders sort row before filters row', () => {
    render(<GameFilters {...defaultProps} />);

    const sortLabel = screen.getByText('Sort by:');
    const filtersLabel = screen.getByText('Filters:');

    // Sort row should come before filters row in the DOM
    expect(sortLabel.compareDocumentPosition(filtersLabel)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });
});
```

**Step 3: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/filter-sort-separation/frontend && npm run test -- --run src/components/games/game-filters.test.tsx`

Expected: FAIL - "Sort by:" and "Filters:" labels not found

**Step 4: Commit failing tests**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/filter-sort-separation
git add frontend/src/components/games/game-filters.test.tsx
git commit -m "test: add tests for two-row filter/sort layout"
```

---

## Task 2: Implement Two-Row Layout Structure

**Files:**
- Modify: `frontend/src/components/games/game-filters.tsx:105-236`

**Step 1: Replace the return statement**

Replace the entire return block (lines 105-237) with:

```tsx
return (
  <div className="flex flex-col gap-3">
    {/* Sort row */}
    <div className="flex flex-wrap gap-4 items-center">
      <span className="text-sm text-muted-foreground w-14">Sort by:</span>

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

      {/* Spacer */}
      <div className="flex-1" />

      {/* View toggle */}
      <div className="flex border rounded-md">
        <Button
          variant={viewMode === 'grid' ? 'secondary' : 'ghost'}
          size="sm"
          onClick={() => onViewModeChange('grid')}
        >
          <Grid className="h-4 w-4" />
        </Button>
        <Button
          variant={viewMode === 'list' ? 'secondary' : 'ghost'}
          size="sm"
          onClick={() => onViewModeChange('list')}
        >
          <List className="h-4 w-4" />
        </Button>
      </div>
    </div>

    {/* Filters row */}
    <div className="flex flex-wrap gap-4 items-center">
      <span className="text-sm text-muted-foreground w-14">Filters:</span>

      {/* Search */}
      <Input
        type="search"
        placeholder="Search games..."
        value={filters.search}
        onChange={(e) => onFiltersChange({ ...filters, search: e.target.value })}
        className="w-full sm:w-64"
      />

      {/* Status filter */}
      <Select
        value={filters.status ?? 'all'}
        onValueChange={(value) =>
          onFiltersChange({
            ...filters,
            status: value === 'all' ? undefined : (value as PlayStatus),
          })
        }
      >
        <SelectTrigger className="w-40">
          <SelectValue placeholder="Status" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All Statuses</SelectItem>
          {statusOptions.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Platform filter (multi-select) */}
      <MultiSelectFilter
        label="Platforms"
        options={platformOptions}
        selected={filters.platforms ?? []}
        onChange={(selected) => onFiltersChange({ ...filters, platforms: selected })}
      />

      {/* Storefront filter (multi-select) */}
      <MultiSelectFilter
        label="Storefronts"
        options={storefrontOptions}
        selected={filters.storefronts ?? []}
        onChange={(selected) => onFiltersChange({ ...filters, storefronts: selected })}
      />

      {/* Genre filter (multi-select) */}
      <MultiSelectFilter
        label="Genres"
        options={genreOptions}
        selected={filters.genres ?? []}
        onChange={(selected) => onFiltersChange({ ...filters, genres: selected })}
      />

      {/* Tags filter (multi-select) */}
      <MultiSelectFilter
        label="Tags"
        options={tagOptions}
        selected={filters.tags ?? []}
        onChange={(selected) => onFiltersChange({ ...filters, tags: selected })}
      />

      {/* Clear filters */}
      {hasActiveFilters && (
        <Button variant="ghost" size="sm" onClick={clearFilters}>
          <X className="h-4 w-4 mr-1" />
          Clear
        </Button>
      )}
    </div>
  </div>
);
```

**Step 2: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/filter-sort-separation/frontend && npm run test -- --run src/components/games/game-filters.test.tsx`

Expected: All 54+ tests PASS

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/filter-sort-separation/frontend && npm run check`

Expected: PASS (no new errors)

**Step 4: Commit implementation**

```bash
cd /home/abo/workspace/home/nexorious/.worktrees/filter-sort-separation
git add frontend/src/components/games/game-filters.tsx
git commit -m "feat: separate filters and sorting into two labeled rows

- Sort row on top with view toggle pushed right
- Filters row below with search, status, platforms, storefronts, genres, tags
- Labels: 'Sort by:' and 'Filters:' with subtle muted styling
- Search input responsive: full width on mobile, fixed 256px on sm+"
```

---

## Task 3: Visual Verification

**Files:**
- None (manual verification)

**Step 1: Start the development server**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/filter-sort-separation/frontend && npm run dev`

**Step 2: Verify in browser**

1. Navigate to http://localhost:3000/games
2. Verify layout matches design:
   - Sort row on top with "Sort by:" label
   - Sort dropdown, direction toggle, then view toggle (right-aligned)
   - Filters row below with "Filters:" label
   - Search, status, multi-selects, clear button
3. Resize browser to verify responsive behavior:
   - Desktop: both rows single-line
   - Mobile: filter controls wrap naturally, search takes full width

**Step 3: Stop dev server**

Press Ctrl+C to stop

---

## Task 4: Run Full Test Suite

**Files:**
- None (verification only)

**Step 1: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/filter-sort-separation/frontend && npm run test`

Expected: All tests PASS

**Step 2: Run type check and lint**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/filter-sort-separation/frontend && npm run check`

Expected: PASS (0 errors, pre-existing warnings only)

---

## Summary

| Task | Description | Commits |
|------|-------------|---------|
| 1 | Add tests for two-row layout | `test: add tests for two-row filter/sort layout` |
| 2 | Implement two-row layout | `feat: separate filters and sorting into two labeled rows` |
| 3 | Visual verification | (no commit) |
| 4 | Full test suite | (no commit) |

**Total commits:** 2
**Files changed:** 2 (`game-filters.tsx`, `game-filters.test.tsx`)

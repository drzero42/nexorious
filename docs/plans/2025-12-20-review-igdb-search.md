# Review Modal IGDB Search Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add IGDB search capability to the review candidates modal so users can find matches outside pre-computed candidates.

**Architecture:** Add a search section at the bottom of the existing candidates modal in the review page. Reuse the existing `useSearchIGDB` hook and `searchIGDB` API function. Search results appear in an autocomplete dropdown; clicking a result immediately matches the review item.

**Tech Stack:** React, TanStack Query, shadcn/ui Input component, existing useSearchIGDB hook

---

### Task 1: Add Search State and Input to Modal

**Files:**
- Modify: `frontend/src/app/(main)/review/page.tsx:476-516` (Dialog component)

**Step 1: Write the failing test**

Add to `frontend/src/app/(main)/review/page.test.tsx`:

```tsx
import userEvent from '@testing-library/user-event';

// Add to the mock at line 27, inside the vi.mock('@/hooks') block:
// useSearchIGDB: vi.fn(() => ({ data: undefined, isLoading: false, error: null })),

// Add import after line 40:
// import { useSearchIGDB } from '@/hooks';
// const mockedUseSearchIGDB = vi.mocked(useSearchIGDB);

// Add new test:
describe('IGDB Search in Modal', () => {
  const mockReviewItem = {
    id: 'item-1',
    sourceTitle: 'Test Game',
    status: 'pending',
    igdbCandidates: [],
    resolvedIgdbId: null,
    matchConfidence: null,
    jobId: 'job-1',
    source: 'import',
    sourceMetadata: null,
    createdAt: '2024-01-01',
    updatedAt: '2024-01-01',
  };

  const mockItemsWithReviewItem = {
    items: [mockReviewItem],
    total: 1,
    page: 1,
    perPage: 20,
    pages: 1,
  };

  it('shows search input in modal when viewing candidates', async () => {
    const user = userEvent.setup();

    mockedUseReviewItems.mockReturnValue({
      data: mockItemsWithReviewItem,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
      isFetching: false,
    } as unknown as ReturnType<typeof useReviewItems>);

    mockedUseReviewSummary.mockReturnValue({
      data: mockSummaryWithPending,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useReviewSummary>);

    render(<ReviewPage />);

    // Click view button to open modal
    const viewButton = screen.getByRole('button', { name: /search igdb/i });
    await user.click(viewButton);

    // Search input should be visible in modal
    expect(screen.getByPlaceholderText(/search igdb/i)).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- --run src/app/\\(main\\)/review/page.test.tsx`
Expected: FAIL - search input not found

**Step 3: Write minimal implementation**

In `frontend/src/app/(main)/review/page.tsx`:

1. Add state for search query after line 101:
```tsx
const [searchQuery, setSearchQuery] = useState('');
```

2. Reset search when modal closes - update the Dialog onOpenChange (around line 476):
```tsx
<Dialog open={!!selectedItem} onOpenChange={(open) => {
  if (!open) {
    setSelectedItem(null);
    setSearchQuery('');
  }
}}>
```

3. Add search section after the candidates list, before the footer buttons (around line 501):
```tsx
{/* IGDB Search Section */}
<div className="border-t pt-4">
  <p className="mb-2 text-sm text-muted-foreground">
    Can't find the right match?
  </p>
  <Input
    placeholder="Search IGDB..."
    value={searchQuery}
    onChange={(e) => setSearchQuery(e.target.value)}
  />
</div>
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- --run src/app/\\(main\\)/review/page.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious && git add frontend/src/app/\(main\)/review/page.tsx frontend/src/app/\(main\)/review/page.test.tsx && git commit -m "feat(review): add search input to candidates modal"
```

---

### Task 2: Wire Up Search Hook and Display Results

**Files:**
- Modify: `frontend/src/app/(main)/review/page.tsx`
- Modify: `frontend/src/app/(main)/review/page.test.tsx`

**Step 1: Write the failing test**

Add to `frontend/src/app/(main)/review/page.test.tsx` in the 'IGDB Search in Modal' describe block:

```tsx
it('displays search results when typing 3+ characters', async () => {
  const user = userEvent.setup();

  const mockSearchResults = [
    {
      igdb_id: 123,
      title: 'Search Result Game',
      release_date: '2023-01-15',
      cover_art_url: 'https://example.com/cover.jpg',
      platforms: ['PC', 'PlayStation 5'],
      description: 'A great game',
    },
  ];

  mockedUseReviewItems.mockReturnValue({
    data: mockItemsWithReviewItem,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
    isFetching: false,
  } as unknown as ReturnType<typeof useReviewItems>);

  mockedUseReviewSummary.mockReturnValue({
    data: mockSummaryWithPending,
    isLoading: false,
    error: null,
  } as unknown as ReturnType<typeof useReviewSummary>);

  mockedUseSearchIGDB.mockReturnValue({
    data: mockSearchResults,
    isLoading: false,
    error: null,
  } as unknown as ReturnType<typeof useSearchIGDB>);

  render(<ReviewPage />);

  // Open modal
  const viewButton = screen.getByRole('button', { name: /search igdb/i });
  await user.click(viewButton);

  // Type search query
  const searchInput = screen.getByPlaceholderText(/search igdb/i);
  await user.type(searchInput, 'Search Result');

  // Results should appear
  expect(screen.getByText('Search Result Game')).toBeInTheDocument();
  expect(screen.getByText('2023')).toBeInTheDocument();
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- --run src/app/\\(main\\)/review/page.test.tsx`
Expected: FAIL - search results not displayed

**Step 3: Write minimal implementation**

In `frontend/src/app/(main)/review/page.tsx`:

1. Add import for useSearchIGDB (around line 51):
```tsx
import { useSearchIGDB } from '@/hooks';
```

2. Add import for IGDBGameCandidate type (around line 52):
```tsx
import type { IGDBGameCandidate } from '@/types';
```

3. Add the search hook after the other hooks (around line 122):
```tsx
const { data: searchResults, isLoading: isSearching } = useSearchIGDB(searchQuery);
```

4. Add search results dropdown after the Input in the search section:
```tsx
{/* IGDB Search Section */}
<div className="border-t pt-4">
  <p className="mb-2 text-sm text-muted-foreground">
    Can't find the right match?
  </p>
  <div className="relative">
    <Input
      placeholder="Search IGDB..."
      value={searchQuery}
      onChange={(e) => setSearchQuery(e.target.value)}
    />
    {searchQuery.length >= 3 && (
      <div className="absolute left-0 right-0 top-full z-50 mt-1 max-h-64 overflow-y-auto rounded-md border bg-popover shadow-lg">
        {isSearching ? (
          <div className="flex items-center justify-center p-4">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span className="ml-2 text-sm text-muted-foreground">Searching...</span>
          </div>
        ) : searchResults && searchResults.length > 0 ? (
          <div className="p-1">
            {searchResults.map((result) => (
              <SearchResultItem
                key={result.igdb_id}
                result={result}
                isProcessing={processingItemId === selectedItem?.id}
                onSelect={() => handleSearchResultMatch(result.igdb_id)}
              />
            ))}
          </div>
        ) : (
          <div className="p-4 text-center text-sm text-muted-foreground">
            No games found for "{searchQuery}"
          </div>
        )}
      </div>
    )}
  </div>
</div>
```

5. Add SearchResultItem component at the bottom of the file (after CandidateButton):
```tsx
interface SearchResultItemProps {
  result: IGDBGameCandidate;
  isProcessing: boolean;
  onSelect: () => void;
}

function SearchResultItem({ result, isProcessing, onSelect }: SearchResultItemProps) {
  const releaseYear = result.release_date
    ? new Date(result.release_date).getFullYear()
    : null;

  return (
    <button
      className="flex w-full items-center gap-3 rounded-md p-2 text-left transition-colors hover:bg-muted disabled:opacity-50"
      onClick={onSelect}
      disabled={isProcessing}
    >
      {result.cover_art_url ? (
        <img
          src={result.cover_art_url}
          alt={result.title}
          className="h-12 w-9 rounded object-cover"
        />
      ) : (
        <div className="flex h-12 w-9 items-center justify-center rounded bg-muted">
          <ImageOff className="h-4 w-4 text-muted-foreground" />
        </div>
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium">
          {result.title}
          {releaseYear && (
            <span className="ml-1 text-muted-foreground">({releaseYear})</span>
          )}
        </p>
        {result.platforms.length > 0 && (
          <p className="truncate text-xs text-muted-foreground">
            {result.platforms.slice(0, 3).join(', ')}
            {result.platforms.length > 3 && ` +${result.platforms.length - 3}`}
          </p>
        )}
      </div>
    </button>
  );
}
```

6. Add handleSearchResultMatch function (after handleModalSkip, around line 236):
```tsx
const handleSearchResultMatch = useCallback(
  async (igdbId: number) => {
    if (!selectedItem) return;
    setProcessingItemId(selectedItem.id);
    try {
      await matchMutation.mutateAsync({ itemId: selectedItem.id, igdbId });
      toast.success(`Matched "${selectedItem.sourceTitle}" to IGDB`);
      setSelectedItem(null);
      setSearchQuery('');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to match item');
    } finally {
      setProcessingItemId(null);
    }
  },
  [selectedItem, matchMutation]
);
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- --run src/app/\\(main\\)/review/page.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious && git add frontend/src/app/\(main\)/review/page.tsx frontend/src/app/\(main\)/review/page.test.tsx && git commit -m "feat(review): display IGDB search results in dropdown"
```

---

### Task 3: Add Test for Matching from Search Results

**Files:**
- Modify: `frontend/src/app/(main)/review/page.test.tsx`

**Step 1: Write the test**

Add to `frontend/src/app/(main)/review/page.test.tsx` in the 'IGDB Search in Modal' describe block:

```tsx
it('matches review item when clicking search result', async () => {
  const user = userEvent.setup();
  const mockMatchMutate = vi.fn().mockResolvedValue({});

  const mockSearchResults = [
    {
      igdb_id: 456,
      title: 'Clicked Game',
      release_date: '2022-06-15',
      cover_art_url: null,
      platforms: ['PC'],
      description: 'Another game',
    },
  ];

  // Re-mock with custom mutateAsync
  vi.mocked(useMatchReviewItem).mockReturnValue({
    mutateAsync: mockMatchMutate,
  } as unknown as ReturnType<typeof useMatchReviewItem>);

  mockedUseReviewItems.mockReturnValue({
    data: mockItemsWithReviewItem,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
    isFetching: false,
  } as unknown as ReturnType<typeof useReviewItems>);

  mockedUseReviewSummary.mockReturnValue({
    data: mockSummaryWithPending,
    isLoading: false,
    error: null,
  } as unknown as ReturnType<typeof useReviewSummary>);

  mockedUseSearchIGDB.mockReturnValue({
    data: mockSearchResults,
    isLoading: false,
    error: null,
  } as unknown as ReturnType<typeof useSearchIGDB>);

  render(<ReviewPage />);

  // Open modal
  const viewButton = screen.getByRole('button', { name: /search igdb/i });
  await user.click(viewButton);

  // Type and click result
  const searchInput = screen.getByPlaceholderText(/search igdb/i);
  await user.type(searchInput, 'Clicked');

  const resultButton = screen.getByRole('button', { name: /clicked game/i });
  await user.click(resultButton);

  // Verify match was called with correct params
  expect(mockMatchMutate).toHaveBeenCalledWith({
    itemId: 'item-1',
    igdbId: 456,
  });
});
```

**Step 2: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- --run src/app/\\(main\\)/review/page.test.tsx`
Expected: PASS (implementation already done in Task 2)

**Step 3: Commit**

```bash
cd /home/abo/workspace/home/nexorious && git add frontend/src/app/\(main\)/review/page.test.tsx && git commit -m "test(review): add test for matching from search results"
```

---

### Task 4: Add Error State Test

**Files:**
- Modify: `frontend/src/app/(main)/review/page.test.tsx`

**Step 1: Write the test**

Add to `frontend/src/app/(main)/review/page.test.tsx` in the 'IGDB Search in Modal' describe block:

```tsx
it('displays error message when search fails', async () => {
  const user = userEvent.setup();

  mockedUseReviewItems.mockReturnValue({
    data: mockItemsWithReviewItem,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
    isFetching: false,
  } as unknown as ReturnType<typeof useReviewItems>);

  mockedUseReviewSummary.mockReturnValue({
    data: mockSummaryWithPending,
    isLoading: false,
    error: null,
  } as unknown as ReturnType<typeof useReviewSummary>);

  mockedUseSearchIGDB.mockReturnValue({
    data: undefined,
    isLoading: false,
    error: new Error('Search failed'),
  } as unknown as ReturnType<typeof useSearchIGDB>);

  render(<ReviewPage />);

  // Open modal
  const viewButton = screen.getByRole('button', { name: /search igdb/i });
  await user.click(viewButton);

  // Type search query
  const searchInput = screen.getByPlaceholderText(/search igdb/i);
  await user.type(searchInput, 'test query');

  // Error message should appear
  expect(screen.getByText(/search failed/i)).toBeInTheDocument();
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- --run src/app/\\(main\\)/review/page.test.tsx`
Expected: FAIL - error message not displayed

**Step 3: Write minimal implementation**

In `frontend/src/app/(main)/review/page.tsx`, update the search hook to get error state:

```tsx
const { data: searchResults, isLoading: isSearching, error: searchError } = useSearchIGDB(searchQuery);
```

Update the dropdown to handle error state (add before the "No games found" case):

```tsx
) : searchError ? (
  <div className="p-4 text-center text-sm text-destructive">
    Search failed. Please try again.
  </div>
) : searchResults && searchResults.length > 0 ? (
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- --run src/app/\\(main\\)/review/page.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious && git add frontend/src/app/\(main\)/review/page.tsx frontend/src/app/\(main\)/review/page.test.tsx && git commit -m "feat(review): add error state for IGDB search"
```

---

### Task 5: Run Full Test Suite and Type Check

**Files:**
- None (validation only)

**Step 1: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: No TypeScript errors

**Step 2: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: All tests pass

**Step 3: Fix any issues**

If type errors or test failures, fix them before proceeding.

**Step 4: Final commit if needed**

```bash
cd /home/abo/workspace/home/nexorious && git add -A && git commit -m "fix(review): address type errors and test failures"
```

---

### Task 6: Manual Testing

**Files:**
- None (manual verification)

**Step 1: Start dev server**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run dev`

**Step 2: Test the feature**

1. Navigate to Review page
2. Click "Search IGDB" or "View Options" on an item
3. Scroll to bottom of modal
4. Type a game name (3+ chars)
5. Verify dropdown appears with results
6. Click a result
7. Verify item is matched and modal closes

**Step 3: Test edge cases**

1. Test with 1-2 characters (no results should appear)
2. Test with query that returns no results
3. Test clicking outside dropdown (should stay open until selecting or closing modal)
4. Test slow network (loading state should show)

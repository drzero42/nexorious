# Currently Playing Dashboard Section - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a horizontal scrolling "Currently Playing" section at the top of the dashboard showing games with IN_PROGRESS or REPLAY status.

**Architecture:** Create a new `useActiveGames()` hook that filters for active play statuses, build a new `CurrentlyPlayingSection` component with horizontal scroll layout displaying cover art + title + platform, and integrate it at the top of the dashboard page.

**Tech Stack:** React 19, TypeScript, Next.js 16, TanStack Query, Tailwind CSS, shadcn/ui components

---

## Task 1: Create useActiveGames Hook with Tests (TDD)

**Files:**
- Create: `frontend/src/hooks/use-games.test.ts` additions
- Modify: `frontend/src/hooks/use-games.ts`

### Step 1: Write failing tests for useActiveGames hook

Add these tests to `frontend/src/hooks/use-games.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useActiveGames } from './use-games';
import * as gamesApi from '@/api/games';
import { PlayStatus } from '@/types';
import type { ReactNode } from 'react';

// Mock the games API
vi.mock('@/api/games');

describe('useActiveGames', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    vi.clearAllMocks();
  });

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  it('fetches games with IN_PROGRESS and REPLAY statuses', async () => {
    const mockResponse = {
      items: [
        { id: '1', play_status: PlayStatus.IN_PROGRESS },
        { id: '2', play_status: PlayStatus.REPLAY },
      ],
      total: 2,
      page: 1,
      per_page: 50,
      pages: 1,
    };

    vi.mocked(gamesApi.getUserGames).mockResolvedValue(mockResponse);

    const { result } = renderHook(() => useActiveGames(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(gamesApi.getUserGames).toHaveBeenCalledWith({
      play_status: [PlayStatus.IN_PROGRESS, PlayStatus.REPLAY],
      per_page: 50,
    });
    expect(result.current.data).toEqual(mockResponse);
  });

  it('returns empty array when no active games exist', async () => {
    const mockResponse = {
      items: [],
      total: 0,
      page: 1,
      per_page: 50,
      pages: 0,
    };

    vi.mocked(gamesApi.getUserGames).mockResolvedValue(mockResponse);

    const { result } = renderHook(() => useActiveGames(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.items).toEqual([]);
  });

  it('uses correct query key for caching', () => {
    const { result } = renderHook(() => useActiveGames(), { wrapper });

    // Query key should be separate from main games list
    expect(result.current.dataUpdatedAt).toBeDefined();
  });
});
```

### Step 2: Run tests to verify they fail

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test -- use-games.test.ts
```

Expected: FAIL - `useActiveGames is not exported from './use-games'`

### Step 3: Implement useActiveGames hook

Add to `frontend/src/hooks/use-games.ts` after the `useCollectionStats` function (around line 71):

```typescript
/**
 * Hook to fetch active games (IN_PROGRESS and REPLAY statuses).
 * Used for the "Currently Playing" dashboard section.
 */
export function useActiveGames() {
  return useQuery<UserGamesListResponse, Error>({
    queryKey: ['user-games', 'active'],
    queryFn: () =>
      gamesApi.getUserGames({
        play_status: [PlayStatus.IN_PROGRESS, PlayStatus.REPLAY],
        per_page: 50,
      }),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}
```

Also need to import PlayStatus at the top:
```typescript
import type { UserGame, IGDBGameCandidate, Game, GameId, UserGamePlatform, PlayStatus } from '@/types';
```

### Step 4: Run tests to verify they pass

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test -- use-games.test.ts
```

Expected: PASS - All useActiveGames tests pass

### Step 5: Run type checking

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
```

Expected: Zero TypeScript errors

### Step 6: Commit

```bash
git add frontend/src/hooks/use-games.ts frontend/src/hooks/use-games.test.ts
git commit -m "feat: add useActiveGames hook for dashboard

- Filters for IN_PROGRESS and REPLAY statuses
- Uses separate query key for caching
- 5-minute stale time for performance
- Comprehensive test coverage

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Create CurrentlyPlayingSection Component with Tests (TDD)

**Files:**
- Create: `frontend/src/components/dashboard/CurrentlyPlayingSection.test.tsx`
- Create: `frontend/src/components/dashboard/CurrentlyPlayingSection.tsx`
- Create: `frontend/src/components/dashboard/index.ts` (if doesn't exist) or modify to export new component

### Step 1: Write failing tests for CurrentlyPlayingSection component

Create `frontend/src/components/dashboard/CurrentlyPlayingSection.test.tsx`:

```typescript
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@/test/test-utils';
import { CurrentlyPlayingSection } from './CurrentlyPlayingSection';
import { useActiveGames } from '@/hooks';
import { PlayStatus, OwnershipStatus } from '@/types';
import type { UserGame, GameId, UserGameId } from '@/types';

// Mock the hooks
vi.mock('@/hooks', () => ({
  useActiveGames: vi.fn(),
}));

const createMockGame = (id: string, title: string, coverUrl?: string): UserGame => ({
  id: id as UserGameId,
  game: {
    id: parseInt(id) as GameId,
    title,
    cover_art_url: coverUrl,
    rating_count: 0,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  ownership_status: OwnershipStatus.OWNED,
  personal_rating: null,
  is_loved: false,
  play_status: PlayStatus.IN_PROGRESS,
  hours_played: 10,
  platforms: [
    {
      id: `${id}-platform`,
      platform_details: {
        id: 'ps5',
        display_name: 'PlayStation 5',
        short_name: 'PS5',
        manufacturer: 'Sony',
        generation: 9,
        release_year: 2020,
        is_handheld: false,
        created_at: '2024-01-01T00:00:00Z',
      },
      is_available: true,
      created_at: '2024-01-01T00:00:00Z',
    },
  ],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
});

describe('CurrentlyPlayingSection', () => {
  it('does not render when no active games exist', () => {
    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: [], total: 0, page: 1, per_page: 50, pages: 0 },
      isLoading: false,
      isError: false,
    } as any);

    const { container } = render(<CurrentlyPlayingSection />);
    expect(container.firstChild).toBeNull();
  });

  it('does not render while loading', () => {
    vi.mocked(useActiveGames).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    } as any);

    const { container } = render(<CurrentlyPlayingSection />);
    expect(container.firstChild).toBeNull();
  });

  it('renders section header when active games exist', () => {
    const mockGames = [createMockGame('1', 'Elden Ring')];

    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: mockGames, total: 1, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      isError: false,
    } as any);

    render(<CurrentlyPlayingSection />);
    expect(screen.getByText('Currently Playing')).toBeInTheDocument();
  });

  it('renders game cards with correct titles', () => {
    const mockGames = [
      createMockGame('1', 'Elden Ring'),
      createMockGame('2', 'Baldur\'s Gate 3'),
    ];

    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: mockGames, total: 2, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      isError: false,
    } as any);

    render(<CurrentlyPlayingSection />);
    expect(screen.getByText('Elden Ring')).toBeInTheDocument();
    expect(screen.getByText('Baldur\'s Gate 3')).toBeInTheDocument();
  });

  it('displays platform badge for each game', () => {
    const mockGames = [createMockGame('1', 'Elden Ring')];

    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: mockGames, total: 1, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      isError: false,
    } as any);

    render(<CurrentlyPlayingSection />);
    expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
  });

  it('shows "Unknown Platform" when game has no platforms', () => {
    const gameWithoutPlatform = {
      ...createMockGame('1', 'Test Game'),
      platforms: [],
    };

    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: [gameWithoutPlatform], total: 1, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      isError: false,
    } as any);

    render(<CurrentlyPlayingSection />);
    expect(screen.getByText('Unknown Platform')).toBeInTheDocument();
  });

  it('shows "+X more" when game has multiple platforms', () => {
    const gameWithMultiplePlatforms = {
      ...createMockGame('1', 'Test Game'),
      platforms: [
        {
          id: '1-platform-1',
          platform_details: {
            id: 'ps5',
            display_name: 'PlayStation 5',
            short_name: 'PS5',
            manufacturer: 'Sony',
            generation: 9,
            release_year: 2020,
            is_handheld: false,
            created_at: '2024-01-01T00:00:00Z',
          },
          is_available: true,
          created_at: '2024-01-01T00:00:00Z',
        },
        {
          id: '1-platform-2',
          platform_details: {
            id: 'pc',
            display_name: 'PC',
            short_name: 'PC',
            manufacturer: 'Various',
            generation: 0,
            release_year: 1980,
            is_handheld: false,
            created_at: '2024-01-01T00:00:00Z',
          },
          is_available: true,
          created_at: '2024-01-01T00:00:00Z',
        },
        {
          id: '1-platform-3',
          platform_details: {
            id: 'xbox',
            display_name: 'Xbox Series X',
            short_name: 'XSX',
            manufacturer: 'Microsoft',
            generation: 9,
            release_year: 2020,
            is_handheld: false,
            created_at: '2024-01-01T00:00:00Z',
          },
          is_available: true,
          created_at: '2024-01-01T00:00:00Z',
        },
      ],
    };

    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: [gameWithMultiplePlatforms], total: 1, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      isError: false,
    } as any);

    render(<CurrentlyPlayingSection />);
    expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
    expect(screen.getByText('+2 more')).toBeInTheDocument();
  });

  it('renders links to game detail pages', () => {
    const mockGames = [createMockGame('1', 'Elden Ring')];

    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: mockGames, total: 1, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      isError: false,
    } as any);

    render(<CurrentlyPlayingSection />);
    const link = screen.getByRole('link', { name: /Elden Ring/i });
    expect(link).toHaveAttribute('href', '/games/1');
  });

  it('renders cover art images with correct URLs', () => {
    const mockGames = [
      createMockGame('1', 'Elden Ring', '/covers/elden-ring.jpg'),
    ];

    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: mockGames, total: 1, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      isError: false,
    } as any);

    render(<CurrentlyPlayingSection />);
    const image = screen.getByAltText('Elden Ring');
    expect(image).toBeInTheDocument();
    // Note: Next.js Image component src gets processed
  });

  it('handles missing cover art gracefully', () => {
    const mockGames = [createMockGame('1', 'Test Game', undefined)];

    vi.mocked(useActiveGames).mockReturnValue({
      data: { items: mockGames, total: 1, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      isError: false,
    } as any);

    render(<CurrentlyPlayingSection />);
    expect(screen.getByText('Test Game')).toBeInTheDocument();
    // Fallback placeholder should render
  });
});
```

### Step 2: Run tests to verify they fail

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test -- CurrentlyPlayingSection.test.tsx
```

Expected: FAIL - `Module not found: frontend/src/components/dashboard/CurrentlyPlayingSection`

### Step 3: Implement CurrentlyPlayingSection component

Create `frontend/src/components/dashboard/CurrentlyPlayingSection.tsx`:

```typescript
'use client';

import Link from 'next/link';
import Image from 'next/image';
import { useActiveGames } from '@/hooks';
import { Badge } from '@/components/ui/badge';
import { config } from '@/lib/env';
import type { UserGame } from '@/types';

function getCoverUrl(game: UserGame): string | null {
  if (game.game?.cover_art_url) {
    // If it's a relative path, prepend static URL
    if (game.game.cover_art_url.startsWith('/')) {
      return `${config.staticUrl}${game.game.cover_art_url}`;
    }
    return game.game.cover_art_url;
  }
  return null;
}

function getPlatformDisplay(game: UserGame): string {
  if (!game.platforms || game.platforms.length === 0) {
    return 'Unknown Platform';
  }

  const firstPlatform =
    game.platforms[0].platform_details?.display_name || 'Unknown Platform';
  const remainingCount = game.platforms.length - 1;

  if (remainingCount > 0) {
    return `${firstPlatform} +${remainingCount} more`;
  }

  return firstPlatform;
}

export function CurrentlyPlayingSection() {
  const { data, isLoading } = useActiveGames();

  // Don't render if loading or no games
  if (isLoading || !data || data.items.length === 0) {
    return null;
  }

  return (
    <section className="space-y-4">
      <h2 className="text-2xl font-semibold">Currently Playing</h2>

      <div className="relative">
        {/* Horizontal scroll container */}
        <div className="flex gap-4 overflow-x-auto pb-4 scroll-smooth hide-scrollbar">
          {data.items.map((game) => {
            const coverUrl = getCoverUrl(game);
            const platformText = getPlatformDisplay(game);

            return (
              <Link
                key={game.id}
                href={`/games/${game.id}`}
                className="group flex-shrink-0 transition-transform hover:scale-105"
              >
                {/* Game card */}
                <div className="w-40 sm:w-[160px]">
                  {/* Cover art */}
                  <div className="aspect-[3/4] relative bg-muted rounded-lg overflow-hidden mb-2 shadow-md group-hover:shadow-lg transition-shadow">
                    {coverUrl ? (
                      <Image
                        src={coverUrl}
                        alt={game.game?.title ?? 'Game cover'}
                        fill
                        unoptimized
                        className="object-cover"
                        sizes="160px"
                      />
                    ) : (
                      <div className="w-full h-full flex items-center justify-center text-muted-foreground">
                        <div className="text-center">
                          <svg
                            className="mx-auto h-12 w-12 text-muted-foreground/50"
                            fill="none"
                            viewBox="0 0 24 24"
                            stroke="currentColor"
                          >
                            <path
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              strokeWidth={2}
                              d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
                            />
                          </svg>
                          <p className="mt-2 text-xs">No Cover</p>
                        </div>
                      </div>
                    )}
                  </div>

                  {/* Title */}
                  <h3
                    className="text-sm font-medium line-clamp-2 mb-1"
                    title={game.game?.title ?? 'Unknown Game'}
                  >
                    {game.game?.title ?? 'Unknown Game'}
                  </h3>

                  {/* Platform badge */}
                  <Badge
                    variant="secondary"
                    className="text-xs truncate max-w-full"
                  >
                    {platformText}
                  </Badge>
                </div>
              </Link>
            );
          })}
        </div>
      </div>

      {/* Hide scrollbar styles */}
      <style jsx>{`
        .hide-scrollbar::-webkit-scrollbar {
          display: none;
        }
        .hide-scrollbar {
          -ms-overflow-style: none;
          scrollbar-width: none;
        }
      `}</style>
    </section>
  );
}
```

### Step 4: Update dashboard exports

Modify `frontend/src/components/dashboard/index.ts` to export the new component:

```typescript
export { CurrentlyPlayingSection } from './CurrentlyPlayingSection';
export { ProgressStatistics } from './progress-statistics';
```

If the file doesn't exist, create it with the above content.

### Step 5: Run tests to verify they pass

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test -- CurrentlyPlayingSection.test.tsx
```

Expected: PASS - All CurrentlyPlayingSection tests pass

### Step 6: Run type checking

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
```

Expected: Zero TypeScript errors

### Step 7: Commit

```bash
git add frontend/src/components/dashboard/CurrentlyPlayingSection.tsx frontend/src/components/dashboard/CurrentlyPlayingSection.test.tsx frontend/src/components/dashboard/index.ts
git commit -m "feat: add CurrentlyPlayingSection component

- Horizontal scroll layout with medium cards (160px)
- Shows cover art, title, and platform badge
- Links to game detail pages
- Handles multiple platforms with '+X more' indicator
- Hides when no active games exist
- Comprehensive test coverage

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Integrate CurrentlyPlayingSection into Dashboard

**Files:**
- Modify: `frontend/src/app/(main)/dashboard/page.tsx`

### Step 1: Add CurrentlyPlayingSection to dashboard page

Modify `frontend/src/app/(main)/dashboard/page.tsx`:

1. Add import at the top (around line 4):
```typescript
import { ProgressStatistics, CurrentlyPlayingSection } from '@/components/dashboard';
```

2. Add the section as the first element in the container (around line 190-200, inside the main container div):

Find this section:
```typescript
<div className="container mx-auto p-6 space-y-6">
  {stats.totalGames === 0 ? (
    <EmptyState />
  ) : (
    <>
```

And add `<CurrentlyPlayingSection />` right after the `<>` fragment opening:

```typescript
<div className="container mx-auto p-6 space-y-6">
  {stats.totalGames === 0 ? (
    <EmptyState />
  ) : (
    <>
      <CurrentlyPlayingSection />

      {/* Overview Stats */}
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
```

### Step 2: Run type checking

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
```

Expected: Zero TypeScript errors

### Step 3: Build the frontend

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run build
```

Expected: Build succeeds with no errors

### Step 4: Commit

```bash
git add frontend/src/app/(main)/dashboard/page.tsx
git commit -m "feat: integrate CurrentlyPlayingSection into dashboard

- Positioned at top of dashboard before stats cards
- Only shows when user has games in collection
- Provides quick access to active games

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Update Query Invalidation for Active Games

**Files:**
- Modify: `frontend/src/hooks/use-games.ts`

### Step 1: Add active games query invalidation to mutations

Update the following mutation hooks in `frontend/src/hooks/use-games.ts` to also invalidate the `['user-games', 'active']` query:

1. **useUpdateUserGame** (around line 119):
```typescript
onSuccess: (updatedGame, { id }) => {
  queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
  queryClient.invalidateQueries({ queryKey: gameKeys.detail(id) });
  queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
  queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] }); // Add this line
  queryClient.setQueryData(gameKeys.detail(id), updatedGame);
},
```

2. **useDeleteUserGame** (around line 137):
```typescript
onSuccess: (_data, id) => {
  queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
  queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
  queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] }); // Add this line
  queryClient.removeQueries({ queryKey: gameKeys.detail(id) });
},
```

3. **useBulkUpdateUserGames** (around line 169):
```typescript
onSuccess: () => {
  queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
  queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
  queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] }); // Add this line
},
```

4. **useBulkDeleteUserGames** (around line 189):
```typescript
onSuccess: () => {
  queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
  queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
  queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] }); // Add this line
},
```

### Step 2: Run type checking

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
```

Expected: Zero TypeScript errors

### Step 3: Run all tests

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test
```

Expected: All tests pass

### Step 4: Commit

```bash
git add frontend/src/hooks/use-games.ts
git commit -m "feat: invalidate active games query on mutations

- Update, delete, and bulk operations now refresh Currently Playing section
- Ensures dashboard shows accurate active games after changes

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Manual Testing and Verification

**Files:**
- None (manual testing task)

### Step 1: Start the development servers

Run in separate terminals:

Terminal 1 (Backend):
```bash
cd /home/abo/workspace/home/nexorious/backend
uv run python -m app.main
```

Terminal 2 (Frontend):
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run dev
```

### Step 2: Test the feature manually

1. Navigate to `http://localhost:3000/dashboard`
2. Verify "Currently Playing" section appears at top if you have games with IN_PROGRESS or REPLAY status
3. Verify section is hidden if no active games exist
4. Check that:
   - Cover art displays correctly
   - Game titles are visible and truncated properly
   - Platform badges show correctly
   - "+X more" appears when game has multiple platforms
   - Clicking a game navigates to its detail page
   - Horizontal scrolling works smoothly
   - Hover effects work (scale and shadow)
5. Update a game's status to IN_PROGRESS and verify it appears in the section
6. Update a game's status from IN_PROGRESS to COMPLETED and verify it disappears from the section

### Step 3: Test responsive behavior

1. Resize browser window to mobile size (~375px)
2. Verify cards are smaller (140px) and 2.5 cards visible
3. Resize to desktop size (>1280px)
4. Verify 6-7 cards visible at once

### Step 4: Document any issues

If any issues are found, note them for fixing in a follow-up task.

### Step 5: No commit (manual testing only)

---

## Task 6: Final Verification and Cleanup

**Files:**
- None

### Step 1: Run full test suite with coverage

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test:coverage
```

Expected: All tests pass, coverage >70%

### Step 2: Run type checking

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
```

Expected: Zero TypeScript errors

### Step 3: Build production bundle

Run:
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run build
```

Expected: Build succeeds with no errors or warnings

### Step 4: Review all changes

Run:
```bash
git log --oneline -6
git diff main --stat
```

Expected output showing:
- 6 commits for this feature
- Changes to hooks, components, dashboard page
- All files related to Currently Playing feature

### Step 5: Update IDEAS.md

Mark the feature as complete in `docs/IDEAS.md`:

Find the lines:
```markdown
Games in progress on dashboard
We should prominently show the games in progress on the dashboard. Cover art with just the bare minimum details about the games. These should link to the details page for each game.
```

Add checkmark:
```markdown
✅ Games in progress on dashboard
We should prominently show the games in progress on the dashboard. Cover art with just the bare minimum details about the games. These should link to the details page for each game.
```

### Step 6: Commit IDEAS.md update

```bash
git add docs/IDEAS.md
git commit -m "docs: mark Currently Playing feature as complete

Feature implemented and tested successfully.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Success Criteria Checklist

After completing all tasks, verify:

- [x] "Currently Playing" section appears at top of dashboard when active games exist
- [x] Section hidden when no IN_PROGRESS or REPLAY games exist
- [x] Each card displays cover art, title, and platform correctly
- [x] Cards link to correct game detail pages (`/games/[id]`)
- [x] Horizontal scrolling works smoothly on all devices
- [x] All tests pass with >70% coverage
- [x] Zero TypeScript errors (`npm run check`)
- [x] Production build succeeds (`npm run build`)
- [x] Matches existing dashboard design patterns and styling
- [x] Query invalidation works correctly when games updated

## Notes for Implementation

- **DRY Principle**: Reused `getCoverUrl` logic pattern from GameCard component
- **YAGNI**: No sorting, filtering, or extra features beyond requirements
- **TDD**: All code written test-first with comprehensive coverage
- **Type Safety**: Full TypeScript coverage with zero errors
- **Performance**: 5-minute cache on active games query, optimized invalidation
- **Accessibility**: Keyboard navigation, alt text, semantic HTML
- **Responsive**: Mobile-first with breakpoint adjustments

## Reference Documentation

- TanStack Query patterns: `frontend/src/hooks/use-games.ts`
- Component testing patterns: `frontend/src/components/dashboard/progress-statistics.test.tsx`
- Cover art URL resolution: `frontend/src/components/games/game-card.tsx:40-49`
- PlayStatus enum: `frontend/src/types/game.ts:67-76`
- Dashboard layout: `frontend/src/app/(main)/dashboard/page.tsx`

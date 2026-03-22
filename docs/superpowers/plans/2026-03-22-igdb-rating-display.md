# IGDB Rating Display & Sorting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display IGDB community ratings in the game detail view and list view, and enable sorting by IGDB rating on the games list.

**Architecture:** One-line backend change adds `rating_average` to the sortable Game fields. A new `formatIgdbRating` utility converts the stored 0–100 float to a displayed 0.0–10.0 string. The detail view Quick Stats bar and list view get new IGDB rating elements consuming this utility. The sort dropdown in both `game-filters.tsx` and `games/index.tsx` gains a new option.

**Tech Stack:** FastAPI (Python), SQLModel, pytest; React 19, TypeScript, Vitest, @testing-library/react, Tailwind CSS v4, lucide-react, shadcn/ui Table

---

## File Map

| File | Change |
|------|--------|
| `backend/app/api/user_games.py` | Add `'rating_average'` to `game_sort_fields` set |
| `backend/app/tests/test_integration_user_games.py` | New test: sort by rating_average asc/desc, nulls last |
| `frontend/src/lib/game-utils.ts` | Add `formatIgdbRating(value: number \| null \| undefined): string` |
| `frontend/src/lib/game-utils.test.ts` | Unit tests for `formatIgdbRating` |
| `frontend/src/components/games/game-list.tsx` | New IGDB column (header + cell + skeleton) |
| `frontend/src/components/games/game-list.test.tsx` | Tests for IGDB column rendering |
| `frontend/src/components/games/game-filters.tsx` | Add `'rating_average'` to `SortField` type and `sortOptions` |
| `frontend/src/routes/_authenticated/games/index.tsx` | Add `'rating_average'` to `SortField` type and `SORT_OPTIONS` |
| `frontend/src/routes/_authenticated/games/$id.index.tsx` | Add IGDB rating element to Quick Stats bar |
| `frontend/src/routes/_authenticated/games/$id.index.test.tsx` | Tests for IGDB rating in Quick Stats |

---

## Task 1: Backend — Enable Sorting by IGDB Rating

**Files:**
- Modify: `backend/app/api/user_games.py`
- Test: `backend/app/tests/test_integration_user_games.py`

- [ ] **Step 1: Write the failing integration test**

Add a new test method to the `TestUserGamesListEndpoint` class in `backend/app/tests/test_integration_user_games.py`. Follow the pattern of the existing `test_list_user_games_sorting_nulls_last` method directly above it:

```python
def test_list_user_games_sorting_rating_average(self, client: TestClient, session: Session):
    """Test sorting by IGDB rating_average, nulls sort last in both directions."""
    from .integration_test_utils import create_test_game, register_and_login_user
    from ..models.user import User
    from ..models.user_game import UserGame
    from decimal import Decimal

    user_data = {"username": "ratingsortuser", "password": "password123"}
    auth_headers = register_and_login_user(client, user_data)

    user = session.exec(select(User).where(User.username == "ratingsortuser")).first()
    assert user is not None

    game_high = create_test_game(title="High Rated", rating_average=Decimal("85.00"))
    game_low = create_test_game(title="Low Rated", rating_average=Decimal("60.00"))
    game_null = create_test_game(title="Unrated", rating_average=None)

    session.add_all([game_high, game_low, game_null])
    session.commit()
    session.refresh(game_high)
    session.refresh(game_low)
    session.refresh(game_null)

    for game in [game_high, game_low, game_null]:
        user_game = UserGame(
            user_id=user.id,
            game_id=game.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add(user_game)
    session.commit()

    # Ascending: 60.0, 85.0, null
    response = client.get(
        "/api/user-games/?sort_by=rating_average&sort_order=asc",
        headers=auth_headers,
    )
    assert_api_success(response, 200)
    games = response.json()["user_games"]
    ratings = [g["game"]["rating_average"] for g in games]
    assert ratings == [60.0, 85.0, None], f"ASC failed: {ratings}"

    # Descending: 85.0, 60.0, null
    response = client.get(
        "/api/user-games/?sort_by=rating_average&sort_order=desc",
        headers=auth_headers,
    )
    assert_api_success(response, 200)
    games = response.json()["user_games"]
    ratings = [g["game"]["rating_average"] for g in games]
    assert ratings == [85.0, 60.0, None], f"DESC failed: {ratings}"
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd /home/abo/workspace/home/nexorious/backend
uv run pytest app/tests/test_integration_user_games.py::TestUserGamesListEndpoint::test_list_user_games_sorting_rating_average -v
```

Expected: FAIL — the sort returns wrong order because `rating_average` is not in `game_sort_fields`.

- [ ] **Step 3: Add `rating_average` to `game_sort_fields`**

In `backend/app/api/user_games.py`, find the line:
```python
game_sort_fields = {'title', 'genre', 'developer', 'publisher', 'release_date', 'howlongtobeat_main'}
```

Change it to:
```python
game_sort_fields = {'title', 'genre', 'developer', 'publisher', 'release_date', 'howlongtobeat_main', 'rating_average'}
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
cd /home/abo/workspace/home/nexorious/backend
uv run pytest app/tests/test_integration_user_games.py::TestUserGamesListEndpoint::test_list_user_games_sorting_rating_average -v
```

Expected: PASS

- [ ] **Step 5: Run the full backend test suite**

```bash
cd /home/abo/workspace/home/nexorious/backend
uv run pytest --cov=app --cov-report=term-missing
```

Expected: all tests pass, coverage ≥ 80%.

- [ ] **Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git checkout -b feat/igdb-rating-display
git add backend/app/api/user_games.py backend/app/tests/test_integration_user_games.py
git commit -m "feat: enable sorting by IGDB rating_average"
```

---

## Task 2: Frontend Utility — `formatIgdbRating`

**Files:**
- Modify: `frontend/src/lib/game-utils.ts`
- Modify: `frontend/src/lib/game-utils.test.ts`

- [ ] **Step 1: Write the failing unit tests**

In `frontend/src/lib/game-utils.test.ts`, add a new `describe` block after the existing `formatTtb` tests. Import `formatIgdbRating` alongside the existing import:

```typescript
import { describe, it, expect } from 'vitest';
import { formatTtb, formatIgdbRating } from './game-utils';

// ... existing formatTtb tests ...

describe('formatIgdbRating', () => {
  it('converts 85.42 to "8.5"', () => {
    expect(formatIgdbRating(85.42)).toBe('8.5');
  });

  it('converts 72.10 to "7.2"', () => {
    expect(formatIgdbRating(72.10)).toBe('7.2');
  });

  it('converts 100 to "10.0"', () => {
    expect(formatIgdbRating(100)).toBe('10.0');
  });

  it('converts 0 to "0.0"', () => {
    expect(formatIgdbRating(0)).toBe('0.0');
  });

  it('returns em-dash for null', () => {
    expect(formatIgdbRating(null)).toBe('—');
  });

  it('returns em-dash for undefined', () => {
    expect(formatIgdbRating(undefined)).toBe('—');
  });
});
```

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test game-utils.test.ts
```

Expected: FAIL — `formatIgdbRating is not a function` or similar.

- [ ] **Step 3: Implement `formatIgdbRating` in `game-utils.ts`**

In `frontend/src/lib/game-utils.ts`, add after the existing `formatTtb` function:

```typescript
export function formatIgdbRating(value: number | null | undefined): string {
  if (value == null) return '—';
  return (value / 10).toFixed(1);
}
```

- [ ] **Step 4: Run the tests to confirm they pass**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test game-utils.test.ts
```

Expected: all 6 new tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend/src/lib/game-utils.ts frontend/src/lib/game-utils.test.ts
git commit -m "feat: add formatIgdbRating utility"
```

---

## Task 3: List View — IGDB Column

**Files:**
- Modify: `frontend/src/components/games/game-list.tsx`
- Modify: `frontend/src/components/games/game-list.test.tsx`

- [ ] **Step 1: Write failing tests**

In `frontend/src/components/games/game-list.test.tsx`, add a new `describe` block after the existing `rating display` block:

```typescript
describe('IGDB rating display', () => {
  it('renders IGDB column header', () => {
    const games = [createMockGame()];
    render(<GameList games={games} />);

    expect(screen.getByRole('columnheader', { name: 'IGDB' })).toBeInTheDocument();
  });

  it('renders formatted IGDB rating when rating_average is present', () => {
    const games = [
      createMockGame({
        game: {
          ...createMockGame().game,
          rating_average: 85.42,
        },
      }),
    ];
    render(<GameList games={games} />);

    expect(screen.getByText('8.5')).toBeInTheDocument();
  });

  it('renders em-dash when rating_average is null', () => {
    const games = [
      createMockGame({
        game: {
          ...createMockGame().game,
          rating_average: undefined,
        },
      }),
    ];
    render(<GameList games={games} />);

    // The em-dash '—' appears in the IGDB cell
    // (TTB cell may also show '—' so we check all and confirm at least one exists)
    const dashes = screen.getAllByText('—');
    expect(dashes.length).toBeGreaterThan(0);
  });
});
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test game-list.test.tsx
```

Expected: FAIL — "IGDB" column header not found.

- [ ] **Step 3: Add the IGDB column to `game-list.tsx`**

Import `formatIgdbRating` at the top of `frontend/src/components/games/game-list.tsx`, alongside the existing `formatTtb` import:

```typescript
import { formatTtb, formatIgdbRating } from '@/lib/game-utils';
```

In the `TableHeader`, add a new `TableHead` after the existing `Rating` head:

```tsx
<TableHead className="w-20">IGDB</TableHead>
```

In each data row (`games.map(...)`), add a new `TableCell` after the existing personal rating cell:

```tsx
<TableCell>
  <span className="text-sm">{formatIgdbRating(game.game?.rating_average)}</span>
</TableCell>
```

In `GameListSkeleton`, add a new `TableCell` after the last existing one (the Rating skeleton):

```tsx
<TableCell>
  <Skeleton className="h-4 w-12" />
</TableCell>
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test game-list.test.tsx
```

Expected: all tests PASS.

- [ ] **Step 5: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
```

Expected: zero errors.

- [ ] **Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend/src/components/games/game-list.tsx frontend/src/components/games/game-list.test.tsx
git commit -m "feat: add IGDB rating column to list view"
```

---

## Task 4: Sort Option — Filters and Games Index

**Files:**
- Modify: `frontend/src/components/games/game-filters.tsx`
- Modify: `frontend/src/routes/_authenticated/games/index.tsx`

These two files each have their own local `SortField` type. TypeScript will not catch a mismatch between them (structural compatibility), so **both must be updated in the same commit**.

- [ ] **Step 1: Update `game-filters.tsx`**

In `frontend/src/components/games/game-filters.tsx`:

1. Find the `SortField` type (line ~18) and add `'rating_average'`:
```typescript
type SortField = 'title' | 'created_at' | 'howlongtobeat_main' | 'personal_rating' | 'release_date' | 'hours_played' | 'rating_average';
```

2. Find the `sortOptions` array and add a new entry after `'personal_rating'`:
```typescript
{ value: 'rating_average', label: 'IGDB Rating' },
```

Note: `SortOption` in this file has only `value` and `label` — do NOT add `defaultOrder`.

- [ ] **Step 2: Update `games/index.tsx`**

In `frontend/src/routes/_authenticated/games/index.tsx`:

1. Find the `SortField` type (line ~20) and add `'rating_average'`:
```typescript
type SortField = 'title' | 'created_at' | 'howlongtobeat_main' | 'personal_rating' | 'release_date' | 'hours_played' | 'rating_average';
```

2. Find the `SORT_OPTIONS` array and add after the `'personal_rating'` entry:
```typescript
{ value: 'rating_average', label: 'IGDB Rating', defaultOrder: 'desc' },
```

Note: `SortOption` in this file has `value`, `label`, and `defaultOrder` — include all three.

- [ ] **Step 3: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
```

Expected: zero errors.

- [ ] **Step 4: Run frontend tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend/src/components/games/game-filters.tsx frontend/src/routes/_authenticated/games/index.tsx
git commit -m "feat: add IGDB Rating sort option to games list"
```

---

## Task 5: Detail View — IGDB Rating in Quick Stats

**Files:**
- Modify: `frontend/src/routes/_authenticated/games/$id.index.tsx`
- Modify: `frontend/src/routes/_authenticated/games/$id.index.test.tsx`

**Access path note:** In this component, `game` is a `UserGame`. Personal rating is at `game.personal_rating` (top-level), but IGDB rating is at `game.game.rating_average` (nested under the `game` sub-object).

- [ ] **Step 1: Write the failing tests**

In `frontend/src/routes/_authenticated/games/$id.index.test.tsx`, add a new `describe` block. The existing test file uses `mockGame` (defined at the top) and imports the component with `await import('./$id.index')`. Add after the last existing `describe` block:

```typescript
describe('GameDetailPage — IGDB rating display', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    sessionStorage.clear();
    vi.spyOn(Route, 'useParams').mockReturnValue({ id: 'game-123' });
  });

  it('shows IGDB rating in Quick Stats when rating_average is present', async () => {
    const { useUserGame, useDeleteUserGame } = vi.mocked(await import('@/hooks'));
    useUserGame.mockReturnValue({
      data: {
        ...mockGame,
        game: { ...mockGame.game, rating_average: 85.42 },
      },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useUserGame>);
    useDeleteUserGame.mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue(undefined),
    } as unknown as ReturnType<typeof useDeleteUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    expect(screen.getByText('8.5')).toBeInTheDocument();
    expect(screen.getByText('IGDB')).toBeInTheDocument();
  });

  it('does not show IGDB rating when rating_average is null', async () => {
    const { useUserGame, useDeleteUserGame } = vi.mocked(await import('@/hooks'));
    useUserGame.mockReturnValue({
      data: {
        ...mockGame,
        game: { ...mockGame.game, rating_average: null },
      },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useUserGame>);
    useDeleteUserGame.mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue(undefined),
    } as unknown as ReturnType<typeof useDeleteUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    expect(screen.queryByText('IGDB')).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test -- "\$id.index.test.tsx"
```

Expected: FAIL — "8.5" and "IGDB" text not found.

- [ ] **Step 3: Add IGDB rating to the Quick Stats bar in `$id.index.tsx`**

In `frontend/src/routes/_authenticated/games/$id.index.tsx`:

1. Add imports near the top (find existing lucide imports and add `Gamepad2`):
```typescript
import { Heart, ExternalLink, Gamepad2 } from 'lucide-react';
```

2. Add `formatIgdbRating` to the game-utils import (find the existing import from `@/lib/game-utils` or add it):
```typescript
import { formatIgdbRating } from '@/lib/game-utils';
```

3. Find the Quick Stats bar (the `flex` div containing `<Badge>` and `<StarRating>`). It looks like:
```tsx
<div className="flex flex-wrap items-center gap-3">
  <Badge className={getStatusColor(game.play_status)}>
    {formatPlayStatus(game.play_status)}
  </Badge>
  <StarRating value={game.personal_rating} readonly size="md" showLabel />
</div>
```

Add the IGDB rating element after `<StarRating>`:
```tsx
{game.game.rating_average != null && (
  <div className="flex items-center gap-1">
    <Gamepad2 className="h-4 w-4 text-muted-foreground" />
    <span className="text-sm font-medium">
      {formatIgdbRating(game.game.rating_average)}
    </span>
    <span className="text-sm text-muted-foreground">IGDB</span>
  </div>
)}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run test -- "\$id.index.test.tsx"
```

Expected: all tests PASS.

- [ ] **Step 5: Run type check and full test suite**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check && npm run test
```

Expected: zero type errors, all tests pass.

- [ ] **Step 6: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend/src/routes/_authenticated/games/\$id.index.tsx frontend/src/routes/_authenticated/games/\$id.index.test.tsx
git commit -m "feat: show IGDB rating in game detail Quick Stats"
```

---

## Final: Open PR

- [ ] **Push branch and open PR**

```bash
cd /home/abo/workspace/home/nexorious
git push -u origin feat/igdb-rating-display
gh pr create \
  --title "feat: IGDB rating display and sorting" \
  --body "$(cat <<'EOF'
## Summary
- Adds IGDB rating to game detail view Quick Stats bar (e.g. 🎮 8.5 IGDB)
- Adds IGDB column to list view table
- Adds "IGDB Rating" sort option to the games list (default desc)
- One-line backend change enables sorting by rating_average

Closes the IGDB ratings item in the roadmap.

## Test plan
- [ ] Backend integration test: sort by rating_average asc/desc, nulls last
- [ ] Frontend unit tests: formatIgdbRating edge cases
- [ ] Frontend component tests: list column and detail Quick Stats
- [ ] Manual: open a game detail page with a known IGDB rating and verify display
- [ ] Manual: sort games list by "IGDB Rating" and verify order

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Remove completed roadmap item**

In `docs/PRD.md`, delete the IGDB ratings display entry (the one starting with `#### IGDB ratings display and sorting`).

```bash
cd /home/abo/workspace/home/nexorious
git add docs/PRD.md
git commit -m "docs: remove completed IGDB ratings item from roadmap"
git push
```

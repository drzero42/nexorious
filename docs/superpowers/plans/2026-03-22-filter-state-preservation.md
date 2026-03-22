# Filter State Preservation on Back Navigation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve the games list filter/sort/view/pagination state when navigating back from the game detail or edit pages.

**Architecture:** When the user clicks a game from the list, store `window.location.search` (the active filter URL params) in `sessionStorage`. All "Back to Games" navigation reads this stored value and reconstructs a `navigate({ to: '/games', search: ... })` call with the original params. The games list page itself remains URL-driven — sessionStorage is a one-time pointer, not a state store.

**Tech Stack:** React 19, TanStack Router v1, Vitest + @testing-library/react, jsdom (sessionStorage is real in jsdom)

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `frontend/src/routes/_authenticated/games/$id.index.tsx` | Modify | Export `GameDetailPage`; add `navigateToReturnUrl` helper; replace 3 `navigate({ to: '/games' })` calls |
| `frontend/src/routes/_authenticated/games/$id.edit.tsx` | Modify | Export `GameEditPage`; add `navigateToReturnUrl` helper; replace 1 `navigate({ to: '/games' })` call |
| `frontend/src/routes/_authenticated/games/index.tsx` | Modify | Add `sessionStorage.setItem` at the top of `handleClickGame` |
| `frontend/src/routes/_authenticated/games/$id.index.test.tsx` | Create | Tests for `GameDetailPage` navigation: normal back, error-state back, post-delete, fallback (no stored URL) |
| `frontend/src/routes/_authenticated/games/$id.edit.test.tsx` | Create | Tests for `GameEditPage` error-state navigation: stored URL and fallback |

---

## The `navigateToReturnUrl` Helper

This function is **duplicated** in `$id.index.tsx` and `$id.edit.tsx` — no shared module needed.

```ts
function navigateToReturnUrl(navigate: ReturnType<typeof useNavigate>): void {
  const stored = sessionStorage.getItem('games_list_return_url');
  if (!stored) {
    navigate({ to: '/games' });
    return;
  }
  const usp = new URLSearchParams(stored);
  const search: Record<string, string | string[]> = {};
  const seen = new Set<string>();
  usp.forEach((_, key) => {
    if (seen.has(key)) return;
    seen.add(key);
    const vals = usp.getAll(key);
    search[key] = vals.length === 1 ? vals[0] : vals;
  });
  navigate({ to: '/games', search: search as Record<string, string> });
}
```

- Handles multi-value params (e.g. `?platform=steam&platform=gog` → `{ platform: ['steam', 'gog'] }`)
- Falls back to bare `/games` when nothing is stored
- Consistent with the existing `navigate({ to: '/games', search: params as Record<string, string> })` pattern in `index.tsx` — the same cast is already in use, so `npm run check` will not flag it

---

## Task 1: Write failing tests for `$id.index.tsx`

**Files:**
- Create: `frontend/src/routes/_authenticated/games/$id.index.test.tsx`

- [ ] **Step 1: Create the test file**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';

// vi.hoisted ensures mockNavigate is captured by the vi.mock factory below
const { mockNavigate } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
}));

// Override the global setup.ts mock for this test file only
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useParams: () => ({ id: 'game-123' }),
    useSearch: () => ({}),
  };
});

vi.mock('@/hooks', () => ({
  useUserGame: vi.fn(),
  useDeleteUserGame: vi.fn(),
}));

// Minimal mock game — only the fields the component null-checks against
const mockGame = {
  id: 'game-123',
  play_status: 'not_started' as const,
  personal_rating: null,
  is_loved: false,
  hours_played: 0,
  personal_notes: null,
  platforms: [],
  game: {
    id: 1,
    title: 'Test Game',
    cover_art_url: null,
    developer: null,
    publisher: null,
    genre: null,
    release_date: null,
    game_modes: null,
    themes: null,
    player_perspectives: null,
    igdb_slug: null,
    description: null,
    howlongtobeat_main: null,
    howlongtobeat_extra: null,
    howlongtobeat_completionist: null,
  },
};

describe('GameDetailPage — Back to Games navigation', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    sessionStorage.clear();

    const { useUserGame, useDeleteUserGame } = vi.mocked(await import('@/hooks'));
    useUserGame.mockReturnValue({
      data: mockGame,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useUserGame>);
    useDeleteUserGame.mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue(undefined),
    } as unknown as ReturnType<typeof useDeleteUserGame>);
  });

  it('navigates to stored return URL when Back to Games is clicked', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', '?q=foo&status=completed');

    const { GameDetailPage } = await import(
      './$id.index'
    );
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { q: 'foo', status: 'completed' },
    });
  });

  it('navigates to bare /games when no return URL is stored', async () => {
    const user = userEvent.setup();
    // sessionStorage is empty (cleared in beforeEach)

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });

  it('error state Back to Games uses stored return URL', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', '?status=in_progress');

    const { useUserGame } = vi.mocked(await import('@/hooks'));
    useUserGame.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Not found'),
    } as ReturnType<typeof useUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { status: 'in_progress' },
    });
  });

  it('error state Back to Games falls back to /games when no URL stored', async () => {
    const user = userEvent.setup();

    const { useUserGame } = vi.mocked(await import('@/hooks'));
    useUserGame.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Not found'),
    } as ReturnType<typeof useUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });

  it('navigates to stored return URL after deleting a game', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', '?q=rpg');

    const mockMutateAsync = vi.fn().mockResolvedValue(undefined);
    const { useDeleteUserGame } = vi.mocked(await import('@/hooks'));
    useDeleteUserGame.mockReturnValue({
      mutateAsync: mockMutateAsync,
    } as unknown as ReturnType<typeof useDeleteUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    // Open the delete confirmation dialog
    await user.click(screen.getByRole('button', { name: /remove/i }));
    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });

    // Confirm deletion (trigger is now inert behind the modal)
    await user.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalledWith('game-123');
    });
    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { q: 'rpg' },
    });
  });

  it('navigates to bare /games after deleting when no return URL stored', async () => {
    const user = userEvent.setup();

    const mockMutateAsync = vi.fn().mockResolvedValue(undefined);
    const { useDeleteUserGame } = vi.mocked(await import('@/hooks'));
    useDeleteUserGame.mockReturnValue({
      mutateAsync: mockMutateAsync,
    } as unknown as ReturnType<typeof useDeleteUserGame>);

    const { GameDetailPage } = await import('./$id.index');
    render(<GameDetailPage />);

    await user.click(screen.getByRole('button', { name: /remove/i }));
    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });
    await user.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalled();
    });
    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test -- "\$id.index.test.tsx"
```

Expected: FAIL — `GameDetailPage` is not exported from `$id.index.tsx`

---

## Task 2: Implement `$id.index.tsx` changes

**Files:**
- Modify: `frontend/src/routes/_authenticated/games/$id.index.tsx`

- [ ] **Step 1: Export `GameDetailPage` and add the `navigateToReturnUrl` helper**

At the top of `GameDetailPage` function definition, add `export`:
```ts
export function GameDetailPage() {
```

Add `navigateToReturnUrl` directly after the import block (before the helpers):
```ts
function navigateToReturnUrl(navigate: ReturnType<typeof useNavigate>): void {
  const stored = sessionStorage.getItem('games_list_return_url');
  if (!stored) {
    navigate({ to: '/games' });
    return;
  }
  const usp = new URLSearchParams(stored);
  const search: Record<string, string | string[]> = {};
  const seen = new Set<string>();
  usp.forEach((_, key) => {
    if (seen.has(key)) return;
    seen.add(key);
    const vals = usp.getAll(key);
    search[key] = vals.length === 1 ? vals[0] : vals;
  });
  navigate({ to: '/games', search: search as Record<string, string> });
}
```

- [ ] **Step 2: Replace the three `navigate({ to: '/games' })` calls**

In file order:

1. **`handleDelete`** (line ~87):
```ts
// Before:
navigate({ to: '/games' });
// After:
navigateToReturnUrl(navigate);
```

2. **Error state "Back to Games" button** (line ~103):
```tsx
// Before:
<Button onClick={() => navigate({ to: '/games' })}>
// After:
<Button onClick={() => navigateToReturnUrl(navigate)}>
```

3. **Normal header "Back to Games" button** (line ~117):
```tsx
// Before:
<Button variant="outline" onClick={() => navigate({ to: '/games' })}>
// After:
<Button variant="outline" onClick={() => navigateToReturnUrl(navigate)}>
```

- [ ] **Step 3: Run the tests to verify they pass**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test -- "\$id.index.test.tsx"
```

Expected: 6 tests PASS

- [ ] **Step 4: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: 0 errors (ignore the ~29 pre-existing `routeTree.gen.ts` errors — these are expected in this environment per CLAUDE.md)

- [ ] **Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend/src/routes/_authenticated/games/$id.index.tsx \
        frontend/src/routes/_authenticated/games/$id.index.test.tsx
git commit -m "feat: preserve filter state on Back to Games from game detail"
```

---

## Task 3: Write failing tests for `$id.edit.tsx`

**Files:**
- Create: `frontend/src/routes/_authenticated/games/$id.edit.test.tsx`

- [ ] **Step 1: Create the test file**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';

const { mockNavigate } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
}));

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
    useParams: () => ({ id: 'game-123' }),
    useSearch: () => ({}),
  };
});

vi.mock('@/hooks', () => ({
  useUserGame: vi.fn(),
}));

// Mock GameEditForm so it does not render in these error-state-focused tests
vi.mock('@/components/games/game-edit-form', () => ({
  GameEditForm: () => <div data-testid="game-edit-form" />,
}));

describe('GameEditPage — error state Back to Games navigation', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    sessionStorage.clear();

    const { useUserGame } = vi.mocked(await import('@/hooks'));
    // Default: put the component in error/not-found state
    useUserGame.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('Not found'),
    } as ReturnType<typeof useUserGame>);
  });

  it('navigates to stored return URL when Back to Games is clicked in error state', async () => {
    const user = userEvent.setup();
    sessionStorage.setItem('games_list_return_url', '?q=zelda&sort=title');

    const { GameEditPage } = await import('./$id.edit');
    render(<GameEditPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/games',
      search: { q: 'zelda', sort: 'title' },
    });
  });

  it('navigates to bare /games when no return URL is stored', async () => {
    const user = userEvent.setup();

    const { GameEditPage } = await import('./$id.edit');
    render(<GameEditPage />);

    await user.click(screen.getByRole('button', { name: /back to games/i }));

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/games' });
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test -- "\$id.edit.test.tsx"
```

Expected: FAIL — `GameEditPage` is not exported from `$id.edit.tsx`

---

## Task 4: Implement `$id.edit.tsx` changes

**Files:**
- Modify: `frontend/src/routes/_authenticated/games/$id.edit.tsx`

- [ ] **Step 1: Export `GameEditPage` and add `navigateToReturnUrl`**

Add `export` to `GameEditPage`:
```ts
export function GameEditPage() {
```

Add `navigateToReturnUrl` (same function body as in `$id.index.tsx`) after the imports:
```ts
function navigateToReturnUrl(navigate: ReturnType<typeof useNavigate>): void {
  const stored = sessionStorage.getItem('games_list_return_url');
  if (!stored) {
    navigate({ to: '/games' });
    return;
  }
  const usp = new URLSearchParams(stored);
  const search: Record<string, string | string[]> = {};
  const seen = new Set<string>();
  usp.forEach((_, key) => {
    if (seen.has(key)) return;
    seen.add(key);
    const vals = usp.getAll(key);
    search[key] = vals.length === 1 ? vals[0] : vals;
  });
  navigate({ to: '/games', search: search as Record<string, string> });
}
```

- [ ] **Step 2: Replace the one `navigate({ to: '/games' })` in the error state**

```tsx
// Before (error state button, line ~32):
<Button onClick={() => navigate({ to: '/games' })}>
// After:
<Button onClick={() => navigateToReturnUrl(navigate)}>
```

- [ ] **Step 3: Run the tests to verify they pass**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test -- "\$id.edit.test.tsx"
```

Expected: 2 tests PASS

- [ ] **Step 4: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: 0 errors (ignore `routeTree.gen.ts` pre-existing errors)

- [ ] **Step 5: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend/src/routes/_authenticated/games/$id.edit.tsx \
        frontend/src/routes/_authenticated/games/$id.edit.test.tsx
git commit -m "feat: preserve filter state on Back to Games from game edit error state"
```

---

## Task 5: Store the return URL when clicking a game

**Files:**
- Modify: `frontend/src/routes/_authenticated/games/index.tsx`

- [ ] **Step 1: Update `handleClickGame` to write sessionStorage**

In `GamesPageContent`, find `handleClickGame` (line ~215) and add the `sessionStorage.setItem` call before navigating:

```ts
// Before:
const handleClickGame = (game: UserGame) => {
  navigate({ to: '/games/$id', params: { id: game.id } });
};

// After:
const handleClickGame = (game: UserGame) => {
  sessionStorage.setItem('games_list_return_url', window.location.search);
  navigate({ to: '/games/$id', params: { id: game.id } });
};
```

`window.location.search` captures the current URL search string (e.g. `?q=foo&status=completed`) at the moment the user clicks. An empty string (no active filters) is stored as `''`, which causes `navigateToReturnUrl` to fall back to bare `/games` — the correct behaviour.

- [ ] **Step 2: Run the full test suite**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test
```

Expected: All tests PASS (no regressions)

- [ ] **Step 3: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: 0 errors (ignore `routeTree.gen.ts` pre-existing errors)

- [ ] **Step 4: Commit**

```bash
cd /home/abo/workspace/home/nexorious
git add frontend/src/routes/_authenticated/games/index.tsx
git commit -m "feat: save filter state to sessionStorage when navigating to a game"
```

---

## Task 6: Final verification and PR

- [ ] **Step 1: Run full test suite with coverage**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test:coverage
```

Expected: All tests pass. Coverage stays above 70%.

- [ ] **Step 2: Remove the filter state task from the PRD roadmap**

The PRD says items are removed when completed. Remove the following line and its description from `docs/PRD.md`:

```
#### Filter state lost when navigating back from game edit `Medium`
Pressing "Back to Games" from the game edit page clears any active filters on the games list. The filter state should be preserved so the user returns to the same filtered view they left.
```

- [ ] **Step 3: Create the PR**

```bash
cd /home/abo/workspace/home/nexorious
gh pr create \
  --title "feat: preserve filter state when navigating back from game detail/edit" \
  --body "$(cat <<'EOF'
## Summary

- Stores the active games list URL params in `sessionStorage` when clicking a game
- All "Back to Games" buttons (game detail normal/error state, game edit error state, post-delete) navigate back to the original filtered view
- Falls back to bare `/games` when the user arrived via bookmark or direct URL
- The games list page remains URL-driven; sessionStorage is only used to reconstruct the return URL

## Test plan

- [ ] Apply active filters on the games list (search, status, platform, etc.)
- [ ] Click a game → verify the games list URL is preserved in sessionStorage
- [ ] Click "Back to Games" from the game detail → verify you land on the same filtered view
- [ ] Navigate to a game's edit page, then cancel → verify Back to Games from the detail still works
- [ ] Trigger the error state on the edit page → verify Back to Games uses the stored URL
- [ ] Delete a game → verify you land on the same filtered view
- [ ] Open a game URL directly (no previous navigation) → verify Back to Games goes to bare `/games`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

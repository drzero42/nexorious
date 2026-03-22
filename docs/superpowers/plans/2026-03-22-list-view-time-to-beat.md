# List View Time to Beat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Time to Beat column to the games library list view showing `main / extra / completionist` values, matching the card view's existing display.

**Architecture:** Extract the `formatTtb` helper from `game-card.tsx` into a new shared `frontend/src/lib/game-utils.ts` utility, then add a new "Time to Beat" table column to `game-list.tsx` that uses it. Tests-first throughout.

**Tech Stack:** React 19, TypeScript, Vitest, @testing-library/react, lucide-react (Timer icon), shadcn/ui Table components.

---

## Context

- **Card view** already shows TTB: `10h / 20h / 30h` (main / extra / completionist), Timer icon, only when ≥1 value is non-null. Source: `frontend/src/components/games/game-card.tsx`.
- **List view** is a `<Table>` with columns: Cover | Title | Status | Platform(s) | Hours | Rating. Source: `frontend/src/components/games/game-list.tsx`.
- **TTB fields** live on `game.game?.howlongtobeat_main | howlongtobeat_extra | howlongtobeat_completionist` (all `number | undefined`).
- **`formatTtb`** is currently a local unexported function in `game-card.tsx`: returns `"Xh"` if non-null, `"—"` otherwise.
- **`game-utils.ts`** does not yet exist.
- **`game-list.test.tsx`** EXISTS (927 lines) with comprehensive tests. Several assertions check column header counts (currently 6 without checkbox, 7 with) — these will need updating.
- **`game-card.test.tsx`** EXISTS (473 lines) with no TTB-specific tests.

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `frontend/src/lib/game-utils.ts` | **Create** | Shared `formatTtb` utility |
| `frontend/src/lib/game-utils.test.ts` | **Create** | Unit tests for `formatTtb` |
| `frontend/src/components/games/game-card.tsx` | **Modify** | Remove local `formatTtb`, import from `@/lib/game-utils` |
| `frontend/src/components/games/game-card.test.tsx` | **Modify** | Add TTB display tests |
| `frontend/src/components/games/game-list.tsx` | **Modify** | Add Time to Beat column + update skeleton |
| `frontend/src/components/games/game-list.test.tsx` | **Modify** | Add TTB column tests; fix column count assertions |

---

## Task 1: Create `formatTtb` utility with tests

**Files:**
- Create: `frontend/src/lib/game-utils.test.ts`
- Create: `frontend/src/lib/game-utils.ts`

- [ ] **Step 1.1: Write the failing tests**

Create `frontend/src/lib/game-utils.test.ts`:

```typescript
import { describe, it, expect } from 'vitest';
import { formatTtb } from './game-utils';

describe('formatTtb', () => {
  it('formats a whole number of hours', () => {
    expect(formatTtb(10)).toBe('10h');
  });

  it('formats 0 hours', () => {
    expect(formatTtb(0)).toBe('0h');
  });

  it('formats decimal hours', () => {
    expect(formatTtb(12.5)).toBe('12.5h');
  });

  it('returns em-dash for null', () => {
    expect(formatTtb(null)).toBe('—');
  });

  it('returns em-dash for undefined', () => {
    expect(formatTtb(undefined)).toBe('—');
  });
});
```

- [ ] **Step 1.2: Run tests — expect failure**

```bash
cd frontend && npm run test game-utils.test.ts
```

Expected: FAIL — `Cannot find module './game-utils'`

- [ ] **Step 1.3: Create the utility**

Create `frontend/src/lib/game-utils.ts`:

```typescript
export function formatTtb(hours: number | null | undefined): string {
  return hours != null ? `${hours}h` : '—';
}
```

- [ ] **Step 1.4: Run tests — expect pass**

```bash
cd frontend && npm run test game-utils.test.ts
```

Expected: All 5 tests PASS.

- [ ] **Step 1.5: Commit**

```bash
git add frontend/src/lib/game-utils.ts frontend/src/lib/game-utils.test.ts
git commit -m "feat: extract formatTtb into shared game-utils utility"
```

---

## Task 2: Update `game-card.tsx` to use shared `formatTtb`

**Files:**
- Modify: `frontend/src/components/games/game-card.tsx`
- Modify: `frontend/src/components/games/game-card.test.tsx`

- [ ] **Step 2.1: Update `game-card.tsx`**

In `frontend/src/components/games/game-card.tsx`:

Remove the local function (lines 10–12):
```typescript
function formatTtb(hours: number | null | undefined): string {
  return hours != null ? `${hours}h` : '—';
}
```

Add import after the existing imports:
```typescript
import { formatTtb } from '@/lib/game-utils';
```

- [ ] **Step 2.2: Run existing game-card tests — expect pass (no behavior change)**

```bash
cd frontend && npm run test game-card.test.tsx
```

Expected: All existing tests PASS.

- [ ] **Step 2.3: Add TTB display tests to `game-card.test.tsx`**

Add a new `describe` block near the end of the `describe('GameCard', ...)` block (before the closing `}`), after the `'edge cases'` block:

```typescript
describe('time to beat display', () => {
  it('renders TTB row when main value is present', () => {
    const game = createMockGame({
      game: {
        ...createMockGame().game,
        howlongtobeat_main: 10,
        howlongtobeat_extra: 20,
        howlongtobeat_completionist: 30,
      },
    });
    render(<GameCard game={game} />);

    expect(screen.getByText('10h / 20h / 30h')).toBeInTheDocument();
  });

  it('renders em-dash for null TTB values', () => {
    const game = createMockGame({
      game: {
        ...createMockGame().game,
        howlongtobeat_main: 10,
        howlongtobeat_extra: null as unknown as number,
        howlongtobeat_completionist: null as unknown as number,
      },
    });
    render(<GameCard game={game} />);

    expect(screen.getByText('10h / — / —')).toBeInTheDocument();
  });

  it('does not render TTB row when all values are null', () => {
    const game = createMockGame({
      game: {
        ...createMockGame().game,
        howlongtobeat_main: undefined,
        howlongtobeat_extra: undefined,
        howlongtobeat_completionist: undefined,
      },
    });
    render(<GameCard game={game} />);

    // Timer icon should not be present
    expect(screen.queryByText(/h \//)).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2.4: Run game-card tests — expect all pass**

```bash
cd frontend && npm run test game-card.test.tsx
```

Expected: All tests PASS (including the 3 new TTB tests).

- [ ] **Step 2.5: Commit**

```bash
git add frontend/src/components/games/game-card.tsx frontend/src/components/games/game-card.test.tsx
git commit -m "feat: update game-card to use shared formatTtb and add TTB tests"
```

---

## Task 3: Add Time to Beat column to `game-list.tsx` (TDD)

**Files:**
- Modify: `frontend/src/components/games/game-list.test.tsx`
- Modify: `frontend/src/components/games/game-list.tsx`

### Step 3a: Write failing tests first

- [ ] **Step 3.1: Add TTB column tests to `game-list.test.tsx`**

In the `describe('table headers', ...)` block, update the existing column count assertions:

Find and update these two tests:

```typescript
// BEFORE:
it('renders selection column header when onSelectGame is provided', () => {
  // ...
  expect(headers.length).toBe(7);
});

it('does not render selection column header when onSelectGame is not provided', () => {
  // ...
  expect(headers.length).toBe(6);
});
```

```typescript
// AFTER:
it('renders selection column header when onSelectGame is provided', () => {
  const games = [createMockGame()];
  const onSelectGame = vi.fn();
  render(<GameList games={games} onSelectGame={onSelectGame} />);

  // Should have 8 headers: checkbox + Cover + Title + Status + Platform(s) + Hours + Time to Beat + Rating
  const headers = screen.getAllByRole('columnheader');
  expect(headers.length).toBe(8);
});

it('does not render selection column header when onSelectGame is not provided', () => {
  const games = [createMockGame()];
  render(<GameList games={games} />);

  // Should have 7 headers: Cover + Title + Status + Platform(s) + Hours + Time to Beat + Rating
  const headers = screen.getAllByRole('columnheader');
  expect(headers.length).toBe(7);
});
```

Also update `renders all column headers without selection column` and `renders table headers during loading` in the `table headers` block to include "Time to Beat":

```typescript
it('renders all column headers without selection column', () => {
  const games = [createMockGame()];
  render(<GameList games={games} />);

  expect(screen.getByText('Cover')).toBeInTheDocument();
  expect(screen.getByText('Title')).toBeInTheDocument();
  expect(screen.getByText('Status')).toBeInTheDocument();
  expect(screen.getByText('Platform(s)')).toBeInTheDocument();
  expect(screen.getByText('Hours')).toBeInTheDocument();
  expect(screen.getByText('Time to Beat')).toBeInTheDocument();
  expect(screen.getByText('Rating')).toBeInTheDocument();
});
```

```typescript
it('renders table headers during loading', () => {
  render(<GameList games={[]} isLoading={true} />);

  expect(screen.getByText('Cover')).toBeInTheDocument();
  expect(screen.getByText('Title')).toBeInTheDocument();
  expect(screen.getByText('Status')).toBeInTheDocument();
  expect(screen.getByText('Platform(s)')).toBeInTheDocument();
  expect(screen.getByText('Hours')).toBeInTheDocument();
  expect(screen.getByText('Time to Beat')).toBeInTheDocument();
  expect(screen.getByText('Rating')).toBeInTheDocument();
});
```

Then add a new `describe('time to beat display', ...)` block after the `'hours played display'` block:

```typescript
describe('time to beat display', () => {
  it('renders TTB values when all three are present', () => {
    const games = [
      createMockGame({
        game: {
          ...createMockGame().game,
          howlongtobeat_main: 10,
          howlongtobeat_extra: 20,
          howlongtobeat_completionist: 30,
        },
      }),
    ];
    render(<GameList games={games} />);

    expect(screen.getByText('10h / 20h / 30h')).toBeInTheDocument();
  });

  it('renders em-dash for null individual TTB values', () => {
    const games = [
      createMockGame({
        game: {
          ...createMockGame().game,
          howlongtobeat_main: 15,
          howlongtobeat_extra: null as unknown as number,
          howlongtobeat_completionist: null as unknown as number,
        },
      }),
    ];
    render(<GameList games={games} />);

    expect(screen.getByText('15h / — / —')).toBeInTheDocument();
  });

  it('renders em-dash cell when all TTB values are null', () => {
    const games = [
      createMockGame({
        game: {
          ...createMockGame().game,
          howlongtobeat_main: undefined,
          howlongtobeat_extra: undefined,
          howlongtobeat_completionist: undefined,
        },
      }),
    ];
    render(<GameList games={games} />);

    // Should show em-dash in the TTB cell
    const cells = screen.getAllByRole('cell');
    const ttbCell = cells.find((cell) => cell.textContent === '—');
    expect(ttbCell).toBeInTheDocument();
  });

  it('renders TTB column header', () => {
    const games = [createMockGame()];
    render(<GameList games={games} />);

    expect(screen.getByText('Time to Beat')).toBeInTheDocument();
  });
});
```

Also update the stale comment in the `'loading state'` → `'renders 10 skeleton rows'` test from:
```
// There should be 7 skeleton cells per row × 10 rows = 70 skeleton divs
```
to:
```
// There should be 8 skeleton cells per row × 10 rows = 80 skeleton divs
```

- [ ] **Step 3.2: Run tests — expect failures**

```bash
cd frontend && npm run test game-list.test.tsx
```

Expected: FAIL — `Time to Beat` header not found, column count assertions fail, TTB cell not found.

### Step 3b: Implement

- [ ] **Step 3.3: Update `game-list.tsx`**

At the top of `frontend/src/components/games/game-list.tsx`, add these imports after the existing imports:

```typescript
import { Timer } from 'lucide-react';
import { formatTtb } from '@/lib/game-utils';
```

In `GameListSkeleton`, add a new `<TableCell>` after the Hours skeleton cell (the one with `w-12` skeleton), growing the skeleton from 7 to 8 cells. Insert before the final `</TableRow>`:

```tsx
<TableCell>
  <Skeleton className="h-4 w-20" />
</TableCell>
```

In the `<TableHeader>` section, add between the Hours and Rating headers:

```tsx
<TableHead className="w-32">Time to Beat</TableHead>
```

In each game row (in `games.map(...)`), add a new `<TableCell>` between the Hours and Rating cells:

```tsx
<TableCell>
  {game.game?.howlongtobeat_main != null ||
  game.game?.howlongtobeat_extra != null ||
  game.game?.howlongtobeat_completionist != null ? (
    <div className="flex items-center gap-1 text-xs text-muted-foreground">
      <Timer className="h-3 w-3" />
      <span>
        {formatTtb(game.game?.howlongtobeat_main)} /{' '}
        {formatTtb(game.game?.howlongtobeat_extra)} /{' '}
        {formatTtb(game.game?.howlongtobeat_completionist)}
      </span>
    </div>
  ) : (
    <span className="text-sm text-muted-foreground">—</span>
  )}
</TableCell>
```

- [ ] **Step 3.4: Run tests — expect pass**

```bash
cd frontend && npm run test game-list.test.tsx
```

Expected: All tests PASS.

- [ ] **Step 3.5: Commit**

```bash
git add frontend/src/components/games/game-list.tsx frontend/src/components/games/game-list.test.tsx
git commit -m "feat: add Time to Beat column to game list view"
```

---

## Task 4: Final verification

- [ ] **Step 4.1: Run the full frontend test suite**

```bash
cd frontend && npm run test
```

Expected: All tests PASS, coverage ≥70%.

- [ ] **Step 4.2: Run type checking**

```bash
cd frontend && npm run check
```

Expected: Zero TypeScript errors.

- [ ] **Step 4.3: Commit if any fixes were needed, then summarize**

If steps 4.1–4.2 were clean, no additional commit needed. The feature is complete.

# Remember Library Filters and Sorting — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist the library view's full state (filters, sort, view mode, per-page, search, page) to `localStorage` and restore it when the user returns to `/games` with no URL params, and make the existing "Clear" button visible enough to find on mobile.

**Architecture:** The library page already keeps 100% of its state in URL search params. Add a small `localStorage` helper module; write the params object on every change (single chokepoint `updateParams`); on a fresh landing with empty params, hydrate the URL from storage via a one-shot effect. Deep/shared links with explicit params always win because hydration only fires when the URL is empty.

**Tech Stack:** React 19, TanStack Router, TypeScript, Vitest + @testing-library/react.

## Global Constraints

- Frontend gate must stay green: `npm run check` (tsc + eslint) and `npm run knip` (no unused exports) and `npm run test`, all run from `ui/frontend/`.
- No new dependencies.
- localStorage key is exactly `nexorious:library-view:v1` (versioned).
- Follow existing patterns: pure logic lives in `src/lib/*.ts` with a sibling `*.test.ts`; the `@/` import alias maps to `src/`.
- The search query is no longer a special case — the entire view state is persisted (this supersedes the original issue's "search not remembered" line, per the brainstorming outcome).

---

### Task 1: `library-prefs` storage helper

**Files:**
- Create: `ui/frontend/src/lib/library-prefs.ts`
- Test: `ui/frontend/src/lib/library-prefs.test.ts`

**Interfaces:**
- Consumes: the global `localStorage` (stubbed in tests by `src/test/setup.ts`, exported as `localStorageMock`).
- Produces:
  - `type LibrarySearch = Record<string, string | string[]>`
  - `saveLibraryPrefs(search: LibrarySearch): void` — never throws
  - `loadLibraryPrefs(): LibrarySearch | null` — never throws
  - `isEmptySearch(search: LibrarySearch): boolean`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/lib/library-prefs.test.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { saveLibraryPrefs, loadLibraryPrefs, isEmptySearch } from './library-prefs';
import { localStorageMock } from '@/test/setup';

describe('library-prefs', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('saveLibraryPrefs writes JSON under the versioned key', () => {
    saveLibraryPrefs({ status: ['playing'], sort: 'title' });
    expect(localStorageMock.setItem).toHaveBeenCalledWith(
      'nexorious:library-view:v1',
      JSON.stringify({ status: ['playing'], sort: 'title' }),
    );
  });

  it('saveLibraryPrefs swallows write errors (e.g. quota exceeded)', () => {
    localStorageMock.setItem.mockImplementationOnce(() => {
      throw new Error('quota');
    });
    expect(() => saveLibraryPrefs({ sort: 'title' })).not.toThrow();
  });

  it('loadLibraryPrefs round-trips a saved object', () => {
    localStorageMock.getItem.mockReturnValueOnce(
      JSON.stringify({ status: ['playing'], page: '2' }),
    );
    expect(loadLibraryPrefs()).toEqual({ status: ['playing'], page: '2' });
  });

  it('loadLibraryPrefs returns null when nothing is stored', () => {
    localStorageMock.getItem.mockReturnValueOnce(null);
    expect(loadLibraryPrefs()).toBeNull();
  });

  it('loadLibraryPrefs returns null (no throw) on corrupt JSON', () => {
    localStorageMock.getItem.mockReturnValueOnce('{not valid json');
    expect(loadLibraryPrefs()).toBeNull();
  });

  it('loadLibraryPrefs returns null when stored value is not an object', () => {
    localStorageMock.getItem.mockReturnValueOnce(JSON.stringify('a string'));
    expect(loadLibraryPrefs()).toBeNull();
  });

  it('isEmptySearch distinguishes empty from populated params', () => {
    expect(isEmptySearch({})).toBe(true);
    expect(isEmptySearch({ sort: 'title' })).toBe(false);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ui/frontend && npm run test -- library-prefs`
Expected: FAIL — module `./library-prefs` cannot be resolved / exports undefined.

- [ ] **Step 3: Write minimal implementation**

Create `ui/frontend/src/lib/library-prefs.ts`:

```ts
// Library view preferences (filters, sort, view mode, per-page, search, page)
// mirrored to localStorage so the library view is restored across browser
// sessions. See docs/superpowers/specs/2026-06-21-remember-library-view-design.md.

const KEY = 'nexorious:library-view:v1';

export type LibrarySearch = Record<string, string | string[]>;

/** Write the current library search params to localStorage. Never throws. */
export function saveLibraryPrefs(search: LibrarySearch): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(search));
  } catch {
    // Quota exceeded or serialization failure — persistence is best-effort.
  }
}

/** Read saved library search params, or null if absent/corrupt. Never throws. */
export function loadLibraryPrefs(): LibrarySearch | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as LibrarySearch;
    }
    return null;
  } catch {
    return null;
  }
}

/** True when the search params object carries no keys. */
export function isEmptySearch(search: LibrarySearch): boolean {
  return Object.keys(search).length === 0;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ui/frontend && npm run test -- library-prefs`
Expected: PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/lib/library-prefs.ts ui/frontend/src/lib/library-prefs.test.ts
git commit -m "feat(ui): add library-prefs localStorage helper (#1129)"
```

---

### Task 2: Persist and hydrate in the library route

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/games/index.tsx`

**Interfaces:**
- Consumes from Task 1: `saveLibraryPrefs`, `loadLibraryPrefs`, `isEmptySearch`, `type LibrarySearch`.

**Why no unit test here:** `useSearch` is globally mocked to return `{}` and the page wires many hooks + MSW handlers, so a faithful hydration render test is impractical and low value. The decision logic (`isEmptySearch`) and storage (`save`/`load`) are fully unit-tested in Task 1; this task is thin wiring whose correctness is covered by tsc/eslint/knip plus the manual smoke check below. (Per the repo testing policy, thin wrappers are not unit-tested.)

- [ ] **Step 1: Add the import**

At the top of `ui/frontend/src/routes/_authenticated/games/index.tsx`, add to the import block:

```ts
import {
  saveLibraryPrefs,
  loadLibraryPrefs,
  isEmptySearch,
  type LibrarySearch,
} from '@/lib/library-prefs';
```

- [ ] **Step 2: Persist on every param change**

In `updateParams` (currently around `index.tsx:127-143`), add the `saveLibraryPrefs(params)` call immediately before `navigate(...)`. The function becomes:

```ts
  const updateParams = useCallback(
    (updates: Record<string, string | string[] | undefined>) => {
      const currentSearch = search as Record<string, string | string[]>;
      const params: Record<string, string | string[]> = { ...currentSearch };

      Object.entries(updates).forEach(([key, value]) => {
        if (value === undefined || value === '' || (Array.isArray(value) && value.length === 0)) {
          delete params[key];
        } else {
          params[key] = value;
        }
      });

      saveLibraryPrefs(params);
      navigate({ to: '/games', search: params as Record<string, string>, replace: true });
    },
    [navigate, search],
  );
```

- [ ] **Step 3: Hydrate on a fresh landing**

Add this effect immediately after `updateParams` is defined (before `filterFields`). It runs once on mount; if the URL has no params and storage holds a non-empty saved view, it replaces the URL with the saved params:

```ts
  // On a fresh landing with no URL params (e.g. the sidebar "Library" link),
  // restore the last-used view from localStorage. Explicit/deep-linked params
  // always win — we only hydrate when the URL carries nothing. Runs once on
  // mount, so it never fights a user's in-session navigation.
  useEffect(() => {
    if (!isEmptySearch(search as LibrarySearch)) return;
    const saved = loadLibraryPrefs();
    if (saved && !isEmptySearch(saved)) {
      navigate({ to: '/games', search: saved as Record<string, string>, replace: true });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
```

(`useEffect` is already imported on `index.tsx:2`.)

- [ ] **Step 4: Verify the frontend gate passes**

Run: `cd ui/frontend && npm run check && npm run knip`
Expected: no type errors, no lint errors, no knip findings (the three new exports are all consumed here).

- [ ] **Step 5: Manual smoke check**

Run the app (`make frontend` then `./nexorious serve`, or the dev server) and verify:
1. Apply a filter + change sort on `/games`. Navigate away (Dashboard) and back via the sidebar "Library" link → the filter and sort are restored.
2. Open `/games?status=playing` directly → it shows playing games (storage did **not** override the explicit param).
3. Click "Clear" → reload `/games` from the sidebar → the cleared (empty) state is what returns, not the old filters.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/games/index.tsx
git commit -m "feat(ui): remember library filters and sorting across sessions (#1129)"
```

---

### Task 3: Make the Clear button visible

**Files:**
- Modify: `ui/frontend/src/components/games/game-filters.tsx:286`

**Interfaces:** none (styling-only change).

**Why no new test:** the change is `variant` only; behavior is unchanged and already covered by `game-filters.test.tsx`. Run that suite to confirm no regression.

- [ ] **Step 1: Change the button variant**

In `ui/frontend/src/components/games/game-filters.tsx`, the "Clear filters" button (around line 286) currently reads `variant="ghost"`. Change it to `variant="outline"` so it has a visible border/background like the adjacent "More filters" button:

```tsx
        {/* Clear filters */}
        {hasActiveFilters && (
          <Button variant="outline" size="sm" onClick={clearFilters}>
            <X className="h-4 w-4 mr-1" />
            Clear
          </Button>
        )}
```

- [ ] **Step 2: Run the filters test suite**

Run: `cd ui/frontend && npm run test -- game-filters`
Expected: PASS (existing Clear-button behavior still works).

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/games/game-filters.tsx
git commit -m "fix(ui): make the library Clear-filters button easier to spot (#1129)"
```

---

## Self-Review

**Spec coverage:**
- Persist full view state → Task 1 (storage) + Task 2 (write in `updateParams`). ✓
- Restore on fresh landing, deep links win → Task 2 hydration effect guarded by `isEmptySearch`. ✓
- Search + page remembered (updated requirement) → whole params object persisted, nothing carved out. ✓
- Clear button visibility → Task 3. ✓
- No backend/migration → none in any task. ✓
- Versioned key `nexorious:library-view:v1` → Task 1. ✓

**Placeholder scan:** none — every step has concrete code/commands.

**Type consistency:** `LibrarySearch`, `saveLibraryPrefs`, `loadLibraryPrefs`, `isEmptySearch` are defined in Task 1 and consumed with the same names/signatures in Task 2. ✓

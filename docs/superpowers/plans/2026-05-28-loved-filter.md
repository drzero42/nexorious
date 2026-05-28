# Loved filter — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-05-28-loved-filter-design.md`](../specs/2026-05-28-loved-filter-design.md)
**Issue:** [#600](https://github.com/Nexorious/nexorious/issues/600)
**Branch:** `feat/loved-filter` (already created)

**Goal:** Add a UI control to the games library page that filters games by their `is_loved` flag, using the existing backend filter and API client wiring.

**Architecture:** Two-file frontend change. `game-filters.tsx` gains a three-option `<Select>` in the primary filter row, mirroring the existing Play Status / Ownership controls. The games index route parses a new `?loved=true|false` URL param and threads `isLoved` through `filterFields` so it reaches `useUserGames` and `useUserGameIds` (which already serialize it to the backend's `?is_loved` param).

**Tech Stack:** React 19, TypeScript, TanStack Router/Query, shadcn/ui `Select`. No backend, no API client, no migration, no new dependencies.

**No new tests:** Per project policy (`CLAUDE.md` → "Do NOT write a test when the function is a thin wrapper… only verifies that calling the function returns what it computes"), this UI-wiring change does not warrant new tests. The backend `ApplyIsLoved` filter is already in place.

---

## File structure

| File | Responsibility | Action |
|---|---|---|
| `ui/frontend/src/components/games/game-filters.tsx` | Filter UI control + prop type | Modify |
| `ui/frontend/src/routes/_authenticated/games/index.tsx` | URL ↔ filter state plumbing | Modify |

The split is by responsibility, not technical layer. Both files must land in the same commit because the `GameFiltersProps['filters']` shape and the route's `handleFiltersChange` type need to agree, and TypeScript will otherwise fail the build at the boundary between them.

---

## Task 1: Wire the loved filter end-to-end

**Files:**
- Modify: `ui/frontend/src/components/games/game-filters.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/games/index.tsx`

### - [ ] Step 1: Add `isLoved?: boolean` to `GameFiltersProps['filters']`

**File:** `ui/frontend/src/components/games/game-filters.tsx`

In the `GameFiltersProps` interface (currently lines 51–72), insert `isLoved?: boolean;` between `ownershipStatus` and `platformId` so the loved field sits with the other status-like fields:

```typescript
export interface GameFiltersProps {
  filters: {
    search: string;
    status?: PlayStatus;
    ownershipStatus?: OwnershipStatus; // Filter by ownership status (matches if ANY platform has this status)
    isLoved?: boolean; // Filter by whether the game is marked as loved
    platformId?: string; // Keep for backwards compat (but will migrate to platforms)
    platforms?: string[]; // New: multi-select
    storefronts?: string[]; // New
    genres?: string[]; // New
    gameModes?: string[]; // New: game modes from IGDB
    themes?: string[]; // New: themes from IGDB
    playerPerspectives?: string[]; // New: player perspectives from IGDB
    tags?: string[]; // New
  };
  // ... rest unchanged
}
```

### - [ ] Step 2: Render the loved `<Select>` in the primary filters row

**File:** `ui/frontend/src/components/games/game-filters.tsx`

In the primary filters row, insert a new `<Select>` block **directly after the Ownership Status `<Select>` (the block ending at line 286) and before the Platform `<MultiSelectFilter>` (currently line 288)**. Use a sentinel-based mapping: `"all"`, `"true"`, `"false"` are the `<Select>` values; the filter shape uses `undefined | true | false`.

```tsx
        {/* Loved filter */}
        <Select
          value={filters.isLoved === undefined ? 'all' : String(filters.isLoved)}
          onValueChange={(value) =>
            onFiltersChange({
              ...filters,
              isLoved: value === 'all' ? undefined : value === 'true',
            })
          }
        >
          <SelectTrigger className="w-36">
            <SelectValue placeholder="Loved" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All games</SelectItem>
            <SelectItem value="true">Loved only</SelectItem>
            <SelectItem value="false">Not loved</SelectItem>
          </SelectContent>
        </Select>
```

### - [ ] Step 3: Include `isLoved` in `hasActiveFilters`

**File:** `ui/frontend/src/components/games/game-filters.tsx`

Replace the existing `hasActiveFilters` expression (lines 138–149) with one that also accounts for `isLoved`. The check is `!== undefined` (not truthy), because `false` is a meaningful active value:

```typescript
  const hasActiveFilters =
    filters.search ||
    filters.status ||
    filters.ownershipStatus ||
    filters.isLoved !== undefined ||
    filters.platformId ||
    (filters.platforms && filters.platforms.length > 0) ||
    (filters.storefronts && filters.storefronts.length > 0) ||
    (filters.genres && filters.genres.length > 0) ||
    (filters.gameModes && filters.gameModes.length > 0) ||
    (filters.themes && filters.themes.length > 0) ||
    (filters.playerPerspectives && filters.playerPerspectives.length > 0) ||
    (filters.tags && filters.tags.length > 0);
```

### - [ ] Step 4: Reset `isLoved` in `clearFilters`

**File:** `ui/frontend/src/components/games/game-filters.tsx`

Replace the `clearFilters` function body (lines 151–165) so it also resets `isLoved` to `undefined`:

```typescript
  const clearFilters = () => {
    onFiltersChange({
      search: '',
      status: undefined,
      ownershipStatus: undefined,
      isLoved: undefined,
      platformId: undefined,
      platforms: [],
      storefronts: [],
      genres: [],
      gameModes: [],
      themes: [],
      playerPerspectives: [],
      tags: [],
    });
  };
```

### - [ ] Step 5: Parse `?loved` URL param into `filters.isLoved`

**File:** `ui/frontend/src/routes/_authenticated/games/index.tsx`

In the `filters` `useMemo` (currently lines 57–82), parse the `loved` URL param. Add the parsing right after `ownershipStatus` is computed, and include `isLoved` in the returned object:

```typescript
  // Read filters from URL params
  const filters = useMemo(() => {
    const statusParam = (search as Record<string, string>)['status'];
    const ownershipParam = (search as Record<string, string>)['ownership'];
    // Handle "null" string or empty string as undefined
    const status = statusParam && statusParam !== 'null' ? (statusParam as PlayStatus) : undefined;
    const ownershipStatus =
      ownershipParam && ownershipParam !== 'null' ? (ownershipParam as OwnershipStatus) : undefined;
    const lovedParam = (search as Record<string, string>)['loved'];
    const isLoved =
      lovedParam === 'true' ? true : lovedParam === 'false' ? false : undefined;
    const s = search as Record<string, string | string[]>;
    const getAll = (key: string): string[] => {
      const val = s[key];
      if (!val) return [];
      return Array.isArray(val) ? val : [val];
    };
    return {
      search: (s['q'] as string) ?? '',
      status,
      ownershipStatus,
      isLoved,
      platforms: getAll('platform'),
      storefronts: getAll('storefront'),
      genres: getAll('genre'),
      gameModes: getAll('gameMode'),
      themes: getAll('theme'),
      playerPerspectives: getAll('playerPerspective'),
      tags: getAll('tag'),
    };
  }, [search]);
```

### - [ ] Step 6: Pass `isLoved` through `filterFields`

**File:** `ui/frontend/src/routes/_authenticated/games/index.tsx`

In the `filterFields` `useMemo` (currently lines 112–127), add `isLoved` so it flows into `useUserGames` / `useUserGameIds` (which already accept it as `GetUserGamesParams.isLoved`):

```typescript
  // Shared filter fields — no pagination params
  const filterFields = useMemo(
    () => ({
      search: filters.search || undefined,
      status: filters.status,
      ownershipStatus: filters.ownershipStatus,
      isLoved: filters.isLoved,
      platform: filters.platforms.length > 0 ? filters.platforms : undefined,
      storefront: filters.storefronts.length > 0 ? filters.storefronts : undefined,
      genre: filters.genres.length > 0 ? filters.genres : undefined,
      gameMode: filters.gameModes.length > 0 ? filters.gameModes : undefined,
      theme: filters.themes.length > 0 ? filters.themes : undefined,
      playerPerspective:
        filters.playerPerspectives.length > 0 ? filters.playerPerspectives : undefined,
      tags: filters.tags.length > 0 ? filters.tags : undefined,
    }),
    [filters],
  );
```

### - [ ] Step 7: Update `handleFiltersChange` to accept and serialize `isLoved`

**File:** `ui/frontend/src/routes/_authenticated/games/index.tsx`

Replace the `handleFiltersChange` callback (currently lines 167–197). Add `isLoved?: boolean` to the inline parameter type and add a `loved` entry to `updateParams`. The serialization mirrors the parser: `true` / `false` → `"true"` / `"false"`, `undefined` → drop the param:

```typescript
  // Wrap filter changes to also clear selection and update URL
  const handleFiltersChange = useCallback(
    (newFilters: {
      search: string;
      status?: PlayStatus;
      ownershipStatus?: OwnershipStatus;
      isLoved?: boolean;
      platforms?: string[];
      storefronts?: string[];
      genres?: string[];
      gameModes?: string[];
      themes?: string[];
      playerPerspectives?: string[];
      tags?: string[];
    }) => {
      updateParams({
        q: newFilters.search || undefined,
        status: newFilters.status,
        ownership: newFilters.ownershipStatus,
        loved: newFilters.isLoved === undefined ? undefined : String(newFilters.isLoved),
        platform: newFilters.platforms,
        storefront: newFilters.storefronts,
        genre: newFilters.genres,
        gameMode: newFilters.gameModes,
        theme: newFilters.themes,
        playerPerspective: newFilters.playerPerspectives,
        tag: newFilters.tags,
        page: undefined,
      });
      setSelectedIds(new Set());
      setSelectionMode('manual');
    },
    [updateParams],
  );
```

### - [ ] Step 8: Run the TypeScript type check

Run: `cd ui/frontend && npm run check`
Expected: exit 0, no errors. If `tsc` complains about missing properties on the filter type, revisit Step 1 / Step 7.

### - [ ] Step 9: Run knip (dead-code check)

Run: `cd ui/frontend && npm run knip`
Expected: exit 0. No new unused exports — the change adds no new components.

### - [ ] Step 10: Build the frontend

Run: `make frontend` (from repo root)
Expected: build completes; `ui/frontend/dist/` is populated.

### - [ ] Step 11: Smoke-test the filter manually

Start the server (`./nexorious` after `make build` if needed) and verify, in order:
1. Open `/games`. Confirm a new "Loved" `<Select>` appears between Ownership and Platforms.
2. Pick "Loved only" — the list narrows to games where the heart is set; URL gains `?loved=true`; the "Clear" button appears (acceptance criterion 4 — indicator activates on `isLoved` alone).
3. Pick "Not loved" — list shows the complement; URL becomes `?loved=false`; "Clear" still visible.
4. Reload — selection survives.
5. Click "Clear" — the loved select returns to "All games", `?loved` disappears from the URL, "Clear" hides.
6. Manually edit the URL to `?loved=garbage` and reload — the select shows "All games" (parser falls through).

If any step misbehaves, fix it before committing.

### - [ ] Step 12: Commit

```bash
git add ui/frontend/src/components/games/game-filters.tsx \
        ui/frontend/src/routes/_authenticated/games/index.tsx
git commit -m "feat: add loved filter to games library (#600)"
```

The branch is already `feat/loved-filter`. After the commit, the next steps (push, PR) are handled by the user or by the `commit-commands:commit-push-pr` skill — they are NOT part of this plan.

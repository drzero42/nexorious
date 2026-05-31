# Issue #650: DRY — GameFiltersValue Type Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the duplicated inline filter shape in `handleFiltersChange` by exporting a named type alias from `game-filters.tsx` and using it everywhere.

**Architecture:** Export `GameFiltersValue = GameFiltersProps['filters']` from `game-filters.tsx`, re-export it through the barrel, and import it in the route file to replace the inline type. Zero behavioral change.

**Tech Stack:** TypeScript, React, Vite

---

### Task 1: Create a feature branch

**Files:**
- No file changes

- [ ] **Step 1: Create and switch to a feature branch**

```bash
git checkout -b fix/650-dry-game-filters-value-type
```

Expected: switched to new branch `fix/650-dry-game-filters-value-type`

---

### Task 2: Export `GameFiltersValue` from `game-filters.tsx` and the barrel

**Files:**
- Modify: `ui/frontend/src/components/games/game-filters.tsx:65`
- Modify: `ui/frontend/src/components/games/index.ts`

- [ ] **Step 1: Add the type alias in `game-filters.tsx`**

After line 65 (the closing `};` of the `filters` inline type inside `GameFiltersProps`), insert one line. The interface currently ends at line 65 with the `onFiltersChange` line at 66. Insert after line 65 (`};` that closes the `filters` block — i.e., after the closing brace of `GameFiltersProps`, which is at line 70):

Open `ui/frontend/src/components/games/game-filters.tsx`. After the full `GameFiltersProps` interface (which ends at line 70), add:

```ts
export type GameFiltersValue = GameFiltersProps['filters'];
```

The result around that area should look like:

```ts
export interface GameFiltersProps {
  filters: {
    search: string;
    status?: PlayStatus;
    ownershipStatus?: OwnershipStatus;
    isLoved?: boolean;
    platformId?: string;
    platforms?: string[];
    storefronts?: string[];
    genres?: string[];
    gameModes?: string[];
    themes?: string[];
    playerPerspectives?: string[];
    tags?: string[];
  };
  onFiltersChange: (filters: GameFiltersProps['filters']) => void;
  viewMode: 'grid' | 'list';
  onViewModeChange: (mode: 'grid' | 'list') => void;
  sortBy: SortField;
  sortOrder: SortOrder;
  // ... rest of interface
}

export type GameFiltersValue = GameFiltersProps['filters'];
```

- [ ] **Step 2: Re-export `GameFiltersValue` through the barrel**

Open `ui/frontend/src/components/games/index.ts`. The current content is:

```ts
export { BulkActions } from './bulk-actions';
export { GameFilters } from './game-filters';
export { GameGrid } from './game-grid';
export { GameList } from './game-list';
export { GamesPagination } from './game-pagination';
```

Change the `game-filters` line to also export the new type:

```ts
export { BulkActions } from './bulk-actions';
export { GameFilters } from './game-filters';
export type { GameFiltersValue } from './game-filters';
export { GameGrid } from './game-grid';
export { GameList } from './game-list';
export { GamesPagination } from './game-pagination';
```

- [ ] **Step 3: Verify TypeScript accepts the new export**

```bash
cd ui/frontend && npm run check
```

Expected: no errors

---

### Task 3: Replace the inline type in `games/index.tsx`

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/games/index.tsx:5,8,172-184`

- [ ] **Step 1: Add `GameFiltersValue` to the existing import**

Line 5 currently reads:

```ts
import { GameFilters, GameGrid, GameList, BulkActions, GamesPagination } from '@/components/games';
```

Change it to:

```ts
import { GameFilters, GameGrid, GameList, BulkActions, GamesPagination, type GameFiltersValue } from '@/components/games';
```

- [ ] **Step 2: Replace the inline type in `handleFiltersChange`**

Lines 172–184 currently read:

```ts
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
```

Replace with:

```ts
const handleFiltersChange = useCallback(
  (newFilters: GameFiltersValue) => {
```

- [ ] **Step 3: Verify `PlayStatus` and `OwnershipStatus` are still needed**

Check line 8:

```ts
import type { PlayStatus, OwnershipStatus, UserGame, SelectionMode } from '@/types';
```

`PlayStatus` is used on line 62 (`statusParam as PlayStatus`) and `OwnershipStatus` on line 64 (`ownershipParam as OwnershipStatus`) — both are still needed. Leave the import unchanged.

- [ ] **Step 4: Run type check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/games/game-filters.tsx \
        ui/frontend/src/components/games/index.ts \
        ui/frontend/src/routes/_authenticated/games/index.tsx
git commit -m "fix: extract GameFiltersValue type to eliminate duplicate filter shape (#650)"
```

---

### Task 4: Open a PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin fix/650-dry-game-filters-value-type
```

- [ ] **Step 2: Open a PR**

```bash
gh pr create \
  --title "fix: extract GameFiltersValue type to eliminate duplicate filter shape" \
  --body "$(cat <<'EOF'
Closes #650

Exports `GameFiltersValue = GameFiltersProps['filters']` from `game-filters.tsx` and re-exports it through the barrel, then replaces the identical inline object type in `handleFiltersChange` with the named alias.

No behavioral change.
EOF
)"
```

# Issue #650: DRY — Extract `GameFiltersValue` type

## Problem

`handleFiltersChange` in `games/index.tsx` declares an inline object type that duplicates the `filters` shape already defined in `GameFiltersProps` in `game-filters.tsx`. Every time a new filter field is added, both places must be updated and they can drift.

## Design

**Single source of truth:** Export a named type alias from `game-filters.tsx` and use it everywhere the filter shape is referenced.

### Changes

**`ui/frontend/src/components/games/game-filters.tsx`**

Add immediately after the `GameFiltersProps` interface (line 65):

```ts
export type GameFiltersValue = GameFiltersProps['filters'];
```

**`ui/frontend/src/components/games/index.ts`**

Add `GameFiltersValue` to the barrel re-export:

```ts
export type { GameFiltersValue } from './game-filters';
```

**`ui/frontend/src/routes/_authenticated/games/index.tsx`**

Import `GameFiltersValue` from `@/components/games` and replace the inline type on `handleFiltersChange`:

```ts
import { GameFilters, GameGrid, GameList, BulkActions, GamesPagination, type GameFiltersValue } from '@/components/games';

const handleFiltersChange = useCallback(
  (newFilters: GameFiltersValue) => { ... },
  [updateParams],
);
```

Check whether `PlayStatus` and `OwnershipStatus` are still referenced elsewhere in the file; remove them from the `@/types` import if they are no longer needed.

## Scope

Pure refactor — no behavioral change, no API change, no new tests required.

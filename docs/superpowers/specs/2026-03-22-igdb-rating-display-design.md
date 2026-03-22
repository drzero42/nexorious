# IGDB Rating Display & Sorting — Design Spec

**Date:** 2026-03-22
**Status:** Approved
**Scope:** Frontend display + sort option + one backend sort field addition

---

## Background

`rating_average` (IGDB community rating, 0–100 integer scale) is stored in the `Game` DB table and returned by the API in all game responses. It was never wired up in the frontend after the rewrite. This spec adds the display and sort capability.

---

## Formatting Convention

IGDB's `rating` field (sourced in `backend/app/services/igdb/parser.py` as `game_data.get('rating')`) is a float on a 0–100 scale, stored as-is in the `Numeric(5,2)` DB column. Real values look like `85.42`, `72.10`, etc. The PRD specifies displaying as 0.0–10.0 with one decimal place.

**Note:** Frontend test fixtures use values like `8.5` and `4.5` for `rating_average` — these are arbitrary and do not represent the real 0–100 stored scale. Do not infer the scale from test mocks.

**Formula:** `(rating_average / 10).toFixed(1)` — e.g. `85.42 → "8.5"`, `72.10 → "7.2"`

A shared helper `formatIgdbRating(value: number | null | undefined): string` is added to `frontend/src/lib/game-utils.ts` alongside the existing `formatTtb`. Following the same pattern as `formatTtb`, the helper handles null/undefined internally and returns `"—"` (em dash) in those cases. This allows callsites to pass `game.game?.rating_average` directly without a separate null guard.

---

## Changes

### 1. Backend — `backend/app/api/user_games.py`

Add `'rating_average'` to the `game_sort_fields` set (find it by searching for `'howlongtobeat_main'` in the same set — do not rely on line numbers):

```python
game_sort_fields = {
    'title', 'genre', 'developer', 'publisher',
    'release_date', 'howlongtobeat_main', 'rating_average'  # added
}
```

`rating_average` is on the `Game` model, so the existing join logic handles it automatically. Nulls already sort to the end (existing `nulls_last` behavior).

---

### 2. Frontend — Utility

**File:** `frontend/src/lib/game-utils.ts`

Add:
```ts
export function formatIgdbRating(value: number | null | undefined): string {
  if (value == null) return '—';
  return (value / 10).toFixed(1);
}
```

Following the `formatTtb` precedent, the helper handles null/undefined internally so callsites can pass `game.game?.rating_average` directly.

---

### 3. Frontend — Game Detail View

**File:** `frontend/src/routes/_authenticated/games/$id.index.tsx`

The data object is `game` (a `UserGame`). Note the access path asymmetry: `personal_rating` is accessed as `game.personal_rating` (top-level on `UserGame`), while `rating_average` lives on the nested game record at `game.game.rating_average`.

The Quick Stats bar currently shows:
```
[Status badge]  [★★★★☆ My Rating]
```

The Quick Stats bar is a `flex` row at line ~207 containing `<Badge>` (play status) and `<StarRating>` (personal rating). Append the IGDB element after `<StarRating>`, only when `game.game.rating_average != null`:

```
[Status badge]  [★★★★☆ My Rating]  [🎮 8.5 IGDB]
```

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

Import `Gamepad2` from `lucide-react` and `formatIgdbRating` from `@/lib/game-utils`.

---

### 4. Frontend — List View Column

**File:** `frontend/src/components/games/game-list.tsx`

The `game` variable in the list is a `UserGame`. Use optional chaining (`game.game?.rating_average`) consistent with how other `game.game.*` fields are accessed in this file.

**Header row** — add after the existing `Rating` column:
```tsx
<TableHead className="w-20">IGDB</TableHead>
```

**Data cell** — add after the existing personal rating cell. Since `formatIgdbRating` handles null/undefined, this mirrors the `formatTtb` callsite pattern:
```tsx
<TableCell>
  <span className="text-sm">{formatIgdbRating(game.game?.rating_average)}</span>
</TableCell>
```

**Skeleton** — `GameListSkeleton` currently has 8 `<TableCell>` entries. Insert a 9th `<TableCell>` after the last existing one (the Rating skeleton cell):
```tsx
<TableCell>
  <Skeleton className="h-4 w-12" />
</TableCell>
```

---

### 5. Frontend — Sort Option

**Files:** `frontend/src/components/games/game-filters.tsx` and `frontend/src/routes/_authenticated/games/index.tsx`

These two files each define their own local `SortField` type (not shared). TypeScript will not catch a mismatch between them due to structural compatibility — **both must be updated together** or the sort will silently break at runtime.

**`game-filters.tsx`** — `SortOption` here has only `value` and `label` (no `defaultOrder` field in this file's interface):
```ts
// SortField type — add 'rating_average'
type SortField = 'title' | 'created_at' | 'howlongtobeat_main' | 'personal_rating' | 'release_date' | 'hours_played' | 'rating_average';

// sortOptions array — add entry (do NOT add defaultOrder; it is not in this file's SortOption interface)
{ value: 'rating_average', label: 'IGDB Rating' },
```

**`games/index.tsx`** — `SortOption` here has `value`, `label`, and `defaultOrder`:
```ts
// SortField type — add 'rating_average'
type SortField = 'title' | 'created_at' | 'howlongtobeat_main' | 'personal_rating' | 'release_date' | 'hours_played' | 'rating_average';

// SORT_OPTIONS array — add entry with defaultOrder
{ value: 'rating_average', label: 'IGDB Rating', defaultOrder: 'desc' },
```

Default sort order is `desc` (highest rating first), consistent with `personal_rating`.

---

## Testing

### Backend
- Integration test: `sort_by=rating_average&sort_order=desc` returns games sorted by rating descending, nulls last
- Integration test: `sort_by=rating_average&sort_order=asc` returns games sorted ascending, nulls last

### Frontend
- `game-list.test.tsx`: IGDB column renders formatted rating (e.g. `"8.5"`) when `rating_average` is present; renders `—` when null
- `$id.index.test.tsx`: IGDB rating element appears in Quick Stats when `game.game.rating_average` is present; is absent when null
- `game-utils.test.ts` (or `game-utils.test.tsx`): unit tests for `formatIgdbRating` — cover `85.42 → "8.5"`, `72.10 → "7.2"`, `100 → "10.0"`, `null → "—"`, `undefined → "—"`

---

## Out of Scope

- No schema changes (field already exists)
- No API response changes (field already returned)
- No filtering by IGDB rating (not requested)
- No display on the game card (grid view) — PRD specifies detail view and list sort only

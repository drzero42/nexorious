# Games Page Sorting Feature Design

## Overview

Add sorting controls to the /games page with alphabetical (A→Z) as the default sort order. Users can choose from multiple sort fields and toggle sort direction. Preferences persist for the browser session.

## Sort Options

| Sort Field | API Value | Label | Default Direction |
|------------|-----------|-------|-------------------|
| Title | `title` | Title | A→Z (asc) |
| Date Added | `created_at` | Date Added | Newest first (desc) |
| Time to Beat | `time_to_beat` | Time to Beat | Shortest first (asc) |
| My Rating | `rating` | My Rating | Highest first (desc) |
| Release Date | `release_date` | Release Date | Newest first (desc) |

## UI Design

### Location

Added to the existing filter bar in `game-filters.tsx`, after the platform filter:

```
[Search...] [Status ▼] [Platform ▼] [Sort: Title ▼] [↑] [Grid|List] [Clear]
```

### Sort Dropdown

- Uses shadcn/ui `Select` component (consistent with existing filters)
- Options: Title, Date Added, Time to Beat, My Rating, Release Date
- Selecting an option applies the default direction for that field

### Direction Toggle

- Icon button next to the dropdown
- Shows direction-appropriate icon (e.g., ArrowUpAZ / ArrowDownAZ for title)
- Clicking toggles between asc/desc
- Tooltip shows current direction

### Behavior

- Changing sort field resets to that field's default direction
- Changing direction preserves current sort field
- Both changes trigger a new API request via existing `useUserGames` hook

## Session Persistence

- Storage: `sessionStorage` (clears when browser tab closes)
- Key: `games-sort-preference`
- Value: `{ sortBy: string, sortOrder: "asc" | "desc" }`
- On page load: read from sessionStorage, fallback to `title` / `asc`
- On sort change: write to sessionStorage immediately

## Implementation

### Files to Modify

| File | Changes |
|------|---------|
| `backend/app/api/user_games.py` | Change defaults: `sort_by="title"`, `sort_order="asc"` |
| `frontend/src/components/games/game-filters.tsx` | Add sort dropdown + direction toggle |
| `frontend/src/app/(main)/games/page.tsx` | Add `sortBy`/`sortOrder` state, sessionStorage logic, pass to filters & hook |

### Files Unchanged

- `frontend/src/hooks/use-games.ts` - Already supports `sort_by`/`sort_order` params
- `frontend/src/api/games.ts` - Already supports `sort_by`/`sort_order` params

### Dependencies

None - uses existing shadcn/ui components and lucide icons.

## Testing

- Backend: Update existing tests if they assert on default sort order
- Frontend: Add tests for sort dropdown behavior and sessionStorage persistence

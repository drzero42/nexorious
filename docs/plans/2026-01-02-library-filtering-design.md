# Library Filtering Design: Storefront, Genre, and Tags

## Overview

Add three new multi-select filters to the Library page: Storefront, Genre, and Tags. These filters integrate with existing filters (search, status, platform) and use URL params for state management.

## Filter Behavior

### Multi-Select Logic

- **Within each filter**: OR logic
  - Selecting "Steam" and "Epic" shows games from Steam OR Epic
- **Between filter types**: AND logic
  - Selecting storefronts + genres + tags narrows results progressively

### Filter Types

| Filter | Data Source | Matching Logic |
|--------|-------------|----------------|
| Storefront | UserGamePlatform.storefront | Game has platform entry with selected storefront |
| Genre | Game.genre (parsed) | Game's genre string contains selected genre |
| Tags | UserGameTag | Game has any of the selected tags |

### Display Format

- Unselected: "Storefronts", "Genres", "Tags"
- Selected: "Storefronts (3)", "Genres (2)", "Tags (1)"
- Clicking opens multi-select dropdown with checkboxes
- Options sorted alphabetically

### Clear Behavior

Single "Clear Filters" button resets all filters including the three new ones.

## UI Layout

### Filter Bar (Single Row)

```
[Search...] [Status ▾] [Platform ▾] [Storefront ▾] [Genre ▾] [Tags ▾] [Sort By ▾] [↑↓] [Clear] [Grid/List]
```

### Multi-Select Dropdown Component

- Trigger button shows filter name + count badge when selections exist
- Dropdown opens with checkboxes for each option
- Clicking outside or pressing Escape closes dropdown
- Selections apply immediately (no "Apply" button needed)

### Data Loading

- Storefronts: Fetched from `/api/storefronts/`
- Genres: New endpoint `/api/user-games/genres`
- Tags: Fetched from `/api/tags/`

### Empty States

- No games with storefronts: Storefront filter disabled/hidden
- No tags created: Tags filter shows "No tags created"
- No genres in collection: Genre filter disabled/hidden

## URL State Management

### URL Structure

```
/games?status=completed&platform=windows&platform=playstation_5&storefront=steam&storefront=epic&genre=RPG&tags=uuid1&tags=uuid2&sort=title&order=asc
```

### Behavior

- Filter changes update URL via `router.replace()` (no history spam)
- Page load reads URL params to initialize filter state
- Empty filters omit the param entirely (clean URLs)
- "Clear Filters" removes all filter params from URL
- Migrate existing sort persistence from sessionStorage to URL params

### Implementation

- Use Next.js `useSearchParams()` and `useRouter()` hooks
- URL params are single source of truth for filter state
- Filter components read from URL, write changes back to URL

## Backend Changes

### New Endpoint: GET /api/user-games/genres

Returns unique parsed genres from user's collection.

```json
{
  "genres": ["Action", "Adventure", "RPG", "Shooter"]
}
```

Implementation:
- Query all `Game.genre` strings from user's collection
- Split by comma, trim whitespace, deduplicate
- Return sorted list of unique genres

### Updated Query Parameters

Multi-value params for `/api/user-games/` and `/api/user-games/ids`:

| Param | Format | Example |
|-------|--------|---------|
| platform | Repeated | `?platform=windows&platform=playstation_5` |
| storefront | Repeated | `?storefront=steam&storefront=epic` |
| genre | Repeated | `?genre=RPG&genre=Action` |
| tag | Repeated | `?tag=uuid1&tag=uuid2` |

### Filter SQL Logic

- Multiple platforms: `WHERE platform IN (...)`
- Multiple storefronts: `WHERE storefront IN (...)`
- Multiple genres: `WHERE (genre ILIKE '%RPG%' OR genre ILIKE '%Action%')`
- Multiple tags: `WHERE EXISTS (SELECT 1 FROM user_game_tags WHERE tag_id IN (...))`

## Frontend Changes

### New Component: MultiSelectFilter

Reusable dropdown with checkboxes.

```typescript
interface MultiSelectFilterProps {
  label: string;                              // "Storefronts", "Genres", "Tags"
  options: { value: string; label: string }[];
  selected: string[];
  onChange: (selected: string[]) => void;
  disabled?: boolean;
}
```

### New Hook: useUserGameGenres

Fetches unique genres from user's collection.

```typescript
function useUserGameGenres(): {
  genres: string[];
  isLoading: boolean;
  error: Error | null;
}
```

### GameFilters Component Updates

- Remove sessionStorage for sort preferences
- Use URL params as source of truth for all filters
- Add MultiSelectFilter instances for Storefront, Genre, Tags
- Update "Clear Filters" to reset all filter params

### API Service Updates

- `getUserGames()` accepts arrays for platform, storefront, genre, tag
- Serialize arrays as repeated query params

## Implementation Checklist

### Backend

- [ ] Add `GET /api/user-games/genres` endpoint
- [ ] Update `/api/user-games/` to accept multi-value platform param
- [ ] Update `/api/user-games/` to accept multi-value storefront param
- [ ] Add genre filter param (multi-value) to `/api/user-games/`
- [ ] Add tag filter param (multi-value) to `/api/user-games/`
- [ ] Update `/api/user-games/ids` with same multi-value params
- [ ] Add tests for new endpoint and filter combinations

### Frontend

- [ ] Create MultiSelectFilter component
- [ ] Create useUserGameGenres hook
- [ ] Refactor GameFilters to use URL params as state source
- [ ] Add Storefront filter using MultiSelectFilter
- [ ] Add Genre filter using MultiSelectFilter
- [ ] Add Tags filter using MultiSelectFilter
- [ ] Update Clear Filters to reset all params
- [ ] Migrate sort preferences from sessionStorage to URL
- [ ] Update API service to handle array params
- [ ] Add tests for new components and hooks

# User-Games Utility Endpoints — Design Spec

## Overview

Four read-only GET endpoints under `/api/user-games/` that the React frontend needs for collection management: bulk selection (ids), filter sidebar population (genres, filter-options), and dashboard statistics (stats). All are JWT-protected and user-scoped.

These are direct ports of the Python implementations. Response shapes match the existing frontend consumers exactly — no frontend changes required.

## Handler Structure

All 4 handlers live on the existing `UserGamesHandler` in `internal/api/user_games.go`. No new files or handler types needed.

## Endpoints

### `GET /api/user-games/ids`

**Purpose:** Lightweight endpoint for "select all matching current filters" — returns just the UUIDs, no pagination, no relations.

**Query params:** Same filter set as `GET /api/user-games`:
- `play_status` — single enum value
- `ownership_status` — single enum value
- `is_loved` — bool
- `platform` — repeated param, multi-value
- `storefront` — repeated param, multi-value
- `genre` — repeated param, multi-value (ILIKE match against comma-separated `games.genre`)
- `game_mode` — repeated param, multi-value (ILIKE)
- `theme` — repeated param, multi-value (ILIKE)
- `player_perspective` — repeated param, multi-value (ILIKE)
- `tag` — repeated param, multi-value (tag UUIDs, subquery match)
- `rating_min` — float
- `rating_max` — float
- `has_notes` — bool
- `q` — search string (ILIKE on game title + personal notes)

No sort or pagination params.

**Response:**
```json
{
  "ids": ["550e8400-e29b-41d4-a716-446655440000", "..."]
}
```

**Implementation:** Reuses the existing `filter.FilterBuilder` to build the WHERE/JOIN clauses — same filter application as `HandleListUserGames` — but selects only `DISTINCT ug.id` with no ORDER BY, OFFSET, or LIMIT. Returns all matching IDs as a flat list.

### `GET /api/user-games/genres`

**Purpose:** Populates the genre filter dropdown with only genres present in the user's collection.

**No query params.**

**Response:**
```json
{
  "genres": ["Action", "Adventure", "RPG", "Simulation"]
}
```

Sorted alphabetically. Empty array when the user has no games or no games have genre data.

**Implementation:**
1. `SELECT DISTINCT g.genre FROM games g JOIN user_games ug ON g.id = ug.game_id WHERE ug.user_id = ? AND g.genre IS NOT NULL`
2. In Go: split each result on `,`, trim whitespace, collect into a `map[string]bool` for deduplication
3. Sort the keys alphabetically, return

### `GET /api/user-games/filter-options`

**Purpose:** Populates all filter dropdowns in the sidebar — covers the 4 comma-separated metadata fields on the `games` table.

**No query params.**

**Response:**
```json
{
  "genres": ["Action", "RPG"],
  "game_modes": ["Single player", "Multiplayer"],
  "themes": ["Horror", "Sci-fi"],
  "player_perspectives": ["First person", "Third person"]
}
```

All arrays sorted alphabetically. Empty arrays when no data exists.

**Implementation:**
1. Single query: `SELECT g.genre, g.game_modes, g.themes, g.player_perspectives FROM games g JOIN user_games ug ON g.id = ug.game_id WHERE ug.user_id = ?`
2. Iterate rows, split each comma-separated field, deduplicate into 4 separate sets
3. Sort each set alphabetically, return

Note: `genres` from filter-options and the `/genres` endpoint return the same data. The `/genres` endpoint exists for backward compatibility with the frontend which calls it separately from filter-options.

### `GET /api/user-games/stats`

**Purpose:** Dashboard statistics for the user's game collection.

**No query params.**

**Response:**
```json
{
  "total_games": 150,
  "completion_stats": {
    "not_started": 50,
    "in_progress": 30,
    "completed": 25,
    "mastered": 10,
    "dominated": 5,
    "shelved": 15,
    "dropped": 10,
    "replay": 5
  },
  "ownership_stats": {
    "owned": 120,
    "borrowed": 5,
    "rented": 3,
    "subscription": 15,
    "no_longer_owned": 7
  },
  "platform_stats": {
    "PC": 80,
    "PlayStation 5": 40,
    "Nintendo Switch": 30
  },
  "genre_stats": {
    "Action": 45,
    "RPG": 30,
    "Adventure": 25
  },
  "pile_of_shame": 50,
  "completion_rate": 33.33,
  "average_rating": 3.8,
  "total_hours_played": 1250.5
}
```

**Implementation — multiple targeted queries:**

1. **total_games:** `SELECT COUNT(*) FROM user_games WHERE user_id = ?`

2. **completion_stats:** `SELECT play_status, COUNT(*) FROM user_games WHERE user_id = ? GROUP BY play_status` — returns `map[string]int`. All 8 play statuses appear in the response; missing statuses default to 0.

3. **ownership_stats:** `SELECT ugp.ownership_status, COUNT(DISTINCT ugp.user_game_id) FROM user_game_platforms ugp JOIN user_games ug ON ug.id = ugp.user_game_id WHERE ug.user_id = ? GROUP BY ugp.ownership_status` — platform-level counting (a game owned on Steam and subscribed on Game Pass counts in both buckets). Missing statuses default to 0.

4. **platform_stats:** `SELECT p.display_name, COUNT(*) FROM user_game_platforms ugp JOIN platforms p ON p.name = ugp.platform JOIN user_games ug ON ug.id = ugp.user_game_id WHERE ug.user_id = ? GROUP BY p.name, p.display_name` — keyed by display_name (e.g. "PC", "PlayStation 5"). Only platforms with at least 1 game appear.

5. **genre_stats:** `SELECT g.genre FROM games g JOIN user_games ug ON g.id = ug.game_id WHERE ug.user_id = ? AND g.genre IS NOT NULL` — then in Go: split each row's `genre` on `,`, trim whitespace, and increment a `map[string]int` counter per individual genre. This matches how `genres` and `filter-options` split comma-separated fields. A game with genre `"Action, RPG"` contributes +1 to both `"Action"` and `"RPG"`.

6. **pile_of_shame:** Derived from completion_stats — `completion_stats["not_started"]`.

7. **completion_rate:** `(completed + mastered + dominated) / total_games * 100`, rounded to 2 decimal places. 0 when total_games is 0.

8. **average_rating:** `SELECT AVG(personal_rating) FROM user_games WHERE user_id = ? AND personal_rating IS NOT NULL` — null when no ratings exist.

9. **total_hours_played:** Iterates all user_games with their platforms. For each user_game: sum platform-level `hours_played`; if that sum > 0, use it; otherwise fall back to `user_games.hours_played` (legacy fallback, matching Python). This requires loading user_games with their platform relations.

## Route Registration

Routes must be registered **before** `/:id` in `router.go` to prevent Echo from interpreting "ids", "genres", etc. as an `:id` parameter value:

```go
userGamesGroup.GET("/ids", ugh.HandleListUserGameIDs)
userGamesGroup.GET("/genres", ugh.HandleListGenres)
userGamesGroup.GET("/filter-options", ugh.HandleFilterOptions)
userGamesGroup.GET("/stats", ugh.HandleCollectionStats)
// existing /:id routes below
userGamesGroup.GET("/:id", ugh.HandleGetUserGame)
```

## Error Handling

All 4 endpoints follow existing patterns:
- 401 if no valid JWT / user ID not found in context
- 500 for any database error (logged server-side, generic message to client)
- No 400/404 cases — these are collection-level queries that always succeed (returning empty/zero values for empty collections)

## Testing

Tests in `internal/api/user_games_test.go`, following existing patterns (testcontainers PostgreSQL, `setupUserGamesUser` helper, `getAuth` helper).

### `TestListUserGameIDs`
- **basic:** Create 3 user games, verify all 3 IDs returned
- **with filter:** Create games with different play statuses, filter by one, verify only matching IDs returned
- **user scoped:** Second user sees empty ids list
- **empty collection:** New user gets `{"ids": []}`

### `TestListGenres`
- **basic:** Create games with genre data, verify sorted unique genres returned
- **comma separation:** Game with "Action, RPG" produces both "Action" and "RPG" as separate entries
- **empty:** No games → `{"genres": []}`
- **null genres:** Games with null genre field are excluded

### `TestFilterOptions`
- **basic:** Verify all 4 arrays populated from game metadata
- **empty:** No games → all 4 arrays empty
- **deduplication:** Same genre across multiple games appears once

### `TestCollectionStats`
- **basic:** Verify all fields populated with correct values
- **empty collection:** total_games=0, all counts 0, completion_rate=0, average_rating=null, total_hours_played=0
- **hours fallback:** Game with platform hours uses platform hours; game without uses user_game.hours_played

## File Map

| File | Changes |
|------|---------|
| `internal/api/user_games.go` | 4 new handler methods + response types |
| `internal/api/user_games_test.go` | 4 new test functions |
| `internal/api/router.go` | 4 new route registrations (before `/:id`) |

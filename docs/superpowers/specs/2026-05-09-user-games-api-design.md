# User Games API — Design Spec

## Overview

User Games is the core collection management domain — it lets users add games to their library, track play status, associate platforms, and manage their collection in bulk. This spec covers the full `/api/user-games` endpoint surface except for the read-only aggregation endpoints (`stats`, `filter-options`, `genres`, `ids`), which will follow as a separate task.

All endpoints require JWT authentication. Every query is scoped to the authenticated user via `WHERE ug.user_id = ?` extracted from the JWT claim.

## Handler Structure

`UserGamesHandler` struct in `internal/api/user_games.go`:

```go
type UserGamesHandler struct {
    db  *bun.DB
    cfg *config.Config
}
```

Follows the same pattern as `GamesHandler`, `TagsHandler`, `PlatformsHandler`. Constructor: `NewUserGamesHandler(db *bun.DB, cfg *config.Config) *UserGamesHandler`.

Registered in `router.go` under a JWT-protected group:

```go
ugh := NewUserGamesHandler(db, cfg)
userGamesGroup := e.Group("/api/user-games", auth.JWTMiddleware(cfg.SecretKey, db))
```

## Model Changes

Add Bun relation tags to existing models in `internal/db/models/models.go`:

**On `UserGame`:**
```go
Game      *Game              `bun:"rel:belongs-to,join:game_id=id"       json:"game,omitempty"`
Platforms []UserGamePlatform `bun:"rel:has-many,join:id=user_game_id"    json:"platforms,omitempty"`
Tags      []UserGameTag      `bun:"rel:has-many,join:id=user_game_id"    json:"tags,omitempty"`
```

**On `UserGameTag`:**
```go
Tag *Tag `bun:"rel:belongs-to,join:tag_id=id" json:"tag,omitempty"`
```

These enable Bun's `Relation()` eager loading for get/list responses.

## Endpoints

### Single-Item CRUD

#### `GET /api/user-games` — List

Query parameters (all optional):

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number (default 1) |
| `per_page` | int | Items per page (default 20, max 100) |
| `sort_by` | string | Sort field (default `created_at`) |
| `sort_order` | string | `asc` or `desc` (default `desc`) |
| `play_status` | string | Filter by play status |
| `ownership_status` | string | Filter by ownership status |
| `is_loved` | bool | Filter by loved flag |
| `rating_min` | float | Minimum personal rating |
| `rating_max` | float | Maximum personal rating |
| `has_notes` | bool | Filter by presence of notes |
| `platform` | []string | Filter by platform(s); `"unknown"` = NULL |
| `storefront` | []string | Filter by storefront(s); `"unknown"` = NULL |
| `genre` | []string | Filter by genre (ILIKE) |
| `game_mode` | []string | Filter by game mode (ILIKE) |
| `theme` | []string | Filter by theme (ILIKE) |
| `player_perspective` | []string | Filter by perspective (ILIKE) |
| `tag` | []string | Filter by tag IDs (UUID strings) |
| `q` | string | Search title + personal notes (ILIKE) |

Multi-value parameters use repeated query params: `?platform=pc&platform=playstation`.

**Implementation:**

1. Base query: `SELECT DISTINCT ON (ug.id) ug.* FROM user_games AS ug WHERE ug.user_id = ?`
2. Parse query params, call corresponding `filter.Apply*` functions on a `filter.NewFilterBuilder()`
3. `fb.Apply(query)` adds accumulated JOINs/WHEREs
4. Count total (before pagination)
5. Apply sort + offset/limit
6. Eager-load relations via `Relation("Game")`, `Relation("Platforms")`, `Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery { return q.Relation("Tag") })`

**Allowed sort fields** (validated against allowlist):

| Field | Join required | SQL expression |
|-------|--------------|----------------|
| `title` | `games` | `g.title` |
| `created_at` | none | `ug.created_at` |
| `updated_at` | none | `ug.updated_at` |
| `play_status` | none | `ug.play_status` |
| `personal_rating` | none | `ug.personal_rating` |
| `is_loved` | none | `ug.is_loved` |
| `hours_played` | none | `ug.hours_played` |
| `release_date` | `games` | `g.release_date` |

Sort fields requiring a games join (`title`, `release_date`) add the join via the filter builder if not already present.

**Response** (200):
```json
{
  "user_games": [
    {
      "id": "uuid",
      "user_id": "uuid",
      "game_id": 123,
      "play_status": "playing",
      "personal_rating": 8,
      "is_loved": true,
      "hours_played": 42.5,
      "personal_notes": "Great game",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z",
      "game": { "id": 123, "title": "The Witcher 3", "cover_art_url": "...", ... },
      "platforms": [{ "id": "uuid", "platform": "pc", "storefront": "steam", ... }],
      "tags": [{ "id": "uuid", "tag_id": "uuid", "tag": { "name": "RPG", "color": "#ff0000" } }]
    }
  ],
  "total": 150,
  "page": 1,
  "per_page": 20,
  "pages": 8
}
```

#### `GET /api/user-games/:id` — Get Single

Fetches one user game by ID with ownership check (`user_id` must match JWT claim). Eager-loads `Game`, `Platforms`, and `Tags` (with nested `Tag`).

**Responses:**
- 200: Full user game object (same shape as list items)
- 404: User game not found or not owned by current user

#### `POST /api/user-games` — Create

**Request body:**
```json
{
  "game_id": 123,
  "play_status": "backlog",
  "personal_rating": null,
  "is_loved": false,
  "hours_played": null,
  "personal_notes": null
}
```

- Generates UUID for `id`, sets `user_id` from JWT, sets `created_at`/`updated_at` to `now`
- Validates `game_id` exists in `games` table (→ 400 if not)
- Returns 409 if `UNIQUE(user_id, game_id)` constraint violation (game already in collection)
- Returns 201 with the created user game (no relations loaded — it's a fresh entry)

#### `PUT /api/user-games/:id` — Update

**Request body** (all fields optional — only provided fields are updated):
```json
{
  "play_status": "completed",
  "personal_rating": 9,
  "is_loved": true,
  "hours_played": 85.0,
  "personal_notes": "Masterpiece"
}
```

- Ownership check: must belong to current user (→ 404 if not)
- Sets `updated_at` to `now`
- Returns 200 with updated user game + eager-loaded relations

`game_id` and `user_id` are immutable — not accepted in update body.

#### `PUT /api/user-games/:id/progress` — Update Progress

Lightweight partial update for quick "log progress" interactions.

**Request body:**
```json
{
  "hours_played": 12.5,
  "play_status": "playing"
}
```

Both fields are optional but at least one must be provided (→ 400 if empty). Sets `updated_at` to `now`. Returns 200 with the updated user game (no relations loaded — keep it fast).

#### `DELETE /api/user-games/:id` — Delete

Within a single transaction:

1. Fetch the user game to get `game_id` (also verifies ownership → 404 if not found/not owned)
2. Delete the `user_games` row — `user_game_platforms` and `user_game_tags` cascade via FK
3. Unreferenced game cleanup: `SELECT COUNT(*) FROM user_games WHERE game_id = ?` — if 0:
   - Delete the `games` row
   - Remove cover art file from `cfg.StoragePath + "/cover_art/"` (best-effort — log warning on error, don't fail the request)

Returns 204 No Content on success.

### Bulk Operations

All bulk operations run within a single transaction. All verify ownership (only operate on user games belonging to the current user — silently skip any IDs not owned).

#### `PUT /api/user-games/bulk-update`

**Request body:**
```json
{
  "ids": ["uuid1", "uuid2"],
  "updates": {
    "play_status": "completed",
    "is_loved": true,
    "personal_rating": 8
  }
}

```

- `ids` required, non-empty (→ 400)
- `updates` must contain at least one field (→ 400)
- Allowed update fields: `play_status`, `is_loved`, `personal_rating`
- Sets `updated_at` to `now` on all affected rows
- Returns 200 with `{"updated": <count>}`

#### `DELETE /api/user-games/bulk-delete`

**Request body:**
```json
{
  "ids": ["uuid1", "uuid2"]
}
```

- Fetches all matching user games (scoped to current user) to collect `game_id`s
- Deletes all matching `user_games` rows (cascades platforms + tags)
- Runs unreferenced game cleanup for each distinct `game_id`
- Returns 200 with `{"deleted": <count>}`

#### `POST /api/user-games/bulk-add-platforms`

**Request body:**
```json
{
  "user_game_ids": ["uuid1", "uuid2"],
  "platform": "pc",
  "storefront": "steam"
}
```

- Verifies all user game IDs belong to current user
- Inserts a `user_game_platforms` row for each user game (generates UUID, sets timestamps)
- Skips duplicates (same user_game_id + platform + storefront combination) — uses `ON CONFLICT DO NOTHING`
- Returns 200 with `{"added": <count>}`

#### `DELETE /api/user-games/bulk-remove-platforms`

**Request body:**
```json
{
  "user_game_ids": ["uuid1", "uuid2"],
  "platform": "pc",
  "storefront": "steam"
}
```

- Deletes matching `user_game_platforms` rows (scoped to user-owned user games)
- Returns 200 with `{"removed": <count>}`

### Platform Sub-Resource

All scoped to a specific user game, with ownership verification.

#### `GET /api/user-games/:id/platforms`

Returns all `user_game_platforms` rows for the given user game.

**Response** (200):
```json
[
  {
    "id": "uuid",
    "user_game_id": "uuid",
    "platform": "pc",
    "storefront": "steam",
    "store_game_id": "12345",
    "store_url": "https://store.steampowered.com/app/12345",
    "is_available": true,
    "hours_played": 42.5,
    "ownership_status": "owned",
    "acquired_date": "2024-06-15T00:00:00Z",
    "created_at": "...",
    "updated_at": "..."
  }
]
```

#### `POST /api/user-games/:id/platforms`

**Request body:**
```json
{
  "platform": "pc",
  "storefront": "steam",
  "store_game_id": "12345",
  "store_url": "https://...",
  "is_available": true,
  "hours_played": null,
  "ownership_status": "owned",
  "acquired_date": null
}
```

- Generates UUID, sets timestamps
- Returns 201 with the created platform association

#### `PUT /api/user-games/:id/platforms/:platform_id`

Updates mutable fields on the platform association. Sets `updated_at` to `now`. Returns 200 with updated platform.

#### `DELETE /api/user-games/:id/platforms/:platform_id`

Deletes the platform association. Returns 204 No Content.

## Error Handling

Follows the existing codebase pattern — `map[string]string{"error": "message"}`:

| Condition | Status | Message |
|-----------|--------|---------|
| Invalid request body | 400 | Descriptive validation error |
| User game not found / not owned | 404 | `"user game not found"` |
| Game ID doesn't exist | 400 | `"game not found"` |
| Duplicate user+game | 409 | `"game already in collection"` |
| Platform association not found | 404 | `"platform not found"` |
| Database error | 500 | `"database error"` |

Distinguish "not found" from DB errors using `errors.Is(err, sql.ErrNoRows)` (Bun wraps pgx errors).

## Testing

Tests in `internal/api/user_games_test.go` using the existing `testcontainers-go` pattern from `games_test.go` and `tags_test.go`:

- Helper: `insertTestUserGame(t, db, userID, gameID)` creates a user game with defaults
- Helper: `insertTestUserGamePlatform(t, db, userGameID, platform, storefront)` creates a platform association
- Test cases for each endpoint covering: happy path, ownership enforcement, not-found, validation errors, constraint violations
- Bulk operation tests: multiple IDs, mixed ownership (should skip non-owned), empty arrays
- Delete test: verify unreferenced game cleanup occurs and cover art file is removed
- Filter integration: verify filter builder criteria work end-to-end through the list endpoint

## File Map

| File | Change |
|------|--------|
| `internal/api/user_games.go` | **New** — handler struct + all endpoint handlers |
| `internal/api/user_games_test.go` | **New** — test suite |
| `internal/api/router.go` | **Modify** — register user-games routes |
| `internal/db/models/models.go` | **Modify** — add Bun relation tags to `UserGame` and `UserGameTag` |

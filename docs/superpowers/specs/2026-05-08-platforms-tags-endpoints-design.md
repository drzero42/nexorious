# Platforms & Tags Endpoints Design Spec

## Overview

Read-only endpoints for platforms and storefronts, plus full CRUD for user-scoped tags. These are the first Phase 2 endpoints — they have no external dependencies (no IGDB, no workers) and are required by later Phase 2 work (user-games with platform associations, tag filtering).

Platforms and storefronts are static reference data seeded via migrations (see `2026-05-07-static-platforms-storefronts-design.md`). No admin CRUD, no stats endpoints. Tags are user-owned and support create/update/delete.

## Endpoints

### Platforms (8 GET endpoints, JWT required)

All platform/storefront endpoints return the full dataset — no pagination. The total set is ~30 platforms and ~20 storefronts; pagination adds complexity for no benefit.

| Endpoint | Description |
|---|---|
| `GET /api/platforms` | All platforms, each with nested storefronts |
| `GET /api/platforms/simple-list` | `[{name, display_name}]` for dropdowns |
| `GET /api/platforms/:platform` | Single platform with nested storefronts; 404 if not found |
| `GET /api/platforms/:platform/storefronts` | Storefronts associated with one platform; 404 if platform not found |
| `GET /api/platforms/:platform/default-storefront` | Default storefront for a platform; 404 if platform not found |
| `GET /api/platforms/storefronts/` | All storefronts |
| `GET /api/platforms/storefronts/simple-list` | `[{name, display_name}]` for dropdowns |
| `GET /api/platforms/storefronts/:storefront` | Single storefront; 404 if not found |

### Tags (4 endpoints, JWT required, user-scoped)

| Endpoint | Description |
|---|---|
| `GET /api/tags` | All tags for the current user, with `game_count` |
| `POST /api/tags` | Create a tag |
| `PUT /api/tags/:id` | Update a tag (partial) |
| `DELETE /api/tags/:id` | Delete a tag (cascades `user_game_tags`) |

Tag assign/remove/bulk operations (`POST /api/tags/assign/:user_game_id`, etc.) are deferred — they depend on the user-games API.

## Response Shapes

### Platform responses

**Full platform** (used by `GET /api/platforms`, `GET /api/platforms/:platform`):

```json
{
  "name": "pc",
  "display_name": "PC",
  "icon": "pc.svg",
  "igdb_platform_id": 6,
  "default_storefront": "steam",
  "storefronts": [
    {
      "name": "steam",
      "display_name": "Steam",
      "icon": "steam.svg",
      "base_url": "https://store.steampowered.com/app/"
    }
  ]
}
```

**Simple list item** (used by `simple-list` endpoints):

```json
{"name": "pc", "display_name": "PC"}
```

**Default storefront mapping** (used by `GET /api/platforms/:platform/default-storefront`):

```json
{
  "platform": "pc",
  "platform_display_name": "PC",
  "default_storefront": {
    "name": "steam",
    "display_name": "Steam",
    "icon": "steam.svg",
    "base_url": "https://store.steampowered.com/app/"
  }
}
```

When no default storefront is configured, `default_storefront` is `null`.

### Tag responses

**Tag** (used in all tag responses):

```json
{
  "id": "uuid",
  "user_id": "uuid",
  "name": "Favorites",
  "color": "#ff0000",
  "created_at": "2026-05-08T00:00:00Z",
  "updated_at": "2026-05-08T00:00:00Z",
  "game_count": 5
}
```

`game_count` is a `COUNT(*)` over `user_game_tags` for that tag. Included on `GET /api/tags` (list) but not on `POST`/`PUT` responses (return the tag without count to avoid an extra query after mutation).

**Tag create request** (`POST /api/tags`):

```json
{"name": "Favorites", "color": "#ff0000"}
```

- `name`: required, max 100 chars, must be unique per user (enforced by DB `UNIQUE(user_id, name)`)
- `color`: optional (nullable in DB)
- Server generates the UUID `id`, sets `user_id` from JWT, sets `created_at`/`updated_at`

**Tag update request** (`PUT /api/tags/:id`):

```json
{"name": "Top Games", "color": "#00ff00"}
```

- Both fields optional — only update fields present in the request body
- `name` uniqueness enforced by DB constraint; duplicate returns 409
- Must own the tag (tag's `user_id` must match JWT user); wrong owner returns 404 (not 403, to avoid leaking existence)
- Updates `updated_at` to `now()`

**Tag delete** (`DELETE /api/tags/:id`):

- Must own the tag; wrong owner returns 404
- Cascades to `user_game_tags` via DB `ON DELETE CASCADE`
- Returns 204 No Content

## Error Responses

All errors use Echo's `echo.NewHTTPError()` which returns `{"message": "error text"}`.

| Condition | Status | Error |
|---|---|---|
| Platform/storefront/tag not found | 404 | `"not found"` |
| Tag name already exists for user | 409 | `"tag name already exists"` |
| Invalid request body | 400 | `"invalid request body"` |
| Missing required field | 400 | `"name is required"` |
| Name too long | 400 | `"name must be 100 characters or less"` |
| Unauthorized (no/invalid JWT) | 401 | `"unauthorized"` |

## Code Structure

### Files

- `internal/api/platforms.go` — `PlatformsHandler` struct, constructor, 8 handler methods
- `internal/api/platforms_test.go` — integration tests with testcontainers
- `internal/api/tags.go` — `TagsHandler` struct, constructor, 4 handler methods
- `internal/api/tags_test.go` — integration tests with testcontainers

### Handler Pattern

Follow the established pattern from `auth.go`:

```go
type PlatformsHandler struct {
    db *bun.DB
}

func NewPlatformsHandler(db *bun.DB) *PlatformsHandler {
    return &PlatformsHandler{db: db}
}

func (h *PlatformsHandler) HandleListPlatforms(c *echo.Context) error { ... }
```

```go
type TagsHandler struct {
    db *bun.DB
}

func NewTagsHandler(db *bun.DB) *TagsHandler {
    return &TagsHandler{db: db}
}

func (h *TagsHandler) HandleListTags(c *echo.Context) error { ... }
```

### Route Registration

In `registerRoutes`, inside the `if db != nil` block, after the existing auth routes:

```go
ph := NewPlatformsHandler(db)
platformsGroup := e.Group("/api/platforms", auth.JWTMiddleware(cfg.SecretKey, db))
platformsGroup.GET("", ph.HandleListPlatforms)
platformsGroup.GET("/simple-list", ph.HandleSimpleList)
platformsGroup.GET("/storefronts/simple-list", ph.HandleStorefrontSimpleList)
platformsGroup.GET("/storefronts/:storefront", ph.HandleGetStorefront)
platformsGroup.GET("/storefronts/", ph.HandleListStorefronts)
platformsGroup.GET("/:platform/storefronts", ph.HandlePlatformStorefronts)
platformsGroup.GET("/:platform/default-storefront", ph.HandleDefaultStorefront)
platformsGroup.GET("/:platform", ph.HandleGetPlatform)

th := NewTagsHandler(db)
tagsGroup := e.Group("/api/tags", auth.JWTMiddleware(cfg.SecretKey, db))
tagsGroup.GET("", th.HandleListTags)
tagsGroup.POST("", th.HandleCreateTag)
tagsGroup.PUT("/:id", th.HandleUpdateTag)
tagsGroup.DELETE("/:id", th.HandleDeleteTag)
```

Route ordering note: `/storefronts/simple-list` and `/storefronts/:storefront` must be registered before `/:platform` to avoid the catch-all param matching `storefronts` as a platform name. Same for `/simple-list` before `/:platform`.

### Database Queries

**Platforms** — use Bun's relation loading to join storefronts via `platform_storefronts`:

- `GET /api/platforms`: `SELECT * FROM platforms ORDER BY display_name` + relation load storefronts via `platform_storefronts` join table
- `GET /api/platforms/:platform`: same but `WHERE name = ?`
- `GET /api/platforms/simple-list`: `SELECT name, display_name FROM platforms ORDER BY display_name`
- `GET /api/platforms/:platform/storefronts`: load platform, then query storefronts joined through `platform_storefronts`
- `GET /api/platforms/:platform/default-storefront`: load platform, if `default_storefront` is set, load that storefront
- `GET /api/platforms/storefronts/`: `SELECT * FROM storefronts ORDER BY display_name`
- `GET /api/platforms/storefronts/simple-list`: `SELECT name, display_name FROM storefronts ORDER BY display_name`
- `GET /api/platforms/storefronts/:storefront`: `SELECT * FROM storefronts WHERE name = ?`

**Tags** — straightforward single-table queries scoped to user:

- `GET /api/tags`: `SELECT t.*, COUNT(ugt.id) as game_count FROM tags t LEFT JOIN user_game_tags ugt ON ugt.tag_id = t.id WHERE t.user_id = ? GROUP BY t.id ORDER BY t.name`
- `POST /api/tags`: `INSERT INTO tags (id, user_id, name, color, created_at, updated_at) VALUES (...)`
- `PUT /api/tags/:id`: `UPDATE tags SET ... WHERE id = ? AND user_id = ?`
- `DELETE /api/tags/:id`: `DELETE FROM tags WHERE id = ? AND user_id = ?`

### Bun Relation for Platform → Storefronts

The `Platform` model needs a `Storefronts` relation field, and the `PlatformStorefront` join model needs Bun relation tags pointing to the actual model structs:

```go
type Platform struct {
    bun.BaseModel `bun:"table:platforms"`
    // ... existing fields ...
    Storefronts []Storefront `bun:"m2m:platform_storefronts,join:Platform=Storefront" json:"storefronts,omitempty"`
}

type PlatformStorefront struct {
    bun.BaseModel `bun:"table:platform_storefronts"`

    PlatformName   string     `bun:"platform,pk"`
    StorefrontName string     `bun:"storefront,pk"`
    Platform       *Platform  `bun:"rel:belongs-to,join:platform=name"`
    Storefront     *Storefront `bun:"rel:belongs-to,join:storefront=name"`
}
```

The existing `PlatformStorefront` fields are renamed from `Platform`/`Storefront` (strings) to `PlatformName`/`StorefrontName` to avoid collision with the relation fields. The `json` tags on `PlatformStorefront` are dropped — this model is never serialised directly.

## Testing

Integration tests using testcontainers-go (same pattern as `auth_test.go` and `router_test.go`):

**Platforms tests** (`platforms_test.go`):
- List platforms returns seeded data with storefronts
- Simple list returns only name + display_name
- Get single platform by name
- Get single platform — not found returns 404
- Platform storefronts returns associated storefronts
- Default storefront mapping — with and without default
- List all storefronts
- Storefront simple list
- Get single storefront
- Get single storefront — not found returns 404
- All endpoints return 401 without JWT

**Tags tests** (`tags_test.go`):
- List tags returns user's tags with game_count
- List tags returns empty array for user with no tags
- Create tag — success
- Create tag — duplicate name returns 409
- Create tag — missing name returns 400
- Update tag — success (partial update)
- Update tag — duplicate name returns 409
- Update tag — not found / wrong owner returns 404
- Delete tag — success, returns 204
- Delete tag — not found / wrong owner returns 404
- Delete tag — cascades user_game_tags
- All endpoints return 401 without JWT

## Out of Scope

- Tag assign/remove/bulk operations (depends on user-games API)
- Platform/storefront stats endpoints (cancelled per static platforms spec)
- Platform/storefront admin CRUD (cancelled per static platforms spec)
- Pagination for platforms/storefronts (unnecessary for static data set size)

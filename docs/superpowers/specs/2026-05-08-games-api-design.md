# Games API & IGDB Service — Design Spec

## Scope

This spec covers the **core games endpoints** and the **IGDB service** needed to power them. It does not cover metadata management endpoints (status, refresh, populate, bulk operations, cover-art bulk download, metadata refresh-job) — those will be a follow-up spec that builds on the IGDB service implemented here.

### Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/games` | List games with pagination, search, filters, sorting |
| `GET` | `/api/games/:id` | Get single game by ID |
| `POST` | `/api/games/search/igdb` | Search IGDB, post-rank with fuzzy matching |
| `GET` | `/api/games/igdb/:igdb_id` | Get single game from IGDB by IGDB ID |
| `POST` | `/api/games/igdb-import` | Import a game from IGDB (create or update) |

All endpoints are JWT-protected (no admin-only routes in this set).

---

## Package Layout

```
internal/
├── api/
│   ├── games.go              # Echo handlers (GamesHandler struct)
│   └── games_test.go         # Integration tests (testcontainers)
├── services/
│   ├── igdb/
│   │   ├── igdb.go           # Client struct: SearchGames, GetGameByID, FetchFullMetadata, DownloadCoverArt
│   │   ├── auth.go           # IGDBAuthManager (Twitch OAuth token lifecycle)
│   │   ├── models.go         # GameMetadata, request/response DTOs
│   │   ├── keywords.go       # Keyword expansion table + query expansion logic
│   │   └── igdb_test.go      # Unit tests (HTTP mocks)
│   └── matching/
│       ├── normalize.go      # NormalizeTitle
│       ├── fuzzy.go          # FuzzyConfidence (go-fuzzywuzzy wrapper)
│       └── matching_test.go
```

**Why `igdb/` and `matching/` are separate:** The matching package is shared by the IGDB search pipeline (this spec) and later by sync/import background jobs (Phase 3/4) which match external game titles to IGDB candidates. Separating them avoids circular dependencies.

---

## Handler: `GamesHandler`

### Constructor

```go
type GamesHandler struct {
    db   *bun.DB
    igdb *igdb.Client
}

func NewGamesHandler(db *bun.DB, igdbClient *igdb.Client) *GamesHandler
```

Follows the same DI pattern as `NewAuthHandler`, `NewPlatformsHandler`, etc.

### Route Registration (in `registerRoutes`)

```go
gh := NewGamesHandler(db, igdbClient)
gamesGroup := e.Group("/api/games", auth.JWTMiddleware(cfg.SecretKey, db))
gamesGroup.GET("", gh.HandleListGames)
gamesGroup.GET("/:id", gh.HandleGetGame)
gamesGroup.POST("/search/igdb", gh.HandleSearchIGDB)
gamesGroup.GET("/igdb/:igdb_id", gh.HandleGetIGDBGame)
gamesGroup.POST("/igdb-import", gh.HandleImportFromIGDB)
```

---

## Endpoint Specifications

### `GET /api/games` — List Games

**Query parameters:**

| Param | Type | Default | Constraints |
|-------|------|---------|-------------|
| `page` | int | 1 | ≥ 1 |
| `per_page` | int | 20 | 1–100 |
| `q` | string | — | Search in title and description via ILIKE |
| `genre` | string | — | ILIKE filter |
| `developer` | string | — | ILIKE filter |
| `publisher` | string | — | ILIKE filter |
| `release_year` | int | — | `EXTRACT(year FROM release_date) = ?` |
| `sort_by` | string | "title" | Whitelist: title, release_date, created_at, rating_average |
| `sort_order` | string | "asc" | "asc" or "desc" |

**Behaviour:**

- When `q` is provided: `WHERE title ILIKE '%q%' OR description ILIKE '%q%'`
- Filters are combined with AND
- Unknown `sort_by` values return 400
- Pagination is offset-based: `OFFSET (page-1)*per_page LIMIT per_page`
- Count query uses the same WHERE clause

**No fuzzy search on the list endpoint.** The Python implementation's `fuzzy_threshold` parameter loads all games into memory for in-process fuzzy matching. The Go port drops this — local list search uses `ILIKE` only (per the main design spec).

**Response: `GameListResponse`**

```json
{
  "games": [GameResponse...],
  "total": 42,
  "page": 1,
  "per_page": 20,
  "pages": 3
}
```

### `GET /api/games/:id` — Get Game

- Query: `SELECT * FROM games WHERE id = ?`
- 404 if not found
- Returns full `GameResponse`

### `POST /api/games/search/igdb` — Search IGDB

**Request body:**

```json
{
  "query": "The Witcher 3",
  "limit": 10
}
```

- `query`: required, non-empty string
- `limit`: optional, default 10, max 50

**Behaviour:**

Delegates to `igdb.Client.SearchGames(ctx, query, limit)` which implements the full search pipeline (see IGDB Service section below).

Time-to-beat fields (`howlongtobeat_main`, `howlongtobeat_extra`, `howlongtobeat_completionist`) are **null** in search results — they are only fetched during import.

**Response: `IGDBSearchResponse`**

```json
{
  "games": [IGDBGameCandidate...],
  "total": 8
}
```

**Error mapping:**

| Service error | HTTP status | Detail |
|---------------|-------------|--------|
| `ErrIGDBNotConfigured` | 503 | "IGDB credentials not configured" |
| `ErrTwitchAuth` | 503 | "IGDB authentication failed: ..." |
| Generic IGDB HTTP error | 502 | "IGDB API error: ..." |

### `GET /api/games/igdb/:igdb_id` — Get IGDB Game by ID

- Calls `igdb.Client.GetGameByID(ctx, igdbID)`
- Returns single `IGDBGameCandidate` wrapped in `IGDBSearchResponse` (same shape as search — `games` array with one element, `total: 1`)
- 404 if IGDB returns no result for that ID
- Same error mapping as search endpoint

### `POST /api/games/igdb-import` — Import from IGDB

**Request body:**

```json
{
  "igdb_id": 1942,
  "custom_overrides": {
    "title": "Optional custom title override"
  },
  "download_cover_art": true
}
```

- `igdb_id`: required int
- `custom_overrides`: optional object — keys are Game field names (title, description, genre, developer, publisher). Values override IGDB data.
- `download_cover_art`: optional bool, default true

**Behaviour:**

1. Call `igdb.Client.FetchFullMetadata(ctx, igdbID)` — fetches complete metadata including time-to-beat
2. Check if a game with this IGDB ID already exists: `SELECT * FROM games WHERE igdb_slug = ?` (using the slug returned by IGDB). The `igdb_slug` field is the canonical IGDB identifier stored in the DB — it is always set during import and is unique per game.
3. If exists → update all metadata fields (respecting custom_overrides). If not → insert new row.
4. If `download_cover_art` is true and metadata has a cover URL → call `igdb.Client.DownloadCoverArt(ctx, coverURL, cfg.StoragePath)` → sets `cover_art_url` to `/static/cover_art/<filename>.jpg`
5. Return `GameResponse`

**Response codes:**
- 201 Created (new game)
- 200 OK (updated existing game)
- 404 if IGDB returns no result for that ID
- 503/502 for IGDB errors (same mapping)

---

## IGDB Service (`internal/services/igdb/`)

### `Client` struct

```go
type Client struct {
    httpClient  *http.Client
    auth        *AuthManager
    limiter     *rate.Limiter
    cfg         *config.Config
}

func NewClient(cfg *config.Config) *Client
```

**Rate limiter:** `rate.NewLimiter(rate.Every(250*time.Millisecond), 8)` — 4 req/s sustained with burst of 8, matching IGDB API limits.

### `AuthManager`

```go
type AuthManager struct {
    mu          sync.Mutex
    accessToken string
    expiresAt   time.Time
    clientID    string
    clientSecret string
    httpClient  *http.Client
}
```

**Token lifecycle:**

1. `GetAccessToken(ctx)` checks cached token
2. If no token, or token expires within 5 minutes → acquire mutex → double-check → POST to `https://id.twitch.tv/oauth2/token` with `client_id`, `client_secret`, `grant_type=client_credentials`
3. Store `access_token` and compute `expiresAt` from `expires_in`
4. Subsequent calls return cached token

**`IGDB_ACCESS_TOKEN` env var:** If set, used as initial token value. If no known expiry, token is used until it returns 401, which triggers a refresh. This is a dev/testing convenience — production uses client credentials auto-refresh.

**Required config:** `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET`. If both are missing, `NewClient` returns a client that responds to all calls with `ErrIGDBNotConfigured`.

### Key Methods

#### `SearchGames(ctx context.Context, query string, limit int) ([]GameMetadata, error)`

Implements the full pipeline from the main design spec (§ Fuzzy Search, Context 2):

1. **Keyword detection** — scan query for known patterns:
   - `"goty"` → `"Game of the Year"`
   - `"The Telltale Series"` → removed
   - `"®"` → removed
   - `"(classic)"` → removed (case-insensitive)
   - `":"` → replaced with space
   - Year-in-parentheses e.g. `(2023)` → removed
   - Standalone `"1"` (excluding "Episode 1", "Chapter 1") → removed

2. **Query expansion** — generate variant queries from detected keywords. If multiple keywords, also generate a fully-transformed variant.

3. **Concurrent IGDB calls for original query** — two goroutines via `errgroup`:
   - Fuzzy/prefix search (IGDB standard search endpoint)
   - Exact-name search (`where name = "..."`)
   
   Both go through `limiter.Wait(ctx)` before executing.

4. **Sequential expanded-query searches** — for each variant, run additional IGDB search. Merge results, deduplicate by IGDB ID (original/exact results take priority).

5. **Post-ranking** — rank merged candidates using `matching.FuzzyConfidence(normalizedQuery, normalizedTitle)`, filter at threshold 0.6, sort descending by score, truncate to `limit`.

#### `GetGameByID(ctx context.Context, igdbID int) (*GameMetadata, error)`

- Single IGDB API call: `where id = {igdbID}`
- Does NOT fetch time-to-beat (lightweight, same as search)
- Returns `ErrGameNotFound` if empty result

#### `FetchFullMetadata(ctx context.Context, igdbID int) (*GameMetadata, error)`

- Fetches complete game data including time-to-beat fields
- Used only by the import endpoint (not search)
- Returns `ErrGameNotFound` if empty result

#### `DownloadCoverArt(ctx context.Context, igdbCoverURL string, storagePath string) (string, error)`

- Downloads the cover image from IGDB's image CDN
- Writes to `<storagePath>/cover_art/<igdb_slug>.jpg`
- Returns the relative URL path: `/static/cover_art/<igdb_slug>.jpg`
- If the file already exists, skips download and returns existing path

### Error Types

```go
var (
    ErrIGDBNotConfigured = errors.New("IGDB credentials not configured")
    ErrGameNotFound      = errors.New("game not found in IGDB")
    ErrTwitchAuth        = errors.New("Twitch authentication failed")
)
```

### `models.go` — DTOs

```go
// GameMetadata is the internal representation of an IGDB game result.
// Used by both search (partial) and import (full).
type GameMetadata struct {
    IgdbID                     int
    IgdbSlug                   string
    Title                      string
    Description                *string
    Genre                      *string
    Developer                  *string
    Publisher                  *string
    ReleaseDate                *string   // ISO date string
    CoverArtURL                *string
    RatingAverage              *float64
    RatingCount                *int32
    EstimatedPlaytimeHours     *int32
    HowlongtobeatMain         *float64  // null in search results
    HowlongtobeatExtra        *float64
    HowlongtobeatCompletionist *float64
    PlatformIDs                []int     // IGDB platform IDs
    PlatformNames              []string  // Human-readable platform names
    GameModes                  *string
    Themes                     *string
    PlayerPerspectives         *string
}
```

---

## Matching Package (`internal/services/matching/`)

### `NormalizeTitle(s string) string`

Applies transformations in order (matching `backend/app/utils/normalize_title.py`):

1. Expand GOTY → "Game of the Year" (case-insensitive)
2. Remove trademark symbols (™, ®)
3. Remove apostrophes (straight ' and curly ' ')
4. Remove colons (:)
5. Remove standalone dashes ( - ) but preserve in-word hyphens (e.g. Spider-Man)
6. Remove year in parentheses, e.g. (2023)
7. Collapse whitespace
8. Lowercase and trim

Result is used only for comparison — never stored or displayed.

### `FuzzyConfidence(query, title string) float64`

Returns a 0.0–1.0 score using the multi-metric weighted approach:

- Uses `go-fuzzywuzzy`: Ratio, PartialRatio, TokenSortRatio, TokenSetRatio
- Weighted max: exact×1.0, ratio×0.9, partial×0.8, token_sort×0.7, token_set×0.6
- Both inputs should be pre-normalized via `NormalizeTitle`

**Known scoring divergence:** `go-fuzzywuzzy` produces different scores than Python's `rapidfuzz` for identical inputs. The 0.6 search threshold and later 0.85 auto-match threshold may behave slightly differently. This is an accepted trade-off — thresholds can be retuned after deployment.

---

## Response Shapes

### `GameResponse`

Maps directly from the `Game` Bun model. All fields serialized as-is via the existing struct tags. No transformation needed — the model's `json` tags already produce the correct shape.

### `GameListResponse`

```go
type GameListResponse struct {
    Games   []models.Game `json:"games"`
    Total   int           `json:"total"`
    Page    int           `json:"page"`
    PerPage int           `json:"per_page"`
    Pages   int           `json:"pages"`
}
```

### `IGDBGameCandidate`

```go
type IGDBGameCandidate struct {
    IgdbID                     int      `json:"igdb_id"`
    IgdbSlug                   *string  `json:"igdb_slug"`
    Title                      string   `json:"title"`
    ReleaseDate                *string  `json:"release_date"`
    CoverArtUrl                *string  `json:"cover_art_url"`
    Description                *string  `json:"description"`
    Platforms                  []string `json:"platforms"`
    HowlongtobeatMain         *float64 `json:"howlongtobeat_main"`
    HowlongtobeatExtra        *float64 `json:"howlongtobeat_extra"`
    HowlongtobeatCompletionist *float64 `json:"howlongtobeat_completionist"`
}
```

### `IGDBSearchResponse`

```go
type IGDBSearchResponse struct {
    Games []IGDBGameCandidate `json:"games"`
    Total int                 `json:"total"`
}
```

### Request Bodies

```go
type IGDBSearchRequest struct {
    Query string `json:"query" validate:"required"`
    Limit int    `json:"limit" validate:"omitempty,min=1,max=50"`
}

type IGDBImportRequest struct {
    IgdbID           int                    `json:"igdb_id" validate:"required"`
    CustomOverrides  map[string]interface{} `json:"custom_overrides"`
    DownloadCoverArt *bool                  `json:"download_cover_art"` // default true if nil
}
```

---

## Testing

### IGDB Service Unit Tests (`igdb_test.go`)

- Mock HTTP server for Twitch token endpoint and IGDB API
- Test token refresh lifecycle (expired → auto-refresh)
- Test `IGDB_ACCESS_TOKEN` seed behaviour
- Test rate limiter blocks when exhausted
- Test keyword expansion (each pattern)
- Test search pipeline end-to-end with mocked IGDB responses
- Test deduplication logic

### Matching Package Tests (`matching_test.go`)

- `NormalizeTitle`: test each transformation rule independently + combined
- `FuzzyConfidence`: test known pairs against expected score ranges (not exact values due to library differences)

### Handler Integration Tests (`games_test.go`)

Using testcontainers-go (same pattern as `auth_test.go`):

- `TestGamesList` — pagination, empty list, total count
- `TestGamesListSearch` — ILIKE matching on title and description
- `TestGamesListFilters` — genre, developer, publisher, release_year
- `TestGamesListSort` — valid sort fields, invalid sort field → 400
- `TestGamesGet` — found, not found (404)
- `TestGamesSearchIGDB` — requires mocked IGDB (inject mock client)
- `TestGamesImport` — create new, update existing, cover art download

---

## Checklist

- [ ] `internal/services/matching/normalize.go` + tests
- [ ] `internal/services/matching/fuzzy.go` + tests
- [ ] `internal/services/igdb/auth.go` + tests
- [ ] `internal/services/igdb/models.go`
- [ ] `internal/services/igdb/keywords.go` + tests
- [ ] `internal/services/igdb/igdb.go` (Client, SearchGames, GetGameByID, FetchFullMetadata, DownloadCoverArt) + tests
- [ ] `internal/api/games.go` (GamesHandler + all 5 handlers)
- [ ] `internal/api/games_test.go` (integration tests)
- [ ] Wire `igdb.NewClient` in `cmd/nexorious/main.go` and pass to router
- [ ] Register routes in `registerRoutes`
- [ ] Add Slumber collection entries for all 5 endpoints
- [ ] `go test ./...` passes

# Filter Package Spec

**Date:** 2026-05-07
**Status:** Approved

## Overview

Implement `internal/filter/` — a reusable SQL query builder used by the `GET /api/user-games` list endpoint and related endpoints (`/ids`, `/filter-options`, `/genres`, `/stats`). The package accumulates JOINs, WHERE conditions, and HAVING conditions into a single `goqu` SELECT expression, following the Stash-style filterBuilder pattern described in the design spec.

This package has no API surface and no HTTP handlers. It is pure query-building logic.

---

## Why This Package Exists

`GET /api/user-games` accepts up to ~13 optional filter parameters. Each filter may or may not require a JOIN to `games` or `user_game_platforms`, and naively joining the same table twice produces duplicate rows. The filterBuilder tracks which JOINs have already been added and deduplicates them. sqlc cannot express these queries statically because the shape changes per-request.

---

## What This Package Does NOT Do

- No HTTP handlers, no Echo types
- No pagination — callers apply `LIMIT`/`OFFSET` after building the expression
- No sorting — callers apply `ORDER BY` after building the expression
- No execution — callers run the built query themselves via `sqlx`
- No fuzzy matching — the `q` (text search) criterion uses `ILIKE` only

---

## Filters to Implement

These are the filter parameters accepted by `GET /api/user-games` in the Python version that are carried forward into the Go port:

| Parameter | Type | Join required | Logic |
|---|---|---|---|
| `play_status` | `string` | none | `user_games.play_status = ?` |
| `ownership_status` | `string` | `user_game_platforms` | `user_game_platforms.ownership_status = ?` |
| `is_loved` | `bool` | none | `user_games.is_loved = ?` |
| `rating_min` | `float` | none | `user_games.personal_rating >= ?` |
| `rating_max` | `float` | none | `user_games.personal_rating <= ?` |
| `has_notes` | `bool` | none | `personal_notes IS NOT NULL AND personal_notes != ''` (true) or `IS NULL OR = ''` (false) |
| `platform` | `[]string` | `user_game_platforms` | Multi-value; `"unknown"` maps to NULL; see below |
| `storefront` | `[]string` | `user_game_platforms` | Multi-value; `"unknown"` maps to NULL; see below |
| `genre` | `[]string` | `games` | OR of `games.genre ILIKE ?` for each value |
| `game_mode` | `[]string` | `games` | OR of `games.game_modes ILIKE ?` for each value |
| `theme` | `[]string` | `games` | OR of `games.themes ILIKE ?` for each value |
| `player_perspective` | `[]string` | `games` | OR of `games.player_perspectives ILIKE ?` for each value |
| `tag` | `[]string` | none (subquery) | `user_games.id IN (SELECT user_game_id FROM user_game_tags WHERE tag_id IN (...))` |
| `q` | `string` | `games` | `games.title ILIKE ? OR (user_games.personal_notes IS NOT NULL AND user_games.personal_notes ILIKE ?)` |

**`fuzzy_threshold` is NOT implemented.** The Python parameter was never wired to the frontend; the Go port uses ILIKE only.

### Platform/Storefront NULL handling

The `"unknown"` sentinel value means "games with no platform/storefront set":

- `platform=["unknown"]` → `user_game_platforms.platform IS NULL`
- `platform=["steam"]` → `user_game_platforms.platform IN ('steam')`
- `platform=["steam","unknown"]` → `user_game_platforms.platform = 'steam' OR user_game_platforms.platform IS NULL`

Same logic applies to `storefront`.

### Duplicate row prevention

When `user_game_platforms` is joined and a user game has multiple platform entries, the JOIN produces multiple rows for the same `user_games.id`. The caller must apply `DISTINCT` on `user_games.id` — or wrap using a subquery — to deduplicate. The filterBuilder itself does not deduplicate; this is documented as a caller responsibility.

---

## Package Design

### File layout

```
internal/filter/
    builder.go      # filterBuilder struct + Join/Where/Having accumulation + Build()
    criteria.go     # One function per filter criterion; all take *filterBuilder and add to it
```

### `builder.go`

```go
package filter

import (
    "github.com/doug-martin/goqu/v9"
    _ "github.com/doug-martin/goqu/v9/dialect/postgres"
)

// join represents a single JOIN clause.
type join struct {
    table     string
    condition goqu.Expression
}

// filterBuilder accumulates JOINs, WHERE conditions, and HAVING conditions
// for a single query. JOINs are deduplicated by table name.
type filterBuilder struct {
    joins          []join
    whereClauses   []goqu.Expression
    havingClauses  []goqu.Expression
    joinSeen       map[string]bool
}

// NewFilterBuilder returns an empty filterBuilder.
func NewFilterBuilder() *filterBuilder

// AddJoin adds a LEFT JOIN if the table has not already been joined.
func (f *filterBuilder) AddJoin(table string, condition goqu.Expression)

// AddWhere appends a WHERE condition (ANDed together at build time).
func (f *filterBuilder) AddWhere(expr goqu.Expression)

// AddHaving appends a HAVING condition (ANDed together at build time).
func (f *filterBuilder) AddHaving(expr goqu.Expression)

// HasJoin reports whether the named table has been joined.
func (f *filterBuilder) HasJoin(table string) bool

// Apply applies accumulated JOINs, WHERE, and HAVING to the provided goqu SelectDataset
// and returns the modified dataset. The caller applies ORDER BY, LIMIT, OFFSET.
func (f *filterBuilder) Apply(ds *goqu.SelectDataset) *goqu.SelectDataset
```

### `criteria.go`

One function per filter. Each receives a `*filterBuilder` and the criterion value. If the value indicates "no filter" (nil pointer, empty slice, zero value), it must be a no-op.

```go
package filter

// ApplyPlayStatus adds a play_status = ? WHERE clause if status is non-empty.
func ApplyPlayStatus(f *filterBuilder, status string)

// ApplyOwnershipStatus adds an ownership_status = ? WHERE clause (with user_game_platforms JOIN) if status is non-empty.
func ApplyOwnershipStatus(f *filterBuilder, status string)

// ApplyIsLoved adds an is_loved = ? WHERE clause if ptr is non-nil.
func ApplyIsLoved(f *filterBuilder, isLoved *bool)

// ApplyRatingMin adds personal_rating >= min WHERE clause if ptr is non-nil.
func ApplyRatingMin(f *filterBuilder, min *float64)

// ApplyRatingMax adds personal_rating <= max WHERE clause if ptr is non-nil.
func ApplyRatingMax(f *filterBuilder, max *float64)

// ApplyHasNotes adds a notes presence/absence WHERE clause if ptr is non-nil.
func ApplyHasNotes(f *filterBuilder, hasNotes *bool)

// ApplyPlatform adds platform IN / IS NULL WHERE clause (with user_game_platforms JOIN) if platforms is non-empty.
// "unknown" in the slice maps to NULL.
func ApplyPlatform(f *filterBuilder, platforms []string)

// ApplyStorefront adds storefront IN / IS NULL WHERE clause (with user_game_platforms JOIN) if storefronts is non-empty.
// "unknown" in the slice maps to NULL.
func ApplyStorefront(f *filterBuilder, storefronts []string)

// ApplyGenre adds OR genre ILIKE ? WHERE clauses (with games JOIN) if genres is non-empty.
func ApplyGenre(f *filterBuilder, genres []string)

// ApplyGameMode adds OR game_modes ILIKE ? WHERE clauses (with games JOIN) if modes is non-empty.
func ApplyGameMode(f *filterBuilder, modes []string)

// ApplyTheme adds OR themes ILIKE ? WHERE clauses (with games JOIN) if themes is non-empty.
func ApplyTheme(f *filterBuilder, themes []string)

// ApplyPlayerPerspective adds OR player_perspectives ILIKE ? WHERE clauses (with games JOIN) if perspectives is non-empty.
func ApplyPlayerPerspective(f *filterBuilder, perspectives []string)

// ApplyTag adds a tag subquery WHERE clause if tagIDs is non-empty.
// user_games.id IN (SELECT user_game_id FROM user_game_tags WHERE tag_id IN (...))
func ApplyTag(f *filterBuilder, tagIDs []string)

// ApplyTextSearch adds title/notes ILIKE WHERE clauses (with games JOIN) if q is non-empty.
func ApplyTextSearch(f *filterBuilder, q string)
```

### Table and column name constants

Use `goqu.T()` / `goqu.C()` / `goqu.I()` for table/column references. The relevant table names are:
- `"user_games"` — aliased columns: `id`, `user_id`, `play_status`, `personal_rating`, `is_loved`, `hours_played`, `personal_notes`
- `"user_game_platforms"` — `user_game_id`, `platform`, `storefront`, `ownership_status`, `hours_played`
- `"games"` — `id`, `title`, `genre`, `game_modes`, `themes`, `player_perspectives`
- `"user_game_tags"` — `user_game_id`, `tag_id`

JOIN conditions:
- `user_game_platforms`: `user_game_platforms.user_game_id = user_games.id`
- `games`: `games.id = user_games.game_id`

---

## Testing

Tests live in `internal/filter/` alongside the implementation. Use table-driven tests.

**Do not use testcontainers for this package.** The filterBuilder produces SQL strings — test by inspecting the generated SQL, not by executing it. Call `ds.ToSQL()` on the result of `Apply()` and assert the SQL string contains the expected fragments.

### Key test cases

**`builder_test.go`:**
- Empty builder applied to a base dataset produces no extra JOINs or WHERE clauses
- `AddJoin` with the same table twice adds only one JOIN
- `AddWhere` with multiple expressions ANDs them
- `Apply` combines all accumulated clauses

**`criteria_test.go`:**
- Each criterion function: when value is zero/nil/empty → no clauses added, no JOINs added
- Each criterion function: when value is set → correct SQL fragment present in output
- Platform `"unknown"` → `IS NULL` in SQL
- Platform `["steam", "unknown"]` → `OR` between `= 'steam'` and `IS NULL`
- Storefront same as platform
- Multi-value genre → `OR` of ILIKE conditions
- Tag → subquery `IN (SELECT user_game_id FROM user_game_tags WHERE tag_id IN (...))`
- Two criteria requiring the same JOIN → JOIN appears only once in SQL
- Text search → title ILIKE and personal_notes ILIKE with OR

---

## Out of Scope

- Sorting — handled by the user-games handler, not the filter package
- Pagination — handled by the caller
- Execution / sqlx wiring — handled by the caller
- `fuzzy_threshold` — dropped (ILIKE only)
- HAVING clause usage — the builder supports it for future use but no current criterion uses it

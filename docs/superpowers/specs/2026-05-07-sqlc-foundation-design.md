# sqlc Foundation — Design Spec

## Overview

This spec covers the sqlc code-generation foundation for Phase 2 of the Go port. It produces three things:

1. `sqlc.yaml` — configuration pointing sqlc at the schema and queries
2. `internal/db/queries/*.sql` — hand-written named SQL queries for all Phase 2 tables
3. `internal/db/gen/` — generated Go code, committed to the repo

No API handler code is written in this step. The output is the query layer that Phase 2 handlers will build on.

---

## sqlc.yaml

Placed at the project root alongside the Makefile.

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/db/queries/"
    schema: "internal/db/migrations/"
    gen:
      go:
        package: "db"
        out: "internal/db/gen"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_pointers_for_null_fields: true
```

**Key decisions:**

- `sql_package: "pgx/v5"` — consistent with the rest of the project; no additional driver wrappers needed.
- `emit_pointers_for_null_fields: true` — nullable columns become `*string`, `*time.Time`, etc. rather than `sql.NullString`. Cleaner to work with in handler code.
- `emit_json_tags: true` — adds `json:"..."` struct tags. Handlers will usually project into dedicated response types rather than return generated structs directly, but the tags aid debugging.
- `schema` points at `internal/db/migrations/` — sqlc natively understands the golang-migrate `.up.sql` / `.down.sql` naming convention and automatically ignores down files when given a directory path. The existing `0001_initial.up.sql` contains the complete table definitions. As future migrations are added (always following the `NNNN_name.up.sql` pattern), sqlc picks them up automatically in lexicographic order — zero-padded numbering (already in use) ensures lexicographic order matches migration order.
- `queries` points at `internal/db/queries/` — one file per domain (see below).

**`pgtype.Numeric` note:** pgx/v5 maps PostgreSQL `NUMERIC` columns to `pgtype.Numeric` in generated structs by default. Columns affected: `rating_average`, `hours_played`, `howlongtobeat_main/extra/completionist`, `personal_rating` (INT, not affected). Handler code must convert `pgtype.Numeric` to `float64` (or `*float64`) when projecting into JSON response types. This is expected and is handled at the response-projection layer, not in the query layer.

**`Platform` and `Storefront` field shape note:** Platforms and storefronts are static reference data. Their generated structs reflect the slimmed-down schema from the static-platforms-storefronts design spec. Key points for handler code:
- The field is `Icon *string` (filename only, e.g. `"pc-windows.svg"`), **not** `IconUrl`. Handlers that return platform/storefront data must construct the full URL as `/logos/platforms/<slug>/<icon>` (or `/logos/storefronts/<slug>/<icon>`). Do **not** prefix with `config.StaticURL` — logos are frontend assets served by Vite/the embedded SPA, not by the Go static file route. `config.StaticURL` is for cover art only.
- `IgdbPlatformId *int32` — nullable; used by the sync service (Phase 3+) to map IGDB platform IDs to local platform slugs. Phase 2 handlers do not need to read or write this field.
- The following columns from the original Python schema **do not exist** in the generated structs: `IsActive`, `Source`, `VersionAdded`, `CreatedAt`, `UpdatedAt`. Do not reference them.

---

## Query files

### Scope

Phase 2 tables only: `games`, `user_games`, `user_game_platforms`, `platforms`, `storefronts`, `platform_storefronts`, `tags`, `user_game_tags`.

**Excluded from this spec:**
- `users`, `user_sessions` — auth uses raw `pool.QueryRow` per the master spec and must not depend on the generated package.
- `jobs`, `job_items`, `pending_tasks`, `external_games`, `user_sync_configs`, `backup_config` — Phase 3+.
- `rate_limiter_tokens` — internal implementation detail of the PostgreSQL rate limiter backend; no sqlc queries needed (the rate limiter manages this table directly via raw SQL).

---

### `internal/db/queries/games.sql`

```sql
-- name: GetGame :one
SELECT * FROM games WHERE id = $1;

-- name: GetGamesByIDs :many
SELECT * FROM games WHERE id = ANY($1::integer[])
ORDER BY title;

-- name: UpsertGame :one
INSERT INTO games (
    id, title, description, genre, developer, publisher,
    release_date, cover_art_url, rating_average, rating_count,
    estimated_playtime_hours, howlongtobeat_main, howlongtobeat_extra,
    howlongtobeat_completionist, igdb_slug, igdb_platform_ids,
    igdb_platform_names, game_modes, themes, player_perspectives,
    game_metadata, last_updated, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13,
    $14, $15, $16,
    $17, $18, $19, $20,
    $21, $22, $23  -- $23 = created_at; passed explicitly to preserve the IGDB creation timestamp across metadata refreshes
)
ON CONFLICT (id) DO UPDATE SET
    title                       = EXCLUDED.title,
    description                 = EXCLUDED.description,
    genre                       = EXCLUDED.genre,
    developer                   = EXCLUDED.developer,
    publisher                   = EXCLUDED.publisher,
    release_date                = EXCLUDED.release_date,
    cover_art_url               = EXCLUDED.cover_art_url,
    rating_average              = EXCLUDED.rating_average,
    rating_count                = EXCLUDED.rating_count,
    estimated_playtime_hours    = EXCLUDED.estimated_playtime_hours,
    howlongtobeat_main          = EXCLUDED.howlongtobeat_main,
    howlongtobeat_extra         = EXCLUDED.howlongtobeat_extra,
    howlongtobeat_completionist = EXCLUDED.howlongtobeat_completionist,
    igdb_slug                   = EXCLUDED.igdb_slug,
    igdb_platform_ids           = EXCLUDED.igdb_platform_ids,
    igdb_platform_names         = EXCLUDED.igdb_platform_names,
    game_modes                  = EXCLUDED.game_modes,
    themes                      = EXCLUDED.themes,
    player_perspectives         = EXCLUDED.player_perspectives,
    game_metadata               = EXCLUDED.game_metadata,
    last_updated                = EXCLUDED.last_updated
RETURNING *;

-- name: UpdateGameMetadata :one
UPDATE games SET
    description                 = $2,
    genre                       = $3,
    developer                   = $4,
    publisher                   = $5,
    release_date                = $6,
    rating_average              = $7,
    rating_count                = $8,
    estimated_playtime_hours    = $9,
    howlongtobeat_main          = $10,
    howlongtobeat_extra         = $11,
    howlongtobeat_completionist = $12,
    game_modes                  = $13,
    themes                      = $14,
    player_perspectives         = $15,
    game_metadata               = $16,
    last_updated                = now()
WHERE id = $1
RETURNING *;

-- name: DeleteGame :exec
DELETE FROM games WHERE id = $1;

-- name: SearchGamesByTitle :many
SELECT * FROM games
WHERE title ILIKE '%' || $1 || '%'
   OR (description IS NOT NULL AND description ILIKE '%' || $1 || '%')
ORDER BY title
LIMIT $2;
```

**No OFFSET:** `SearchGamesByTitle` powers the `q` search parameter on `GET /api/games` — the global game catalog endpoint (IGDB records cached in the local DB), which is the mechanism users use to add games to their collection. It does **not** back the user-games collection list (`GET /api/user-games`), which is handled entirely by the goqu dynamic `filterBuilder` and has no sqlc query. The Python `list_games` handler searches title OR description (OR logic, with a null guard on description) via ILIKE; the query above replicates that exactly. No pagination: the handler passes a fixed limit and the user refines their query if needed.

---

### `internal/db/queries/user_games.sql`

```sql
-- name: GetUserGame :one
SELECT * FROM user_games WHERE id = $1 AND user_id = $2;

-- name: GetUserGameByGameID :one
SELECT * FROM user_games WHERE user_id = $1 AND game_id = $2;

-- name: CreateUserGame :one
INSERT INTO user_games (
    id, user_id, game_id, play_status, personal_rating,
    is_loved, hours_played, personal_notes, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, now(), now()
)
RETURNING *;

-- name: UpdateUserGame :one
UPDATE user_games SET
    play_status     = $2,
    personal_rating = $3,
    is_loved        = $4,
    hours_played    = $5,
    personal_notes  = $6,
    updated_at      = now()
WHERE id = $1 AND user_id = $7
RETURNING *;

-- name: DeleteUserGame :exec
DELETE FROM user_games WHERE id = $1 AND user_id = $2;

-- name: CountUserGamesByUser :one
SELECT COUNT(*) FROM user_games WHERE user_id = $1;

-- name: CountUserGamesByGameID :one
-- Used by unreferenced-game cleanup: after deleting a user_game, the handler
-- checks this count; if zero, the games row and its cover art file are deleted.
SELECT COUNT(*) FROM user_games WHERE game_id = $1;
```

---

### `internal/db/queries/user_game_platforms.sql`

```sql
-- name: GetUserGamePlatform :one
SELECT * FROM user_game_platforms WHERE id = $1 AND user_game_id = $2;

-- name: ListUserGamePlatforms :many
SELECT * FROM user_game_platforms
WHERE user_game_id = $1
ORDER BY platform, storefront;

-- name: CreateUserGamePlatform :one
INSERT INTO user_game_platforms (
    id, user_game_id, platform, storefront,
    store_game_id, store_url, is_available, hours_played,
    ownership_status, acquired_date, original_platform_name,
    original_storefront_name, external_game_id, sync_from_source,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11,
    $12, $13, $14,
    now(), now()
)
RETURNING *;

-- name: UpdateUserGamePlatform :one
UPDATE user_game_platforms SET
    store_game_id    = $2,
    store_url        = $3,
    is_available     = $4,
    hours_played     = $5,
    ownership_status = $6,
    acquired_date    = $7,
    updated_at       = now()
WHERE id = $1
RETURNING *;

-- name: DeleteUserGamePlatform :exec
DELETE FROM user_game_platforms WHERE id = $1;
```

---

### `internal/db/queries/platforms.sql`

Read-only queries only. Platforms are static reference data — all write operations happen via migrations, not at runtime.

```sql
-- name: ListPlatforms :many
SELECT * FROM platforms ORDER BY display_name;

-- name: GetPlatform :one
SELECT * FROM platforms WHERE name = $1;
```

---

### `internal/db/queries/storefronts.sql`

Read-only queries only. Storefronts are static reference data — all write operations happen via migrations, not at runtime.

```sql
-- name: ListStorefronts :many
SELECT * FROM storefronts ORDER BY display_name;

-- name: GetStorefront :one
SELECT * FROM storefronts WHERE name = $1;
```

---

### `internal/db/queries/platform_storefronts.sql`

Read-only. The `platform_storefronts` join table is static reference data populated by the initial migration — there are no runtime mutation queries.

```sql
-- name: ListStorefrontsForPlatform :many
SELECT s.* FROM storefronts s
JOIN platform_storefronts ps ON ps.storefront = s.name
WHERE ps.platform = $1
ORDER BY s.display_name;
```

---

### `internal/db/queries/tags.sql`

```sql
-- name: ListUserTags :many
SELECT * FROM tags WHERE user_id = $1 ORDER BY name;

-- name: GetUserTag :one
SELECT * FROM tags WHERE id = $1 AND user_id = $2;

-- name: CreateTag :one
INSERT INTO tags (id, user_id, name, color, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
RETURNING *;

-- name: UpdateTag :one
UPDATE tags SET
    name       = $2,
    color      = $3,
    updated_at = now()
WHERE id = $1 AND user_id = $4
RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1 AND user_id = $2;
```

---

### `internal/db/queries/user_game_tags.sql`

```sql
-- name: ListUserGameTags :many
SELECT * FROM user_game_tags WHERE user_game_id = $1 ORDER BY created_at;

-- name: AddUserGameTag :one
INSERT INTO user_game_tags (id, user_game_id, tag_id, created_at)
VALUES ($1, $2, $3, now())
RETURNING *;

-- name: RemoveUserGameTag :exec
DELETE FROM user_game_tags WHERE user_game_id = $1 AND tag_id = $2;
```

---

## Generated output

Running `make sqlc` (which calls `sqlc generate`) produces `internal/db/gen/` containing:

- `db.go` — the `Queries` struct and `New(*pgxpool.Pool) *Queries` constructor
- `models.go` — one Go struct per table (e.g. `Game`, `UserGame`, `Platform`, `Tag`, etc.)
- One `.go` file per query file (e.g. `games.sql.go`, `tags.sql.go`, etc.) containing the typed query functions

The generated code is committed to the repo. Contributors do not need sqlc installed to build — only to regenerate after editing query files.

---

## Dependency injection pattern

`main.go` creates the pgxpool and passes it to `db.New(pool)` to get a `*db.Queries`. This is injected into handler constructors alongside the pool itself:

- Handlers that only need static queries receive `*db.Queries`.
- Handlers that also need dynamic queries (user-games list, built later with goqu) additionally receive `*pgxpool.Pool`.

**DBTX interface:** sqlc generates a `DBTX` interface satisfied by both `*pgxpool.Pool` and `pgx.Tx`. `db.New(tx)` produces a transaction-scoped `*db.Queries`, enabling handlers that need atomic multi-statement operations to call `pool.BeginTx()` and pass the resulting `pgx.Tx` directly. The interface is automatically generated — no manual wiring is required.

No handler code is written in this step. This pattern is established here so the next spec (games API) can assume it without re-litigating the design.

---

## Verification

The `sqlc` binary is already present in `devenv.nix` under `packages` — no changes to the devenv configuration are needed.

After `make sqlc` succeeds:

- `internal/db/gen/` is populated.
- `go build ./...` succeeds (nothing imports the generated package yet, but it must be valid Go).
- `go vet ./...` is clean.

---

## Out of scope

- `internal/filter/` — goqu dynamic filter builder, separate spec.
- Any API handler code.
- Phase 3+ query files (`jobs`, `job_items`, `pending_tasks`, `external_games`, `user_sync_configs`, `backup_config`).
- Auth query files (`users`, `user_sessions`) — auth uses raw `pool.QueryRow` per the master spec.
- Platform/storefront write queries — there are none. Platforms and storefronts are static reference data managed exclusively via migrations.

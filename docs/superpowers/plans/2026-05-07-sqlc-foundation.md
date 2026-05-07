# sqlc Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Write `sqlc.yaml`, all Phase 2 SQL query files, run code generation, and commit the generated `internal/db/gen/` package so it's ready for Phase 2 handler code to build on.

**Architecture:** `sqlc generate` reads the existing migration schema at `internal/db/migrations/` and the new hand-written query files at `internal/db/queries/`, and emits type-safe Go into `internal/db/gen/`. The generated package is committed to the repo so contributors do not need sqlc installed to build. No handler code is written here.

**Tech Stack:** sqlc (already in devenv.nix), pgx/v5, Go 1.25

---

## File Map

| Action | Path |
|--------|------|
| Create | `sqlc.yaml` |
| Create | `internal/db/queries/games.sql` |
| Create | `internal/db/queries/user_games.sql` |
| Create | `internal/db/queries/user_game_platforms.sql` |
| Create | `internal/db/queries/platforms.sql` |
| Create | `internal/db/queries/storefronts.sql` |
| Create | `internal/db/queries/platform_storefronts.sql` |
| Create | `internal/db/queries/tags.sql` |
| Create | `internal/db/queries/user_game_tags.sql` |
| Generate + commit | `internal/db/gen/` (db.go, models.go, *.sql.go) |

---

### Task 1: Create feature branch and `sqlc.yaml`

**Files:**
- Create: `sqlc.yaml`

- [ ] **Step 1: Create the feature branch**

```bash
git checkout -b feat/sqlc-foundation
```

Expected: branch `feat/sqlc-foundation` created.

- [ ] **Step 2: Verify sqlc is available**

```bash
devenv shell -- sqlc version
```

Expected output: something like `v1.27.x` (exact version varies; any v1.x is fine).

- [ ] **Step 3: Write `sqlc.yaml`**

Create `sqlc.yaml` at the project root (alongside `Makefile`):

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

- [ ] **Step 4: Commit `sqlc.yaml`**

```bash
git add sqlc.yaml
git commit -m "chore: add sqlc.yaml for Phase 2 query generation"
```

---

### Task 2: Write `internal/db/queries/games.sql`

**Files:**
- Create: `internal/db/queries/games.sql`

- [ ] **Step 1: Create the queries directory**

```bash
mkdir -p internal/db/queries
```

- [ ] **Step 2: Write `games.sql`**

Create `internal/db/queries/games.sql`:

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
    $21, $22, $23
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
    title                       = $2,
    description                 = $3,
    genre                       = $4,
    developer                   = $5,
    publisher                   = $6,
    release_date                = $7,
    rating_average              = $8,
    rating_count                = $9,
    estimated_playtime_hours    = $10,
    howlongtobeat_main          = $11,
    howlongtobeat_extra         = $12,
    howlongtobeat_completionist = $13,
    game_modes                  = $14,
    themes                      = $15,
    player_perspectives         = $16,
    game_metadata               = $17,
    igdb_slug                   = $18,
    igdb_platform_ids           = $19,
    igdb_platform_names         = $20,
    last_updated                = now()
WHERE id = $1
RETURNING *;

-- name: UpdateGameCoverArtUrl :exec
UPDATE games SET cover_art_url = $2 WHERE id = $1;

-- name: DeleteGame :exec
DELETE FROM games WHERE id = $1;

-- name: SearchGamesByTitle :many
SELECT * FROM games
WHERE title ILIKE '%' || $1 || '%'
   OR (description IS NOT NULL AND description ILIKE '%' || $1 || '%')
ORDER BY title
LIMIT $2;
```

- [ ] **Step 3: Commit**

```bash
git add internal/db/queries/games.sql
git commit -m "feat: add games.sql sqlc query file"
```

---

### Task 3: Write `internal/db/queries/user_games.sql`

**Files:**
- Create: `internal/db/queries/user_games.sql`

- [ ] **Step 1: Write `user_games.sql`**

Create `internal/db/queries/user_games.sql`:

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

-- name: CountUserGamesByGameID :one
-- Used by unreferenced-game cleanup: after deleting a user_game, the handler
-- checks this count; if zero, the games row and its cover art file are deleted.
SELECT COUNT(*) FROM user_games WHERE game_id = $1;
```

- [ ] **Step 2: Commit**

```bash
git add internal/db/queries/user_games.sql
git commit -m "feat: add user_games.sql sqlc query file"
```

---

### Task 4: Write `internal/db/queries/user_game_platforms.sql`

**Files:**
- Create: `internal/db/queries/user_game_platforms.sql`

- [ ] **Step 1: Write `user_game_platforms.sql`**

Create `internal/db/queries/user_game_platforms.sql`:

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
    store_game_id    = $3,
    store_url        = $4,
    is_available     = $5,
    hours_played     = $6,
    ownership_status = $7,
    acquired_date    = $8,
    updated_at       = now()
WHERE id = $1 AND user_game_id = $2
RETURNING *;

-- name: DeleteUserGamePlatform :exec
DELETE FROM user_game_platforms WHERE id = $1 AND user_game_id = $2;
```

- [ ] **Step 2: Commit**

```bash
git add internal/db/queries/user_game_platforms.sql
git commit -m "feat: add user_game_platforms.sql sqlc query file"
```

---

### Task 5: Write static reference data query files

**Files:**
- Create: `internal/db/queries/platforms.sql`
- Create: `internal/db/queries/storefronts.sql`
- Create: `internal/db/queries/platform_storefronts.sql`

These are all read-only. Platforms, storefronts, and their join table are static reference data managed exclusively via migrations — there are no runtime write operations.

- [ ] **Step 1: Write `platforms.sql`**

Create `internal/db/queries/platforms.sql`:

```sql
-- name: ListPlatforms :many
SELECT * FROM platforms ORDER BY display_name;

-- name: GetPlatform :one
SELECT * FROM platforms WHERE name = $1;
```

- [ ] **Step 2: Write `storefronts.sql`**

Create `internal/db/queries/storefronts.sql`:

```sql
-- name: ListStorefronts :many
SELECT * FROM storefronts ORDER BY display_name;

-- name: GetStorefront :one
SELECT * FROM storefronts WHERE name = $1;
```

- [ ] **Step 3: Write `platform_storefronts.sql`**

Create `internal/db/queries/platform_storefronts.sql`:

```sql
-- name: ListStorefrontsForPlatform :many
SELECT s.* FROM storefronts s
JOIN platform_storefronts ps ON ps.storefront = s.name
WHERE ps.platform = $1
ORDER BY s.display_name;
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/queries/platforms.sql \
        internal/db/queries/storefronts.sql \
        internal/db/queries/platform_storefronts.sql
git commit -m "feat: add platforms, storefronts, platform_storefronts sqlc query files"
```

---

### Task 6: Write tags query files

**Files:**
- Create: `internal/db/queries/tags.sql`
- Create: `internal/db/queries/user_game_tags.sql`

- [ ] **Step 1: Write `tags.sql`**

Create `internal/db/queries/tags.sql`:

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

- [ ] **Step 2: Write `user_game_tags.sql`**

Create `internal/db/queries/user_game_tags.sql`:

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

- [ ] **Step 3: Commit**

```bash
git add internal/db/queries/tags.sql internal/db/queries/user_game_tags.sql
git commit -m "feat: add tags and user_game_tags sqlc query files"
```

---

### Task 7: Generate code, verify, and commit

**Files:**
- Generate: `internal/db/gen/db.go`
- Generate: `internal/db/gen/models.go`
- Generate: `internal/db/gen/games.sql.go`
- Generate: `internal/db/gen/user_games.sql.go`
- Generate: `internal/db/gen/user_game_platforms.sql.go`
- Generate: `internal/db/gen/platforms.sql.go`
- Generate: `internal/db/gen/storefronts.sql.go`
- Generate: `internal/db/gen/platform_storefronts.sql.go`
- Generate: `internal/db/gen/tags.sql.go`
- Generate: `internal/db/gen/user_game_tags.sql.go`

- [ ] **Step 1: Run sqlc generation**

```bash
devenv shell -- make sqlc
```

Expected: exits 0 with no errors. If sqlc reports errors about missing columns or type mismatches, they indicate a discrepancy between the query file and the schema — fix the query file (do NOT edit generated files).

Common error patterns and fixes:
- `column "X" does not exist` → check the column name against `internal/db/migrations/0001_initial.up.sql`
- `unknown parameter type` → may need to cast, e.g. `$1::integer[]`
- `query "X" used with ":one" but returns no rows in schema` → wrong result annotation (`:one` vs `:exec`)

- [ ] **Step 2: Verify `internal/db/gen/` was populated**

```bash
ls internal/db/gen/
```

Expected: shows `db.go`, `models.go`, and one `.sql.go` file per query file:
```
db.go
games.sql.go
models.go
platform_storefronts.sql.go
platforms.sql.go
storefronts.sql.go
tags.sql.go
user_game_platforms.sql.go
user_game_tags.sql.go
user_games.sql.go
```

- [ ] **Step 3: Verify the generated code builds cleanly**

```bash
devenv shell -- go build ./...
```

Expected: exits 0, no output. Nothing imports `internal/db/gen` yet, but the package itself must compile.

- [ ] **Step 4: Run `go vet`**

```bash
devenv shell -- go vet ./...
```

Expected: exits 0, no output.

- [ ] **Step 5: Run the existing test suite to confirm nothing regressed**

```bash
devenv shell -- go test ./...
```

Expected: all tests pass. The new `internal/db/gen/` package has no tests of its own (generated code is not tested directly), but the rest of the suite must remain green.

- [ ] **Step 6: Commit the generated output**

```bash
git add internal/db/gen/
git commit -m "feat: generate sqlc db package from Phase 2 query files"
```

---

## Notes for handler code (not implemented here)

These are reference points for the next spec — recorded here so the decision context is preserved:

**`pgtype.Numeric`:** PostgreSQL `NUMERIC` columns (`rating_average`, `hours_played`, `howlongtobeat_*`) map to `pgtype.Numeric` in generated structs. Handler projection code must convert to `float64`/`*float64` when building JSON responses.

**Platform/Storefront icon field:** The generated `Platform` and `Storefront` structs have `Icon *string` (filename only, e.g. `"pc-windows.svg"`). Handlers that return these must construct the full URL as `/logos/platforms/<slug>/<icon>` — do NOT use `config.StaticURL` (that is for cover art only).

**`DBTX` interface:** The generated `db.go` exports a `DBTX` interface satisfied by both `*pgxpool.Pool` and `pgx.Tx`. Pass `pool.BeginTx(ctx, pgx.TxOptions{})` to `db.New(tx)` for atomic multi-statement handlers.

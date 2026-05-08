# Bun Migration — Design Spec

**Date:** 2026-05-07
**Status:** Approved

## Overview

This spec covers the mechanical work to replace the three-tool database stack (sqlc + goqu/sqlx + golang-migrate) with Bun (`uptrace/bun`) across the current state of the codebase. It does not add new features — everything the code does today must work the same way afterward. The scope is exactly what currently exists on the main branch.

---

## What Changes and What Stays the Same

### Removed entirely

| Thing | Why |
|---|---|
| `internal/db/gen/` (10 generated files) | Replaced by hand-written Bun model structs |
| `internal/db/queries/` (8 SQL files) | Replaced by Bun query builder calls co-located with handlers |
| `sqlc.yaml` | No longer needed |
| `github.com/golang-migrate/migrate/v4` | Replaced by `github.com/uptrace/bun/migrate` |
| `github.com/doug-martin/goqu/v9` | Replaced by Bun's composable SelectQuery |
| `github.com/jmoiron/sqlx` | Replaced by Bun (also manages its own database/sql pool) |
| `github.com/jackc/pgx/v5` (pgxpool) | Replaced by `bun/driver/pgdriver` |
| `sqlc` in `devenv.nix` | No longer needed |
| `go-migrate` CLI in `devenv.nix` | No longer needed |
| `generate` target in Makefile | No longer needed |

### Added

| Thing | Purpose |
|---|---|
| `github.com/uptrace/bun` | Core ORM + query builder |
| `github.com/uptrace/bun/dialect/pgdialect` | PostgreSQL dialect for Bun |
| `github.com/uptrace/bun/driver/pgdriver` | PostgreSQL driver (replaces pgx) |
| `github.com/uptrace/bun/migrate` | Migration runner (replaces golang-migrate) |
| `internal/db/models/` | Hand-written Bun model structs (one file per domain) |

### Unchanged

- All SQL migration files in `internal/db/migrations/` — content is identical; only the filename format changes (see Migrations section)
- The `internal/migrate/migrator.go` state machine logic — `AppState`, `StartDBProbe`, the log channel, `NeedsSetup` — none of this changes, with the exception of `recoverFromUnavailable` (see Migrator Changes)
- All `internal/api/` handler logic — same queries, different API to invoke them
- `internal/auth/` — raw SQL queries become `db.NewRaw(...)`
- All tests — testcontainers-go still works; Bun runs against real Postgres

---

## Migration File Format Change

Bun uses timestamp-based filenames instead of golang-migrate's sequential integer format. The naming convention is:

```
golang-migrate:  0001_initial.up.sql
Bun:             20260503000001_initial.up.sql
```

The `Migrations.Discover(sqlMigrations)` call in Bun discovers all `.up.sql` files from an embedded FS, sorted lexicographically — which is correct when using the timestamp prefix.

**Action:** rename existing migration files in `internal/db/migrations/` to the timestamp format. The content of each file is unchanged.

---

## DB Connection

Replace the pgxpool setup in `main.go` with Bun's pgdriver:

```go
// Before
pool, err := pgxpool.New(ctx, resolvedDatabaseURL)

// After
import (
    "database/sql"
    "github.com/uptrace/bun"
    "github.com/uptrace/bun/dialect/pgdialect"
    "github.com/uptrace/bun/driver/pgdriver"
)

sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(resolvedDatabaseURL)))
db := bun.NewDB(sqldb, pgdialect.New())
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
```

`pgdriver` accepts standard `postgres://` URLs — the `pgx5://` scheme rewriting in `migrator.go` is removed.

The `*bun.DB` is the single dependency injected everywhere that previously received `*pgxpool.Pool` or `*db.Queries`.

---

## Migrator Changes

The three methods that touch the underlying migration library are replaced. Everything else in `migrator.go` is untouched.

### Struct fields

```go
// Before
type Migrator struct {
    // ...
    databaseURL string
    src         migsource.Driver
    m           *gmigrate.Migrate
    // ...
}

// After
type Migrator struct {
    // ...
    db      *bun.DB
    bunMig  *bunmigrate.Migrator  // nil until first determineState() call
    // ...
}
```

`NewMigrator` now receives `*bun.DB` instead of a URL string.

### `determineState()`

```go
// Before: creates gmigrate.Migrate, calls m.Version(), checks dirty flag, counts pending via iofs walk
// After:
func (mg *Migrator) determineState() error {
    if mg.bunMig == nil {
        mg.bunMig = bunmigrate.NewMigrator(mg.db, migrations.Migrations)
        if err := mg.bunMig.Init(context.Background()); err != nil {
            return fmt.Errorf("determine state: init: %w", err)
        }
    }
    ms, err := mg.bunMig.MigrationsWithStatus(context.Background())
    if err != nil {
        return fmt.Errorf("determine state: %w", err)
    }
    if len(ms.Unapplied()) > 0 {
        mg.state.Store(int32(AppStateNeedsMigration))
    } else {
        mg.state.Store(int32(AppStateReady))
    }
    return nil
}
```

The `dirty` state concept from golang-migrate does not exist in Bun (Bun uses a locks table to prevent concurrent runs). The dirty check is removed.

### `PendingCount()`

`PendingCount()` must guard against `mg.bunMig` being nil — it can be called from tests before `determineState()` has run. If `bunMig` is nil, call `determineState()` first:

```go
func (mg *Migrator) PendingCount() (int, error) {
    if mg.bunMig == nil {
        if err := mg.determineState(); err != nil {
            return 0, fmt.Errorf("pending count: init: %w", err)
        }
    }
    ms, err := mg.bunMig.MigrationsWithStatus(context.Background())
    if err != nil {
        return 0, fmt.Errorf("pending count: %w", err)
    }
    return len(ms.Unapplied()), nil
}
```

### `RunMigrations()`

```go
// After:
func (mg *Migrator) RunMigrations(ctx context.Context) error {
    mg.migrateMu.Lock()
    defer mg.migrateMu.Unlock()

    if AppState(mg.state.Load()) == AppStateMigrating {
        return fmt.Errorf("migrations already in progress")
    }

    ch := make(chan string, 256)
    mg.logCh = ch
    mg.state.Store(int32(AppStateMigrating))

    if err := mg.bunMig.Lock(ctx); err != nil {
        mg.state.Store(int32(AppStateNeedsMigration))
        close(ch)
        return fmt.Errorf("migrate: acquire lock: %w", err)
    }
    defer mg.bunMig.Unlock(ctx)

    group, err := mg.bunMig.Migrate(ctx)
    if err != nil {
        mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", err))
        mg.state.Store(int32(AppStateNeedsMigration))
        close(ch)
        return err
    }
    if group.IsZero() {
        mg.sendLog(ch, "No new migrations to run\n")
    } else {
        mg.sendLog(ch, fmt.Sprintf("Migrated to group %s\n", group))
    }
    close(ch)
    return nil
}
```

The `logAdapter` / `mg.m.Log` pattern is replaced by `mg.sendLog` — a small helper that writes to either the channel or the writer, same as before:

```go
func (mg *Migrator) sendLog(ch chan string, line string) {
    if mg.logWriter != nil {
        _, _ = fmt.Fprint(mg.logWriter, line)
        return
    }
    select {
    case ch <- line:
    default: // drop if buffer full
    }
}
```

### `Close()`

```go
// After: nothing to close; Bun's migrator holds no independent connection
func (mg *Migrator) Close() error { return nil }
```

### `recoverFromUnavailable` — one line to update

The state machine method `recoverFromUnavailable` contains a block that resets the golang-migrate instance to force `determineState()` to reconnect:

```go
// Before — remove this
if mg.m != nil {
    _, _ = mg.m.Close()
    mg.m = nil
}
```

Replace with the Bun equivalent:

```go
// After
if mg.bunMig != nil {
    mg.bunMig = nil
}
```

Setting `bunMig` to nil causes the next `determineState()` call to recreate it with a fresh connection — the same intent as before.

### Probe method signature

`StartDBProbe` changes its second parameter from `*pgxpool.Pool` to `*bun.DB`. The ping call changes from `pool.Ping(pingCtx)` to `db.PingContext(pingCtx)`. `InitNeedsSetup` similarly changes from `pool.QueryRow(...)` to a scalar raw query via the underlying `database/sql` interface:

```go
// Before
err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)

// After — use QueryRowContext, not db.NewRaw, for scalar results
err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
```

`db.NewRaw(...).Scan(ctx, &count)` is designed to scan into structs or slices, not bare scalars. Use `db.QueryRowContext` (which delegates to the underlying `database/sql` `*sql.DB`) for single scalar values.

---

## Model Structs

Delete `internal/db/gen/` entirely. Create `internal/db/models/` with one file per domain. Each file defines the struct(s) for that domain with `bun:""` struct tags and standard Go pointer types for nullable columns.

### Type mapping

| Previously (pgtype) | Now (Go stdlib) |
|---|---|
| `pgtype.Text` | `*string` |
| `pgtype.Int4` | `*int32` |
| `pgtype.Numeric` | `*float64` |
| `pgtype.Date` | `*time.Time` |
| `pgtype.Timestamptz` (nullable) | `*time.Time` |
| `pgtype.Timestamptz` (not null) | `time.Time` |

### Example: `internal/db/models/game.go`

```go
package models

import (
    "time"
    "github.com/uptrace/bun"
)

type Game struct {
    bun.BaseModel `bun:"table:games"`

    ID                         int32      `bun:"id,pk"                          json:"id"`
    Title                      string     `bun:"title,notnull"                  json:"title"`
    Description                *string    `bun:"description"                    json:"description"`
    Genre                      *string    `bun:"genre"                          json:"genre"`
    Developer                  *string    `bun:"developer"                      json:"developer"`
    Publisher                  *string    `bun:"publisher"                      json:"publisher"`
    ReleaseDate                *time.Time `bun:"release_date"                   json:"release_date"`
    CoverArtUrl                *string    `bun:"cover_art_url"                  json:"cover_art_url"`
    RatingAverage              *float64   `bun:"rating_average"                 json:"rating_average"`
    RatingCount                *int32     `bun:"rating_count"                   json:"rating_count"`
    EstimatedPlaytimeHours     *int32     `bun:"estimated_playtime_hours"       json:"estimated_playtime_hours"`
    HowlongtobeatMain          *float64   `bun:"howlongtobeat_main"             json:"howlongtobeat_main"`
    HowlongtobeatExtra         *float64   `bun:"howlongtobeat_extra"            json:"howlongtobeat_extra"`
    HowlongtobeatCompletionist *float64   `bun:"howlongtobeat_completionist"    json:"howlongtobeat_completionist"`
    IgdbSlug                   *string    `bun:"igdb_slug"                      json:"igdb_slug"`
    IgdbPlatformIds            *string    `bun:"igdb_platform_ids"              json:"igdb_platform_ids"`
    IgdbPlatformNames          *string    `bun:"igdb_platform_names"            json:"igdb_platform_names"`
    GameModes                  *string    `bun:"game_modes"                     json:"game_modes"`
    Themes                     *string    `bun:"themes"                         json:"themes"`
    PlayerPerspectives         *string    `bun:"player_perspectives"            json:"player_perspectives"`
    GameMetadata               *string    `bun:"game_metadata"                  json:"game_metadata"`
    LastUpdated                time.Time  `bun:"last_updated,notnull"           json:"last_updated"`
    CreatedAt                  time.Time  `bun:"created_at,notnull"             json:"created_at"`
}
```

Write equivalent structs for: `UserGame`, `UserGamePlatform`, `Platform`, `Storefront`, `PlatformStorefront`, `Tag`, `UserGameTag`, `User`, `UserSession`.

---

## Query Rewrites

### Handler dependency injection

All handlers that previously received `*db.Queries` now receive `*bun.DB`. The `Handler` struct field changes from `queries *db.Queries` to `db *bun.DB`.

### Static queries — before/after examples

**Get by ID:**
```go
// Before (sqlc)
game, err := h.queries.GetGame(ctx, int32(id))

// After (Bun)
var game models.Game
err := h.db.NewSelect().Model(&game).Where("id = ?", id).Scan(ctx)
if errors.Is(err, sql.ErrNoRows) { /* 404 */ }
```

**Upsert:**
```go
// Before (sqlc)
game, err := h.queries.UpsertGame(ctx, db.UpsertGameParams{...})

// After (Bun)
game := &models.Game{ /* fields */ }
_, err := h.db.NewInsert().Model(game).
    On("CONFLICT (id) DO UPDATE").
    Set("title = EXCLUDED.title, ...").
    Returning("*").
    Exec(ctx)
```

**Raw SQL (auth queries, complex cases):**
```go
// Before (raw pgxpool)
err := h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)

// After — scalar values: use QueryRowContext (delegates to database/sql)
err := h.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)

// After — struct/slice results: use NewRaw
var users []models.User
err := h.db.NewRaw("SELECT * FROM users WHERE is_active = ?", true).Scan(ctx, &users)
```

`db.NewRaw(...).Scan()` works for struct and slice targets. For single scalar values (`int`, `string`, etc.) always use `db.QueryRowContext(...).Scan(&val)` via the underlying `database/sql` interface.

### Dynamic filter queries — new implementation

`internal/filter/` does not exist yet. This spec is where it gets built, using Bun's `SelectQuery` as the foundation. The filterBuilder was previously planned in its own spec; that spec is superseded by this one.

```go
// builder.go
type filterBuilder struct {
    joins    map[string]string // alias → "LEFT JOIN table AS alias ON ..." (deduplicates by alias)
    wheres   []func(*bun.SelectQuery) *bun.SelectQuery
    havings  []func(*bun.SelectQuery) *bun.SelectQuery
}

func newFilterBuilder() *filterBuilder {
    return &filterBuilder{joins: make(map[string]string)}
}

func (f *filterBuilder) AddJoin(alias, clause string) {
    f.joins[alias] = clause
}

func (f *filterBuilder) AddWhere(fn func(*bun.SelectQuery) *bun.SelectQuery) {
    f.wheres = append(f.wheres, fn)
}

func (f *filterBuilder) AddHaving(fn func(*bun.SelectQuery) *bun.SelectQuery) {
    f.havings = append(f.havings, fn)
}

func (f *filterBuilder) Apply(q *bun.SelectQuery) *bun.SelectQuery {
    for _, clause := range f.joins {
        q = q.Join(clause)
    }
    for _, fn := range f.wheres {
        q = fn(q)
    }
    for _, fn := range f.havings {
        q = fn(q)
    }
    return q
}
```

Criterion handler example:
```go
func applyPlayStatusFilter(fb *filterBuilder, status string) {
    if status == "" {
        return
    }
    fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
        return q.Where("ug.play_status = ?", status)
    })
}
```

### Criteria to implement (`criteria.go`)

One function per filter in `internal/filter/criteria.go`. Each receives a `*filterBuilder` and the criterion value. If the value is zero/nil/empty it must be a no-op.

| Parameter | Type | Join required | SQL logic |
|---|---|---|---|
| `play_status` | `string` | none | `user_games.play_status = ?` |
| `ownership_status` | `string` | `user_game_platforms` | `user_game_platforms.ownership_status = ?` |
| `is_loved` | `*bool` | none | `user_games.is_loved = ?` |
| `rating_min` | `*float64` | none | `user_games.personal_rating >= ?` |
| `rating_max` | `*float64` | none | `user_games.personal_rating <= ?` |
| `has_notes` | `*bool` | none | `personal_notes IS NOT NULL AND personal_notes != ''` (true) or `IS NULL OR = ''` (false) |
| `platform` | `[]string` | `user_game_platforms` | Multi-value; `"unknown"` maps to NULL (see below) |
| `storefront` | `[]string` | `user_game_platforms` | Multi-value; `"unknown"` maps to NULL (see below) |
| `genre` | `[]string` | `games` | OR of `games.genre ILIKE '%' || ? || '%'` for each value |
| `game_mode` | `[]string` | `games` | OR of `games.game_modes ILIKE '%' || ? || '%'` for each value |
| `theme` | `[]string` | `games` | OR of `games.themes ILIKE '%' || ? || '%'` for each value |
| `player_perspective` | `[]string` | `games` | OR of `games.player_perspectives ILIKE '%' || ? || '%'` for each value |
| `tag` | `[]string` | none (subquery) | `user_games.id IN (SELECT user_game_id FROM user_game_tags WHERE tag_id IN (...))` |
| `q` | `string` | `games` | `games.title ILIKE ? OR (user_games.personal_notes IS NOT NULL AND user_games.personal_notes ILIKE ?)` |

JOIN conditions:
- `user_game_platforms`: `LEFT JOIN user_game_platforms ugp ON ugp.user_game_id = ug.id`
- `games`: `LEFT JOIN games g ON g.id = ug.game_id`

**`"unknown"` sentinel for platform/storefront:** The value `"unknown"` means "games with no platform/storefront set":
- `platform=["unknown"]` → `ugp.platform IS NULL`
- `platform=["steam"]` → `ugp.platform IN ('steam')`
- `platform=["steam","unknown"]` → `ugp.platform = 'steam' OR ugp.platform IS NULL`

Same logic applies to `storefront`. Use `WhereGroup` with `WhereOr` inside for the mixed case.

**Security:** The filterBuilder never adds a `user_id` scope. The caller (user-games handler) is responsible for adding `WHERE ug.user_id = ?` to the base query before calling `Apply()`. Omitting this would expose all users' games to any authenticated user.

### Transactions

```go
// Before (pgx)
tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{})
qtx := h.queries.WithTx(tx)

// After (Bun)
tx, err := h.db.BeginTx(ctx, nil)
// use tx directly with Bun query builders:
_, err = tx.NewInsert().Model(&userGame).Exec(ctx)
```

---

## Migrations Package

Create `internal/db/migrations/migrations.go`:

```go
package migrations

import (
    "embed"
    "github.com/uptrace/bun/migrate"
)

//go:embed *.sql
var sqlFiles embed.FS

var Migrations = migrate.NewMigrations()

func init() {
    if err := Migrations.Discover(sqlFiles); err != nil {
        panic(err)
    }
}
```

This replaces the current `internal/db/migrations/migrations.go` which uses `iofs` from golang-migrate.

---

## Files to Delete

```
internal/db/gen/               (entire directory)
internal/db/queries/           (entire directory)
sqlc.yaml
```

## Files to Create

```
internal/db/models/game.go
internal/db/models/user_game.go
internal/db/models/user_game_platform.go
internal/db/models/platform.go
internal/db/models/storefront.go
internal/db/models/platform_storefront.go
internal/db/models/tag.go
internal/db/models/user_game_tag.go
internal/db/models/user.go
internal/db/models/user_session.go
internal/filter/builder.go       (new — see Dynamic filter queries section)
internal/filter/criteria.go      (new — see Criteria to implement section)
```

## Files to Modify

| File | Change |
|---|---|
| `go.mod` / `go.sum` | Remove sqlc, pgx, golang-migrate, goqu, sqlx deps; add bun |
| `devenv.nix` | Remove `sqlc`, `go-migrate` from packages |
| `Makefile` | Remove `generate` target; remove it from `all` |
| `cmd/nexorious/main.go` | Replace pgxpool setup with Bun setup; pass `*bun.DB` everywhere |
| `internal/migrate/migrator.go` | Replace golang-migrate internals (see Migrator Changes above) |
| `internal/migrate/migrator_test.go` | Update for new `NewMigrator(*bun.DB)` signature |
| `internal/migrate/handler.go` | `InitNeedsSetup` uses `db.NewRaw(...)` |
| `internal/api/router.go` | Inject `*bun.DB` instead of `*db.Queries` + `*pgxpool.Pool` |
| `internal/api/auth.go` | Raw queries → `db.NewRaw(...)` |
| `internal/api/auth_test.go` | Update setup helpers |
| `internal/api/setup.go` | Raw queries → `db.NewRaw(...)` |
| `internal/api/setup_test.go` | Update setup helpers |
| `internal/api/db_error.go` | Update PostgreSQL error type detection: `*pgconn.PgError` (pgx) → `pgdriver.Error` (Bun's driver). Code checks like unique-constraint detection (`Code == "23505"`) must use `pgdriver.Error.Field('C')` or the equivalent. Without this change, constraint violations silently return 500 instead of 409. |
| `internal/db/migrations/migrations.go` | Replace iofs with Bun Discover (see above) |
| Any file importing `internal/db/gen` | Switch to `internal/db/models` |
| Any file importing `pgx/v5/pgxpool` | Remove |
| Any file importing `goqu`, `sqlx` | Remove |

---

## Verification Steps

After each logical chunk of changes, verify:

1. `go build ./...` — no compile errors
2. `go vet ./...` — clean
3. `go test ./...` — all tests pass (testcontainers-go spins up real Postgres)
4. `golangci-lint run` — no new lint errors

Final smoke test: start the binary, verify the migration UI appears, run migrations, verify the setup page appears, create admin, verify the React SPA loads.

---

## Out of Scope

- Adding new API handlers or query files (Phase 2+ work)
- Changing any SQL migration content
- Changing any API behaviour
- Adding Bun model structs for Phase 3+ tables (jobs, job_items, pending_tasks, external_games, user_sync_configs, backup_config) — these are written when those phases are implemented
- `docs/superpowers/specs/2026-05-07-filter-package.md` is superseded by this spec; `internal/filter/builder.go` is implemented here

# Migration System Design

**Date:** 2026-05-05
**Status:** Approved
**Phase:** Phase 1 — Infrastructure Skeleton

## Overview

Implements the golang-migrate runner, app-state machine (`NeedsMigration → Migrating → Ready`), browser migration UI with SSE progress streaming, and the Echo app-state middleware. A minimal stub migration (`0001_initial`) is included; the full schema is built incrementally alongside the API handlers.

---

## Scope

- `internal/migrate/migrator.go` — state machine + golang-migrate wrapper
- `internal/migrate/handler.go` — Echo handlers for migration routes
- `internal/db/migrations/0001_initial.up.sql` / `.down.sql` — minimal stub
- `ui/ui.go` — two `embed.FS` vars (`UIBox`, `MigrateBox`)
- `ui/migrate/migrate.html` — standalone migration UI (Go template, vanilla JS)
- `ui/dist/.gitkeep` — placeholder so `//go:embed dist` compiles before `make frontend`
- `internal/api/router.go` — app-state middleware + migration route registration; `api.New` gains `*migrate.Migrator` parameter
- `cmd/nexorious/main.go` — create `Migrator`, pass to `api.New`
- `go.mod` / `go.sum` — add golang-migrate dependencies

---

## Dependencies

New entries in `go.mod`:

```
github.com/golang-migrate/migrate/v4
github.com/golang-migrate/migrate/v4/database/pgx/v5   (pgx driver adapter)
github.com/golang-migrate/migrate/v4/source/iofs       (embed.FS source)
```

---

## State Machine

```
NeedsMigration → Migrating → Ready
                     ↓ (on error)
               NeedsMigration
```

`AppState` is an `int32` constant; stored as `atomic.Int32` inside `Migrator`.

```go
type AppState int32

const (
    AppStateNeedsMigration AppState = iota
    AppStateMigrating
    AppStateReady
)
```

---

## `internal/migrate/migrator.go`

```go
type Migrator struct {
    state       atomic.Int32   // AppState
    databaseURL string         // DSN passed to golang-migrate's pgx/v5 driver
    logCh       chan string     // SSE log lines; buffered (256); closed on RunMigrations completion
    logWriter   io.Writer      // alternate log sink for --migrate-only mode (slog-backed); nil = use logCh
    mu          sync.Mutex     // prevents concurrent RunMigrations calls
}
```

**Note on the driver:** golang-migrate's pgx/v5 driver opens its own connection internally using a DSN string — it does not accept a `*pgxpool.Pool`. `NewMigrator` therefore takes `databaseURL string` (from `cfg.DatabaseURL`) rather than the pool. The pool is not stored in the `Migrator` struct; it is managed entirely by `main.go` and passed separately to API handlers.

**`logWriter` field:** When `logWriter` is non-nil, the golang-migrate logger adapter writes to it instead of `logCh`. This is used in `--migrate-only` mode (no HTTP server, no SSE consumer) so that migration output reaches `slog`/stdout rather than being silently buffered and discarded. `main.go` sets this via a `WithLogWriter(w io.Writer)` option or a dedicated constructor before calling `RunMigrations`. When `logWriter` is nil (normal server mode), log lines go to `logCh` as usual.

### Methods

| Method | Description |
|---|---|
| `NewMigrator(ctx, databaseURL) (*Migrator, error)` | Creates struct; checks pending migrations; handles dirty state; sets `NeedsMigration` or `Ready` |
| `State() AppState` | Atomic read |
| `PendingCount() (int, error)` | Number of unapplied migrations |
| `CurrentVersion() (uint, bool, error)` | Current version + dirty flag |
| `LogCh() <-chan string` | Returns current log channel for SSE handler |
| `RunMigrations(ctx) error` | Transitions state, runs `m.Up()`, streams logs, transitions to `Ready` or back to `NeedsMigration` on failure |

### Dirty state handling in `NewMigrator`

If `dirty=true` is returned by golang-migrate at startup (a previous migration failed mid-run), `NewMigrator` logs a clear error message — e.g. `"database schema is dirty at version N — manual intervention required (run migrate force N-1 or fix the migration)"` — and sets state to `NeedsMigration`. The migration UI will be shown and the admin can investigate. The binary does **not** crash; it does **not** attempt to auto-fix the dirty state.

### RunMigrations behaviour

1. Acquire `mu` — return error if already `Migrating`
2. Create fresh buffered `logCh` (capacity 256)
3. Set state to `Migrating`
4. Run `m.Up()` in the same goroutine (caller is already in a goroutine from the HTTP handler)
5. On success: set state to `Ready`, close `logCh`; **deferred hook:** when workers and the scheduler are added (Phase 3), the `Ready` transition here is where `pool.Start()` and `scheduler.Start()` will be called — a `OnReady func()` callback on the `Migrator` struct is the intended extension point
6. On error: set state to `NeedsMigration`, send error line to `logCh`, close `logCh`, return error

golang-migrate's logger interface is satisfied by a small adapter that writes to `logCh`.

---

## `internal/migrate/handler.go`

```go
type Handler struct {
    migrator *Migrator
}
```

### Routes (all in migration zone — bypass app-state middleware)

| Method | Path | Description |
|---|---|---|
| `GET` | `/migrate` | Render `migrate.html` template; inject `PendingCount` and `CurrentVersion` |
| `GET` | `/api/migrate/status` | JSON: `{pending_count, current_version, dirty, state}` |
| `POST` | `/api/migrate/run` | Start migration async; 202 if `NeedsMigration`, 409 if `Migrating`, 400 if `Ready` |
| `GET` | `/api/migrate/progress` | SSE stream from `logCh`; `event: complete` on close |

### SSE protocol (`/api/migrate/progress`)

```
data: Applying migration 1/1...\n\n
data: Migration complete.\n\n
event: complete\ndata: {}\n\n
```

Handler sets `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`. Reads from `migrator.LogCh()` until closed, then sends `event: complete` and returns.

---

## App-State Middleware (`internal/api/router.go`)

Added to the Echo middleware stack after `Recover` and `RequestLogger`, before route groups:

```
if migrator.State() != Ready && !isBypassedPath(path) {
    redirect to /migrate (302)
}
```

The bypass check is implemented as a configurable prefix list (initially `["/migrate"]`) rather than a hardcoded string comparison. This allows the setup zone (`/api/auth/setup*`) to be added to the bypass list when that feature is built in Phase 1, without restructuring the middleware.

The middleware is registered globally via `e.Use()`. Migration routes bypass it purely because their paths match the bypass prefix list — not because of route registration order.

`api.New` signature change:

```go
func New(cfg *config.Config, migrator *migrate.Migrator) *echo.Echo
```

### Route registration order in `registerRoutes`

1. Migration zone: `/migrate` (GET), `/api/migrate/*` (GET/POST)
2. App-state middleware (inserted here — only runs for non-migration paths)
3. CORS middleware — uses `cfg.CORSOrigins`; in production both API and frontend are same-origin so this is a no-op unless `CORS_ORIGINS` is set (development use)
4. Health: `/health`
5. Static files: `/static/cover_art/*`, `/static/logos/*` (Echo `Static` — directories must exist at startup or routes are no-ops)
6. SPA catch-all: serves `ui.UIBox` (`ui/dist/`); unknown paths fall back to `index.html`

---

## `ui/ui.go`

```go
package ui

import "embed"

//go:embed dist
var UIBox embed.FS

//go:embed migrate
var MigrateBox embed.FS
```

`ui/dist/.gitkeep` ensures the `dist/` directory exists so `//go:embed dist` compiles before `make frontend` has been run.

---

## `ui/migrate/migrate.html`

Go template rendered by the migration handler. Template variables: `{{.PendingCount}}`, `{{.CurrentVersion}}`.

Behaviour:
1. Display pending count and current version from template vars
2. "Run Migrations" button — disabled after first click
3. On click: `POST /api/migrate/run`, then open `EventSource('/api/migrate/progress')`
4. Each `message` event: append line to scrollable terminal `<div>`, auto-scroll to bottom
5. On `complete` event: brief success message, then `window.location = '/'`
6. On `EventSource` error: display error message, re-enable button

Inline styles only. No external JS or CSS dependencies.

---

## Stub Migration

### `internal/db/migrations/0001_initial.up.sql`

```sql
CREATE TABLE IF NOT EXISTS schema_info (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

### `internal/db/migrations/0001_initial.down.sql`

```sql
DROP TABLE IF EXISTS schema_info;
```

The full application schema is added in subsequent migrations as each API domain is implemented.

---

## `main.go` Changes

```go
// After pool.Ping succeeds:
migrator, err := migrate.NewMigrator(ctx, cfg.DatabaseURL)
if err != nil {
    slog.Error("failed to initialise migrator", "err", err)
    pool.Close()
    os.Exit(1)
}

// --migrate-only mode: no HTTP server or SSE consumer, so direct log output to slog.
if migrateOnly {
    migrator.SetLogWriter(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Writer())
    if err := migrator.RunMigrations(ctx); err != nil {
        slog.Error("migration failed", "err", err)
        pool.Close()
        os.Exit(1)
    }
    pool.Close()
    os.Exit(0)
}

// HTTP server:
e := api.New(cfg, migrator)
```

---

## Error Handling

| Scenario | Behaviour |
|---|---|
| DB unreachable on startup | `pool.Ping` fails → log + `os.Exit(1)` (existing) |
| `NewMigrator` fails (migrate source error) | Log + `os.Exit(1)` |
| Dirty schema at startup (`dirty=true`) | Log clear error with version number; set state to `NeedsMigration`; migration UI shown; no auto-fix |
| Migration SQL error | State → `NeedsMigration`; error line sent to `logCh`; SSE client sees error line + `complete` event; page re-enables button |
| `POST /api/migrate/run` while `Migrating` | 409 Conflict |
| `POST /api/migrate/run` while `Ready` | 400 Bad Request |
| SSE client disconnects mid-migration | Handler returns; migration continues; `logCh` drains normally (buffered) |

---

## Testing

- `migrator_test.go`: uses testcontainers-go PostgreSQL; verifies `NeedsMigration` → `Ready` transition, `PendingCount` before/after, `CurrentVersion` after
- `handler_test.go`: Echo `httptest`; verifies `/api/migrate/status` JSON shape, `POST /api/migrate/run` status codes (202/409/400), SSE completion event
- App-state middleware tested via `router_test.go`: non-migration path while `NeedsMigration` → 302 to `/migrate`

# Nexorious Go Port — Design Spec

**Date:** 2026-05-03
**Status:** Approved

## Overview

Port of Nexorious (Python FastAPI + NATS + React) to a single Go binary. The Go application serves the existing React SPA, handles all API routes, runs background workers and scheduled maintenance tasks, and manages database migrations — with no external queue or broker dependency.

The React frontend source is moved into the Go repository under `ui/` (matching Stash's layout) and kept otherwise unchanged except for minor build configuration adjustments. A rewrite of the frontend is out of scope.

---

## Goals

- Single self-contained binary: Go HTTP server + background workers + scheduler + migration runner
- Eliminate NATS entirely: goroutines and channels replace JetStream queues; in-process token bucket (or PostgreSQL table) replaces NATS KV rate limiting
- Preserve all existing API routes and behaviour so the React frontend requires minimal changes
- Browser-based migration UI (Stash-style): app starts immediately, gated behind a migration screen when schema updates are pending
- Fresh database schema (no Alembic history ported); existing data migrated via the nexorious JSON export/import format
- Single-instance primary target; PostgreSQL-backed rate limiting available for multi-instance via config flag

---

## Stack

| Concern | Library |
|---|---|
| HTTP framework | `echo/v5` |
| DB driver | `pgx/v5` (`pgxpool`) |
| Type-safe static queries | `sqlc` (code generation) |
| Dynamic filter queries | `sqlx` + `goqu/v9` (see Database Layer) |
| Fuzzy matching (IGDB) | `github.com/paul-mannino/go-fuzzywuzzy` (IGDB ranking only — local DB search uses ILIKE) |
| Migrations | `golang-migrate/v4` |
| JWT | `golang-jwt/jwt/v5` |
| Rate limiting | `golang.org/x/time/rate` + PostgreSQL table (optional) |
| Scheduled tasks | `go-co-op/gocron/v2` |
| Config | `caarlos0/env/v11` |
| Frontend embedding | stdlib `embed` (Stash-pattern: `ui/ui.go` with separate `UIBox` + `MigrateBox`) |
| Testing | stdlib `testing` + `testcontainers-go` |

---

## Architecture

### Startup Sequence

1. Parse config from env vars (+ `.env` file via `godotenv`)
2. `pgxpool.New()` — fatal only on DSN parse error (misconfiguration); not a transient failure
3. `NewMigrator()` — creates the Migrator struct; `state` zero-values to `AppStateDBUnavailable`, which is correct before any DB check
4. `pool.Ping()` — if unreachable, leave state as `AppStateDBUnavailable` and continue (no exit); if reachable, call `initAppState()`
5. `initAppState()` — `determineState()` + (if `Ready`) `InitNeedsSetup()` on the already-created Migrator; sets the correct operational state
6. Start Echo HTTP server (always — serves DB error page, migration UI, setup UI, or the app depending on state; Migrator always exists so middleware is safe)
7. Start `StartDBProbe(shutdownCtx, pool)` goroutine — monitors DB connectivity, drives state transitions, calls `initAppState()` on first recovery if state is still `DBUnavailable`
8. Spawn worker/scheduler gate-loop goroutine — starts workers + gocron only when `State() == Ready && !NeedsSetup()`

Workers and the scheduler are never started until migrations have been applied and setup is complete. Graceful shutdown on `SIGTERM`/`SIGINT` drains the worker queue before the process exits.

### App State Machine

```
DBUnavailable ↔ NeedsMigration → Migrating → Ready
DBUnavailable ↔ Ready
```

`AppStateDBUnavailable` is the new zero value (iota=0). Before any DB check, the state is "unavailable" — which is the correct semantic. All transitions into and out of `DBUnavailable` are driven by the `StartDBProbe` goroutine.

**State constants (`internal/migrate/migrator.go`):**
```go
const (
    AppStateDBUnavailable  AppState = iota  // 0 — zero value; DB unreachable
    AppStateNeedsMigration                  // 1
    AppStateMigrating                       // 2
    AppStateReady                           // 3
)
```

**Migrator struct additions:**
```go
type Migrator struct {
    state     atomic.Int32
    prevState atomic.Int32  // state saved before entering DBUnavailable; restored on recovery
    // ... existing fields
}
```

**`StartDBProbe(ctx, pool)`** — polls `pool.Ping()` every 5 seconds:

- **Ping fails** + state ≠ `DBUnavailable` → save current state to `prevState`, set `DBUnavailable`, log WARN, record `lastUnavailableAt`.
- **Ping succeeds** + state == `DBUnavailable` → three sub-cases based on `prevState`:
  1. `prevState` indicates the Migrator has never been initialised (state is still the zero-value `DBUnavailable` with no prior operational state) → call `onRecovery` callback (`initAppState`: runs `determineState()` + `InitNeedsSetup()`); on success the callback sets the correct state; log INFO.
  2. `prevState == Migrating` → call `determineState()` directly on the existing Migrator to re-consult the DB. Do **not** blindly restore `Migrating`: the migration goroutine that was running when the DB dropped has since failed; the DB now holds whatever state it was in at that point, which `determineState()` will discover correctly. Log INFO.
  3. `prevState == NeedsMigration` or `prevState == Ready` → restore `prevState` directly (safe; these are stable states that cannot have changed during the outage). Then: if the restored state is `Ready` **and** `NeedsSetup()` is still `true`, call `InitNeedsSetup()` to re-check the user count. This handles the race where `POST /api/auth/setup/admin` committed the user row but the DB went unavailable before `SetNeedsSetup(false)` was called — without this check `needsSetup` would remain `true` indefinitely, blocking all non-setup routes even though an admin exists. If `InitNeedsSetup()` fails, log ERROR and remain in `DBUnavailable`. Log INFO on success.
  - If the callback or `determineState()` call returns an error, log ERROR and remain in `DBUnavailable` — the probe will retry on the next successful ping.

Signature:
```go
func (mg *Migrator) StartDBProbe(ctx context.Context, pool *pgxpool.Pool, onRecovery func(ctx context.Context) error)
```

`onRecovery` is passed from `main.go` as the `initAppState` closure. This avoids a circular import: `migrator.go` does not need to know about `main.go`'s initialisation logic; `main.go` injects it as a callback.

**Middleware** — three sequential checks on every request:

```
1. if State() == DBUnavailable → 302 /db-error?from=<original path>
       bypass: /db-error, /health
2. if State() != Ready         → 302 /migrate
       bypass: /migrate, /api/migrate/*, /health
3. if NeedsSetup()             → 302 /setup
       bypass: /setup, /api/auth/setup/*, /health, /api/migrate/*
```

**`GET /db-error`** — if `State() != DBUnavailable`, redirects to `?from=` param (or `/`). Otherwise serves the static error page (auto-reloads itself every 5s via `setTimeout(() => location.reload(), 5000)`). When the auto-reload fires and the DB has recovered, the server-side redirect sends the user back to their original page.

The page displays a redacted connection string and a last-failed timestamp for debugging — useful in cloud environments where an operator needs to verify the DSN without leaking the password. Both values are injected **at serve time** (not registration time) by the `GET /db-error` handler:
- **Redacted DSN**: computed from the **resolved database URL** (the URL actually passed to `pgxpool.New`, not `cfg.DatabaseURL` which may be empty when the URL was constructed from individual `DB_*` env vars) via `net/url` once at `DBErrorHandler` construction time and stored as a string; the handler injects the pre-computed string into the HTML at each serve. Keep: scheme, user, host, port, database name, non-sensitive query params (e.g. `sslmode`). Redact: password → `***`; any query param whose name contains `password`, `secret`, or `key` → `***`.
- **Last-failed-at timestamp**: read from `migrator.LastUnavailableAt()` (returns `time.Time` stored as `atomic.Value`) at each serve, formatted as UTC. Updated by the probe each time it transitions into `DBUnavailable`.

Example display: `postgres://myuser:***@db.example.com:5432/nexorious?sslmode=require`

The handler uses `html/template` (or simple `strings.ReplaceAll` on placeholder tokens) to inject both values into the static HTML — no JavaScript fetch required.

**`AppState.String()` mapping:**

| `AppState` constant | `String()` return value | Used in |
|---|---|---|
| `AppStateDBUnavailable` | `"db_unavailable"` | `/health` body |
| `AppStateNeedsMigration` | `"needs_migration"` | `/health` body |
| `AppStateMigrating` | `"migrating"` | `/health` body |
| `AppStateReady` | `"ready"` | `/health` body |

**`GET /health`** — bypasses all three middleware gates (always reachable), always returns `200`:
- `200 {"status": "ok"}` when `Ready` (including when `needsSetup=true`)
- `200 {"status": "db_unavailable"}` when `DBUnavailable`
- `200 {"status": "needs_migration"}` when `NeedsMigration`
- `200 {"status": "migrating"}` when `Migrating`

Always `200` because the HTTP server can always serve meaningful content to the user in every state — the db-error page, the migration UI, the setup page, or the app itself. A non-`200` would cause a Kubernetes readiness probe to stop routing traffic (or a liveness probe to kill the process), preventing users from reaching the server-driven state pages. The `status` field carries state information for monitoring tools without affecting traffic routing.

`needsSetup` remains a bool on the Migrator (not a state machine state), checked inline in the `Ready` branch of the middleware and cleared once by the setup handler.

---

## Project Structure

```
nexorious-go/
├── cmd/
│   └── nexorious/
│       └── main.go              # Entry point — wires all components, starts server
├── internal/
│   ├── config/
│   │   └── config.go            # Config struct with caarlos0/env struct tags
│   ├── api/
│   │   ├── router.go            # Echo instance, middleware stack, route registration
│   │   ├── auth.go
│   │   ├── games.go
│   │   ├── user_games.go
│   │   ├── platforms.go
│   │   ├── sync.go
│   │   ├── jobs.go
│   │   ├── import.go
│   │   ├── export.go
│   │   ├── backup.go
│   │   ├── tags.go
│   │   └── status.go
│   ├── db/
│   │   ├── migrations/          # golang-migrate SQL files: 0001_initial.up.sql, etc.
│   │   ├── queries/             # sqlc input SQL: games.sql, user_games.sql, etc.
│   │   └── gen/                 # sqlc output (generated, committed, never hand-edited)
│   │       ├── db.go
│   │       ├── models.go
│   │       └── *.sql.go
│   ├── migrate/
│   │   ├── migrator.go          # State machine + golang-migrate wrapper
│   │   └── handler.go           # Echo handlers for /migrate and /api/migrate/*
│   ├── worker/
│   │   ├── pool.go              # Goroutine pool + buffered chan TaskFunc
│   │   └── tasks/
│   │       ├── sync.go          # DispatchSyncTask, ProcessSyncItemTask
│   │       ├── import.go        # ProcessImportItemTask
│   │       ├── export.go        # ExportTask
│   │       └── maintenance.go   # BackupCreateTask, MetadataRefreshDispatchTask,
│   │                            # MetadataRefreshProcessTask, CleanupJobResultsTask,
│   │                            # CleanupExportsTask, CleanupSessionsTask,
│   │                            # BackupScheduledTask
│   ├── scheduler/
│   │   └── scheduler.go         # gocron v2 job definitions
│   ├── services/
│   │   ├── igdb.go              # IGDB API client with rate limiter
│   │   ├── steam.go
│   │   ├── psn.go
│   │   ├── epic.go              # Shell out to legendary-gl (best effort)
│   │   ├── storage.go           # Cover art + logos local filesystem storage
│   │   ├── matching.go          # IGDB game lookup + title matching during sync/import
│   │   └── platform_resolution.go  # Raw platform name → Platform slug resolution
│   ├── auth/
│   │   └── jwt.go               # Token generation, validation, Echo middleware
│   ├── filter/
│   │   ├── builder.go           # filterBuilder: accumulates JOINs, WHERE, HAVING
│   │   └── handlers.go          # Criterion handlers for each filter type
│   └── ratelimit/
│       ├── limiter.go           # Interface: Acquire(ctx context.Context) error
│       ├── local.go             # golang.org/x/time/rate implementation
│       └── postgres.go          # PostgreSQL SELECT FOR UPDATE implementation
├── ui/                          # Go package + frontend source (mirrors Stash's ui/ layout)
│   ├── ui.go                    # //go:embed dist AND //go:embed migrate — two separate embed.FS vars
│   ├── migrate/
│   │   └── migrate.html         # Standalone migration UI — Go template, SSE progress, no bundler
│   ├── dist/                    # Built React SPA (gitignored, populated by `make frontend`)
│   ├── src/                     # React source (from nexorious/frontend/src)
│   ├── package.json
│   ├── vite.config.ts
│   └── ...                      # remainder of React project files
├── sqlc.yaml
├── Makefile                     # frontend build → sqlc generate → go build
├── go.mod
└── go.sum
```

`internal/db/gen/` is committed to the repository. `sqlc generate` only needs to be run when query files change, not on every build.

`ui/dist/` is gitignored; it is populated by `make frontend` before `go build`. `ui/ui.go` follows the Stash pattern exactly:

```go
package ui

import "embed"

//go:embed dist
var UIBox embed.FS      // main React SPA

//go:embed migrate
var MigrateBox embed.FS // standalone migration UI (Go template)
```

---

## HTTP Layer

### Route Zones

**Migration zone** — always available, bypasses app-state middleware:
```
GET  /migrate                    Standalone migration UI (web/migrate.html)
GET  /api/migrate/status         Pending count, current version, app state
POST /api/migrate/run            Trigger migration async
GET  /api/migrate/progress       SSE stream: live log lines + completion event
```

**Setup zone** — requires `Ready` state (migrations must have run); bypasses JWT (no users exist yet during first-run); all other routes redirect to `/setup` while `needsSetup` is true:
```
POST /api/auth/setup/admin       Create initial admin; 403 if any user exists
POST /api/auth/setup/restore     Restore from .tar.gz backup archive during setup; 403 if any user exists
                                 (deferred to Phase 3 — implement alongside backup/restore system)
```

> **Note:** `GET /api/auth/setup/status` is dropped. The server enforces the setup gate via middleware redirect, so the frontend does not need to poll for setup state. The `/setup` React route renders an unconditional setup form — if the user somehow reaches it after setup is complete, `POST /api/auth/setup/admin` returns 403 and the frontend redirects to `/login`.

**API zone** — gated by app-state middleware, then JWT where required:
```
POST /api/auth/login
POST /api/auth/refresh
POST /api/auth/logout            Invalidates server-side UserSession row

GET  /api/auth/me                Current user profile; also used by frontend as token-validity check on load
PUT  /api/auth/me                Update user preferences

PUT  /api/auth/change-password   Invalidates all sessions on success; user must re-login
GET  /api/auth/username/check/:username   Returns {available: bool}; no side effects
PUT  /api/auth/username

GET  /api/games                          Query params: page, per_page, q (ILIKE title/description), genre, developer, publisher, release_year, sort_by, sort_order
GET  /api/games/:id
POST /api/games/search/igdb
GET  /api/games/igdb/:igdb_id            Returns IGDBSearchResponse (same shape as search), not GameResponse
POST /api/games/igdb-import
GET  /api/games/:id/metadata/status
POST /api/games/:id/metadata/refresh
POST /api/games/:id/metadata/populate
POST /api/games/metadata/bulk            operation field: "refresh" | "populate" | "cover_art"
POST /api/games/:id/cover-art/download
POST /api/games/cover-art/bulk-download
POST /api/games/metadata/refresh-job

GET    /api/user-games
GET    /api/user-games/ids
GET    /api/user-games/genres
GET    /api/user-games/filter-options
GET    /api/user-games/stats
GET    /api/user-games/:id
POST   /api/user-games
PUT    /api/user-games/bulk-update
DELETE /api/user-games/bulk-delete
POST   /api/user-games/bulk-add-platforms
DELETE /api/user-games/bulk-remove-platforms
PUT    /api/user-games/:id
PUT    /api/user-games/:id/progress
DELETE /api/user-games/:id
GET    /api/user-games/:id/platforms
POST   /api/user-games/:id/platforms
PUT    /api/user-games/:id/platforms/:platform_id
DELETE /api/user-games/:id/platforms/:platform_id

GET  /api/platforms                              List platforms (JWT required)
GET  /api/platforms/simple-list                  Slug-only list for dropdowns (JWT required)
GET  /api/platforms/:platform                    Get single platform (JWT required)
GET  /api/platforms/:platform/storefronts        List storefronts for a platform (JWT required)
GET  /api/platforms/:platform/default-storefront Get default storefront mapping (JWT required)
GET  /api/platforms/storefronts/                 List storefronts (JWT required)
GET  /api/platforms/storefronts/simple-list      Slug-only list for dropdowns (JWT required)
GET  /api/platforms/storefronts/:storefront      Get single storefront (JWT required)

GET    /api/tags
POST   /api/tags
PUT    /api/tags/:id
DELETE /api/tags/:id
POST   /api/tags/assign/:user_game_id    Assign tag set to a game (replaces current tags for that game)
DELETE /api/tags/remove/:user_game_id    Remove specific tags from a game
POST   /api/tags/bulk-assign             Assign tags to multiple games at once
DELETE /api/tags/bulk-remove             Remove tags from multiple games at once

GET  /api/jobs
GET  /api/jobs/summary
GET  /api/jobs/pending-review-count
GET  /api/jobs/active/:job_type
GET  /api/jobs/recent/:source
GET  /api/jobs/:id
GET  /api/jobs/:id/items     List job items for a job (pagination via query params)
POST /api/jobs/:id/cancel
DELETE /api/jobs/:id
POST /api/jobs/:id/retry-failed

GET  /api/job-items/:id
POST /api/job-items/:id/resolve   Resolve item to an IGDB game (review workflow)
POST /api/job-items/:id/skip      Skip item during review
POST /api/job-items/:id/retry

POST /api/sync/:platform          Trigger manual sync (GOG returns 501 Not Implemented)
GET  /api/sync/:platform/status   Current sync status
POST /api/sync/steam/verify       Verify Steam credentials
DELETE /api/sync/steam/connection
POST /api/sync/epic/auth/start
POST /api/sync/epic/auth/complete
GET  /api/sync/epic/auth/check
DELETE /api/sync/epic/connection
POST /api/sync/psn/configure
GET  /api/sync/psn/status
DELETE /api/sync/psn/disconnect
GET  /api/sync/config             All sync configs for current user
GET  /api/sync/config/:platform
PUT  /api/sync/config/:platform
GET  /api/sync/ignored            List skipped external games (is_skipped=true in external_games)
DELETE /api/sync/ignored/:id      Un-skip (clears is_skipped flag on external_games row)

GET  /api/status              (public — no JWT required)
GET  /health

POST /api/import/nexorious   Upload and process nexorious JSON export file
POST /api/export/json        Start JSON export job
POST /api/export/csv         Start CSV export job
GET  /api/export/:id/download
```

> **Import scope note:** The Go port implements only the nexorious JSON import (`POST /api/import/nexorious`). The Python codebase has additional dead routes under `/api/import/` (`/sources`, `/jobs`, `/history`) that were part of a multi-source import system (Darkadia, one-shot Steam import, etc.) that was never completed. No frontend UI calls any of them. They are not carried forward.

**Admin zone** — gated by app-state middleware + JWT + admin role check:
```
POST   /api/auth/admin/users                       Create user account
GET    /api/auth/admin/users                       List all users
GET    /api/auth/admin/users/:id                   Get user
PUT    /api/auth/admin/users/:id                   Update user (role, enabled state)
PUT    /api/auth/admin/users/:id/password          Reset user password
GET    /api/auth/admin/users/:id/deletion-impact   Preview what will be cascade-deleted
DELETE /api/auth/admin/users/:id                   Delete user and all associated data

GET    /api/admin/backups/config     Get backup schedule configuration
PUT    /api/admin/backups/config     Update backup schedule configuration; rebuilds gocron backup job
GET    /api/admin/backups            List available backups
POST   /api/admin/backups            Create manual backup
DELETE /api/admin/backups/:id        Delete a backup
GET    /api/admin/backups/:id/download   Download backup archive
POST   /api/admin/backups/:id/restore   Restore from a stored backup — drops all in-flight requests (see Restore Behaviour below)
POST   /api/admin/backups/restore/upload   Upload and restore an external .tar.gz archive — same

GET    /api/platforms/stats                        Platform usage statistics

GET    /api/platforms/storefronts/stats            Storefront usage statistics
```

**Static files zone** — served directly by the Go server:
```
/static/cover_art/*     Local cover art files from StoragePath/cover_art/ (on-disk, NOT embedded)
```

Platform/storefront logo files live in `ui/public/logos/` and are served as part of the frontend SPA — no separate route needed. The `icon` column in `platforms` and `storefronts` stores only the filename (e.g. `steam.svg`); the frontend constructs the full path.

**Frontend zone** — catch-all, gated by app-state middleware:
- Serves `ui.UIBox` (`ui/dist/`) via `embed.FS`
- Paths not matching a file fall back to `index.html` (required for TanStack Router)

### Middleware Stack (outermost → innermost)

1. `recover` — panic recovery
2. `logger` — request logging
3. `app-state` — two sequential checks:
   - Migration check: redirect to `/migrate` unless state is `Ready` or path is `/migrate*`
   - Setup check: redirect to `/setup` unless `needsSetup` is false or path is in the setup bypass list (`/setup`, `/api/auth/setup/*`, `/health`, `/api/migrate/*`)
4. `cors` — same origins as Python version; in production both API and frontend are same-origin so CORS is only needed in development
5. `jwt` — on protected route groups only

### Frontend Configuration

The React source lives in `ui/` and is built there (`cd ui && npm run build` → `ui/dist/`).

The frontend uses Vite and reads env vars with the `VITE_` prefix. The relevant vars are defined in `ui/src/lib/env.ts`.

```ts
export const config = {
  apiUrl: import.meta.env.VITE_API_URL ?? '/api',
  staticUrl: import.meta.env.VITE_STATIC_URL ?? '',
  appName: import.meta.env.VITE_APP_NAME ?? 'Nexorious',
  appVersion: import.meta.env.VITE_APP_VERSION ?? '1.0.0',
  isDevelopment: import.meta.env.DEV,
  isProduction: import.meta.env.PROD,
}
```

**Production build (embedded in Go binary):** No `.env` file is needed. The defaults (`/api` and empty string) produce same-origin URLs that work correctly when the Go server serves both the SPA and the API.

**Development (npm run dev):** Vite's dev server runs on port 3000 and proxies `/api` and `/static` to the Go backend on port 8000 — matching the Python dev setup exactly. No `.env` file is required for local dev either; the defaults and proxy config in `vite.config.ts` handle it. The Go backend must be running separately (`go run ./cmd/nexorious`) for the proxy to resolve.

`VITE_STATIC_URL` must remain empty string for the embedded production build. Cover art is served from `/static/cover_art/` on the same origin (see Static Files Zone above).

---

## Database Layer

### Migrations (golang-migrate)

SQL migration files live in `internal/db/migrations/` and are embedded in the binary via `embed.FS`. The first migration (`0001_initial.up.sql`) is a clean schema derived from the current Python models — no Alembic history is ported. The migrator wraps `golang-migrate` and drives the app state machine.

Existing users migrate their data using the nexorious JSON export from the Python version, imported via `POST /api/import` (nexorious format handler).

### Static vs. Dynamic Queries: Hybrid Approach

The database layer uses two complementary approaches, following the pattern used by [Stash](https://github.com/stashapp/stash):

**sqlc** (static queries): Used for all CRUD operations where the query shape is fixed — auth, games fetch-by-id, jobs, scheduler tasks, external_games upsert, etc. Generated code is committed. Run `sqlc generate` only when query files change.

**sqlx + goqu** (dynamic filter queries): Used for list endpoints with optional multi-value filters and conditional JOINs. These queries cannot be expressed statically. `sqlx` handles query execution and struct scanning; `goqu` builds the SQL programmatically.

The `internal/filter/` package provides a Stash-style `filterBuilder`:

```go
// builder.go
type filterBuilder struct {
    joins         joins         // accumulated JOINs, deduplicated
    whereClauses  []sqlClause   // ANDed WHERE conditions
    havingClauses []sqlClause   // ANDed HAVING conditions
}

// Add only if criterion is non-nil; join deduplication prevents bloat
func (f *filterBuilder) addJoin(j join) { f.joins.addUnique(j) }
func (f *filterBuilder) addWhere(clause string, args ...any) { ... }
```

Each filter criterion gets its own handler (a small function that adds JOINs/WHERE/HAVING only if the criterion is non-nil). The `user_games` list handler composes ~12 such handlers. This gives full control over generated SQL without ORM overhead and avoids the N-variant sqlc anti-pattern.

**Why not GORM?** GORM adds a heavy ORM layer, trades type-safety for flexibility, and generates SQL that is harder to reason about. The Stash project (our reference implementation) deliberately chose custom filter building over GORM for these reasons.

### Schema: Key ID Types

- `games.id`: `INT` — the IGDB ID is used directly as the primary key
- `users.id`, `user_games.id`, `user_game_platforms.id`, `jobs.id`, `job_items.id`, `external_games.id`, `user_sync_configs.id`, `tags.id`, `user_game_tags.id`: `TEXT` (UUID v4 string)
- `platforms.name`, `storefronts.name`: `TEXT` — slug used as primary key (e.g. `"steam"`, `"pc-windows"`)

Echo route params that bind to UUID IDs are treated as `string` in handlers. Game IDs bind as `int`.

### sqlc Workflow

`sqlc.yaml` points at `internal/db/migrations/` for schema and `internal/db/queries/` for query definitions. Query files use sqlc annotations:

```sql
-- name: ListUserGames :many
SELECT g.*, ug.play_status, ug.personal_rating
FROM user_games ug
JOIN games g ON g.id = ug.game_id
WHERE ug.user_id = $1
ORDER BY g.title
LIMIT $2 OFFSET $3;
```

Running `sqlc generate` produces fully typed Go functions in `internal/db/gen/`. The generated code is committed so contributors do not need sqlc installed to build.

### Connection Pool

A single `*pgxpool.Pool` created at startup is injected into handlers, workers, and the scheduler via `*db.Queries` (sqlc's query runner). Handlers that need transactions call `pool.BeginTx()` and construct a transaction-scoped `*db.Queries` from the `pgx.Tx`.

Dynamic-query handlers receive the `*pgxpool.Pool` directly and use `sqlx.NewDb(stdlib.OpenDBFromPool(pool), "postgres")` to create an `*sqlx.DB` for goqu execution.

**sqlx/pgx bridge note:** `stdlib.OpenDBFromPool` wraps the pgx pool in a `database/sql` adapter so that `sqlx` (which requires `database/sql`) can share the same physical connections. The same underlying connections are used by both paths — there is no second pool. The practical gotchas to be aware of:
- pgx-native error types (e.g. `*pgconn.PgError`) surface through the `database/sql` path as wrapped errors; use `errors.As` rather than type-asserting directly
- `database/sql` and pgx account for idle connections slightly differently — monitor pool metrics (pgxpool exposes `Stat()`) if connection exhaustion is suspected
- The bridge is a well-established pattern (used by Stash and others); it is documented here so future contributors understand why both `pgx` and `sqlx` imports coexist

### Sort Field Note

The Python version has a known issue where sort fields requiring a JOIN must be kept in sync between backend and frontend manually. In the Go version this is addressed by defining explicit named queries per sort variant in sqlc, making sort field mismatches a compile-time rather than runtime problem. Dynamic sort fields in the user_games list (which uses the filterBuilder) are validated against an allowlist at handler entry.

### Schema: Full Table List

The initial migration must create all of the following tables (derived from Python models):

| Table | Notes |
|---|---|
| `users` | UUID PK; `username`, `password_hash`; `is_active`, `is_admin`; `preferences` (JSON text) |
| `user_sessions` | UUID PK; `token_hash`, `refresh_token_hash`, `user_agent`, `ip_address` |
| `games` | INT PK (IGDB ID); `title`, `description`, `genre`, `developer`, `publisher`, `release_date`; `cover_art_url`; `rating_average` (NUMERIC 5,2), `rating_count`; `estimated_playtime_hours`; `howlongtobeat_main`, `howlongtobeat_extra`, `howlongtobeat_completionist` (hours, converted from IGDB seconds); `igdb_slug`, `igdb_platform_ids` (JSON array), `igdb_platform_names` (JSON array); `game_modes`, `themes`, `player_perspectives` (comma-separated strings); `game_metadata` (JSON text); `last_updated` (IGDB metadata refresh timestamp); `created_at` (TIMESTAMPTZ, NOT NULL DEFAULT now() — when the game was first inserted into this database; passed explicitly on upsert to preserve the value across metadata refreshes, which only update `last_updated`). **HowLongToBeat field mapping:** IGDB's `game_time_to_beats` endpoint returns fields named `hastily`, `normally`, `completely` (in seconds). These map to `howlongtobeat_main`, `howlongtobeat_extra`, `howlongtobeat_completionist` respectively (converted to hours). This mapping is non-obvious and must be replicated exactly from the Python `map_igdb_time_to_beat_to_db_fields()` function. |
| `user_games` | UUID PK; `play_status`, `personal_rating`, `is_loved`, `hours_played`, `personal_notes`; UNIQUE(user_id, game_id) |
| `user_game_platforms` | UUID PK; `platform`, `storefront`; `store_game_id`, `store_url`; `is_available`, `hours_played`, `ownership_status`, `acquired_date`; `original_platform_name`, `original_storefront_name`; `external_game_id`, `sync_from_source`; UNIQUE(user_game_id, platform, storefront) |
| `platforms` | TEXT PK (slug); `display_name`, `icon` (filename only, e.g. `steam.svg` — frontend builds the full path), `igdb_platform_id` (nullable INT — IGDB's numeric platform identifier, for linking platform records to IGDB data), `default_storefront` (FK → storefronts.name, nullable); data inserted by migration |
| `storefronts` | TEXT PK (slug); `display_name`, `icon` (filename only — frontend builds the full path), `base_url`; data inserted by migration |
| `platform_storefronts` | Composite PK (`platform` TEXT FK, `storefront` TEXT FK); many-to-many join table between platforms and storefronts |
| `tags` | UUID PK; per-user |
| `user_game_tags` | UUID PK; join table for user_games ↔ tags |
| `external_games` | UUID PK; UNIQUE(user_id, storefront, external_id); stores IGDB resolution cache; `is_skipped` flag replaces the old `ignored_external_games` table |
| `user_sync_configs` | UUID PK; UNIQUE(user_id, platform); stores credentials as JSON text |
| `jobs` | UUID PK |
| `job_items` | UUID PK; FK → jobs |
| `pending_tasks` | UUID PK; DB-backed task queue; `task_type`, `payload` (JSONB), `priority`, `status`, `attempts`, `last_error`, `claimed_at`, `done_at` |
| `backup_config` | **Singleton** (INT PK, always id=1); admin-only. `schedule_cron` (TEXT — standard 5-field cron expression, e.g. `"0 2 * * *"` for daily at 2 AM UTC; empty string means disabled/manual-only), `retention_mode` (enum: `days`\|`count`), `retention_value` (integer). **Note:** the Python version stored schedule as three separate fields (`schedule` enum, `schedule_time` string, `schedule_day` integer). The Go port consolidates these into a single cron expression. The backup admin UI must be updated accordingly — the old schedule dropdowns are replaced by a single cron input field. |
| `rate_limiter_tokens` | TEXT PK; used by postgres rate limiter backend |

#### Platforms and Storefronts

`platforms` and `storefronts` use a TEXT slug as their primary key (e.g. `"pc-windows"`, `"steam"`). The `platform_storefronts` join table records which storefronts are valid for a given platform (many-to-many). `platforms.default_storefront` is a nullable FK into `storefronts` for the most common storefront for that platform.

Both tables are **static reference data** — they are populated entirely by migration `INSERT` statements and are never modified at runtime. There is no admin API for managing them and no seed mechanism. To add a new platform or storefront, add a migration. To retire one (rare), write a migration that migrates any affected `user_game_platforms` rows first, then deletes the entry. All users are read-only consumers of this data.

**Frontend change required:** The Python frontend includes admin UI for managing platforms and storefronts (creating, editing, deleting entries, uploading logos, managing associations). These screens must be **removed entirely** when porting the frontend. Any navigation links, routes, API client methods (`src/api/platforms.ts` mutations, logo upload calls), and TypeScript types relating to platform/storefront write operations should be deleted. The read-only API calls (listing platforms, listing storefronts for a platform, fetching a single platform/storefront for display in dropdowns) are kept.

#### External Games

`external_games` is load-bearing for the sync system. Each row represents a game seen from an external source (Steam, PSN, Epic) for a given user. It stores:
- The raw `external_id` and `title` from the platform
- `resolved_igdb_id` — set once after IGDB matching; never re-computed on subsequent syncs
- `is_skipped` — user-controlled flag; when true the game is excluded from future syncs
- Current source state: `is_available`, `is_subscription`, `playtime_hours`, `ownership_status`

`UserGamePlatform` rows reference `external_games.id` via `external_game_id` to link collection entries back to their sync source.

The Python codebase previously had a separate `ignored_external_games` table. An Alembic migration (Mar 2026) migrated all ignored-game data to `external_games.is_skipped = true` and dropped the old table. **The Go port does not include an `ignored_external_games` table.** Skip/un-skip functionality is exposed via the sync router (`GET /api/sync/ignored`, `DELETE /api/sync/ignored/:id`), which reads/writes `external_games.is_skipped`.

The Python `Wishlist` model is also **not included** in the Go schema. No user-facing API endpoints for wishlists exist, the frontend calls no wishlist API, and the table is not brought forward. The Go schema starts clean without it.

#### User Sync Configs

`user_sync_configs` stores per-user, per-platform sync settings:
- `platform` (slug: `"steam"`, `"psn"`, `"epic"`)
- `frequency` (enum: `manual` | `hourly` | `daily` | `weekly`)
- `auto_add` — if true, matched games are added automatically; otherwise queued for review
- `platform_credentials` — JSON text; for Steam: API key; for Epic: legendary user.json content
- `last_synced_at`

The sync config API (`GET/PUT /api/sync/config/:platform`) lets users configure these settings and supply credentials. Credentials are stored encrypted at rest (AES-GCM, key derived from `SECRET_KEY`).

**`is_configured` field:** The `SyncConfigResponse` includes an `is_configured` boolean indicating whether the user has working credentials stored for that platform. In the Go port this is determined by checking whether `user_sync_configs.platform_credentials` is non-null and non-empty for the row (after decryption). In the Python version, credentials were stored in `users.preferences` rather than `user_sync_configs`, so the Python `_is_platform_configured()` logic reads from a different location. The Go implementation reads from `user_sync_configs.platform_credentials` exclusively.

#### Backup Config

`backup_config` is a **singleton table** — exactly one row always exists (id=1, created by the initial migration with default values). It is not per-user. Backup configuration and backup management are admin-only features. The scheduler reads `backup_config` to determine whether and when to run automatic backups.

The `schedule_cron` field stores a standard 5-field cron expression (e.g. `"0 2 * * *"` for daily at 2 AM UTC). An empty string means backups are disabled (manual-only). The gocron scheduler rebuilds the backup job whenever the config is updated via `PUT /api/admin/backups/config`. This is simpler than the Python version's enum-based `schedule`/`schedule_time`/`schedule_day` approach and gives administrators full scheduling flexibility without requiring code changes for unusual schedules.

**Frontend change required:** The backup admin UI (`/admin/backups`) must be updated to replace the `schedule`/`schedule_time`/`schedule_day` dropdowns with a single cron expression text input. The frontend API client (`src/api/backup.ts`) and types (`src/types/backup.ts`) must likewise be updated to use `schedule_cron` instead of the three-field model. This is an explicit in-scope frontend change for the Go port.

---

## Fuzzy Search

The Python backend has fuzzy matching in three contexts. Two are carried forward; one is dropped.

### Context 1: Database list search — DROPPED

The Python backend accepts a `fuzzy_threshold` query parameter on both `GET /api/games` and `GET /api/user-games`. These are two distinct endpoints with different purposes: `GET /api/games` queries the **global game catalog** (IGDB records cached in the local DB) and is the mechanism by which users add games to their collection — it is JWT-protected but not admin-only. `GET /api/user-games` queries the **user's personal collection** and is the endpoint the frontend uses for game browsing. Despite serving different purposes, neither endpoint's `fuzzy_threshold` is ever sent by the frontend: `fuzzyThreshold` exists in `UserGameListParams` in the API client but no UI component populates it. The feature was built in both endpoints but never surfaced in any UI. Despite serving different purposes, neither endpoint's `fuzzy_threshold` is ever sent by the frontend: `fuzzyThreshold` exists in `UserGameListParams` in the API client but no UI component populates it. The feature was built in both endpoints but never surfaced in any UI.

The Go port uses `ILIKE` for text search on both list endpoints. The `fuzzy_threshold` parameter is not implemented on either. `pg_trgm` is not needed and is not enabled.

### Context 2: IGDB search result ranking (`POST /api/games/search/igdb`)

The IGDB search endpoint calls the IGDB API and receives a candidate list. It then **post-ranks those results in-process** using the fuzzy algorithm — `pg_trgm` cannot help here because the data comes from an external HTTP response, not our database.

**Approach: Go port of the full IGDB search pipeline**

The Python implementation (`backend/app/services/igdb/search.py`) is more complex than a simple post-rank step. The Go port must replicate the full pipeline:

1. **Keyword detection** — Scan the query for known keywords and patterns using an `KEYWORD_EXPANSIONS` table equivalent. Currently handled patterns:
   - `"goty"` → `"Game of the Year"`
   - `"The Telltale Series"` → `""` (removed)
   - `"®"` → `""` (removed)
   - `"(classic)"` → `""` (removed, case-insensitive)
   - `":"` → `" "` (replaced with space)
   - Year-in-parentheses pattern, e.g. `(2023)` → `""` (removed)
   - Standalone `"1"` pattern (excluding `Episode 1`, `Chapter 1`, etc.) → `""` (removed)

2. **Query expansion** — For each detected keyword, generate a variant query with that keyword transformed. If multiple keywords are detected, also generate a fully-transformed variant. This produces a list of candidate queries alongside the original.

3. **Concurrent IGDB calls for the original query** — Run two IGDB queries concurrently for the normalised original query (matching the Python `asyncio.gather` behaviour):
   - A fuzzy/prefix search (IGDB's standard search)
   - An exact-name search (`where name = "..."`)
   Exact-name results are merged first (highest priority), then fuzzy results, deduplicated by IGDB ID.

   Both calls go through `Limiter.Acquire` before executing. With the default burst capacity of 8 at 4 req/s there is ample headroom for two simultaneous calls; running them concurrently matches the Python implementation and avoids unnecessary latency on the common path.

4. **Expanded-query searches** — For each expanded query variant, run additional IGDB searches **sequentially**. Results are merged into the combined list (original/exact results take priority), deduplicated by IGDB ID.

   Expanded queries are serialized (unlike the Python version which uses a second `asyncio.gather`) because the number of expanded queries is variable. Serializing them keeps rate-limiter accounting straightforward without requiring burst-capacity estimation. The concurrency in step 3 covers the latency-sensitive common case (no keyword expansion).

5. **Post-ranking** — The merged candidate list is ranked in-process using `FuzzyConfidence` and filtered at threshold 0.6.

The `services/igdb.go` package implements this pipeline. The `services/matching.go` package exposes the shared scoring primitives:

```go
// NormalizeTitle applies the following transformations in order, matching
// backend/app/utils/normalize_title.py exactly:
//  1. Expand GOTY → "Game of the Year" (case-insensitive)
//  2. Remove trademark symbols (™, ®)
//  3. Remove apostrophes (straight ' and curly ' ')
//  4. Remove colons (:)
//  5. Remove standalone dashes ( - ) but preserve in-word hyphens (e.g. Spider-Man)
//  6. Remove year in parentheses, e.g. (2023)
//  7. Collapse whitespace
//  8. Lowercase and trim
// Result is used only for comparison — never stored or displayed.
func NormalizeTitle(s string) string

// FuzzyConfidence returns a 0.0–1.0 score using the same multi-metric
// weighted approach as the Python version (rapidfuzz-equivalent scoring).
// Uses go-fuzzywuzzy: Ratio, PartialRatio, TokenSortRatio, TokenSetRatio.
// Weighted max: exact*1.0, ratio*0.9, partial*0.8, token_sort*0.7, token_set*0.6
func FuzzyConfidence(query, title string) float64
```

**Library compatibility — scoring divergence:** The Python implementation uses `rapidfuzz`; `go-fuzzywuzzy` is based on the original `fuzzywuzzy` and produces different scores for identical inputs (rapidfuzz uses optimised algorithms with different edge-case handling). In practice this means the 0.85 auto-match threshold may behave slightly differently: games that Python auto-matched could land in `PENDING_REVIEW` in Go, or vice versa. The thresholds are heuristics, not exact values, so some drift is acceptable — but after initial deployment, compare auto-match rates against the Python baseline and retune thresholds if needed. Do not re-raise this as a concern during implementation; it is a known, accepted trade-off.

### Context 3: Sync/import IGDB matching (background jobs)

When a new `ExternalGame` has no `resolved_igdb_id`, the matching service searches IGDB by title and uses `FuzzyConfidence` to decide:
- Score ≥ 0.85 → auto-match (sets `resolved_igdb_id`, game added to collection if `auto_add = true`)
- Score < 0.85 but > 0 → `PENDING_REVIEW` (user selects the correct match via job-items UI)
- No candidates → `NO_MATCH`

This uses the same `FuzzyConfidence` function as Context 2. Minor score differences introduced by using `go-fuzzywuzzy` instead of `rapidfuzz` are accepted.

---

## Browser Migration UI

Follows Stash's pattern for its `ui/login/login.html`: a standalone page embedded separately from the main SPA, served directly by Go as a **Go template**, with no JavaScript bundler or external dependencies.

`ui/migrate/migrate.html` is rendered by `migrate/handler.go` using `html/template`, with template variables for pending migration count and current schema version. The page itself handles progress via a small amount of vanilla JS:

1. Displays pending migration count and current version (from template variables)
2. Presents a "Run Migrations" button
3. On click, `POST /api/migrate/run` then opens an SSE connection to `/api/migrate/progress`
4. Streams log lines into a scrollable terminal-style div as migrations execute
5. On receiving the completion SSE event, redirects to `/`

Inline styles only. The React app (`ui/dist/`) is never loaded until migrations succeed, ensuring it never runs against a stale schema.

---

## Worker System

### Design: Database-Backed Task Queue

Rather than an in-memory channel queue, tasks are persisted as rows in a `pending_tasks` table. This gives the same durability guarantees as the Python/NATS JetStream design: tasks survive process restarts, are never silently dropped under load, and the design naturally supports horizontal scaling later (multiple instances poll the same table).

```sql
CREATE TABLE pending_tasks (
    id          TEXT PRIMARY KEY,          -- UUID v4
    task_type   TEXT NOT NULL,             -- e.g. "sync", "import_item", "export", "metadata_refresh"
    payload     JSONB NOT NULL DEFAULT '{}',
    priority    INTEGER NOT NULL DEFAULT 0, -- higher = more urgent
    status      TEXT NOT NULL DEFAULT 'pending', -- pending | running | done | failed
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at  TIMESTAMPTZ,               -- set when a worker picks it up
    done_at     TIMESTAMPTZ
);
CREATE INDEX pending_tasks_claim_idx ON pending_tasks (status, priority DESC, created_at)
    WHERE status = 'pending';
```

**Index coverage note:** `pending_tasks_claim_idx` is a partial index filtered to `status = 'pending'`. It covers the worker claim query efficiently. Rows in `running`, `done`, or `failed` states fall outside the partial index; monitoring or retry queries filtering on those statuses will do a full table scan. For typical workloads this is acceptable — `pending` is the hot path. Add a separate unfiltered index on `status` if operational queries against non-pending rows become slow.

Workers claim tasks using `SELECT ... FOR UPDATE SKIP LOCKED`, which is PostgreSQL's idiomatic mechanism for concurrent queue consumers — rows locked by one worker are transparently skipped by others:

```sql
UPDATE pending_tasks
SET status = 'running', claimed_at = now(), attempts = attempts + 1
WHERE id = (
    SELECT id FROM pending_tasks
    WHERE status = 'pending'
    ORDER BY priority DESC, created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;
```

Workers poll the table on a short interval (default 1s) and are additionally woken immediately by an in-process `chan struct{}` notification that API handlers send after inserting a new task — so latency for user-triggered tasks is effectively zero.

### Pool

```go
type Pool struct {
    db      *pgxpool.Pool
    notify  chan struct{}   // capacity 1; sending is non-blocking (drop if already pending)
    wg      sync.WaitGroup
}

func (p *Pool) Submit(ctx context.Context, taskType string, payload any, priority int) error
    // Inserts a pending_tasks row; sends on notify channel; returns DB error only

func (p *Pool) Start(ctx context.Context, workers int)
    // Starts N goroutines; each polls DB, claims a task, executes it, marks done/failed

func (p *Pool) Shutdown()
    // Cancels ctx, waits for in-flight tasks to complete
```

`Submit` never blocks and never drops tasks — if the DB write succeeds the task is durable. The only failure mode is a DB error, which is returned to the caller (HTTP handler returns 503; scheduler logs and retries at next tick).

Failed tasks (handler returned error) are marked `status = 'failed'` with `last_error` populated. The existing `/api/jobs/:id/retry-failed` and `/api/job-items/:id/retry` endpoints re-insert failed tasks as new `pending_tasks` rows.

### Task Domains

| File | Tasks |
|---|---|
| `tasks/sync.go` | `DispatchSyncTask`, `ProcessSyncItemTask` |
| `tasks/import.go` | `ProcessImportItemTask` |
| `tasks/export.go` | `ExportTask` |
| `tasks/maintenance.go` | `BackupCreateTask`, `BackupScheduledTask`, `MetadataRefreshDispatchTask`, `MetadataRefreshProcessTask` |

Cleanup tasks (`CleanupJobResultsTask`, `CleanupExportsTask`, `CleanupSessionsTask`) run inline in the gocron goroutine — they are fast DB operations with no external I/O and do not go through the task queue.

### Linking pending_tasks to jobs

Most tasks that go through the worker pool are associated with a user-visible `jobs` row. The link is carried in the **task payload**, not as a DB foreign key on `pending_tasks`. When a task is submitted, the payload JSON includes a `job_id` field (UUID). The worker function receives the payload, extracts `job_id`, and uses it to update the `jobs` and `job_items` tables directly throughout execution.

Example payload for a sync dispatch task:
```json
{ "job_id": "...", "user_id": "...", "source": "steam" }
```

This mirrors the Python/NATS pattern exactly — in the Python version, `job_id` is passed as a positional argument to each `@broker.task` function. In the Go version it is a field in the JSONB payload.

**Maintenance tasks** (backup, metadata refresh dispatch) also receive a `job_id` in their payload. **Cleanup tasks** (session cleanup, export cleanup, job results cleanup) run inline in the gocron goroutine and do not create `jobs` rows — they are fire-and-forget housekeeping with no user-visible tracking.

`Submit` callers (HTTP handlers and the scheduler) are responsible for creating the `jobs` row *before* calling `Submit`, so that the task can immediately update job status when it starts. The typical call sequence is:

1. Handler creates a `jobs` row (`status = pending`)
2. Handler calls `pool.Submit(ctx, taskType, payload{job_id: ...}, priority)`
3. Worker claims the task, sets `jobs.status = running`, proceeds
4. Worker marks `jobs.status = completed` (or `failed`) on exit

### Job Progress

Job state (`pending`, `running`, `completed`, `failed`, `cancelled`) is persisted in the `jobs` table via sqlc queries. Individual item progress is tracked in the `job_items` table. Workers write progress updates during execution. The `/api/jobs` and `/api/job-items` endpoints read directly from the tables — no in-memory state.

### Horizontal Scaling Note

The `SELECT FOR UPDATE SKIP LOCKED` claim pattern is safe for multiple concurrent workers across multiple instances. When multi-instance deployments are needed, setting `WORKER_COUNT` appropriately per instance and pointing all instances at the same PostgreSQL is sufficient — no additional coordination is required.

---

## Scheduler

`gocron` v2 jobs start only after the app transitions to `Ready`.

| Job | Schedule | Notes |
|---|---|---|
| Cleanup job results | Daily at 3:00 AM UTC | Inline in gocron goroutine (fast DB op) |
| Cleanup exports | Daily at 4:00 AM UTC | Inline in gocron goroutine (fast DB op) |
| Cleanup sessions | Every 30 minutes | Inline in gocron goroutine (fast DB op) |
| Scheduled backup | Cron expression from `backup_config.schedule_cron` (default `"0 2 * * *"`; empty string = disabled) | Submits `BackupScheduledTask` to worker pool |
| Metadata refresh dispatch | Configurable interval (`METADATA_REFRESH_INTERVAL`, default 24h) | Submits `MetadataRefreshDispatchTask` to pool |
| Check pending syncs | Every 15 minutes | Submits `DispatchSyncTask` to pool |

Jobs that generate significant work (metadata refresh dispatch, sync check, scheduled backup) submit tasks to the worker pool rather than executing inline. Cleanup jobs (job results, exports, sessions) run inline in the gocron goroutine — they are fast single-query operations with no external I/O.

---

## IGDB Authentication (Twitch OAuth)

IGDB uses Twitch's OAuth2 client credentials flow. The Go port implements the same auto-refresh pattern as the Python version (`backend/app/services/igdb/auth.py`).

### Token lifecycle

1. On first IGDB request, `IGDBAuthManager.GetAccessToken()` checks the in-memory cached token
2. If no token, or the cached token expires within 5 minutes, it requests a new one from `https://id.twitch.tv/oauth2/token` using `IGDB_CLIENT_ID` + `IGDB_CLIENT_SECRET` (client credentials grant)
3. The response includes `access_token` and `expires_in` (seconds). The manager stores both and computes an absolute expiry time
4. Subsequent calls return the cached token until it nears expiry

### Pre-configured token (`IGDB_ACCESS_TOKEN`)

If `IGDB_ACCESS_TOKEN` is set in the environment, it is used as the initial token value. The manager still tracks expiry — if the pre-configured token has no known expiry (`_shared_token_expires_at` is nil), the 5-minute threshold check skips the expiry guard and the token is used until it fails with a 401, at which point a fresh token is fetched automatically.

**Practical implication:** `IGDB_ACCESS_TOKEN` is an optional convenience for dev/testing. In production, leave it unset and rely on the client credentials auto-refresh. `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` are always required.

### Concurrency safety

A single `sync.Mutex` (equivalent to the Python `asyncio.Lock`) guards concurrent token-fetch attempts. The double-checked locking pattern is used: check without lock → acquire lock → check again → fetch if still needed.

### Process-level token sharing

The token is stored in a package-level variable in `services/igdb.go`, shared across all usages within the process. There is no per-request auth overhead once the token is cached.

---

## Rate Limiting

The `Limiter` interface:

```go
type Limiter interface {
    Acquire(ctx context.Context) error
}
```

Both `igdb.go` and `steam.go` hold a `Limiter` and call `Acquire` before every outbound request. Each service gets its own limiter instance with independent configuration. The implementation is selected at startup via `RATE_LIMITER_BACKEND`:

**`local`** (default): wraps `golang.org/x/time/rate.NewLimiter`. IGDB defaults to 4 req/s burst 8; Steam defaults to 4 req/s burst 10.

**`postgres`**: a `rate_limiter_tokens` table:
```sql
CREATE TABLE rate_limiter_tokens (
    key         TEXT PRIMARY KEY,
    tokens      FLOAT   NOT NULL,
    last_refill TIMESTAMPTZ NOT NULL DEFAULT now()
);
```
Each acquisition does `SELECT ... FOR UPDATE`, refills tokens based on elapsed time since `last_refill`, decrements if available, updates the row. The `key` column allows multiple named limiters (e.g. `"igdb"`, `"steam"`) in the same table. Adequate for coordinating a small number of instances — no Redis required.

The `rate_limiter_tokens` table is always created by the initial migration regardless of which backend is selected.

---

## Authentication

JWT access + refresh tokens via `golang-jwt/jwt/v5`:

- **Access token**: short-lived (default 15 min, configurable via `ACCESS_TOKEN_EXPIRE_MINUTES`), carries `{type, sub, exp, iat}` claims (`sub` = user ID); no `role` or `is_admin` in the token — admin status is loaded from the DB on each authenticated request; validated by Echo middleware on protected routes
- **Refresh token**: longer-lived (default 30 days, configurable via `REFRESH_TOKEN_EXPIRE_DAYS`), stored as a hash in the `user_sessions` table, cleaned up every 30 minutes by the scheduler
- **Logout**: deletes the `UserSession` row; subsequent refresh attempts with that token return 401

> **Frontend fix required:** The Python frontend's logout flow only clears client-side token storage (localStorage/memory) without calling `POST /api/auth/logout`. As a result, the `UserSession` row is never deleted and refresh tokens remain valid indefinitely until the scheduler cleans them up. The Go port must fix this: the frontend logout handler **must** call `POST /api/auth/logout` before clearing local state. If the request fails (network error, 401), the frontend should still clear local state and redirect to `/login` — but the API call is not optional. This is an explicit in-scope frontend change for the Go port.

### First-Run Setup Flow

The Go port uses **server-driven setup gating**, matching the same pattern as the migration gate. Rather than the frontend polling a status endpoint, the middleware redirects all routes to `/setup` until at least one user exists. Full design detail is in `docs/superpowers/specs/2026-05-05-setup-flow-design.md`.

**Key decisions:**

1. `InitNeedsSetup` queries `SELECT COUNT(*) FROM users` at startup (after migrations) and sets a `needsSetup bool` on the `Migrator` struct. It is a **single-attempt call** — if the DB is unreachable, the `StartDBProbe` goroutine handles state transitions at the state-machine level; there is no internal retry loop in `InitNeedsSetup`. `InitNeedsSetup` is called from `initAppState()` only when the DB is confirmed reachable. Must complete **before** the HTTP server starts accepting requests — the zero value `false` would incorrectly bypass the gate during the startup window. If `initAppState()` itself fails (e.g. `determineState()` errors immediately after a successful ping), log the error and continue with state as `DBUnavailable`; `StartDBProbe` will retry on the next successful ping.
2. While `needsSetup` is true, the app-state middleware redirects all requests (except `/setup`, `/api/auth/setup/*`, `/health`, `/api/migrate/*`) to `/setup`.
3. `GET /setup` serves a self-contained static HTML page (inlined CSS + JS, no Vite build, same pattern as `ui/migrate/`). If `needsSetup=false`, it redirects to `/` — the gate works in both directions.
4. `POST /api/auth/setup/admin` creates the first admin user (SERIALIZABLE transaction to handle concurrent-request race). On success: issues access + refresh tokens via a shared `issueTokensAndSession` helper (same as login), clears `needsSetup`, and returns 201 with the user profile + tokens. The static setup page writes these to `localStorage` under key `'auth'` (camelCase shape expected by `AuthProvider`) then redirects to `/` — the user is immediately authenticated without a separate login step.
5. `POST /api/auth/setup/restore` — **deferred to Phase 3**.
6. Setup endpoints are JWT-exempt (no user exists yet to authenticate as).
7. Workers and the gocron scheduler are held in a gate-loop goroutine (same `shutdownCtx` as HTTP graceful shutdown) that polls `State() == Ready && !NeedsSetup()` every 2 seconds. They start only after setup is complete. This prevents scheduled tasks from running against an empty database.

**No React SPA changes** are required for this feature. The `RouteGuard` simplification (removing the `GET /api/auth/setup/status` call) should be confirmed during Phase 2 frontend work if that call exists in the Python code being ported.

> **`GET /api/auth/me` is a Phase 1 endpoint.** The setup page writes tokens to `localStorage` then redirects to `/`, at which point the React SPA's `AuthProvider` immediately calls `GET /api/auth/me` to validate the token and populate the user object. If this endpoint is deferred to Phase 2, the SPA breaks immediately after setup completes. It must be implemented alongside the setup flow in Phase 1. See the Phase 1 roadmap entry and Profile and Credential Management section.

### Profile and Credential Management

- `GET /api/auth/me` — returns current user profile; used by the frontend as a token-validity probe on app load
- `PUT /api/auth/me` — update user preferences (stored as JSON in `users.preferences`)
- `PUT /api/auth/change-password` — changes password, invalidates **all** existing sessions for that user, forcing re-login on all devices
- `GET /api/auth/username/check/:username` — availability check, no side effects
- `PUT /api/auth/username` — change username

### User Registration

There is no public self-registration endpoint in the Go port. The Python codebase had a `POST /auth/register` endpoint but it was removed early in development; admin-created users is the only supported flow. New users are created exclusively via `POST /api/auth/admin/users`. The Go port does not implement a `/register` route.

### Admin User Management

All `/api/auth/admin/*` endpoints require the `is_admin` claim. An admin cannot remove their own admin privileges or delete their own account (enforced in the handler). Deleting a user cascades to all their games, tags, jobs, sessions, and sync configs.

`GET /api/auth/admin/users/:id/deletion-impact` returns a preview of what will be deleted. The response does **not** include a `total_wishlist_items` field — the Wishlist feature is not implemented in the Go port (see Out of Scope). The Python version returned this field; the frontend will be updated to remove that count from the deletion-impact display.

`UserSession` stores `token_hash` and `refresh_token_hash` (bcrypt cost 12, defined as `const bcryptCost = 12` in `internal/api/auth.go` and shared by all password-hashing call sites), plus `user_agent` and `ip_address` for audit purposes.

---

## Platform Resolution and Game Matching Services

Two internal services that the sync system depends on, not exposed as API endpoints:

**`services/platform_resolution.go`** — maps raw platform name strings arriving from external sync sources (e.g. `"PC"`, `"PlayStation 5"`) to canonical `Platform` slugs in the database (e.g. `"pc-windows"`, `"ps5"`). Maintains a mapping table derived from the Python `platform_resolution` service. Used by `ProcessSyncItemTask` to fill `UserGamePlatform.platform` and `UserGamePlatform.original_platform_name`.

**`services/matching.go`** — IGDB game lookup during sync/import. When a new `ExternalGame` is encountered with no `resolved_igdb_id`, this service searches IGDB by title, applies fuzzy confidence scoring via `FuzzyConfidence` (go-fuzzywuzzy), and returns the best candidate. Once resolved, the result is cached in `external_games.resolved_igdb_id` and never re-queried. Used by `ProcessSyncItemTask` and `ProcessImportItemTask`.

---

## Services: Static Files

**`services/storage.go`** manages two on-disk directories:

- **Cover art**: `{STORAGE_PATH}/cover_art/` — downloaded from IGDB during game import. Served at `/static/cover_art/*`.
- **Platform/storefront logos**: committed to the repository under `ui/public/logos/` and served as part of the frontend SPA. The `icon` column in `platforms` and `storefronts` stores the filename only (e.g. `steam.svg`); the frontend constructs the full path. No Go-side route or embed is needed for logos.

Both paths are registered as Echo `Static` routes before the SPA catch-all. URLs stored in the database for cover art use the `/static/cover_art/` prefix, matching the Python version — no URL migration is required when importing data.

### Backup Archive Format

Backup archives are `.tar.gz` files created by `BackupCreateTask` and the manual `POST /api/admin/backups` endpoint. Each archive contains:

```
backup-{id}.tar.gz
└── backup-{id}/
    ├── manifest.json       # Metadata: backup ID, type, timestamps, file checksums, DB stats
    ├── database.sql        # Full pg_dump output (plain SQL format)
    └── cover_art/          # Copy of {STORAGE_PATH}/cover_art/ directory
```

**Restore** (`POST /api/admin/backups/:id/restore`, `POST /api/admin/backups/restore/upload`, `POST /api/auth/setup/restore`) extracts the archive and:
1. Runs `psql` (or equivalent) to restore `database.sql` into the database.
2. Copies `cover_art/` back to `{STORAGE_PATH}/cover_art/`.

Platform/storefront logos are embedded in the binary and are not included in backup archives.

**Manifest** (`manifest.json`) includes: `backup_id`, `backup_type` (`manual`|`scheduled`), `created_at`, per-file checksums and sizes, and DB stats (user count, game count, tag count). The `backup_type` field uses the same enum values as the Python version.

### Unreferenced Game Cleanup (via `DELETE /api/user-games/:id` or bulk delete), the handler checks whether any other user has the same `game_id` in their collection. If no other `user_games` row references that game, the `games` row is deleted and its cover art file on disk is removed. This mirrors the Python `cleanup_unreferenced_game()` behaviour.

The check and cleanup run within the same transaction as the user-game deletion. No separate worker task is needed — the operation is fast (single indexed lookup + conditional delete).

The Go port does **not** check the wishlists table (which is dropped), simplifying the reference check to `user_games` only.

---

## Restore Behaviour

`POST /api/admin/backups/:id/restore` and `POST /api/admin/backups/restore/upload` reset the database to a previous state. Because the database is being replaced wholesale, all in-flight application state becomes invalid the moment the restore starts.

**Approach:** the restore handler:
1. Sets a process-level **maintenance mode** flag before touching the pool.
2. While maintenance mode is active, the maintenance middleware returns `503 Service Unavailable` for all requests except `/health` and the backup admin endpoints (`/api/admin/backups/*`, `/api/auth/me`). This gives in-flight requests a clean failure rather than a connection-reset error.
3. Closes the pgxpool connection pool, waits for active connections to drain (10-second timeout; forced close if exceeded).
4. Shuts down the worker pool (same `Shutdown()` path as graceful SIGTERM). In-flight worker tasks will fail; their `pending_tasks` rows remain in `running` state and are orphaned by the restore (the restored DB has different data). This is correct.
5. Runs the restore (pg_restore + file copy from archive).
6. Calls `os.Exit(0)`. The process manager (systemd, Kubernetes, Docker) restarts the binary; on restart the app re-runs its startup sequence and transitions to `Ready`.

### Maintenance Mode Middleware

A package-level `maintenanceMode bool` (protected by a `sync.RWMutex`) drives the middleware:

```go
// internal/middleware/maintenance.go

var (
    mu              sync.RWMutex
    maintenanceMode bool
)

func SetMaintenanceMode(enabled bool) {
    mu.Lock()
    defer mu.Unlock()
    maintenanceMode = enabled
}

func IsMaintenanceMode() bool {
    mu.RLock()
    defer mu.RUnlock()
    return maintenanceMode
}
```

The Echo middleware for maintenance mode sits inside the app-state middleware (i.e. only runs once state is `Ready`) and is checked on every request. Allowed during maintenance:

- `GET /health`
- `GET|POST|DELETE /api/admin/backups/*` — the restore operation itself, and any concurrent admin backup actions
- `GET /api/auth/me` — lets the frontend confirm the session is still valid while maintenance is in progress

All other requests receive:
```json
{ "error": "Service Unavailable", "detail": "Restore in progress", "maintenance_mode": true }
```
with HTTP status `503`.

---

## Configuration

All existing Python env var names are preserved. New Go-specific vars are additive. The following Python vars are dropped and must be removed from `.env` when migrating:

| Dropped var | Reason |
|---|---|
| `NATS_URL` | NATS eliminated |
| `RATE_LIMITER_NATS_BUCKET`, `RATE_LIMITER_CAS_MAX_RETRIES`, `RATE_LIMITER_CAS_RETRY_BASE_MS`, `RATE_LIMITER_CAS_RETRY_MAX_MS` | NATS rate limiter replaced by local/postgres backends |
| `INTERNAL_API_KEY`, `INTERNAL_API_URL` | Worker-to-API HTTP callbacks eliminated; workers run in-process. The Python `POST /api/admin/backups/internal/create` endpoint (hidden from schema) was called by the worker via HTTP using `INTERNAL_API_KEY`; this is replaced by a direct in-process function call in the Go port. |
| `JWT_SECRET` | Consolidated into `SECRET_KEY` (the Python version only uses `SECRET_KEY` for JWT) |
| `SCHEDULER_RECONNECT_*` | Scheduler reconnection logic was NATS-specific |

```go
type Config struct {
    // Database
    // DATABASE_URL takes priority when set. When not set, the individual DB_* vars are
    // used to construct the URL — matching the Python config behaviour exactly.
    DatabaseURL string `env:"DATABASE_URL"`
    DBHost      string `env:"DB_HOST" envDefault:"localhost"`
    DBPort      int    `env:"DB_PORT" envDefault:"5432"`
    DBUser      string `env:"DB_USER" envDefault:"nexorious"`
    DBPassword  string `env:"DB_PASSWORD" envDefault:"nexorious"`
    DBName      string `env:"DB_NAME" envDefault:"nexorious"`
    DBSSLMode   string `env:"DB_SSLMODE" envDefault:"disable"`

    // Security
    SecretKey string `env:"SECRET_KEY,required"` // used for JWT signing and credential encryption

    // JWT lifetimes
    // Note: Python defaults to 30 minutes; Go port uses 15 minutes (deliberate tightening).
    AccessTokenExpireMinutes int `env:"ACCESS_TOKEN_EXPIRE_MINUTES" envDefault:"15"`
    RefreshTokenExpireDays   int `env:"REFRESH_TOKEN_EXPIRE_DAYS" envDefault:"30"`

    // IGDB
    IGDBClientID          string  `env:"IGDB_CLIENT_ID,required"`   // required — see note below
    IGDBClientSecret      string  `env:"IGDB_CLIENT_SECRET,required"` // required — see note below
    IGDBAccessToken       string  `env:"IGDB_ACCESS_TOKEN"`           // optional pre-configured bearer token
    IGDBRequestsPerSecond float64 `env:"IGDB_REQUESTS_PER_SECOND" envDefault:"4.0"`
    IGDBBurstCapacity     int     `env:"IGDB_BURST_CAPACITY" envDefault:"8"`
    IGDBMaxRetries        int     `env:"IGDB_MAX_RETRIES" envDefault:"3"`
    IGDBBackoffFactor     float64 `env:"IGDB_BACKOFF_FACTOR" envDefault:"1.0"`

    // Steam
    SteamRequestsPerSecond float64 `env:"STEAM_REQUESTS_PER_SECOND" envDefault:"4.0"`
    SteamBurstCapacity     int     `env:"STEAM_BURST_CAPACITY" envDefault:"10"`

    // Storage
    StoragePath    string `env:"STORAGE_PATH" envDefault:"./storage"`
    BackupPath     string `env:"BACKUP_PATH" envDefault:"./storage/backups"`
    TempStorageDir string `env:"TEMP_STORAGE_DIR" envDefault:"/tmp/nexorious_uploads"`

    // Application
    Port     int    `env:"PORT" envDefault:"8000"`
    LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
    Debug    bool   `env:"DEBUG" envDefault:"false"`

    // CORS (development only — production is same-origin)
    CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`

    // Workers
    WorkerCount int `env:"WORKER_COUNT" envDefault:"4"`

    // Scheduler
    MetadataRefreshInterval string `env:"METADATA_REFRESH_INTERVAL" envDefault:"24h"`
    // Note: backup schedule is stored in the backup_config table, not as an env var.
    // The initial migration seeds backup_config with schedule_cron = "0 2 * * *" (daily at 2 AM UTC).

    // Rate limiter
    RateLimiterBackend string `env:"RATE_LIMITER_BACKEND" envDefault:"local"`
}
```

**Database URL resolution:** At startup, if `DATABASE_URL` is set (non-empty), it is used as-is. If it is empty or absent, the binary constructs a URL from `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, and `DB_SSLMODE`, URL-encoding the user and password components. `DATABASE_URL` must be set or the individual `DB_*` vars must produce a valid URL — the binary fails to start if neither is usable.

**Resolved URL construction** (the single authoritative snippet used across `main.go`, `NewMigrator`, `NewDBErrorHandler`, and `--migrate-only`):

```go
// resolveDBURL returns the connection string to pass to pgxpool.New and
// golang-migrate. It is computed once in main() and passed everywhere.
func resolveDBURL(cfg *config.Config) string {
    if cfg.DatabaseURL != "" {
        return cfg.DatabaseURL
    }
    // Construct from individual DB_* vars. URL-encode user and password so
    // special characters (@ : / etc.) in the values do not break URL parsing.
    return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
        url.QueryEscape(cfg.DBUser),
        url.QueryEscape(cfg.DBPassword),
        cfg.DBHost,
        cfg.DBPort,
        cfg.DBName,
        cfg.DBSSLMode,
    )
}
```

`resolveDBURL` is called once in `main()` before `pgxpool.New`. The returned string is stored as `resolvedDatabaseURL` and passed to:
- `pgxpool.New(ctx, resolvedDatabaseURL)` — pool creation
- `NewMigrator(resolvedDatabaseURL)` — stored for lazy `gmigrate.NewWithSourceInstance` call
- `NewDBErrorHandler(resolvedDatabaseURL, migrator)` — DSN redaction at construction time
- Both the normal and `--migrate-only` paths use the same `resolveDBURL` call; there is no second resolution site.

**IGDB credential note:** `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` are marked `required` — the binary will refuse to start without them. The Python implementation marks them `Optional` and emits a startup warning instead, which was a mistake: IGDB credentials are load-bearing for game search, import, and the sync matching pipeline. Allowing the app to start without them produces a degraded-but-plausible-looking state where those features silently fail at runtime. The Go port treats them as required to make the dependency explicit at startup.

**Storage path notes:**

- `STORAGE_PATH` and `BACKUP_PATH` default to directories relative to the working directory of the running binary (`./storage` and `./storage/backups`). In production these should be set to absolute paths on a persistent volume.
- `BACKUP_PATH` must be readable by the Go process at request time because `GET /api/admin/backups/:id/download` streams backup archives directly from disk to the HTTP response. The default `./storage/backups` satisfies this — it is under the same root as cover art and is managed entirely by the application. Do **not** point `BACKUP_PATH` at a path the process cannot read.

---

## CLI Flags

The binary accepts a small set of command-line flags. Configuration remains env-var-driven (`caarlos0/env`); flags are reserved for *per-invocation modes and overrides* — things you'd set at launch time rather than in a deployment's environment.

Implemented using stdlib `flag` (no new dependency for Phase 1). Cobra subcommands are deferred to Phase 5 (see Phased Roadmap).

### Phase 1 flags

| Flag | Default | Description |
|---|---|---|
| `--help`, `-h` | — | Print usage and exit (stdlib `flag` provides this automatically) |
| `--version`, `-v` | — | Print `nexorious <version> (<commit>)` and exit; version and commit are injected at build time via `-ldflags` |
| `--config` | `""` | Path to a `.env` file; loaded before env vars are parsed by `caarlos0/env`. When empty, the binary checks for a `.env` file in the working directory (matching `godotenv`'s default behaviour) |
| `--migrate-only` | `false` | Run pending migrations then exit with code 0 (or non-zero on failure). Does not start the HTTP server or workers. The standard pattern for Kubernetes `initContainers` |

**`--migrate-only` startup sequence** (explicit — differs meaningfully from the normal server path):

```
Parse config
Resolve DATABASE_URL (see Configuration section)
pgxpool.New(resolvedURL)   ← fatal on any error (DSN parse or TLS config; these are always misconfigurations)
pool.Ping()                ← retry with 2s backoff for up to 30 seconds; fatal if DB unreachable after timeout
NewMigrator(resolvedURL)   ← cheap constructor; state = DBUnavailable
migrator.determineState()  ← connects golang-migrate to DB; fatal on error
if state == Ready:
    print "No pending migrations." and exit 0
migrator.RunMigrations()   ← runs all pending migrations; streams log lines to stderr
if RunMigrations() fails:
    print error and exit 1
exit 0
```

Key differences from the normal server path:
- **No HTTP server is started.** The browser migration UI is irrelevant.
- **No `StartDBProbe` goroutine.** `--migrate-only` is not a long-running process; transient DB unavailability is fatal after the retry window (no self-healing needed).
- **No `InitNeedsSetup` call.** Setup is intentionally skipped — this mode is for Kubernetes `initContainers` that run migrations and exit; the web UI handles setup on the next normal start.
- **No worker/scheduler gate loop.** Workers are never started.
- **`pgxpool.New` errors are always fatal** in `--migrate-only` mode (same as normal mode; any `pgxpool.New` error is a misconfiguration, not a transient failure).
- **`pool.Ping()` retry:** normal mode leaves state as `DBUnavailable` and continues; `--migrate-only` must actually reach the DB to run migrations, so it retries with a short backoff (2s × 15 = 30s) and fatals on timeout. Log each retry attempt at WARN level.

### Build-time version injection

```makefile
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT  ?= $(shell git rev-parse --short HEAD)

build:
    go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" ./cmd/nexorious
```

`--version` prints: `nexorious v0.1.0-3-gabcdef1 (abcdef1)`

### Future: Cobra subcommands (Phase 5)

Phase 1 uses stdlib `flag` because the binary has a single mode of operation. Phase 5 will migrate to `cobra` to support subcommands such as:

```
nexorious serve           # default: start the HTTP server (all current behaviour)
nexorious migrate         # run pending migrations (replaces --migrate-only)
nexorious migrate status  # print pending count and current version, then exit
nexorious version         # print version info (replaces --version flag)
```

The migration to Cobra in Phase 5 is a breaking change to the CLI surface. Any tooling (Helm chart, systemd units, Kubernetes manifests) that invokes `nexorious --migrate-only` must be updated to `nexorious migrate`.

---

## Epic Games Store Sync

The Python version uses the `legendary-gl` Python library directly. The Go version shells out to the `legendary` CLI binary if it is present on `PATH`. If `legendary` is not found, Epic sync is disabled gracefully (the sync endpoint returns a descriptive error; other sync providers are unaffected).

This feature may be deferred if the shell-out approach proves unreliable in practice.

---

## Testing

- **DB tests**: `testcontainers-go` spins up a real PostgreSQL container per test suite; migrations run against it before tests execute
- **Handler tests**: Echo's `httptest` utilities with a real test DB — no DB mocking
- **Task tests**: `TaskFunc` functions are plain Go functions, tested directly without the pool
- **IGDB tests**: `net/http/httptest` server mocks IGDB API responses
- **Coverage target**: >80% overall, matching the Python version

---

## Development Environment (devenv)

The Go repo uses a single flat `devenv.nix` at the root — no subdirectory devenv imports are needed since Go and the React frontend (`ui/`) both live in the same repo.

### `devenv.nix`

```nix
{ pkgs, lib, config, inputs, ... }:

{
  packages = with pkgs; [
    sqlc           # Query code generation (run when queries/*.sql changes)
    golangci-lint  # Go linter (equivalent of ruff)
    go-migrate     # golang-migrate CLI for manual migration inspection
    legendary-gl   # Epic Games Store sync (optional, may be absent)
  ];

  languages.go = {
    enable  = true;
    package = pkgs.go_1_24;  # or latest available
  };

  languages.javascript = {
    enable = true;
    npm.enable = true;
  };

  services.postgres = {
    enable     = true;
    package    = pkgs.postgresql_16;
    initialDatabases = [{ name = "nexorious"; }];
  };

  env = {
    DATABASE_URL = "postgresql://localhost/nexorious?sslmode=disable";
    ENABLE_LSP_TOOL = 1;  # Claude Code LSP workaround (matches Python devenv)
  };
}
```

### `devenv.yaml`

```yaml
inputs:
  nixpkgs:
    url: github:cachix/devenv-nixpkgs/rolling
```

No `imports:` — unlike the Python version which merged `backend/` and `frontend/` devenvs, the Go repo is a single unified environment.

### What `devenv shell` provides

| Tool | Purpose |
|---|---|
| `go` | Go toolchain (build, test, vet) |
| `node` / `npm` | React frontend build (`cd ui && npm run build`) |
| `sqlc` | Regenerate `internal/db/gen/` from query files |
| `golangci-lint` | Linting (`golangci-lint run ./...`) |
| `migrate` | Inspect migration state manually |
| `legendary` | Epic sync (if available in nixpkgs) |
| `psql` | Direct DB access; `$DATABASE_URL` is pre-set |

PostgreSQL runs as a devenv service. **Services are not started by `devenv shell`** — they require a separate `devenv up` (foreground) or `devenv up -d` (background/daemonized). `devenv shell` only activates the environment (tools, env vars, language toolchains). The typical workflow is to run `devenv up -d` once in the background and then work in `devenv shell`. No separate Postgres install or Docker container needed for development — testcontainers is still used for CI and isolated test runs.

---

## Build Process (Makefile)

```makefile
.PHONY: all frontend generate build

all: frontend generate build

frontend:
	cd ui && npm install && npm run build

generate:
	sqlc generate

build:
	go build ./cmd/nexorious
```

The React source lives in `ui/` inside the Go repo (Stash-style). `make frontend` builds it in place; `go build` then embeds `ui/dist/` via the `//go:embed` directive in `ui/ui.go`. No cross-repo file copying required.

---

## Phased Implementation Roadmap

Implementation should proceed in phases. Each phase ends with a working, deployable binary. Start a new planning session (`/plan`) for each phase when ready to implement it.

### Phase 1 — Infrastructure Skeleton
*Goal: a working binary that starts, runs migrations in the browser, serves the React SPA, and handles auth.*

- Project scaffolding: `go.mod`, directory structure, Makefile
- CLI flags: `--help`, `--version`, `--config`, `--migrate-only` (stdlib `flag`; build-time version injection via `-ldflags`)
- Config (`caarlos0/env`)
- `pgxpool` connection + initial schema migration (`0001_initial.up.sql`) — full table list including all models
- golang-migrate + migration state machine + browser migration UI (SSE)
- Echo HTTP server: middleware stack, route zones, SPA fallback with `embed.FS`
- Static file routes: `/static/cover_art/*` and `/static/logos/*`
- JWT auth: login, refresh, logout; first-run setup flow (server-driven middleware gate, setup/admin); `needsSetup` flag cleared after first admin created
- `GET /api/auth/me` — current user profile; required in Phase 1 because the setup page writes tokens to `localStorage` then redirects to `/`, at which point the React SPA's `AuthProvider` immediately calls this endpoint to validate the token and populate the user object. Without it the SPA breaks on first load after setup. Implementation: verify JWT, query `users` table by `user_id` claim, return profile. Shares the same raw `pgxpool` approach as other auth endpoints (not sqlc; see Database Layer).
- Health/status endpoint
- `internal/filter/` package: filterBuilder + criterion handlers

**Checkpoint:** binary starts, browser shows migration UI on first run, React app loads after migration, setup completes end-to-end (including SPA redirect), login works.

---

### Phase 2 — Core Game API
*Goal: full read/write game collection functionality via the existing React UI.*

- sqlc schema definitions (from Python models) + generated code
- Games API (`/api/games`, `/api/games/:id`, search, IGDB import, metadata endpoints)
- User games API (list with dynamic filtering via filterBuilder + goqu, sort, CRUD, platform associations)
- IGDB result ranking: `go-fuzzywuzzy` + `NormalizeTitle`; local list search uses `ILIKE` only
- Platforms, tags, and user-games filter-options / genres / stats / ids endpoints
- IGDB service (rate-limited HTTP client, cover art + logos storage)
- Remaining auth profile endpoints: `PUT /api/auth/me`, `PUT /api/auth/change-password`, `GET /api/auth/username/check/:username`, `PUT /api/auth/username` (`GET /api/auth/me` is Phase 1 — see above)

**Checkpoint:** React frontend fully usable for browsing and managing game collection.

---

### Phase 3 — Background Workers + Import/Export
*Goal: data migration path from Python version, plus export/backup.*

- Worker pool (database-backed task queue: `pending_tasks` table, `SELECT FOR UPDATE SKIP LOCKED`, goroutine workers with in-process notify channel for zero-latency wake-up)
- Job tracking (jobs + job_items tables, full `/api/jobs` and `/api/job-items` endpoints including review workflow)
- gocron scheduler (cleanup jobs wired up)
- Import handler (`POST /api/import/nexorious` — nexorious JSON format, the upgrade path from Python)
- Export handler
- Backup create + scheduled backup
- `POST /api/auth/setup/restore` — deferred from Phase 1; shares restore logic with the backup system implemented here

**Checkpoint:** existing Python users can export their data and import it into the Go version.

---

### Phase 4 — Sync Integrations
*Goal: automated library sync from external platforms.*

- `external_games` and `user_sync_configs` sync config API handlers
- Skip/un-skip endpoints via sync router (`GET/DELETE /api/sync/ignored`) operating on `external_games.is_skipped`
- `services/platform_resolution.go` — raw platform name → slug mapping
- `services/matching.go` — IGDB title matching using `FuzzyConfidence` (go-fuzzywuzzy)
- Steam sync (dispatch + process)
- PSN sync
- Epic Games Store sync (legendary-gl shell-out; defer if unreliable)
- Metadata refresh (dispatch + process)
- Remaining scheduler jobs (sync check every 15 minutes, metadata refresh interval)

**Checkpoint:** sync integrations work end-to-end.

---

### Phase 5 — Polish + Production Readiness
*Goal: production-grade deployment.*

- PostgreSQL-backed rate limiter (multi-instance support)
- Migrate CLI surface to `cobra` subcommands (`serve`, `migrate`, `migrate status`, `version`); update Helm chart, systemd units, and any tooling that uses `--migrate-only`
- Full test coverage (testcontainers-go, >80%)
- Dockerfile (single-stage: React build → go build → minimal runtime image)
- Helm chart (adapted from existing nexorious chart)
- Documentation updates

**Checkpoint:** ready to replace the Python version in production.

---

### Phase 6 — Embedded PostgreSQL (Zero-Dependency Mode)
*Goal: single binary that works out of the box with no external dependencies, for evaluation and personal use.*

Use [`fergusstrange/embedded-postgres`](https://github.com/fergusstrange/embedded-postgres) to bundle a real PostgreSQL instance that the Go binary can start and manage itself. This is strictly opt-in — production deployments continue to use an external PostgreSQL configured via `DATABASE_URL`.

**Approach:**
- Add `POSTGRES_MODE=embedded|external` config flag (default `external`)
- When `embedded`: binary starts its own PostgreSQL on a local port, sets `DATABASE_URL` internally, manages data directory via `EMBEDDED_POSTGRES_DATA_DIR` (default `./data`)
- When `external`: behaviour is identical to the current design — `DATABASE_URL` is required
- Migration UI and all other behaviour is identical in both modes
- Embedded mode is not recommended for multi-user or production use; a startup warning makes this clear

**Why last:** embedded-postgres adds meaningful binary size and download complexity (it fetches a Postgres binary at first run). It should only be added once the port is stable, well-tested, and the external-Postgres path is the proven baseline. Doing it earlier risks conflating embedded-mode bugs with port bugs.

**Checkpoint:** user can download a single binary, run it, and have a working nexorious instance with no other setup.

---

## Out of Scope

- Frontend rewrite (React SPA is kept as-is)
- Porting Alembic migration history (fresh schema; JSON import for data migration)
- PSN sync implementation details (ported from Python, no architectural changes)
- Helm chart (can be adapted from the existing nexorious chart once the binary is stable)
- Darkadia import source (not currently active in the Python version)
- One-time Steam library import (the Python `import_sources/steam.py` one-shot import; ongoing sync via `user_sync_configs` covers this use case)
- Wishlist table and API — the Python `Wishlist` model has no user-facing API routes and the frontend calls no wishlist endpoints. **Do not bring the `wishlists` table over** to the Go schema. The `total_wishlist_items` field in the Python deletion-impact response is dropped — the Go port's `GET /api/auth/admin/users/:id/deletion-impact` response does not include it, and the frontend deletion-impact UI must be updated to remove that row. **Import handling:** the Python JSON export format includes a `wishlist` array in the export payload. The Go import handler must silently discard this key — do not error if it is present.
- GOG sync — the `SyncPlatform` enum includes `"gog"` for forward-compatibility, but `POST /api/sync/gog` returns `501 Not Implemented`. No GOG sync adapter is implemented in this port; it is deferred to a future task.
- `ignored_external_games` table — **do not bring this over**; it was already dropped in the Python codebase (Mar 2026 Alembic migration `bbcb63f60154`) and replaced by `external_games.is_skipped`. The Python ORM file `backend/app/models/ignored_external_game.py` still exists as a leftover artefact but the table no longer exists in a migrated database. The Go schema has no `ignored_external_games` table.
- `import_mappings` — **do not bring this over**; dead code in both Python and frontend. The Python backend has schema definitions and a Pydantic schema file but no DB model and no registered router — the `/api/import-mappings/` endpoint never existed at runtime. The frontend has a full API client, hooks, and type definitions for it, but zero route components or UI pages call any of those functions. If the feature is ever completed, it should be designed and implemented from scratch.
- `pg_trgm` / local-DB fuzzy search — the Python `fuzzy_threshold` parameter on list endpoints was never wired to the frontend UI; the Go port uses `ILIKE` for local text search only
- Credentials in the JSON export — the nexorious export format contains only collection data (games, platforms, tags, user metadata). `user_sync_configs` and platform credentials are deliberately excluded from exports, so the import handler requires no credential-migration handling

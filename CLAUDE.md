# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Reference

### Common Commands

| Task                     | Command                                                  |
|--------------------------|----------------------------------------------------------|
| Run cmd in dev shell     | `devenv shell -- <command>` (only if truly necessary)    |
| Build backend            | `make build`                                             |
| Build frontend           | `make frontend`                                          |
| Build everything         | `make`                                                   |
| Run server               | `./nexorious` or `./nexorious serve`                     |
| Run migrations           | `./nexorious migrate`                                    |
| Migration status         | `./nexorious migrate status`                             |
| Run tests (Go)           | `go test -timeout 600s ./...`                            |
| Run single test          | `go test ./internal/api/... -run TestGamesList -v`       |
| Run tests with coverage  | `go test -timeout 600s -cover ./...`                     |
| Type check (frontend)    | `npm run check`  (from `ui/frontend/`)                   |
| Run frontend tests       | `npm run test`   (from `ui/frontend/`)                   |
| Lint Go                  | `golangci-lint run`                                      |
| Run API client           | `slumber`                                                |

### Environment Validation
```bash
go version   # expect go 1.25+
make --version
```

## Setup & Development

### Development Environment
Uses devenv for a reproducible shell (Go 1.25, golangci-lint, make, Node 24, TypeScript).

> **Run commands directly** — `go`, `make`, `npm`, etc. are all on PATH in the devenv-activated shell. Only use `devenv shell -- <command>` when a tool is genuinely not available in the current environment. Never use `devenv shell --command`; the correct separator is `--`.

### Build
```bash
make             # frontend → go build
make frontend    # builds React SPA into ui/frontend/dist/
make build       # compiles the Go binary
```

`ui/frontend/dist/` is gitignored and must be populated by `make frontend` before `go build`. The Go binary embeds four UI dirs via `//go:embed`: `all:frontend/dist`, `all:migrate`, `db-error`, and `setup` (see `ui/ui.go`).

### Initial Setup
```bash
devenv shell
make                     # builds everything
export DATABASE_URL="postgres://..."
./nexorious              # starts server; visits /migrate if schema is pending
```

## Project Structure

- `cmd/nexorious/` — entry point, wires config/db/echo/workers
- `internal/api/` — Echo route handlers per domain (games, user_games, auth, setup, platforms, tags, jobs, job_items, import, export, backup, sync, db_error)
- `internal/db/` — database layer
  - `migrations/` — Bun migrate SQL files with timestamp-prefix naming (`20260503000001_name.up.sql` / `.down.sql`); auto-discovered via `//go:embed *.sql` in `migrations.go`
  - `models/` — Bun model structs (hand-edited)
- `internal/migrate/` — migration state machine + Echo handlers for `/migrate` and `/api/migrate/*`
- `internal/worker/` — River job worker implementations; `tasks/` contains workers for sync, import, export, and metadata refresh
- `internal/scheduler/` — River worker implementations for periodic maintenance jobs (cleanup, backup polling, stale job pruning); `BuildPeriodicJobs()` registers cron-scheduled River `PeriodicJob` entries
- `internal/services/` — IGDB client, Steam/PSN sync, game matching, platform resolution
- `internal/auth/` — JWT generation/validation + Echo middleware
- `internal/filter/` — dynamic query builder (Bun) for user-game list filtering
- `internal/ratelimit/` — interface + local (`x/time/rate`) and PostgreSQL implementations
- `internal/config/` — config struct via `caarlos0/env`
- `internal/enum/` — shared enum types
- `internal/middleware/` — Echo middleware (state gate, auth bridge, etc.)
- `internal/backup/` — backup orchestration (invoked from scheduler)
- `ui/` — contains `ui/frontend/` (React SPA source + build output), `ui/migrate/` (migration HTML), `ui/db-error/`, `ui/setup/`

## Architecture

### Startup & App State Machine
```
DBUnavailable ↔ NeedsMigration → Migrating → Ready
```
Echo middleware blocks all non-migration routes until state is `Ready`. River workers and the cron-based periodic job scheduler start only after migrations complete. Graceful shutdown waits for in-flight River jobs on `SIGTERM`/`SIGINT`.

### Database Layer
- **ORM**: Bun (`uptrace/bun`) with model structs in `internal/db/models/`. Queries use Bun's query builder.
- **Exception**: `internal/auth` uses raw `db.NewRaw`/`db.QueryRow` directly (not Bun models) to keep auth isolated.
- **Dynamic filter queries**: Bun query builder used in `internal/filter/` for user-game list filtering.
- **Driver**: `pgx/v5` via Bun's own `pgdriver` (`uptrace/bun/driver/pgdriver`).
- **Migrations**: Bun migrate (`uptrace/bun/migrate`); SQL files live in `internal/db/migrations/` with timestamp-prefix naming (`YYYYMMDDHHmmss_name.up.sql`); discovered automatically via `Migrations.Discover(FS)` in `migrations.go`.

### Frontend Embedding (Stash pattern)
`ui/ui.go` exposes four `embed.FS` vars:
```go
//go:embed all:frontend/dist
var UIBox embed.FS      // main React SPA

//go:embed all:migrate
var MigrateBox embed.FS // standalone migration UI

//go:embed db-error
var DBErrorBox embed.FS

//go:embed setup
var SetupBox embed.FS
```
FastAPI is eliminated; the Go binary serves the React SPA itself.

### Frontend Stack (unchanged from nexorious)
- Vite 6 + React 19 + TypeScript
- TanStack Router (file-based routes in `ui/frontend/src/routes/`)
- TanStack Query, Tailwind CSS v4, shadcn/ui, React Hook Form + Zod, TipTap
- Vitest + @testing-library/react

### Route Zones
- **Migration zone** (`/migrate`, `/api/migrate/*`) — always available, bypasses state middleware
- **Setup zone** (`/api/auth/setup/*`) — requires `Ready` state, no JWT (no users exist yet)
- **API zone** — gated by state middleware, then JWT where required

### Workers & Scheduler
River (`riverqueue/river`) is the job queue. Worker structs live under `worker/tasks/` (sync, import, export, metadata refresh) and `internal/scheduler/` (cleanup jobs, backup polling). Periodic schedules are registered in `scheduler.BuildPeriodicJobs()` using `robfig/cron/v3` expressions and River `PeriodicJob`. Backup orchestration still lives in `internal/backup/` and is invoked by the `CheckScheduledBackupWorker`.

### Rate Limiting
`ratelimit.Limiter` interface with two implementations:
- `local.go` — `golang.org/x/time/rate` (single instance)
- `postgres.go` — PostgreSQL `SELECT FOR UPDATE`` (multi-instance, opt-in via config)

## Testing

### Policy

Write a test when:
- The behaviour is security-sensitive (auth, token validation, permission checks)
- There are multiple meaningful edge cases (missing fields, wrong types, not found, conflict)
- The logic is non-obvious or involves a subtle invariant
- A real bug was found — the test documents that it cannot regress

Do NOT write a test when:
- The function is a thin wrapper or a struct field accessor
- The test only verifies that calling the function returns what it computes (tautology)
- The only assertion is "no panic on happy path" with no behavioural verification
- Coverage percentage is the motivation

There is no coverage gate in CI. The quality gate is: does the PR touching non-trivial logic include a test that would have caught a plausible bug in that logic?

### Performance

Each package that needs a real database uses a shared PostgreSQL container via `TestMain`. The container starts once per `go test` invocation per package; migrations run once at startup. Each test calls `truncateAllTables(t)` at the top for isolation. Do NOT call a per-test `setupXxxDB(t)` helper that starts a new container — use the shared `testDB` package variable instead.

### Running Tests

- **Framework**: stdlib `testing` + `testcontainers-go`
- Run all: `go test ./...`
- Run single: `go test ./internal/api/... -run TestFunctionName -v`
- Frontend: from `ui/frontend/` — `npm run test`, single file: `npm run test game-card.test.tsx`

## Development Rules

> **Always ask questions if you are uncertain about something!**

### Essential Workflow
1. **Planning**: Read `docs/superpowers/specs/` for design context
2. **Branching**: Create a feature branch before starting any task
3. **Migrations**: Add new `.up.sql` / `.down.sql` files in `internal/db/migrations/` using timestamp-prefix naming (`YYYYMMDDHHmmss_name.up.sql`); Bun discovers them automatically via `Migrations.Discover(FS)`
4. **Testing**: Run `go test ./...` after any Go changes; `npm run check && npm run test` after any frontend changes
5. **Plan files**: `docs/superpowers/plans/` is tracked — always commit the plan file on the feature branch

### Branch Workflow (MANDATORY)
- ✅ Always create a branch before starting task work
- ✅ Use `--squash --delete-branch` when merging PRs
- ❌ Never commit directly to main unless instructed to
- ❌ Never merge a PR on your own initiative — only when the user explicitly instructs

### Code Style

**Go (Backend)**
- Standard Go conventions: `camelCase` unexported, `PascalCase` exported, `UPPER_CASE` constants
- Errors returned, not panicked; wrap with `fmt.Errorf("context: %w", err)`
- Echo handler signature: `func (h *Handler) ListGames(c *echo.Context) error` — note `*echo.Context` (pointer) in v5; middleware is `func(echo.HandlerFunc) echo.HandlerFunc`
- Use `*bun.DB` for DB access; pass via dependency injection

**TypeScript (Frontend)**
- Same conventions as nexorious: external → internal (`@/...`) → types import order
- TanStack Query for server state; `useState` for local state only
- `routeTree.gen.ts` is gitignored — run `npm run build` once in a fresh worktree before type-checking

### Quality Gates
- Zero Go build errors and zero `golangci-lint` errors before committing
- Zero TypeScript errors (`npm run check`) before committing
- All tests must pass before committing

### Slumber Collection Maintenance
When adding a new API route, always add a corresponding request to `slumber.yaml`:
- Add it to the matching domain folder (e.g. a new `GET /api/games` goes in a `games/` folder)
- If the route requires JWT, add the `authentication: type: bearer` block with `"{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"`
- If it's a new domain with no existing folder, add new domain folders in alphabetical order; `bootstrap/` always stays first as the workflow anchor
- Use profile variables (`{{base_url}}`) for all URLs — never hardcode `localhost:8000`
- Run `slumber collection` to verify the collection loads without errors after any change

## Known Gotchas

- **`sql.ErrNoRows` vs DB errors** — always `errors.Is(err, sql.ErrNoRows)` to distinguish "not found" (→ 404/401) from real connection failures (→ 500); import `"database/sql"` for the sentinel. Bun wraps pgx errors into `sql.ErrNoRows`, so use the stdlib sentinel (not `pgx.ErrNoRows`)
- **`//go:embed all:dir`** — use `all:` prefix when the directory contains dot-files (e.g. `.gitkeep`); without it, Go silently excludes them and the build fails
- **Package name `migrate`** — `internal/migrate` uses package name `migrate`; when importing `uptrace/bun/migrate` inside that package, alias it: `bunmigrate "github.com/uptrace/bun/migrate"` to avoid the collision
- **`os.Exit` skips deferred calls** — call `pool.Close()` explicitly before any `os.Exit` in main; deferred `pool.Close()` will not run
- **Background goroutines** — use `context.Background()`, not `c.Request().Context()`, for work that outlives an HTTP handler
- **`errcheck` linter and `resp.Body`** — always `defer func() { _ = resp.Body.Close() }()`; bare `defer resp.Body.Close()` is flagged by errcheck
- **Priority type mismatch** — `jobs.priority` is TEXT (`'high'`/`'low'`); `pending_tasks.priority` is INTEGER; don't conflate the two columns
- **Echo v5 route order** — register static routes before parameterised ones (e.g. `GET /sync/steam/status` before `GET /sync/:id`); Echo v5 doesn't auto-sort and will match the wrong handler otherwise
- **Service package import cycles** — if `internal/api` imports `internal/services/steam` and vice-versa, break the cycle by having each service package define its own local summary types; `router.go` bridges them with adapter structs that satisfy the handler's interface
- **Platform/storefront `icon` field** — DB stores bare filename (e.g. `steam-icon-light.svg`); API responses must construct `icon_url` as `/logos/storefronts/<name>/<filename>` (or `platforms/`). Logos are bundled in the SPA dist and served at `/logos/...` — not `/static/logos/`.
- **River queue is independent of `job_items`** — `UPDATE job_items SET status='pending'` does NOT re-enqueue the item. River only processes rows in `river_job`. Always use `POST /api/jobs/{id}/retry-failed` or `POST /api/job-items/{id}/retry` which call `retryInsert` and write both tables. A direct DB reset leaves the item permanently stuck.
- **`ui/frontend/src/api/index.ts` barrel file** — new functions in `api/*.ts` are not automatically importable as `@/api`. Add them explicitly to the matching `export { ... } from './jobs'` block in `index.ts`.
- **bun raw scan struct tags** — when scanning raw SQL into a struct, bun maps columns by snake_casing the field name. If the column alias doesn't match exactly (e.g. `is_new_addition` → field `IsNewAdd` → bun expects `is_new_add`), the scan silently returns nil rows. Use explicit `bun:"column_name"` tags on all fields in raw-query result structs.
- **psql dev connection** — `psql "${DATABASE_URL/\/.s.PGSQL.5432/}"` — `DATABASE_URL` includes the full socket path; this substitution strips the socket filename so psql gets just the directory as `host`.
- **`pending_review` is settled, not active** — in `syncCheckJobCompletion` and similar job-completion checks, only `pending` and `processing` count as active remaining work. `pending_review` items wait indefinitely for user action and must not block auto-retry or job termination logic.


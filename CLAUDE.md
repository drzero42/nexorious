# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Code Exploration Policy

Always use jCodemunch-MCP tools — never fall back to Read, Grep, Glob, or Bash for code exploration.
- Before reading a file: use `get_file_outline` or `get_file_content`
- Before searching: use `search_symbols` or `search_text`
- Before exploring structure: use `get_file_tree` or `get_repo_outline`
- **Session start**: Always run `mcp__jcodemunch__index_folder` on the project root and `mcp__jdocmunch__index_local` on `docs/` at the start of every session (use `incremental: true`). Call `list_repos` first to check if already indexed.
- **Scope**: jCodemunch is for code (Go, TypeScript). For markdown/docs, use jdocmunch (`local/docs` repo; verify with `mcp__jdocmunch__list_repos`).

## Quick Reference

### Common Commands

| Task                     | Command                                                  |
|--------------------------|----------------------------------------------------------|
| Enter dev shell          | `devenv shell`                                           |
| Build backend            | `make build`                                             |
| Build frontend           | `make frontend`                                          |
| Build everything         | `make`                                                   |
| Run server               | `./nexorious`                                            |
| Run tests (Go)           | `go test ./...`                                          |
| Run single test          | `go test ./internal/api/... -run TestGamesList -v`       |
| Run tests with coverage  | `go test -cover ./...`                                   |
| Generate sqlc code       | `make sqlc`                                              |
| Type check (frontend)    | `npm run check`  (from `ui/`)                            |
| Run frontend tests       | `npm run test`   (from `ui/`)                            |
| Lint Go                  | `golangci-lint run`                                      |

### Environment Validation
```bash
devenv shell
go version   # expect go 1.23+
sqlc version
make --version
```

## Setup & Development

### Development Environment
Uses devenv for a reproducible shell (Go 1.25, sqlc, golangci-lint, make, Node 24, TypeScript):
```bash
devenv shell
```

### Build
```bash
make             # frontend → sqlc generate → go build
make frontend    # builds React SPA into ui/dist/
make sqlc        # regenerates internal/db/gen/ from SQL queries
make build       # compiles the Go binary
```

`ui/dist/` is gitignored and must be populated by `make frontend` before `go build`. The Go binary embeds `ui/dist` and `ui/migrate` via `//go:embed`.

`internal/db/gen/` is committed — run `make sqlc` only when query files under `internal/db/queries/` change.

### Initial Setup
```bash
devenv shell
make                     # builds everything
export DATABASE_URL="postgres://..."
./nexorious              # starts server; visits /migrate if schema is pending
```

## Project Structure

- `cmd/nexorious/` — entry point, wires config/db/echo/workers
- `internal/api/` — Echo route handlers per domain (games, user_games, auth, platforms, tags, jobs, import, export, backup, sync, status)
- `internal/db/` — database layer
  - `migrations/` — golang-migrate SQL files (`0001_initial.up.sql`, etc.)
  - `queries/` — sqlc input SQL (hand-edited)
  - `gen/` — sqlc output (generated, committed, never hand-edited)
- `internal/migrate/` — migration state machine + Echo handlers for `/migrate` and `/api/migrate/*`
- `internal/worker/` — goroutine pool with buffered `chan TaskFunc`; tasks under `worker/tasks/`
- `internal/scheduler/` — gocron v2 job definitions (scheduled maintenance)
- `internal/services/` — IGDB client, Steam/PSN/Epic sync, cover art storage, game matching, platform resolution
- `internal/auth/` — JWT generation/validation + Echo middleware
- `internal/filter/` — dynamic query builder (goqu + sqlx) for user-game list filtering
- `internal/ratelimit/` — interface + local (`x/time/rate`) and PostgreSQL implementations
- `internal/config/` — config struct via `caarlos0/env`
- `ui/` — React SPA source (`ui/src/`) + build output (`ui/dist/`) + migration HTML (`ui/migrate/`)

## Architecture

### Startup & App State Machine
```
NeedsMigration → Migrating → Ready
```
Echo middleware blocks all non-migration routes until state is `Ready`. Workers and the gocron scheduler start only after migrations complete. Graceful shutdown drains the worker queue on `SIGTERM`/`SIGINT`.

### Database Layer
- **Static queries**: `sqlc` generates type-safe Go from SQL in `internal/db/queries/`. Never edit `internal/db/gen/` by hand.
- **Dynamic filter queries**: `goqu/v9` + `sqlx` used in `internal/filter/` for user-game list filtering (JOINs, WHERE, HAVING accumulate via `filterBuilder`).
- **Driver**: `pgx/v5` with `pgxpool`.
- **Migrations**: `golang-migrate/v4`; migration SQL lives in `internal/db/migrations/`.

### Frontend Embedding (Stash pattern)
`ui/ui.go` exposes two `embed.FS` vars:
```go
//go:embed dist
var UIBox embed.FS      // main React SPA

//go:embed migrate
var MigrateBox embed.FS // standalone migration UI
```
FastAPI is eliminated; the Go binary serves the React SPA itself.

### Frontend Stack (unchanged from nexorious)
- Vite 6 + React 19 + TypeScript
- TanStack Router (file-based routes in `ui/src/routes/`)
- TanStack Query, Tailwind CSS v4, shadcn/ui, React Hook Form + Zod, TipTap
- Vitest + @testing-library/react

### Route Zones
- **Migration zone** (`/migrate`, `/api/migrate/*`) — always available, bypasses state middleware
- **Setup zone** (`/api/auth/setup/*`) — requires `Ready` state, no JWT (no users exist yet)
- **API zone** — gated by state middleware, then JWT where required

### Workers & Scheduler
Workers are goroutines reading from a buffered channel (`worker/pool.go`). Task types live under `worker/tasks/`: sync, import, export, maintenance (backup, metadata refresh, cleanup). The gocron scheduler dispatches recurring tasks after `Ready`.

### Rate Limiting
`ratelimit.Limiter` interface with two implementations:
- `local.go` — `golang.org/x/time/rate` (single instance)
- `postgres.go` — PostgreSQL `SELECT FOR UPDATE` (multi-instance, opt-in via config)

## Testing

- **Framework**: stdlib `testing` + `testcontainers-go` (spins up real PostgreSQL containers)
- Run all: `go test ./...`
- Run single: `go test ./internal/api/... -run TestFunctionName -v`
- Frontend: from `ui/` — `npm run test`, single file: `npm run test game-card.test.tsx`

## Development Rules

> **Always ask questions if you are uncertain about something!**

### Essential Workflow
1. **Planning**: Read `docs/superpowers/specs/` for design context
2. **Branching**: Create a feature branch before starting any task
3. **sqlc**: Run `make sqlc` after editing query files; commit the generated `internal/db/gen/` output
4. **Migrations**: Add new `.up.sql` / `.down.sql` files in `internal/db/migrations/`; never hand-edit generated code
5. **Testing**: Run `go test ./...` after any Go changes; `npm run check && npm run test` after any frontend changes

### Branch Workflow (MANDATORY)
- ✅ Always create a branch before starting task work
- ✅ Use `--squash --delete-branch` when merging PRs
- ❌ Never commit directly to main
- ❌ Never merge a PR on your own initiative — only when the user explicitly instructs

### Code Style

**Go (Backend)**
- Standard Go conventions: `camelCase` unexported, `PascalCase` exported, `UPPER_CASE` constants
- Errors returned, not panicked; wrap with `fmt.Errorf("context: %w", err)`
- Echo handler signature: `func (h *Handler) ListGames(c echo.Context) error`
- Use `pgxpool` for DB; pass `*db.Queries` via dependency injection

**TypeScript (Frontend)**
- Same conventions as nexorious: external → internal (`@/...`) → types import order
- TanStack Query for server state; `useState` for local state only
- `routeTree.gen.ts` is gitignored — run `npm run build` once in a fresh worktree before type-checking

### Quality Gates
- Zero Go build errors and zero `golangci-lint` errors before committing
- Zero TypeScript errors (`npm run check`) before committing
- All tests must pass before committing

## Known Gotchas

- **`//go:embed all:dir`** — use `all:` prefix when the directory contains dot-files (e.g. `.gitkeep`); without it, Go silently excludes them and the build fails
- **golang-migrate driver** — use blank import `_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"` + `gmigrate.NewWithSourceInstance("iofs", src, databaseURL)`; no `pgx5driver.Open()` exists
- **Package name `migrate`** — collides with the golang-migrate import; alias it: `gmigrate "github.com/golang-migrate/migrate/v4"`
- **`iofs.Source.Next(ver)`** — returns `(uint, error)`, not 3 values
- **`os.Exit` skips deferred calls** — call `pool.Close()` explicitly before any `os.Exit` in main; deferred `pool.Close()` will not run
- **Background goroutines** — use `context.Background()`, not `c.Request().Context()`, for work that outlives an HTTP handler

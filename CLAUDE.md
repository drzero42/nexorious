# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Code Exploration Policy

Always use jCodemunch-MCP tools — never fall back to Read, Grep, Glob, or Bash for code exploration.
- Before reading a file: use `get_file_outline` or `get_file_content`
- Before searching: use `search_symbols` or `search_text`
- Before exploring structure: use `get_file_tree` or `get_repo_outline`
- **Session start**: Always run `mcp__jcodemunch__index_folder` on the project root at the start of every session before searching — the index can be stale if code changed since the last session. Use `incremental: true` (the default) so it's fast.
- Call `list_repos` first; if the project is not indexed, call `index_folder` with the current directory.
- **Scope**: jCodemunch is for code only (Go, TypeScript, etc.). For markdown and documentation files, use jdocmunch MCP (`local/docs` repo; call `mcp__jdocmunch__list_repos` first to verify it's indexed — if not, run `mcp__jdocmunch__index_local` on `docs/`).
- **Session start**: Always run `mcp__jdocmunch__index_local` on `docs/` at the start of every session before searching — the index can be stale if docs changed since the last session. Use `incremental: true` (the default) so it's fast.

## Quick Reference

### Common Commands

| Task                     | Command                                                  |
|--------------------------|----------------------------------------------------------|
| Enter dev shell          | `devenv shell`                                           |
| Build backend            | `make build`                                             |
| Build frontend           | `make frontend`                                          |
| Build everything         | `make`                                                   |
| Run server               | `./nexorious`                                            |
| Run tests (Go)           | `go test -timeout 300s ./...`                            |
| Run single test          | `go test ./internal/api/... -run TestGamesList -v`       |
| Run tests with coverage  | `go test -cover ./...`                                   |
| Type check (frontend)    | `npm run check`  (from `ui/frontend/`)                   |
| Run frontend tests       | `npm run test`   (from `ui/frontend/`)                   |
| Lint Go                  | `golangci-lint run`                                      |
| Run API client           | `slumber`                                                |

### Environment Validation
```bash
devenv shell
go version   # expect go 1.25+
make --version
```

## Setup & Development

### Development Environment
Uses devenv for a reproducible shell (Go 1.25, golangci-lint, make, Node 24, TypeScript):
```bash
devenv shell
```

### Build
```bash
make             # frontend → go build
make frontend    # builds React SPA into ui/frontend/dist/
make build       # compiles the Go binary
```

`ui/frontend/dist/` is gitignored and must be populated by `make frontend` before `go build`. The Go binary embeds `ui/frontend/dist` and `ui/migrate` via `//go:embed`.

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
  - `migrations/` — golang-migrate SQL files (`0001_initial.up.sql`, etc.)
  - `models/` — Bun model structs (hand-edited)
- `internal/migrate/` — migration state machine + Echo handlers for `/migrate` and `/api/migrate/*`
- `internal/worker/` — goroutine pool with buffered `chan TaskFunc`; tasks under `worker/tasks/`
- `internal/scheduler/` — gocron v2 job definitions (scheduled maintenance)
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
NeedsMigration → Migrating → Ready
```
Echo middleware blocks all non-migration routes until state is `Ready`. Workers and the gocron scheduler start only after migrations complete. Graceful shutdown drains the worker queue on `SIGTERM`/`SIGINT`.

### Database Layer
- **ORM**: Bun (`uptrace/bun`) with model structs in `internal/db/models/`. Queries use Bun's query builder.
- **Exception**: `internal/auth` uses raw `db.NewRaw`/`db.QueryRow` directly (not Bun models) to keep auth isolated.
- **Dynamic filter queries**: Bun query builder used in `internal/filter/` for user-game list filtering.
- **Driver**: `pgx/v5` via Bun's `pgdriver` adapter (`bunpgx`).
- **Migrations**: `golang-migrate/v4`; migration SQL lives in `internal/db/migrations/`.

### Frontend Embedding (Stash pattern)
`ui/ui.go` exposes two `embed.FS` vars:
```go
//go:embed frontend/dist
var UIBox embed.FS      // main React SPA

//go:embed migrate
var MigrateBox embed.FS // standalone migration UI
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
Workers are goroutines reading from a buffered channel (`worker/pool.go`). Task types live under `worker/tasks/`: `sync.go`, `import_item.go`, `export.go`, `metadata_refresh.go`. Backup logic lives in `internal/backup/` (invoked from the scheduler, not as a worker task). The gocron scheduler dispatches recurring tasks after `Ready`.

### Rate Limiting
`ratelimit.Limiter` interface with two implementations:
- `local.go` — `golang.org/x/time/rate` (single instance)
- `postgres.go` — PostgreSQL `SELECT FOR UPDATE`` (multi-instance, opt-in via config)

## Testing

- **Framework**: stdlib `testing` + `testcontainers-go` (spins up real PostgreSQL containers)
- Run all: `go test ./...`
- Run single: `go test ./internal/api/... -run TestFunctionName -v`
- Frontend: from `ui/frontend/` — `npm run test`, single file: `npm run test game-card.test.tsx`

## Development Rules

> **Always ask questions if you are uncertain about something!**

### Essential Workflow
1. **Planning**: Read `docs/superpowers/specs/` for design context
2. **Branching**: Create a feature branch before starting any task
3. **Migrations**: Add new `.up.sql` / `.down.sql` files in `internal/db/migrations/`; never hand-edit generated code
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
- **golang-migrate driver** — use blank import `_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"` + `gmigrate.NewWithSourceInstance("iofs", src, databaseURL)`; no `pgx5driver.Open()` exists. Connection string must use `pgx5://` scheme: `"pgx5" + strings.TrimPrefix(connStr, "postgres")`
- **Package name `migrate`** — collides with the golang-migrate import; alias it: `gmigrate "github.com/golang-migrate/migrate/v4"`
- **`iofs.Source.Next(ver)`** — returns `(uint, error)`, not 3 values
- **`os.Exit` skips deferred calls** — call `pool.Close()` explicitly before any `os.Exit` in main; deferred `pool.Close()` will not run
- **Background goroutines** — use `context.Background()`, not `c.Request().Context()`, for work that outlives an HTTP handler
- **`errcheck` linter and `resp.Body`** — always `defer func() { _ = resp.Body.Close() }()`; bare `defer resp.Body.Close()` is flagged by errcheck
- **Priority type mismatch** — `jobs.priority` is TEXT (`'high'`/`'low'`); `pending_tasks.priority` is INTEGER; don't conflate the two columns
- **Echo v5 route order** — register static routes before parameterised ones (e.g. `GET /sync/steam/status` before `GET /sync/:id`); Echo v5 doesn't auto-sort and will match the wrong handler otherwise
- **Service package import cycles** — if `internal/api` imports `internal/services/steam` and vice-versa, break the cycle by having each service package define its own local summary types; `router.go` bridges them with adapter structs that satisfy the handler's interface
- **Platform/storefront `icon` field** — DB stores bare filename (e.g. `steam-icon-light.svg`); API responses must construct `icon_url` as `/logos/storefronts/<name>/<filename>` (or `platforms/`). Logos are bundled in the SPA dist and served at `/logos/...` — not `/static/logos/`.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:7510c1e2 -->
## Task Tracking

This project uses **bd (beads)** for persistent issue and task tracking across sessions.
Run `bd prime` to see full workflow context and commands.

### Workflow

Use the full pipeline for any non-trivial feature work:

```
brainstorming -> design spec -> superpowers:writing-plans -> plan-to-epic -> epic-executor
```

For small changes (single file, few lines): implement directly without the pipeline.

### Skill Routing

- **Never use `superpowers:brainstorming` directly.** Always use the plain `brainstorming` skill instead — it is an extended version that chains into the beads pipeline (design doc → plan → epic → execution).

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- **Always pass `--json`** to all `bd` commands for reliable parsing
- **Always include `--description`** when creating issues — context matters across sessions
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files
- **Do not create `docs/plans/` markdown task files** — that's what beads is for.
  Design docs and implementation plans written by `superpowers:writing-plans` are fine;
  do not create additional freestanding task-tracking markdown.

### Beads File Handling

Beads manages its own files — do not second-guess what it stages or commits:

- **`issues.jsonl`** — auto-exported and auto-staged by beads after every change; include it in commits alongside your code changes
- **`interactions.jsonl`** — audit log written by beads; include it in commits alongside your code changes
- **`embeddeddolt/`** — gitignored by beads; never touch it, it syncs via `bd dolt push` / `bd dolt pull`

When beads files appear staged, commit them. Do not unstage them.

### Resuming Work

To resume an in-progress epic:

```
continue epic <epic-id>
```

The epic-executor will check `bd epic status`, find the next ready task, and continue without re-explanation.

### Key Commands

```bash
bd prime                                      # Inject workflow context (runs automatically on session start)
bd ready --json                               # Show tasks with no blockers
bd epic status <epic-id> --json               # Show epic completion %
bd list --status open --json                  # List all open issues
bd show <task-id> --json                      # Show task details
bd update <task-id> --claim --json            # Claim a task
bd close <task-id> --reason "..." --json      # Close a task
bd blocked --json                             # Show what's blocked and why
bd dolt push                                  # Sync Dolt state to remote
bd dolt pull                                  # Sync Dolt state from remote
```

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   bd dolt push
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
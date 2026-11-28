# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Reference

### Common Commands

| Task                     | Command                                                  |
|--------------------------|----------------------------------------------------------|
| Run cmd in dev shell     | `devenv shell -- <command>` (only if truly necessary)    |
| Build backend            | `make build`                                             |
| Build frontend           | `make frontend`                                          |
| Build everything         | `make`                                                   |
| Run server               | `./nexorious serve`                                      |
| Run migrations           | `./nexorious migrate`                                    |
| Migration status         | `./nexorious migrate status`                             |
| Run tests (Go)           | `go test -timeout 600s ./...`                            |
| Run single test          | `go test ./internal/api/... -run TestGamesList -v`       |
| Run tests with coverage  | `go test -timeout 600s -cover ./...`                     |
| Type check (frontend)    | `npm run check`  (from `ui/frontend/`)                   |
| Dead-code check (frontend)| `npm run knip`   (from `ui/frontend/`)                   |
| Run frontend tests       | `npm run test`   (from `ui/frontend/`)                   |
| Run one frontend test    | `npm run test game-card.test.tsx` (from `ui/frontend/`)  |
| Lint Go                  | `golangci-lint run`                                      |

### Environment Validation
```bash
go version   # expect go 1.27+
make --version
```

## Setup & Development

### Development Environment
Uses devenv for a reproducible shell (Go 1.27, golangci-lint, make, Node 26, TypeScript).

> **Run commands directly** — `go`, `make`, `npm`, etc. are all on PATH in the devenv-activated shell. Only use `devenv shell -- <command>` when a tool is genuinely not available in the current environment. The separator is `--` (never `devenv shell --command`).

### Build
```bash
make             # frontend → go build
make frontend    # builds React SPA into ui/frontend/dist/
make build       # compiles the Go binary
```

`ui/frontend/dist/` is gitignored and must be populated by `make frontend` before `go build`; the binary embeds it and the other UI dirs (`ui/ui.go`).

### Initial Setup
```bash
devenv shell
make                     # builds everything
export DATABASE_URL="postgres://..."
export DB_ENCRYPTION_KEY="<random-secret>"  # required; generate: openssl rand -base64 32
# Optional: IGDB_CLIENT_ID + IGDB_CLIENT_SECRET for metadata enrichment
# Optional: PORT (default 8000), LOG_LEVEL (default info), WORKER_COUNT (default 4)
./nexorious serve        # starts server; visits /migrate if schema is pending
```

## Project Structure

One line per package: what it owns and the rule that guards it. Where a package has a doc comment (`go doc ./internal/<pkg>`), the details live there; read it before changing the package. A new package gets one line here, a new rule one sentence — never a list of its functions.

- `cmd/nexorious/` — the server binary, with the commands that need direct DB access (e.g. `migrate`, `ensure-db-role`).
- `cmd/nexctl/` — the REST client binary; its command reference is the package doc (`cmd/nexctl/doc.go`). It talks to the server only over REST (`cliclient`), puts operations shared with MCP in `internal/clientops`, and never links the MCP SDK or `internal/mcptools` (`TestNexctlDoesNotLinkMCPSDK`).
- `internal/clientops/` — the one implementation of operations both nexctl and MCP run. Never prompts, imports neither cobra nor the MCP SDK.
- `internal/mcptools/` — the MCP tools at `/api/mcp`, mirroring nexctl's game/pool/tag/sync/doctor trees on `clientops`. Every tool declares its scope in `toolScopes` (`scope.go`), or `addTool` panics.
- `internal/cliui/`, `internal/cliauth/` — nexctl's terminal helpers, and the login/setup/migrate orchestration.
- `internal/api/` — Echo route handlers per domain.
- `internal/db/` — `migrations/` (SQL, see [Migrations](#essential-workflow)) and `models/` (hand-edited Bun structs).
- `internal/migrate/` — migration state machine and the `/migrate` handlers.
- `internal/joblifecycle/` — the **only** writer of `jobs.status`, `job_items.status`, `dispatch_complete` and `river_job_id`, and the creator of Jobs, items and their River jobs (spec: `docs/job-lifecycle.md`). Imports neither `internal/worker/tasks` nor `internal/api`.
- `internal/worker/tasks/` — River workers (sync, import, export, metadata refresh); kinds register with the lifecycle in `tasks.RegisterLifecycleKinds`.
- `internal/scheduler/` — periodic River jobs (`BuildPeriodicJobs`).
- `internal/services/` — IGDB client, storefront clients, game matching, platform resolution.
- `internal/usergame/` — the **only** place user games are mutated: API, sync and import all call its operations.
- `internal/externalgame/` — the **only** non-test code that writes `external_games` / `external_game_platforms` or row-locks them, and the one definition of editions (ADR 0008, ADR 0009). Imports neither `internal/joblifecycle`, `internal/worker/tasks` nor `internal/api`; `guard_test.go` enforces the write and import rules, not the locks.
- `internal/storefront/` — owns a storefront Connection (`user_sync_configs.storefront_credentials`): its stored shape, connecting and disconnecting it, storing and loading it, and building the library adapter from it (ADR 0010). Read and write the stored sign-in through it. Imports neither `internal/api`, `internal/worker/tasks` nor `internal/joblifecycle`.
- `internal/librarysmells/` — library health: the Checks and their fixes, which go through `internal/usergame`.
- `internal/auth/` — session + API-key auth. A read-scoped key may only use non-mutating methods; the exemptions are the lists in `scope.go`.
- `internal/dbrole/` — keeps the app's DB role unprivileged.
- `internal/filter/` — Bun query builder for user-game list filters.
- `internal/ratelimit/` — limiter interface: local (`x/time/rate`) and PostgreSQL (`SELECT FOR UPDATE`, multi-instance, opt-in).
- `internal/config/`, `internal/enum/`, `internal/middleware/`, `internal/backup/` — env config, shared enums, Echo middleware, backup orchestration.
- `ui/` — `frontend/` (React SPA), plus the standalone `migrate/`, `db-error/`, `setup/` pages.
- `docs/` — guides and reference docs. Only `user-guide.md` and `admin-guide.md` are embedded and served in-app (`/help/:slug`).

## Architecture

### Startup & App State Machine
```
DBUnavailable ↔ NeedsMigration → Migrating → Ready
```
Echo middleware blocks all non-migration routes until state is `Ready`. River workers and the periodic scheduler start only after migrations complete. Graceful shutdown waits for in-flight River jobs on `SIGTERM`/`SIGINT`.

### Database Layer
- **ORM**: Bun (`uptrace/bun`) with the `pgdriver` driver; queries use Bun's query builder.
- **Exception**: `internal/auth` uses raw `db.NewRaw`/`db.QueryRow` (not Bun models) to keep auth isolated.

### Frontend Stack
- Vite 8 + React 19 + TypeScript
- TanStack Router (file-based routes in `ui/frontend/src/routes/`)
- TanStack Query, Tailwind CSS v4, shadcn/ui, React Hook Form + Zod, TipTap
- Vitest + @testing-library/react

### Route Zones
- **Migration zone** (`/migrate`, `/api/migrate/*`) — always available, bypasses state middleware
- **Setup zone** (`/api/auth/setup/*`) — requires `Ready` state, no auth (no users exist yet)
- **API zone** — gated by state middleware, then `AuthMiddleware` where required
  - `/api/mcp` — Bearer API key only (`APIKeyMiddleware`, never the session cookie), behind an Origin check (`Origin` host must equal `Host` or be in `CORS_ORIGINS`). Not registered at all when `MCP_ENABLED=false`.

## Testing

### Policy

Write a test when:
- The behaviour is security-sensitive (auth, token validation, permission checks)
- There are multiple meaningful edge cases (missing fields, wrong types, not found, conflict)
- The logic is non-obvious or involves a subtle invariant
- A real bug was found — the test documents that it cannot regress

Skip a test when:
- The function is a thin wrapper or a struct field accessor
- The test only verifies that calling the function returns what it computes (tautology)
- The only assertion is "no panic on happy path" with no behavioural verification
- Coverage percentage is the motivation

There is no coverage gate in CI. The quality gate is: does the PR touching non-trivial logic include a test that would have caught a plausible bug in that logic?

### Shared database

Each package that needs a real database uses a shared PostgreSQL container via `TestMain` (`testcontainers-go`): it starts once per package per `go test` run, with migrations applied once. Each test calls `truncateAllTables(t)` at the top for isolation and uses the shared `testDB` package variable — a per-test helper that starts its own container is too slow.

## Development Rules

> **Always ask questions if you are uncertain about something!**

### Essential Workflow
1. **Planning**: Design decisions and plans are recorded in GitHub issues (and their epics), not in repo docs — read the issue and its epic before starting; the landed code is the authority. Domain terms live in `GLOSSARY.md`.
2. **Working on an issue**: When asked to work on an issue ("work on #825"), read it with `gh issue view <n> --comments` — later comments often correct the original — and treat every claim in it (diagnosis, proposed fix, file/function references) as a hypothesis to verify against the code; tell the user when the issue is wrong or stale instead of implementing against a false premise. Then present what you found and the change you intend, and **wait for explicit approval before making any change** — creating the branch and writing the first test count as changes. Verified ≠ approved; an explicit "just fix it" in the session overrides this.
3. **Branching**: Create a feature branch before starting any task.
4. **Migrations**: Add a new `.up.sql` / `.down.sql` pair in `internal/db/migrations/` named `YYYYMMDD<nnnnnn>_name.up.sql`, where `<nnnnnn>` is a zero-padded running number (e.g. `20260503000001_name.up.sql`); Bun discovers them via `Migrations.Discover(FS)`.
5. **Testing**: The mechanical gates run automatically via hooks (see [Automated Checks](#automated-checks)). Run targeted tests (e.g. `go test ./internal/api/... -run TestX -v`) for the logic you're actively changing.
6. **Dead code**: After a Go change that **removes or renames a caller, deletes a code path, or refactors across package boundaries**, run `make deadcode` and reconcile any *new* entries against your diff — `staticcheck`'s `unused` only flags unexported symbols, so a newly-orphaned **exported** method slips past CI. Pure additions don't need it. The tool can't see reflection / `//go:embed` / interface-dispatch-only calls, so verify before deleting.
7. **Issues** you write describe behaviour and rules; they never ask for an `AGENTS.md` inventory update. A new package's doc comment carries its details.

### Out-of-Scope Findings
When working with code, if you identify a problem, bug, or inconsistency that you judge to be out-of-scope for the current task, surface it and **offer to file a follow-up GitHub issue** (`gh issue create`) rather than fixing or ignoring it silently. Only create the issue once the user agrees.

Label an issue at creation (`gh issue create --label ...`), picking from `gh label list` (e.g. `documentation`, `enhancement`, `bug`, `tech-debt`, `security`, `observability`) — a docs feature request is `documentation` + `enhancement`.

**Cross-references in issue/epic bodies go stale.** Epic bodies list "child/related issues" in hand-written prose that does not update when a child closes or is re-scoped. Before acting on a referenced issue, verify its live state (`gh issue view <n> --json state`) and treat its current body as the authority; `gh issue view` returns a body for closed issues too.

### Branch Workflow (MANDATORY)
- ✅ Always create a branch before starting task work.
- ✅ **Every change lands through a PR.** `main` is protected by a ruleset with no bypass (require PR, squash-only); a direct push fails with `GH013`. This covers docs, CI workflows and one-line fixes alike.
- ✅ Merge with `--squash --delete-branch`, and only when the user explicitly says to.
- ✅ Put a closing keyword before **each** issue number in the PR body (`Closes #846, closes #847`).
- ✅ Keep the required `CI Gate` job in `.github/workflows/test.yaml` green and reporting when editing that file.

What `gh pr merge` does to the local checkout, the closing-keyword details and the `CI Gate` wiring: [docs/agents/pull-requests.md](docs/agents/pull-requests.md).

Renovate PRs (grouping, post-upgrade scripts, merging a major and a group PR of the same ecosystem): [docs/agents/renovate.md](docs/agents/renovate.md).

### Code Style

**Go (Backend)**
- Standard Go conventions: `camelCase` unexported, `PascalCase` exported, `UPPER_CASE` constants
- Errors returned, not panicked; wrap with `fmt.Errorf("context: %w", err)`
- Echo handler signature: `func (h *Handler) ListGames(c *echo.Context) error` — note `*echo.Context` (pointer) in v5; middleware is `func(echo.HandlerFunc) echo.HandlerFunc`
- Use `*bun.DB` for DB access; pass via dependency injection
- **Logging:** in request/job code use `slog.*Context(ctx, …)` (not bare `slog.*`) and the `internal/logging` `Key*` constants; set `logging.Cat(...)` on error/warn boundaries. Correlation ids (`request_id`/`job_id`/`river_job_id`/`user_id`) are injected automatically from `ctx` — never add them by hand. Never log secrets or bodies. Full conventions: [docs/logging-conventions.md](docs/logging-conventions.md).

**TypeScript (Frontend)**
- Import order: external → internal (`@/...`) → types
- TanStack Query for server state; `useState` for local state only
- `routeTree.gen.ts` is generated by the TanStack Router Vite plugin and committed; never edit it by hand. After adding, moving, renaming or removing a route, run `npm run build` and commit the regenerated file with the route edit (the `Route tree is up to date` CI step fails on any difference). If a build dirties it while you haven't touched `src/routes/`, the committed copy is stale: leave it out of your commit, keep the change, and tell the user so it lands in its own `chore(frontend): regenerate route tree` PR.
- Stage files explicitly, not with `git add -A`.
- shadcn/ui components in `ui/frontend/src/components/ui/` follow an "include and prune" policy — knip removes any unused component or sub-export. If a feature needs a missing one, re-add it via `npx shadcn@latest add <component>`.
- **Responsive card-vs-table swaps** use the `useMediaQuery` hook (`@/hooks`) and render a single layout tree (`isMobile ? <cards> : <table>`), so the inactive layout isn't mounted and jsdom tests (which ignore CSS) don't see duplicated nodes. Examples: `game-list.tsx` and `admin/users/index.tsx`. CSS-only `hidden lg:block` toggles are for purely *presentational* tweaks (sidebars, info boxes, icon-only buttons, grid column counts).

### Automated Checks

Hooks enforce the quality bar (zero Go build/`golangci-lint` errors, zero `npm run check`/knip findings, all tests passing), so you rarely run the gates by hand. **Launch Claude Code from inside `devenv shell`** (or an active direnv); without `go`, `golangci-lint`, `node` and `jq` on `PATH` the Claude Code hooks silently no-op.

- **Each edit** (`.claude/hooks/post-edit.sh`): `gofmt` + `golangci-lint` on the Go package, or `prettier` + `eslint --fix` on the frontend file. Fix what it reports.
- **Turn end** (`.claude/hooks/stop-check.sh`): `go build ./...` / `tsc --noEmit` for dirty code — a nudge, not the gate.
- **`git push`** (devenv pre-push): the full Go and frontend suites — the hard gate. `git push --no-verify` bypasses it in a pinch.

### Nix

After any `go.mod` / `go.sum` change, run `./nix/update-go-vendor-hash.sh` and commit the updated `goVendorHash` in `flake.nix`; the `Nix Build` workflow fails on a stale hash. Details: [docs/agents/nix.md](docs/agents/nix.md).

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues on `drzero42/nexorious` via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one root `GLOSSARY.md` + `docs/adr/` (created lazily). See `docs/agents/domain.md`.

## Release Process (MANDATORY commit convention)

release-please cuts releases from the Conventional Commits on `main`. PRs are squash-merged as `<PR title> (#N)` with no body, so the **PR title** is the commit message: write it as a Conventional Commit (`feat:` minor, `fix:` patch, `feat!:` major; `chore:`/`ci:`/`docs:`/`refactor:`/`test:` no release). The PR description never reaches the commit.

The literal override start token belongs in a PR description only when you mean to override the release (release-please splits on it even inside prose or backticks); a `Release-As:` / `BREAKING CHANGE:` line outside an override block fails the `PR Commit Message` check.

Release notes and labels, cutting a release, overrides, skipping a release, 1.0.0: [docs/agents/releasing.md](docs/agents/releasing.md).

## Known Gotchas

- **`sql.ErrNoRows` vs DB errors** — always `errors.Is(err, sql.ErrNoRows)` to distinguish "not found" (→ 404/401) from real connection failures (→ 500); import `"database/sql"` for the sentinel. Bun wraps pgx errors into `sql.ErrNoRows`, so use the stdlib sentinel (not `pgx.ErrNoRows`)
- **`//go:embed all:dir`** — use `all:` prefix when the directory contains dot-files (e.g. `.gitkeep`); without it, Go silently excludes them and the build fails
- **Package name `migrate`** — `internal/migrate` uses package name `migrate`; when importing `uptrace/bun/migrate` inside that package, alias it: `bunmigrate "github.com/uptrace/bun/migrate"` to avoid the collision
- **`os.Exit` skips deferred calls** — call `pool.Close()` explicitly before any `os.Exit` in main; deferred `pool.Close()` will not run
- **Background goroutines** — use `context.Background()`, not `c.Request().Context()`, for work that outlives an HTTP handler
- **`errcheck` runs with `check-blank`** — every `_ =` error discard fails CI (`_test.go` exempt). **Handle** the error: an API handler logs and returns `echo.NewHTTPError(500, …)`, a worker logs. A discard that is genuinely fine is covered by the `std-error-handling` preset (`Close`/`Fprint`), the `(bun.Tx).Rollback` allowlist, or a per-site `//nolint:errcheck // <reason>`.
- **`gosec` is enabled** — findings fail CI (`_test.go` excluded). Fix genuine ones (tighten perms to `0600`/`0750`, guard user-derived paths against traversal); a confirmed false positive gets a per-site `//nolint:gosec // <why it's safe>`, never a config-wide exclusion.
- **`jobs` / `job_items` columns** — `jobs` has **no `updated_at`**; `job_items` has **no `error`** column (it's `error_message`). Check the migrations before naming a column.
- **Job and item status belong to `internal/joblifecycle`** — a raw `UPDATE` of `jobs.status`, `job_items.status`, `dispatch_complete` or `river_job_id` breaks ownership (an item can sit `pending` with no River job forever). Ask the module for a transition; `docs/job-lifecycle.md` defines when a Job waits on the user, completes or is recovered.
- **External games belong to `internal/externalgame`** — a write to `external_games` or `external_game_platforms` anywhere else fails the module's write guard test. Add an operation to the module instead; test fixtures write rows through `egtest`. Explicit row locks on these tables belong in the module too, but the guard does not check them (ADR 0009).
- **User-game mutations belong to `internal/usergame`** — hand-chaining platform inserts skips the invariant tail (wishlist, promotion, pools, tags). Call the module's operation.
- **Priority type mismatch** — `jobs.priority` is TEXT (`'high'`/`'low'`); `pending_tasks.priority` is INTEGER; keep the two columns apart
- **Echo v5 route order** — register static routes before parameterised ones (e.g. `GET /sync/steam/status` before `GET /sync/:id`); Echo v5 doesn't auto-sort and will match the wrong handler otherwise
- **Service package import cycles** — if `internal/api` imports `internal/services/steam` and vice-versa, break the cycle by having each service package define its own local summary types; `router.go` bridges them with adapter structs that satisfy the handler's interface
- **Platform/storefront `icon` field** — DB stores bare filename (e.g. `steam-icon-light.svg`); API responses must construct `icon_url` as `/logos/storefronts/<name>/<filename>` (or `platforms/`). Logos are bundled in the SPA dist and served at `/logos/...` — not `/static/logos/`.
- **`user_game_platforms.platform` / `.storefront` are FKs** to `platforms(name)` / `storefronts(name)` (`ON DELETE RESTRICT`). `platform` is NOT NULL; `storefront` is nullable (a row with no storefront is never matched by a `storefront = ?` filter). Tests inserting platform rows must use **seeded** names (e.g. platform `pc-windows`, storefront `steam`), not arbitrary strings like `pc`, or the insert fails with a foreign-key violation.
- **bun raw scan struct tags** — when scanning raw SQL into a struct, bun maps columns by snake_casing the field name. If the column alias doesn't match exactly (e.g. `is_new_addition` → field `IsNewAdd` → bun expects `is_new_add`), the scan silently returns nil rows. Use explicit `bun:"column_name"` tags on all fields in raw-query result structs.
- **MCP `jsonschema` struct tags** are bare description strings; the `description=…` form panics at `AddTool`.
- **In-app doc links** — `ui/frontend/src/lib/doc-links.ts` turns a relative link to a `docs/*.md` sibling into an in-app `/help/:slug` route, but only the two guides are embedded (`docs/embed.go`). Linking a guide to any other doc means embedding it too, or the link 404s.
- **psql dev connection** — `psql "${DATABASE_URL/\/.s.PGSQL.5432/}"` — `DATABASE_URL` includes the full socket path; this substitution strips the socket filename so psql gets just the directory as `host`.
- **Helm `values.schema.json` must stay in sync** — `additionalProperties: false` on `nexorious` means a field added to `values.yaml` under `nexorious:` must be registered in `deploy/helm/values.schema.json`, or `helm lint --strict` fails. A field with a `fail` guard on its placeholder default also needs `--set nexorious.<field>=x` in the lint step of `.github/workflows/test.yaml`; removing the field means removing that flag too.

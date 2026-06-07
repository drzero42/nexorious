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
| Run server               | `./nexorious serve`                                      |
| Run migrations           | `./nexorious migrate`                                    |
| Migration status         | `./nexorious migrate status`                             |
| Run tests (Go)           | `go test -timeout 600s ./...`                            |
| Run single test          | `go test ./internal/api/... -run TestGamesList -v`       |
| Run tests with coverage  | `go test -timeout 600s -cover ./...`                     |
| Type check (frontend)    | `npm run check`  (from `ui/frontend/`)                   |
| Dead-code check (frontend)| `npm run knip`   (from `ui/frontend/`)                   |
| Run frontend tests       | `npm run test`   (from `ui/frontend/`)                   |
| Lint Go                  | `golangci-lint run`                                      |

### Environment Validation
```bash
go version   # expect go 1.26+
make --version
```

## Setup & Development

### Development Environment
Uses devenv for a reproducible shell (Go 1.26, golangci-lint, make, Node 24, TypeScript).

> **Run commands directly** — `go`, `make`, `npm`, etc. are all on PATH in the devenv-activated shell. Only use `devenv shell -- <command>` when a tool is genuinely not available in the current environment. Never use `devenv shell --command`; the correct separator is `--`.

### Build
```bash
make             # frontend → go build
make frontend    # builds React SPA into ui/frontend/dist/
make build       # compiles the Go binary
```

`ui/frontend/dist/` is gitignored and must be populated by `make frontend` before `go build`. The Go binary embeds five UI dirs via `//go:embed`: `all:frontend/dist`, `all:migrate`, `db-error`, `setup`, and `all:shared` (see `ui/ui.go`).

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

- `cmd/nexorious/` — entry point, wires config/db/echo/workers
- `internal/api/` — Echo route handlers per domain (games, user_games, auth, setup, platforms, tags, jobs, job_items, import, export, backup, sync, db_error)
- `internal/db/` — database layer
  - `migrations/` — Bun migrate SQL files with timestamp-prefix naming (`20260503000001_name.up.sql` / `.down.sql`); auto-discovered via `//go:embed *.sql` in `migrations.go`
  - `models/` — Bun model structs (hand-edited)
- `internal/migrate/` — migration state machine + Echo handlers for `/migrate` and `/api/migrate/*`
- `internal/worker/` — River job worker implementations; `tasks/` contains workers for sync, import, export, and metadata refresh
- `internal/scheduler/` — River worker implementations for periodic maintenance jobs (cleanup, backup polling, stale job pruning); `BuildPeriodicJobs()` registers cron-scheduled River `PeriodicJob` entries
- `internal/services/` — IGDB client, storefront sync (Steam, PSN, GOG, Epic), game matching, platform resolution
- `internal/auth/` — session + API key auth + Echo middleware
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
- **Migrations**: Bun migrate (`uptrace/bun/migrate`); SQL files live in `internal/db/migrations/` with timestamp-prefix naming (`YYYYMMDD<nnnnnn>_name.up.sql`, where `<nnnnnn>` is a zero-padded running number, e.g. `20260503000001_name.up.sql`); discovered automatically via `Migrations.Discover(FS)` in `migrations.go`.

### Frontend Embedding (Stash pattern)
`ui/ui.go` exposes five `embed.FS` vars:
```go
//go:embed all:frontend/dist
var UIBox embed.FS      // main React SPA

//go:embed all:migrate
var MigrateBox embed.FS // standalone migration UI

//go:embed db-error
var DBErrorBox embed.FS

//go:embed setup
var SetupBox embed.FS

//go:embed all:shared
var SharedBox embed.FS
```
The Go binary serves the React SPA itself.

### Frontend Stack
- Vite 8 + React 19 + TypeScript
- TanStack Router (file-based routes in `ui/frontend/src/routes/`)
- TanStack Query, Tailwind CSS v4, shadcn/ui, React Hook Form + Zod, TipTap
- Vitest + @testing-library/react

### Route Zones
- **Migration zone** (`/migrate`, `/api/migrate/*`) — always available, bypasses state middleware
- **Setup zone** (`/api/auth/setup/*`) — requires `Ready` state, no auth (no users exist yet)
- **API zone** — gated by state middleware, then `AuthMiddleware` where required

### Workers & Scheduler
River (`riverqueue/river`) is the job queue. Worker structs live under `worker/tasks/` (sync, import, export, metadata refresh) and `internal/scheduler/` (cleanup jobs, backup polling). Periodic schedules are registered in `scheduler.BuildPeriodicJobs()` using `robfig/cron/v3` expressions and River `PeriodicJob`. Backup orchestration still lives in `internal/backup/` and is invoked by the `CheckScheduledBackupWorker`. Sync workers cover Steam, PSN, GOG, and Epic (via Legendary), plus IGDB metadata refresh.

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
3. **Migrations**: Add new `.up.sql` / `.down.sql` files in `internal/db/migrations/` using the naming convention `YYYYMMDD<nnnnnn>_name.up.sql` where `<nnnnnn>` is a zero-padded running number (e.g. `20260503000001_name.up.sql`); Bun discovers them automatically via `Migrations.Discover(FS)`
4. **Testing**: The mechanical gates run automatically via hooks (see [Automated Checks](#automated-checks)) — format/lint on every edit, build + typecheck when a turn ends, and the full suites at `git push`. You don't need to re-run the whole suites by hand; do run targeted tests (e.g. `go test ./internal/api/... -run TestX -v`) for the logic you're actively changing.
5. **Plan files**: `docs/superpowers/plans/` is tracked — always commit the plan file on the feature branch

### Out-of-Scope Findings
When working with code, if you identify a problem, bug, or inconsistency that you judge to be out-of-scope for the current task, do **not** silently fix it or ignore it — surface it and **offer to file a follow-up GitHub issue** (`gh issue create`) to track the fix. Only create the issue once the user agrees.

### Branch Workflow (MANDATORY)
- ✅ Always create a branch before starting task work
- ✅ Use `--squash --delete-branch` when merging PRs
- ❌ Never commit directly to main unless instructed to
- ❌ Never merge a PR on your own initiative — only when the user explicitly instructs

**`gh pr merge` handles everything locally:** when you run `gh pr merge --squash --delete-branch` while checked out on the PR's branch, `gh` switches your local checkout to `main`, deletes the local feature branch, **and pulls `main`** so it is already up to date with the new squash commit. Do **not** run `git checkout main`, `git branch -D`, `git pull`, or `git reset --hard origin/main` afterward — `gh` has already done it. Just confirm with `git status` (expect `## main...origin/main`, no divergence).

**Auto-closing issues from a PR (MANDATORY syntax):** a [closing keyword](https://docs.github.com/en/issues/tracking-your-work-with-issues/linking-a-pull-request-to-an-issue) (`Closes`/`Fixes`/`Resolves`) applies **only to the single issue number immediately following it**. A comma-separated list does **not** work: `Closes #846, #847` auto-closes only #846 and leaves #847 open. Repeat the keyword for every issue — `Closes #846, closes #847` — or put one per line:
```
Closes #846
Closes #847
```
Cross-repo: `Closes owner/repo#123`. The keyword must be in the **PR body** (the squash commit body is the PR body via `squash_merge_commit_message=PR_BODY`), not only the title. After merging a multi-issue PR, verify every referenced issue actually closed (`gh issue view <n> --json state`) and close any stragglers manually with a comment linking the PR.

### Merging Renovate PRs

All Renovate dependency PRs that touch `ui/frontend/package-lock.json`, `flake.lock`, or `nix/frontend.nix` conflict with each other — merging one immediately invalidates the others' lockfiles. **Never attempt to merge multiple such PRs in a single step.**

The correct workflow:
1. Verify all CI checks pass on the target PR (use `gh run list --branch <branch>` — the GitHub API's `statusCheckRollup` field lags and often shows stale/empty data)
2. Merge **one PR** with `gh pr merge <n> --squash --delete-branch`
3. The remaining PRs are now conflicting — trigger a rebase on each: `gh pr comment <n> --body "@renovatebot rebase"`
4. Wait for Renovate to push the rebase (~1–2 min) and CI to pass (~4 min), then repeat from step 1
5. Do **not** batch-trigger `@renovatebot rebase` on more PRs than you intend to merge in this cycle — each rebase triggers a full CI run and generates GitHub notification emails

### Code Style

**Go (Backend)**
- Standard Go conventions: `camelCase` unexported, `PascalCase` exported, `UPPER_CASE` constants
- Errors returned, not panicked; wrap with `fmt.Errorf("context: %w", err)`
- Echo handler signature: `func (h *Handler) ListGames(c *echo.Context) error` — note `*echo.Context` (pointer) in v5; middleware is `func(echo.HandlerFunc) echo.HandlerFunc`
- Use `*bun.DB` for DB access; pass via dependency injection

**TypeScript (Frontend)**
- Import order: external → internal (`@/...`) → types
- TanStack Query for server state; `useState` for local state only
- `routeTree.gen.ts` is generated by the TanStack Router Vite plugin and committed to git. After adding, moving, renaming, or removing a route, run `npm run build` to regenerate it, then commit the updated file alongside the route edit — CI fails if the committed file drifts from the route definitions.
- shadcn/ui components in `ui/frontend/src/components/ui/` follow an "include and prune" policy — knip removes any unused component or sub-export. If a feature needs a missing one (e.g. `Form`, `Tabs`, `Separator`), re-add it via `npx shadcn@latest add <component>` and import it.

### Quality Gates
Enforced automatically by the hooks in [Automated Checks](#automated-checks); this list is the contract they uphold:
- Zero Go build errors and zero `golangci-lint` errors before committing
- Zero TypeScript/lint errors (`npm run check`) and zero knip findings (`npm run knip`) before committing
- All tests must pass before committing

### Automated Checks

Format, lint, build, and test gates run automatically — you rarely invoke them by hand. **Launch Claude Code from inside `devenv shell`** (or an active direnv) so `go`, `golangci-lint`, `node`, and `jq` are on `PATH`; otherwise the Claude Code hooks silently no-op.

| When | Mechanism | What runs |
|---|---|---|
| After each file edit | `PostToolUse` hook → `.claude/hooks/post-edit.sh` | Go: `gofmt -w` + `golangci-lint` on the file's package. Frontend: `prettier --write` + `eslint --fix` on the file. Remaining findings are fed back to fix. |
| When a turn ends | `Stop` hook → `.claude/hooks/stop-check.sh` | `go build ./...` if any `.go` is dirty; `tsc --noEmit` if `ui/frontend/` is dirty. Build/typecheck only — a one-time nudge, not the hard gate. |
| `git push` | devenv `git-hooks` (pre-push) | Full `go test ./...` (when `.go` files change) and `npm run check && npm run knip && npm run test` (when `ui/frontend/` changes). The hard gate. |

Config lives in `.claude/settings.json` (Claude Code hooks) and `devenv.nix` → `git-hooks.hooks` (git). Pre-push hooks install/refresh on `devenv shell`; bypass in a pinch with `git push --no-verify`.

### Nix Flake Maintenance

The `nix/` directory contains the Nix package and NixOS module. Two hashes must be kept in sync with their respective lock files:

- **`npmDepsHash` in `nix/frontend.nix`** — update after any `ui/frontend/package-lock.json` change:
  ```bash
  nix run nixpkgs#prefetch-npm-deps -- ui/frontend/package-lock.json
  # paste the output hash into nix/frontend.nix → npmDepsHash
  ```
- **`vendorHash` in `nix/package.nix`** — update after any `go.mod` / `go.sum` change:
  ```bash
  # Set vendorHash = pkgs.lib.fakeHash; in nix/package.nix, then:
  nix build .#nexorious 2>&1 | grep "got:"
  # paste the "got:" hash into nix/package.nix → vendorHash
  ```

The `version` field in `flake.nix` is managed automatically by release-please (same as `Chart.yaml`).

## Release Process

Releases are produced by [release-please](https://github.com/googleapis/release-please) from the Conventional Commits on `main`. The full design is in [docs/superpowers/specs/2026-05-20-release-process-design.md](docs/superpowers/specs/2026-05-20-release-process-design.md).

### Commit message convention (MANDATORY)

All commits on `main` must follow [Conventional Commits](https://www.conventionalcommits.org/). PRs are squash-merged, so the **PR title** is the commit message release-please parses.

| Prefix | Effect |
|---|---|
| `feat: …` | minor bump |
| `fix: …` | patch bump |
| `feat!: …` or `BREAKING CHANGE:` footer | **major bump** |
| `chore:`, `ci:`, `docs:`, `refactor:`, `test:` | no release |

### Cutting a release

1. Wait until there is a `feat:` or `fix:` on `main` since the last release (otherwise release-please's Release PR will be empty).
2. Find the open PR titled `chore(main): release X.Y.Z` (opened by `release-please-action`).
3. Review the proposed `CHANGELOG.md` diff, `Chart.yaml` / `docker-compose.yml` version bumps.
4. Merge the Release PR. release-please creates the `vX.Y.Z` tag, publishes a GitHub Release, and `build-push.yaml` pushes the image and chart (semver tag + `latest`).

### Overrides

To force the next release to a specific version, add a `Release-As: X.Y.Z` line to the **PR description** before squash-merging. The repo is configured with `squash_merge_commit_message=PR_BODY`, so GitHub uses the PR description as the commit body — release-please will find the trailer there.

Alternatively, push an empty commit directly to `main`:

```bash
git commit --allow-empty -m "chore: release X.Y.Z" -m "Release-As: X.Y.Z"
git push origin main
```

To skip a release after an unwanted `feat:` / `fix:` lands: close the Release PR; release-please reopens it on the next push to main, so a true skip requires absorbing the change into the next legitimate release.

### Promoting to 1.0.0

A `feat!:` commit (or `BREAKING CHANGE:` footer) will naturally produce a `1.0.0` Release PR when the current version is `0.x.y`. Merge that PR to cut 1.0.0.

## Known Gotchas

- **`sql.ErrNoRows` vs DB errors** — always `errors.Is(err, sql.ErrNoRows)` to distinguish "not found" (→ 404/401) from real connection failures (→ 500); import `"database/sql"` for the sentinel. Bun wraps pgx errors into `sql.ErrNoRows`, so use the stdlib sentinel (not `pgx.ErrNoRows`)
- **`//go:embed all:dir`** — use `all:` prefix when the directory contains dot-files (e.g. `.gitkeep`); without it, Go silently excludes them and the build fails
- **Package name `migrate`** — `internal/migrate` uses package name `migrate`; when importing `uptrace/bun/migrate` inside that package, alias it: `bunmigrate "github.com/uptrace/bun/migrate"` to avoid the collision
- **`os.Exit` skips deferred calls** — call `pool.Close()` explicitly before any `os.Exit` in main; deferred `pool.Close()` will not run
- **Background goroutines** — use `context.Background()`, not `c.Request().Context()`, for work that outlives an HTTP handler
- **`errcheck` runs with `check-blank`** (`.golangci.yml`) — every `_ =` / `_, _ =` error discard fails CI. Default fix is to **handle** it (API handler → `slog.Error` + `echo.NewHTTPError(500, …)`; worker/scheduler → log-only `slog.Error(...)`). Suppress only a genuinely-acceptable discard, via one of: the `std-error-handling` preset (covers the `Close`/`Fprint` family — so `defer func() { _ = resp.Body.Close() }()` needs no annotation), the `(bun.Tx).Rollback` allowlist, or a per-site `//nolint:errcheck // <one-line reason>` (e.g. clamped param parse, advisory `RowsAffected`, marshal of a fixed struct). `_ =` in `_test.go` is exempt.
- **`gosec` is enabled** (`.golangci.yml`) — security findings (G1xx/G2xx/G3xx/G7xx) fail CI. `_test.go` is excluded wholesale (fixtures legitimately carry fake credentials, password-in-URL cases, marshaled test tokens). For non-test findings: fix genuine issues (e.g. tighten file/dir perms to `0600`/`0750`; validate user-derived paths against traversal); suppress only confirmed false positives with a per-site `//nolint:gosec // <reason>` stating *why* it's safe (e.g. fixed-binary subprocess, internally-derived path, value encrypted before storage, guarded against traversal above). Don't blanket-suppress a rule in config — keep each suppression at the site, auditable.
- **`jobs` table columns** — `id`, `user_id`, `job_type`, `source`, `status`, `priority`, `file_path`, `total_items`, `error_message`, `auto_retry_done`, `dispatch_complete`, `created_at`, `started_at`, `completed_at`. There is **no `updated_at`** column.
- **`job_items` table columns** — `id`, `job_id`, `user_id`, `external_game_id`, `item_key`, `source_title`, `source_metadata` (jsonb), `status`, `result` (jsonb), `error_message`, `igdb_candidates` (jsonb), `resolved_igdb_id`, `match_confidence`, `created_at`, `processed_at`, `resolved_at`. There is **no `error`** column (it's `error_message`).
- **Priority type mismatch** — `jobs.priority` is TEXT (`'high'`/`'low'`); `pending_tasks.priority` is INTEGER; don't conflate the two columns
- **Echo v5 route order** — register static routes before parameterised ones (e.g. `GET /sync/steam/status` before `GET /sync/:id`); Echo v5 doesn't auto-sort and will match the wrong handler otherwise
- **Service package import cycles** — if `internal/api` imports `internal/services/steam` and vice-versa, break the cycle by having each service package define its own local summary types; `router.go` bridges them with adapter structs that satisfy the handler's interface
- **Platform/storefront `icon` field** — DB stores bare filename (e.g. `steam-icon-light.svg`); API responses must construct `icon_url` as `/logos/storefronts/<name>/<filename>` (or `platforms/`). Logos are bundled in the SPA dist and served at `/logos/...` — not `/static/logos/`.
- **River queue is independent of `job_items`** — `UPDATE job_items SET status='pending'` does NOT re-enqueue the item. River only processes rows in `river_job`. Always use `POST /api/jobs/{id}/retry-failed` or `POST /api/job-items/{id}/retry` which call `retryInsert` and write both tables. A direct DB reset leaves the item permanently stuck.
- **bun raw scan struct tags** — when scanning raw SQL into a struct, bun maps columns by snake_casing the field name. If the column alias doesn't match exactly (e.g. `is_new_addition` → field `IsNewAdd` → bun expects `is_new_add`), the scan silently returns nil rows. Use explicit `bun:"column_name"` tags on all fields in raw-query result structs.
- **psql dev connection** — `psql "${DATABASE_URL/\/.s.PGSQL.5432/}"` — `DATABASE_URL` includes the full socket path; this substitution strips the socket filename so psql gets just the directory as `host`.
- **`pending_review` is settled, not active** — in `syncCheckJobCompletion`, only `pending` and `processing` count as "active" remaining work. However, `pending_review` items DO block job termination: the job stays in `processing` until the user resolves every pending_review item (manually matches, skips, etc.). Never mark a job `completed` while `pending_review` items exist.
- **Helm `values.schema.json` must stay in sync** — `deploy/helm/values.schema.json` uses `"additionalProperties": false` on the `nexorious` object; any new field added to `values.yaml` under `nexorious:` must also be registered in the schema or `helm lint --strict` fails. Also add `--set nexorious.<field>=x` to the lint step in `.github/workflows/test.yaml` if the field has a `fail` guard for its default placeholder value. Conversely, when **removing** a field, also remove its `--set nexorious.<field>=x` flag from `test.yaml` — `additionalProperties: false` will reject it and fail the lint.


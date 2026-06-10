# Development Guide

## Releases

Releases are managed by [release-please](https://github.com/googleapis/release-please), which watches commits on `main` and maintains an open Release PR. Merge the Release PR when you're ready to ship.

### Normal release

1. Merge one or more `feat:` or `fix:` PRs to `main`.
2. release-please opens (or updates) a PR titled `chore: release main` proposing the next version.
3. Review the `CHANGELOG.md` diff and version bumps in `Chart.yaml`, `docker-compose.yml`, and `flake.nix`.
4. Merge the Release PR. CI (`release-artifacts.yaml`) then builds, for both amd64 and arm64, the raw binary, the `.deb`, the `.rpm`, and the multi-arch container image — all from the same per-arch binary — smoke-tests every package, publishes the GitHub Release assets, pushes the image and Helm chart, and advances the `release` branch. There is no nightly or dev build flow; artifacts are produced only when a release is published.

### Forcing a specific version

If you want the next release to be a specific version (e.g. bump minor instead of patch), add a `Release-As: X.Y.Z` line to the **PR description** before squash-merging. The repo uses `squash_merge_commit_message=PR_BODY`, so GitHub includes the PR description in the commit body and release-please picks up the trailer automatically.

Alternatively, push an empty commit directly to `main`:

```bash
git commit --allow-empty -m "chore: release 0.2.0" -m "Release-As: 0.2.0"
git push origin main
```

Either way, release-please will update the open Release PR to propose the specified version.

### Version bump rules (pre-1.0)

| Commit prefix | Version bump |
|---|---|
| `fix:` | patch (0.1.0 → 0.1.1) |
| `feat:` | patch (0.1.0 → 0.1.1) |
| `feat!:` or `BREAKING CHANGE:` footer | minor (0.1.0 → 0.2.0) |
| `chore:`, `docs:`, `ci:`, etc. | no release |

## Prerequisites

- [devenv](https://devenv.sh) installed
- Run `devenv shell` to enter the development environment

## Building

Inside `devenv shell`, the toolchain (Go, Node, make, golangci-lint) is on `PATH`.

```bash
make             # build the frontend, then the Go binary
make frontend    # build only the React SPA into ui/frontend/dist/
make build       # compile only the Go binary (expects dist/ to exist)
```

`ui/frontend/dist/` is gitignored and embedded into the binary at build time, so `make frontend` (or a full `make`) must run before `make build`.

Common commands:

| Task | Command |
|---|---|
| Build everything | `make` |
| Build backend only | `make build` |
| Build frontend only | `make frontend` |
| Run server | `./nexorious serve` |
| Run migrations only | `./nexorious migrate` |
| Migration status | `./nexorious migrate status` |
| Run Go tests | `go test -timeout 600s ./...` |
| Run frontend tests | `npm run test` (from `ui/frontend/`) |
| Type check frontend | `npm run check` (from `ui/frontend/`) |
| Lint Go | `golangci-lint run` |

## Starting the database

Services (including PostgreSQL) are **not** started by `devenv shell`. You must start them separately:

```bash
devenv up        # foreground — logs stream to the terminal
devenv up -d     # background — returns to the prompt immediately
```

PostgreSQL listens on a Unix socket only (no TCP). devenv automatically exports `PGHOST` pointing at the socket directory, so `psql`, `go run`, and `DATABASE_URL` all work without any additional configuration once the service is running.

To verify the database is up:

```bash
psql nexorious
```

## Stopping the database

Due to a [known devenv bug](https://github.com/cachix/devenv/issues/2619), Ctrl+C on `devenv up` kills the process manager but leaves PostgreSQL running. Use the dedicated task to stop it cleanly:

```bash
devenv tasks run db:stop
```

This is useful when testing the DB-unavailability path in the app (Gate 1 redirects to `/db-error` within ~5 seconds of the DB going down).

## Resetting the database

### Option 1: Drop and recreate the database (keeps the cluster running)

Use this when you want a clean slate but don't need to wipe migrations or cluster-level state:

```bash
dropdb nexorious
createdb nexorious
```

The server keeps running. Reconnect with `psql nexorious` or restart the Go binary to re-run migrations.

### Option 2: Wipe the entire PostgreSQL cluster (full reset)

Use this when you want to start completely from scratch — e.g. the cluster is corrupt or you want to test `initialDatabases` behaviour:

```bash
# 1. Stop devenv services
devenv processes down   # stops background processes started with `devenv up -d`
                        # (Ctrl-C if running in the foreground instead)

# 2. Delete the cluster data directory
rm -rf .devenv/state/postgres

# 3. Restart — devenv recreates the cluster and the nexorious database
devenv up
```

## Database Migrations

Migrations live in `internal/db/migrations/` as paired SQL files named `YYYYMMDD<nnnnnn>_name.up.sql` / `.down.sql`, where `<nnnnnn>` is a zero-padded running number (e.g. `20260503000001_initial.up.sql`). They're discovered automatically (via `//go:embed`), but not applied silently — `serve` detects a pending schema and serves the `/migrate` page; apply them there or with `./nexorious migrate`. To add one:

```bash
touch internal/db/migrations/20260101000001_my_change.up.sql
touch internal/db/migrations/20260101000001_my_change.down.sql
```

Check status without applying:

```bash
./nexorious migrate status
```

## Frontend Dev Server

For iterating on frontend changes, use Vite's dev server instead of rebuilding the full embedded binary on every change. Vite proxies `/api` and `/static` requests to the running Go backend.

**Two-terminal workflow:**

```bash
# Terminal 1 — build and run the Go server
go build -o nexorious ./cmd/nexorious && ./nexorious serve

# Terminal 2 — Vite dev server with HMR on :3000
cd ui/frontend && npm run dev
```

Open `http://localhost:3000`. Frontend changes hot-reload instantly; backend changes require rebuilding and restarting the Go server (Terminal 1).

The proxy target defaults to `http://localhost:8000`. Override with `API_TARGET` if your backend runs on a different port:

```bash
API_TARGET=http://localhost:9000 npm run dev
```

## CLI Subcommands

The `nexorious` binary uses [cobra](https://github.com/spf13/cobra) subcommands:

| Subcommand       | What it does                                                   |
|------------------|----------------------------------------------------------------|
| `serve`          | Start the HTTP server                                          |
| `migrate`        | Apply all pending DB migrations and exit                       |
| `migrate status` | Print pending migrations without applying them                 |
| `version`        | Print build version and commit SHA                             |
| `login`          | Authenticate to a server and store an API key locally          |
| `logout`         | Revoke the stored API key and clear the local profile          |
| `whoami`         | Print the account behind the stored API key                    |
| `api-key`        | Manage API keys (generate, list, revoke)                       |
| `reset-password` | Reset a user's password directly in the database (offline)     |
| `setup`          | Create the first admin user on a running server (via HTTP)     |

Running `./nexorious` with no subcommand prints the help overview and exits non-zero; `serve` must be explicit to start the server.

A persistent `--config <file>` flag on the root command loads a `.env`-style file before parsing environment variables.

## Test Coverage

Run coverage across all packages:

```bash
go test -timeout 600s -cover ./...
```

Per-package detail (useful for finding gaps):

```bash
go test -timeout 600s -coverprofile=coverage.out ./internal/<pkg>/...
go tool cover -func=coverage.out | grep -v "100.0%"
```

Known coverage status (non-trivial packages, as of Phase 5):

| Package                         | Coverage |
|---------------------------------|----------|
| `internal/api`                  | ~67%     |
| `internal/auth`                 | ~89%     |
| `internal/backup`               | ~56%     |
| `internal/migrate`              | ~58%     |
| `internal/ratelimit`            | ~75%     |
| `internal/scheduler`            | ~12%     |
| `internal/services/igdb`        | ~48%     |
| `internal/worker`               | ~86%     |
| `internal/worker/tasks`         | ~47%     |

`cmd/nexorious` (5%) is excluded — it is startup wiring with no testable logic. The scheduler package is low because the goroutine lifecycle and gocron wiring are not unit-testable.

## Project Layout

```
nexorious/
├── cmd/nexorious/   # Entry point — wires config, DB, Echo, workers
├── internal/
│   ├── api/         # Echo route handlers (games, auth, sync, import, export, …)
│   ├── db/          # Bun ORM models and SQL migrations
│   ├── worker/      # River job workers (sync, import, export, metadata)
│   ├── scheduler/   # Periodic maintenance jobs (cleanup, backup polling)
│   ├── services/    # IGDB client, Steam/PSN/GOG/Epic sync, game matching
│   ├── auth/        # Session and API key auth + Echo middleware
│   └── config/      # Environment variable config
├── ui/
│   ├── frontend/    # React + Vite SPA source
│   └── migrate/     # Standalone migration UI (embedded in binary)
├── deploy/
│   ├── helm/        # Helm chart (bjw-s common library)
│   └── docker/      # Docker Compose for simple deployments
└── docs/            # Documentation
```

The route handlers in `internal/api/` are the source of truth for available API endpoints — each domain (games, user_games, auth, platforms, tags, jobs, import, export, sync, …) has its own handler file with the registered routes.

## Tech Stack

- **Backend**: Go 1.26, Echo v5, Bun ORM, River job queue, pgx/v5
- **Frontend**: React 19, Vite 8, TypeScript, TanStack Router + Query, Tailwind CSS v4, shadcn/ui
- **Database**: PostgreSQL 16+
- **Testing**: stdlib `testing` + testcontainers-go (Go); Vitest + @testing-library/react (frontend)

## Container Image (Docker)

The repo ships a multi-stage `Dockerfile` that builds the React SPA, compiles the Go binary, and produces a minimal `alpine:3.23` runtime image containing the `nexorious` binary, `ca-certificates`, `postgresql18-client` (for backup/restore), and `legendary-gl` with its Python runtime (for Epic Games Store sync). No Go or Node toolchain, source, or git is shipped in the final image.

**Build** (full source build — the default `runtime` target):

```bash
make docker   # builds the runtime target, tags nexorious:local
```

Or directly, e.g. to pass explicit version metadata:

```bash
docker build --target runtime \
  --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
  --build-arg COMMIT="$(git rev-parse --short HEAD)" \
  -t nexorious:local .
```

The `Dockerfile` shares a single `runtime-base` stage between `runtime` (the full source build above) and `runtime-ci` (used only by CI, which copies a prebuilt per-arch binary from a buildx named context instead of compiling). Building `runtime-ci` locally requires that named context — see `.github/workflows/release-artifacts.yaml`.

**Run the server:**

```bash
docker run --rm -p 8000:8000 \
  -e DATABASE_URL="postgres://user:pass@host:5432/nexorious?sslmode=disable" \
  nexorious:local serve
```

**Run migrations (one-shot):**

```bash
docker run --rm \
  -e DATABASE_URL="postgres://user:pass@host:5432/nexorious?sslmode=disable" \
  nexorious:local migrate
```

**Print version:**

```bash
docker run --rm nexorious:local version
```

All configuration is via environment variables (see `internal/config/`). The `ENTRYPOINT` is the `nexorious` binary, so the `CMD` (default `serve`) is the cobra subcommand — pass `migrate`, `migrate status`, `version`, etc. as arguments.



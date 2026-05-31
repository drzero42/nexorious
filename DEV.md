# Development Guide

## Releases

Releases are managed by [release-please](https://github.com/googleapis/release-please), which watches commits on `main` and maintains an open Release PR. Merge the Release PR when you're ready to ship.

### Normal release

1. Merge one or more `feat:` or `fix:` PRs to `main`.
2. release-please opens (or updates) a PR titled `chore: release main` proposing the next version.
3. Review the `CHANGELOG.md` diff and version bumps in `Chart.yaml`, `docker-compose.yml`, and `flake.nix`.
4. Merge the Release PR. CI automatically tags the commit, publishes a GitHub Release, pushes the container image and Helm chart, and advances the `release` branch.

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

## Frontend Dev Server

For iterating on frontend changes, use Vite's dev server instead of rebuilding the full embedded binary on every change. Vite proxies `/api` and `/static` requests to the running Go backend.

**Two-terminal workflow:**

```bash
# Terminal 1 — build and run the Go server
go build -o nexorious ./cmd/nexorious && ./nexorious

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
| `serve`          | Start the HTTP server (default when no subcommand is given)    |
| `migrate`        | Apply all pending DB migrations and exit                       |
| `migrate status` | Print pending migrations without applying them                 |
| `version`        | Print build version and commit SHA                             |

Running `./nexorious` with no arguments is equivalent to `./nexorious serve`.

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

## Container Image (Docker)

The repo ships a multi-stage `Dockerfile` that builds the React SPA, compiles the Go binary, and produces a minimal `alpine:3.23` runtime image containing only the `nexorious` binary, `ca-certificates`, and `postgresql18-client` (for backup/restore). No Go or Node toolchain, source, or git is shipped in the final image.

**Build:**

```bash
docker build \
  --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
  --build-arg COMMIT="$(git rev-parse --short HEAD)" \
  -t nexorious:local .
```

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

## API Client (Slumber)

The project includes a [Slumber](https://github.com/LucasPickering/slumber) collection for testing the API from the terminal. Slumber is included in the devenv shell — no separate install needed.

**Starting Slumber:**

```bash
slumber
```

**First-time setup (fresh database):**

Run these requests in order from the `bootstrap/` folder:

1. `bootstrap/run_migrations` — applies all pending database migrations
2. `bootstrap/migration_status` — check until status shows `ready` (run a few times if needed)
3. `bootstrap/create_admin` — creates the admin user (`admin` / `abcd1234`)

After that, any request requiring authentication will automatically log in on first use — no manual token handling.

**Day-to-day use:**

Open `slumber`, select the `local` profile, and run any request. Auth-protected routes auto-authenticate when needed using the cached credentials from the `local` profile.


# Development Guide

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

`initialDatabases` only runs on first start (when the data directory does not exist), so this is the only way to re-trigger it.

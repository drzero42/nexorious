# Nexorious Admin Guide

This guide is for whoever runs a Nexorious server — installing it, configuring it, looking after its data, and keeping it healthy. For using the app day to day (adding games, syncing, tracking progress), see the [User Guide](user-guide.md).

Administration in Nexorious comes in two parts: an **admin section** in the web app, available to any account with the admin role, and a set of **command-line tools** for the host itself. Both are covered below.

> Nexorious is under active development and not yet ready for production use. Treat any data you put in as replaceable for now, and read [Upgrades and versioning](#upgrades-and-versioning) before relying on it.

## Deploying Nexorious

Nexorious is a single binary with the web interface built in, run alongside a PostgreSQL database. Whichever way you deploy it, the server needs a database it can reach and the [configuration](#configuration) below, and it serves HTTP on port 8000 by default. The schema is brought up to date by a migration step that's separate from the running server — some deployments run it for you (the Helm chart does, by default), and otherwise you run it once yourself; see [First run](#first-run).

Pick the option that fits your environment.

### Docker Compose

The quickest route to a production-like setup uses the published container image and a database, wired together:

```bash
cp .env.example .env   # fill in DB_ENCRYPTION_KEY, IGDB_CLIENT_ID, IGDB_CLIENT_SECRET, POSTGRES_PASSWORD
docker compose -f deploy/docker/docker-compose.yml up -d
```

On the first launch the app serves the migration page until you apply the schema (see [First run](#first-run)); you can also run the migration as a one-shot with `docker compose -f deploy/docker/docker-compose.yml run --rm app migrate`. Make sure the volumes backing `STORAGE_PATH` and `BACKUP_PATH` persist.

### Kubernetes / Helm

A Helm chart is published to `oci://ghcr.io/drzero42/charts` and built on the [bjw-s common library](https://bjw-s-labs.github.io/helm-charts/):

```bash
helm install nexorious oci://ghcr.io/drzero42/charts/nexorious \
  --set nexorious.igdbClientId=YOUR_CLIENT_ID \
  --set nexorious.igdbClientSecret=YOUR_CLIENT_SECRET \
  --set nexorious.postgresql.password="$(openssl rand -hex 16)"
```

By default the chart adds a `migrate` initContainer that runs `nexorious migrate` before the main container serves traffic, so pending migrations are applied automatically on every deploy — there's no manual migration step. See `deploy/helm/values.yaml` in the repository for the full set of chart values (including how to adjust or disable that initContainer).

### NixOS

The Nix flake exposes a package, an overlay, and a NixOS module. Two input URLs are available:

```nix
# Latest stable release — updates with `nix flake update`
inputs.nexorious.url = "github:drzero42/nexorious/release";

# Bleeding edge (tracks main)
inputs.nexorious.url = "github:drzero42/nexorious";
```

A minimal configuration:

```nix
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.nexorious.url = "github:drzero42/nexorious/release";

  outputs = { nixpkgs, nexorious, ... }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        nexorious.nixosModules.default
        ({ pkgs, ... }: {
          nixpkgs.overlays = [ nexorious.overlays.default ];

          services.nexorious = {
            enable = true;
            environmentFile = "/run/secrets/nexorious.env";
          };
        })
      ];
    };
  };
}
```

The environment file holds the secrets:

```bash
DB_ENCRYPTION_KEY=...   # generate: openssl rand -base64 32
IGDB_CLIENT_ID=...
IGDB_CLIENT_SECRET=...
```

The main module options:

| Option | Default | Description |
|---|---|---|
| `services.nexorious.enable` | `false` | Enable the service. |
| `services.nexorious.port` | `8000` | TCP port to listen on. |
| `services.nexorious.database.createLocally` | `true` | Manage a local PostgreSQL instance automatically. |
| `services.nexorious.database.name` | `"nexorious"` | Database name (when `createLocally = true`). |
| `services.nexorious.storagePath` | `/var/lib/nexorious` | Path for uploads and backups. |
| `services.nexorious.environmentFile` | — | Path to the environment file with secrets. |

With `database.createLocally = true` (the default) PostgreSQL is set up for you and you don't need `DATABASE_URL`. To use an external database, set `database.createLocally = false` and put `DATABASE_URL=postgresql://…` in the environment file.

### Single binary

You can also build the binary and run it directly next to a PostgreSQL instance — simplest for a plain server or for trying it out. Build it (`make`), set at least the [database connection and `DB_ENCRYPTION_KEY`](#configuration), and start it:

```bash
export DATABASE_URL="postgres://user:password@host:5432/nexorious"
export DB_ENCRYPTION_KEY="$(openssl rand -base64 32)"
./nexorious serve
```

On the first start it detects the pending schema and serves the migration page instead of the app; apply the migrations from there, or run `./nexorious migrate` first (see [First run](#first-run)). After that it serves on `PORT` (default 8000). Building from source is covered in the [Development Guide](../DEV.md).

Note that the in-app backup and restore feature shells out to `pg_dump` and `psql`, so install the PostgreSQL client tools on the host if you want it; see [Backups and restore](#backups-and-restore).

## Configuration

Nexorious is configured entirely through environment variables. Two things are genuinely required — a database connection and an encryption key — and IGDB credentials are required in practice. Everything else has a sensible default, so a minimal configuration is short.

### The essentials

- **Database connection.** Either set `DATABASE_URL` to a full `postgres://…` URL, or set the individual `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, and `DB_NAME` parts. If `DATABASE_URL` is present it wins; otherwise the parts are assembled into one.
- **`DB_ENCRYPTION_KEY`** (required) — a random secret used to encrypt sensitive data at rest, above all users' stored sync credentials. Generate one with `openssl rand -base64 32`. **Keep it safe and unchanged:** if you lose it or change it, the data encrypted under it (sync tokens) can no longer be decrypted, and affected users have to reconnect their sync accounts.
- **`IGDB_CLIENT_ID` / `IGDB_CLIENT_SECRET`** — credentials for IGDB, which powers search, cover art, and all metadata enrichment. The server runs without them, but search, adding games by search, and Darkadia import won't work until they're set. See [Setting up IGDB credentials](#setting-up-igdb-credentials).
- **`SESSION_COOKIE_SECURE`** — `true` by default, which tells browsers to send the login cookie only over HTTPS. With it on, logging in over plain HTTP on anything other than `localhost` silently fails; set it to `false` only when you deliberately serve over plain HTTP (a trusted LAN, say), and keep it `true` behind HTTPS.

### Full reference

All variables, grouped by area. Anything without a default is unset unless you provide it.

**Database**

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | Full PostgreSQL connection URL. Takes priority over the `DB_*` parts below. |
| `DB_HOST` | `localhost` | Database host (used only when `DATABASE_URL` is unset). |
| `DB_PORT` | `5432` | Database port. |
| `DB_USER` | `nexorious` | Database user. |
| `DB_PASSWORD` | `nexorious` | Database password. Change this for any real deployment. |
| `DB_NAME` | `nexorious` | Database name. |

**Security & sessions**

| Variable | Default | Description |
|---|---|---|
| `DB_ENCRYPTION_KEY` | — (required) | Secret for at-rest encryption of stored credentials. Generate with `openssl rand -base64 32`; keep it stable. |
| `SESSION_EXPIRE_DAYS` | `30` | How long a login session stays valid. |
| `SESSION_COOKIE_SECURE` | `true` | Restrict the session cookie to HTTPS. Set `false` only for plain-HTTP access (see above). |

**IGDB**

| Variable | Default | Description |
|---|---|---|
| `IGDB_CLIENT_ID` | — | IGDB/Twitch client ID. |
| `IGDB_CLIENT_SECRET` | — | IGDB/Twitch client secret. |
| `IGDB_ACCESS_TOKEN` | — | Advanced: supply a pre-obtained access token instead of letting Nexorious fetch one from the client ID and secret. Normally leave unset. |
| `IGDB_REQUESTS_PER_SECOND` | `4.0` | Outbound IGDB request rate. The defaults respect IGDB's published limits — only lower them if asked to. |
| `IGDB_BURST_CAPACITY` | `4` | Burst allowance for the IGDB rate limiter. |
| `IGDB_MAX_RETRIES` | `3` | Retries for a failed IGDB request. |
| `IGDB_BACKOFF_FACTOR` | `1.0` | Backoff multiplier between IGDB retries. |

**Steam**

| Variable | Default | Description |
|---|---|---|
| `STEAM_REQUESTS_PER_SECOND` | `4.0` | Outbound Steam Web API request rate. |
| `STEAM_BURST_CAPACITY` | `10` | Burst allowance for the Steam rate limiter. |

**Storage & paths**

| Variable | Default | Description |
|---|---|---|
| `STORAGE_PATH` | `./storage` | Where uploads and other persistent files live. Put this on a persistent volume in containers. |
| `BACKUP_PATH` | `./storage/backups` | Where backups are written. Also needs to persist. |
| `TEMP_STORAGE_DIR` | `/tmp/nexorious_uploads` | Scratch space for in-progress uploads. Can be ephemeral. |

**Application**

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8000` | HTTP port to listen on. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `DEBUG` | `false` | Extra debug behaviour; leave off in production. |
| `CORS_ORIGINS` | — | Comma-separated allowed origins. Only needed for split-origin development; production is same-origin and needs nothing here. |
| `WORKER_COUNT` | `4` | How many background jobs (syncs, imports, metadata refreshes) run at once. |
| `UPDATE_CHECK_ENABLED` | `true` | Periodically check GitHub for a newer release. When one exists, the sidebar shows an update notice and admins receive a one-time notification per release. The check runs server-side (one request to the GitHub API every 6 hours); set `false` to disable it entirely, which also disables the check performed by the `version` subcommand (one-off skips: `nexorious version --no-check`). |

**Scheduling & retention**

| Variable | Default | Description |
|---|---|---|
| `METADATA_REFRESH_INTERVAL` | `24h` | How often the automatic metadata refresh runs (a Go duration string, e.g. `24h`). |
| `STALE_JOB_THRESHOLD` | `4h` | How long a stuck metadata-refresh job may sit before housekeeping marks it failed. |
| `SYNC_HISTORY_RETENTION_DAYS` | `90` | How long sync-history entries are kept before nightly pruning. |
| `NOTIFY_EVENTS_RETENTION_DAYS` | `90` | How long notification events are kept. |
| `RATE_LIMITER_BACKEND` | `local` | `local` for a single instance, or `postgres` to share rate-limit state across multiple instances. |

**Epic Games Store**

| Variable | Default | Description |
|---|---|---|
| `LEGENDARY_WORK_DIR` | — | Base directory for the per-user Legendary configuration that Epic sync relies on. **Epic sync is disabled while this is unset** (see [Epic Games Store sync](#epic-games-store-sync)). |

The defaults are fine for most deployments — a typical setup only sets the database connection, `DB_ENCRYPTION_KEY`, the two IGDB variables, and possibly `STORAGE_PATH`/`BACKUP_PATH` and `SESSION_COOKIE_SECURE`.

## First run

1. **Apply migrations.** The database schema is created and updated by migrations. The running server (`serve`) never applies them itself — it's a separate step — but how that step happens depends on how you deploy:
   - **Helm** runs it for you: the chart's `migrate` initContainer applies anything pending before the server starts, so there's nothing manual to do (see [Kubernetes / Helm](#kubernetes--helm)).
   - **Otherwise**, you run it yourself — either from the command line with `nexorious migrate` (and `nexorious migrate status` to preview what's pending), or through the web UI: when the server comes up against a schema that's behind, it holds back the normal interface and serves a migration page at `/migrate`, where you click to run them and watch live progress.

   Until the schema is up to date, the server gates every route other than the migration page.

2. **Create the first admin.** With no users yet, opening the site prompts you to create the first account, which is automatically an admin. You can also create it from the host with `nexorious setup` — handy for scripted or headless installs.

3. **Configure IGDB.** Set `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` as part of your configuration, ideally before the first start — see [Setting up IGDB credentials](#setting-up-igdb-credentials) for how to obtain them. The app shows a banner while IGDB is unconfigured or its credentials are rejected, so an instance still runs without them; you just can't search or add games until they're set. IGDB is configured through these environment variables, not in the web interface, and they're read at startup — so if you add or change them on an already-running server, restart it to pick up the change.

After that, users sign in and use the app as described in the [User Guide](user-guide.md). Each user connects their own storefront sync accounts — there's nothing server-wide to set up for sync.

## Setting up IGDB credentials

IGDB is the database behind Nexorious's game search, cover art, descriptions, release dates, time-to-beat estimates, and the rest of the metadata. The app talks to it through Twitch's developer platform, so you need a free Twitch account and a registered application. The whole thing takes a few minutes and you only do it once per instance.

1. **Have a Twitch account with two-factor enabled.** Sign up at [twitch.tv](https://twitch.tv) if you don't have one, then turn on two-factor authentication in [Twitch security settings](https://www.twitch.tv/settings/security) — Twitch won't let you into the developer console without it.

2. **Register an application.** Go to the [Twitch Developer Console](https://dev.twitch.tv/console), sign in, and register a new application. Give it any name (for example "Nexorious"), set the OAuth redirect URL to `http://localhost` (it isn't actually used), and pick the "Application Integration" category.

3. **Collect the credentials.** Open the application you just made. Copy its **Client ID**, then generate a **Client Secret** — copy that immediately, because Twitch only shows it once.

4. **Give them to Nexorious.** Set `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` the same way you set the rest of your configuration — in a `.env` file, in your Docker Compose or Helm values, or as plain environment variables — alongside your other settings before you start the server. Treat the secret like any other credential: keep it out of version control, and rotate it in the developer console if it ever leaks.

5. **Check it took.** Once the server is running with the credentials in place, the startup logs won't warn about missing IGDB credentials, the "IGDB not configured" banner is gone, and a search on the Add Game page returns results. If you added the credentials to a server that was already running, restart it first — they're only read at startup. If search still fails or the banner sticks around, re-check the Client ID and regenerate the secret; a rotated or mistyped secret is the usual cause.

## Epic Games Store sync

Most storefront syncs need nothing from you as the operator — each user supplies their own credentials from their profile, and Nexorious talks to the store's API directly. **Epic is the exception:** it works through the `legendary` command-line tool (from the legendary-gl project), so `legendary` has to be available to the server, and you point Nexorious at a writable directory for its per-user data with `LEGENDARY_WORK_DIR`. While `LEGENDARY_WORK_DIR` is unset, Epic sync is disabled and users can't connect an Epic account.

Getting `legendary` in place depends on how you deploy:

- **Official container image** — `legendary` is already bundled in the image, so there's nothing to install. Just set `LEGENDARY_WORK_DIR` to a path on a persistent volume. The provided `deploy/docker/docker-compose.yml` already does this (it sets `LEGENDARY_WORK_DIR` and mounts a volume for it), so Epic sync works out of the box there.
- **Single binary / from source** — install `legendary` (the legendary-gl package) on the host yourself, so the `legendary` command is on the server's `PATH`, then set `LEGENDARY_WORK_DIR`.

Either way, point `LEGENDARY_WORK_DIR` at a writable, persistent directory and restart the server. Steam, PlayStation, GOG, and Humble Bundle sync have no such requirement.

## The admin section

Accounts with the admin role get an extra section in the sidebar. It's where you manage users and look after the instance.

### Admin dashboard

A summary view: how many users and admins exist, the total number of games across the instance, and shortcuts to create or manage users.

### User management

Under **User Management** you can:

- **Create users** with a username and password, optionally granting them the admin role.
- **Edit a user** — change their username, toggle the admin role, or activate/deactivate the account. Deactivating blocks sign-in without deleting anything. You can't deactivate or remove your own account this way, so an instance can't lock itself out.
- **Reset a password** for a user who's locked out.
- **Delete a user**, which also removes their games and data. Because that's destructive, it tells you how much will be removed and asks you to type the username to confirm.

The admin role is what unlocks this section, backups, the activity feed, maintenance, and the database reset — so grant it sparingly.

### Backups and restore

Under **Backup / Restore** you manage the instance's data backups.

- **Schedule** — back up manually only, or daily, or weekly, at a time (and, for weekly, a day) you choose.
- **Retention** — keep either the most recent *N* backups or everything newer than a set age; older ones are pruned automatically.
- **Manual backup** — create one immediately with a single button.
- **The backups list** — each backup shows its type (scheduled, manual, or the automatic pre-restore one), when it was made, and its size. You can **download** any backup to keep off-server, **delete** ones you don't need, and **upload** a backup file from elsewhere.
- **Restore** — restoring replaces the instance's current data with the backup's, so it's guarded by a confirmation and automatically takes a fresh "pre-restore" backup first, giving you a way back if a restore wasn't what you wanted.

Backups and restores are produced by the standard PostgreSQL client tools — `pg_dump` writes each backup and `psql` applies a restore — so both have to be available to the server. As with `legendary`, this depends on how you deploy:

- **Official container image / Helm** — the postgres client tools are already bundled in the image, so there's nothing to install.
- **Single binary / from source** — install the PostgreSQL client package (`postgresql-client`, ideally matching your server's major version) on the host yourself, so `pg_dump` and `psql` are on the server's `PATH`.

If they're missing, Nexorious starts normally but disables this whole section — backup creation and restore both return an error, and the startup log notes `pg_dump not found` / `psql not found`.

Keeping backups somewhere off the server, especially before upgrades, is the safest habit while the project is still moving quickly.

### Maintenance

The **Maintenance** page runs occasional, instance-wide jobs by hand:

- **Refresh metadata** — re-pull game details from IGDB (titles, cover art, ratings, and so on) so your catalogue reflects IGDB's latest data.
- **Refresh store links** — re-resolve the direct storefront links shown on games.

Each shows progress while running, lets you cancel it, and reports failures with a retry option. Alongside these, Nexorious runs its own periodic housekeeping in the background — pruning old sync history and finished jobs, polling for scheduled backups, and clearing out expired exports — so those tables don't grow without bound. That housekeeping needs no attention from you; some of its retention windows are adjustable through environment variables (for example sync-history and notification-event retention), while others are fixed.

### Activity feed

**Activity** is the instance's event log, newest first. Filter it by category, event type, scope (user-level vs admin-level), the user involved, or a date range, and expand any entry for its full detail. It's the first place to look when you want to know what happened and when.

### Database reset

The Maintenance page also has a **database reset** in its danger zone. It permanently clears the library data — every user's games, tags, sync configurations, and jobs — and removes all non-admin users, returning the instance to a near-empty state. It deliberately keeps admin accounts and your existing backups, so it's a data wipe rather than a full factory reset. It's guarded by a typed confirmation (you type `RESET`). You'll rarely want this outside of testing, and it is not the upgrade mechanism (see below).

## Command-line tools

The same binary that serves the app is also a command-line tool for tasks you do from the host. Run `nexorious <command> --help` for the full details of any of them. A `--config` flag on every command points at a `.env` file if you keep your settings in one.

| Command | What it does |
|---|---|
| `nexorious serve` | Start the HTTP server, background workers, and scheduler. This is the main run command. |
| `nexorious migrate` | Apply any pending database migrations and exit. Useful as an init step in orchestrated deployments. |
| `nexorious migrate status` | Show how many migrations are pending and the current schema version, without changing anything. |
| `nexorious setup` | Create the first admin user against a running server — good for headless installs. Supports `--username` and reading the password from stdin (`--password-stdin`), and can store an API key for you with `--login`. |
| `nexorious reset-password <username>` | Reset a user's password by talking to the database directly. Your recovery path if an admin is locked out and no other admin can help. |
| `nexorious login` / `logout` / `whoami` | Authenticate the CLI to a server and store an API key, revoke and clear it, or show who the stored key belongs to. |
| `nexorious api-key generate \| list \| revoke` | Manage API keys from the command line: create one (with a name, scope, and optional expiry), list your keys, or revoke one by id or name. |
| `nexorious version` | Print version information. |

## Monitoring and operations

- **Logs** — Nexorious logs to standard output; set `LOG_LEVEL=debug` when you need more detail. In Docker, Kubernetes, or systemd, collect them however you collect logs from anything else.
- **Version** — `nexorious version` on the host, and the running version is shown in the app's sidebar, so you can confirm what's actually deployed.
- **Notifications** — Nexorious's notifications are configured per user (in each user's profile), not centrally. As an operator you mainly care that the server can reach whatever channels users configure (for example outbound network access for email or webhooks).

## Upgrades and versioning

Schema changes ship as migrations, applied as a step separate from the running server. On Helm, the chart's `migrate` initContainer handles this automatically on each deploy. On other deployments you apply them yourself — run `nexorious migrate`, or let the server come up against the new schema and use the `/migrate` page it serves. So a normal upgrade is: take a backup, deploy the new version, make sure migrations are applied (automatic on Helm; `nexorious migrate` or the `/migrate` page otherwise), done.

**The one big caveat is the 1.0.0 release.** The first stable release will be a deliberate clean break with **no automatic upgrade path** from the pre-1.0 versions. To move onto 1.0.0 you'll export each user's collection (JSON export from Import / Export), start fresh with an empty database, and import again. Until 1.0.0 lands, keep this in mind and don't treat a pre-1.0 instance as permanent storage — keep exports of anything you'd be sad to re-enter.

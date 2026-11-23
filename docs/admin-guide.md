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
cd deploy/docker
cp .env.example .env   # fill in DB_ENCRYPTION_KEY, IGDB_CLIENT_ID, IGDB_CLIENT_SECRET, POSTGRES_PASSWORD
docker compose up -d
```

On the first launch the app serves the migration page until you apply the schema (see [First run](#first-run)); you can also run the migration as a one-shot with `docker compose run --rm app migrate`. Before the app starts, a one-shot `db-role` service makes the app's own non-superuser database role (see [Database role](#database-role)). The compose file provisions a `nexorious_storage` volume at `/app/storage` (with backups under `/app/storage/backups`), so uploads and backups survive a `docker compose down` and recreate; only `docker compose down -v` removes it.

### Native packages (Debian/Ubuntu, RHEL/Fedora/Rocky)

For Debian/Ubuntu and RHEL/Fedora/Rocky hosts there are native `.deb` and `.rpm` packages on each [GitHub Release](https://github.com/drzero42/nexorious/releases), for `amd64` and `arm64`. They install the binary, a systemd service, a dedicated `nexorious` user, and a managed config file — no hand-wiring.

```bash
# Pick the file matching your distro and architecture from the Release, then:
# Verify it against the release checksums (optional but recommended):
sha256sum -c sha256sums.txt --ignore-missing

# Debian / Ubuntu:
sudo apt install ./nexorious_X.Y.Z_amd64.deb

# RHEL / Fedora / Rocky:
sudo dnf install ./nexorious-X.Y.Z.x86_64.rpm
```

The package depends on the PostgreSQL client tools (`postgresql-client` on Debian/Ubuntu, `postgresql` on RHEL-family), which your package manager pulls in automatically. It installs config to `/etc/nexorious/nexorious.env`, presets `STORAGE_PATH`/`BACKUP_PATH` to `/var/lib/nexorious`, and **auto-generates `DB_ENCRYPTION_KEY` on first install**.

After installing, set your database connection and start the service:

```bash
sudo nano /etc/nexorious/nexorious.env   # set DATABASE_URL
sudo systemctl enable --now nexorious
```

The service is not started automatically on install — it idles on the "database unavailable" page until `DATABASE_URL` is set, so you enable it once you've configured it (see [First run](#first-run)). Logs go to the journal: `journalctl -u nexorious -f`.

### Kubernetes / Helm

A Helm chart is published to `oci://ghcr.io/drzero42/charts` and built on the [bjw-s common library](https://bjw-s-labs.github.io/helm-charts/):

```bash
helm install nexorious oci://ghcr.io/drzero42/charts/nexorious \
  --set nexorious.igdbClientId=YOUR_CLIENT_ID \
  --set nexorious.igdbClientSecret=YOUR_CLIENT_SECRET \
  --set nexorious.postgresql.password="$(openssl rand -hex 16)"
```

**Choosing the database.** By default the chart runs PostgreSQL itself as a single StatefulSet. That's fine for trying Nexorious out, but for anything you care about, use the chart's [CloudNativePG](https://cloudnative-pg.io/) option instead: set `cnpg.enabled` and turn off the bundled PostgreSQL, and the chart creates a CloudNativePG `Cluster` and connects Nexorious to it. CloudNativePG handles major-version upgrades, failover and its own backups with point-in-time recovery, which the bundled StatefulSet leaves to you. It needs the CloudNativePG operator and its CRDs installed in the cluster first. The chart README's [CloudNativePG section](../deploy/helm/README.md#cloudnativepg-recommended) has the values; to move an existing install over, see [Switching to CloudNativePG](#switching-to-cloudnativepg) below. You can also point the chart at a database you run yourself, including a CloudNativePG cluster you created ([Bring your own PostgreSQL](../deploy/helm/README.md#bring-your-own-postgresql-eg-cloudnativepg)).

With the bundled PostgreSQL, a `db-role` initContainer first makes the app's own non-superuser database role (see [Database role](#database-role)). By default the chart also adds a `migrate` initContainer that runs `nexorious migrate` before the main container serves traffic, so pending migrations are applied automatically on every deploy — there's no manual migration step. See `deploy/helm/values.yaml` in the repository for the full set of chart values (including how to adjust or disable that initContainer).

#### Switching to CloudNativePG

Switching an existing install from the bundled PostgreSQL to CloudNativePG starts a new, empty database, so you move your data with a backup and a restore.

Before you start:

- Install the [CloudNativePG operator](https://cloudnative-pg.io/documentation/current/installation_upgrade/) in the cluster.
- Pick a quiet moment. Changes made after the backup in step 1 are lost.
- Log `nexctl` in as an admin (`nexctl account login`).
- Keep `nexorious.dbEncryptionKey` (or `dbEncryptionKeyFrom`) exactly as it is. The stored sync credentials in the backup are encrypted with it.

1. **Take a backup.**

   ```bash
   nexctl backup create
   ```

   It's saved on the Nexorious storage volume, which the switch leaves alone. To keep a copy off the cluster as well, run `nexctl backup download <id>` with the id it printed.

2. **Switch the chart to CloudNativePG.** Add these values and run `helm upgrade`:

   ```yaml
   cnpg:
     enabled: true
   controllers:
     postgresql:
       enabled: false
   service:
     postgresql:
       enabled: false
   persistence:
     postgresql-data:
       enabled: false
   ```

   **This deletes the bundled database and its volume.** Nexorious starts on the new, empty CloudNativePG database and shows the setup screen. The pod waits until CloudNativePG has created the database, which can take a minute or two.

3. **Restore the backup.** List the backups on the storage volume, then restore the one from step 1:

   ```bash
   nexctl setup backups --url https://nexorious.example.com
   nexctl setup restore <filename> --url https://nexorious.example.com
   ```

   The filename is the backup id plus `.tar.gz`. You can also restore from the setup screen in the browser. When the restore is done, log in with your usual account.

If something goes wrong, you still have the backup on the storage volume (and the copy you downloaded): `nexctl setup restore --file <path>` uploads one.

> **Tip: keep the old database until the switch has worked.** Before step 2, run `helm upgrade` once with `persistence.postgresql-data.retain: true` added to your values. Helm then leaves the old database volume in place when step 2 turns it off. Once you're happy with the new database, delete it: `kubectl get pvc` shows it as the volume whose name ends in `-postgresql-data` (`nexorious-postgresql-data` for a release named `nexorious`), and `kubectl delete pvc <name>` removes it.

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

On Debian/Ubuntu or RHEL/Fedora/Rocky, prefer the [native packages](#native-packages-debianubuntu-rhelfedorarocky) above — they wire up the service, user, and config for you. For other distributions, or just to try it out, you can build and run the binary directly next to a PostgreSQL instance. Build it (`make`), set at least the [database connection and `DB_ENCRYPTION_KEY`](#configuration), and start it:

```bash
export DATABASE_URL="postgres://user:password@host:5432/nexorious"
export DB_ENCRYPTION_KEY="$(openssl rand -base64 32)"
./nexorious serve
```

On the first start it detects the pending schema and serves the migration page instead of the app; apply the migrations from there, run `./nexorious migrate` first, or start with `./nexorious serve --migrate` to do both in one go (see [First run](#first-run)). After that it serves on `PORT` (default 8000). Building from source is covered in the [Development Guide](../DEV.md).

Note that the in-app backup and restore feature shells out to `pg_dump` and `psql`, so install the PostgreSQL client tools on the host if you want it; see [Backups and restore](#backups-and-restore).

## Configuration

Nexorious is configured entirely through environment variables. Two things are genuinely required — a database connection and an encryption key — and IGDB credentials are required in practice. Everything else has a sensible default, so a minimal configuration is short.

### The essentials

- **Database connection.** Either set `DATABASE_URL` to a full `postgres://…` URL, or set the individual `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, and `DB_NAME` parts. If `DATABASE_URL` is present it wins; otherwise the parts are assembled into one.
- **`DB_ENCRYPTION_KEY`** (required) — a random secret used to encrypt sensitive data at rest, above all users' stored sync credentials. Generate one with `openssl rand -base64 32`. **Keep it safe and unchanged:** if you lose it or change it, the data encrypted under it (sync tokens) can no longer be decrypted, and affected users have to reconnect their sync accounts.
- **`IGDB_CLIENT_ID` / `IGDB_CLIENT_SECRET`** — credentials for IGDB, which powers search, cover art, and all metadata enrichment. The server runs without them, but search, adding games by search, and importing a library from any source won't work until they're set. See [Setting up IGDB credentials](#setting-up-igdb-credentials).
- **`SESSION_COOKIE_SECURE`** — `true` by default, which tells browsers to send the login cookie only over HTTPS. With it on, logging in over plain HTTP on anything other than `localhost` silently fails; set it to `false` only when you deliberately serve over plain HTTP (a trusted LAN, say), and keep it `true` behind HTTPS.
- **Where config lives.** With the `.deb`/`.rpm` packages, configuration is the env file at `/etc/nexorious/nexorious.env` (read by the systemd service); the package presets `STORAGE_PATH` and `BACKUP_PATH` to `/var/lib/nexorious` and generates `DB_ENCRYPTION_KEY` for you on first install — never change that generated key afterwards. With Docker, Helm, or the raw binary, you supply the same variables your usual way.

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
| `CORS_ORIGINS` | — | Comma-separated allowed origins. Only needed for split-origin development; production is same-origin and needs nothing here. The MCP endpoint also trusts these origins (see [MCP endpoint](#mcp-endpoint-for-ai-agents)). |
| `MCP_ENABLED` | `true` | Serve the MCP endpoint for AI chats and agents at `/api/mcp`. Set `false` to turn it off; see [MCP endpoint](#mcp-endpoint-for-ai-agents). |
| `WORKER_COUNT` | `4` | How many background jobs (syncs, imports, metadata refreshes) run at once. |
| `UPDATE_CHECK_ENABLED` | `true` | Periodically check GitHub for a newer release. When one exists, the sidebar shows an update notice and admins receive a one-time notification per release. The check runs server-side (one request to the GitHub API every 6 hours); set `false` to disable it entirely, which also disables the check performed by the `version` subcommand (one-off skips: `nexorious version --no-check`). |

**Scheduling & retention**

| Variable | Default | Description |
|---|---|---|
| `METADATA_REFRESH_INTERVAL` | `24h` | How often the automatic metadata refresh runs (a Go duration string, e.g. `24h`). |
| `METADATA_REFRESH_WORKERS` | `1` | Concurrency of the dedicated metadata-refresh queue. 1 keeps the nightly refresh to ~half the IGDB rate budget so user-facing IGDB traffic stays responsive; raise to finish faster at the cost of crowding user IGDB requests. |
| `METADATA_REFRESH_MIN_AGE` | `23h` | Go duration; a game is only re-refreshed once its `last_updated` is older than this. Set slightly below `METADATA_REFRESH_INTERVAL` (default 23h vs 24h) so a game refreshed yesterday is reliably eligible today. |
| `STALE_JOB_THRESHOLD` | `4h` | How long a sync whose dispatch never finished may sit before housekeeping marks it failed. Metadata and store-link refreshes and exports are recovered without a time limit. |
| `SYNC_HISTORY_RETENTION_DAYS` | `90` | How long sync-history entries are kept before nightly pruning. |
| `NOTIFY_EVENTS_RETENTION_DAYS` | `90` | How long notification events are kept. |
| `RATE_LIMITER_BACKEND` | `local` | `local` for a single instance, or `postgres` to share rate-limit state across multiple instances. |

**Epic Games Store**

| Variable | Default | Description |
|---|---|---|
| `LEGENDARY_WORK_DIR` | — | Writable directory where Epic sign-ins and syncs keep Legendary's files while they run (each operation removes its own when it finishes). **Epic sync is disabled while this is unset** (see [Epic Games Store sync](#epic-games-store-sync)). |

**Observability**

| Variable | Default | Description |
|---|---|---|
| `OTEL_SERVICE_NAME` | `nexorious` | The `service.name` attached to exported metrics. Set a distinct name per environment (e.g. `nexorious-staging`) if you scrape several instances into one Prometheus. |
| `OTEL_METRICS_ENABLED` | `true` | Expose Prometheus metrics at `/metrics`. Set `false` to remove the endpoint entirely. |
| `PPROF_ENABLED` | `false` | Start a loopback-only profiling listener for on-demand heap/goroutine/CPU profiles. Off by default; see [Monitoring and operations](#monitoring-and-operations). |
| `PPROF_ADDR` | `127.0.0.1:6060` | Bind address for the pprof listener when enabled. Keep it on loopback — it is unauthenticated and must never be reachable from outside the host/pod. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | Set to an OTLP/HTTP collector endpoint (e.g. `http://collector:4318`) to enable trace export. Unset = tracing off entirely, with no overhead. Only this generic variable enables tracing (the signal-specific `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` alone does not). Sampling and batching follow the standard OTel env vars (`OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`, `OTEL_BSP_*`). |

The defaults are fine for most deployments — a typical setup only sets the database connection, `DB_ENCRYPTION_KEY`, the two IGDB variables, and possibly `STORAGE_PATH`/`BACKUP_PATH` and `SESSION_COOKIE_SECURE`.

## First run

1. **Apply migrations.** The database schema is created and updated by migrations. By default the running server (`serve`) never applies them itself — it's a separate step — but how that step happens depends on how you deploy:
   - **Helm** runs it for you: the chart's `migrate` initContainer applies anything pending before the server starts, so there's nothing manual to do (see [Kubernetes / Helm](#kubernetes--helm)).
   - **Single-binary or single-container setups** can opt in with `nexorious serve --migrate`: one command that applies anything pending and then serves, with no extra orchestration. It waits up to 30 seconds for the database to become reachable, and if migrations fail the server refuses to start rather than run against a broken schema. With no pending migrations it's a no-op, so the flag is safe to leave on permanently.
   - **Otherwise**, you run it yourself — either from the command line with `nexorious migrate` (and `nexorious migrate status` to preview what's pending), or through the web UI: when the server comes up against a schema that's behind, it holds back the normal interface and serves a migration page at `/migrate`, where you click to run them and watch live progress.

   Until the schema is up to date, the server gates every route other than the migration page.

   For a package install you'll have set `DATABASE_URL` and run `systemctl enable --now nexorious` (see [Native packages](#native-packages-debianubuntu-rhelfedorarocky)); from there migrations work the same as any other deployment — `nexorious migrate` on the host, or the `/migrate` page the running service serves.

2. **Create the first admin.** With no users yet, opening the site prompts you to create the first account, which is automatically an admin. You can also create it with `nexctl setup admin` — handy for scripted or headless installs. `nexctl` ships inside the container image, so you can run it via `kubectl exec`/`docker exec` on a fresh instance.

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

Most storefront syncs need nothing from you as the operator — each user supplies their own credentials from their profile, and Nexorious talks to the store's API directly. **Epic is the exception:** it works through the `legendary` command-line tool (from the legendary-gl project), so `legendary` has to be available to the server, and you point Nexorious at a writable directory for its temporary files with `LEGENDARY_WORK_DIR`. While `LEGENDARY_WORK_DIR` is unset, Epic sync is disabled and users can't connect an Epic account.

Getting `legendary` in place depends on how you deploy:

- **Official container image** — `legendary` is already bundled in the image, so there's nothing to install. Just set `LEGENDARY_WORK_DIR` to a writable path. The provided `deploy/docker/docker-compose.yml` already does this (it sets `LEGENDARY_WORK_DIR` and mounts a volume for it), so Epic sync works out of the box there.
- **Single binary / from source** — install `legendary` (the legendary-gl package) on the host yourself, so the `legendary` command is on the server's `PATH`, then set `LEGENDARY_WORK_DIR`.
- **Native package (`.deb`/`.rpm`)** — `legendary` is **not** bundled in the packages (it is a pip tool with no distro package). Install it yourself (`pipx install legendary-gl` or `pip install --user legendary-gl`) so `legendary` is on the service's `PATH`, then set `LEGENDARY_WORK_DIR`. Epic sync degrades gracefully — everything else works — while it is absent.

Either way, point `LEGENDARY_WORK_DIR` at a writable directory and restart the server. It doesn't need to persist: the Epic sign-in is stored (encrypted) in the database, and the directory only holds files while a sign-in or sync runs. Steam, PlayStation, GOG, and Humble Bundle sync have no such requirement.

## Amazon Games sync

Amazon Games sync works through the `nile` command-line tool ([imLinguin/nile](https://github.com/imLinguin/nile), the Amazon Games client Heroic uses), version 1.2.0 or newer. It needs no directory or setting: each run uses a temporary directory that is deleted afterwards, and the sign-in is kept encrypted in the database. While `nile` is not on the server's `PATH`, Amazon sync is disabled and users can't connect an Amazon account. A sign-in in progress is carried by the browser, encrypted with `DB_ENCRYPTION_KEY`, so it finishes on any replica and survives a restart; changing that key during a sign-in means starting it again. Disconnecting Amazon also deregisters that device from the Amazon account when it can.

- **Official container image**: `nile` is bundled, so Amazon sync works out of the box. It carries one fix to nile 1.2.0 (`deploy/docker/nile/`): unpatched, nile accepts a failed last page of the library as if it were complete, and a sync would then mark the games on that page as removed.
- **Single binary / from source, or native package (`.deb`/`.rpm`)**: install nile 1.2.0 yourself, from its tagged source with the same patch, so `nile` is on the service's `PATH`. nile has no PyPI release, and its prebuilt binaries cannot take the patch. It goes in its own virtual environment (Debian and Ubuntu need the `python3-venv`, `patch` and `curl` packages), linked onto `PATH`. From a checkout of this repository, as root; the chain stops at the first step that fails:

  ```sh
  curl -fsSL -o nile.tar.gz https://github.com/imLinguin/nile/archive/refs/tags/v1.2.0.tar.gz \
    && echo "a06dd595248f0b4bf8fccf2b14a534391c94b810f725b6bbeaca92bc3ac2673d  nile.tar.gz" | sha256sum -c - \
    && tar -xzf nile.tar.gz && rm -rf nile-1.2.0/assets \
    && patch -d nile-1.2.0 -p1 < deploy/docker/nile/1.2.0-library-status.patch \
    && python3 -m venv /opt/nile \
    && /opt/nile/bin/pip install ./nile-1.2.0 \
    && ln -sf /opt/nile/bin/nile /usr/local/bin/nile
  ```
- **Nix**: the package puts nile 1.2.0 on the server's `PATH`, with the same patch, so Amazon sync works out of the box. (The nixos-26.05 channel ships nile 1.1.2, which keeps its sign-in in a format Nexorious does not read, so the package builds 1.2.0 itself.)

## MCP endpoint for AI agents

The server serves the [Model Context Protocol](https://modelcontextprotocol.io) at `/api/mcp`, so users can connect AI chats and agents to their collection. The [User Guide](user-guide.md#connecting-ai-chats-and-agents) covers the client side: connection settings, key scopes, and what agents can and can't do.

- **Authentication.** Requests must carry a user's API key as `Authorization: Bearer <key>`. The login cookie is never accepted here. A read-scoped key is only offered the read tools; any change needs a write-scoped key.
- **Turning it off.** Set `MCP_ENABLED=false` (Helm: under `controllers.nexorious.containers.main.env` in your values; NixOS: `services.nexorious.mcp.enable = false`). The route is then not registered at all, and the server sends no MCP-specific reply: a `POST /api/mcp` gets a generic `405` and a `GET` gets `404`. `nexctl mcp serve` reports either one as "this server has no MCP endpoint (disabled via MCP_ENABLED=false, or the server is too old)".
- **Browser origins.** Browser-based clients send an `Origin` header. The server accepts the request only if that origin's host matches the request's `Host`, or the origin is listed in `CORS_ORIGINS`; anything else gets `403`. Desktop and command-line clients send no `Origin` and aren't affected.
- **Reverse proxies.** The endpoint is plain, stateless request/response JSON (no long-lived streams), so a proxy needs no special timeouts or buffering settings. It must pass the original `Host` header through (in nginx, `proxy_set_header Host $host;`). A proxy that rewrites `Host` makes the origin check fail for browser-based clients, unless their origin is in `CORS_ORIGINS`.

## Why sync and import take a while

Syncs and imports are paced deliberately, so a first run over a large library isn't instant. Every game brought in is looked up in IGDB for its metadata, and both IGDB and the storefront APIs cap how many requests they accept per second. Nexorious throttles its outbound calls to stay within those published limits — exceeding them risks HTTP 429s and temporary blocks that would only slow things down further. This is expected behaviour, not a misconfiguration, and it's the answer when a user asks why a large sync is taking a while.

The defaults already match the providers' limits, so there's normally nothing to change. The rates and burst allowances are tunable if you have a reason — see `IGDB_REQUESTS_PER_SECOND`, `IGDB_BURST_CAPACITY`, `STEAM_REQUESTS_PER_SECOND`, and `STEAM_BURST_CAPACITY` in the [configuration reference](#full-reference). Lower them only if asked to; raising them past what a provider allows is counterproductive.

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
- **Restore** — restoring replaces the instance's current data with the backup's, so it's guarded by a confirmation and automatically takes a fresh "pre-restore" backup first, giving you a way back if a restore wasn't what you wanted. See [What a restore does](#what-a-restore-does).

Backups and restores are produced by the standard PostgreSQL client tools — `pg_dump` writes each backup and `psql` applies a restore — so both have to be available to the server. As with `legendary`, this depends on how you deploy:

- **Official container image / Helm** — the postgres client tools are already bundled in the image, so there's nothing to install.
- **Single binary / from source** — install the PostgreSQL client package (`postgresql-client`, ideally matching your server's major version) on the host yourself, so `pg_dump` and `psql` are on the server's `PATH`.

If they're missing, Nexorious starts normally but disables this whole section — backup creation and restore both return an error, and the startup log notes `pg_dump not found` / `psql not found`.

A backup holds your data, not the background work that was queued when it was taken: queued and running background jobs are left out. Notifications that were still waiting to be sent at that moment are therefore not sent after the backup is restored.

Neither tool may run forever, but the limit grows with your data: `pg_dump` gets 5 minutes plus 2 seconds for every MiB the database takes up, and a restore's `psql` gets 5 minutes plus 2 seconds for every MiB of the backup's `database.sql`. A run that goes over is stopped, and the backup or restore fails with "timed out". A restore on a running instance starts with a pre-restore backup, so a dump that times out there stops the restore before anything has changed.

A restore loads the backup's `database.sql` with `psql` in restricted mode, so psql commands in a backup file (such as `\!`, which runs a shell command) are refused and abort the restore; a backup can only change the database. This needs a `psql` with the `\restrict` command: PostgreSQL 18, or 17.6, 16.10, 15.14, 14.19, 13.22 or later in an older major version. With an older `psql`, restore fails with "psql is too old to restore safely". Backup files made by earlier Nexorious versions still restore. A restore runs as one transaction: if any statement in the backup fails, nothing from it is kept, and Nexorious then rolls back to the pre-restore backup when one was taken.

The SQL in a backup still runs as Nexorious's database role, so **the role in `DATABASE_URL` must not be a PostgreSQL superuser**. See [Database role](#database-role).

Keeping backups somewhere off the server, especially before upgrades, is the safest habit while the project is still moving quickly.

#### What a restore does

A restore first checks what it can without changing anything: that the backup file is intact (its checksums match), that it was made by a version this server can restore, that `pg_dump` is there for the pre-restore backup, and that the database role may create the schema `nexorious_restore`, where the restore records its progress. If any of that fails, the restore is refused and nothing changes.

Otherwise it goes on, in this order:

1. The server goes into maintenance mode. Requests other than `/health` and the backup pages answer "restore in progress", and backups, backup deletions, retention clean-ups and changes to the backup schedule are refused until the restore is over.
2. All background work stops at once, without waiting for it to finish. If it has not stopped within 30 seconds, the restore gives up with nothing restored.
3. The pre-restore backup is taken (not for a restore from the setup screen of a fresh install, where there is nothing to keep). If it fails, nothing is restored.
4. The database is replaced with the backup's. If loading the backup fails, the pre-restore backup is loaded back. A failed restore from the setup screen leaves an empty database, back at the setup screen.
5. The restored database is migrated if it is older than the server.
6. The server answers the request and then **restarts itself**, however the restore ended, success or failure. It keeps its process id, so Docker, Kubernetes and systemd see no exit; it is unreachable for a few seconds while it starts again. If the restart itself fails, the process exits and relies on your supervisor to start it again: the Compose file sets `restart: unless-stopped` for this.

**Work that was running or waiting is cancelled, not resumed**, whether the restore succeeded, failed or was rolled back. After the restart no Job is active: every Job that was pending or running is shown as failed with "interrupted by a restore". In practice:

- an import has to be uploaded again, including one that was waiting for you to review matches;
- an export has to be started again;
- an interrupted sync is picked up by the next sync, which also raises again any games that were waiting for review.

The restored database has its own sessions, so you usually have to log in again.

The admin page, the setup screen and `nexctl backup restore` / `nexctl setup restore` wait for the server to come back before they report the result. The browser then moves on to the login page (or the setup screen, after a failed restore from it) and shows the result there. They keep waiting as long as the server reports a restore still in progress, even when a proxy has given up on the request, and give up two minutes after it stops doing so. Then check the server logs.

Do not stop the server while a restore runs. A load cut off that way leaves the database in the state described below.

The restore needs room in the server's temporary directory (`/tmp`): it extracts the backup there and, while that is still there, stages the pre-restore backup (a database dump and a copy of the cover art). If it has to roll back, it extracts the pre-restore backup there as well. An uploaded backup file also passes through it. Allow roughly twice the uncompressed size of a backup, plus the upload. The Helm chart limits `/tmp` to 2 GiB; raise `nexorious.tmpDir.sizeLimit` if your backups are larger.

#### When a restore did not finish

A restore records its progress in the database, in a table of its own, `nexorious_restore.marker`, which the restore itself never drops. If a restore failed and could not be rolled back, or the server stopped in the middle of one, the row says `restoring`: the database may be only partly loaded. Nexorious then refuses to use it until you have dealt with it, across restarts:

- The server shows a **Restore Did Not Finish** page naming the backup that was being restored and the pre-restore backup, if one was taken. `/health` reports the status `restore_failed`. Every other page and API route is refused, the migration page included.
- The server does not migrate the database and does not start background work, with or without `serve --migrate`.
- `nexorious migrate` exits with an error naming the marker. On Helm, which runs it in an init container, the pod therefore stays in its init phase and the page is never served: look at the init container's log. `nexorious migrate status` prints `state=restore_failed` and changes nothing.

To recover, connect to the database with `psql` as the role in `DATABASE_URL` and look at the row:

```sql
SELECT * FROM nexorious_restore.marker;
```

If the data needs reloading, load a backup by hand. `pre_restore_backup_id` names the backup taken just before the restore, at `$BACKUP_PATH/<id>.tar.gz`; any other backup works too. The archive holds a directory named after the backup, with `database.sql` and `cover_art/` in it:

```sh
mkdir /tmp/recover && tar -xzf "$BACKUP_PATH/<id>.tar.gz" -C /tmp/recover
psql "$DATABASE_URL" -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
psql "$DATABASE_URL" --single-transaction -v ON_ERROR_STOP=1 -f /tmp/recover/<id>/database.sql
```

Then copy `/tmp/recover/<id>/cover_art/` over `$STORAGE_PATH/cover_art/`.

When the database is in the state you want, **set the phase to `cleanup`** and restart the server:

```sql
UPDATE nexorious_restore.marker SET phase = 'cleanup';
```

Always set `cleanup`; never delete the row. On the next start, before the server reports ready, Nexorious clears whatever was in flight when the restore began: it empties the background queue and marks every Job that was still pending or running as failed with "interrupted by a restore", cancelling its unfinished items. Then it deletes the row and starts as usual, offering the migration page if the loaded backup is older than the server.

### Database role

A restored backup is SQL that runs as the role in `DATABASE_URL`. A PostgreSQL superuser can run programs on the database host from plain SQL (`COPY … TO PROGRAM`), so if Nexorious connects as one, a crafted backup file could run commands there. The same goes for a role in `pg_execute_server_program` or `pg_write_server_files`. Nexorious needs none of that: a plain role that owns its database is enough, migrations included.

When Nexorious is connected as such a role, it logs a warning at startup, and admins see a banner in the web interface until it's fixed.

How each deployment handles it:

- **Docker Compose and the Helm chart's bundled PostgreSQL** — the official `postgres` image creates `POSTGRES_USER` (`nexorious`) as a superuser. Nexorious keeps that role for administration only. Before every start, `nexorious ensure-db-role` (the Compose `db-role` service, or the chart's `db-role` initContainer) connects as it and makes `nexorious_app` a non-superuser role that owns the database and everything in it. The app connects as `nexorious_app`, with the same `POSTGRES_PASSWORD`. Existing installs are taken over the same way on their first start after upgrading, with no manual steps. In the Helm chart the role name is `nexorious.postgresql.appUsername`.
- **NixOS module** — its local database already uses a plain owner role.
- **The Helm chart's CloudNativePG option** (`cnpg.enabled`) — CloudNativePG creates `nexorious_app` as a plain role that owns the database, and the chart connects with the cluster's `-app` Secret. Nothing to do.
- **Your own PostgreSQL** (including a CloudNativePG cluster you created yourself) — give Nexorious a role that owns its database and is not a superuser. With CloudNativePG, use the cluster's `-app` Secret, not `-superuser`.

`nexorious_app` shares its password with the `nexorious` superuser. SQL in a backup can't use that, but anyone holding the app's credentials could also log in as the superuser. To keep them apart, give the superuser its own password, or clear it so it can only log in from inside the database container:

```bash
docker compose exec db psql -U nexorious -c "ALTER ROLE nexorious PASSWORD NULL"
# Helm: kubectl exec -it <release>-postgresql-0 -- psql -U nexorious -c "ALTER ROLE nexorious PASSWORD NULL"
```

If you change `POSTGRES_PASSWORD` later, `ensure-db-role` updates `nexorious_app`'s password on the next start; the superuser's password is yours to manage.

### Fixing the database role warning

If Nexorious warns that its database role is a superuser, follow the steps for how you run it. When you're done, restart Nexorious: the startup warning stops and the banner goes away on the next page load.

**Docker Compose with the bundled PostgreSQL**

1. Replace your `docker-compose.yml` with the one from the release you run (`deploy/docker/docker-compose.yml`). It adds a `db-role` service and connects the app as `nexorious_app`. If you've customised your compose file, make those two changes by hand: copy in the `db-role` service, and in the `app` service's `DATABASE_URL` change the user from `nexorious` to `nexorious_app`.
2. Run `docker compose pull && docker compose up -d`.

Your `.env` file doesn't change.

**Helm with the bundled PostgreSQL**

1. Upgrade to the current chart with `helm upgrade`.
2. If you set `nexorious.databaseUrl`, `nexorious.databaseUrlFrom` or the `nexorious.db*From` values, the chart doesn't manage the role for you. Remove them so the chart builds the connection itself, or follow the steps for your own PostgreSQL below.

**CloudNativePG**

With the chart's `cnpg.enabled` option the chart already uses the `-app` Secret. For a cluster you created yourself, point `nexorious.databaseUrlFrom` at the cluster's `<cluster>-app` Secret, not `<cluster>-superuser`.

**Your own PostgreSQL (native packages, single binary, an external server)**

1. Connect to the database as an admin and remove the extra rights from the role Nexorious uses (here `nexorious`):

   ```sql
   ALTER ROLE nexorious NOSUPERUSER;
   REVOKE pg_execute_server_program, pg_write_server_files FROM nexorious;
   ```

   A warning that the role "has not been granted membership" is fine: it just didn't have that right.

2. If PostgreSQL refuses with "The bootstrap superuser must have the SUPERUSER attribute", that role created the database cluster and can't be changed. Make Nexorious a new role instead. `nexorious ensure-db-role` creates it and hands it the database:

   ```bash
   DATABASE_URL='postgres://nexorious_app:NEW_PASSWORD@DB_HOST:5432/nexorious' \
   DB_ADMIN_USER=nexorious DB_ADMIN_PASSWORD='CURRENT_PASSWORD' \
     nexorious ensure-db-role
   ```

   Then set `DATABASE_URL` in your configuration to the same `nexorious_app` URL. If the password has characters like `@`, `:` or `/`, URL-encode them.

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

Nexorious has two command-line programs. The **`nexorious`** binary is the server, and it doubles as a host-side tool for deployment tasks (running the server, applying migrations, bootstrapping the first admin). The separate **`nexctl`** client talks to a running server over its API and is what you use day-to-day — including for the operator tasks below. Run `<program> <command> --help` for the full details of any command.

### The `nexorious` server binary

These run on the server host. A `--config` flag on every command points at a `.env` file if you keep your settings in one.

| Command | What it does |
|---|---|
| `nexorious serve` | Start the HTTP server, background workers, and scheduler. This is the main run command. Add `--migrate` to apply any pending migrations on the way up — startup aborts if they fail, rather than serving against a broken schema. |
| `nexorious migrate` | Apply any pending database migrations and exit. Useful as an init step in orchestrated deployments. |
| `nexorious migrate status` | Show how many migrations are pending and the current schema version, without changing anything. |
| `nexorious ensure-db-role` | Connect as a PostgreSQL superuser (`DB_ADMIN_USER` / `DB_ADMIN_PASSWORD`) and make the role in `DATABASE_URL` a non-superuser that owns the database. Safe to run on every start; the Compose file and Helm chart run it for you (see [Database role](#database-role)). |
| `nexctl setup admin` | Create the first admin user against a running server — good for headless installs. Supports `--username` and reading the password from stdin (`--password-stdin`), runs pending migrations first, and can store an API key for you with `--login`. `nexctl` is available in the container image. |
| `nexorious reset-password <username>` | Reset a user's password by talking to the database directly. Your recovery path if an admin is locked out and no other admin can help. |
| `nexorious version` | Print version information. |

### `nexctl`, the API client

`nexctl` is a separate program that drives a running server over its API; authentication, profiles, and the everyday command groups (`game`, `pool`, `tag`, `sync`, `import`/`export`) are covered in the [User Guide](user-guide.md#using-nexctl-the-command-line-client). It ships with each release as its own raw binary plus `.deb` / `.rpm` packages, and it is **bundled in the server container image** (at `/usr/local/bin/nexctl`, so `docker exec`/`kubectl exec … nexctl …` works out of the box) — but it is **not** in the Helm chart or the NixOS module, so on those install it wherever you run commands from (with the native packages it's a separate download from the server package).

Two command groups are operator-only and require an **admin** user's key — note that an API key carries no admin scope, so these need a key minted for an admin account (a non-admin key gets a `403` from the server):

| Command | What it does |
|---|---|
| `nexctl admin user list \| show \| create \| enable \| disable \| set-admin \| passwd \| rm` | Manage users from the terminal — the same operations as the [User Management](#user-management) page. `user rm` shows the deletion impact before confirming. |
| `nexctl admin reset` | The instance [database reset](#database-reset). The loudest confirmation in the client; mirrors the danger-zone action in the UI. |
| `nexctl backup list \| create \| rm \| download \| restore \| schedule set` | Manage [backups](#backups-and-restore): create one synchronously, download or remove backups, restore from a backup id or an uploaded file (a loud destructive confirm), and set the backup schedule. |

Per-user settings live under **`nexctl config`** (deal region, notification channels and subscriptions) — these aren't admin-only; any user manages their own.

`nexctl mcp serve` connects AI clients that only speak stdio to the server's [MCP endpoint](#mcp-endpoint-for-ai-agents): it relays each message to `/api/mcp` with the active profile's API key and hosts no tools of its own. `nexctl mcp config` prints ready-made client entries for both the stdio route and a direct HTTP connection. The operator groups (`admin`, `backup`) are deliberately not exposed over MCP.

## Monitoring and operations

- **Logs** — Nexorious logs to standard output; set `LOG_LEVEL=debug` when you need more detail. With the native packages it runs under systemd, so its output lands in the journal — `journalctl -u nexorious` (add `-f` to follow). In Docker or Kubernetes, collect stdout however you collect logs from anything else.
- **Metrics** — Nexorious exposes Prometheus metrics at `/metrics` (on by default; disable with `OTEL_METRICS_ENABLED=false`). The endpoint is unauthenticated and carries no secrets — its labels are bounded (storefront, status, outcome; never per-user). It stays scrapable even while the app is starting up, migrating, or in restore-maintenance mode, so a scrape never flaps the target. Alongside Go SQL/database query metrics you get River job metrics (the `river_work_*` family — job counts and durations) and the sync-outcome business metrics `nexorious_sync_total{source,status}` and `nexorious_sync_items_total{source,outcome}`. The River and `nexorious_sync_*` series are registered lazily, so they appear once the first background job (respectively, the first sync) has run. Point any Prometheus-compatible scraper at `http://<host>:<PORT>/metrics`.
- **Tracing** — set `OTEL_EXPORTER_OTLP_ENDPOINT` to an OTLP/HTTP collector (e.g. `http://collector:4318`) and Nexorious exports traces: each background job becomes one waterfall — the River job at the root, outbound storefront/IGDB API calls beneath it, and the individual database queries under those. Outbound requests also carry a `traceparent` header, and every log line written inside a traced job includes matching `trace_id`/`span_id` fields, so you can jump from a log line to the exact trace. Everything is sampled by default; dial it down with the standard `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` env vars. Two things to know: queries and API calls made while serving ordinary HTTP requests appear as small standalone traces (there is deliberately no HTTP-server span), and span attributes carry no secrets — SQL statements are recorded as templates with placeholders, never argument values, and query strings are stripped from recorded request URLs. Leave the variable unset and tracing is off entirely.
- **Profiling (pprof)** — for diagnosing memory or goroutine problems (for example an unexplained OOM under a tight memory limit), set `PPROF_ENABLED=true` to start a loopback-only `net/http/pprof` listener on `PPROF_ADDR` (default `127.0.0.1:6060`). It is intentionally **not** exposed publicly and is unauthenticated, so reach it from your workstation by tunnelling in first — e.g. `kubectl port-forward <pod> 6060:6060`, then `go tool pprof http://localhost:6060/debug/pprof/heap` (or `.../goroutine`). Leave it off in normal operation.
- **Version** — `nexorious version` on the host, and the running version is shown in the app's sidebar, so you can confirm what's actually deployed.
- **Notifications** — Nexorious's notifications are configured per user (in each user's profile), not centrally. As an operator you mainly care that the server can reach whatever channels users configure (for example outbound network access for email or webhooks).

## Upgrades and versioning

Schema changes ship as migrations, applied as a step separate from the running server. On Helm, the chart's `migrate` initContainer handles this automatically on each deploy. On other deployments you apply them yourself — run `nexorious migrate`, start the server with `serve --migrate`, or let the server come up against the new schema and use the `/migrate` page it serves. So a normal upgrade is: take a backup, deploy the new version, make sure migrations are applied (automatic on Helm; `nexorious migrate`, `serve --migrate`, or the `/migrate` page otherwise), done.

With the `.deb`/`.rpm` packages, a normal `apt upgrade` / `dnf upgrade` replaces the binary and preserves your `/etc/nexorious/nexorious.env` — the env file is a conffile (`%config(noreplace)` on rpm), so your settings and the generated `DB_ENCRYPTION_KEY` survive the upgrade, and the service is restarted only if it was already running. Removal differs by family: `apt purge` deletes `/var/lib/nexorious` **including backups** and removes the `nexorious` user (a true clean uninstall), while a plain `apt remove` and any `dnf` uninstall leave config, key, and data in place for a later reinstall (rpm keeps a modified env file as `nexorious.env.rpmsave`).

### Upgrading to v0.90.0+ (the migration squash)

v0.90.0 collapses the early migration history into a single baseline. To upgrade
safely you must first be on **v0.17.1** with its schema fully migrated, then move
to v0.90.0 or any later version. On a fully-migrated v0.17.1 database the migrator
**adopts** the existing schema in place — it rewrites the migration history to the
baseline and applies anything newer, with **zero data loss** (collections, pools,
users, API keys, sync connections and credentials, notification channels and
subscriptions, and settings are all preserved).

If the database is older than v0.17.1, partially migrated, or otherwise in an
unrecognized state, the upgrade is **refused** rather than risking corruption:
- The web `/migrate` page shows a refusal with these instructions instead of a run button.
- `nexorious migrate` and `serve --migrate` exit non-zero (so a Helm `migrate`
  initContainer fails loudly rather than starting against a broken schema).

To recover a refused database, first upgrade it to v0.17.1 and let it migrate
fully, then upgrade to v0.90.0+. If it's in an inconsistent state and can't reach
v0.17.1, use the export/import fallback: export each user's collection (JSON
export from Import / Export), start fresh with an empty database on v0.90.0+, and
import. Export preserves **games** (status, rating, loved/wishlisted, notes,
platforms, tags) and **pools** (filters + members). After a fresh install + import
you must redo: users & passwords (re-run setup), API keys (re-mint), sync
connections / storefront credentials (`sync connect`), notification channels &
subscriptions, user settings (deal-region, etc.), the backup schedule, and sync
state / external-game match history (rebuilt by re-running syncs).

**The one big caveat is the 1.0.0 release.** The first stable release will be a deliberate clean break with **no automatic upgrade path** from the pre-1.0 versions. To move onto 1.0.0 you'll export each user's collection (JSON export from Import / Export), start fresh with an empty database, and import again. Until 1.0.0 lands, keep this in mind and don't treat a pre-1.0 instance as permanent storage — keep exports of anything you'd be sad to re-enter.

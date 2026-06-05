# Nexorious

> [!WARNING]
> **Work in Progress — Not Ready for Use**
> Nexorious is under active development and is not ready for production use or general adoption. Expect breaking changes, missing features, incomplete documentation, and rough edges. Use at your own risk.
>
> **Heads up about version 1.0.0:** The first stable release will be a clean break. When 1.0.0 lands, there will be no automatic upgrade path. To move to it you'll need to export any games you've added, start over with a fresh, empty database, and import your games again. Keep this in mind before you put a lot of data into Nexorious in the meantime.

A self-hosted web application for managing personal video game collections with comprehensive IGDB integration for tracking, organizing, and discovering games across multiple platforms and storefronts.

Nexorious was inspired by [Darkadia](https://darkadia.com) (RIP), a beloved game collection tracker that is no longer operating.

## What Nexorious is — and isn't

Nexorious is a self-hosted catalog and tracker for the games you own across many platforms and storefronts — Steam, Epic, GOG, PlayStation, Xbox, Nintendo, physical media, and more — gathered into one searchable source of truth and enriched automatically with metadata from IGDB. Increasingly, it is also a place to track play status and work through your backlog.

It is **not** a launcher or a library manager. Nexorious never installs, downloads, launches, or plays your games, and it does not touch your save files or game files. If that is what you are after, look at [Playnite](https://playnite.link/), [Heroic](https://heroicgameslauncher.com/), or [Lutris](https://lutris.net/).

## Reasons to use Nexorious

- **Your library is scattered across storefronts.** If you buy games on Steam *and* Epic *and* GOG *and* PlayStation *and* pick up the odd physical copy, Nexorious gives you one consolidated, searchable view of everything you own. This is its core value.
- **Stop buying games you already own.** Before grabbing that bundle or sale, check Nexorious to see whether — and on which storefront — you already own the game.
- **Search, don't type.** Add a game by searching for it — Nexorious pulls everything from IGDB automatically: cover art, descriptions, release dates, genres, ratings, and time-to-beat estimates. Your collection looks complete without manual data entry.
- **Own your data.** Self-hosted, single binary, no third-party cloud, MIT-licensed.
- **Automatic library sync** from Steam, PlayStation Network, GOG, and Epic Games Store today, with more sources on the way.
- **Migrating from Darkadia?** Darkadia (RIP) is gone, but your collection doesn't have to be. Export your Darkadia library to CSV and import it straight into Nexorious — games, platforms, ratings, notes, and the date you added each game all come across.

## Reasons not to use Nexorious

- **You want to launch or play your games.** Nexorious is a catalog, not a launcher — use a launcher like Playnite, Heroic, or Lutris instead.
- **You only use a single storefront.** If everything you own is on Steam (or only PlayStation, or only Xbox), that storefront already shows you your whole library — cross-platform consolidation will not buy you much.
- **You want zero-ops or a hosted service.** There is no SaaS. You run the server and a PostgreSQL database yourself.
- **You're uncomfortable with AI-assisted code.** Nexorious is built extensively with it — see [AI-Assisted Development](#ai-assisted-development).

## Alternatives

- **Hosted web trackers** — [Backloggd](https://www.backloggd.com/), [Grouvee](https://www.grouvee.com/), [Completionator](https://www.completionator.com/), and [HowLongToBeat](https://howlongtobeat.com/) are the closest peers in what they do. They are zero-ops and often social, but they are cloud-hosted rather than self-hosted, and you do not own the data.
- **[Playnite](https://playnite.link/)** — an open-source desktop library aggregator, similar in spirit, but it *is* a launcher and runs only on the Windows desktop rather than as a self-hosted web app.
- **Spreadsheets / Notion** — the DIY default. Total control, but everything is manual: no library sync and no metadata enrichment.

## Features

- **Multi-Platform Game Tracking**: Support for Steam, Epic Games Store, PlayStation, Xbox, Nintendo, GOG, and physical media
- **Sync Integrations**: Automatic library sync from Steam, PlayStation Network (PSN), GOG, and Epic Games Store (via legendary-gl)
- **Rich Game Discovery**: Search and import games from IGDB's extensive database with automatic metadata population
- **Progress Tracking**: Track play status, personal ratings, time played, and detailed notes
- **Import & Export**: Export your collection to JSON or CSV; import from Nexorious's own JSON format or a Darkadia CSV export
- **Single Binary**: Go backend with embedded React SPA — one process serves everything
- **Modern Tech Stack**: Go backend with React + Vite frontend

## Quick Start

### Prerequisites

- **Go 1.26+**
- **Node.js 24+** with npm
- **PostgreSQL 16+**
- **Nix + devenv** (optional, recommended for reproducible development environment)
- **IGDB API Credentials** (required) — see [IGDB Setup Guide](docs/igdb-setup.md)

### Development Setup

#### Option 1: Using devenv (Recommended)

```bash
git clone https://github.com/drzero42/nexorious.git
cd nexorious
devenv shell       # enters reproducible shell with Go, Node, golangci-lint, make
devenv up -d       # starts PostgreSQL in the background
make               # builds frontend then Go binary
export DATABASE_URL="postgres:///nexorious"  # devenv uses a Unix socket
./nexorious serve  # starts server on :8000; visits /migrate if schema is pending
```

#### Option 2: Manual Setup

```bash
git clone https://github.com/drzero42/nexorious.git
cd nexorious
make               # builds frontend (npm ci + vite build) then Go binary
export DATABASE_URL="postgres://user:password@localhost:5432/nexorious"
./nexorious serve  # starts server; auto-runs migrations on first launch
```

### Initial Setup

1. **First Run**: Navigate to `http://localhost:8000` — you will be prompted to create an admin account
2. **Migrations**: Run automatically on startup; the app gate-keeps all routes until the schema is ready
3. **IGDB Credentials**: Configure via the admin settings page after first login

## Production Deployment

### Docker Compose

The simplest production-like deployment uses the published container image:

```bash
cp .env.example .env   # fill in DB_ENCRYPTION_KEY, IGDB_CLIENT_ID, IGDB_CLIENT_SECRET, POSTGRES_PASSWORD
docker compose -f deploy/docker/docker-compose.yml up -d
```

The app container automatically runs migrations on startup before serving traffic.

### Kubernetes / Helm

A Helm chart is published to `oci://ghcr.io/drzero42/charts` and is built on the [bjw-s common library](https://bjw-s-labs.github.io/helm-charts/).

```bash
helm install nexorious oci://ghcr.io/drzero42/charts/nexorious \
  --set nexorious.igdbClientId=YOUR_CLIENT_ID \
  --set nexorious.igdbClientSecret=YOUR_CLIENT_SECRET \
  --set nexorious.postgresql.password="$(openssl rand -hex 16)"
```

See `deploy/helm/values.yaml` for the full values reference.

### NixOS

The Nix flake exposes a package, an overlay, and a NixOS module. Two flake input URLs are available:

```nix
# Latest stable release — updates automatically with `nix flake update`
inputs.nexorious.url = "github:drzero42/nexorious/release";

# Bleeding edge (tracks main)
inputs.nexorious.url = "github:drzero42/nexorious";
```

Minimal NixOS configuration:

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

The environment file must contain:

```bash
DB_ENCRYPTION_KEY=...   # generate: openssl rand -base64 32
IGDB_CLIENT_ID=...      # from https://dev.twitch.tv/console
IGDB_CLIENT_SECRET=...
```

Key module options:

| Option | Default | Description |
|---|---|---|
| `services.nexorious.enable` | `false` | Enable the service |
| `services.nexorious.port` | `8000` | TCP port to listen on |
| `services.nexorious.database.createLocally` | `true` | Manage a local PostgreSQL instance automatically |
| `services.nexorious.database.name` | `"nexorious"` | Database name (when `createLocally = true`) |
| `services.nexorious.storagePath` | `/var/lib/nexorious` | Path for uploads and backups |
| `services.nexorious.environmentFile` | — | Path to the environment file with secrets |

When `database.createLocally = true` (the default), PostgreSQL is configured automatically and no `DATABASE_URL` is needed. To use an external database, set `database.createLocally = false` and add `DATABASE_URL=postgresql://...` to the environment file.

### Environment Variables

```bash
# Required
DATABASE_URL=postgres://user:password@host:5432/nexorious?sslmode=disable
DB_ENCRYPTION_KEY=your-db-encryption-key  # generate: openssl rand -base64 32
IGDB_CLIENT_ID=your-igdb-client-id
IGDB_CLIENT_SECRET=your-igdb-client-secret

# Optional
PORT=8000                                  # default: 8000
STORAGE_PATH=/path/to/storage             # default: ./storage
BACKUP_PATH=/path/to/backups              # default: ./storage/backups
LOG_LEVEL=info                             # default: info
SESSION_COOKIE_SECURE=true                 # default: true; set false to serve over plain HTTP (see below)
```

> **`SESSION_COOKIE_SECURE`** — controls the `Secure` flag on the session cookie.
> It defaults to `true`, which tells browsers to send the cookie only over HTTPS.
> Browsers treat `localhost` as a secure context, so login works over
> `http://localhost` even with the default. But if you reach the instance over
> **plain HTTP on any other host** (e.g. a LAN IP or hostname), the browser will
> drop the cookie and login will silently fail — set `SESSION_COOKIE_SECURE=false`
> in that case. Keep it `true` in production behind HTTPS.

### Production Checklist

- [ ] PostgreSQL configured
- [ ] `DATABASE_URL` set
- [ ] `DB_ENCRYPTION_KEY` set to a cryptographically random value
- [ ] IGDB API credentials configured
- [ ] Storage directory writable
- [ ] Backup procedures in place
- [ ] `SESSION_COOKIE_SECURE` left at `true` (HTTPS); set `false` only when serving over plain HTTP

## Development

### Key Commands

| Task | Command |
|------|---------|
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

### Project Structure

```
nexorious/
├── cmd/nexorious/        # Entry point — wires config, DB, Echo, workers
├── internal/
│   ├── api/             # Echo route handlers (games, auth, sync, import, export, …)
│   ├── db/              # Bun ORM models and SQL migrations
│   ├── worker/          # River job workers (sync, import, export, metadata)
│   ├── scheduler/       # Periodic maintenance jobs (cleanup, backup polling)
│   ├── services/        # IGDB client, Steam/PSN sync, game matching
│   ├── auth/            # Session and API key auth + Echo middleware
│   └── config/          # Environment variable config
├── ui/
│   ├── frontend/        # React + Vite SPA source
│   └── migrate/         # Standalone migration UI (embedded in binary)
├── deploy/
│   ├── helm/            # Helm chart (bjw-s common library)
│   └── docker/          # Docker Compose for simple deployments
└── docs/                # Documentation
```

### Tech Stack

- **Backend**: Go 1.26, Echo v5, Bun ORM, River job queue, pgx/v5
- **Frontend**: React 19, Vite 8, TypeScript, TanStack Router + Query, Tailwind CSS v4, shadcn/ui
- **Database**: PostgreSQL 16+
- **Testing**: stdlib `testing` + testcontainers-go (Go); Vitest + @testing-library/react (frontend)

### Database Migrations

Migrations live in `internal/db/migrations/` as SQL files named `YYYYMMDD<nnnnnn>_name.up.sql` / `.down.sql`, where `<nnnnnn>` is a zero-padded running number (e.g. `20260503000001_initial.up.sql`). They are discovered and run automatically on startup. To add a new migration:

```bash
# Create migration files (replace timestamp and name)
touch internal/db/migrations/20260101000001_my_change.up.sql
touch internal/db/migrations/20260101000001_my_change.down.sql
```

Check migration status without applying:

```bash
./nexorious migrate status
```

## Testing

### Go

```bash
go test -timeout 600s ./...

# Single package with verbose output
go test ./internal/api/... -run TestGamesList -v

# With coverage
go test -timeout 600s -cover ./...
```

Tests use testcontainers-go to spin up a real PostgreSQL container — no mocking the database. Requires Docker or Podman socket access.

### Frontend

```bash
cd ui/frontend

npm run test           # run all tests
npm run check          # TypeScript type check
```

## Sync Integrations

### Steam

Configure via the admin settings page: enter your Steam Web API key and Steam ID.

### PlayStation Network (PSN)

Configure via the admin settings page with your PSN NPSSO token.

### GOG

Configure via the admin settings page using the GOG OAuth flow: open the GOG login URL, sign in, and paste the resulting redirect URL (or authorization code) back into Nexorious. The stored refresh token is used to sync your library automatically thereafter.

### Epic Games Store

Requires the `legendary-gl` package installed (it provides the `legendary` command) and authenticated on the host. See [issue #45](https://github.com/drzero42/nexorious/issues/45) for container image support status.

## API Documentation

The route handlers in `internal/api/` are the source of truth for available endpoints — each domain (games, user_games, auth, platforms, tags, jobs, import, export, sync, …) has its own handler file with the registered routes.

See `DEV.md` for the full development guide including database reset procedures and the two-terminal Vite dev server workflow.

## AI-Assisted Development

Nexorious was built with extensive use of AI coding tools. AI assistance was used throughout the project for code generation, architecture decisions, debugging, and documentation.

This is an intentional choice — Nexorious is partly an experiment in what AI-assisted software development can produce. If you have strong objections to AI-generated or AI-assisted code, Nexorious may not be the right project for you.

## Trademarks and Copyright

All mentioned trademarks, brand names, and logos for gaming platforms and storefronts (including but not limited to PlayStation, Xbox, Nintendo, Steam, Epic Games Store, GOG, Apple App Store, Google Play Store, and others) are the property of their respective owners. These trademarks are used solely for identification and compatibility purposes.

The use of these trademarks and brand names does not imply any affiliation, endorsement, or partnership with the respective companies. All rights to these trademarks remain with their original owners.

The logos and icons used in this application are sourced from SVG Repo and other public repositories under various open-source licenses (MIT, CC0, Logo License, etc.).

## License

MIT License — see LICENSE file for details.

---

**Self-hosted game collection management made simple.**

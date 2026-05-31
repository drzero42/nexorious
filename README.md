# Nexorious

> [!WARNING]
> **Work in Progress — Not Ready for Use**
> Nexorious is under active development and is not ready for production use or general adoption. Expect breaking changes, missing features, incomplete documentation, and rough edges. Use at your own risk.

A self-hostable web application for managing personal video game collections with comprehensive IGDB integration for tracking, organizing, and discovering games across multiple platforms and storefronts.

Nexorious was inspired by [Darkadia](https://darkadia.com) (RIP), a beloved game collection tracker that is no longer operating.

## Features

- **IGDB-Only Game Database**: All games sourced from the Internet Game Database (IGDB) with comprehensive metadata, cover art, ratings, and completion time estimates
- **Multi-Platform Game Tracking**: Support for Steam, Epic Games Store, PlayStation, Xbox, Nintendo, GOG, and physical media
- **Sync Integrations**: Automatic library sync from Steam, Epic Games Store (via legendary-gl), and PlayStation Network (PSN)
- **Rich Game Discovery**: Search and import games from IGDB's extensive database with automatic metadata population
- **Progress Tracking**: Track play status, personal ratings, time played, and detailed notes
- **Bulk Operations**: Import from CSV exports (Darkadia format) with intelligent conflict resolution
- **Single Binary**: Go backend with embedded React SPA — one process serves everything
- **Modern Tech Stack**: Go backend with React + Vite frontend

## Quick Start

### Prerequisites

- **Go 1.25+**
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
./nexorious        # starts server on :8000; visits /migrate if schema is pending
```

#### Option 2: Manual Setup

```bash
git clone https://github.com/drzero42/nexorious.git
cd nexorious
make               # builds frontend (npm ci + vite build) then Go binary
export DATABASE_URL="postgres://user:password@localhost:5432/nexorious"
./nexorious        # starts server; auto-runs migrations on first launch
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
```

### Production Checklist

- [ ] PostgreSQL configured
- [ ] `DATABASE_URL` set
- [ ] `DB_ENCRYPTION_KEY` set to a cryptographically random value
- [ ] IGDB API credentials configured
- [ ] Storage directory writable
- [ ] Backup procedures in place

## Development

### Key Commands

| Task | Command |
|------|---------|
| Build everything | `make` |
| Build backend only | `make build` |
| Build frontend only | `make frontend` |
| Run server | `./nexorious` or `./nexorious serve` |
| Run migrations only | `./nexorious migrate` |
| Migration status | `./nexorious migrate status` |
| Run Go tests | `go test -timeout 600s ./...` |
| Run frontend tests | `npm run test` (from `ui/frontend/`) |
| Type check frontend | `npm run check` (from `ui/frontend/`) |
| Lint Go | `golangci-lint run` |
| API client | `slumber` |

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

- **Backend**: Go 1.25, Echo v5, Bun ORM, River job queue, pgx/v5
- **Frontend**: React 19, Vite 6, TypeScript, TanStack Router + Query, Tailwind CSS v4, shadcn/ui
- **Database**: PostgreSQL 16+
- **Testing**: stdlib `testing` + testcontainers-go (Go); Vitest + @testing-library/react (frontend)

### Database Migrations

Migrations live in `internal/db/migrations/` as timestamped SQL files (`YYYYMMDDHHmmss_name.up.sql` / `.down.sql`). They are discovered and run automatically on startup. To add a new migration:

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

Configure via the admin settings page: enter your Steam Web API key and Steam ID. See the API documentation for full details.

### PlayStation Network (PSN)

Configure via the admin settings page with your PSN NPSSO token.

### Epic Games Store

Requires `legendary-gl` installed and authenticated on the host. See [issue #45](https://github.com/drzero42/nexorious/issues/45) for container image support status.

## API Documentation

The API is self-documenting. With the server running, explore endpoints via `slumber` (included in the devenv shell):

```bash
slumber
```

See `DEV.md` for the full development guide including database reset procedures and the two-terminal Vite dev server workflow.

## AI-Assisted Development

Nexorious was built with extensive use of AI tooling, specifically [Claude Code](https://claude.ai/code) by Anthropic. AI assistance was used throughout the project for code generation, architecture decisions, debugging, and documentation.

This is an intentional choice — Nexorious is partly an experiment in what AI-assisted software development can produce. If you have strong objections to AI-generated or AI-assisted code, Nexorious may not be the right project for you.

## Trademarks and Copyright

All mentioned trademarks, brand names, and logos for gaming platforms and storefronts (including but not limited to PlayStation, Xbox, Nintendo, Steam, Epic Games Store, GOG, Apple App Store, Google Play Store, and others) are the property of their respective owners. These trademarks are used solely for identification and compatibility purposes.

The use of these trademarks and brand names does not imply any affiliation, endorsement, or partnership with the respective companies. All rights to these trademarks remain with their original owners.

The logos and icons used in this application are sourced from SVG Repo and other public repositories under various open-source licenses (MIT, CC0, Logo License, etc.).

## License

MIT License — see LICENSE file for details.

---

**Self-hosted game collection management made simple.**

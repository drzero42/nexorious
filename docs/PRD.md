# Nexorious

> **Your entire game collection, one place.**

Nexorious is a self-hostable web application for managing a personal or household video game collection. It syncs automatically with Steam, PlayStation Network, and Epic Games Store so your library stays up to date without manual entry. All your data lives on your own infrastructure — no accounts, no cloud subscriptions, no tracking.

---

## What Nexorious Is Not

- Not a gaming client (you cannot launch games from it)
- Not a social platform (no public profiles, friends lists, or sharing)
- Not a game store (no purchasing or DRM integration)
- Not a replacement for platform-specific libraries (it reads from them, not instead of them)

---

## User Model

Nexorious is designed for a single household or small group — typically 1–5 users sharing one instance.

- **One admin** manages the instance: creates and manages all user accounts, configures platforms and storefronts, and runs maintenance tools. There is no multi-admin support and no self-registration.
- **Each user** has their own independent collection, sync configurations, tags, and wishlist.
- **Authentication**: username + password login; JWT tokens with refresh; bcrypt password hashing. No email required.

---

## Feature Inventory

### Collection

- Search IGDB for games and add them to your collection
- All games must originate from IGDB — no free-form manual entries
- Track ownership per platform and storefront (e.g. PS5 / PlayStation Store, PC / Steam)
- A game can be owned on multiple platforms and storefronts simultaneously
- Track progress status: Not Started, In Progress, Completed, Mastered, Dominated, Shelved, Dropped, No Longer Owned
- Personal notes (rich text via TipTap editor)
- Star rating (personal)
- User-defined tags
- Wishlist (games you want but don't own yet)
- Grid and list library views
- Search by title; filter by platform, storefront, status, tags, genre
- Bulk selection and bulk operations

### Sync

Nexorious syncs automatically with connected storefronts in the background. Sync runs on a schedule and can be triggered manually.

| Storefront | Status |
|---|---|
| Steam | Supported |
| PlayStation Network (PSN) | Supported |
| Epic Games Store | Supported |

Each sync source requires independent configuration and authentication. Background jobs handle sync; status and history are visible in the Jobs view.

### IGDB Integration

- Game search and metadata retrieval (title, description, cover art, genre, developer, release date, IGDB rating)
- Cover art downloaded from IGDB at import time and served locally
- Time-to-beat estimates
- Periodic scheduled metadata refresh (background job keeps data current)

### Import / Export

- Export your full collection to Nexorious JSON format
- Import a Nexorious JSON file; entries with unrecognized platforms or storefronts are flagged for user review post-import rather than silently dropped

### Backup / Restore

- On-demand backup of the full database
- Scheduled automatic backups (configurable)
- Restore from a previous backup

### Jobs

- All sync, import/export, and maintenance work runs as background jobs via a NATS-backed queue
- Job history view: see past and running jobs with status, timestamps, and outcome
- Scheduled maintenance tasks (session cleanup, metadata refresh, export cleanup)

### Admin

- **User management**: create, edit, and delete user accounts
- **Platform management**: add, edit, and remove gaming platforms (PlayStation 5, Nintendo Switch, PC, etc.)
- **Storefront management**: add, edit, and remove storefronts (Steam, PlayStation Store, Epic, GOG, etc.) and their platform associations
- **Maintenance tools**: manual triggers for cleanup and refresh tasks
- Seeded defaults for all major platforms and storefronts on first run

### Setup

On first run (empty user table), Nexorious presents a setup wizard that guides admin account creation and automatically seeds platform/storefront reference data. The seed operation is idempotent.

### Tags

Per-user free-form labels. Tags are managed entities (create, rename, delete). Assignable to any game. Filterable in the library view.

### Wishlist

Games the user wants but does not yet own. Stored as collection entries with a wishlist ownership status. Appear separately in the wishlist view and can be moved to owned when acquired.

---

## Tech Stack

### Backend
| Component | Technology |
|---|---|
| Language | Python 3.13 |
| Framework | FastAPI |
| ORM | SQLModel |
| Database | PostgreSQL |
| Migrations | Alembic |
| Message queue | NATS |
| Authentication | JWT (python-jose), bcrypt |
| External API | IGDB (game metadata) |

### Frontend
| Component | Technology |
|---|---|
| Build tool | Vite 6 |
| Framework | React 19 |
| Language | TypeScript |
| Routing | TanStack Router (file-based) |
| Server state | TanStack Query |
| Styling | Tailwind CSS v4 |
| UI components | shadcn/ui |
| Rich text | TipTap |

### Infrastructure
| Component | Technology |
|---|---|
| Background workers | NATS-backed task queues + scheduled jobs |
| Cover art storage | Local filesystem |
| Structured data | PostgreSQL |
| API surface | REST; OpenAPI docs at `/docs`; JWT bearer auth on all non-public endpoints |
| Container orchestration | Kubernetes (Helm chart) or Docker (podman-compose) |
| Dev environment | devenv (Nix-based), uv (Python dependency management) |

---

## Architecture

```
┌─────────────────────────────────────────────┐
│                  Browser                    │
│          Vite SPA (React + TS)              │
└─────────────────┬───────────────────────────┘
                  │ HTTP/REST
┌─────────────────▼───────────────────────────┐
│               FastAPI                       │
│  /api/*  →  route handlers                  │
│  /*      →  static SPA (dist/)              │
└──────┬──────────────────┬───────────────────┘
       │ SQL               │ NATS publish
┌──────▼──────┐    ┌───────▼───────────────────┐
│ PostgreSQL  │    │       Workers              │
│  (all data) │    │  sync / import / export   │
└─────────────┘    │  maintenance / refresh    │
                   └───────────────────────────┘
```

- FastAPI serves both the REST API and the pre-built Vite SPA. No separate frontend server in production.
- NATS is required for background tasks. Basic collection CRUD (read/write games) works without NATS, but sync and scheduled maintenance will not run.
- PostgreSQL is the single source of truth. Alembic manages all schema migrations.
- Cover art is downloaded from IGDB once at import time and served from local disk thereafter.

---

## Deployment

### Development

```bash
podman-compose up        # starts API + PostgreSQL + NATS
cd frontend && npm run dev  # starts Vite dev server on :3000
```

Backend API: http://localhost:8000
Frontend dev: http://localhost:3000
API docs: http://localhost:8000/docs

### Production — Docker / Podman Compose

```bash
podman-compose up --build
```

The build process compiles the frontend (`dist/`) and copies it into the backend image. The compose stack starts API + PostgreSQL + NATS. The API serves both the REST endpoints and the static frontend from a single container.

### Production — Kubernetes

A Helm chart is provided at `deploy/helm/` using the bjw-s common library (v4.6.2).

```bash
helm dependency update deploy/helm/
helm upgrade --install nexorious deploy/helm/ -f my-values.yaml
```

Configure via `values.yaml`. External secrets support (for IGDB credentials and CNPG-managed PostgreSQL) is planned — see roadmap.

### Configuration

All secrets and environment-specific values are provided via environment variables:

| Variable | Purpose |
|---|---|
| `SECRET_KEY` | JWT signing key |
| `INTERNAL_API_KEY` | Internal service auth |
| `DATABASE_URL` | PostgreSQL connection string |
| `IGDB_CLIENT_ID` | IGDB API credentials |
| `IGDB_CLIENT_SECRET` | IGDB API credentials |

---

## Roadmap

Items are removed when completed. When a completed item introduces a new feature domain, sync source, or deployment option, the spec above is updated to reflect it.

### UX & Library

| Item | Priority |
|---|---|
| IGDB ratings display fix (show X.X not XX) | High |
| Search field icon overlaps placeholder text | High |
| Storefront management table clips edit/delete buttons | High |
| Remove all "coming soon" placeholder messages | High |
| Epic Games Store auth UX (inline flow, not dialog) | High |
| Backlog view (unfinished, unshelved games) | Medium |
| Clickable dashboard status counts → filtered library | Medium |
| Choose-next-game flow | Low |

### Sync

| Item | Priority |
|---|---|
| Sync All Now button | High |
| Sync reports (summary of added/changed/removed per run) | High |
| Sync to remove subscription games no longer available | Medium |
| User-configurable skipped games and mapping corrections | Medium |
| GOG sync (via lgogdownloader CLI) | Medium |
| Xbox sync (via xbox-webapi-python) | Low |
| Humble Bundle sync | Low |

### Data Integrity

| Item | Priority |
|---|---|
| UserGame lifecycle: no delete on last platform removal — change status to "no longer owned" instead | High |
| IGDB search fixes (apostrophes in titles, colour/color normalization) | High |
| IGDB ID / game ID refactor (remove redundant `game_id` field now that IGDB ID is primary key) | Medium |
| Darkadia CSV-to-Nexorious-JSON conversion: map missing platforms/storefronts to "unknown" for post-import review | Medium |
| Data smell detection: maintenance function to surface suspicious platform/storefront combinations | Low |

### Notifications

| Item | Priority |
|---|---|
| External notifications via helper library (Telegram, Pushover, etc.) | Medium |
| Configurable notification events (re-auth needed, new games added, sync complete) | Medium |

### Platform & Storefront

| Item | Priority |
|---|---|
| Mac platform icon not displaying | Medium |
| Platform icon tooltip: also show storefront name | Medium |

### Achievements & Trophies

| Item | Priority |
|---|---|
| Steam achievement/trophy tracking (percentage or full detail) | Low |

### Operations

| Item | Priority |
|---|---|
| External secrets support in Helm chart (IGDB creds, CNPG-managed PostgreSQL secret) | High |
| Remove hard docker-compose service dependencies (graceful degradation when DB/NATS unavailable) | Medium |
| Maintenance job for orphaned file cleanup | Medium |

### Code Quality

| Item | Priority |
|---|---|
| Remove leftover CSV import code | High |
| knip — frontend dead code detection | Low |
| vulture — backend dead code detection | Low |

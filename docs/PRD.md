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

---

### UX & Library

#### Remove all "coming soon" placeholder messages `High`
There are multiple places in the app that claim things are "coming soon". This is not helpful and should be removed.

#### Epic Games Store auth UX `High`
When clicking Connect for Epic, a box pops up with a "Start Authentication" button, after which another box appears with a link and a code input field. This is inconsistent with the other sync sources. The authentication information should be displayed directly on the page without popup dialogs.

#### IGDB ratings display `High`
IGDB ratings (`rating_average`) are stored in the DB and returned by the API but are not displayed anywhere in the UI — the display was dropped during the frontend rewrite. The rating should be shown on the game detail view. IGDB stores ratings as integers (0–100) but they should be displayed as decimals (0.0–10.0) with a single digit after the decimal point.

#### Search field icon overlaps placeholder text `High`
In the search field on My Games, the magnifying glass icon is placed on top of the word "Search" in the placeholder text.

#### Clickable dashboard status counts `Medium`
The dashboard shows a breakdown of progress — how many games are not started, in progress, completed, dropped, etc. Clicking one of these counts should navigate to the library view filtered to that status.

#### Backlog view `Medium`
Add a view that shows all games that are not completed (or mastered/dominated) and not shelved — i.e. the active backlog.

#### Choose-next-game flow `Low`
Add functionality to help the user decide what to play next (a "Next Up" view). Should consider wishlist games and use sorting/filtering based on genres, platforms, and time-to-beat estimates.

---

### Sync

#### Sync All Now button `High`
Add a "Sync All Now" button on the sync page that triggers a sync on all configured sync sources simultaneously.

#### Sync reports `High`
After a sync runs, generate a summary report showing what was added, changed, or removed. Reports should be viewable in the sync history list and reusable for notifications when that feature is added.

#### Sync to remove subscription games `Medium`
During sync, check games in the database that have IDs for the sync source and an ownership status of "subscription" to verify they are still available. If a game is no longer available (e.g. a PS Plus Extra game that left the catalogue), remove the storefront association. This also catches mismatches where a user manually added a game as owned on Steam but the Steam sync doesn't find it — the Steam link should be removed to reflect reality.

Requires ownership status tracking per storefront (already implemented).

#### User-configurable skipped games and mapping corrections `Medium`
Any game skipped during a sync should be revisable by the user. Game-to-IGDB mappings made during sync should also be correctable if the user made the wrong choice.

#### GOG sync `Medium`
Sync the user's GOG library using [lgogdownloader](https://github.com/Sude-/lgogdownloader) as a CLI tool to pull library information.

#### Xbox sync `Low`
Sync the user's Xbox library using [xbox-webapi-python](https://github.com/OpenXbox/xbox-webapi-python).

#### Humble Bundle sync `Low`
Sync games acquired through Humble Bundle.

---

### Data Integrity

#### UserGame lifecycle rules `High`
Ensure the following logic is applied everywhere:
- When removing the last platform/storefront from a UserGame, change ownership status to "No Longer Owned" rather than deleting the record.
- When adding a platform/storefront to a "No Longer Owned" UserGame, change ownership status to "Owned".
- Only actually delete a UserGame if the user explicitly deletes it.

#### IGDB search fixes `High`
- Searching for titles with apostrophes returns no results.
- "Colour" and "Color" are not treated as equivalent search terms.

#### IGDB ID / game ID refactor `Medium`
After refactoring to use the IGDB ID as the primary key for games, both `igdb_id` and `game_id` fields now exist in several schemas and models — but they refer to the same value. The redundant `game_id` field should be removed. This may require rethinking how sync and import flows reference games, since `game_id` should no longer be used as an indicator of whether a game has been synced.

#### Transparent IGDB import `Medium`
The current workflow requires an explicit "import from IGDB" step before adding a UserGame entry. This should be transparent: the user-games add endpoint should accept an IGDB ID directly. If no game with that ID exists in the database, it is imported automatically. If it already exists, it is used as-is.

#### Darkadia CSV import: unknown platform/storefront handling `Medium`
When converting a Darkadia CSV export to Nexorious JSON for import, games with missing or unrecognised platforms/storefronts should be mapped to "unknown" associations rather than being dropped or erroring. This allows the user to review and resolve them after import. A helper function to identify and sort out games with missing platform/storefront data may also be useful.

#### Data smell detection `Low`
Add a maintenance function that identifies suspicious data combinations — for example, a PS3 game with Humble Bundle as the storefront. These are not necessarily wrong, but surfacing them lets the user review and correct any mismatches.

---

### Notifications

#### External notifications `Medium`
Allow notifications to be sent to external services such as Telegram, Pushover, and others. Use a helper library that supports multiple services to keep the implementation lean. Examples of notification events: needing to re-authenticate Epic, new games added from a sync, sync completed.

#### Configurable notification events `Medium`
Users should be able to choose which notification types they want to receive.

---

### Platform & Storefront

#### Mac platform icon not displaying `Medium`
Icons for the Mac platform do not show up in the UI.

#### Storefront name in platform icon tooltip `Medium`
When hovering over a platform indicator on a game card, the tooltip should also show which storefront the game is on (not just the platform).

---

### Achievements & Trophies

#### Steam achievement/trophy tracking `Low`
Steam exposes achievement and trophy data. At minimum, store the percentage of achievements earned per game. A more detailed view could be added later.

---

### Operations

#### External secrets support in Helm chart `High`
The Helm chart should allow values to point to externally-managed Kubernetes secrets for all sensitive configuration. This enables IGDB credentials to be managed securely and allows pointing to a secret managed by CNPG (CloudNativePG) for the PostgreSQL password.

#### Remove hard docker-compose service dependencies `Medium`
Unlike Kubernetes, the docker-compose setup uses hard `depends_on` relationships. The API backend and workers/scheduler must be able to start and gracefully handle unavailability of the database and/or NATS, consistent with how they behave in Kubernetes.

#### Maintenance job for orphaned file cleanup `Medium`
Add scheduled maintenance jobs to clean up orphaned cover art files and expired/stale job records.

---

### Code Quality

#### Remove leftover CSV import code `High`
Support for importing via CSV was removed, but references to CSV as an import source remain in the code. These should be removed.

#### knip — frontend dead code detection `Low`
Use [knip](https://knip.dev) to identify unused exports, files, and dependencies in the frontend. Run periodically to keep the codebase lean.

#### vulture — backend dead code detection `Low`
Use [vulture](https://github.com/jendrikseipp/vulture) to find unused Python code in the backend.

#### Experiment with slumber `Low`
[slumber](https://github.com/LucasPickering/slumber) is a TUI HTTP client that may be more reliable than ad-hoc curl commands for API testing and development workflows.

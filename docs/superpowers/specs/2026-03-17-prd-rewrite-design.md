# PRD Rewrite Design — Nexorious

**Date**: 2026-03-17
**Status**: Approved (user session 2026-03-17)

## Goal

Replace the existing `docs/PRD.md` (a phase-based planning artifact from early development) with a clean, authoritative product document that reflects what Nexorious is today, who it's for, and where it is going. Retire `docs/IDEAS.md` by absorbing its content into the new PRD as a prioritized roadmap.

## Decisions Made

### Format: Spec + Roadmap
Two logical parts:
- **Part 1 — Spec**: describes Nexorious as it exists today; stable unless new features ship
- **Part 2 — Roadmap**: prioritized backlog organized by theme; items are removed when completed; Part 1 is updated if a completed item introduces a new feature domain, sync source, or deployment option

**Convention**: Everything in Part 1 is fully implemented. Part 2 contains what is not yet done. No forward-looking annotations in Part 1 — known defects and improvements belong in Part 2.

### Product Name: Nexorious
Use "Nexorious" as the proper product name throughout. Provide a tagline. Write with product identity, not generic project framing.

### User Model: Household / Small Group
One instance per household or small group, typically 1–5 users. Admin manages all accounts (no self-registration). No multi-admin support — only one user holds the admin role. Each user has their own collection and sync configs.

### Roadmap: Full IDEAS.md Absorption
All content from `docs/IDEAS.md` moves into the PRD roadmap section, organized and prioritized. `IDEAS.md` is retired (deleted).

### Detail Level: Comprehensive
Include user-facing features, tech stack, architecture overview, and deployment options. Serves both end users and contributors/self-hosters.

---

## New PRD Structure

### Part 1: Spec

#### 1. Product Identity
- Name, tagline, one-paragraph description
- Explicit scope: what Nexorious is NOT

#### 2. User Model
- Self-hosted, household/small group (typically 1–5 users)
- Single admin role — admin manages all user accounts, no self-registration
- Per-user: collection, sync configs, tags
- Authentication: username + password login, JWT tokens with refresh, bcrypt password hashing

#### 3. Feature Inventory (by domain)
- **Collection**: IGDB search, ownership tracking (platforms/storefronts), progress status, personal notes, star rating, tags, wishlist
- **Sync**: Steam, PSN, Epic Games Store; background job queue; per-source auth and configuration
- **IGDB Integration**: search, metadata, cover art, periodic scheduled metadata refresh (background job), time-to-beat estimates
- **Import/Export**: Nexorious JSON export/import; unknown platform/storefront entries are flagged for user review post-import
- **Backup/Restore**: scheduled and on-demand backups; restore from backup
- **Jobs**: NATS-backed queue; job history and status visibility; scheduled maintenance tasks
- **Admin**: user management, platform/storefront management (including seeded defaults), maintenance tools
- **Setup**: first-run wizard — on empty user table, presents admin creation screen and seeds platform/storefront data
- **Tags**: per-user managed entities (free-form labels); assignable to games; filterable in library view
- **Wishlist**: games the user wants but does not own; stored as UserGame records with a wishlist ownership status

#### 4. Tech Stack
- Backend: Python 3.13, FastAPI, SQLModel, PostgreSQL, Alembic, NATS, JWT auth (bcrypt)
- Frontend: Vite 6, React 19, TypeScript, TanStack Router (file-based), TanStack Query, Tailwind CSS v4, shadcn/ui, TipTap (notes editor)
- Workers: NATS-backed queues + scheduled jobs for sync/import/export/maintenance
- Storage: local filesystem (cover art), PostgreSQL (all structured data)
- API: REST; documented at `/docs` (OpenAPI/Swagger); JWT bearer auth required for all non-public endpoints
- Deployment: podman-compose (dev + simple production), Helm chart for Kubernetes (bjw-s common library v4.6.2)
- Dev tooling: devenv (Nix-based), uv (Python deps)

#### 5. Architecture Overview
- FastAPI serves both the REST API (`/api/*`) and the Vite SPA (`dist/`) via static file catch-all — no separate frontend server in production
- NATS dispatches background tasks; workers consume tasks for sync, import/export, and maintenance. Basic collection CRUD works without NATS, but sync and scheduled tasks will not run without it.
- PostgreSQL is the single source of truth; Alembic manages schema migrations
- Cover art is downloaded from IGDB at import time and served from local filesystem storage
- Section 3 "Setup" describes the user-facing first-run behavior; the technical trigger: when the user table is empty, the frontend redirects to the setup wizard, which calls the backend to create the admin account and run idempotent seed data

#### 6. Deployment
- **Dev**: `podman-compose up` starts API + PostgreSQL + NATS; `npm run dev` starts the frontend dev server separately
- **Production — Docker**: `podman-compose up --build`; the build bundles frontend `dist/` into the backend image; the compose file includes PostgreSQL and NATS; a single compose stack serves everything
- **Production — Kubernetes**: Helm chart at `deploy/helm/`; configure via `values.yaml`; external secrets support is planned (see roadmap)
- **Configuration**: environment variables for all secrets (IGDB credentials, JWT secret, DB URL, internal API key)

---

### Part 2: Roadmap

Items are removed when completed. If a completed item introduces a new feature, sync source, or deployment option, update Part 1 to reflect it.

#### UX & Library
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

#### Sync
| Item | Priority |
|---|---|
| Sync All Now button | High |
| Sync reports (summary of added/changed/removed per run) | High |
| Sync to remove subscription games no longer available | Medium |
| User-configurable skipped games and mapping corrections | Medium |
| GOG sync (via lgogdownloader CLI) | Medium |
| Xbox sync (via xbox-webapi-python) | Low |
| Humble Bundle sync | Low |

#### Data Integrity
| Item | Priority |
|---|---|
| UserGame lifecycle: no delete on last platform removal — change to "no longer owned" instead | High |
| IGDB search fixes (apostrophes in titles, colour/color normalization) | High |
| IGDB ID / game ID refactor (remove redundant `game_id` field now that IGDB ID is primary key) | Medium |
| Darkadia CSV-to-Nexorious-JSON conversion: map missing platforms/storefronts to "unknown" for post-import review | Medium |
| Data smell detection: maintenance function to surface suspicious platform/storefront combos | Low |

#### Notifications
| Item | Priority |
|---|---|
| External notifications via helper library (Telegram, Pushover, etc.) | Medium |
| Configurable notification events (re-auth needed, new games added, sync complete) | Medium |

#### Platform & Storefront
| Item | Priority |
|---|---|
| Mac platform icon not displaying | Medium |
| Platform icon tooltip: also show storefront name | Medium |

#### Achievements & Trophies
| Item | Priority |
|---|---|
| Steam achievement/trophy tracking (percentage or full detail) | Low |

#### Operations
| Item | Priority |
|---|---|
| External secrets support in Helm chart (IGDB creds, CNPG-managed PostgreSQL secret) | High |
| Remove hard docker-compose service dependencies (graceful degradation when DB/NATS unavailable) | Medium |
| Maintenance job for orphaned file cleanup | Medium |

#### Code Quality
| Item | Priority |
|---|---|
| Remove leftover CSV import code | High |
| knip — frontend dead code detection | Low |
| vulture — backend dead code detection | Low |

---

## Files Affected

- `docs/PRD.md` — replaced entirely
- `docs/IDEAS.md` — deleted (content absorbed into new PRD roadmap)

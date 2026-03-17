# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Code Exploration Policy
Always use jCodemunch-MCP tools — never fall back to Read, Grep, Glob, or Bash for code exploration.
- Before reading a file: use get_file_outline or get_file_content
- Before searching: use search_symbols or search_text
- Before exploring structure: use get_file_tree or get_repo_outline
- Call list_repos first; if the project is not indexed, call index_folder with the current directory.
- **Exception**: jCodemunch does not reliably index markdown/docs files. If get_file_content returns "File not found" for a `.md` file, fall back to the Read tool — do not attempt index_folder for a single missing doc.

## Quick Reference

### Common Commands
| Task                     | Backend Command                                            | Frontend Command                                                |
|--------------------------|------------------------------------------------------------|-----------------------------------------------------------------|
| Install dependencies     | `cd backend && uv sync`                                    | `cd frontend && npm install`                                    |
| Start development server | `uv run python -m app.main`                                | `npm run dev`                                                   |
| Run tests                | `uv run pytest`                                            | `npm run test`                                                  |
| Run tests with coverage  | `uv run pytest --cov=app --cov-report=term-missing`        | `npm run test:coverage`                                         |
| Type checking            | `uv run pyrefly check`                                     | `npm run check`                                                 |
| Linting                  | `uv run ruff check .`                                      | N/A                                                             |
| Database migrations      | `uv run alembic upgrade head`                              | N/A                                                             |

### Environment Validation
```bash
# Verify development environment
devenv shell  # Enter development shell
cd backend && uv --version
cd frontend && npm --version
```

### Docker/Podman Compose (Alternative)
| Task               | Command                         |
|--------------------|---------------------------------|
| Start all services | `podman-compose up --build`     |
| Stop services      | `podman-compose down`           |
| Stop and reset DB  | `podman-compose down -v`        |
| Rebuild backend    | `podman-compose build api`      |
| Rebuild frontend   | `podman-compose build frontend` |

### Important URLs
- Backend API Docs: http://localhost:8000/docs
- Health Check: http://localhost:8000/health
- Frontend Dev: http://localhost:3000

## Setup & Development

### Development Environment
The project uses devenv for reproducible development:
```bash
devenv shell  # Enter development shell with Python 3.13, uv, ruff, pyrefly, pytest
```

**Note**: `pyrefly` is installed in the backend venv via uv, not in the devenv shell. Run it using `uv run pyrefly` from the backend directory.

**Note**: `pyrightconfig.json` in the project root is for IDE/Pylance integration only. The authoritative type checker for CI and commits is `pyrefly`.

**Note**: The `LSP` tool (goToDefinition, findReferences, hover, documentSymbol, etc.) works for both Python and TypeScript. Ignore LSP diagnostics — Pyright reports missing imports for `fastapi`/`uvicorn` etc. (can't see `backend/.venv`) and TypeScript LSP reports missing modules for `@tanstack/react-router` etc. (node_modules path issue). Use `uv run pyrefly check` and `npm run check` as authoritative type checkers instead.

### Initial Setup
```bash
# Backend setup
cd backend
uv sync  # Install all dependencies
uv run alembic upgrade head  # Run database migrations

# Frontend setup
cd frontend
npm install  # Install all dependencies
```

### Project Structure
- `backend/` - FastAPI Python backend with API routes, models, services
  - `app/api/` - Route handlers (FastAPI routers)
  - `app/models/` - SQLModel ORM models
  - `app/schemas/` - Pydantic request/response schemas
  - `app/services/` - Business logic
  - `app/core/` - Database session, config, settings, JWT security
  - `app/middleware/` - HTTP middleware
  - `app/worker/` - Background tasks (NATS-backed queues, scheduled maintenance, sync)
  - `app/seed_data/` - Idempotent DB seed data
  - `app/utils/` - Shared utilities (rate limiter, fuzzy match, JSON serialization, etc.)
  - `app/tests/` - pytest test files
- `frontend/` - Vite + React SPA with TanStack Router (file-based), Tailwind CSS v4, shadcn/ui, TanStack Query
  - `src/routes/` - TanStack Router file-based routes (`_authenticated/`, `_public/`, `__root.tsx`)
  - `src/components/` - Reusable React components
  - `src/api/` - API client functions
  - `src/hooks/` - Custom TanStack Query hooks (use-games, use-sync, use-platforms, etc.)
  - `src/types/` - Shared TypeScript type definitions
  - `src/providers/` - React context providers (auth, query)
  - `src/lib/` - Utilities and helpers
  - `src/styles/` - Global CSS (Tailwind v4)
- `docs/` - Project documentation, specifications, and reference guides
- `storage/` - Runtime file storage for cover art

## Additional Commands

### Database Migrations (Alembic)
```bash
# Apply pending migrations
uv run alembic upgrade head

# Generate new migration after model changes
uv run alembic revision --autogenerate -m "description of changes"
```

**IMPORTANT**: Never write migration files manually. Always use `--autogenerate` to generate migrations from model changes. Claude must run the autogenerate command, not create migration files directly.

### Alternative Server Start
```bash
uv run uvicorn app.main:app --reload
```

### Frontend Building
```bash
# Build for production (outputs to frontend/dist/)
npm run build

# Preview production build locally
npm run preview

# Run tests with UI
npm run test:ui
```

## Testing & Quality Assurance

### Testing Requirements
- **Backend**: >80% coverage, all tests must pass
- **Frontend**: >70% coverage, all tests must pass
- **Zero tolerance**: Fix failing tests immediately

### Type Checking Requirements
- **Backend**: Zero pyrefly type errors allowed
- **Frontend**: Zero TypeScript errors allowed (tsc --noEmit)
- **Zero tolerance**: Fix type errors before committing - do not introduce new type errors

### Backend Testing (pytest)
- **Framework**: pytest with pytest-asyncio for async testing
- **Test Types**: Unit tests, API integration tests, IGDB mocking
- **Coverage Reports**: HTML reports in `htmlcov/` directory
- **Database Testing**: PostgreSQL support

### Frontend Testing (Vitest)
- **Framework**: Vitest with @testing-library/react
- **Test Types**: Component tests, hook tests, API service tests
- **Coverage Reports**: HTML reports in `coverage/` directory
- **DOM Testing**: jsdom environment

### Test Conventions
- Backend: `test_*.py` files in `app/tests/`; run single test: `uv run pytest app/tests/test_file.py::test_function_name -v`
- Frontend: `*.test.ts` or `*.test.tsx` files alongside source files; run single test: `npm run test game-card.test.tsx`

## Development Rules

> **Always ask questions if you are uncertain about something!**

### Essential Workflow
1. **Planning**: Read `docs/PRD.md` when working on new features or if product context is unclear
2. **Branching**: Create feature branch before starting ANY task work (see Branch Workflow below)
3. **Development**: Use full paths for `cd` commands, use `uv run python` for backend
4. **Testing**: Run tests after ANY code changes - zero failures accepted
5. **Documentation**: Use context7 MCP to verify API usage in generated code

### Branch Workflow (MANDATORY)
**AI agents MUST use branches when working on tasks. Never commit directly to main.**

#### Rules
- ✅ Always create a branch before starting task work
- ✅ Keep branches focused on a single task/issue
- ✅ Create PRs for code review before merging to main
- ✅ Review PR diff before merging; ask user only if issues found
- ✅ Always use `--squash --delete-branch` when merging PRs (squash commits for clean history)
- ❌ Never commit directly to main
- ❌ Never merge PRs without reviewing the diff first
- ❌ Never work on multiple unrelated changes in one branch

### Code Style

**Python (Backend)**
- Imports: stdlib → third-party → local (`from ..core.database import get_session`)
- Naming: `snake_case` functions/vars, `PascalCase` classes, `UPPER_CASE` constants
- FastAPI DI pattern: `Annotated[Session, Depends(get_session)]`
- Always use `async def` for route handlers and DB operations

**TypeScript (Frontend)**
- Imports: external libs → internal (`@/...`) → types
- Naming: `camelCase` functions/vars, `PascalCase` components, `UPPER_CASE` constants
- Props: interface-typed with destructuring — `function GameCard({ game }: Props)`
- TanStack Query for server state, `useState` for local state only

### Code Reference Documents
- **Pydantic Code**: Always read `docs/pydantic-v2-best-practices.md` before generating any Pydantic models or validators
- **SQLModel Computed Fields**: Always read `docs/sqlmodel-computed-fields-guide.md` when working with computed fields in SQLModel
- **Product context**: Read `docs/PRD.md` for feature context and planned work (roadmap); `docs/BUGS.md` for known issues
- **Design history**: `docs/superpowers/specs/` contains brainstorming design docs; `docs/superpowers/plans/` contains implementation plans
- **Import/CLI specs**: See `docs/CLI_IMPLEMENTATION_GUIDE.md`, `docs/DARKADIA_CSV_IMPORT_SPECIFICATION.md` for import feature context

### Required After Code Changes

#### Frontend Changes
```bash
npm run check  # Must pass (tsc --noEmit && eslint)
npm run test   # Must pass
```

#### Backend Changes
```bash
uv run alembic upgrade head  # After pulling changes
uv run ruff check .  # Check for common errors and problems (linting)
uv run pyrefly check  # Type checking with pyrefly (installed in venv)
uv run pytest --cov=app --cov-report=term-missing  # Must pass with >80% coverage
```

### Quality Gates
- All tests must pass before committing
- All type checks must pass before committing (pyrefly for backend, tsc for frontend)
- Backend: >80% coverage required
- Frontend: >70% coverage required
- Fix failing tests and type errors immediately in same session

## Project Architecture

### Backend Stack
- **Framework**: FastAPI (Python 3.13) - High-performance async web framework  
- **Database**: SQLModel ORM supporting PostgreSQL
- **Migrations**: Alembic for database schema versioning
- **Authentication**: JWT tokens with refresh mechanism
- **External APIs**: IGDB integration for game metadata and cover art
- **File Storage**: Local filesystem storage for cover art with configurable paths
- **Testing**: pytest

### Frontend Stack
- **Framework**: Vite 6 + React 19 + TypeScript — SPA served by FastAPI in production
- **Routing**: TanStack Router v1 with file-based routes (`src/routes/`)
- **Styling**: Tailwind CSS v4 (`@import "tailwindcss"` syntax, PostCSS via `@tailwindcss/postcss`)
- **UI Components**: shadcn/ui for accessible, customizable components
- **State Management**: TanStack Query (React Query) for server state and caching
- **Forms**: React Hook Form with Zod validation
- **Rich Text**: TipTap editor for notes and descriptions
- **Testing**: Vitest with @testing-library/react
- **Production serving**: Built `dist/` is copied into the backend Docker image; FastAPI serves it via `StaticFiles(html=True)` catch-all

### Database Design
- **Database**: PostgreSQL
- **Schema**: Comprehensive game collection models with platform/storefront relationships
- **Migrations**: Automatic schema management via Alembic
- **Seeding**: Idempotent seed data for platforms and storefronts

### External Integrations
- **IGDB API**: Game metadata, cover art, and completion time estimates with built-in rate limiting (4 req/s)
- **Rate Limiting**: Token bucket algorithm with configurable burst capacity and automatic retries
- **Cover Art Storage**: Automatic download and local storage during game import
- **Platform Support**: Multi-platform game ownership tracking
- **Storefront Integration**: Support for Steam, Epic, GOG, PlayStation, Xbox, Nintendo, and physical media

## Helm Chart

### Location & Commands
- Chart lives at `deploy/helm/` (single chart, release name: `nexorious`)
- `helm dependency update deploy/helm/` — fetch/update `charts/common-4.6.2.tgz`
- `helm lint --strict deploy/helm/ --set nexorious.secretKey=x --set nexorious.internalApiKey=x --set nexorious.postgresql.password=x --set nexorious.igdbClientId=x --set nexorious.igdbClientSecret=x`
- `helm template nexorious deploy/helm/ --set nexorious.secretKey=x ...` — dry-run to inspect rendered resources

### bjw-s Common Library (v4.6.2)
- Repo: `https://bjw-s-labs.github.io/helm-charts/` — required entrypoint: `{{- include "bjw-s.common.loader.all" . -}}` in `templates/common.yaml`
- **Controller disable cascade**: Disabling a controller (e.g. `controllers.postgresql.enabled: false`) also requires disabling its service (`service.postgresql.enabled: false`) and any persistence that references it via `advancedMounts` — failing to do so causes `No enabled controller found` errors
- Go template functions **cannot** be called in `values.yaml` — compute dynamic values (DATABASE_URL, etc.) in `templates/` files
- `values.schema.json`: validate only custom `nexorious.*` block; use `additionalProperties: true` on bjw-s-managed blocks (`controllers`, `service`, `ingress`, `persistence`)


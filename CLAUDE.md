# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Reference

### Common Commands
| Task                     | Backend Command                                            | Frontend Command                                                |
|--------------------------|------------------------------------------------------------|-----------------------------------------------------------------|
| Install dependencies     | `cd /home/abo/workspace/home/nexorious/backend && uv sync` | `cd /home/abo/workspace/home/nexorious/frontend && npm install` |
| Start development server | `uv run python -m app.main`                                | `npm run dev`                                                   |
| Run tests                | `uv run pytest`                                            | `npm run test`                                                  |
| Run tests with coverage  | `uv run pytest --cov=app --cov-report=term-missing`        | `npm run test:coverage`                                         |
| Type checking            | `uv run pyrefly check`                                     | `npm run check`                                                 |
| Linting                  | `uv run ruff check .`                                      | N/A                                                             |
| Database migrations      | `uv run alembic upgrade head`                              | N/A                                                             |

### Environment Validation
```bash
# Verify development environment
nix develop  # Enter development shell
cd /home/abo/workspace/home/nexorious/backend && uv --version
cd /home/abo/workspace/home/nexorious/frontend && npm --version
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
The project uses Nix for reproducible development:
```bash
nix develop  # Enter development shell with Python 3.13, uv, ruff, pyrefly, pytest
```

**Note**: `pyrefly` is installed in the backend venv via uv, not in the Nix shell. Run it using `uv run pyrefly` from the backend directory.

### Initial Setup
```bash
# Backend setup
cd /home/abo/workspace/home/nexorious/backend
uv sync  # Install all dependencies
uv run alembic upgrade head  # Run database migrations

# Frontend setup  
cd /home/abo/workspace/home/nexorious/frontend
npm install  # Install all dependencies
```

### Project Structure
- `backend/` - FastAPI Python backend with API routes, models, services
- `frontend/` - Next.js 16 TypeScript frontend with React 19, Tailwind CSS, shadcn/ui, TanStack Query
- `docs/` - PRD, task breakdown, wireframes
- `storage/` - Runtime file storage for cover art

## Additional Commands

### Database Management
```bash
# Create new migration (after model changes)
# IMPORTANT: Claude Code should run this command when migrations are needed
# DO NOT write migration files manually - always use autogenerate
uv run alembic revision --autogenerate -m "description of changes"

# Alternative backend server start
uv run uvicorn app.main:app --reload
```

### Frontend Building
```bash
# Build for production
npm run build

# Preview production build  
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
- **Database Testing**: PostgreSQL and SQLite support

#### CSV Import Testing
Comprehensive test coverage (>90%) in `backend/scripts/tests/`:
- Idempotency validation, merge strategies, decision caching
- Platform duplicate prevention, performance testing
- Documentation: `backend/scripts/tests/README.md`

### Frontend Testing (Vitest)
- **Framework**: Vitest with @testing-library/react
- **Test Types**: Component tests, hook tests, API service tests
- **Coverage Reports**: HTML reports in `coverage/` directory
- **DOM Testing**: jsdom environment

### Test Commands
```bash
# Backend - all must pass
uv run pytest
uv run pytest --cov=app --cov-report=term-missing

# Frontend - all must pass
npm run check
npm run test

# CSV Import specific tests
uv run pytest scripts/tests/ -v
```

### Test Conventions
- Backend: `test_*.py` files in `app/tests/`
- Frontend: `*.test.ts` or `*.test.tsx` files alongside source files

## Development Rules

> **Always ask questions if you are uncertain about something!**

### Essential Workflow
1. **Planning**: Read `docs/PRD.md` before starting work
2. **Branching**: Create feature branch before starting ANY task work (see Branch Workflow below)
3. **Development**: Use full paths for `cd` commands, use `uv run python` for backend
4. **Testing**: Run tests after ANY code changes - zero failures accepted
5. **Documentation**: Use context7 MCP to verify API usage in generated code

### Branch Workflow (MANDATORY)
**AI agents MUST use branches when working on tasks. Never commit directly to main.**

#### Rules
- ✅ Always create a branch before starting task work
- ✅ Name branches with the beads issue ID
- ✅ Keep branches focused on a single task/issue
- ✅ Create PRs for code review before merging to main
- ✅ Review PR diff before merging; ask user only if issues found
- ✅ Always use `--squash --delete-branch` when merging PRs (squash commits for clean history)
- ❌ Never commit directly to main
- ❌ Never merge PRs without reviewing the diff first
- ❌ Never work on multiple unrelated changes in one branch

### Code Reference Documents
- **Pydantic Code**: Always read `docs/pydantic-v2-best-practices.md` before generating any Pydantic models or validators
- **SQLModel Computed Fields**: Always read `docs/sqlmodel-computed-fields-guide.md` when working with computed fields in SQLModel
- **Alembic Migrations**: Always read `docs/alembic-migrations-guide.md` before creating or modifying database migrations

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

### File Naming Rules
- Backend tests: `test_*.py` files in `app/tests/`
- Frontend tests: `*.test.ts` or `*.test.tsx` files alongside source files

### Quality Gates
- All tests must pass before committing
- All type checks must pass before committing (pyrefly for backend, tsc for frontend)
- Backend: >80% coverage required
- Frontend: >70% coverage required
- Fix failing tests and type errors immediately in same session

## Project Architecture

### Backend Stack
- **Framework**: FastAPI (Python 3.13) - High-performance async web framework  
- **Database**: SQLModel ORM supporting both PostgreSQL (production) and SQLite (development)
- **Migrations**: Alembic for database schema versioning
- **Authentication**: JWT tokens with refresh mechanism
- **External APIs**: IGDB integration for game metadata and cover art
- **File Storage**: Local filesystem storage for cover art with configurable paths
- **Testing**: pytest with >80% coverage requirement

### Frontend Stack
- **Framework**: Next.js 16 with React 19 and TypeScript - Modern React framework with App Router
- **Styling**: Tailwind CSS for utility-first styling
- **UI Components**: shadcn/ui for accessible, customizable components
- **State Management**: TanStack Query (React Query) for server state and caching
- **Forms**: React Hook Form with Zod validation
- **Rich Text**: TipTap editor for notes and descriptions
- **Build Tool**: Turbopack for fast development builds
- **Testing**: Vitest with @testing-library/react, >70% coverage requirement

### Database Design
- **Primary Database**: PostgreSQL for production deployments
- **Development Database**: SQLite for local development and testing
- **Schema**: Comprehensive game collection models with platform/storefront relationships
- **Migrations**: Automatic schema management via Alembic
- **Seeding**: Idempotent seed data for platforms and storefronts

### External Integrations
- **IGDB API**: Game metadata, cover art, and completion time estimates with built-in rate limiting (4 req/s)
- **Rate Limiting**: Token bucket algorithm with configurable burst capacity and automatic retries
- **Cover Art Storage**: Automatic download and local storage during game import
- **Platform Support**: Multi-platform game ownership tracking
- **Storefront Integration**: Support for Steam, Epic, GOG, PlayStation, Xbox, Nintendo, and physical media


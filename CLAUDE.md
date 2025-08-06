# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a project named "nexorious". The backend is written in Python with FastAPI and the frontend is written in Typescript with Svelte. It is licensed under the MIT License.

## Repository Structure

The project is a full-featured game collection management service with comprehensive backend and frontend implementations:

### Core Directories
- `backend/` - FastAPI Python backend with complete API implementation
  - `app/` - Main application package with API routes, models, services
  - `alembic/` - Database migration management
  - `storage/` - File storage for cover art and uploads
  - `tests/` - Comprehensive test suite with >80% coverage
- `frontend/` - SvelteKit TypeScript frontend with complete UI
  - `src/` - Application source code with components, routes, stores
  - `tests/` - Frontend test suite with >70% coverage
  - `static/` - Static assets and PWA configuration
- `docs/` - Project documentation and planning
  - `PRD.md` - Product Requirements Document
  - `TASK_BREAKDOWN.md` - Detailed task tracking
  - `wireframes/` - UI/UX design mockups
- `storage/` - Runtime file storage for cover art

### Configuration Files
- `flake.nix` / `flake.lock` - Nix development environment
- `backend/pyproject.toml` - Python dependencies and project configuration
- `frontend/package.json` - Node.js dependencies and scripts
- `README.md` - Project overview and setup instructions
- `LICENSE` - MIT license

## Development Environment

### Nix Development Shell
The project includes a `flake.nix` file that provides a reproducible development environment:
- Run `nix develop` to enter the development shell
- Includes Python 3.13, uv, ruff, mypy, pytest, and system dependencies
- Uses nixpkgs unstable for latest packages

### Python Package Managers
This project uses **uv** for Python dependency management with support for:
- Standard Python development
- Modern Python tooling
- Development tools (pytest, mypy, ruff)
- IDE support (PyCharm, VSCode, Cursor)

## Backend Development

### Setup and Dependencies
```bash
cd /home/abo/workspace/home/nexorious/backend
uv sync  # Install all dependencies including dev dependencies
```

### Database Management
```bash
# Run database migrations
uv run alembic upgrade head

# Create new migration (after model changes)
uv run alembic revision --autogenerate -m "description of changes"
```

### Development Server
```bash
# Start development server with auto-reload
uv run python -m app.main

# Alternative using uvicorn directly
uv run uvicorn app.main:app --reload
```

### Testing and Quality Assurance
```bash
# Run all tests
uv run pytest

# Run tests with coverage (target >80%)
uv run pytest --cov=app --cov-report=term-missing

# Generate HTML coverage report
uv run pytest --cov=app --cov-report=html

# Run specific test file
uv run pytest app/tests/test_business_logic.py

# Run tests with verbose output
uv run pytest -v
```

### API Documentation
- Swagger UI: http://localhost:8000/docs
- ReDoc: http://localhost:8000/redoc  
- Health check: http://localhost:8000/health

## Frontend Development

### Setup and Dependencies
```bash
cd /home/abo/workspace/home/nexorious/frontend
npm install  # Install all dependencies
```

### Development Server
```bash
# Start development server with hot reload
npm run dev
```

### Building and Checking
```bash
# Type checking and validation
npm run check

# Build for production
npm run build

# Preview production build
npm run preview
```

### Testing
```bash
# Run all tests
npm run test

# Run tests with coverage (target >70%)
npm run test:coverage

# Run tests with UI
npm run test:ui

# Run tests once (CI mode)
npm run test:run
```

## Testing Framework and Coverage Requirements

### Backend Testing (pytest)
- **Framework**: pytest with pytest-asyncio for async testing
- **Coverage Target**: >80% for all business logic, API endpoints, and services
- **Test Types**:
  - Unit tests for models, services, and business logic
  - Integration tests for API endpoints with database
  - External API integration tests with mocking (IGDB)
- **Coverage Analysis**: HTML reports generated in `htmlcov/` directory
- **Database Testing**: Supports both PostgreSQL and SQLite test databases

#### CSV Import Testing
The Darkadia CSV import system has comprehensive test coverage (>90%):
- **Location**: `backend/scripts/tests/`
- **Test Files**: 5 modules covering all import functionality
- **Key Features**:
  - Idempotency validation (safe to re-run imports)
  - All three merge strategies (Interactive, Overwrite, Preserve)
  - Decision caching for Interactive merger
  - Platform duplicate prevention
  - Large dataset performance testing
- **Quick Test Commands**:
  ```bash
  # Run all import tests
  uv run pytest scripts/tests/ -v
  
  # Run with coverage
  uv run pytest scripts/tests/ --cov=scripts --cov-report=term-missing
  
  # Test specific functionality
  uv run pytest scripts/tests/test_idempotency.py -v
  ```
- **Documentation**: See `backend/scripts/tests/README.md` for complete testing guide

### Frontend Testing (Vitest)
- **Framework**: Vitest with @testing-library/svelte
- **Coverage Target**: >70% for components, stores, and utilities  
- **Test Types**:
  - Unit tests for Svelte components and stores
  - Integration tests for user workflows
  - Utility function tests
- **Coverage Analysis**: HTML reports generated in `coverage/` directory
- **DOM Testing**: jsdom environment with @testing-library utilities

### Test Naming Conventions
- Backend: `test_*.py` files in `app/tests/`
- Frontend: `*.test.ts` files alongside source code (NOT starting with `+` for Svelte files)

## Standard operating procedure

These rules must always be adhered to during development.

** ALWAYS ASK QUESTIONS IF YOU ARE UNCERTAIN ABOUT SOMETHING! **

### Directory and Command Management
- This project has a frontend and a backend. They live in dirs called `frontend/` and `backend/`.
- Always cd into the frontend dir before running commands related to the frontend.
- Always cd into the backend dir before running commands related to the backend.
- When running cd always use full paths.
- Always use `uv run python` instead of just `python` for backend commands.

### Planning and Documentation
- Before performing any work always read docs/PRD.md and docs/TASK_BREAKDOWN.md
- When a task has been implemented mark the task(s) as done in the task breakdown
- When you are writing code, please use context7 MCP to learn the APIs used and verify that your generated code is valid

### Version Control and Branching
- When you are asked to work on a task you will create a branch that contains the task name.
- When you are told 'lets work on task XXX' you must first create a branch that contains the task name.

### Frontend Development Rules
- For Svelte Typescript tests, the filename is not allowed to start with `+`
- **MANDATORY**: Run `npm run check` after ANY frontend code changes and fix all errors
- **MANDATORY**: Run `npm run test` after ANY frontend changes and ensure 100% pass rate
- **CRITICAL**: All 778 frontend tests must pass - zero failures accepted

### Backend Development Rules  
- Run database migrations with `uv run alembic upgrade head` after pulling changes
- **MANDATORY**: Run `uv run pytest` after ANY backend code changes and ensure 100% pass rate
- **MANDATORY**: Run `uv run pytest --cov=app --cov-report=term-missing` to verify test coverage meets >80% requirement
- **CRITICAL**: All backend tests must pass - zero failures accepted
- Always test API endpoints manually using Swagger UI at http://localhost:8000/docs

### Quality Assurance
- Backend code coverage must maintain >80%
- Frontend code coverage must maintain >70%
- All tests must pass before committing changes
- Use type checking tools (`npm run check` for frontend, mypy via pytest for backend)

### Test Execution Requirements
**CRITICAL**: All tests must pass at all times. After ANY code changes, you MUST:

#### Frontend Testing (Required After Any Frontend Changes)
```bash
# Run TypeScript checking (must pass)
npm run check

# Run all tests (must pass 100%)
npm run test

# If any tests fail, fix them immediately before proceeding
```

#### Backend Testing (Required After Any Backend Changes)  
```bash
# Run all tests with coverage (must pass 100%)
uv run pytest --cov=app --cov-report=term-missing

# If any tests fail, fix them immediately before proceeding
```

#### Test Failure Policy
- **Zero tolerance for failing tests** - All tests must pass before any commit
- **Immediate fix required** - If code changes break tests, fix tests in the same session
- **No exceptions** - Tests failing due to "intentional errors" in stderr are acceptable only if the test itself passes
- **Full suite validation** - Always run the complete test suite, not just individual tests

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
- **Framework**: SvelteKit with TypeScript - Modern reactive frontend framework
- **Styling**: Tailwind CSS for utility-first styling
- **State Management**: Svelte stores for reactive state
- **Rich Text**: TipTap editor for notes and descriptions
- **Build Tool**: Vite for fast development and optimized builds
- **Testing**: Vitest with @testing-library/svelte, >70% coverage requirement

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

### Development Workflow
1. **Planning**: Tasks defined in `docs/TASK_BREAKDOWN.md`
2. **Development**: Feature branches with descriptive names
3. **Testing**: **MANDATORY** - All tests must pass after every code change
   - Frontend: Run `npm run check` + `npm run test` (100% pass rate required)
   - Backend: Run `pytest` + coverage validation (100% pass rate required)
4. **Quality Assurance**: Type checking and linting with zero tolerance for failures
5. **Documentation**: Comprehensive API documentation via OpenAPI/Swagger

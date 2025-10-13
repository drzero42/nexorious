# Technical Stack

> Last Updated: 2025-09-11
> Version: 1.0.0

## Application Framework

- **Framework:** FastAPI
- **Version:** Latest (Python 3.13)
- **Rationale:** High-performance async web framework with automatic OpenAPI documentation, excellent for API-first applications

## Database

- **Primary Database:** PostgreSQL (production), SQLite (development)
- **ORM:** SQLModel - provides Pydantic models with SQLAlchemy power
- **Migrations:** Alembic for automatic schema versioning and upgrades
- **Design:** Comprehensive game collection models with platform/storefront relationships

## JavaScript

- **Framework:** SvelteKit with TypeScript
- **Build Tool:** Vite for fast development and optimized builds
- **State Management:** Svelte stores for reactive state management
- **Testing:** Vitest with @testing-library/svelte (>70% coverage requirement)

## CSS Framework

- **Framework:** Tailwind CSS
- **Approach:** Utility-first styling for rapid development and consistent design
- **Benefits:** Small bundle size, excellent developer experience, highly customizable

## External Dependencies

### IGDB API Integration (CRITICAL DEPENDENCY)
- **Service:** Internet Game Database (IGDB) API
- **Status:** REQUIRED - Application cannot function without IGDB access
- **Rate Limiting:** 4 requests per second with token bucket algorithm
- **Features:** Game metadata, cover art, ratings, completion time estimates
- **Implementation:** Automatic retries, built-in rate limiting, comprehensive error handling
- **Data Storage:** Cover art automatically downloaded and stored locally

**IMPORTANT:** IGDB API access is mandatory. All games must have valid IGDB IDs and metadata. The application is designed around IGDB data structure and cannot operate without this integration.

## Development Tools

### Package Management
- **Backend:** uv (Python package manager) - fast, reliable Python dependency management
- **Frontend:** npm - standard Node.js package manager
- **Environment:** Nix development shell for reproducible builds

### Development Environment
- **Shell:** Nix develop for consistent Python 3.13, uv, ruff, mypy, pytest environment
- **Hot Reload:** uvicorn --reload for backend, SvelteKit dev server for frontend
- **Database:** Automatic migrations on startup via Alembic

### Testing Framework
- **Backend:** pytest with pytest-asyncio for async testing
- **Coverage:** >80% required with HTML reports in htmlcov/
- **Frontend:** Vitest with jsdom environment
- **Coverage:** >70% required with HTML reports in coverage/
- **CSV Import:** Specialized test suite with >90% coverage for import functionality

### Code Quality
- **Linting:** Ruff for Python code checking
- **Type Checking:** MyPy built into pytest, TypeScript for frontend
- **Formatting:** Automated via development tools
- **Standards:** KISS and DRY principles, practical over perfect

## Architecture Patterns

### Backend Architecture
- **Pattern:** RESTful API with FastAPI routes
- **Authentication:** JWT tokens with refresh mechanism
- **Database:** SQLModel ORM with automatic relationship handling
- **File Storage:** Local filesystem for cover art with configurable paths
- **Error Handling:** Comprehensive logging and user-friendly error responses

### Frontend Architecture
- **Pattern:** SvelteKit with TypeScript for type safety
- **Routing:** File-based routing with SvelteKit conventions
- **Components:** Reactive Svelte 5 components with TypeScript
- **Rich Text:** TipTap editor for notes and descriptions
- **API Integration:** Fetch-based API communication with error handling

### Data Flow
1. **Import:** CSV/Steam library → Backend processing → IGDB API lookup → Database storage
2. **Metadata:** IGDB API → Rate-limited requests → Local caching → Cover art download
3. **User Data:** Frontend forms → API validation → Database persistence → Real-time updates
4. **Authentication:** JWT tokens → Refresh mechanism → Secure API access

## Deployment Considerations

### Self-Hosting Requirements
- **Python:** 3.13 with uv package manager
- **Database:** PostgreSQL recommended, SQLite for smaller deployments
- **Storage:** Local filesystem access for cover art storage
- **Network:** IGDB API access required (cannot function offline)
- **Resources:** Minimal resource requirements suitable for personal servers

### Development Setup
- **Environment:** Nix development shell (optional but recommended)
- **Database:** Automatic SQLite setup for development
- **Dependencies:** Single command setup with uv sync and npm install
- **Testing:** Comprehensive test suites ensure deployment reliability
# Docker/Podman Compose Development Setup Design

**Date:** 2025-12-13
**Status:** Approved

## Overview

Development-focused container orchestration for Nexorious. This setup provides a consistent development environment with hot reload for both backend and frontend, and serves as the foundation for the background task system.

## Goals

- Provide reproducible development environment
- Enable hot reload for rapid iteration
- Support both Docker Compose and Podman Compose (rootless)
- Serve as foundation for background task system (worker/scheduler services added later)

## Scope

**In scope:**
- Development environment only
- PostgreSQL, backend, and frontend services
- Hot reload via bind mounts
- Docker and Podman compatibility

**Out of scope (deferred):**
- Production configuration
- Worker and scheduler services (see background task system design)
- Additional services (pgAdmin, mail server, etc.)

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Database | PostgreSQL only | Aligns with background task design, avoids dialect mismatches |
| Hot reload | Bind mounts + anonymous volumes | Works with both Docker and Podman |
| uv pattern | Astral's "developing in container" guide | Official recommendation for uv in Docker |
| Podman mode | Rootless with `:Z` SELinux labels | Default secure mode |
| DB persistence | Named volume | Avoids rootless permission issues |
| Configuration | Hardcoded dev defaults | Simple, no env file management for dev |

## Project Structure

```
nexorious/
├── docker-compose.yml          # Main compose file
├── backend/
│   └── Dockerfile              # Backend container definition
└── frontend/
    └── Dockerfile              # Frontend container definition
```

## Backend Dockerfile

```dockerfile
# backend/Dockerfile
FROM ghcr.io/astral-sh/uv:python3.13-bookworm-slim

WORKDIR /app

# Enable bytecode compilation for faster startup
ENV UV_COMPILE_BYTECODE=1

# Copy dependency files first (better layer caching)
COPY pyproject.toml uv.lock ./

# Install dependencies (without the project itself)
RUN uv sync --frozen --no-install-project

# Copy source code
COPY . .

# Install the project
RUN uv sync --frozen

# Run with uvicorn (--reload enabled via compose command override)
CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

Key points:
- Uses official uv image with Python 3.13
- Two-stage `uv sync` for better layer caching (deps first, then project)
- `UV_COMPILE_BYTECODE=1` for faster startup
- Default CMD without `--reload` (compose overrides for dev)

## Frontend Dockerfile

```dockerfile
# frontend/Dockerfile
FROM node:22-slim

WORKDIR /app

# Copy dependency files first (better layer caching)
COPY package.json package-lock.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Expose Vite dev server port
EXPOSE 5173

# Run dev server (host 0.0.0.0 to be accessible from outside container)
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
```

Key points:
- Node 22 LTS (slim variant)
- `npm ci` for reproducible installs from lockfile
- `--host 0.0.0.0` required for container networking

## Docker Compose Configuration

```yaml
# docker-compose.yml
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: nexorious
      POSTGRES_PASSWORD: nexorious
      POSTGRES_DB: nexorious
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U nexorious"]
      interval: 5s
      timeout: 5s
      retries: 5

  api:
    build: ./backend
    command: uv run uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload
    environment:
      DATABASE_URL: postgresql://nexorious:nexorious@db:5432/nexorious
    ports:
      - "8000:8000"
    volumes:
      - ./backend:/app:Z
      - /app/.venv
    depends_on:
      db:
        condition: service_healthy

  frontend:
    build: ./frontend
    ports:
      - "5173:5173"
    environment:
      PUBLIC_API_URL: http://localhost:8000
    volumes:
      - ./frontend:/app:Z
      - /app/node_modules
    depends_on:
      - api

volumes:
  postgres_data:
```

Key points:
- `:Z` suffix on bind mounts for SELinux (rootless Podman)
- Anonymous volumes (`/app/.venv`, `/app/node_modules`) exclude platform-specific dirs
- Backend waits for DB health check before starting
- `--reload` flag on backend for hot reload
- `PUBLIC_API_URL` points to localhost (browser accesses API directly)

## Usage

### Starting the environment

```bash
# Docker
docker compose up --build

# Podman
podman-compose up --build
```

### Stopping

```bash
# Docker
docker compose down        # Keeps volumes
docker compose down -v     # Removes volumes (fresh DB)

# Podman
podman-compose down        # Keeps volumes
podman-compose down -v     # Removes volumes (fresh DB)
```

### Accessing services

| Service | URL |
|---------|-----|
| Frontend | http://localhost:5173 |
| Backend API | http://localhost:8000 |
| API Docs | http://localhost:8000/docs |
| PostgreSQL | localhost:5432 (user: nexorious, pass: nexorious) |

### Rebuilding after dependency changes

```bash
# Docker
docker compose build api        # Backend (pyproject.toml changed)
docker compose build frontend   # Frontend (package.json changed)

# Podman
podman-compose build api        # Backend (pyproject.toml changed)
podman-compose build frontend   # Frontend (package.json changed)
```

## Relationship to Other Plans

### Background Task System Design
That plan extends this compose setup by adding:
- `worker` service (same backend image, `taskiq worker` command)
- `scheduler` service (same backend image, `taskiq scheduler` command)

No changes needed to that design - it builds on this foundation.

### Export to File Design
Works with this setup unchanged. Export files are written to `./storage/exports/` which is covered by the backend bind mount.

## Future Considerations

- **Production compose:** Separate `docker-compose.prod.yml` without hot reload, with proper secrets management
- **CI integration:** Use Dockerfiles for CI builds and testing
- **Health checks for all services:** Add health checks to frontend and api services

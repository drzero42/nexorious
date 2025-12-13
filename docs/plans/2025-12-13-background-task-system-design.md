# Background Task System Design

## Overview

This document describes the architecture for switching from FastAPI's `BackgroundTasks` and in-memory session management to a proper background task processing system using taskiq with PostgreSQL as the broker and result backend.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Database | PostgreSQL-only | Enables PostgreSQL-specific features (LISTEN/NOTIFY, advisory locks), simplifies codebase |
| Task framework | taskiq + taskiq-pg | PostgreSQL as broker and result backend, no Redis/external services needed |
| Topology | 3 containers | API, Worker, Scheduler - clean separation of concerns |
| Scheduler | Singleton process | taskiq does not support leader election; must run exactly one scheduler |
| User schedules | Fan-out pattern | Periodic task checks who needs syncing, enqueues individual tasks |
| Task sessions | Per-task sessions | Each task manages its own SQLModel session |
| Result retention | 7 days | Medium retention for debugging/auditing |

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │     │                 │
│   API Server    │     │     Worker      │     │    Scheduler    │
│   (FastAPI)     │     │    (taskiq)     │     │    (taskiq)     │
│                 │     │                 │     │                 │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │                         │
                    │       PostgreSQL        │
                    │                         │
                    │  • Application tables   │
                    │  • taskiq_messages      │
                    │  • taskiq_results       │
                    │                         │
                    └─────────────────────────┘
```

**Components:**

- **API Server**: Handles HTTP requests. Can enqueue tasks on-demand (e.g., "sync my Steam library now").
- **Worker**: Listens for tasks via PostgreSQL `LISTEN/NOTIFY`, executes them, stores results. Can be scaled horizontally.
- **Scheduler**: Runs cron-like schedules, enqueues tasks at the right time. Single instance only (no scaling).
- **PostgreSQL**: Single database for app data, task queue (broker), and task results.

## Dependencies

Add to `pyproject.toml`:

```toml
dependencies = [
    # ... existing deps ...
    "taskiq>=0.11",
    "taskiq-pg>=0.5",
]
```

Remove:
- `aiosqlite>=0.21.0`

## Project Structure

```
backend/
├── app/
│   ├── worker/
│   │   ├── __init__.py
│   │   ├── broker.py          # Broker configuration
│   │   ├── tasks/
│   │   │   ├── __init__.py
│   │   │   ├── sync.py        # External service sync tasks
│   │   │   ├── maintenance.py # DB cleanup, cache tasks
│   │   │   └── reports.py     # Report generation tasks
│   │   └── schedules.py       # Cron schedule definitions
```

## Broker Configuration

```python
# app/worker/broker.py
from taskiq_pg import AsyncpgBroker, AsyncpgResultBackend
from taskiq.serializers import JSONSerializer

from app.core.config import settings


def _get_database_url() -> str:
    """Deferred DSN resolution for broker startup."""
    return settings.database_url


result_backend = AsyncpgResultBackend(
    dsn=_get_database_url,
    serializer=JSONSerializer(),
    keep_results=True,
)

broker = AsyncpgBroker(
    dsn=_get_database_url,
).with_result_backend(result_backend)
```

Key points:
- DSN resolved via callable (not at import time) to avoid config loading issues
- `JSONSerializer` for human-readable results in the database
- Results retained for later cleanup by maintenance task

## Task Definitions

### Sync Tasks

```python
# app/worker/tasks/sync.py
from app.worker.broker import broker
from app.database import get_session_context


@broker.task()
async def check_pending_syncs() -> dict:
    """
    Periodic task that checks which users need syncing.
    Runs every 15 minutes, enqueues individual sync tasks.
    """
    async with get_session_context() as session:
        users_needing_sync = await get_users_needing_sync(session)

        for user in users_needing_sync:
            await sync_steam_library.kiq(user_id=str(user.id))

        return {"enqueued": len(users_needing_sync)}


@broker.task()
async def sync_steam_library(user_id: str) -> dict:
    """Sync a single user's Steam library."""
    async with get_session_context() as session:
        # ... actual sync logic ...
        return {"user_id": user_id, "games_synced": 42}


@broker.task()
async def refresh_igdb_metadata(game_id: str) -> dict:
    """Refresh IGDB metadata for a game."""
    async with get_session_context() as session:
        # ... refresh logic ...
        return {"game_id": game_id, "updated": True}
```

### Maintenance Tasks

```python
# app/worker/tasks/maintenance.py
from app.worker.broker import broker


@broker.task()
async def cleanup_task_results(days_old: int = 7) -> dict:
    """Remove task results older than N days."""
    # ... cleanup logic ...
    return {"deleted_count": 150}


@broker.task()
async def cleanup_expired_sessions() -> dict:
    """Remove expired batch sessions from database."""
    # ... cleanup logic ...
    return {"deleted_count": 5}
```

### Report Tasks

```python
# app/worker/tasks/reports.py
from app.worker.broker import broker


@broker.task()
async def generate_collection_stats(user_id: str) -> dict:
    """Generate collection statistics report."""
    # ... report logic ...
    return {"user_id": user_id, "report_path": "/reports/..."}
```

## Schedule Definitions

```python
# app/worker/schedules.py
from taskiq import TaskiqScheduler
from taskiq_pg import AsyncpgScheduleSource

from app.worker.broker import broker
from app.worker.tasks.sync import check_pending_syncs
from app.worker.tasks.maintenance import cleanup_task_results, cleanup_expired_sessions
from app.core.config import settings


def _get_database_url() -> str:
    return settings.database_url


schedule_source = AsyncpgScheduleSource(
    dsn=_get_database_url,
)

scheduler = TaskiqScheduler(broker, [schedule_source])


# Static schedules
cleanup_task_results.schedule_by_cron(
    scheduler,
    "0 3 * * *",  # Daily at 3 AM
    days_old=7,
)

cleanup_expired_sessions.schedule_by_cron(
    scheduler,
    "*/30 * * * *",  # Every 30 minutes
)

check_pending_syncs.schedule_by_cron(
    scheduler,
    "*/15 * * * *",  # Every 15 minutes
)
```

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
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U nexorious"]
      interval: 5s
      timeout: 5s
      retries: 5

  api:
    build: ./backend
    command: uvicorn app.main:app --host 0.0.0.0 --port 8000
    environment:
      DATABASE_URL: postgresql://nexorious:nexorious@db:5432/nexorious
    ports:
      - "8000:8000"
    depends_on:
      db:
        condition: service_healthy

  worker:
    build: ./backend
    command: taskiq worker app.worker.broker:broker app.worker.tasks
    environment:
      DATABASE_URL: postgresql://nexorious:nexorious@db:5432/nexorious
    depends_on:
      db:
        condition: service_healthy

  scheduler:
    build: ./backend
    command: taskiq scheduler app.worker.schedules:scheduler
    environment:
      DATABASE_URL: postgresql://nexorious:nexorious@db:5432/nexorious
    depends_on:
      db:
        condition: service_healthy
    deploy:
      replicas: 1  # Must remain 1 - no scaling

  frontend:
    build: ./frontend
    ports:
      - "5173:5173"
    environment:
      PUBLIC_API_URL: http://api:8000
    depends_on:
      - api

volumes:
  postgres_data:
```

**Notes:**
- All backend services use the same Docker image with different commands
- Scheduler explicitly limited to 1 replica (no leader election in taskiq)
- Workers can be scaled: `docker compose up --scale worker=3`
- Health check ensures DB is ready before services start

## Migration Path

### 1. Remove SQLite Support

- Remove `aiosqlite` dependency from `pyproject.toml`
- Update `app/database.py` to only support PostgreSQL
- Simplify Alembic configuration (single dialect)
- Update tests to use PostgreSQL (testcontainers or similar)

### 2. Replace In-Memory BatchSessionManager

Current state:
- `batch_session_manager.py` stores sessions in memory
- Sessions lost on server restart

New approach:
- Batch operations become taskiq tasks
- Progress stored in taskiq result backend
- API endpoints query task status via task ID

### 3. Replace FastAPI BackgroundTasks

Current state:
- `darkadia.py` uses `BackgroundTasks` for CSV import

New approach:
- Enqueue as taskiq task
- Return task ID to client
- Client polls for status via task ID

### 4. New Database Tables

Managed automatically by taskiq-pg:
- `taskiq_messages` - Task queue
- `taskiq_results` - Task results

Application tables (via Alembic migration):
- User sync preferences (for fan-out pattern)

## Breaking Changes

Users upgrading will need to:

1. **PostgreSQL required** - No SQLite fallback
2. **Docker Compose changes** - New worker and scheduler containers
3. **Environment variables** - Database URL must be PostgreSQL format

## Future Considerations

- **Monitoring**: taskiq provides hooks for metrics; consider Prometheus integration
- **Retries**: taskiq supports retry policies; configure per-task as needed
- **Priority queues**: taskiq-pg supports priorities if needed later
- **Dead letter queue**: Failed tasks can be tracked via result backend

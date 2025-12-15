# Lazy Database Engine Initialization

## Problem

Tests fail when PostgreSQL container is not running because `database.py` creates the database engine at module import time. When test files import `app.main`, it triggers:

1. Import of `app.core.database`
2. Immediate creation of `engine = create_engine(settings.database_url, ...)`
3. Connection attempt to `localhost:5432`
4. Failure if PostgreSQL isn't running

Testcontainers creates its own isolated PostgreSQL on a random port, but the app's main engine still tries to connect to the "real" database URL at import time.

## Solution

Lazy initialization of the database engine - defer creation until first use.

## Design

### Core Engine Changes (database.py)

Replace eager engine creation:

```python
# Before (eager - fails on import if DB unavailable)
engine = create_engine(settings.database_url, ...)

# After (lazy - only connects when first used)
_engine = None

def get_engine():
    global _engine
    if _engine is None:
        if not settings.database_url.startswith("postgresql"):
            raise ValueError("Only PostgreSQL is supported...")
        _engine = create_engine(
            settings.database_url,
            echo=settings.debug,
            pool_pre_ping=True
        )
    return _engine

def _reset_engine():
    """Reset engine for testing. Not for production use."""
    global _engine
    _engine = None
```

### Update Consumers in database.py

All functions using `engine` directly will call `get_engine()`:

1. `create_db_and_tables()` - `SQLModel.metadata.create_all(get_engine())`
2. `get_session()` - `with Session(get_engine()) as session:`
3. `get_sync_session()` - `return Session(get_engine())`
4. `get_session_context()` - `session = Session(get_engine())`

### Test Integration (integration_test_utils.py)

Inject testcontainer engine before tests run:

```python
from ..core.database import _reset_engine
import app.core.database as db_module

@pytest.fixture(scope="session", autouse=True)
def setup_test_database():
    """Set up the test database once per session."""
    # Reset any existing engine
    _reset_engine()

    # Create testcontainer engine
    container = get_postgres_container()
    test_engine = create_engine(container.get_connection_url(), ...)

    # Inject it into the database module
    db_module._engine = test_engine

    SQLModel.metadata.create_all(test_engine)
    yield
```

## Files Changed

| File | Change |
|------|--------|
| `backend/app/core/database.py` | Replace eager `engine` with lazy `get_engine()`, add `_reset_engine()` |
| `backend/app/tests/integration_test_utils.py` | Inject testcontainer engine into `db_module._engine` |

## Behavior After Change

- `uv run pytest` works without external PostgreSQL running
- App startup still validates PostgreSQL URL (deferred to first use)
- Tests use isolated testcontainer database
- No changes required to individual test files

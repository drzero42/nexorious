# Design: Backend Support for Individual DB Connection Env Vars

**Date:** 2026-03-23
**Status:** Approved
**Prerequisite for:** Helm chart component-mode external database secret support

## Problem

The backend currently accepts only a single `DATABASE_URL` env var for database configuration. Helm's component-mode external database secret support requires injecting individual connection parameters (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`) via a Kubernetes secret. There is no way to use those without constructing a full URL outside the app.

## Goals

- Accept individual DB connection env vars as an alternative to `DATABASE_URL`
- `DATABASE_URL` takes priority when set
- Fall back to sensible defaults for any unset individual vars
- Zero changes to existing consumers of `settings.database_url`

## Non-Goals

- Changing any database consumer code (`database.py`, `backup_service.py`, `alembic/env.py`, `main.py`)
- Supporting database backends other than PostgreSQL
- Validating that the constructed URL is actually reachable at startup

## Design

### Priority Rules

| `DATABASE_URL` set? | Individual vars set? | Result |
|---|---|---|
| Yes | Any | `DATABASE_URL` used directly |
| No | Some/all | URL constructed from vars; missing vars fall back to defaults |
| No | None | URL constructed from all defaults → existing dev default |

"Set" means the env var is present and non-empty. The default value of `DATABASE_URL` does not count as "set" — if the user does not provide it, it is `None`.

### Settings Changes (`backend/app/core/config.py`)

Add five individual DB vars with defaults matching the existing dev URL (`postgresql://nexorious:nexorious@localhost:5432/nexorious`):

```python
db_host: str = Field(default="localhost", description="PostgreSQL host")
db_port: int = Field(default=5432, description="PostgreSQL port")
db_user: str = Field(default="nexorious", description="PostgreSQL username")
db_password: str = Field(default="nexorious", description="PostgreSQL password")
db_name: str = Field(default="nexorious", description="PostgreSQL database name")
```

Change `database_url` from `str` with a hardcoded default to `Optional[str] = None`:

```python
database_url: Optional[str] = Field(
    default=None,
    description="PostgreSQL database URL. If set, takes priority over individual DB_* vars."
)
```

Add a `model_validator(mode='after')` that runs after all fields (including env vars) are resolved:

```python
@model_validator(mode='after')
def resolve_database_url(self) -> 'Settings':
    if self.database_url is None:
        self.database_url = (
            f"postgresql://{self.db_user}:{self.db_password}"
            f"@{self.db_host}:{self.db_port}/{self.db_name}"
        )
    return self
```

After validation, `database_url` is always a non-`None` `str`. The existing postgresql-only check in `database.py` continues to work without changes.

### Env Var Mapping

pydantic-settings maps field names to env vars case-insensitively by default (`case_sensitive=False` is already set). The individual field names map to:

| Field | Env Var |
|---|---|
| `db_host` | `DB_HOST` |
| `db_port` | `DB_PORT` |
| `db_user` | `DB_USER` |
| `db_password` | `DB_PASSWORD` |
| `db_name` | `DB_NAME` |
| `database_url` | `DATABASE_URL` |

### No Consumer Changes

All existing callers of `settings.database_url` are unaffected:

- `backend/app/core/database.py` — `get_engine()`, `run_alembic_migrations()`
- `backend/app/alembic/env.py` — alembic config setup
- `backend/app/services/backup_service.py` — URL parsing for pg_dump/pg_restore
- `backend/app/main.py` — debug log

### Tests (`backend/app/tests/test_settings.py`)

New test file covering:

1. **Default (no env vars)** — `database_url` equals `postgresql://nexorious:nexorious@localhost:5432/nexorious`
2. **`DATABASE_URL` set** — used directly; individual vars ignored even if also set
3. **Individual vars set, no `DATABASE_URL`** — URL constructed correctly from all five vars
4. **Partial individual vars** — unset vars use their defaults in the constructed URL

Tests use `Settings(database_url=..., db_host=..., ...)` direct construction (not env var patching) to stay fast and isolated.

## Implementation Steps

1. Update `backend/app/core/config.py` — add individual vars, change `database_url` to `Optional[str]`, add `model_validator`
2. Add `backend/app/tests/test_settings.py` with the four test cases above
3. Run `uv run pyrefly check` and `uv run pytest` to verify

## Risks / Edge Cases

- **`DB_PASSWORD` with special characters** — pydantic-settings reads env vars as strings before URL construction; special characters in `DB_PASSWORD` must be percent-encoded if the password contains `@`, `/`, `:`. This is a known limitation of connection-string passwords and is not introduced by this change (it applies equally to `DATABASE_URL` today).
- **`DB_PORT` validation** — pydantic will coerce `DB_PORT` from string to `int` automatically and raise a `ValidationError` on invalid values, which is acceptable startup behavior.

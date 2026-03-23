# Design: Backend Support for Individual DB Connection Env Vars

**Date:** 2026-03-23
**Status:** Approved
**Prerequisite for:** Helm chart component-mode external database secret support

## Problem

The backend currently accepts only a single `DATABASE_URL` env var for database configuration. Helm's component-mode external database secret support requires injecting individual connection parameters (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`) via a Kubernetes secret. There is no way to use those without constructing a full URL outside the app.

## Goals

- Accept individual DB connection env vars as an alternative to `DATABASE_URL`
- `DATABASE_URL` takes priority when set (non-empty)
- Fall back to sensible defaults for any unset individual vars
- Zero changes to existing consumers of `settings.database_url`
- No type-checker regressions in consumer code

## Non-Goals

- Changing any database consumer code (`database.py`, `backup_service.py`, `alembic/env.py`, `main.py`)
- Supporting database backends other than PostgreSQL
- Validating that the constructed URL is actually reachable at startup

## Design

### Priority Rules

| `DATABASE_URL` set? | Individual vars set? | Result |
|---|---|---|
| Yes (non-empty) | Any | `DATABASE_URL` used directly |
| No / empty string | Some/all | URL constructed from vars; missing vars fall back to defaults |
| No / empty string | None | URL constructed from all defaults → existing dev default |

"Set" means the env var is present **and non-empty**. An empty-string `DATABASE_URL` is treated the same as absent.

### Settings Changes (`backend/app/core/config.py`)

Add five individual DB vars with defaults matching the existing dev URL (`postgresql://nexorious:nexorious@localhost:5432/nexorious`):

```python
db_host: str = Field(default="localhost", description="PostgreSQL host")
db_port: int = Field(default=5432, description="PostgreSQL port")
db_user: str = Field(default="nexorious", description="PostgreSQL username")
db_password: str = Field(default="nexorious", description="PostgreSQL password")
db_name: str = Field(default="nexorious", description="PostgreSQL database name")
```

Change `database_url` from `str` with a hardcoded default to `str = ""` (empty-string sentinel):

```python
database_url: str = Field(
    default="",
    description=(
        "PostgreSQL database URL. If set (non-empty), takes priority over individual "
        "DB_* vars. Format: postgresql://user:pass@host:port/db"
    )
)
```

Using an empty-string sentinel (rather than `Optional[str]`) keeps the field typed as `str` at all times, so **no consumer code gets a type-checker regression** — callers that do `settings.database_url.startswith(...)` etc. remain valid.

Add a `model_validator(mode='after')` that runs after all fields (including env vars) are resolved:

```python
from urllib.parse import quote as urlquote

@model_validator(mode='after')
def resolve_database_url(self) -> 'Settings':
    if not self.database_url:
        user = urlquote(self.db_user, safe='')
        password = urlquote(self.db_password, safe='')
        dbname = urlquote(self.db_name, safe='')
        self.database_url = (
            f"postgresql://{user}:{password}"
            f"@{self.db_host}:{self.db_port}/{dbname}"
        )
    return self
```

`urllib.parse.quote(value, safe='')` percent-encodes any characters that are invalid in URL userinfo or path segments (e.g. `@`, `/`, `:`, space). This applies to `db_user`, `db_password`, and `db_name`. `db_host` and `db_port` do not require encoding (hostnames and port numbers contain no special URL characters).

Note: The existing `Settings` `model_config` does not set `frozen=True`, so direct attribute assignment inside a `mode='after'` validator is safe. If `frozen=True` were ever added, `object.__setattr__(self, 'database_url', ...)` would be needed instead.

After validation, `database_url` is always a **non-empty `str`**. The existing postgresql-only check in `database.py` (`settings.database_url.startswith("postgresql")`) continues to work without changes.

### Env Var Mapping

pydantic-settings maps field names to env vars case-insensitively (`case_sensitive=False` is already set in the model config):

| Field | Env Var |
|---|---|
| `db_host` | `DB_HOST` |
| `db_port` | `DB_PORT` |
| `db_user` | `DB_USER` |
| `db_password` | `DB_PASSWORD` |
| `db_name` | `DB_NAME` |
| `database_url` | `DATABASE_URL` |

`DB_PORT` is coerced from string to `int` by pydantic; an invalid value (e.g. `"abc"`) raises a `ValidationError` at startup, which is acceptable behavior.

### No Consumer Changes

All existing callers of `settings.database_url` are unaffected:

- `backend/app/core/database.py` — `get_engine()`, `run_alembic_migrations()`
- `backend/app/alembic/env.py` — alembic config setup
- `backend/app/services/backup_service.py` — URL parsing for pg_dump/pg_restore
- `backend/app/main.py` — debug log

### Tests (`backend/app/tests/test_settings.py`)

New test file covering:

1. **Default (no env vars)** — `Settings()` direct construction with no args produces `database_url == "postgresql://nexorious:nexorious@localhost:5432/nexorious"`
2. **`DATABASE_URL` set** — `database_url` passed directly is used as-is; individual vars are ignored even if also set
3. **`DATABASE_URL` empty string** — treated as absent; URL is constructed from individual vars
4. **Individual vars set, no `DATABASE_URL`** — URL constructed correctly from all five vars
5. **Partial individual vars** — unset vars use their defaults in the constructed URL
6. **Special characters in password/user** — verified to be percent-encoded in the constructed URL (e.g. `p@ss` → `p%40ss`)
7. **Env var path (integration)** — at least one test uses `monkeypatch.setenv("DB_HOST", "db.example.com")` + `Settings()` to confirm that pydantic-settings actually reads the env var and the correct URL is produced

Test cases 1–6 use direct `Settings(...)` construction for speed. Test 7 uses `monkeypatch` to cover the actual env var resolution path.

## Implementation Steps

1. Update `backend/app/core/config.py`:
   - Add `db_host`, `db_port`, `db_user`, `db_password`, `db_name` fields
   - Change `database_url` default from hardcoded URL to `""`
   - Add `resolve_database_url` model validator with percent-encoding
2. Add `backend/app/tests/test_settings.py` with the seven test cases above
3. Run `uv run pyrefly check` — confirm zero new type errors
4. Run `uv run pytest` — confirm all tests pass

## Risks / Edge Cases

- **Special characters in `DB_PASSWORD`, `DB_USER`, `DB_NAME`** — handled by `urllib.parse.quote(..., safe='')` in the validator. Characters like `@`, `/`, `:`, and space are percent-encoded before URL construction. Operators do **not** need to pre-encode these values.
- **`DATABASE_URL` with special characters** — unchanged from today: if `DATABASE_URL` is set directly, the operator is responsible for correct encoding, as the value is used verbatim.
- **`DB_PORT` type coercion** — pydantic automatically coerces the string env var to `int`. An invalid port value raises `ValidationError` at startup.
- **Empty-string `DATABASE_URL`** — treated as absent (validator checks `if not self.database_url`), consistent with the stated "set means non-empty" rule.
- **Model freezing** — current `Settings` is not frozen; noted for future reference.

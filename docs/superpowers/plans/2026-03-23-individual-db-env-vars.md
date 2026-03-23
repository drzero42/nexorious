# Individual DB Connection Env Vars Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow the backend to accept `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` as an alternative to `DATABASE_URL`, with `DATABASE_URL` taking priority when set.

**Architecture:** Add five individual DB fields to `Settings` with sensible defaults, change `database_url` from a hardcoded-default `str` to an empty-string sentinel, and add a `model_validator(mode='after')` that constructs the URL from parts (with percent-encoding) when `database_url` is empty. All existing consumers remain untouched.

**Tech Stack:** Python 3.13, pydantic-settings v2, pydantic v2, urllib.parse (stdlib), pytest

**Spec:** `docs/superpowers/specs/2026-03-23-individual-db-env-vars-design.md`

---

## File Map

| Action | File | What changes |
|---|---|---|
| Modify | `backend/app/core/config.py` | Add 5 individual DB fields, change `database_url` default to `""`, add `model_validator`, add `urlquote` import |
| Create | `backend/app/tests/test_settings.py` | 8 test cases covering all priority/encoding/default scenarios |

No other files require changes.

---

### Task 0: Create feature branch

- [ ] **Step 0.1: Push main and create a feature branch**

```bash
cd /home/abo/workspace/home/nexorious && git push && git checkout -b feat/individual-db-env-vars
```

---

### Task 1: Write the failing tests

**Files:**
- Create: `backend/app/tests/test_settings.py`

- [ ] **Step 1.1: Create the test file with all 8 test cases**

Create `backend/app/tests/test_settings.py`:

```python
"""Tests for Settings DB connection configuration.

All tests that rely on constructed URLs pass _env_file=None and clear
all DB_* env vars to ensure isolation regardless of the developer's
local shell environment or CI configuration.
"""
import pytest
from app.core.config import Settings

# All DB_* env var names that might leak in from the shell/CI
_DB_ENV_VARS = ("DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DATABASE_URL")


class TestSettingsDatabaseUrl:
    """Test DATABASE_URL priority and individual DB var construction."""

    def test_defaults_produce_dev_url(self, monkeypatch):
        """No env vars set → URL matches the existing dev default."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(_env_file=None)
        assert s.database_url == "postgresql://nexorious:nexorious@localhost:5432/nexorious"

    def test_database_url_takes_priority(self, monkeypatch):
        """DATABASE_URL set → used as-is; individual vars are ignored even if also set."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(
            _env_file=None,
            database_url="postgresql://custom:secret@db.example.com:5433/mydb",
            db_host="should-be-ignored",
            db_port=9999,
            db_user="ignored",
            db_password="ignored",
            db_name="ignored",
        )
        assert s.database_url == "postgresql://custom:secret@db.example.com:5433/mydb"

    def test_empty_database_url_with_defaults_falls_back_to_constructed_url(self, monkeypatch):
        """Empty-string DATABASE_URL, no individual vars → URL built from defaults."""
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        s = Settings(_env_file=None, database_url="")
        assert s.database_url == "postgresql://nexorious:nexorious@localhost:5432/nexorious"

    def test_all_individual_vars_construct_url(self):
        """All five individual vars set explicitly → URL constructed from them."""
        s = Settings(
            _env_file=None,
            database_url="",
            db_host="db.example.com",
            db_port=5433,
            db_user="myuser",
            db_password="mypassword",
            db_name="mydb",
        )
        assert s.database_url == "postgresql://myuser:mypassword@db.example.com:5433/mydb"

    def test_partial_individual_vars_use_defaults_for_missing(self):
        """Only db_host set → remaining vars fall back to their field defaults."""
        s = Settings(
            _env_file=None,
            database_url="",
            db_host="custom-host",
        )
        assert s.database_url == "postgresql://nexorious:nexorious@custom-host:5432/nexorious"

    def test_special_characters_in_user_and_password_are_percent_encoded(self):
        """Special chars in db_user and db_password are percent-encoded in the URL."""
        s = Settings(
            _env_file=None,
            database_url="",
            db_user="user@domain",
            db_password="p@ss/word:secret",
            db_host="localhost",
            db_port=5432,
            db_name="nexorious",
        )
        # @ → %40, / → %2F, : → %3A
        assert "user%40domain" in s.database_url
        assert "p%40ss%2Fword%3Asecret" in s.database_url
        # host and port must appear unencoded
        assert "@localhost:5432" in s.database_url

    def test_special_characters_in_db_name_are_percent_encoded(self):
        """Special chars in db_name are percent-encoded in the URL path segment."""
        s = Settings(
            _env_file=None,
            database_url="",
            db_host="localhost",
            db_port=5432,
            db_user="nexorious",
            db_password="nexorious",
            db_name="my db/name",
        )
        # space → %20, / → %2F
        assert "my%20db%2Fname" in s.database_url

    def test_env_var_path_reads_db_host_from_environment(self, monkeypatch):
        """Env var DB_HOST is read by pydantic-settings and appears in the URL."""
        # Clear all DB_* vars so only our explicit one is active
        for var in _DB_ENV_VARS:
            monkeypatch.delenv(var, raising=False)
        monkeypatch.setenv("DB_HOST", "env-injected-host")
        s = Settings(_env_file=None)
        assert "env-injected-host" in s.database_url
        assert s.database_url.startswith("postgresql://")
```

- [ ] **Step 1.2: Run the tests to confirm they fail**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_settings.py -v 2>&1 | head -60
```

Expected: Most tests fail with `TypeError` (unknown kwargs `db_host`, `db_port`, etc.). `test_defaults_produce_dev_url` may accidentally pass on the old code because the old default URL matches the assertion — that is acceptable. What matters is that the new test cases that require `db_host` etc. all fail.

---

### Task 2: Implement the feature in config.py

**Files:**
- Modify: `backend/app/core/config.py`

Note: Steps 2.1–2.3 all modify `config.py`. Apply all three before running any tests — partial application will leave the file in a broken state.

- [ ] **Step 2.1: Add imports**

In `backend/app/core/config.py`:

1. Add `model_validator` to the **existing** `from pydantic import ...` line. Do not add a new line — just extend the existing import.
2. Add `from urllib.parse import quote as urlquote` as a new line. Place it after the existing import lines, matching the ordering style already in the file (check the current import block to confirm placement).

After these edits, the import block should look like:

```python
from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field, field_validator, model_validator
from typing import Optional, Union
from urllib.parse import quote as urlquote
```

- [ ] **Step 2.2: Replace the `database_url` field block**

Find the database section in `config.py` that currently reads:

```python
    # Database (PostgreSQL only)
    database_url: str = Field(
        default="postgresql://nexorious:nexorious@localhost:5432/nexorious",
        description="PostgreSQL database URL. Format: postgresql://user:pass@host:port/db"
    )
```

Replace it with:

```python
    # Database (PostgreSQL only)
    # Individual vars are used to construct the URL when DATABASE_URL is not set.
    db_host: str = Field(default="localhost", description="PostgreSQL host")
    db_port: int = Field(default=5432, description="PostgreSQL port")
    db_user: str = Field(default="nexorious", description="PostgreSQL username")
    db_password: str = Field(default="nexorious", description="PostgreSQL password")
    db_name: str = Field(default="nexorious", description="PostgreSQL database name")

    database_url: str = Field(
        default="",
        description=(
            "PostgreSQL database URL. If set (non-empty), takes priority over individual "
            "DB_* vars. Format: postgresql://user:pass@host:port/db"
        )
    )
```

- [ ] **Step 2.3: Add the `model_validator` method**

Add the following method inside the `Settings` class, immediately after the existing `parse_cors_origins` `@field_validator` method:

```python
    @model_validator(mode='after')
    def resolve_database_url(self) -> 'Settings':
        """Construct database_url from individual DB vars when DATABASE_URL is not set."""
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

- [ ] **Step 2.4: Run the settings tests to confirm they all pass**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_settings.py -v
```

Expected: 8 tests, all PASSED.

---

### Task 3: Quality checks and commit

- [ ] **Step 3.1: Run ruff linting**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run ruff check .
```

Expected: No errors. If `urlquote` is flagged as unused (`F401`), confirm the import name matches the usage in `resolve_database_url`. If an import ordering error (`I001`) is reported, reorder the `from urllib.parse` line to match ruff's expectation.

- [ ] **Step 3.2: Run pyrefly type check**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

Expected: Zero errors. If pyrefly flags `self.database_url =` in the validator, see the troubleshooting section below.

- [ ] **Step 3.3: Run the full test suite with coverage**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing 2>&1 | tail -20
```

Expected: All tests pass, coverage ≥ 80%.

- [ ] **Step 3.4: Commit**

```bash
cd /home/abo/workspace/home/nexorious && git add backend/app/core/config.py backend/app/tests/test_settings.py && git commit -m "$(cat <<'EOF'
feat: accept individual DB_* env vars as alternative to DATABASE_URL

DATABASE_URL takes priority when set (non-empty). Individual vars
(DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME) are used to
construct the URL when DATABASE_URL is absent or empty. Percent-
encoding is applied to user, password, and dbname components.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Troubleshooting

**`TypeError: unexpected keyword argument 'db_host'`** — The field was not added to `Settings` in step 2.2. Verify the field block was added inside the class body.

**`model_validator` not found** — Check that `model_validator` was added to the existing pydantic import line in step 2.1 (not as a new import line).

**Env var test fails unexpectedly** — The test clears all `DB_*` and `DATABASE_URL` env vars using `monkeypatch.delenv` before setting only `DB_HOST`. If it still fails, check whether a `.env` file is being loaded despite `_env_file=None` (this should not happen, but verify the pydantic-settings version behaviour).

**pyrefly error on `self.database_url =`** — This would only occur if the model were frozen (`frozen=True` in `model_config`). The current config is not frozen, so this should not happen. If it does, replace the assignment with `object.__setattr__(self, 'database_url', ...)`.

**ruff `I001` import order error** — Move `from urllib.parse import quote as urlquote` above the `from pydantic_settings ...` line (stdlib before third-party per PEP 8). If the existing file already violates this ordering, check the project's ruff configuration in `pyproject.toml` to see if isort rules are disabled.

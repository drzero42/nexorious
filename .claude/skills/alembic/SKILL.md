---
name: alembic-migrations
description: Use when creating a database migration, adding or modifying SQLModel models, running alembic upgrade, or when schema changes need to be applied. Use when autogenerate produces an empty migration or misses expected changes.
---

# Alembic Database Migrations

All migrations use `uv run alembic` from `backend/`. **Never write migration files manually — always use `--autogenerate`.**

## Core Workflow

```bash
cd /home/abo/workspace/home/nexorious/backend

# 1. After changing a model:
uv run alembic revision --autogenerate -m "brief description"

# 2. Review the generated file in app/alembic/versions/ before applying

# 3. Apply:
uv run alembic upgrade head
```

## Critical: env.py Model Imports

`app/alembic/env.py` must import every model or autogenerate silently misses it.

When adding a **new model file**, add it to the import block at line 17:

```python
from app.models import (  # noqa: F401
    User, UserSession, Platform, Storefront, PlatformStorefront,
    Game, UserGame, UserGamePlatform, Tag, UserGameTag, Wishlist, Job, JobItem,
    # Add new models here
)
```

If autogenerate produces an empty migration and you expected changes, a missing import here is the most likely cause.

## Review Checklist Before Applying

- [ ] `upgrade()` contains expected operations
- [ ] `downgrade()` is present and correct
- [ ] No unexpected `drop_table` / `drop_column`
- [ ] Enum changes handled (autogenerate cannot detect them — see references/gotchas.md)
- [ ] Data migrations added if needed (autogenerate never adds these)

## Status Commands

```bash
uv run alembic current   # Applied revision
uv run alembic heads     # Should show exactly 1 head
uv run alembic check     # DB vs models diff
```

## NOT NULL Column Pattern

```python
def upgrade():
    op.add_column('table', sa.Column('col', sa.String(),
                   nullable=False, server_default='default'))
    op.alter_column('table', 'col', server_default=None)

def downgrade():
    op.drop_column('table', 'col')
```

## See Also

`references/gotchas.md` — PostgreSQL enum changes, autogenerate limits, multiple heads

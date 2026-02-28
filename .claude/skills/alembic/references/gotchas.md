# Alembic Gotchas & Edge Cases

## PostgreSQL Enum Type Changes

Autogenerate **cannot modify existing enum types**. Adding a value to a Python `Enum` class produces an empty migration. Write it manually:

```python
def upgrade():
    op.execute("ALTER TYPE myenum ADD VALUE IF NOT EXISTS 'NEW_VALUE'")

def downgrade():
    # PostgreSQL cannot remove enum values without recreating the type
    op.execute("ALTER TYPE myenum RENAME TO myenum_old")
    op.execute("CREATE TYPE myenum AS ENUM('VALUE1', 'VALUE2')")
    op.execute("ALTER TABLE mytable ALTER COLUMN col TYPE myenum USING col::text::myenum")
    op.execute("DROP TYPE myenum_old")
```

For new enum types on existing tables, use `create_type=False` if the type already exists:

```python
op.add_column('table', sa.Column('col',
    sa.Enum('A', 'B', name='myenum', create_type=False), nullable=False))
```

## Autogenerate Does NOT Detect

| What | Workaround |
|------|------------|
| Enum value additions | Write raw `ALTER TYPE` SQL |
| `CHECK` constraints | Add manually |
| Partial indexes | `op.create_index(..., postgresql_where=...)` |
| Functions / triggers | Always manual |
| `server_default` value changes | Verify carefully |
| SQLModel `@computed_field` | Python-only, no DB column — expected |

After autogenerate, run `uv run alembic check` to confirm DB is in sync.

## Multiple Heads (Branch Conflicts)

Two branches created migrations independently → two heads → `upgrade head` errors.

```bash
uv run alembic heads          # Shows both head revisions
uv run alembic merge heads -m "merge branch migrations"
uv run alembic upgrade head
```

The merge migration will have two `down_revision` values:
```python
down_revision = ('abc123', 'def456')
```

## Rolling Back

```bash
uv run alembic downgrade -1          # One step back
uv run alembic downgrade <revision>  # Specific revision
```

Rolling back destructive operations (`drop_column`, `drop_table`) cannot recover data.

## Diagnosing Empty Migrations

If autogenerate produces empty `upgrade()` / `downgrade()`:

1. **Missing import in env.py** — model not imported, Alembic can't see it
2. **Missing `table=True`** — class must be `class MyModel(SQLModel, table=True)`
3. **DB already matches** — run `uv run alembic check`; if clean, migration is correct
4. **`@computed_field`** — these don't create DB columns; expected to be absent

# Alembic Migrations Guide

Quick reference for creating optimal Alembic migrations with SQLModel and SQLite compatibility.

## Essential Commands

```bash
# Generate migration (ALWAYS use autogenerate)
uv run alembic revision --autogenerate -m "description of changes"

# Apply migrations
uv run alembic upgrade head

# Check current version
uv run alembic current

# View migration history
uv run alembic history --verbose
```

## Required SQLModel Import

**CRITICAL**: All generated migrations must include SQLModel import for proper functionality:

```python
# Add to top of every migration file
import sqlmodel
```

## SQLite Batch Operations (Required)

Use batch operations for ALL table modifications to ensure SQLite compatibility:

```python
def upgrade() -> None:
    # CORRECT: Use batch operations
    with op.batch_alter_table("games", schema=None) as batch_op:
        batch_op.add_column(sa.Column('new_field', sa.String(), nullable=True))
        batch_op.drop_column('old_field')

def downgrade() -> None:
    with op.batch_alter_table("games", schema=None) as batch_op:
        batch_op.add_column(sa.Column('old_field', sa.String(), nullable=True))
        batch_op.drop_column('new_field')
```

## Migration Patterns

### Adding Columns
```python
def upgrade() -> None:
    with op.batch_alter_table("table_name", schema=None) as batch_op:
        batch_op.add_column(sa.Column('new_column', sa.String(255), nullable=True))
```

### Modifying Columns
```python
def upgrade() -> None:
    with op.batch_alter_table("table_name", schema=None) as batch_op:
        batch_op.alter_column('column_name', 
                             existing_type=sa.String(),
                             type_=sa.Text(),
                             nullable=False)
```

### Adding Indexes
```python
def upgrade() -> None:
    with op.batch_alter_table("table_name", schema=None) as batch_op:
        batch_op.create_index('ix_table_column', ['column_name'])
```

### Foreign Key Constraints
```python
def upgrade() -> None:
    with op.batch_alter_table("table_name", schema=None) as batch_op:
        batch_op.create_foreign_key('fk_table_ref', 'ref_table', ['local_col'], ['ref_col'])
```

## Configuration Requirements

### env.py Setup
```python
import sqlmodel
from app.models import *  # Import all models
target_metadata = sqlmodel.SQLModel.metadata

# Enable batch operations for autogenerate
context.configure(
    render_as_batch=True  # Required for SQLite compatibility
)
```

## Best Practices

1. **Always use autogenerate**: Never write migrations manually
2. **Batch operations mandatory**: Required for SQLite compatibility
3. **Include SQLModel import**: Add to every migration file
4. **Review before applying**: Check generated migrations for accuracy
5. **Test migrations**: Run upgrade/downgrade locally first
6. **Descriptive messages**: Use clear, specific migration descriptions

## Common Patterns

### Safe Column Addition
```python
def upgrade() -> None:
    with op.batch_alter_table("games", schema=None) as batch_op:
        batch_op.add_column(sa.Column('rating', sa.Float(), nullable=True))

def downgrade() -> None:
    with op.batch_alter_table("games", schema=None) as batch_op:
        batch_op.drop_column('rating')
```

### Index Management
```python
def upgrade() -> None:
    with op.batch_alter_table("games", schema=None) as batch_op:
        batch_op.create_index('ix_games_title', ['title'])

def downgrade() -> None:
    with op.batch_alter_table("games", schema=None) as batch_op:
        batch_op.drop_index('ix_games_title')
```

## Troubleshooting

- **SQLite errors**: Always use batch operations
- **Import errors**: Ensure SQLModel is imported
- **Constraint issues**: Use named constraints for better SQLite support
- **Type changes**: Specify existing_type when altering columns

## Migration Checklist

- [ ] Used `--autogenerate` flag
- [ ] Added `import sqlmodel` to migration
- [ ] Wrapped all operations in `batch_alter_table`
- [ ] Tested upgrade and downgrade locally
- [ ] Used descriptive migration message
# SQLAlchemy Mapped[] Annotations and SQLModel Incompatibility

## TL;DR

**Do NOT use `Mapped[]` annotations with SQLModel.** They break Pydantic validation and cause pytest to fail with `PydanticSchemaGenerationError`.

## The Problem

SQLAlchemy 2.0 introduced `Mapped[]` type annotations for improved type checking:

```python
from sqlalchemy.orm import Mapped

class User(SQLModel, table=True):
    id: Mapped[str] = Field(primary_key=True)  # ❌ BREAKS SQLModel!
    username: Mapped[str] = Field(...)          # ❌ BREAKS SQLModel!
```

This looks appealing because type checkers like `pyrefly` and `mypy` can better understand SQLAlchemy column methods (`.is_not()`, `.ilike()`, `.label()`, etc.).

However, **this breaks SQLModel at runtime** with the following error:

```
pydantic.errors.PydanticSchemaGenerationError: Unable to generate pydantic-core
schema for sqlalchemy.orm.base.Mapped[str]. Set `arbitrary_types_allowed=True`
in the model_config to ignore this error or implement `__get_pydantic_core_schema__`
on your type to fully support it.
```

## Why It Breaks

1. **SQLModel is built on Pydantic**, which performs runtime validation and schema generation
2. **Pydantic doesn't understand `Mapped[]`** - it's a SQLAlchemy-specific type wrapper
3. When Pydantic tries to generate validation schemas for model fields, it encounters `Mapped[T]` and doesn't know how to handle it
4. This causes model classes to fail instantiation, breaking pytest and any code that tries to import the models

## The Trade-off

### With `Mapped[]` annotations:
- ✅ Better type checking with pyrefly/mypy (~55 fewer errors)
- ✅ Type checkers understand SQLAlchemy column methods
- ❌ **Breaks pytest - tests cannot run**
- ❌ **Breaks runtime model instantiation**
- ❌ Cannot use Pydantic validation features

### Without `Mapped[]` annotations (Standard SQLModel):
- ✅ **Pytest works - all 1011 tests pass**
- ✅ **Models work at runtime**
- ✅ Full Pydantic validation support
- ✅ SQLModel's designed approach
- ⚠️  Type checkers have more errors (~55 more)

## Correct Approach for SQLModel

Use standard SQLModel syntax **without** `Mapped[]`:

```python
from sqlmodel import SQLModel, Field

class User(SQLModel, table=True):
    """User model for authentication."""

    __tablename__ = "users"

    # ✅ CORRECT: Standard SQLModel field definitions
    id: str = Field(primary_key=True)
    username: str = Field(unique=True, index=True)
    is_active: bool = Field(default=True)
```

SQLModel handles the SQLAlchemy mapping internally without requiring explicit `Mapped[]` annotations.

## Attempted Solutions That Don't Work

### Option 1: `arbitrary_types_allowed` Config
```python
class User(SQLModel, table=True):
    model_config = {"arbitrary_types_allowed": True}

    id: Mapped[str] = Field(...)  # Still broken
```

**Result**: May allow model instantiation, but breaks Pydantic validation. Not recommended and untested with SQLModel.

### Option 2: TYPE_CHECKING Blocks
```python
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from sqlalchemy.orm import Mapped
    id: Mapped[str]
else:
    id: str = Field(primary_key=True)
```

**Result**: Requires duplicating every field definition. Extremely verbose and error-prone. Not worth the complexity.

## When Will This Be Fixed?

This is a known issue in the SQLModel/Pydantic ecosystem:
- **GitHub Issue**: [fastapi/sqlmodel#1016](https://github.com/fastapi/sqlmodel/discussions/1016)
- **Root Cause**: Pydantic v2's stricter type validation doesn't support SQLAlchemy's `Mapped[]` wrapper
- **Status**: No official solution as of SQLModel 0.0.24 (2025)

SQLModel would need to add special handling for `Mapped[]` types or Pydantic would need to add native support.

## Recommendations

1. **Use standard SQLModel syntax** without `Mapped[]` annotations
2. **Accept the pyrefly/mypy type errors** as a necessary trade-off
3. **Wait for official SQLModel support** before attempting `Mapped[]` again
4. If type checking is critical, consider:
   - Using SQLAlchemy ORM directly (without SQLModel)
   - Using SQLAlchemy 2.0's new `MappedAsDataclass` approach
   - Contributing to SQLModel to add `Mapped[]` support

## Historical Context

- **Commit b29c601** (2025-10-24): Added `Mapped[]` to 4 models (game, darkadia_game, user_game, platform)
- **Result**: pyrefly errors reduced from 341 to 286 (16% improvement)
- **Problem**: Broke all pytest tests with `PydanticSchemaGenerationError`
- **Resolution**: Reverted all `Mapped[]` annotations to restore pytest functionality

## Testing Impact

- **Before revert**: 0 tests could run (collection error)
- **After revert**: All 1011 tests pass ✅

## Conclusion

While `Mapped[]` annotations provide better static type checking, they are **fundamentally incompatible** with SQLModel's Pydantic-based approach. The ability to run tests and have working runtime validation is far more important than reducing type checker errors.

**Always prioritize working code over type checker satisfaction.**

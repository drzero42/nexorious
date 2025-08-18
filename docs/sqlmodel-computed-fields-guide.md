# SQLModel Computed Fields Reference Guide

## The Problem

When working with SQLModel, you may need fields that are computed dynamically (like counts, calculated values, derived properties) that should **NOT** be stored in the database but should be available in API responses. This creates a conflict between database operations and API serialization.

**Common scenario:** You have a `Tag` model and want to include a `game_count` field that shows how many games use that tag, but this count should be calculated dynamically, not stored in the database.

## Critical Rule: SQLModel Always Includes Defined Fields in SQL

**Any field defined on a SQLModel `table=True` class will ALWAYS be included in SQL queries, regardless of Pydantic parameters.**

This is by design - SQLModel generates SELECT statements for ALL defined fields on the model class, even if they don't exist in the actual database table.

## What DOESN'T Work (Common Mistakes)

### ❌ These approaches will FAIL:

```python
class Tag(SQLModel, table=True):
    id: str = Field(primary_key=True)
    name: str
    
    # WRONG - Still included in SQL SELECT statements
    game_count: Optional[int] = Field(default=None, exclude=True)
    
    # WRONG - sa_column=None is not a valid parameter
    game_count: Optional[int] = Field(default=None, sa_column=None)
    
    # WRONG - Still generates SQL for the field
    @computed_field
    @property
    def game_count(self) -> Optional[int]:
        return self._game_count
```

**Result:** `sqlite3.OperationalError: no such column: tags.game_count`

### Why These Don't Work:

- `exclude=True` - Only affects Pydantic serialization (JSON output), not SQL generation
- `sa_column=None` - Not a valid SQLAlchemy/SQLModel parameter, gets ignored
- `@computed_field` - Pydantic v2 computed fields are still considered model fields for SQL purposes
- Dynamic `setattr()` alone - Field still exists in model definition, so SQL includes it

## What DOES Work (Proven Solutions)

### ✅ Solution 1: PrivateAttr() (Recommended for Simple Cases)

**Best for:** Single computed field, minimal code changes required

```python
from pydantic import PrivateAttr

class Tag(SQLModel, table=True):
    # Database fields only
    id: str = Field(primary_key=True)
    name: str = Field(max_length=100)
    color: str = Field(default="#6B7280")
    # ... other DB fields
    
    # Computed field - completely excluded from SQL
    _game_count: Optional[int] = PrivateAttr(default=None)
    
    @property
    def game_count(self) -> Optional[int]:
        return self._game_count

# Service layer usage:
def get_tags_with_counts(user_id: str) -> List[Tag]:
    tags = session.exec(select(Tag).where(Tag.user_id == user_id)).all()
    
    for tag in tags:
        # Set the private attribute
        tag._game_count = calculate_game_count(tag.id)
    
    return tags

# API usage:
tag = get_tag_by_id("some-id")
result = tag.game_count  # Access via property
```

### ✅ Solution 2: Model Inheritance (Recommended for Complex Cases)

**Best for:** Multiple computed fields, clean architecture, team projects

```python
# Base model with shared fields
class TagBase(SQLModel):
    name: str = Field(max_length=100)
    color: str = Field(default="#6B7280")
    description: Optional[str] = None
    created_at: datetime
    updated_at: datetime

# Database model - ONLY persisted fields
class Tag(TagBase, table=True):
    __tablename__ = "tags"
    
    id: str = Field(primary_key=True)
    user_id: str = Field(foreign_key="users.id")
    # No computed fields here!

# API response model - includes computed fields
class TagRead(TagBase):
    id: str
    user_id: str
    game_count: Optional[int] = None  # Safe - not a table model
    last_used_date: Optional[datetime] = None  # Another computed field

# Service layer:
def get_tags_with_counts(user_id: str) -> List[TagRead]:
    # Query database model
    tags = session.exec(select(Tag).where(Tag.user_id == user_id)).all()
    
    # Batch calculate counts for efficiency
    tag_ids = [tag.id for tag in tags]
    counts = get_game_counts_batch(tag_ids)
    
    # Convert to response model with computed fields
    return [
        TagRead(
            **tag.model_dump(),
            game_count=counts.get(tag.id, 0),
            last_used_date=get_last_used_date(tag.id)
        )
        for tag in tags
    ]
```

### ✅ Solution 3: Plain Property (For Dynamic Assignment)

**Best for:** Existing codebases using setattr() patterns, quick fixes

```python
class Tag(SQLModel, table=True):
    # Database fields only
    id: str = Field(primary_key=True)
    name: str = Field(max_length=100)
    
    @property
    def game_count(self) -> Optional[int]:
        # Access dynamically set attribute
        return getattr(self, '_game_count', None)

# Service layer (existing setattr pattern):
def add_game_counts(tags: List[Tag]) -> List[Tag]:
    for tag in tags:
        count = calculate_game_count(tag.id)
        setattr(tag, '_game_count', count)
    return tags
```

## Decision Matrix

| Use Case | Recommended Solution | Pros | Cons |
|----------|---------------------|------|------|
| Simple computed field, minimal refactoring | **PrivateAttr()** | - Keeps changes localized<br>- Type-safe property access | - Requires understanding of Pydantic private attrs |
| Multiple computed fields, clean architecture | **Model Inheritance** | - Best separation of concerns<br>- Clear API contracts<br>- Team-friendly | - More files to maintain<br>- Requires refactoring |
| Existing codebase with setattr() pattern | **Plain Property** | - Minimal disruption<br>- Quick fix | - Less type safety<br>- Relies on getattr() |
| Complex calculations, performance critical | **Model Inheritance + batch queries** | - Most efficient<br>- Scalable | - Most complex implementation |

## Performance Considerations

### ❌ Avoid N+1 Queries:
```python
# BAD - queries database for each tag
def add_game_counts_slow(tags: List[Tag]) -> List[Tag]:
    for tag in tags:
        # This hits the database once per tag!
        tag._game_count = session.exec(
            select(func.count(UserGameTag.id)).where(UserGameTag.tag_id == tag.id)
        ).one()
    return tags
```

### ✅ Use Batch Queries:
```python
# GOOD - single query for all counts
def add_game_counts_fast(tags: List[Tag]) -> List[Tag]:
    tag_ids = [tag.id for tag in tags]
    
    # Single JOIN query to get all counts
    counts_query = (
        select(UserGameTag.tag_id, func.count(UserGameTag.id).label('count'))
        .where(UserGameTag.tag_id.in_(tag_ids))
        .group_by(UserGameTag.tag_id)
    )
    
    counts_result = session.exec(counts_query).all()
    counts_dict = {row.tag_id: row.count for row in counts_result}
    
    # Apply counts to tags
    for tag in tags:
        tag._game_count = counts_dict.get(tag.id, 0)
    
    return tags
```

## Key Principles

1. **Never define computed fields on table=True models** - They will always be included in SQL
2. **Use PrivateAttr() for fields that shouldn't exist in SQL** - Completely excludes them from database operations
3. **Use model inheritance for clean API vs DB separation** - Best practice for complex applications
4. **Always batch compute expensive calculated fields** - Avoid N+1 query problems
5. **Test SQL generation after any field changes** - Verify no unexpected columns in queries
6. **Document computed field calculations** - Make it clear how values are derived

## Testing Computed Fields

### Verify Field is Excluded from SQL:
```python
def test_computed_field_not_in_sql():
    """Ensure game_count doesn't cause SQL errors."""
    with Session(engine) as session:
        # This should NOT include game_count in SELECT
        tags = session.exec(select(Tag)).all()
        assert len(tags) >= 0  # Should not raise OperationalError

def test_computed_field_accessible():
    """Ensure computed field can be accessed after calculation."""
    tag = Tag(name="Test Tag")
    tag._game_count = 5  # Simulate service layer setting this
    assert tag.game_count == 5
```

### Test Batch Performance:
```python
def test_batch_game_count_calculation(benchmark):
    """Ensure batch calculation is performant."""
    # Create test data
    tags = [Tag(name=f"Tag {i}") for i in range(100)]
    
    # Benchmark the batch calculation
    result = benchmark(add_game_counts_fast, tags)
    assert len(result) == 100
```

## Migration Strategy

When fixing existing computed field issues:

### Step 1: Remove Problematic Field Definition
```python
# BEFORE (causing SQL errors)
class Tag(SQLModel, table=True):
    name: str
    game_count: Optional[int] = Field(default=None, exclude=True)  # REMOVE THIS

# AFTER (clean database model)
class Tag(SQLModel, table=True):
    name: str
    # No computed fields here!
```

### Step 2: Add Proper Computed Field
```python
# Option A: PrivateAttr
class Tag(SQLModel, table=True):
    name: str
    _game_count: Optional[int] = PrivateAttr(default=None)
    
    @property
    def game_count(self) -> Optional[int]:
        return self._game_count

# Option B: Model inheritance
class TagRead(TagBase):
    game_count: Optional[int] = None
```

### Step 3: Update Service Layer
```python
# Update methods to set computed values properly
def get_user_tags(user_id: str) -> List[Tag]:
    tags = session.exec(select(Tag).where(Tag.user_id == user_id)).all()
    
    # Add computed values
    for tag in tags:
        tag._game_count = calculate_game_count(tag.id)  # PrivateAttr approach
    
    return tags
```

### Step 4: Test and Verify
```python
# Ensure no SQL errors and correct API responses
def test_migration_success():
    tags = get_user_tags("test-user-id")
    assert len(tags) > 0
    assert all(tag.game_count is not None for tag in tags)
```

## Common Pitfalls to Avoid

### 1. Mixing Database and API Concerns
```python
# DON'T DO THIS - computed field on table model
class Tag(SQLModel, table=True):
    name: str
    game_count: int  # Will cause SQL errors!
```

### 2. Forgetting to Set Computed Values
```python
# DON'T DO THIS - returning tags without computed values
def get_tags(user_id: str) -> List[Tag]:
    return session.exec(select(Tag).where(Tag.user_id == user_id)).all()
    # game_count will be None for all tags!
```

### 3. N+1 Query Problems
```python
# DON'T DO THIS - individual queries for each tag
for tag in tags:
    tag._game_count = get_single_game_count(tag.id)  # Database hit per tag
```

## Real-World Example: Tag Management System

Here's a complete example of how to implement computed fields properly in a tag management system:

```python
from pydantic import PrivateAttr
from sqlmodel import SQLModel, Field, Session, select, func
from typing import List, Optional
from datetime import datetime

# Clean database model - no computed fields
class Tag(SQLModel, table=True):
    __tablename__ = "tags"
    
    id: str = Field(primary_key=True)
    user_id: str = Field(foreign_key="users.id")
    name: str = Field(max_length=100)
    color: str = Field(default="#6B7280")
    created_at: datetime = Field(default_factory=datetime.utcnow)
    
    # Computed field using PrivateAttr
    _game_count: Optional[int] = PrivateAttr(default=None)
    
    @property
    def game_count(self) -> Optional[int]:
        return self._game_count

# Service layer with batch optimization
class TagService:
    def __init__(self, session: Session):
        self.session = session
    
    def get_user_tags_with_counts(self, user_id: str) -> List[Tag]:
        # Get tags from database
        tags = self.session.exec(
            select(Tag).where(Tag.user_id == user_id)
        ).all()
        
        # Batch calculate game counts
        self._add_game_counts_batch(tags)
        
        return tags
    
    def _add_game_counts_batch(self, tags: List[Tag]) -> None:
        if not tags:
            return
        
        tag_ids = [tag.id for tag in tags]
        
        # Single query to get all counts
        counts_query = (
            select(UserGameTag.tag_id, func.count().label('count'))
            .where(UserGameTag.tag_id.in_(tag_ids))
            .group_by(UserGameTag.tag_id)
        )
        
        counts_result = self.session.exec(counts_query).all()
        counts_dict = {row.tag_id: row.count for row in counts_result}
        
        # Set computed values
        for tag in tags:
            tag._game_count = counts_dict.get(tag.id, 0)

# API endpoint
@router.get("/tags", response_model=List[TagResponse])
def get_tags(
    user: User = Depends(get_current_user),
    session: Session = Depends(get_session)
):
    service = TagService(session)
    tags = service.get_user_tags_with_counts(user.id)
    
    # Convert to response models
    return [
        TagResponse(
            id=tag.id,
            name=tag.name,
            color=tag.color,
            game_count=tag.game_count,  # Computed field available
            created_at=tag.created_at
        )
        for tag in tags
    ]
```

This example demonstrates the complete pattern: clean database model, proper computed field handling, batch optimization, and API integration.

## Summary

- **Use PrivateAttr()** for simple computed fields that need to be excluded from SQL
- **Use model inheritance** for complex applications with multiple computed fields  
- **Always batch expensive calculations** to avoid performance issues
- **Test SQL generation** to ensure computed fields don't cause database errors
- **Document your computed field logic** so the team understands the calculations

Following these patterns will help you avoid the common SQLModel computed field pitfalls and build maintainable, performant applications.
# Platform & Storefront Slug-Based ID Refactor - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace UUID primary keys with slug-based primary keys for platforms and storefronts, rename `platform_id`/`storefront_id` columns to `platform`/`storefront`.

**Architecture:** Remove UUID `id` fields from Platform/Storefront models, make `name` (slug) the primary key. Update all foreign key references and API routes to use slugs directly.

**Tech Stack:** Python/FastAPI, SQLModel, Alembic, TypeScript/Next.js, Vitest

---

## Task 1: Delete Existing Migrations

**Files:**
- Delete: `backend/app/alembic/versions/*.py` (all migration files)

**Step 1: Delete all migration files**

```bash
rm backend/app/alembic/versions/*.py
```

**Step 2: Verify deletion**

Run: `ls backend/app/alembic/versions/`
Expected: Empty directory (only `__pycache__` if present)

**Step 3: Commit**

```bash
git add -A backend/app/alembic/versions/
git commit -m "chore: remove all migrations for fresh schema"
```

---

## Task 2: Update Storefront Model

**Files:**
- Modify: `backend/app/models/platform.py`

**Step 1: Update Storefront class**

Replace the Storefront class with:

```python
class Storefront(SQLModel, table=True):
    """Storefront model for digital game stores."""

    __tablename__ = "storefronts"  # type: ignore[assignment]

    name: str = Field(primary_key=True, max_length=100)
    display_name: str = Field(max_length=100)
    icon_url: Optional[str] = Field(default=None, max_length=500)
    base_url: Optional[str] = Field(default=None, max_length=500)
    is_active: bool = Field(default=True)
    source: str = Field(default="custom", max_length=20, description="Source of the storefront: 'official' or 'custom'")
    version_added: Optional[str] = Field(default=None, max_length=10, description="Version when this official storefront was added")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user_game_platforms: List["UserGamePlatform"] = Relationship(back_populates="storefront")
    default_for_platforms: List["Platform"] = Relationship(
        back_populates="default_storefront",
        sa_relationship_kwargs={"foreign_keys": "[Platform.default_storefront]"}
    )
```

**Step 2: Run type check to verify syntax**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/models/platform.py`
Expected: May show errors (expected at this stage, Platform not yet updated)

---

## Task 3: Update Platform Model

**Files:**
- Modify: `backend/app/models/platform.py`

**Step 1: Update Platform class**

Replace the Platform class with:

```python
class Platform(SQLModel, table=True):
    """Platform model for gaming platforms (Windows, PlayStation, Xbox, Nintendo Switch, etc.)."""

    __tablename__ = "platforms"  # type: ignore[assignment]

    name: str = Field(primary_key=True, max_length=100)
    display_name: str = Field(max_length=100)
    icon_url: Optional[str] = Field(default=None, max_length=500)
    default_storefront: Optional[str] = Field(default=None, foreign_key="storefronts.name", description="Default storefront for this platform")
    is_active: bool = Field(default=True)
    source: str = Field(default="custom", max_length=20, description="Source of the platform: 'official' or 'custom'")
    version_added: Optional[str] = Field(default=None, max_length=10, description="Version when this official platform was added")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user_game_platforms: List["UserGamePlatform"] = Relationship(back_populates="platform")
    default_storefront_rel: Optional["Storefront"] = Relationship(
        back_populates="default_for_platforms",
        sa_relationship_kwargs={"foreign_keys": "[Platform.default_storefront]"}
    )
```

**Step 2: Remove uuid import if no longer needed**

Check if `uuid` is still used in the file. If not, remove the import.

---

## Task 4: Update PlatformStorefront Junction Model

**Files:**
- Modify: `backend/app/models/platform.py`

**Step 1: Update PlatformStorefront class**

Replace the PlatformStorefront class with:

```python
class PlatformStorefront(SQLModel, table=True):
    """Junction table for many-to-many platform-storefront associations."""

    __tablename__ = "platform_storefronts"  # type: ignore[assignment]

    platform: str = Field(foreign_key="platforms.name", primary_key=True)
    storefront: str = Field(foreign_key="storefronts.name", primary_key=True)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/models/platform.py`
Expected: Pass or show only unrelated errors

**Step 3: Commit**

```bash
git add backend/app/models/platform.py
git commit -m "refactor: update Platform/Storefront models to use slug PK"
```

---

## Task 5: Update UserGamePlatform Model

**Files:**
- Modify: `backend/app/models/user_game.py`

**Step 1: Rename platform_id to platform, storefront_id to storefront**

Find and replace in UserGamePlatform class:
- `platform_id` → `platform`
- `storefront_id` → `storefront`

The updated fields should be:

```python
platform: Optional[str] = Field(default=None, foreign_key="platforms.name", index=True)
storefront: Optional[str] = Field(default=None, foreign_key="storefronts.name")
```

**Step 2: Update the unique constraint**

Change the constraint from:
```python
UniqueConstraint("user_game_id", "platform_id", "storefront_id",
                name="uq_user_game_platform_storefront"),
```

To:
```python
UniqueConstraint("user_game_id", "platform", "storefront",
                name="uq_user_game_platform_storefront"),
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/models/user_game.py`
Expected: Pass

**Step 4: Commit**

```bash
git add backend/app/models/user_game.py
git commit -m "refactor: rename platform_id/storefront_id to platform/storefront"
```

---

## Task 6: Update Platform Schemas

**Files:**
- Modify: `backend/app/schemas/platform.py`

**Step 1: Update PlatformResponse**

Remove `id` field, rename `default_storefront_id` to `default_storefront`:

```python
class PlatformResponse(BaseModel, TimestampMixin):
    """Response schema for platform data."""
    name: str
    display_name: str
    icon_url: Optional[str]
    is_active: bool
    source: str = Field(description="Source of the platform: 'official' or 'custom'")
    version_added: Optional[str] = Field(None, description="Version when this official platform was added")
    default_storefront: Optional[str] = Field(None, description="Default storefront for this platform")
    storefronts: List['StorefrontResponse'] = Field(default_factory=list, description="Associated storefronts for this platform")

    model_config = ConfigDict(from_attributes=True)
```

**Step 2: Update StorefrontResponse**

Remove `id` field:

```python
class StorefrontResponse(BaseModel, TimestampMixin):
    """Response schema for storefront data."""
    name: str
    display_name: str
    icon_url: Optional[str]
    base_url: Optional[str]
    is_active: bool
    source: str = Field(description="Source of the storefront: 'official' or 'custom'")
    version_added: Optional[str] = Field(None, description="Version when this official storefront was added")

    model_config = ConfigDict(from_attributes=True)
```

**Step 3: Update PlatformCreateRequest**

Rename `default_storefront_id` to `default_storefront`:

```python
class PlatformCreateRequest(BaseModel):
    """Request schema for creating a platform."""
    name: str = Field(..., min_length=1, max_length=100, description="Platform name (unique identifier)")
    display_name: str = Field(..., min_length=1, max_length=100, description="Display name for platform")
    icon_url: Optional[str] = Field(None, description="Platform icon URL (full URL or relative path starting with /static/)")
    is_active: Optional[bool] = Field(True, description="Whether platform is active")
    default_storefront: Optional[str] = Field(None, description="Default storefront for this platform")
    # ... keep existing validator
```

**Step 4: Update PlatformUpdateRequest**

Rename `default_storefront_id` to `default_storefront`:

```python
class PlatformUpdateRequest(BaseModel):
    """Request schema for updating a platform."""
    display_name: Optional[str] = Field(None, max_length=100, description="Display name for platform")
    icon_url: Optional[str] = Field(None, description="Platform icon URL (full URL or relative path starting with /static/)")
    is_active: Optional[bool] = Field(None, description="Whether platform is active")
    default_storefront: Optional[str] = Field(None, description="Default storefront for this platform")
    # ... keep existing validator
```

**Step 5: Update PlatformUsageStats**

Rename `platform_id` to `platform`, remove `platform_name`:

```python
class PlatformUsageStats(BaseModel):
    """Usage statistics for a platform."""
    platform: str
    platform_display_name: str
    usage_count: int = Field(description="Number of users using this platform")
```

**Step 6: Update StorefrontUsageStats**

Rename `storefront_id` to `storefront`, remove `storefront_name`:

```python
class StorefrontUsageStats(BaseModel):
    """Usage statistics for a storefront."""
    storefront: str
    storefront_display_name: str
    usage_count: int = Field(description="Number of users using this storefront")
```

**Step 7: Update PlatformDefaultMapping**

Rename `platform_id` to `platform`, remove `platform_name`:

```python
class PlatformDefaultMapping(BaseModel):
    """Response schema for platform default storefront mapping."""
    platform: str
    platform_display_name: str
    default_storefront: Optional['StorefrontResponse'] = Field(None, description="Default storefront for this platform")

    model_config = ConfigDict(from_attributes=True)
```

**Step 8: Update UpdatePlatformDefaultRequest**

Rename `storefront_id` to `storefront`:

```python
class UpdatePlatformDefaultRequest(BaseModel):
    """Request schema for updating platform default storefront."""
    storefront: Optional[str] = Field(None, description="Storefront to set as default, or null to remove default")
```

**Step 9: Update PlatformStorefrontsResponse**

Rename `platform_id` to `platform`, remove `platform_name`:

```python
class PlatformStorefrontsResponse(BaseModel):
    """Response schema for platform storefronts list."""
    platform: str
    platform_display_name: str
    storefronts: List[StorefrontResponse]
    total_storefronts: int = Field(description="Total number of associated storefronts")

    model_config = ConfigDict(from_attributes=True)
```

**Step 10: Update PlatformStorefrontAssociationResponse**

Rename fields:

```python
class PlatformStorefrontAssociationResponse(BaseModel):
    """Response schema for platform-storefront association operations."""
    platform: str
    platform_display_name: str
    storefront: str
    storefront_display_name: str
    message: str = Field(description="Operation result message")

    model_config = ConfigDict(from_attributes=True)
```

**Step 11: Update all resolution schemas**

Update all `*_id` fields to remove `_id` suffix throughout the file for:
- PlatformSuggestion: `platform_id` → `platform`
- StorefrontSuggestion: `storefront_id` → `storefront`
- PlatformResolutionData: `resolved_platform_id` → `resolved_platform`, `resolved_storefront_id` → `resolved_storefront`
- StorefrontResolutionData: `resolved_storefront_id` → `resolved_storefront`
- PlatformResolutionRequest: fields
- StorefrontSuggestionsRequest: `platform_id` → `platform`
- StorefrontSuggestionsResponse: `platform_id` → `platform`
- StorefrontResolutionRequest: `resolved_storefront_id` → `resolved_storefront`
- StorefrontCompatibilityRequest: `platform_id` → `platform`, `storefront_id` → `storefront`
- StorefrontCompatibilityResponse: same fields

**Step 12: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/schemas/platform.py`
Expected: Pass

**Step 13: Commit**

```bash
git add backend/app/schemas/platform.py
git commit -m "refactor: update platform schemas to use slug-based naming"
```

---

## Task 7: Update UserGame Schemas

**Files:**
- Modify: `backend/app/schemas/user_game.py`

**Step 1: Update UserGamePlatformCreateRequest**

Rename fields:

```python
class UserGamePlatformCreateRequest(BaseModel):
    """Request schema for adding platform association to user game."""
    platform: str = Field(..., description="Platform slug")
    storefront: Optional[str] = Field(None, description="Storefront slug")
    store_game_id: Optional[str] = Field(None, max_length=200, description="Game ID in store")
    store_url: Optional[HttpUrl] = Field(None, description="Store URL for game")
    is_available: bool = Field(default=True, description="Whether the game is available on this platform")
```

**Step 2: Update UserGamePlatformResponse**

Rename fields:

```python
class UserGamePlatformResponse(BaseModel, TimestampMixin):
    """Response schema for user game platform association."""
    id: str
    platform: Optional[str]
    storefront: Optional[str]
    platform_details: Optional[PlatformResponse] = Field(None, alias="platform_obj")
    storefront_details: Optional[StorefrontResponse] = Field(None, alias="storefront_obj")
    store_game_id: Optional[str]
    store_url: Optional[str]
    is_available: bool
    original_platform_name: Optional[str]

    model_config = ConfigDict(from_attributes=True, populate_by_name=True)
```

Note: Changed `platform` and `storefront` relationship fields to `platform_details` and `storefront_details` to avoid name collision with the slug fields.

**Step 3: Update UserGameListRequest**

Rename filter fields:

```python
class UserGameListRequest(BaseModel):
    """Request schema for filtering user's game collection."""
    play_status: Optional[PlayStatus] = Field(None, description="Filter by play status")
    ownership_status: Optional[OwnershipStatus] = Field(None, description="Filter by ownership status")
    is_loved: Optional[bool] = Field(None, description="Filter by loved status")
    platform: Optional[str] = Field(None, description="Filter by platform")
    storefront: Optional[str] = Field(None, description="Filter by storefront")
    rating_min: Optional[float] = Field(None, ge=1, le=5, description="Minimum rating filter")
    rating_max: Optional[float] = Field(None, ge=1, le=5, description="Maximum rating filter")
    has_notes: Optional[bool] = Field(None, description="Filter by presence of notes")
```

**Step 4: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/schemas/user_game.py`
Expected: Pass

**Step 5: Commit**

```bash
git add backend/app/schemas/user_game.py
git commit -m "refactor: update user_game schemas to use slug-based naming"
```

---

## Task 8: Update Platform API Routes

**Files:**
- Modify: `backend/app/api/platforms.py`

**Step 1: Update route path parameters**

Change all `{platform_id}` to `{platform}` and `{storefront_id}` to `{storefront}` in route decorators and function signatures.

**Step 2: Update get_platform function**

```python
@router.get("/{platform}", response_model=PlatformResponse)
async def get_platform(
    platform: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get a specific platform by name."""

    platform_obj = session.get(Platform, platform)
    if not platform_obj:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )

    return platform_obj
```

**Step 3: Update all other platform routes**

Update similarly:
- `get_platform_storefronts` - parameter and queries
- `create_platform_storefront_association` - parameters and queries
- `delete_platform_storefront_association` - parameters and queries
- `update_platform` - parameter and queries
- `delete_platform` - parameter and queries
- `upload_platform_logo` - parameter
- `delete_platform_logo` - parameter
- `list_platform_logos` - parameter
- `get_platform_default_storefront` - parameter
- `update_platform_default_storefront` - parameter

**Step 4: Update storefront routes**

Change `{storefront_id}` to `{storefront}`:
- `get_storefront`
- `update_storefront`
- `delete_storefront`
- `upload_storefront_logo`
- `delete_storefront_logo`
- `list_storefront_logos`

**Step 5: Update list_platforms to use new field names**

Update the query that joins PlatformStorefront to use `platform` and `storefront` instead of `platform_id` and `storefront_id`.

**Step 6: Update stats endpoints**

Update `get_platform_usage_stats` and `get_storefront_usage_stats` to use new column/field names.

**Step 7: Update response constructions**

All places that construct response objects need to use new field names.

**Step 8: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/api/platforms.py`
Expected: Pass

**Step 9: Commit**

```bash
git add backend/app/api/platforms.py
git commit -m "refactor: update platform API routes to use slug-based naming"
```

---

## Task 9: Update Seed Data and Seeder

**Files:**
- Modify: `backend/app/seed_data/platforms.py`
- Modify: `backend/app/seed_data/storefronts.py`
- Modify: `backend/app/seed_data/seeder.py`

**Step 1: Update seeder to not generate UUIDs**

In `seed_platforms` function, remove UUID generation:

```python
new_platform = Platform(
    name=platform_data["name"],
    display_name=platform_data["display_name"],
    icon_url=platform_data.get("icon_url"),
    default_storefront=default_storefront_name,  # Use name directly, not ID
    is_active=platform_data.get("is_active", True),
    source="official",
    version_added=version,
    created_at=datetime.now(timezone.utc),
    updated_at=datetime.now(timezone.utc)
)
```

**Step 2: Update seed_platforms to use slug FK**

Change the default storefront lookup to just use the name directly since that's now the FK:

```python
default_storefront_name = platform_data.get("default_storefront_name")
# ... later ...
default_storefront=default_storefront_name,
```

**Step 3: Update seed_storefronts similarly**

Remove UUID generation.

**Step 4: Update seed_platform_storefront_associations**

Use slug fields instead of IDs:

```python
new_association = PlatformStorefront(
    platform=platform.name,
    storefront=storefront.name,
    created_at=datetime.now(timezone.utc)
)
```

And update the query:
```python
existing_association = session.exec(
    select(PlatformStorefront).where(
        PlatformStorefront.platform == platform.name,
        PlatformStorefront.storefront == storefront.name
    )
).first()
```

**Step 5: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/seed_data/seeder.py`
Expected: Pass

**Step 6: Commit**

```bash
git add backend/app/seed_data/
git commit -m "refactor: update seeder to use slug-based references"
```

---

## Task 10: Update UserGame API Routes

**Files:**
- Modify: `backend/app/api/user_games.py`

**Step 1: Search for platform_id and storefront_id references**

Run: `grep -n "platform_id\|storefront_id" backend/app/api/user_games.py`

**Step 2: Update all references**

Replace `platform_id` with `platform` and `storefront_id` with `storefront` in:
- Query filters
- Request body field accesses
- Response constructions

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check app/api/user_games.py`
Expected: Pass

**Step 4: Commit**

```bash
git add backend/app/api/user_games.py
git commit -m "refactor: update user_games API to use slug-based naming"
```

---

## Task 11: Update Other Backend Files

**Files:**
- Search and update any other files referencing `platform_id` or `storefront_id`

**Step 1: Search for remaining references**

Run: `grep -rn "platform_id\|storefront_id\|default_storefront_id" backend/app/ --include="*.py" | grep -v "__pycache__" | grep -v ".pyc"`

**Step 2: Update each file found**

For each file, update the references appropriately.

**Step 3: Run full type check**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: Pass

**Step 4: Commit**

```bash
git add backend/app/
git commit -m "refactor: update remaining backend files for slug-based naming"
```

---

## Task 12: Update Darkadia Converter Script

**Files:**
- Modify: `backend/scripts/darkadia_to_nexorious.py`

**Step 1: Update generate_nexorious_json function**

Change the platform entry generation to use new field names:

```python
platforms.append(
    {
        "platform": platform_name,
        "storefront": storefront_name,
        "store_game_id": None,
        "store_url": None,
        "is_available": True,
    }
)
```

**Step 2: Remove the _id fields from output**

The `platform_id` and `storefront_id` fields should be removed entirely since we now use `platform` and `storefront`.

**Step 3: Test the script syntax**

Run: `cd /home/abo/workspace/home/nexorious/backend && python -m py_compile scripts/darkadia_to_nexorious.py`
Expected: No errors

**Step 4: Commit**

```bash
git add backend/scripts/darkadia_to_nexorious.py
git commit -m "refactor: update darkadia converter for slug-based naming"
```

---

## Task 13: Create Fresh Migration

**Files:**
- Create: `backend/app/alembic/versions/xxxx_initial_schema.py` (auto-generated)

**Step 1: Reset database**

Run: `cd /home/abo/workspace/home/nexorious && podman-compose down -v`

**Step 2: Generate new migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic revision --autogenerate -m "initial_schema"`
Expected: Creates a new migration file

**Step 3: Review the migration**

Read the generated migration file and verify it:
- Creates `storefronts` table with `name` as PK
- Creates `platforms` table with `name` as PK and `default_storefront` FK
- Creates `platform_storefronts` with `platform` and `storefront` as composite PK
- Creates `user_game_platforms` with `platform` and `storefront` columns

**Step 4: Apply migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic upgrade head`
Expected: Migration applies successfully

**Step 5: Commit**

```bash
git add backend/app/alembic/versions/
git commit -m "feat: add fresh initial migration with slug-based PKs"
```

---

## Task 14: Run Backend Tests

**Files:**
- Modify: `backend/app/tests/test_integration_platforms.py`
- Modify: `backend/app/tests/integration_test_utils.py`

**Step 1: Update test fixtures and assertions**

In test files, update:
- `test_platform.id` → `test_platform.name`
- `platform_id` → `platform`
- `storefront_id` → `storefront`
- Remove assertions on `id` field in responses
- Update URL patterns in test requests

**Step 2: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_integration_platforms.py -v`
Expected: All tests pass (after updates)

**Step 3: Run full test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest`
Expected: All tests pass

**Step 4: Commit**

```bash
git add backend/app/tests/
git commit -m "test: update backend tests for slug-based naming"
```

---

## Task 15: Update Frontend Types

**Files:**
- Modify: `frontend/src/types/platform.ts`
- Modify: `frontend/src/types/game.ts`

**Step 1: Update Platform interface**

```typescript
export interface Platform {
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
  source: string;
  default_storefront?: string;
  storefronts?: Storefront[];
  created_at: string;
  updated_at: string;
}
```

**Step 2: Update Storefront interface**

```typescript
export interface Storefront {
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active: boolean;
  source: string;
  created_at: string;
  updated_at: string;
}
```

**Step 3: Update UserGamePlatform interface in game.ts**

```typescript
export interface UserGamePlatform {
  id: string;
  platform?: string;
  storefront?: string;
  platform_details?: Platform;
  storefront_details?: Storefront;
  store_game_id?: string;
  store_url?: string;
  is_available: boolean;
  original_platform_name?: string;
  created_at: string;
}
```

**Step 4: Update UserGameCreateRequest**

```typescript
export interface UserGameCreateRequest {
  game_id: GameId;
  ownership_status?: OwnershipStatus;
  play_status?: PlayStatus;
  platforms?: Array<{
    platform: string;
    storefront?: string;
  }>;
}
```

**Step 5: Update UserGameFilters**

```typescript
export interface UserGameFilters {
  q?: string;
  play_status?: PlayStatus;
  ownership_status?: OwnershipStatus;
  platform?: string;
  tag_id?: string;
  is_loved?: boolean;
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
  page?: number;
  per_page?: number;
}
```

**Step 6: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Type errors (components not yet updated)

**Step 7: Commit**

```bash
git add frontend/src/types/
git commit -m "refactor: update frontend types for slug-based naming"
```

---

## Task 16: Update Frontend API Client

**Files:**
- Modify: `frontend/src/api/platforms.ts`

**Step 1: Update API response interfaces**

Remove `id` fields from `PlatformApiResponse` and `StorefrontApiResponse`:

```typescript
interface PlatformApiResponse {
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
  source: string;
  default_storefront?: string;
  storefronts?: StorefrontApiResponse[];
  created_at: string;
  updated_at: string;
}

interface StorefrontApiResponse {
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active: boolean;
  source: string;
  created_at: string;
  updated_at: string;
}
```

**Step 2: Update transformation functions**

Remove `id` from transformations:

```typescript
function transformStorefront(apiStorefront: StorefrontApiResponse): Storefront {
  return {
    name: apiStorefront.name,
    display_name: apiStorefront.display_name,
    icon_url: apiStorefront.icon_url,
    base_url: apiStorefront.base_url,
    is_active: apiStorefront.is_active,
    source: apiStorefront.source,
    created_at: apiStorefront.created_at,
    updated_at: apiStorefront.updated_at,
  };
}

function transformPlatform(apiPlatform: PlatformApiResponse): Platform {
  return {
    name: apiPlatform.name,
    display_name: apiPlatform.display_name,
    icon_url: apiPlatform.icon_url,
    is_active: apiPlatform.is_active,
    source: apiPlatform.source,
    default_storefront: apiPlatform.default_storefront,
    storefronts: apiPlatform.storefronts?.map(transformStorefront),
    created_at: apiPlatform.created_at,
    updated_at: apiPlatform.updated_at,
  };
}
```

**Step 3: Update API functions to use name in URLs**

```typescript
export async function getPlatform(name: string): Promise<Platform> {
  const response = await api.get<PlatformApiResponse>(`/platforms/${name}`);
  return transformPlatform(response);
}

export async function getPlatformStorefronts(
  platform: string,
  activeOnly?: boolean
): Promise<Storefront[]> {
  const response = await api.get<{
    platform: string;
    platform_display_name: string;
    storefronts: StorefrontApiResponse[];
    total_storefronts: number;
  }>(`/platforms/${platform}/storefronts`, {
    params: { active_only: activeOnly ?? true },
  });

  return response.storefronts.map(transformStorefront);
}
```

**Step 4: Update CRUD operations**

Update parameter names and request body field names.

**Step 5: Update PlatformCreateData and PlatformUpdateData**

```typescript
export interface PlatformCreateData {
  name: string;
  display_name: string;
  icon_url?: string;
  is_active?: boolean;
  default_storefront?: string;
}

export interface PlatformUpdateData {
  display_name?: string;
  icon_url?: string | null;
  is_active?: boolean;
  default_storefront?: string | null;
}
```

**Step 6: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Errors in components (will fix next)

**Step 7: Commit**

```bash
git add frontend/src/api/platforms.ts
git commit -m "refactor: update frontend API client for slug-based naming"
```

---

## Task 17: Update Frontend Components

**Files:**
- Search and update all components using `platform.id`, `storefront.id`, `platform_id`, `storefront_id`

**Step 1: Search for references**

Run: `grep -rn "platform\.id\|storefront\.id\|platform_id\|storefront_id\|default_storefront_id" frontend/src/ --include="*.tsx" --include="*.ts" | grep -v node_modules | grep -v ".test."`

**Step 2: Update each component found**

Replace:
- `platform.id` → `platform.name`
- `storefront.id` → `storefront.name`
- `platform_id` → `platform`
- `storefront_id` → `storefront`
- `default_storefront_id` → `default_storefront`

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Pass

**Step 4: Commit**

```bash
git add frontend/src/
git commit -m "refactor: update frontend components for slug-based naming"
```

---

## Task 18: Update Frontend Tests

**Files:**
- Modify: `frontend/src/api/platforms.test.ts`
- Modify other test files as needed

**Step 1: Update mock data**

Remove `id` fields from mock platform and storefront objects:

```typescript
const mockPlatformApi = {
  name: 'pc',
  display_name: 'PC',
  icon_url: 'https://example.com/pc.png',
  is_active: true,
  source: 'official',
  default_storefront: 'steam',
  storefronts: [
    {
      name: 'steam',
      display_name: 'Steam',
      // ...
    },
  ],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};
```

**Step 2: Update test assertions**

Remove assertions on `id` field, update field name references.

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/
git commit -m "test: update frontend tests for slug-based naming"
```

---

## Task 19: Full Integration Test

**Step 1: Start all services**

Run: `cd /home/abo/workspace/home/nexorious && podman-compose up --build`

**Step 2: Seed the database**

Use the API or run the seed command to populate platforms and storefronts.

**Step 3: Verify API responses**

- `GET /api/platforms/` - should return platforms without `id` field
- `GET /api/platforms/pc-windows` - should work with slug
- `GET /api/platforms/pc-windows/storefronts` - should return storefronts

**Step 4: Test frontend**

Navigate to the app and verify platform/storefront selection works correctly.

**Step 5: Run full test suites**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest
cd /home/abo/workspace/home/nexorious/frontend && npm run test
```

Expected: All tests pass

---

## Task 20: Final Cleanup and Commit

**Step 1: Run linting**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run ruff check .
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

**Step 2: Fix any linting issues**

**Step 3: Create final commit if needed**

```bash
git add -A
git commit -m "chore: final cleanup for slug-based platform/storefront refactor"
```

---

## Summary of All Changes

| File | Changes |
|------|---------|
| `backend/app/models/platform.py` | Remove UUID `id`, make `name` PK, rename FK fields |
| `backend/app/models/user_game.py` | Rename `platform_id` → `platform`, `storefront_id` → `storefront` |
| `backend/app/schemas/platform.py` | Remove `id` fields, rename `*_id` → `*` |
| `backend/app/schemas/user_game.py` | Rename `platform_id` → `platform`, `storefront_id` → `storefront` |
| `backend/app/api/platforms.py` | Update route params and queries |
| `backend/app/api/user_games.py` | Update field references |
| `backend/app/seed_data/seeder.py` | Remove UUID generation, use slug FKs |
| `backend/scripts/darkadia_to_nexorious.py` | Update output field names |
| `backend/app/alembic/versions/*` | Delete all, create fresh migration |
| `backend/app/tests/*` | Update test fixtures and assertions |
| `frontend/src/types/platform.ts` | Remove `id` fields |
| `frontend/src/types/game.ts` | Rename `platform_id` → `platform`, etc. |
| `frontend/src/api/platforms.ts` | Update transformations and API calls |
| `frontend/src/**/*.tsx` | Update component references |
| `frontend/src/**/*.test.ts` | Update test mocks and assertions |

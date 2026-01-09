# Ownership Status Per Platform Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move `ownership_status` and `acquired_date` from `UserGame` to `UserGamePlatform` to support different ownership types per platform/storefront.

**Architecture:** Add ownership fields to `UserGamePlatform` model, migrate existing data, update schemas to reflect new locations, update frontend to show/edit ownership per platform entry.

**Tech Stack:** SQLModel/SQLAlchemy, Alembic migrations, Pydantic schemas, FastAPI endpoints, React/TypeScript frontend with TanStack Query.

---

## Task 1: Update UserGamePlatform Model

**Files:**
- Modify: [user_game.py](backend/app/models/user_game.py)

**Step 1: Add ownership fields to UserGamePlatform model**

In `backend/app/models/user_game.py`, add `ownership_status` and `acquired_date` to the `UserGamePlatform` class:

```python
class UserGamePlatform(SQLModel, table=True):
    """User game platform model for platform-specific ownership data."""

    __tablename__ = "user_game_platforms"  # type: ignore[assignment]

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_game_id: str = Field(foreign_key="user_games.id", index=True)
    platform: Optional[str] = Field(default=None, foreign_key="platforms.name", index=True)
    storefront: Optional[str] = Field(default=None, foreign_key="storefronts.name")
    store_game_id: Optional[str] = Field(default=None, max_length=200)
    store_url: Optional[str] = Field(default=None, max_length=500)
    is_available: bool = Field(default=True)
    hours_played: int = Field(default=0, ge=0, description="Hours played on this storefront")
    ownership_status: OwnershipStatus = Field(default=OwnershipStatus.OWNED)
    acquired_date: Optional[date] = Field(default=None)
    original_platform_name: Optional[str] = Field(default=None, max_length=200, description="Original platform name for unresolved platforms")
    original_storefront_name: Optional[str] = Field(default=None, max_length=200, description="Original storefront name for unresolved storefronts")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships (unchanged)
    user_game: UserGame = Relationship(back_populates="platforms")
    platform_rel: Optional["Platform"] = Relationship(back_populates="user_game_platforms")
    storefront_rel: Optional["Storefront"] = Relationship(back_populates="user_game_platforms")

    __table_args__ = (
        UniqueConstraint("user_game_id", "platform", "storefront", name="uq_user_game_platform_storefront"),
        {"extend_existing": True},
    )
```

**Step 2: Run type checker to verify model changes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: Pass with no errors related to model changes

---

## Task 2: Generate Alembic Migration

**Files:**
- Create: `backend/app/alembic/versions/<generated>_move_ownership_to_platform.py` (auto-generated)

**Step 1: Generate the migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic revision --autogenerate -m "move ownership status and acquired date to platform"`

Expected: New migration file created

**Step 2: Verify and enhance the migration with data migration logic**

The auto-generated migration will add columns but won't migrate data. Edit the generated file to add data migration:

```python
def upgrade() -> None:
    """Upgrade schema."""
    # Add new columns to user_game_platforms
    op.add_column('user_game_platforms', sa.Column('ownership_status', sa.Enum('OWNED', 'BORROWED', 'RENTED', 'SUBSCRIPTION', 'NO_LONGER_OWNED', name='ownershipstatus'), nullable=False, server_default='OWNED'))
    op.add_column('user_game_platforms', sa.Column('acquired_date', sa.Date(), nullable=True))

    # Copy ownership_status and acquired_date from user_games to all related user_game_platforms
    op.execute("""
        UPDATE user_game_platforms ugp
        SET ownership_status = ug.ownership_status,
            acquired_date = ug.acquired_date
        FROM user_games ug
        WHERE ugp.user_game_id = ug.id
    """)

    # Remove server default after data migration
    op.alter_column('user_game_platforms', 'ownership_status', server_default=None)

    # Drop columns from user_games
    op.drop_column('user_games', 'ownership_status')
    op.drop_column('user_games', 'acquired_date')


def downgrade() -> None:
    """Downgrade schema."""
    # Add columns back to user_games
    op.add_column('user_games', sa.Column('ownership_status', sa.Enum('OWNED', 'BORROWED', 'RENTED', 'SUBSCRIPTION', 'NO_LONGER_OWNED', name='ownershipstatus'), nullable=False, server_default='OWNED'))
    op.add_column('user_games', sa.Column('acquired_date', sa.Date(), nullable=True))

    # Copy back from first platform association (best effort for downgrade)
    op.execute("""
        UPDATE user_games ug
        SET ownership_status = (
            SELECT ugp.ownership_status
            FROM user_game_platforms ugp
            WHERE ugp.user_game_id = ug.id
            ORDER BY ugp.created_at ASC
            LIMIT 1
        ),
        acquired_date = (
            SELECT ugp.acquired_date
            FROM user_game_platforms ugp
            WHERE ugp.user_game_id = ug.id
            ORDER BY ugp.created_at ASC
            LIMIT 1
        )
    """)

    # Remove server default
    op.alter_column('user_games', 'ownership_status', server_default=None)

    # Drop columns from user_game_platforms
    op.drop_column('user_game_platforms', 'acquired_date')
    op.drop_column('user_game_platforms', 'ownership_status')
```

**Step 3: Run the migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic upgrade head`
Expected: Migration completes successfully

---

## Task 3: Remove Fields from UserGame Model

**Files:**
- Modify: [user_game.py](backend/app/models/user_game.py)

**Step 1: Remove ownership_status and acquired_date from UserGame**

Remove these two fields from the `UserGame` class in `backend/app/models/user_game.py`:

```python
# REMOVE these lines from UserGame class:
# ownership_status: OwnershipStatus = Field(default=OwnershipStatus.OWNED)
# acquired_date: Optional[date] = Field(default=None)
```

The UserGame class should now look like:

```python
class UserGame(SQLModel, table=True):
    """User game model linking users to games with ownership and progress data."""

    __tablename__ = "user_games"  # type: ignore[assignment]

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    game_id: int = Field(foreign_key="games.id", index=True)
    personal_rating: Optional[Decimal] = Field(default=None, max_digits=2, decimal_places=1)
    is_loved: bool = Field(default=False, index=True)
    play_status: PlayStatus = Field(default=PlayStatus.NOT_STARTED, index=True)
    hours_played: int = Field(default=0)
    personal_notes: Optional[str] = Field(default=None)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user: "User" = Relationship(back_populates="user_games")
    game: "Game" = Relationship(back_populates="user_games")
    platforms: List["UserGamePlatform"] = Relationship(back_populates="user_game", cascade_delete=True)
    tags: List["UserGameTag"] = Relationship(back_populates="user_game", cascade_delete=True)

    # Unique constraint
    __table_args__ = (
        UniqueConstraint("user_id", "game_id", name="uq_user_games_user_game"),
        {"extend_existing": True},
    )
```

**Step 2: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: Errors will appear for code referencing removed fields (this is expected - we'll fix in subsequent tasks)

---

## Task 4: Update Backend Schemas

**Files:**
- Modify: [user_game.py](backend/app/schemas/user_game.py)

**Step 1: Add ownership fields to UserGamePlatformCreateRequest**

```python
class UserGamePlatformCreateRequest(BaseModel):
    """Request schema for adding platform association to user game."""
    platform: str = Field(..., description="Platform slug")
    storefront: Optional[str] = Field(None, description="Storefront slug")
    store_game_id: Optional[str] = Field(None, max_length=200, description="Game ID in store")
    store_url: Optional[HttpUrl] = Field(None, description="Store URL for game")
    is_available: bool = Field(default=True, description="Whether the game is available on this platform")
    hours_played: int = Field(default=0, ge=0, description="Hours played on this storefront")
    ownership_status: OwnershipStatus = Field(default=OwnershipStatus.OWNED, description="Ownership status for this platform")
    acquired_date: Optional[date] = Field(None, description="Date when game was acquired on this platform")
```

**Step 2: Add ownership fields to UserGamePlatformResponse**

```python
class UserGamePlatformResponse(BaseModel, TimestampMixin):
    """Response schema for user game platform association."""
    id: str
    platform: Optional[str]
    storefront: Optional[str]
    platform_details: Optional[PlatformResponse] = Field(default=None, validation_alias="platform_rel")
    storefront_details: Optional[StorefrontResponse] = Field(default=None, validation_alias="storefront_rel")
    store_game_id: Optional[str]
    store_url: Optional[str]
    is_available: bool
    hours_played: int
    ownership_status: OwnershipStatus
    acquired_date: Optional[date]
    original_platform_name: Optional[str] = None
    original_storefront_name: Optional[str] = None

    model_config = ConfigDict(from_attributes=True)
```

**Step 3: Remove ownership fields from UserGameCreateRequest**

Remove `ownership_status` and `acquired_date` from `UserGameCreateRequest`:

```python
class UserGameCreateRequest(BaseModel):
    """Request schema for adding a game to user's collection."""
    game_id: int = Field(..., gt=0, description="Game ID to add to collection")
    personal_rating: Optional[float] = Field(None, ge=1, le=5, description="Personal rating (1-5)")
    is_loved: bool = Field(default=False, description="Whether game is marked as loved")
    play_status: PlayStatus = Field(default=PlayStatus.NOT_STARTED, description="Current play status")
    hours_played: int = Field(default=0, ge=0, description="Hours played")
    personal_notes: Optional[str] = Field(None, description="Personal notes about the game")
    platforms: Optional[List[UserGamePlatformCreateRequest]] = Field(default_factory=list, description="Platform associations with complete details")
```

**Step 4: Remove ownership fields from UserGameUpdateRequest**

Remove `ownership_status` and `acquired_date` from `UserGameUpdateRequest`:

```python
class UserGameUpdateRequest(BaseModel):
    """Request schema for updating user's game collection entry."""
    personal_rating: Optional[float] = Field(None, ge=1, le=5, description="Personal rating (1-5)")
    is_loved: Optional[bool] = Field(None, description="Whether game is marked as loved")
    play_status: Optional[PlayStatus] = Field(None, description="Current play status")
    hours_played: Optional[int] = Field(None, ge=0, description="Hours played")
    personal_notes: Optional[str] = Field(None, description="Personal notes about the game")
```

**Step 5: Remove ownership fields from UserGameResponse**

Remove `ownership_status` and `acquired_date` from `UserGameResponse`:

```python
class UserGameResponse(BaseModel, TimestampMixin):
    """Response schema for user's game collection entry."""
    id: str
    game: GameResponse
    personal_rating: Optional[float]
    is_loved: bool
    play_status: PlayStatus
    hours_played: int
    personal_notes: Optional[str]
    platforms: List[UserGamePlatformResponse]

    model_config = ConfigDict(from_attributes=True)

    @model_validator(mode='after')
    def compute_hours_played(self) -> Self:
        """Compute aggregate hours_played from platforms with legacy fallback."""
        platform_hours = sum(p.hours_played for p in self.platforms)
        if platform_hours > 0:
            self.hours_played = platform_hours
        return self
```

**Step 6: Update BulkStatusUpdateRequest to remove ownership_status**

```python
class BulkStatusUpdateRequest(BaseModel):
    """Request schema for bulk status updates."""
    user_game_ids: List[str] = Field(..., min_length=1, description="List of user game IDs to update")
    play_status: Optional[PlayStatus] = Field(None, description="New play status")
    personal_rating: Optional[float] = Field(None, ge=1, le=5, description="New rating")
    is_loved: Optional[bool] = Field(None, description="New loved status")
```

**Step 7: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: Some errors for API code - we'll fix those next

---

## Task 5: Update API Endpoints

**Files:**
- Modify: [user_games.py](backend/app/api/user_games.py)

**Step 1: Remove _update_ownership_status_after_platform_change function**

This function at lines 70-100 manages automatic ownership status changes. Since ownership is now per-platform, this logic needs to be removed:

```python
# DELETE the entire _update_ownership_status_after_platform_change function (lines 70-100)
```

**Step 2: Update add_platform endpoint to accept ownership fields**

Find the add_platform endpoint and update it to use the new ownership fields from the request:

```python
@router.post("/{user_game_id}/platforms", response_model=UserGamePlatformResponse, status_code=status.HTTP_201_CREATED)
async def add_platform_to_user_game(
    user_game_id: str,
    platform_data: UserGamePlatformCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> UserGamePlatformResponse:
    """Add a platform association to a user's game."""
    # ... existing validation code ...

    # Create platform association with ownership fields
    platform_association = UserGamePlatform(
        user_game_id=user_game_id,
        platform=platform_data.platform,
        storefront=platform_data.storefront,
        store_game_id=platform_data.store_game_id,
        store_url=str(platform_data.store_url) if platform_data.store_url else None,
        is_available=platform_data.is_available,
        hours_played=platform_data.hours_played,
        ownership_status=platform_data.ownership_status,
        acquired_date=platform_data.acquired_date,
    )
    # ... rest of endpoint ...
```

**Step 3: Remove ownership_status from bulk update endpoint**

Find the bulk update endpoint and remove the ownership_status handling:

```python
# In bulk_update_status endpoint, remove this line:
# if request.ownership_status is not None:
#     user_game.ownership_status = request.ownership_status
```

**Step 4: Update create_user_game endpoint**

Remove ownership_status and acquired_date from the UserGame creation:

```python
# In create_user_game endpoint, the UserGame creation should not include ownership_status or acquired_date
user_game = UserGame(
    user_id=current_user.id,
    game_id=request.game_id,
    personal_rating=Decimal(str(request.personal_rating)) if request.personal_rating else None,
    is_loved=request.is_loved,
    play_status=request.play_status,
    hours_played=request.hours_played,
    personal_notes=request.personal_notes,
)
```

**Step 5: Update update_user_game endpoint**

Remove ownership_status and acquired_date handling:

```python
# In update_user_game endpoint, remove:
# if request.ownership_status is not None:
#     user_game.ownership_status = request.ownership_status
# if request.acquired_date is not None:
#     user_game.acquired_date = request.acquired_date
```

**Step 6: Update filtering for ownership_status**

The filter by ownership_status should now query UserGamePlatform instead of UserGame. Find the list endpoint and update:

```python
# Change from:
# if ownership_status:
#     statement = statement.where(UserGame.ownership_status == ownership_status)

# To:
if ownership_status:
    # Filter games that have ANY platform with this ownership status
    statement = statement.where(
        UserGame.id.in_(
            select(UserGamePlatform.user_game_id).where(
                UserGamePlatform.ownership_status == ownership_status
            )
        )
    )
```

**Step 7: Run type checker and tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest -x`
Expected: Type checker passes, some tests may fail (expected - we update tests next)

---

## Task 6: Update Export Schemas and Logic

**Files:**
- Modify: [export.py](backend/app/schemas/export.py)
- Modify: [export.py](backend/app/worker/tasks/import_export/export.py)

**Step 1: Move ownership fields from ExportGameData to ExportPlatformData**

In `backend/app/schemas/export.py`:

```python
class ExportPlatformData(BaseModel):
    """Platform data in export format."""

    platform_id: Optional[str] = None
    platform_name: Optional[str] = None
    storefront_id: Optional[str] = None
    storefront_name: Optional[str] = None
    store_game_id: Optional[str] = None
    store_url: Optional[str] = None
    is_available: bool = True
    hours_played: int = 0
    ownership_status: str = "owned"
    acquired_date: Optional[date] = None


class ExportGameData(BaseModel):
    """Game data in export format (for JSON exports)."""

    # IGDB data
    igdb_id: int = Field(..., description="IGDB game ID for reliable re-import")
    title: str
    release_year: Optional[int] = None

    # User data (ownership moved to platforms)
    play_status: str
    personal_rating: Optional[float] = None
    is_loved: bool = False
    hours_played: int = 0
    personal_notes: Optional[str] = None

    # Relationships
    platforms: List[ExportPlatformData] = Field(default_factory=list)
    tags: List[ExportTagData] = Field(default_factory=list)

    # Timestamps
    created_at: datetime
    updated_at: datetime
```

**Step 2: Update CsvExportRow**

For CSV, we'll use a comma-separated list of ownership statuses (or take the first one):

```python
class CsvExportRow(BaseModel):
    """Single row for CSV export."""

    igdb_id: int
    title: str
    release_year: Optional[int] = None
    play_status: str
    personal_rating: Optional[float] = None
    is_loved: bool = False
    hours_played: int = 0
    personal_notes: Optional[str] = None
    platforms: str = ""  # Comma-separated platform names
    storefronts: str = ""  # Comma-separated storefront names
    ownership_statuses: str = ""  # Comma-separated ownership statuses per platform
    acquired_dates: str = ""  # Comma-separated acquired dates per platform
    tags: str = ""  # Comma-separated tag names
    created_at: str
    updated_at: str
```

**Step 3: Update _user_game_to_export_data function**

In `backend/app/worker/tasks/import_export/export.py`:

```python
def _user_game_to_export_data(
    session: Session,
    user_game: UserGame,
) -> ExportGameData:
    """Convert a UserGame to export data format."""
    platforms_data: List[ExportPlatformData] = []
    for ugp in user_game.platforms:
        platform_data = ExportPlatformData(
            platform_id=ugp.platform,
            platform_name=ugp.platform_rel.display_name if ugp.platform_rel else ugp.original_platform_name,
            storefront_id=ugp.storefront,
            storefront_name=ugp.storefront_rel.display_name if ugp.storefront_rel else ugp.original_storefront_name,
            store_game_id=ugp.store_game_id,
            store_url=ugp.store_url,
            is_available=ugp.is_available,
            hours_played=ugp.hours_played,
            ownership_status=ugp.ownership_status.value,
            acquired_date=ugp.acquired_date,
        )
        platforms_data.append(platform_data)

    # ... tags handling unchanged ...

    release_year = None
    if user_game.game.release_date:
        release_year = user_game.game.release_date.year

    return ExportGameData(
        igdb_id=user_game.game.id,
        title=user_game.game.title,
        release_year=release_year,
        play_status=user_game.play_status.value,
        personal_rating=float(user_game.personal_rating) if user_game.personal_rating else None,
        is_loved=user_game.is_loved,
        hours_played=user_game.hours_played,
        personal_notes=user_game.personal_notes,
        platforms=platforms_data,
        tags=tags_data,
        created_at=user_game.created_at,
        updated_at=user_game.updated_at,
    )
```

**Step 4: Update _user_game_to_csv_row function**

```python
def _user_game_to_csv_row(
    session: Session,
    user_game: UserGame,
) -> CsvExportRow:
    """Convert a UserGame to CSV row format."""
    platform_names: List[str] = []
    storefront_names: List[str] = []
    ownership_statuses: List[str] = []
    acquired_dates: List[str] = []

    for ugp in user_game.platforms:
        if ugp.platform_rel:
            platform_names.append(ugp.platform_rel.name)
        elif ugp.original_platform_name:
            platform_names.append(ugp.original_platform_name)
        if ugp.storefront_rel:
            storefront_names.append(ugp.storefront_rel.name)
        ownership_statuses.append(ugp.ownership_status.value)
        if ugp.acquired_date:
            acquired_dates.append(ugp.acquired_date.isoformat())

    # ... tags handling unchanged ...

    release_year = None
    if user_game.game.release_date:
        release_year = user_game.game.release_date.year

    return CsvExportRow(
        igdb_id=user_game.game.id,
        title=user_game.game.title,
        release_year=release_year,
        play_status=user_game.play_status.value,
        personal_rating=float(user_game.personal_rating) if user_game.personal_rating else None,
        is_loved=user_game.is_loved,
        hours_played=user_game.hours_played,
        personal_notes=user_game.personal_notes,
        platforms=", ".join(sorted(set(platform_names))),
        storefronts=", ".join(sorted(set(storefront_names))),
        ownership_statuses=", ".join(ownership_statuses),
        acquired_dates=", ".join(acquired_dates),
        tags=", ".join(sorted(tag_names)),
        created_at=user_game.created_at.isoformat(),
        updated_at=user_game.updated_at.isoformat(),
    )
```

**Step 5: Update _calculate_export_stats**

Update the stats calculation to count ownership statuses from platforms:

```python
def _calculate_export_stats(games: List[ExportGameData]) -> Dict[str, Any]:
    """Calculate summary statistics for export."""
    stats: Dict[str, Any] = {
        "total_games": len(games),
        "by_play_status": {},
        "by_ownership_status": {},
        # ... other stats ...
    }

    for game in games:
        # Play status stats
        play_status = game.play_status
        stats["by_play_status"][play_status] = stats["by_play_status"].get(play_status, 0) + 1

        # Ownership status stats - count from platforms
        for platform in game.platforms:
            ownership = platform.ownership_status
            stats["by_ownership_status"][ownership] = stats["by_ownership_status"].get(ownership, 0) + 1

    return stats
```

**Step 6: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_export_tasks.py -v`
Expected: Some failures - we'll fix in test updates

---

## Task 7: Update Import Logic

**Files:**
- Modify: [import_nexorious_helpers.py](backend/app/worker/tasks/import_export/import_nexorious_helpers.py)

**Step 1: Update _process_nexorious_game to handle new schema**

The import now needs to handle ownership at platform level. Update the function:

```python
async def _process_nexorious_game(
    session: Session,
    game_service: GameService,
    user_id: str,
    game_data: Dict[str, Any],
) -> str:
    """Process a single game from Nexorious export."""
    # ... validation code unchanged ...

    # Create UserGame WITHOUT ownership_status and acquired_date
    user_game = UserGame(
        user_id=user_id,
        game_id=igdb_id,
        play_status=_map_play_status(game_data.get("play_status")),
        personal_rating=_parse_rating(game_data.get("personal_rating")),
        is_loved=game_data.get("is_loved", False),
        hours_played=game_data.get("hours_played", 0),
        personal_notes=game_data.get("personal_notes"),
    )
    session.add(user_game)
    # ... rest unchanged ...
```

**Step 2: Update _import_platforms to accept ownership data**

```python
async def _import_platforms(
    session: Session,
    user_game: UserGame,
    platforms_data: List[Dict[str, Any]],
    default_ownership_status: Optional[str] = None,
    default_acquired_date: Optional[str] = None,
) -> None:
    """Import platform associations for a user game."""
    for platform_data in platforms_data:
        platform_name = platform_data.get("platform_name") or platform_data.get("platform_id")
        storefront_name = platform_data.get("storefront_name") or platform_data.get("storefront_id")

        # Get ownership from platform data or fall back to defaults (for v1.x import compatibility)
        ownership_status = _map_ownership_status(
            platform_data.get("ownership_status") or default_ownership_status
        )
        acquired_date = _parse_date(
            platform_data.get("acquired_date") or default_acquired_date
        )

        # ... platform/storefront resolution unchanged ...

        ugp = UserGamePlatform(
            user_game_id=user_game.id,
            platform=resolved_platform,
            storefront=resolved_storefront,
            store_game_id=platform_data.get("store_game_id"),
            store_url=platform_data.get("store_url"),
            is_available=platform_data.get("is_available", True),
            hours_played=platform_data.get("hours_played", 0),
            ownership_status=ownership_status,
            acquired_date=acquired_date,
            original_platform_name=original_platform_name,
            original_storefront_name=original_storefront_name,
        )
        session.add(ugp)

    session.commit()
```

**Step 3: Update _process_nexorious_game to pass legacy ownership data for backward compatibility**

For imports from v1.x exports that have ownership at game level:

```python
# In _process_nexorious_game, after creating user_game:
    platforms_data = game_data.get("platforms", [])
    if platforms_data:
        # Pass game-level ownership as defaults for backward compatibility with v1.x exports
        await _import_platforms(
            session,
            user_game,
            platforms_data,
            default_ownership_status=game_data.get("ownership_status"),
            default_acquired_date=game_data.get("acquired_date"),
        )
```

**Step 4: Run import tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_import_tasks.py -v`

---

## Task 8: Update Sync Process Item

**Files:**
- Modify: [process_item.py](backend/app/worker/tasks/sync/process_item.py)

**Step 1: Update the raw SQL INSERT to not include ownership fields**

The INSERT statement at lines 241-246 needs updating:

```python
insert_sql = text("""
    INSERT INTO user_games (id, user_id, game_id, personal_rating, is_loved, play_status, hours_played, personal_notes, created_at, updated_at)
    VALUES (gen_random_uuid(), :user_id, :game_id, NULL, false, 'NOT_STARTED', 0, NULL, NOW(), NOW())
    ON CONFLICT (user_id, game_id) DO NOTHING
    RETURNING id
""")
```

**Step 2: Update _add_platform_association to include ownership_status**

Find the `_add_platform_association` function and update it to set ownership_status:

```python
def _add_platform_association(
    session: Session,
    user_game_id: str,
    platform: Optional[str],
    storefront: Optional[str],
    external_id: Optional[str],
    playtime_hours: int,
    ownership_status: OwnershipStatus = OwnershipStatus.OWNED,
    acquired_date: Optional[date] = None,
) -> None:
    """Add a platform association to a user game."""
    # ... existing logic ...

    ugp = UserGamePlatform(
        user_game_id=user_game_id,
        platform=platform,
        storefront=storefront,
        store_game_id=external_id,
        hours_played=playtime_hours,
        ownership_status=ownership_status,
        acquired_date=acquired_date,
        # ... other fields ...
    )
```

**Step 3: Run sync tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_sync_process_item.py -v`

---

## Task 9: Update Backend Tests

**Files:**
- Modify: [test_integration_user_games.py](backend/app/tests/test_integration_user_games.py)
- Modify: [integration_test_utils.py](backend/app/tests/integration_test_utils.py)
- Modify: [test_export_tasks.py](backend/app/tests/test_export_tasks.py)
- Modify: [test_import_tasks.py](backend/app/tests/test_import_tasks.py)

**Step 1: Update integration_test_utils.py**

Remove ownership_status and acquired_date from test data creation:

```python
def create_test_user_game_data(game_id: int) -> dict:
    """Create test user game data."""
    return {
        "game_id": game_id,
        "play_status": "not_started",
        "personal_rating": 4.0,
        "is_loved": False,
        "hours_played": 0,
        "platforms": [
            {
                "platform": "pc",
                "storefront": "steam",
                "ownership_status": "owned",
                "acquired_date": "2024-01-01",
            }
        ],
    }
```

**Step 2: Update test assertions**

In tests that check for ownership_status on UserGame response, update to check platform level:

```python
# Change from:
# assert data["ownership_status"] == "owned"

# To:
assert data["platforms"][0]["ownership_status"] == "owned"
```

**Step 3: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests pass with >80% coverage

---

## Task 10: Update Frontend Types

**Files:**
- Modify: [game.ts](frontend/src/types/game.ts)

**Step 1: Add ownership fields to UserGamePlatform interface**

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
  hours_played: number;
  ownership_status: OwnershipStatus;
  acquired_date?: string;
  original_platform_name?: string;
  created_at: string;
}
```

**Step 2: Remove ownership fields from UserGame interface**

```typescript
export interface UserGame {
  id: UserGameId;
  game: Game;
  personal_rating?: number | null;
  is_loved: boolean;
  play_status: PlayStatus;
  hours_played: number;
  personal_notes?: string;
  platforms: UserGamePlatform[];
  tags?: Tag[];
  created_at: string;
  updated_at: string;
}
```

**Step 3: Update UserGameUpdateRequest**

```typescript
export interface UserGameUpdateRequest {
  personal_rating?: number | null;
  is_loved?: boolean;
  play_status?: PlayStatus;
  hours_played?: number;
  personal_notes?: string;
}
```

**Step 4: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Type errors will appear (expected - we fix in next tasks)

---

## Task 11: Update Frontend Game Edit Form

**Files:**
- Modify: [game-edit-form.tsx](frontend/src/components/games/game-edit-form.tsx)

**Step 1: Move ownership state to per-platform**

Replace the single ownershipStatus state with per-platform tracking:

```typescript
// Remove:
// const [ownershipStatus, setOwnershipStatus] = useState<OwnershipStatus>(game.ownership_status);
// const [acquiredDate, setAcquiredDate] = useState(game.acquired_date ?? '');

// Add:
const [platformOwnership, setPlatformOwnership] = useState<Record<string, {
  ownershipStatus: OwnershipStatus;
  acquiredDate: string;
}>>(
  Object.fromEntries(
    game.platforms.map((p) => [
      p.id,
      {
        ownershipStatus: p.ownership_status,
        acquiredDate: p.acquired_date ?? ''
      }
    ])
  )
);
```

**Step 2: Update handleSave to not send ownership to game update**

```typescript
// In handleSave, update the updateGame call:
await updateGame.mutateAsync({
  id: game.id,
  data: {
    playStatus,
    personalRating,
    isLoved,
    personalNotes: personalNotes || undefined,
  },
});
```

**Step 3: Move ownership UI to platform section**

In the "Platforms & Playtime" card, add ownership controls per platform:

```tsx
{game.platforms.map((p) => {
  const isSteamPlatform = p.storefront === 'steam';
  const isDisabled = isSteamSyncEnabled && isSteamPlatform;
  const platformData = platformOwnership[p.id] ?? {
    ownershipStatus: p.ownership_status,
    acquiredDate: p.acquired_date ?? ''
  };

  return (
    <div key={p.id} className="space-y-2 p-4 border rounded-lg">
      <div className="font-medium">
        {p.storefront_details?.display_name ||
          p.storefront ||
          p.platform_details?.display_name ||
          p.platform ||
          'Unknown'}
      </div>

      <div className="grid grid-cols-2 gap-4">
        {/* Ownership Status */}
        <div className="space-y-1">
          <Label className="text-xs">Ownership</Label>
          <Select
            value={platformData.ownershipStatus}
            onValueChange={(v) =>
              setPlatformOwnership((prev) => ({
                ...prev,
                [p.id]: { ...prev[p.id], ownershipStatus: v as OwnershipStatus },
              }))
            }
          >
            <SelectTrigger className="h-8">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {OWNERSHIP_STATUS_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Acquired Date */}
        <div className="space-y-1">
          <Label className="text-xs">Acquired</Label>
          <Input
            type="date"
            className="h-8"
            value={platformData.acquiredDate}
            onChange={(e) =>
              setPlatformOwnership((prev) => ({
                ...prev,
                [p.id]: { ...prev[p.id], acquiredDate: e.target.value },
              }))
            }
          />
        </div>

        {/* Hours Played */}
        <div className="space-y-1">
          <Label className="text-xs">Hours</Label>
          <div className="flex items-center gap-2">
            <Input
              type="number"
              min="0"
              className="h-8 w-20"
              value={platformPlaytimes[p.id] ?? p.hours_played}
              onChange={(e) =>
                setPlatformPlaytimes((prev) => ({
                  ...prev,
                  [p.id]: parseInt(e.target.value) || 0,
                }))
              }
              disabled={isDisabled}
            />
            {isDisabled && (
              <span className="text-xs text-muted-foreground">(Synced)</span>
            )}
          </div>
        </div>
      </div>
    </div>
  );
})}
```

**Step 4: Remove the old ownership status and acquired date cards**

Remove the "Ownership Status" and "Acquired Date" fields from the "Status & Rating" card - they're now per-platform.

**Step 5: Update platform association update to include ownership**

```typescript
// In handleSave, update the platform association update:
for (const [platformId, data] of Object.entries(platformOwnership)) {
  const originalPlatform = game.platforms.find((p) => p.id === platformId);
  if (originalPlatform) {
    const hours = platformPlaytimes[platformId] ?? originalPlatform.hours_played;
    const needsUpdate =
      originalPlatform.hours_played !== hours ||
      originalPlatform.ownership_status !== data.ownershipStatus ||
      (originalPlatform.acquired_date ?? '') !== data.acquiredDate;

    if (needsUpdate) {
      await updatePlatformAssoc.mutateAsync({
        userGameId: game.id,
        platformAssociationId: platformId,
        data: {
          platform: originalPlatform.platform || '',
          storefront: originalPlatform.storefront,
          hoursPlayed: hours,
          ownershipStatus: data.ownershipStatus,
          acquiredDate: data.acquiredDate || undefined,
        },
      });
    }
  }
}
```

**Step 6: Run type checker and tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`

---

## Task 12: Update Frontend API and Hooks

**Files:**
- Modify: [games.ts](frontend/src/api/games.ts)

**Step 1: Update platform update request type**

Add ownership fields to the platform update:

```typescript
export interface UpdatePlatformAssociationRequest {
  platform: string;
  storefront?: string;
  hoursPlayed?: number;
  ownershipStatus?: OwnershipStatus;
  acquiredDate?: string;
}
```

**Step 2: Update addPlatformToUserGame request**

```typescript
export interface AddPlatformRequest {
  platform: string;
  storefront?: string;
  ownershipStatus?: OwnershipStatus;
  acquiredDate?: string;
}
```

**Step 3: Run type checker**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: Pass

---

## Task 13: Update Frontend Tests

**Files:**
- Modify: [game-edit-form.test.tsx](frontend/src/components/games/game-edit-form.test.tsx)
- Modify: [games.test.ts](frontend/src/api/games.test.ts)
- Modify: Various other test files using UserGame

**Step 1: Update mock data factories**

Create test factories that include ownership at platform level:

```typescript
const mockUserGamePlatform: UserGamePlatform = {
  id: 'ugp-1',
  platform: 'pc',
  storefront: 'steam',
  is_available: true,
  hours_played: 10,
  ownership_status: OwnershipStatus.OWNED,
  acquired_date: '2024-01-01',
  created_at: '2024-01-01T00:00:00Z',
};

const mockUserGame: UserGame = {
  id: 'ug-1' as UserGameId,
  game: mockGame,
  personal_rating: 4.5,
  is_loved: true,
  play_status: PlayStatus.COMPLETED,
  hours_played: 50,
  personal_notes: 'Great game!',
  platforms: [mockUserGamePlatform],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};
```

**Step 2: Update test assertions**

Remove assertions checking for ownership_status on UserGame, add assertions for platform-level ownership.

**Step 3: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: All tests pass

---

## Task 14: Final Verification

**Step 1: Run full backend test suite**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests pass with >80% coverage

**Step 2: Run full frontend test suite**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check && npm run test`
Expected: All type checks and tests pass

**Step 3: Manual integration test**

1. Start backend: `cd /home/abo/workspace/home/nexorious/backend && uv run python -m app.main`
2. Start frontend: `cd /home/abo/workspace/home/nexorious/frontend && npm run dev`
3. Test scenarios:
   - Add a game with multiple platforms, each with different ownership statuses
   - Edit a game and change ownership status on one platform
   - Filter by ownership status and verify correct games appear
   - Export collection and verify ownership is at platform level in JSON
   - Import the export and verify ownership preserved

**Step 4: Commit**

```bash
git add -A
git commit -m "feat: move ownership status and acquired date to platform level

- Add ownership_status and acquired_date to UserGamePlatform model
- Create migration to move data from UserGame to UserGamePlatform
- Update schemas to reflect new field locations
- Update API endpoints for platform-level ownership
- Update export/import to handle platform-level ownership
- Update frontend to edit ownership per platform
- Maintain backward compatibility for v1.x imports"
```

---

## Notes

### Backward Compatibility
- v1.x JSON exports have ownership at game level
- Import handles this by using game-level values as defaults for all platforms
- v2.x exports have ownership at platform level

### Export Version
- Update export_version to "2.0" to indicate the schema change
- Document the change in export format

### Migration Safety
- Migration copies ownership from UserGame to ALL associated platforms
- Downgrade copies from first platform (by created_at) back to UserGame
- No data loss in either direction

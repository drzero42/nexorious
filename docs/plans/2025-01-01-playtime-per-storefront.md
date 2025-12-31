# Playtime Per Storefront Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Store playtime per storefront instead of a single value per game, enabling accurate tracking across multiple storefronts with automatic Steam sync integration.

**Architecture:** Add `hours_played` field to `UserGamePlatform` model. Keep `UserGame.hours_played` as legacy fallback. Compute aggregate as sum of platform hours. Steam sync auto-populates playtime (read-only when sync enabled).

**Tech Stack:** FastAPI, SQLModel, Alembic, React, TypeScript, TanStack Query

---

## Task 1: Add hours_played to UserGamePlatform Model

**Files:**
- Modify: `backend/app/models/user_game.py:75-99`

**Step 1: Add the field to UserGamePlatform**

In `backend/app/models/user_game.py`, add `hours_played` field to `UserGamePlatform` class:

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
    hours_played: int = Field(default=0, ge=0, description="Hours played on this storefront")  # NEW
    original_platform_name: Optional[str] = Field(default=None, max_length=200, description="Original platform name for unresolved platforms")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    # ... rest unchanged
```

**Step 2: Run type check**

Run: `cd backend && uv run pyrefly check`
Expected: PASS

**Step 3: Commit**

```bash
git add backend/app/models/user_game.py
git commit -m "feat(model): Add hours_played field to UserGamePlatform"
```

---

## Task 2: Create Database Migration

**Files:**
- Create: `backend/app/alembic/versions/XXXX_add_hours_played_to_user_game_platforms.py`

**Step 1: Generate migration**

Run: `cd backend && uv run alembic revision --autogenerate -m "add hours_played to user_game_platforms"`
Expected: New migration file created

**Step 2: Verify migration content**

The migration should contain:
```python
def upgrade() -> None:
    op.add_column('user_game_platforms', sa.Column('hours_played', sa.Integer(), nullable=False, server_default='0'))

def downgrade() -> None:
    op.drop_column('user_game_platforms', 'hours_played')
```

**Step 3: Apply migration**

Run: `cd backend && uv run alembic upgrade head`
Expected: Migration applied successfully

**Step 4: Run tests to verify no regressions**

Run: `cd backend && uv run pytest -x -q`
Expected: All tests pass

**Step 5: Commit**

```bash
git add backend/app/alembic/versions/
git commit -m "feat(db): Add hours_played column to user_game_platforms table"
```

---

## Task 3: Update Backend Schemas

**Files:**
- Modify: `backend/app/schemas/user_game.py:35-42` (UserGamePlatformCreateRequest)
- Modify: `backend/app/schemas/user_game.py:75-88` (UserGamePlatformResponse)

**Step 1: Update UserGamePlatformCreateRequest**

```python
class UserGamePlatformCreateRequest(BaseModel):
    """Request schema for adding platform association to user game."""
    platform: str = Field(..., description="Platform slug")
    storefront: Optional[str] = Field(None, description="Storefront slug")
    store_game_id: Optional[str] = Field(None, max_length=200, description="Game ID in store")
    store_url: Optional[HttpUrl] = Field(None, description="Store URL for game")
    is_available: bool = Field(default=True, description="Whether the game is available on this platform")
    hours_played: int = Field(default=0, ge=0, description="Hours played on this storefront")  # NEW
```

**Step 2: Update UserGamePlatformResponse**

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
    hours_played: int  # NEW
    original_platform_name: Optional[str]

    model_config = ConfigDict(from_attributes=True)
```

**Step 3: Run type check**

Run: `cd backend && uv run pyrefly check`
Expected: PASS

**Step 4: Commit**

```bash
git add backend/app/schemas/user_game.py
git commit -m "feat(schema): Add hours_played to platform request/response schemas"
```

---

## Task 4: Update API Endpoints for Platform Playtime

**Files:**
- Modify: `backend/app/api/user_games.py:1014-1121` (add_platform_to_user_game)
- Modify: `backend/app/api/user_games.py:1124-1233` (update_platform_association)

**Step 1: Update add_platform_to_user_game to accept hours_played**

In the `add_platform_to_user_game` function, update the `UserGamePlatform` creation (around line 1093):

```python
new_platform = UserGamePlatform(
    user_game_id=user_game_id,
    platform=platform_data.platform,
    storefront=platform_data.storefront,
    store_game_id=platform_data.store_game_id,
    store_url=str(platform_data.store_url) if platform_data.store_url else None,
    is_available=platform_data.is_available,
    hours_played=platform_data.hours_played,  # NEW
)
```

**Step 2: Update update_platform_association to handle hours_played**

In the `update_platform_association` function, update the field assignments (around line 1213):

```python
# Update the platform association
platform_assoc.platform = platform_data.platform
platform_assoc.storefront = platform_data.storefront
platform_assoc.store_game_id = platform_data.store_game_id
platform_assoc.store_url = str(platform_data.store_url) if platform_data.store_url else None
platform_assoc.is_available = platform_data.is_available
platform_assoc.hours_played = platform_data.hours_played  # NEW
platform_assoc.updated_at = datetime.now(timezone.utc)
```

**Step 3: Run type check**

Run: `cd backend && uv run pyrefly check`
Expected: PASS

**Step 4: Run tests**

Run: `cd backend && uv run pytest app/tests/test_integration_user_games.py app/tests/test_integration_platforms.py -v`
Expected: All tests pass

**Step 5: Commit**

```bash
git add backend/app/api/user_games.py
git commit -m "feat(api): Support hours_played in platform CRUD operations"
```

---

## Task 5: Update Bulk Platform Operations

**Files:**
- Modify: `backend/app/api/user_games.py:577-654` (bulk_add_platforms_to_user_games)

**Step 1: Update bulk add to include hours_played**

In the `bulk_add_platforms_to_user_games` function, update the `UserGamePlatform` creation (around line 634):

```python
platform_obj = UserGamePlatform(
    user_game_id=user_game.id,
    platform=platform_assoc.platform,
    storefront=platform_assoc.storefront,
    store_game_id=platform_assoc.store_game_id,
    store_url=str(platform_assoc.store_url) if platform_assoc.store_url else None,
    is_available=platform_assoc.is_available,
    hours_played=platform_assoc.hours_played,  # NEW
)
```

**Step 2: Run tests**

Run: `cd backend && uv run pytest app/tests/test_integration_user_games.py -k bulk -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add backend/app/api/user_games.py
git commit -m "feat(api): Support hours_played in bulk platform operations"
```

---

## Task 6: Implement Aggregate Playtime Calculation

**Files:**
- Modify: `backend/app/schemas/user_game.py:90-104` (UserGameResponse)
- Create helper function for calculation

**Step 1: Create a helper function for aggregate calculation**

Add this near the top of `backend/app/schemas/user_game.py` (after imports):

```python
from pydantic import field_validator, model_validator
from typing import Self

# ... existing code ...
```

**Step 2: Update UserGameResponse with computed hours_played**

Replace the `UserGameResponse` class:

```python
class UserGameResponse(BaseModel, TimestampMixin):
    """Response schema for user's game collection entry."""
    id: str
    game: GameResponse
    ownership_status: OwnershipStatus
    personal_rating: Optional[float]
    is_loved: bool
    play_status: PlayStatus
    hours_played: int  # This will be computed
    personal_notes: Optional[str]
    acquired_date: Optional[date]
    platforms: List[UserGamePlatformResponse]

    model_config = ConfigDict(from_attributes=True)

    @model_validator(mode='after')
    def compute_hours_played(self) -> Self:
        """Compute aggregate hours_played from platforms with legacy fallback."""
        platform_hours = sum(p.hours_played for p in self.platforms)
        # If platforms have playtime, use that; otherwise keep legacy value
        if platform_hours > 0:
            self.hours_played = platform_hours
        # else: keep the original hours_played value (legacy fallback)
        return self
```

**Step 3: Run type check**

Run: `cd backend && uv run pyrefly check`
Expected: PASS

**Step 4: Run tests**

Run: `cd backend && uv run pytest app/tests/test_integration_user_games.py -v`
Expected: All tests pass

**Step 5: Commit**

```bash
git add backend/app/schemas/user_game.py
git commit -m "feat(schema): Compute aggregate hours_played from platform playtimes"
```

---

## Task 7: Update Collection Stats for Aggregate Playtime

**Files:**
- Modify: `backend/app/api/user_games.py:332-462` (get_collection_stats)

**Step 1: Update total_hours calculation**

The current implementation sums `UserGame.hours_played` directly. We need to update it to sum platform hours with legacy fallback. Replace the total hours calculation (around line 416):

```python
# Total hours played - sum from platforms with legacy fallback
# First, get all user games with their platforms
user_games_for_hours = session.exec(
    select(UserGame).where(UserGame.user_id == current_user.id)
).all()

total_hours = 0
for ug in user_games_for_hours:
    platform_hours = sum(p.hours_played for p in ug.platforms)
    if platform_hours > 0:
        total_hours += platform_hours
    else:
        total_hours += ug.hours_played  # Legacy fallback
```

**Step 2: Run tests**

Run: `cd backend && uv run pytest app/tests/test_integration_user_games.py -k stats -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add backend/app/api/user_games.py
git commit -m "feat(api): Update collection stats to use aggregate platform playtime"
```

---

## Task 8: Write Backend Tests for Platform Playtime

**Files:**
- Modify: `backend/app/tests/test_integration_user_games.py`

**Step 1: Add test for platform hours_played**

Add a new test class for platform playtime:

```python
class TestUserGamePlatformPlaytime:
    """Test platform-specific playtime functionality."""

    def test_add_platform_with_hours_played(
        self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session
    ):
        """Test adding a platform with hours_played."""
        # First ensure we have a platform
        from ..models.platform import Platform, Storefront
        platform = session.exec(select(Platform).limit(1)).first()
        storefront = session.exec(select(Storefront).limit(1)).first()

        response = client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            headers=auth_headers,
            json={
                "platform": platform.name,
                "storefront": storefront.name if storefront else None,
                "hours_played": 25
            }
        )

        assert response.status_code == 201
        data = response.json()
        # Check that the platform has hours_played
        platform_entry = next(
            (p for p in data["platforms"] if p["platform"] == platform.name), None
        )
        assert platform_entry is not None
        assert platform_entry["hours_played"] == 25

    def test_aggregate_hours_from_platforms(
        self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session
    ):
        """Test that aggregate hours_played is sum of platform hours."""
        from ..models.platform import Platform, Storefront

        # Get two different storefronts
        platforms = session.exec(select(Platform).limit(1)).all()
        storefronts = session.exec(select(Storefront).limit(2)).all()

        if len(storefronts) < 2 or len(platforms) < 1:
            pytest.skip("Need at least 1 platform and 2 storefronts")

        # Add first platform with 10 hours
        client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            headers=auth_headers,
            json={
                "platform": platforms[0].name,
                "storefront": storefronts[0].name,
                "hours_played": 10
            }
        )

        # Add second platform with 20 hours
        response = client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            headers=auth_headers,
            json={
                "platform": platforms[0].name,
                "storefront": storefronts[1].name,
                "hours_played": 20
            }
        )

        assert response.status_code == 201
        data = response.json()
        # Aggregate should be 10 + 20 = 30
        assert data["hours_played"] == 30

    def test_legacy_fallback_when_no_platform_hours(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test that legacy hours_played is used when platforms have 0 hours."""
        from ..models.game import Game

        # Create a game and user_game with legacy hours
        game = session.exec(select(Game).limit(1)).first()

        # Create user game with legacy hours but no platforms
        response = client.post(
            "/api/user-games/",
            headers=auth_headers,
            json={
                "game_id": game.id + 1000,  # Use different game
                "hours_played": 50
            }
        )

        # If game doesn't exist, skip
        if response.status_code == 404:
            pytest.skip("No available game for test")

        data = response.json()
        # Should show legacy hours since no platform hours
        assert data["hours_played"] == 50
```

**Step 2: Run new tests**

Run: `cd backend && uv run pytest app/tests/test_integration_user_games.py::TestUserGamePlatformPlaytime -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add backend/app/tests/test_integration_user_games.py
git commit -m "test: Add tests for platform-specific playtime"
```

---

## Task 9: Update Frontend Types

**Files:**
- Modify: `frontend/src/types/game.ts:108-119` (UserGamePlatform interface)

**Step 1: Add hours_played to UserGamePlatform**

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
  hours_played: number;  // NEW
  original_platform_name?: string;
  created_at: string;
}
```

**Step 2: Run type check**

Run: `cd frontend && npm run check`
Expected: PASS

**Step 3: Commit**

```bash
git add frontend/src/types/game.ts
git commit -m "feat(types): Add hours_played to UserGamePlatform interface"
```

---

## Task 10: Update Game Detail Page Playtime Display

**Files:**
- Modify: `frontend/src/app/(main)/games/[id]/page.tsx`

**Step 1: Add playtime breakdown component**

Find the hours played display section and update it to show breakdown:

```tsx
{/* Playtime Section */}
<div className="space-y-2">
  <div className="flex items-center gap-2">
    <Clock className="h-4 w-4 text-muted-foreground" />
    <span className="font-medium">Total Playtime:</span>
    <span>{game.hours_played} hours</span>
  </div>

  {/* Playtime breakdown by storefront */}
  {game.platforms.some(p => p.hours_played > 0) && (
    <div className="ml-6 space-y-1 text-sm text-muted-foreground">
      {game.platforms
        .filter(p => p.hours_played > 0)
        .map(p => (
          <div key={p.id} className="flex items-center gap-2">
            <span>
              {p.storefront_details?.display_name || p.storefront || 'Unknown'}:
            </span>
            <span>{p.hours_played} hours</span>
          </div>
        ))}
    </div>
  )}
</div>
```

**Step 2: Run frontend tests**

Run: `cd frontend && npm run test -- --run`
Expected: All tests pass (may need to update snapshots)

**Step 3: Commit**

```bash
git add frontend/src/app/\(main\)/games/\[id\]/page.tsx
git commit -m "feat(ui): Display playtime breakdown by storefront on game detail page"
```

---

## Task 11: Update Game Edit Form for Per-Platform Playtime

**Files:**
- Modify: `frontend/src/components/games/game-edit-form.tsx`

**Step 1: Remove game-level hours played input**

Remove the "Hours Played" input from the "Progress & Dates" card (lines 358-370). Replace with a note:

```tsx
{/* Hours Played - now per-platform */}
<div className="space-y-2">
  <Label>Hours Played</Label>
  <p className="text-sm text-muted-foreground">
    Playtime is now tracked per platform. Edit playtime in the Platforms section below.
  </p>
  <p className="text-lg font-medium">{hoursPlayed} hours total</p>
</div>
```

**Step 2: Update platform selector to show/edit hours**

This requires modifying the PlatformSelector component or creating inline playtime inputs. For now, add hours display per platform in the Platforms card:

```tsx
{/* Platforms with playtime */}
<Card>
  <CardHeader>
    <CardTitle>Platforms & Playtime</CardTitle>
  </CardHeader>
  <CardContent className="space-y-4">
    {platformsLoading ? (
      <Skeleton className="h-10 w-full" />
    ) : (
      <>
        <PlatformSelector
          selectedPlatforms={selectedPlatforms}
          availablePlatforms={platforms}
          onChange={setSelectedPlatforms}
          placeholder="Select platforms..."
        />

        {/* Playtime per platform */}
        {game.platforms.length > 0 && (
          <div className="space-y-3 pt-4 border-t">
            <Label>Playtime by Platform</Label>
            {game.platforms.map(p => (
              <div key={p.id} className="flex items-center gap-4">
                <span className="min-w-[150px] text-sm">
                  {p.storefront_details?.display_name || p.storefront || p.platform_details?.display_name || p.platform}
                </span>
                <Input
                  type="number"
                  min="0"
                  className="w-24"
                  value={platformPlaytimes[p.id] ?? p.hours_played}
                  onChange={(e) => handlePlatformPlaytimeChange(p.id, parseInt(e.target.value) || 0)}
                  disabled={isSteamSyncEnabled && p.storefront === 'steam'}
                />
                <span className="text-sm text-muted-foreground">hours</span>
                {isSteamSyncEnabled && p.storefront === 'steam' && (
                  <span className="text-xs text-muted-foreground">(Synced from Steam)</span>
                )}
              </div>
            ))}
          </div>
        )}
      </>
    )}
  </CardContent>
</Card>
```

**Step 3: Add state and handlers for platform playtime**

Add to the component state:

```tsx
const [platformPlaytimes, setPlatformPlaytimes] = useState<Record<string, number>>(
  Object.fromEntries(game.platforms.map(p => [p.id, p.hours_played]))
);

const handlePlatformPlaytimeChange = (platformId: string, hours: number) => {
  setPlatformPlaytimes(prev => ({ ...prev, [platformId]: hours }));
};

// Compute total for display
const hoursPlayed = useMemo(() => {
  const platformHours = Object.values(platformPlaytimes).reduce((sum, h) => sum + h, 0);
  return platformHours > 0 ? platformHours : game.hours_played;
}, [platformPlaytimes, game.hours_played]);
```

**Step 4: Update save handler to update platform playtimes**

In `handleSave`, add logic to update platform playtimes:

```tsx
// Update platform playtimes
for (const [platformId, hours] of Object.entries(platformPlaytimes)) {
  const originalPlatform = game.platforms.find(p => p.id === platformId);
  if (originalPlatform && originalPlatform.hours_played !== hours) {
    await updatePlatformPlaytime.mutateAsync({
      userGameId: game.id,
      platformAssociationId: platformId,
      data: { hours_played: hours }
    });
  }
}
```

**Step 5: Run frontend tests**

Run: `cd frontend && npm run test -- --run`
Expected: Tests may need updates for changed structure

**Step 6: Commit**

```bash
git add frontend/src/components/games/game-edit-form.tsx
git commit -m "feat(ui): Update game edit form for per-platform playtime editing"
```

---

## Task 12: Add Steam Sync Playtime Check

**Files:**
- Modify: `frontend/src/components/games/game-edit-form.tsx`
- Create: `frontend/src/hooks/use-steam-sync-status.ts` (if needed)

**Step 1: Check if Steam sync is enabled**

Add logic to determine if Steam sync is enabled for the current user. This may require a new API call or checking user preferences:

```tsx
// Check if Steam sync is enabled (from user settings/preferences)
const { data: userSettings } = useUserSettings();
const isSteamSyncEnabled = userSettings?.steam?.is_verified &&
  userSettings?.sync_configs?.some(c => c.platform === 'steam' && c.frequency !== 'manual');
```

**Step 2: Disable editing for Steam platforms when sync enabled**

Already implemented in Task 11 via the `disabled` prop on the Input.

**Step 3: Commit**

```bash
git add frontend/src/components/games/game-edit-form.tsx frontend/src/hooks/
git commit -m "feat(ui): Disable Steam playtime editing when sync is enabled"
```

---

## Task 13: Update Steam Sync to Include Playtime

**Files:**
- Modify: `backend/app/worker/tasks/sync/adapters/steam.py`
- Modify: `backend/app/worker/tasks/sync/adapters/base.py`
- Modify: `backend/app/services/steam.py`

**Step 1: Update SteamGame dataclass to include playtime**

In `backend/app/services/steam.py`, update the `SteamGame` class:

```python
@dataclass
class SteamGame:
    """Steam game information from Steam Web API."""
    appid: int
    name: str
    playtime_forever: int = 0  # NEW: Total playtime in minutes
```

**Step 2: Update get_owned_games to extract playtime**

In `backend/app/services/steam.py`, update the `get_owned_games` method:

```python
for game_data in games_data:
    game = SteamGame(
        appid=game_data["appid"],
        name=game_data.get("name", ""),
        playtime_forever=game_data.get("playtime_forever", 0)  # NEW
    )
    games.append(game)
```

**Step 3: Update ExternalGame to include playtime**

In `backend/app/worker/tasks/sync/adapters/base.py`, add playtime to `ExternalGame`:

```python
@dataclass
class ExternalGame:
    """Standardized external game representation."""
    external_id: str
    title: str
    platform: str
    storefront: str
    metadata: dict = field(default_factory=dict)
    playtime_hours: int = 0  # NEW
```

**Step 4: Update SteamSyncAdapter to pass playtime**

In `backend/app/worker/tasks/sync/adapters/steam.py`:

```python
return [
    ExternalGame(
        external_id=str(game.appid),
        title=game.name,
        platform="pc-windows",
        storefront="steam",
        playtime_hours=game.playtime_forever // 60,  # Convert minutes to hours
        metadata={
            "appid": game.appid,
            "playtime_minutes": game.playtime_forever,
        },
    )
    for game in steam_games
]
```

**Step 5: Update sync process to save playtime**

In `backend/app/worker/tasks/sync/process_item.py`, when creating/updating `UserGamePlatform`, include the playtime:

```python
# When creating or updating platform association
platform_assoc.hours_played = external_game.playtime_hours
```

**Step 6: Run tests**

Run: `cd backend && uv run pytest app/tests/test_sync_adapters.py app/tests/test_steam_service.py -v`
Expected: All tests pass

**Step 7: Commit**

```bash
git add backend/app/services/steam.py backend/app/worker/tasks/sync/
git commit -m "feat(sync): Extract and store playtime from Steam sync"
```

---

## Task 14: Update Frontend Tests

**Files:**
- Modify: `frontend/src/components/games/game-edit-form.test.tsx`
- Modify: `frontend/src/app/(main)/games/[id]/page.test.tsx`

**Step 1: Update game-edit-form tests**

Update tests to account for per-platform playtime:

```typescript
it('displays playtime per platform', () => {
  const mockGame = createMockUserGame({
    platforms: [
      { id: '1', platform: 'windows', storefront: 'steam', hours_played: 50 },
      { id: '2', platform: 'windows', storefront: 'epic_games', hours_played: 10 },
    ],
  });

  render(<GameEditForm game={mockGame} />);

  expect(screen.getByText('60 hours total')).toBeInTheDocument();
  expect(screen.getByDisplayValue('50')).toBeInTheDocument();
  expect(screen.getByDisplayValue('10')).toBeInTheDocument();
});
```

**Step 2: Update game detail page tests**

```typescript
it('shows playtime breakdown when platforms have hours', () => {
  // ... mock game with platform hours
  expect(screen.getByText('Steam: 50 hours')).toBeInTheDocument();
});
```

**Step 3: Run all frontend tests**

Run: `cd frontend && npm run test -- --run`
Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/components/games/game-edit-form.test.tsx frontend/src/app/\(main\)/games/\[id\]/page.test.tsx
git commit -m "test: Update frontend tests for per-platform playtime"
```

---

## Task 15: Update Test Mocks and Fixtures

**Files:**
- Modify: `frontend/src/test/mocks/handlers.ts`
- Modify: `backend/app/tests/integration_test_utils.py`

**Step 1: Update frontend mock handlers**

Ensure mock responses include `hours_played` on platforms:

```typescript
// In handlers.ts, update UserGamePlatform mocks
platforms: [
  {
    id: '...',
    platform: 'windows',
    storefront: 'steam',
    hours_played: 25,  // NEW
    // ...
  }
]
```

**Step 2: Update backend test utilities**

Ensure test fixtures include `hours_played`:

```python
def create_test_user_game_platform(
    user_game_id: str,
    platform: str = "windows",
    storefront: str = "steam",
    hours_played: int = 0,  # NEW
    session: Session = None
) -> UserGamePlatform:
    # ...
```

**Step 3: Run all tests**

Run: `cd backend && uv run pytest -x -q && cd ../frontend && npm run test -- --run`
Expected: All tests pass

**Step 4: Commit**

```bash
git add frontend/src/test/mocks/handlers.ts backend/app/tests/integration_test_utils.py
git commit -m "test: Update test fixtures and mocks for hours_played"
```

---

## Task 16: Final Integration Test

**Step 1: Run full backend test suite**

Run: `cd backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests pass, coverage >80%

**Step 2: Run full frontend test suite**

Run: `cd frontend && npm run check && npm run test -- --run`
Expected: Type check passes, all tests pass

**Step 3: Create final commit**

```bash
git add -A
git commit -m "feat: Complete playtime per storefront implementation"
```

---

## Summary

This plan implements playtime tracking per storefront with:

1. **Backend changes:**
   - New `hours_played` field on `UserGamePlatform` model
   - Database migration
   - Updated schemas with aggregate computation
   - Updated API endpoints
   - Steam sync integration

2. **Frontend changes:**
   - Updated TypeScript types
   - Game detail page shows playtime breakdown
   - Game edit form allows per-platform playtime editing
   - Steam platforms are read-only when sync is enabled

3. **Testing:**
   - Backend integration tests for new functionality
   - Frontend component tests updated
   - Mock data updated

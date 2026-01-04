# PSN Playtime Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add playtime synchronization for PSN games by fetching title_stats from PSNAWP and enriching game data during library sync.

**Architecture:** Fetch `title_stats()` alongside `game_entitlements()` in PSNService, build a lookup map by title_id, and enrich PSNGame objects with playtime. The adapter simply passes playtime through to ExternalGame (matching Steam's pattern).

**Tech Stack:** Python, PSNAWP library, pytest

---

## Task 1: Fix PSN Service Tests (Broken Mocks)

The existing PSN service tests mock `purchased_games()` but the implementation uses `game_entitlements()`. Fix tests to mock the correct API.

**Files:**
- Modify: `backend/app/tests/test_psn_service.py:77-193`

**Step 1: Fix test_get_account_info_success mock**

The test mocks `get_region()` returning a string "us" but the implementation expects a Country object with `.alpha_2` attribute.

```python
@pytest.mark.asyncio
async def test_get_account_info_success():
    """Test getting account info succeeds."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_client = Mock()
        mock_client.online_id = "test_user"
        mock_client.account_id = "account123"
        # Mock Country object with alpha_2 attribute
        mock_region = Mock()
        mock_region.alpha_2 = "US"
        mock_client.get_region.return_value = mock_region
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.get_account_info()

        assert result.online_id == "test_user"
        assert result.account_id == "account123"
        assert result.region == "US"
```

**Step 2: Fix test_get_library_success to mock game_entitlements**

```python
@pytest.mark.asyncio
async def test_get_library_success():
    """Test getting library returns PSN games with platform detection."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        # Mock game_entitlements response (list of dicts)
        mock_entitlement1 = {
            "productId": "PROD001",
            "titleMeta": {"titleId": "GAME001", "name": "Test Game 1"},
            "entitlementAttributes": [{"platformId": "ps5"}]
        }
        mock_entitlement2 = {
            "productId": "PROD002",
            "titleMeta": {"titleId": "GAME002", "name": "Test Game 2 (PS4+PS5)"},
            "entitlementAttributes": [{"platformId": "ps5"}, {"platformId": "ps4"}]
        }

        mock_client = Mock()
        mock_client.game_entitlements.return_value = [mock_entitlement1, mock_entitlement2]
        mock_client.title_stats.return_value = []  # No playtime data
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.get_library()

        assert len(result) == 2
        assert result[0].product_id == "GAME001"
        assert result[0].name == "Test Game 1"
        assert result[0].platforms == ["playstation-5"]
        assert result[0].playtime_hours == 0
        assert result[1].product_id == "GAME002"
        assert result[1].platforms == ["playstation-5", "playstation-4"]
```

**Step 3: Fix test_get_library_fallback_platform**

```python
@pytest.mark.asyncio
async def test_get_library_fallback_platform():
    """Test library falls back to PS5 when no platform info."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_entitlement = {
            "productId": "PROD003",
            "titleMeta": {"titleId": "GAME003", "name": "Test Game 3"},
            "entitlementAttributes": []  # No platform attributes
        }

        mock_client = Mock()
        mock_client.game_entitlements.return_value = [mock_entitlement]
        mock_client.title_stats.return_value = []
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.get_library()

        assert len(result) == 1
        assert result[0].platforms == ["playstation-5"]
```

**Step 4: Fix test_get_library_expired_token**

```python
@pytest.mark.asyncio
async def test_get_library_expired_token():
    """Test library fetch fails with expired token."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_client = Mock()
        mock_client.title_stats.side_effect = Exception("Token expired")
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService, PSNTokenExpiredError
        service = PSNService("a" * 64)

        with pytest.raises(PSNTokenExpiredError):
            await service.get_library()
```

**Step 5: Run tests to verify fixes**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pytest app/tests/test_psn_service.py -v`
Expected: All 12 tests PASS

**Step 6: Commit**

```bash
git add app/tests/test_psn_service.py
git commit -m "fix(tests): update PSN service tests to mock game_entitlements API"
```

---

## Task 2: Add Playtime to PSNGame Dataclass

**Files:**
- Modify: `backend/app/services/psn.py:20-26`

**Step 1: Add playtime_hours field to PSNGame**

```python
@dataclass
class PSNGame:
    """PSN game information from purchased library."""
    product_id: str       # Unique game identifier
    name: str             # Game title
    platforms: List[str]  # ["playstation-4", "playstation-5"]
    metadata: Dict[str, Any]  # Additional game metadata
    playtime_hours: int = 0   # Total playtime in hours
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
git add app/services/psn.py
git commit -m "feat(psn): add playtime_hours field to PSNGame dataclass"
```

---

## Task 3: Implement Playtime Fetch in PSNService

**Files:**
- Modify: `backend/app/services/psn.py:106-159`

**Step 1: Update get_library to fetch title_stats and merge playtime**

```python
async def get_library(self) -> List[PSNGame]:
    """Get purchased games from PSN library with playtime (PS4/PS5 only).

    Returns:
        List of PSN games with platform entitlements and playtime

    Raises:
        PSNTokenExpiredError: If token has expired
        PSNAPIError: If library cannot be retrieved
    """
    try:
        client = self.psnawp.me()

        # Fetch playtime stats and build lookup by title_id
        playtime_lookup: Dict[str, int] = {}
        for stats in client.title_stats(limit=None):
            if stats.title_id and stats.play_duration:
                hours = int(stats.play_duration.total_seconds() // 3600)
                playtime_lookup[stats.title_id] = hours

        logger.info(f"Fetched playtime data for {len(playtime_lookup)} games from PSN")

        # Fetch game entitlements
        game_entitlements = client.game_entitlements(limit=None)

        games = []
        for entitlement in game_entitlements:
            # Get game name and title ID
            title_meta = entitlement.get("titleMeta", {})
            game_name = title_meta.get("name", "Unknown Game")
            title_id = title_meta.get("titleId", entitlement.get("productId", ""))

            # Look up playtime by title_id
            playtime = playtime_lookup.get(title_id, 0)

            # Detect which platforms user has entitlement for based on entitlementAttributes
            platforms = []
            entitlement_attrs = entitlement.get("entitlementAttributes", [])
            for attr in entitlement_attrs:
                platform_id = attr.get("platformId", "")
                if platform_id == "ps5":
                    platforms.append("playstation-5")
                elif platform_id == "ps4":
                    platforms.append("playstation-4")

            # Fallback to PS5 if no platform info
            if not platforms:
                platforms = ["playstation-5"]

            psn_game = PSNGame(
                product_id=title_id,
                name=game_name,
                platforms=platforms,
                metadata={
                    "product_id": entitlement.get("productId", ""),
                    "title_id": title_id,
                },
                playtime_hours=playtime,
            )
            games.append(psn_game)

        logger.info(f"Retrieved {len(games)} games from PSN library")
        return games

    except Exception as e:
        error_str = str(e).lower()
        if "expired" in error_str or "unauthorized" in error_str:
            raise PSNTokenExpiredError("NPSSO token has expired")
        raise PSNAPIError(f"Failed to retrieve PSN library: {e}")
```

**Step 2: Add Dict import if not present**

Ensure `from typing import List, Dict, Any` is at the top of the file.

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pyrefly check`
Expected: No errors

**Step 4: Run PSN service tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pytest app/tests/test_psn_service.py -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add app/services/psn.py
git commit -m "feat(psn): fetch title_stats and merge playtime into library results"
```

---

## Task 4: Add Playtime Test for PSN Service

**Files:**
- Modify: `backend/app/tests/test_psn_service.py`

**Step 1: Add test for playtime enrichment**

Add this test after the existing library tests:

```python
@pytest.mark.asyncio
async def test_get_library_with_playtime():
    """Test getting library includes playtime from title_stats."""
    from datetime import timedelta

    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        # Mock game_entitlements response
        mock_entitlement = {
            "productId": "PROD001",
            "titleMeta": {"titleId": "CUSA12345", "name": "Test Game"},
            "entitlementAttributes": [{"platformId": "ps5"}]
        }

        # Mock title_stats response with playtime
        mock_stats = Mock()
        mock_stats.title_id = "CUSA12345"
        mock_stats.play_duration = timedelta(hours=42, minutes=30)

        mock_client = Mock()
        mock_client.game_entitlements.return_value = [mock_entitlement]
        mock_client.title_stats.return_value = [mock_stats]
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.get_library()

        assert len(result) == 1
        assert result[0].product_id == "CUSA12345"
        assert result[0].playtime_hours == 42  # 42h30m truncates to 42h
```

**Step 2: Run test**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pytest app/tests/test_psn_service.py::test_get_library_with_playtime -v`
Expected: PASS

**Step 3: Commit**

```bash
git add app/tests/test_psn_service.py
git commit -m "test(psn): add test for playtime enrichment from title_stats"
```

---

## Task 5: Update PSN Adapter to Pass Playtime

**Files:**
- Modify: `backend/app/worker/tasks/sync/adapters/psn.py:127`

**Step 1: Update playtime_hours in ExternalGame creation**

Change line 127 from:
```python
playtime_hours=0  # PSN doesn't provide playtime in library
```

To:
```python
playtime_hours=game.playtime_hours,
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Run adapter tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pytest app/tests/test_psn_sync_adapter.py -v`
Expected: All tests PASS

**Step 4: Commit**

```bash
git add app/worker/tasks/sync/adapters/psn.py
git commit -m "feat(psn): pass playtime_hours through to ExternalGame"
```

---

## Task 6: Fix PSN Adapter _mark_token_expired Test

The test expects `user.preferences` to be modified directly, but the implementation modifies `user.preferences_json`.

**Files:**
- Modify: `backend/app/tests/test_psn_sync_adapter.py:99-117`

**Step 1: Fix the test to check preferences_json**

```python
def test_mark_token_expired():
    """Test _mark_token_expired marks token as invalid."""
    import json
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    user = Mock()
    user.id = "user123"
    user.preferences = {
        "psn": {
            "npsso_token": "a" * 64,
            "is_verified": True
        }
    }
    session = Mock()

    adapter = PSNSyncAdapter()
    adapter._mark_token_expired(user, session)

    # The implementation sets preferences_json, not preferences directly
    updated_prefs = json.loads(user.preferences_json)
    assert updated_prefs["psn"]["is_verified"] is False
    assert "token_expired_at" in updated_prefs["psn"]
    session.add.assert_called_once_with(user)
```

**Step 2: Run test**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pytest app/tests/test_psn_sync_adapter.py::test_mark_token_expired -v`
Expected: PASS

**Step 3: Commit**

```bash
git add app/tests/test_psn_sync_adapter.py
git commit -m "fix(tests): update _mark_token_expired test to check preferences_json"
```

---

## Task 7: Add Playtime Test for PSN Adapter

**Files:**
- Modify: `backend/app/tests/test_psn_sync_adapter.py`

**Step 1: Add test for playtime passthrough**

Add after existing fetch_games tests:

```python
@pytest.mark.asyncio
async def test_fetch_games_includes_playtime():
    """Test fetch_games includes playtime from PSNGame."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter
    from app.services.psn import PSNGame

    user = Mock()
    user.id = "user123"
    user.preferences = {
        "psn": {
            "npsso_token": "a" * 64,
            "is_verified": True
        }
    }
    session = Mock()

    mock_game = PSNGame(
        product_id="CUSA12345",
        name="Test Game",
        platforms=["playstation-5"],
        metadata={"product_id": "CUSA12345"},
        playtime_hours=42,
    )

    with patch('app.worker.tasks.sync.adapters.psn.PSNService') as mock_service_class:
        mock_service = AsyncMock()
        mock_service.get_library.return_value = [mock_game]
        mock_service_class.return_value = mock_service

        adapter = PSNSyncAdapter()
        result = await adapter.fetch_games(user, session)

    assert len(result) == 1
    assert result[0].playtime_hours == 42
```

**Step 2: Run test**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pytest app/tests/test_psn_sync_adapter.py::test_fetch_games_includes_playtime -v`
Expected: PASS

**Step 3: Commit**

```bash
git add app/tests/test_psn_sync_adapter.py
git commit -m "test(psn): add test for playtime passthrough in adapter"
```

---

## Task 8: Fix Sync Dispatch Tests (Broken Mocks)

The tests fail because `_create_job_item` now checks for existing items via `session.exec()`, which returns items causing the function to return `None`.

**Files:**
- Modify: `backend/app/tests/test_sync_dispatch.py`

**Step 1: Fix TestCreateJobItem tests to mock session.exec**

Update all three tests in `TestCreateJobItem` class to mock `session.exec`:

```python
class TestCreateJobItem:
    """Tests for _create_job_item helper."""

    def test_creates_job_item_with_correct_fields(self):
        """Test JobItem is created with correct fields."""
        session = MagicMock()
        job = MagicMock()
        job.id = "job123"

        game = ExternalGame(
            external_id="12345",
            title="Test Game",
            platform="pc-windows",
            storefront="steam",
            metadata={"playtime_minutes": 100},
        )

        # Mock session.exec to return empty result (no existing item)
        mock_result = MagicMock()
        mock_result.first.return_value = None
        session.exec.return_value = mock_result

        # Mock session behavior
        def mock_refresh(item):
            item.id = "item123"

        session.refresh = mock_refresh

        job_item = _create_job_item(session, job, "user123", game)

        assert job_item is not None
        assert job_item.job_id == "job123"
        assert job_item.user_id == "user123"
        assert job_item.item_key == "steam_12345"
        assert job_item.source_title == "Test Game"

        metadata = json.loads(job_item.source_metadata_json)
        assert metadata["external_id"] == "12345"
        assert metadata["platform"] == "pc-windows"
        assert metadata["storefront"] == "steam"
        assert metadata["metadata"]["playtime_minutes"] == 100

    def test_creates_job_item_with_empty_metadata(self):
        """Test JobItem is created correctly with empty metadata."""
        session = MagicMock()
        job = MagicMock()
        job.id = "job456"

        game = ExternalGame(
            external_id="99999",
            title="Another Game",
            platform="pc-windows",
            storefront="gog",
            metadata={},
        )

        # Mock session.exec to return empty result
        mock_result = MagicMock()
        mock_result.first.return_value = None
        session.exec.return_value = mock_result

        def mock_refresh(item):
            item.id = "item456"

        session.refresh = mock_refresh

        job_item = _create_job_item(session, job, "user456", game)

        assert job_item is not None
        assert job_item.job_id == "job456"
        assert job_item.user_id == "user456"
        assert job_item.item_key == "gog_99999"
        assert job_item.source_title == "Another Game"

        metadata = json.loads(job_item.source_metadata_json)
        assert metadata["metadata"] == {}

    def test_session_add_and_commit_called(self):
        """Test that session.add and session.commit are called."""
        session = MagicMock()
        job = MagicMock()
        job.id = "job789"

        game = ExternalGame(
            external_id="11111",
            title="Game Name",
            platform="pc-windows",
            storefront="steam",
            metadata={},
        )

        # Mock session.exec to return empty result
        mock_result = MagicMock()
        mock_result.first.return_value = None
        session.exec.return_value = mock_result

        session.refresh = MagicMock()

        _create_job_item(session, job, "user789", game)

        session.add.assert_called_once()
        session.commit.assert_called_once()
        session.refresh.assert_called_once()
```

**Step 2: Fix TestDispatchSyncItems tests to mock session.exec**

Update tests that create JobItems to mock `session.exec`:

```python
@pytest.mark.asyncio
async def test_dispatches_items_for_each_game(self):
    """Test creates JobItems and dispatches tasks for each game."""
    mock_games = [
        ExternalGame("1", "Game 1", "pc-windows", "steam", {}),
        ExternalGame("2", "Game 2", "pc-windows", "steam", {}),
    ]

    with (
        patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
        patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
        patch(
            "app.worker.tasks.sync.dispatch._dispatch_process_task"
        ) as mock_dispatch,
    ):
        mock_session = MagicMock()
        mock_job = MagicMock()
        mock_job.id = "job123"
        mock_job.priority = BackgroundJobPriority.HIGH
        mock_user = MagicMock()
        mock_user.id = "user123"

        def get_side_effect(model, id):
            if model == Job:
                return mock_job
            return mock_user

        mock_session.get.side_effect = get_side_effect

        # Mock session.exec to return empty result (no existing items)
        mock_exec_result = MagicMock()
        mock_exec_result.first.return_value = None
        mock_session.exec.return_value = mock_exec_result

        # Mock refresh to set ID
        def mock_refresh(item):
            if hasattr(item, "id") and item.id is None:
                item.id = f"item_{item.item_key}"

        mock_session.refresh = mock_refresh

        async_ctx = AsyncMock()
        async_ctx.__aenter__.return_value = mock_session
        async_ctx.__aexit__.return_value = None
        mock_ctx.return_value = async_ctx

        mock_adapter_instance = MagicMock()
        mock_adapter_instance.fetch_games = AsyncMock(return_value=mock_games)
        mock_adapter.return_value = mock_adapter_instance

        mock_dispatch.return_value = None

        result = await dispatch_sync_items("job123", "user123", "steam")

        assert result["status"] == "dispatched"
        assert result["total_games"] == 2
        assert result["dispatched"] == 2
        assert mock_dispatch.call_count == 2
```

**Step 3: Fix test_counts_errors_during_dispatch**

```python
@pytest.mark.asyncio
async def test_counts_errors_during_dispatch(self):
    """Test that errors during individual item dispatch are counted."""
    mock_games = [
        ExternalGame("1", "Game 1", "pc-windows", "steam", {}),
        ExternalGame("2", "Game 2", "pc-windows", "steam", {}),
    ]

    with (
        patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
        patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
        patch(
            "app.worker.tasks.sync.dispatch._dispatch_process_task"
        ) as mock_dispatch,
    ):
        mock_session = MagicMock()
        mock_job = MagicMock()
        mock_job.id = "job123"
        mock_job.priority = BackgroundJobPriority.HIGH
        mock_user = MagicMock()
        mock_user.id = "user123"

        mock_session.get.side_effect = lambda model, id: (
            mock_job if model == Job else mock_user
        )

        # Mock session.exec to return empty result
        mock_exec_result = MagicMock()
        mock_exec_result.first.return_value = None
        mock_session.exec.return_value = mock_exec_result

        # First refresh succeeds, second raises an error
        call_count = [0]

        def mock_refresh(item):
            call_count[0] += 1
            if call_count[0] == 1:
                item.id = "item_1"
            else:
                raise Exception("Database error")

        mock_session.refresh = mock_refresh

        async_ctx = AsyncMock()
        async_ctx.__aenter__.return_value = mock_session
        async_ctx.__aexit__.return_value = None
        mock_ctx.return_value = async_ctx

        mock_adapter_instance = MagicMock()
        mock_adapter_instance.fetch_games = AsyncMock(return_value=mock_games)
        mock_adapter.return_value = mock_adapter_instance

        mock_dispatch.return_value = None

        result = await dispatch_sync_items("job123", "user123", "steam")

        assert result["status"] == "dispatched"
        assert result["total_games"] == 2
        assert result["dispatched"] == 1
        assert result["errors"] == 1
```

**Step 4: Run sync dispatch tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pytest app/tests/test_sync_dispatch.py -v`
Expected: All 11 tests PASS

**Step 5: Commit**

```bash
git add app/tests/test_sync_dispatch.py
git commit -m "fix(tests): update sync dispatch tests to mock session.exec for idempotency check"
```

---

## Task 9: Run Full Test Suite and Verify

**Step 1: Run all backend tests**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pytest -v 2>&1 | tail -30`
Expected: All 1214+ tests PASS (0 failures)

**Step 2: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Run linting**

Run: `cd /home/abo/workspace/home/nexorious/.worktrees/psn-playtime/backend && uv run ruff check .`
Expected: No errors

---

## Task 10: Final Commit and Summary

**Step 1: Review all changes**

Run: `git log --oneline main..HEAD`
Expected: 7-8 commits showing the implementation

**Step 2: Create summary commit if needed**

If any final adjustments were made, commit them.

---

## Summary of Changes

1. **PSNGame dataclass** - Added `playtime_hours: int = 0` field
2. **PSNService.get_library()** - Fetches `title_stats()`, builds lookup, enriches games with playtime
3. **PSNSyncAdapter.fetch_games()** - Passes `game.playtime_hours` to ExternalGame
4. **Tests fixed:**
   - `test_psn_service.py` - Updated mocks from `purchased_games` to `game_entitlements`/`title_stats`
   - `test_psn_sync_adapter.py` - Fixed `_mark_token_expired` test, added playtime test
   - `test_sync_dispatch.py` - Added `session.exec` mock for idempotency check

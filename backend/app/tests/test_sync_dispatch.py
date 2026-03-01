"""Tests for sync dispatch task."""

import pytest
import json
from unittest.mock import AsyncMock, MagicMock, patch

from app.worker.tasks.sync.dispatch import dispatch_sync_items, _create_job_item
from app.worker.tasks.sync.adapters import ExternalLibraryEntry
from app.models.external_game import ExternalGame as ExternalGameModel
from app.models.user_game import OwnershipStatus, UserGamePlatform
from app.models.job import (
    Job,
    BackgroundJobStatus,
    BackgroundJobPriority,
)


class TestCreateJobItem:
    """Tests for _create_job_item helper."""

    def _make_eg(self, **kwargs) -> ExternalGameModel:
        defaults = dict(
            id="eg123", user_id="user123", storefront="steam",
            external_id="12345", title="Test Game",
        )
        defaults.update(kwargs)
        return ExternalGameModel(**defaults)

    def test_creates_job_item_with_correct_fields(self):
        """Test JobItem is created with correct fields from ExternalGame."""
        session = MagicMock()
        job = MagicMock()
        job.id = "job123"
        eg = self._make_eg()

        mock_result = MagicMock()
        mock_result.first.return_value = None
        session.exec.return_value = mock_result
        session.refresh = MagicMock()

        job_item = _create_job_item(session, job, eg)

        assert job_item is not None
        assert job_item.job_id == "job123"
        assert job_item.user_id == "user123"
        assert job_item.item_key == "steam_12345"
        assert job_item.source_title == "Test Game"

        metadata = json.loads(job_item.source_metadata_json)
        assert metadata["external_game_id"] == "eg123"

    def test_creates_job_item_for_different_storefront(self):
        """Test JobItem item_key reflects storefront."""
        session = MagicMock()
        job = MagicMock()
        job.id = "job456"
        eg = self._make_eg(id="eg456", user_id="user456", storefront="gog", external_id="99999", title="Another Game")

        mock_result = MagicMock()
        mock_result.first.return_value = None
        session.exec.return_value = mock_result
        session.refresh = MagicMock()

        job_item = _create_job_item(session, job, eg)

        assert job_item is not None
        assert job_item.job_id == "job456"
        assert job_item.user_id == "user456"
        assert job_item.item_key == "gog_99999"
        assert job_item.source_title == "Another Game"

    def test_session_add_and_commit_called(self):
        """Test that session.add and session.commit are called."""
        session = MagicMock()
        job = MagicMock()
        job.id = "job789"
        eg = self._make_eg(id="eg789", user_id="user789", storefront="steam", external_id="11111", title="Game Name")

        mock_result = MagicMock()
        mock_result.first.return_value = None
        session.exec.return_value = mock_result
        session.refresh = MagicMock()

        job_item = _create_job_item(session, job, eg)

        assert job_item is not None
        session.add.assert_called_once()
        session.commit.assert_called_once()
        session.refresh.assert_called_once()


class TestDispatchSyncItems:
    """Tests for dispatch_sync_items task."""

    @pytest.mark.asyncio
    async def test_returns_error_when_job_not_found(self):
        """Test returns error when job doesn't exist."""
        with patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx:
            mock_session = MagicMock()
            mock_session.get.return_value = None

            # Create async context manager mock
            async_ctx = AsyncMock()
            async_ctx.__aenter__.return_value = mock_session
            async_ctx.__aexit__.return_value = None
            mock_ctx.return_value = async_ctx

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "error"
            assert "Job not found" in result["error"]

    @pytest.mark.asyncio
    async def test_returns_error_when_user_not_found(self):
        """Test returns error when user doesn't exist."""
        with patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx:
            mock_session = MagicMock()
            mock_job = MagicMock()
            mock_job.id = "job123"

            # Return job on first call, None on second (user lookup)
            mock_session.get.side_effect = lambda model, id: mock_job if model == Job else None

            async_ctx = AsyncMock()
            async_ctx.__aenter__.return_value = mock_session
            async_ctx.__aexit__.return_value = None
            mock_ctx.return_value = async_ctx

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "error"
            assert "User" in result["error"]

    @pytest.mark.asyncio
    async def test_dispatches_items_for_each_game(self):
        """Test creates JobItems and dispatches tasks for each game."""
        mock_games = [
            ExternalLibraryEntry("1", "Game 1", "pc-windows", "steam", {}),
            ExternalLibraryEntry("2", "Game 2", "pc-windows", "steam", {}),
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

    @pytest.mark.asyncio
    async def test_updates_job_status_to_processing(self):
        """Test job status is updated to PROCESSING when dispatch starts."""
        mock_games = [ExternalLibraryEntry("1", "Game 1", "pc-windows", "steam", {})]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task"),
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
            mock_session.refresh = MagicMock()

            async_ctx = AsyncMock()
            async_ctx.__aenter__.return_value = mock_session
            async_ctx.__aexit__.return_value = None
            mock_ctx.return_value = async_ctx

            mock_adapter_instance = MagicMock()
            mock_adapter_instance.fetch_games = AsyncMock(return_value=mock_games)
            mock_adapter.return_value = mock_adapter_instance

            await dispatch_sync_items("job123", "user123", "steam")

            # Verify job status was set to PROCESSING
            assert mock_job.status == BackgroundJobStatus.PROCESSING

    @pytest.mark.asyncio
    async def test_updates_job_total_items(self):
        """Test job total_items is updated with number of actionable games."""
        mock_games = [
            ExternalLibraryEntry("1", "Game 1", "pc-windows", "steam", {}),
            ExternalLibraryEntry("2", "Game 2", "pc-windows", "steam", {}),
            ExternalLibraryEntry("3", "Game 3", "pc-windows", "steam", {}),
        ]
        mock_egs = [
            ExternalGameModel(id=f"eg{i}", user_id="user123", storefront="steam",
                              external_id=str(i), title=f"Game {i}",
                              is_available=True, is_skipped=False)
            for i in range(1, 4)
        ]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._upsert_external_game", side_effect=mock_egs),
            patch("app.worker.tasks.sync.dispatch._mark_removed_games"),
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task"),
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
            mock_session.exec.return_value.first.return_value = None
            mock_session.refresh = MagicMock()

            async_ctx = AsyncMock()
            async_ctx.__aenter__.return_value = mock_session
            async_ctx.__aexit__.return_value = None
            mock_ctx.return_value = async_ctx

            mock_adapter_instance = MagicMock()
            mock_adapter_instance.fetch_games = AsyncMock(return_value=mock_games)
            mock_adapter.return_value = mock_adapter_instance

            await dispatch_sync_items("job123", "user123", "steam")

            # Verify total_items was set to actionable count (all 3 available, non-skipped)
            assert mock_job.total_items == 3

    @pytest.mark.asyncio
    async def test_handles_adapter_fetch_error(self):
        """Test that adapter fetch errors are handled properly."""
        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
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

            async_ctx = AsyncMock()
            async_ctx.__aenter__.return_value = mock_session
            async_ctx.__aexit__.return_value = None
            mock_ctx.return_value = async_ctx

            mock_adapter_instance = MagicMock()
            mock_adapter_instance.fetch_games = AsyncMock(
                side_effect=Exception("API Error")
            )
            mock_adapter.return_value = mock_adapter_instance

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "error"
            assert "API Error" in result["error"]
            # Verify job was marked as failed
            assert mock_job.status == BackgroundJobStatus.FAILED

    @pytest.mark.asyncio
    async def test_handles_empty_game_list(self):
        """Test handling when no games are returned from adapter."""
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
            mock_session.refresh = MagicMock()

            async_ctx = AsyncMock()
            async_ctx.__aenter__.return_value = mock_session
            async_ctx.__aexit__.return_value = None
            mock_ctx.return_value = async_ctx

            mock_adapter_instance = MagicMock()
            mock_adapter_instance.fetch_games = AsyncMock(return_value=[])
            mock_adapter.return_value = mock_adapter_instance

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "dispatched"
            assert result["total_games"] == 0
            assert result["dispatched"] == 0
            assert mock_dispatch.call_count == 0

    @pytest.mark.asyncio
    async def test_counts_errors_during_dispatch(self):
        """Test that errors during ExternalGame upsert are counted."""
        mock_games = [
            ExternalLibraryEntry("1", "Game 1", "pc-windows", "steam", {}),
            ExternalLibraryEntry("2", "Game 2", "pc-windows", "steam", {}),
        ]
        eg1 = ExternalGameModel(
            id="eg1", user_id="user123", storefront="steam",
            external_id="1", title="Game 1",
            is_available=True, is_skipped=False,
        )

        def upsert_side_effect(session, user_id, entry):
            if entry.external_id == "1":
                return eg1
            raise Exception("Database error")

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._upsert_external_game", side_effect=upsert_side_effect),
            patch("app.worker.tasks.sync.dispatch._mark_removed_games"),
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task") as mock_dispatch,
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
            mock_session.exec.return_value.first.return_value = None
            mock_session.refresh = MagicMock()

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


class TestUpsertExternalGames:
    """Tests for Phase 2: _upsert_external_game."""

    def test_creates_new_external_game(self):
        from app.worker.tasks.sync.dispatch import _upsert_external_game
        session = MagicMock()
        session.exec.return_value.first.return_value = None

        entry = ExternalLibraryEntry(
            external_id="730", title="CS2", platform="pc-windows",
            storefront="steam", metadata={}, playtime_hours=10,
        )
        result = _upsert_external_game(session, "user1", entry)

        session.add.assert_called_once()
        assert result.external_id == "730"
        assert result.is_available is True
        assert result.playtime_hours == 10

    def test_updates_existing_external_game(self):
        from app.worker.tasks.sync.dispatch import _upsert_external_game
        session = MagicMock()
        existing = ExternalGameModel(
            id="eg1", user_id="user1", storefront="steam",
            external_id="730", title="Counter-Strike 2",
            playtime_hours=5, is_available=False,
        )
        session.exec.return_value.first.return_value = existing

        entry = ExternalLibraryEntry(
            external_id="730", title="CS2", platform="pc-windows",
            storefront="steam", metadata={}, playtime_hours=20,
        )
        result = _upsert_external_game(session, "user1", entry)

        assert result.playtime_hours == 20
        assert result.title == "CS2"
        assert result.is_available is True


class TestMarkRemovedGames:
    """Tests for Phase 3: _mark_removed_games."""

    def test_marks_missing_game_unavailable(self):
        from app.worker.tasks.sync.dispatch import _mark_removed_games
        session = MagicMock()
        removed_game = ExternalGameModel(
            id="eg1", user_id="user1", storefront="steam",
            external_id="999", title="Old Game",
            is_available=True, is_subscription=False,
        )
        removed_game.user_game_platforms = []
        session.exec.return_value.all.return_value = [removed_game]

        _mark_removed_games(session, "user1", "steam", {"730", "440"})

        assert removed_game.is_available is False

    def test_subscription_lapse_downgrades_ownership_status(self):
        from app.worker.tasks.sync.dispatch import _mark_removed_games
        session = MagicMock()

        ugp = MagicMock()
        ugp.ownership_status = OwnershipStatus.SUBSCRIPTION

        removed_sub = ExternalGameModel(
            id="eg1", user_id="user1", storefront="playstation-store",
            external_id="PPSA001", title="PS Plus Game",
            is_available=True, is_subscription=True,
        )
        removed_sub.user_game_platforms = [ugp]
        session.exec.return_value.all.return_value = [removed_sub]

        _mark_removed_games(session, "user1", "playstation-store", set())

        assert ugp.ownership_status == OwnershipStatus.NO_LONGER_OWNED

    def test_owned_game_not_downgraded_on_subscription_lapse(self):
        from app.worker.tasks.sync.dispatch import _mark_removed_games
        session = MagicMock()

        ugp = MagicMock()
        ugp.ownership_status = OwnershipStatus.OWNED

        removed_sub = ExternalGameModel(
            id="eg1", user_id="user1", storefront="playstation-store",
            external_id="PPSA001", title="Owned Game",
            is_available=True, is_subscription=True,
        )
        removed_sub.user_game_platforms = [ugp]
        session.exec.return_value.all.return_value = [removed_sub]

        _mark_removed_games(session, "user1", "playstation-store", set())

        assert ugp.ownership_status == OwnershipStatus.OWNED

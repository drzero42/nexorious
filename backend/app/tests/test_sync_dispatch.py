"""Tests for sync dispatch task."""

import pytest
import json
from unittest.mock import AsyncMock, MagicMock, patch

from app.worker.tasks.sync.dispatch import dispatch_sync_items, _create_job_item
from app.worker.tasks.sync.adapters import ExternalGame
from app.models.job import (
    Job,
    BackgroundJobStatus,
    BackgroundJobPriority,
)
from app.models.user_game import OwnershipStatus


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

        job_item = _create_job_item(session, job, "user123", game, "ext_game_123")

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
        assert metadata["external_game_id"] == "ext_game_123"

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

        # Mock session.exec to return empty result (no existing item)
        mock_result = MagicMock()
        mock_result.first.return_value = None
        session.exec.return_value = mock_result

        def mock_refresh(item):
            item.id = "item456"

        session.refresh = mock_refresh

        job_item = _create_job_item(session, job, "user456", game, "ext_game_456")

        assert job_item is not None
        assert job_item.job_id == "job456"
        assert job_item.user_id == "user456"
        assert job_item.item_key == "gog_99999"
        assert job_item.source_title == "Another Game"

        metadata = json.loads(job_item.source_metadata_json)
        assert metadata["metadata"] == {}
        assert metadata["external_game_id"] == "ext_game_456"

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

        # Mock session.exec to return empty result (no existing item)
        mock_result = MagicMock()
        mock_result.first.return_value = None
        session.exec.return_value = mock_result

        session.refresh = MagicMock()

        job_item = _create_job_item(session, job, "user789", game, "ext_game_789")

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
            ExternalGame("1", "Game 1", "pc-windows", "steam", {}),
            ExternalGame("2", "Game 2", "pc-windows", "steam", {}),
        ]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch(
                "app.worker.tasks.sync.dispatch._dispatch_process_task"
            ) as mock_dispatch,
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock ExternalGameService
            mock_ext_service = MagicMock()
            mock_external_game = MagicMock()
            mock_external_game.id = "ext_game_id"
            mock_ext_service.create_or_update.return_value = mock_external_game
            mock_ext_service.mark_unavailable_except.return_value = 0
            mock_ext_service_class.return_value = mock_ext_service

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "dispatched"
            assert result["total_games"] == 2
            assert result["dispatched"] == 2
            assert mock_dispatch.call_count == 2
            assert mock_ext_service.create_or_update.call_count == 2

    @pytest.mark.asyncio
    async def test_updates_job_status_to_processing(self):
        """Test job status is updated to PROCESSING when dispatch starts."""
        mock_games = [ExternalGame("1", "Game 1", "pc-windows", "steam", {})]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task"),
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock ExternalGameService
            mock_ext_service = MagicMock()
            mock_external_game = MagicMock()
            mock_external_game.id = "ext_game_id"
            mock_ext_service.create_or_update.return_value = mock_external_game
            mock_ext_service.mark_unavailable_except.return_value = 0
            mock_ext_service_class.return_value = mock_ext_service

            await dispatch_sync_items("job123", "user123", "steam")

            # Verify job status was set to PROCESSING
            assert mock_job.status == BackgroundJobStatus.PROCESSING

    @pytest.mark.asyncio
    async def test_updates_job_total_items(self):
        """Test job total_items is updated with number of games."""
        mock_games = [
            ExternalGame("1", "Game 1", "pc-windows", "steam", {}),
            ExternalGame("2", "Game 2", "pc-windows", "steam", {}),
            ExternalGame("3", "Game 3", "pc-windows", "steam", {}),
        ]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task"),
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock ExternalGameService
            mock_ext_service = MagicMock()
            mock_external_game = MagicMock()
            mock_external_game.id = "ext_game_id"
            mock_ext_service.create_or_update.return_value = mock_external_game
            mock_ext_service.mark_unavailable_except.return_value = 0
            mock_ext_service_class.return_value = mock_ext_service

            await dispatch_sync_items("job123", "user123", "steam")

            # Verify total_items was set
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
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock ExternalGameService
            mock_ext_service = MagicMock()
            mock_ext_service.mark_unavailable_except.return_value = 0
            mock_ext_service_class.return_value = mock_ext_service

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "dispatched"
            assert result["total_games"] == 0
            assert result["dispatched"] == 0
            assert mock_dispatch.call_count == 0

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
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock session.exec to return empty result (no existing items)
            mock_exec_result = MagicMock()
            mock_exec_result.first.return_value = None
            mock_session.exec.return_value = mock_exec_result

            # Mock ExternalGameService - first succeeds, second raises error
            mock_ext_service = MagicMock()
            call_count = [0]

            def create_or_update_side_effect(**kwargs):
                call_count[0] += 1
                if call_count[0] == 1:
                    mock_external_game = MagicMock()
                    mock_external_game.id = "ext_game_1"
                    return mock_external_game
                else:
                    raise Exception("Database error")

            mock_ext_service.create_or_update.side_effect = create_or_update_side_effect
            mock_ext_service.mark_unavailable_except.return_value = 0
            mock_ext_service_class.return_value = mock_ext_service

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
            assert result["dispatched"] == 1
            assert result["errors"] == 1


class TestDispatchCreatesExternalGames:
    """Tests for ExternalGame creation during dispatch."""

    @pytest.mark.asyncio
    async def test_creates_external_games_for_each_game(self):
        """Test that dispatch creates ExternalGame records for each game."""
        mock_games = [
            ExternalGame("1", "Game 1", "pc-windows", "steam", {}, playtime_hours=10),
            ExternalGame("2", "Game 2", "pc-windows", "steam", {}, playtime_hours=20),
        ]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task") as mock_dispatch,
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock ExternalGameService
            mock_ext_service = MagicMock()
            ext_game_counter = [0]

            def create_or_update_returns_id(**kwargs):
                ext_game_counter[0] += 1
                mock_external_game = MagicMock()
                mock_external_game.id = f"ext_game_{ext_game_counter[0]}"
                return mock_external_game

            mock_ext_service.create_or_update.side_effect = create_or_update_returns_id
            mock_ext_service.mark_unavailable_except.return_value = 0
            mock_ext_service_class.return_value = mock_ext_service

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "dispatched"
            assert mock_ext_service.create_or_update.call_count == 2

            # Verify create_or_update was called with correct parameters
            calls = mock_ext_service.create_or_update.call_args_list
            assert calls[0].kwargs["user_id"] == "user123"
            assert calls[0].kwargs["storefront"] == "steam"
            assert calls[0].kwargs["external_id"] == "1"
            assert calls[0].kwargs["title"] == "Game 1"
            assert calls[0].kwargs["playtime_hours"] == 10

            assert calls[1].kwargs["external_id"] == "2"
            assert calls[1].kwargs["title"] == "Game 2"
            assert calls[1].kwargs["playtime_hours"] == 20

    @pytest.mark.asyncio
    async def test_external_game_id_passed_to_job_item(self):
        """Test that external_game_id is included in JobItem metadata."""
        from app.models.job import JobItem

        mock_games = [
            ExternalGame("1", "Game 1", "pc-windows", "steam", {}),
        ]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task") as mock_dispatch,
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock session.exec to return empty result (no existing items)
            mock_exec_result = MagicMock()
            mock_exec_result.first.return_value = None
            mock_session.exec.return_value = mock_exec_result

            # Capture created job items (only JobItem instances, not mocks)
            created_items = []

            def mock_add(item):
                if isinstance(item, JobItem):
                    created_items.append(item)

            mock_session.add.side_effect = mock_add

            # Mock refresh to set ID
            def mock_refresh(item):
                if hasattr(item, "item_key") and hasattr(item, "id") and item.id is not None:
                    pass  # JobItem already has an ID from default_factory

            mock_session.refresh = mock_refresh

            async_ctx = AsyncMock()
            async_ctx.__aenter__.return_value = mock_session
            async_ctx.__aexit__.return_value = None
            mock_ctx.return_value = async_ctx

            mock_adapter_instance = MagicMock()
            mock_adapter_instance.fetch_games = AsyncMock(return_value=mock_games)
            mock_adapter.return_value = mock_adapter_instance

            mock_dispatch.return_value = None

            # Mock ExternalGameService
            mock_ext_service = MagicMock()
            mock_external_game = MagicMock()
            mock_external_game.id = "ext_game_unique_id"
            mock_ext_service.create_or_update.return_value = mock_external_game
            mock_ext_service.mark_unavailable_except.return_value = 0
            mock_ext_service_class.return_value = mock_ext_service

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "dispatched"
            assert len(created_items) == 1

            # Verify external_game_id is in the metadata
            import json
            metadata = json.loads(created_items[0].source_metadata_json)
            assert metadata["external_game_id"] == "ext_game_unique_id"

    @pytest.mark.asyncio
    async def test_marks_unavailable_games(self):
        """Test that games not in source are marked as unavailable."""
        mock_games = [
            ExternalGame("1", "Game 1", "pc-windows", "steam", {}),
        ]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task") as mock_dispatch,
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock ExternalGameService
            mock_ext_service = MagicMock()
            mock_external_game = MagicMock()
            mock_external_game.id = "ext_game_id"
            mock_ext_service.create_or_update.return_value = mock_external_game
            mock_ext_service.mark_unavailable_except.return_value = 3  # 3 games marked unavailable
            mock_ext_service_class.return_value = mock_ext_service

            result = await dispatch_sync_items("job123", "user123", "steam")

            assert result["status"] == "dispatched"
            assert result["marked_unavailable"] == 3

            # Verify mark_unavailable_except was called with correct parameters
            mock_ext_service.mark_unavailable_except.assert_called_once_with(
                user_id="user123",
                storefront="steam",
                available_external_ids={"1"},
            )

    @pytest.mark.asyncio
    async def test_creates_external_game_with_subscription_ownership(self):
        """Test that subscription games are created with is_subscription=True."""
        mock_games = [
            ExternalGame(
                "1",
                "Subscription Game",
                "playstation-5",
                "psn",
                {},
                ownership_status=OwnershipStatus.SUBSCRIPTION,
            ),
        ]

        with (
            patch("app.worker.tasks.sync.dispatch.get_session_context") as mock_ctx,
            patch("app.worker.tasks.sync.dispatch.get_sync_adapter") as mock_adapter,
            patch("app.worker.tasks.sync.dispatch._dispatch_process_task") as mock_dispatch,
            patch("app.worker.tasks.sync.dispatch.ExternalGameService") as mock_ext_service_class,
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

            # Mock ExternalGameService
            mock_ext_service = MagicMock()
            mock_external_game = MagicMock()
            mock_external_game.id = "ext_game_id"
            mock_ext_service.create_or_update.return_value = mock_external_game
            mock_ext_service.mark_unavailable_except.return_value = 0
            mock_ext_service_class.return_value = mock_ext_service

            result = await dispatch_sync_items("job123", "user123", "psn")

            assert result["status"] == "dispatched"

            # Verify create_or_update was called with is_subscription=True
            call = mock_ext_service.create_or_update.call_args
            assert call.kwargs["ownership_status"] == OwnershipStatus.SUBSCRIPTION
            assert call.kwargs["is_subscription"] is True

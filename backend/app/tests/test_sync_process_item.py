"""Tests for sync process item task."""

import pytest
import json
from unittest.mock import MagicMock, patch

from app.worker.tasks.sync.process_item import (
    process_sync_item,
    _get_existing_sync,
    _is_ignored,
    _add_platform_association,
)
from app.models.job import JobItemStatus


class TestGetExistingSync:
    """Tests for _get_existing_sync helper."""

    def test_returns_tuple_when_synced(self):
        """Test returns (user_game_id, game_id, game_title) when platform association exists."""
        session = MagicMock()
        mock_user_game = MagicMock()
        mock_user_game.id = "ug123"
        mock_game = MagicMock()
        mock_game.id = 456
        mock_game.title = "Test Game"
        session.exec.return_value.first.return_value = (MagicMock(), mock_user_game, mock_game)

        result = _get_existing_sync(session, "user123", "steam", "12345")
        assert result == ("ug123", 456, "Test Game")

    def test_returns_none_when_not_synced(self):
        """Test returns None when no platform association."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None

        result = _get_existing_sync(session, "user123", "steam", "12345")
        assert result is None


class TestIsIgnored:
    """Tests for _is_ignored helper."""

    def test_returns_true_when_skipped(self):
        """Test returns True when ExternalGame.is_skipped is True."""
        session = MagicMock()
        mock_external_game = MagicMock()
        mock_external_game.is_skipped = True
        session.exec.return_value.first.return_value = mock_external_game

        result = _is_ignored(session, "user123", "steam", "12345")
        assert result is True

    def test_returns_false_when_not_skipped(self):
        """Test returns False when ExternalGame.is_skipped is False."""
        session = MagicMock()
        mock_external_game = MagicMock()
        mock_external_game.is_skipped = False
        session.exec.return_value.first.return_value = mock_external_game

        result = _is_ignored(session, "user123", "steam", "12345")
        assert result is False

    def test_returns_false_when_no_external_game(self):
        """Test returns False when ExternalGame not found."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None

        result = _is_ignored(session, "user123", "steam", "12345")
        assert result is False


class TestAddPlatformAssociation:
    """Tests for _add_platform_association helper."""

    def test_creates_association_when_not_exists(self):
        """Test creates new association."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None  # Not found

        _add_platform_association(session, "ug123", "pc-windows", "steam", "12345")

        session.add.assert_called_once()
        session.commit.assert_called_once()

    def test_creates_association_with_playtime(self):
        """Test creates new association with playtime."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None  # Not found

        _add_platform_association(session, "ug123", "pc-windows", "steam", "12345", 50)

        # Check the added platform has correct playtime
        call_args = session.add.call_args
        platform = call_args[0][0]
        assert platform.hours_played == 50

    def test_updates_playtime_when_association_exists(self):
        """Test updates playtime if association already exists."""
        session = MagicMock()
        existing_platform = MagicMock()
        existing_platform.hours_played = 10
        session.exec.return_value.first.return_value = existing_platform

        _add_platform_association(session, "ug123", "pc-windows", "steam", "12345", 25)

        # Should update playtime on existing platform
        assert existing_platform.hours_played == 25
        session.add.assert_called_once_with(existing_platform)
        session.commit.assert_called_once()

    def test_sets_steam_store_url(self):
        """Test sets correct store URL for Steam."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None

        _add_platform_association(session, "ug123", "pc-windows", "steam", "12345")

        # Check the added platform has correct URL
        call_args = session.add.call_args
        platform = call_args[0][0]
        assert platform.store_url == "https://store.steampowered.com/app/12345"

    def test_no_store_url_for_non_steam(self):
        """Test no store URL is set for non-Steam storefronts."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None

        _add_platform_association(session, "ug123", "pc-windows", "gog", "12345")

        call_args = session.add.call_args
        platform = call_args[0][0]
        assert platform.store_url is None

    def test_creates_association_with_external_game_id(self):
        """Test creates new association with external_game_id."""
        session = MagicMock()
        session.exec.return_value.first.return_value = None  # Not found

        _add_platform_association(
            session, "ug123", "pc-windows", "steam", "12345",
            external_game_id="ext-game-123"
        )

        # Check the added platform has external_game_id set
        call_args = session.add.call_args
        platform = call_args[0][0]
        assert platform.external_game_id == "ext-game-123"

    def test_updates_external_game_id_when_not_set(self):
        """Test updates external_game_id if not already set on existing association."""
        session = MagicMock()
        existing_platform = MagicMock()
        existing_platform.hours_played = 10
        existing_platform.external_game_id = None  # Not set
        session.exec.return_value.first.return_value = existing_platform

        _add_platform_association(
            session, "ug123", "pc-windows", "steam", "12345", 25,
            external_game_id="ext-game-123"
        )

        # Should update external_game_id
        assert existing_platform.external_game_id == "ext-game-123"

    def test_preserves_existing_external_game_id(self):
        """Test preserves existing external_game_id if already set."""
        session = MagicMock()
        existing_platform = MagicMock()
        existing_platform.hours_played = 10
        existing_platform.external_game_id = "original-ext-id"  # Already set
        session.exec.return_value.first.return_value = existing_platform

        _add_platform_association(
            session, "ug123", "pc-windows", "steam", "12345", 25,
            external_game_id="new-ext-id"
        )

        # Should NOT update external_game_id when already set
        assert existing_platform.external_game_id == "original-ext-id"


class TestProcessSyncItem:
    """Tests for process_sync_item task."""

    @pytest.mark.asyncio
    async def test_returns_error_when_job_item_not_found(self):
        """Test returns error when job item doesn't exist."""
        with patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session:
            mock_session = MagicMock()
            mock_session.get.return_value = None
            mock_get_session.return_value = mock_session

            result = await process_sync_item("item123")

            assert result["status"] == "error"
            assert "JobItem not found" in result["error"]

    @pytest.mark.asyncio
    async def test_skips_already_processed_items(self):
        """Test skips items not in PENDING/PROCESSING status."""
        with patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session:
            mock_session = MagicMock()
            mock_item = MagicMock()
            mock_item.status = JobItemStatus.COMPLETED
            mock_session.get.return_value = mock_item
            mock_get_session.return_value = mock_session

            result = await process_sync_item("item123")

            assert result["status"] == "skipped"
            assert result["reason"] == "already_processed"

    @pytest.mark.asyncio
    async def test_marks_already_synced_as_completed(self):
        """Test marks already synced items as COMPLETED."""
        with (
            patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session,
            patch("app.worker.tasks.sync.process_item._get_existing_sync") as mock_synced,
            patch("app.worker.tasks.sync.process_item._complete_job_item") as mock_complete,
        ):
            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = None
            mock_item.source_metadata_json = json.dumps({
                "external_id": "12345",
                "storefront": "steam",
                "platform": "pc-windows",
            })
            mock_item.source_title = "Test Game"

            mock_session.get.return_value = mock_item
            mock_synced.return_value = ("ug123", 456, "Test Game")
            mock_complete.return_value = {"status": "success", "result": "already_synced"}

            result = await process_sync_item("item123")

            mock_complete.assert_called_once()
            assert result["result"] == "already_synced"

    @pytest.mark.asyncio
    async def test_marks_ignored_as_skipped(self):
        """Test marks ignored items as SKIPPED."""
        with (
            patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session,
            patch("app.worker.tasks.sync.process_item._get_existing_sync") as mock_synced,
            patch("app.worker.tasks.sync.process_item._is_ignored") as mock_ignored,
            patch("app.worker.tasks.sync.process_item._complete_job_item") as mock_complete,
        ):
            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = None
            mock_item.source_metadata_json = json.dumps({
                "external_id": "12345",
                "storefront": "steam",
                "platform": "pc-windows",
            })
            mock_item.source_title = "Test Game"

            mock_session.get.return_value = mock_item
            mock_synced.return_value = None
            mock_ignored.return_value = True
            mock_complete.return_value = {"status": "success", "result": "ignored"}

            result = await process_sync_item("item123")

            mock_complete.assert_called_once()
            assert result["result"] == "ignored"

    @pytest.mark.asyncio
    async def test_returns_error_for_invalid_json(self):
        """Test returns error when metadata JSON is invalid."""
        with (
            patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session,
            patch("app.worker.tasks.sync.process_item._update_job_item_error") as mock_error,
        ):
            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = None
            mock_item.source_metadata_json = "invalid json{"
            mock_item.source_title = "Test Game"

            mock_session.get.return_value = mock_item
            mock_error.return_value = {"status": "error", "error": "Invalid metadata"}

            result = await process_sync_item("item123")

            mock_error.assert_called_once()
            assert result["status"] == "error"

    @pytest.mark.asyncio
    async def test_returns_error_for_missing_external_id(self):
        """Test returns error when external_id is missing."""
        with (
            patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session,
            patch("app.worker.tasks.sync.process_item._update_job_item_error") as mock_error,
        ):
            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = None
            mock_item.source_metadata_json = json.dumps({
                "storefront": "steam",
                "platform": "pc-windows",
            })
            mock_item.source_title = "Test Game"

            mock_session.get.return_value = mock_item
            mock_error.return_value = {"status": "error", "error": "Missing external_id"}

            result = await process_sync_item("item123")

            mock_error.assert_called_once()
            assert result["status"] == "error"

    @pytest.mark.asyncio
    async def test_processes_with_resolved_igdb_id(self):
        """Test processes items with user-provided IGDB ID."""
        with (
            patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session,
            patch("app.worker.tasks.sync.process_item._get_existing_sync") as mock_synced,
            patch("app.worker.tasks.sync.process_item._is_ignored") as mock_ignored,
            patch("app.worker.tasks.sync.process_item._process_with_resolved_id") as mock_process,
        ):
            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = 12345
            mock_item.source_metadata_json = json.dumps({
                "external_id": "steam123",
                "storefront": "steam",
                "platform": "pc-windows",
            })
            mock_item.source_title = "Test Game"

            mock_session.get.return_value = mock_item
            mock_synced.return_value = None
            mock_ignored.return_value = False
            mock_process.return_value = {"status": "success", "result": "imported_new"}

            result = await process_sync_item("item123")

            mock_process.assert_called_once()
            assert result["result"] == "imported_new"

    @pytest.mark.asyncio
    async def test_processes_with_matching_when_no_resolved_id(self):
        """Test falls back to IGDB matching when no resolved_igdb_id."""
        with (
            patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session,
            patch("app.worker.tasks.sync.process_item._get_existing_sync") as mock_synced,
            patch("app.worker.tasks.sync.process_item._is_ignored") as mock_ignored,
            patch("app.worker.tasks.sync.process_item._process_with_matching") as mock_match,
        ):
            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = None
            mock_item.source_metadata_json = json.dumps({
                "external_id": "steam123",
                "storefront": "steam",
                "platform": "pc-windows",
            })
            mock_item.source_title = "Test Game"

            mock_session.get.return_value = mock_item
            mock_synced.return_value = None
            mock_ignored.return_value = False
            mock_match.return_value = {"status": "success", "result": "auto_imported"}

            result = await process_sync_item("item123")

            mock_match.assert_called_once()
            assert result["result"] == "auto_imported"

    @pytest.mark.asyncio
    async def test_updates_status_to_processing(self):
        """Test job item status is updated to PROCESSING."""
        with (
            patch("app.worker.tasks.sync.process_item.get_sync_session") as mock_get_session,
            patch("app.worker.tasks.sync.process_item._get_existing_sync") as mock_synced,
            patch("app.worker.tasks.sync.process_item._complete_job_item") as mock_complete,
        ):
            mock_session = MagicMock()
            mock_get_session.return_value = mock_session

            mock_item = MagicMock()
            mock_item.status = JobItemStatus.PENDING
            mock_item.user_id = "user123"
            mock_item.job_id = "job123"
            mock_item.resolved_igdb_id = None
            mock_item.source_metadata_json = json.dumps({
                "external_id": "12345",
                "storefront": "steam",
                "platform": "pc-windows",
            })
            mock_item.source_title = "Test Game"

            mock_session.get.return_value = mock_item
            mock_synced.return_value = ("ug123", 456, "Test Game")
            mock_complete.return_value = {"status": "success", "result": "already_synced"}

            await process_sync_item("item123")

            # Verify status was set to PROCESSING before processing
            assert mock_item.status == JobItemStatus.PROCESSING

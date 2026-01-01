"""Tests for Epic sync adapter."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.worker.tasks.sync.adapters.epic import EpicSyncAdapter
from app.models.job import BackgroundJobSource
from app.services.epic import EpicAuthExpiredError


class TestEpicSyncAdapter:
    """Tests for EpicSyncAdapter."""

    def test_source_is_epic(self):
        """Test adapter has correct source."""
        adapter = EpicSyncAdapter()
        assert adapter.source == BackgroundJobSource.EPIC

    @pytest.mark.asyncio
    async def test_fetch_games_success(self):
        """Test fetch_games returns ExternalGame list on success."""
        adapter = EpicSyncAdapter()
        user = MagicMock()
        user.id = "user123"
        user.preferences = {
            "epic": {
                "account_id": "epic-account-123",
                "display_name": "TestUser",
                "is_verified": True,
            }
        }

        # Mock session
        mock_session = MagicMock()

        # Mock Epic game response
        mock_epic_game = MagicMock()
        mock_epic_game.app_name = "CypressGame"
        mock_epic_game.title = "Cypress Test Game"

        with patch("app.worker.tasks.sync.adapters.epic.EpicService") as mock_service:
            mock_instance = AsyncMock()
            mock_instance.get_library = AsyncMock(return_value=[mock_epic_game])
            mock_service.return_value = mock_instance

            games = await adapter.fetch_games(user, mock_session)

        assert len(games) == 1
        assert games[0].external_id == "CypressGame"
        assert games[0].title == "Cypress Test Game"
        assert games[0].platform == "pc-windows"
        assert games[0].storefront == "epic-games-store"
        assert games[0].metadata["app_name"] == "CypressGame"
        assert games[0].playtime_hours == 0

    @pytest.mark.asyncio
    async def test_fetch_games_auth_expired(self):
        """Test fetch_games propagates EpicAuthExpiredError."""
        adapter = EpicSyncAdapter()
        user = MagicMock()
        user.id = "user123"
        user.preferences = {
            "epic": {
                "account_id": "epic-account-123",
                "display_name": "TestUser",
                "is_verified": True,
            }
        }

        # Mock session
        mock_session = MagicMock()

        with patch("app.worker.tasks.sync.adapters.epic.EpicService") as mock_service:
            mock_instance = AsyncMock()
            mock_instance.get_library = AsyncMock(side_effect=EpicAuthExpiredError("Token expired"))
            mock_service.return_value = mock_instance

            with pytest.raises(EpicAuthExpiredError, match="Token expired"):
                await adapter.fetch_games(user, mock_session)

    @pytest.mark.asyncio
    async def test_fetch_games_not_configured(self):
        """Test fetch_games raises ValueError when Epic not configured."""
        adapter = EpicSyncAdapter()
        user = MagicMock()
        user.id = "user123"
        user.preferences = {}

        # Mock session
        mock_session = MagicMock()

        with pytest.raises(ValueError, match="Epic credentials not configured"):
            await adapter.fetch_games(user, mock_session)

    def test_get_credentials_configured(self):
        """Test get_credentials returns epic config when is_verified=True."""
        adapter = EpicSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "epic": {
                "account_id": "epic-account-123",
                "display_name": "TestUser",
                "is_verified": True,
            }
        }

        creds = adapter.get_credentials(user)
        assert creds == {
            "account_id": "epic-account-123",
            "display_name": "TestUser",
            "is_verified": True,
        }

    def test_get_credentials_not_configured(self):
        """Test get_credentials returns None when not verified."""
        adapter = EpicSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "epic": {
                "account_id": "epic-account-123",
                "display_name": "TestUser",
                "is_verified": False,
            }
        }

        assert adapter.get_credentials(user) is None

    def test_is_configured_true(self):
        """Test is_configured returns True for verified user."""
        adapter = EpicSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "epic": {
                "account_id": "epic-account-123",
                "display_name": "TestUser",
                "is_verified": True,
            }
        }

        assert adapter.is_configured(user) is True

    def test_is_configured_false(self):
        """Test is_configured returns False for unverified user."""
        adapter = EpicSyncAdapter()
        user = MagicMock()
        user.preferences = {}

        assert adapter.is_configured(user) is False

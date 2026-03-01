"""Tests for sync source adapters."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.worker.tasks.sync.adapters import ExternalLibraryEntry, get_sync_adapter
from app.worker.tasks.sync.adapters.steam import SteamSyncAdapter
from app.models.job import BackgroundJobSource


class TestExternalLibraryEntry:
    """Tests for ExternalLibraryEntry dataclass."""

    def test_external_game_creation(self):
        """Test creating an ExternalLibraryEntry."""
        game = ExternalLibraryEntry(
            external_id="12345",
            title="Test Game",
            platform="pc-windows",
            storefront="steam",
            metadata={"appid": 12345},
        )

        assert game.external_id == "12345"
        assert game.title == "Test Game"
        assert game.platform == "pc-windows"
        assert game.storefront == "steam"
        assert game.metadata == {"appid": 12345}


class TestGetSyncAdapter:
    """Tests for get_sync_adapter factory."""

    def test_get_steam_adapter(self):
        """Test getting Steam adapter."""
        adapter = get_sync_adapter("steam")
        assert isinstance(adapter, SteamSyncAdapter)
        assert adapter.source == BackgroundJobSource.STEAM

    def test_get_steam_adapter_case_insensitive(self):
        """Test adapter lookup is case insensitive."""
        adapter = get_sync_adapter("STEAM")
        assert isinstance(adapter, SteamSyncAdapter)

    def test_get_epic_adapter(self):
        """Test getting Epic adapter."""
        from app.worker.tasks.sync.adapters.epic import EpicSyncAdapter
        adapter = get_sync_adapter("epic")
        assert isinstance(adapter, EpicSyncAdapter)
        assert adapter.source == BackgroundJobSource.EPIC

    def test_get_epic_adapter_case_insensitive(self):
        """Test Epic adapter lookup is case insensitive."""
        from app.worker.tasks.sync.adapters.epic import EpicSyncAdapter
        adapter = get_sync_adapter("EPIC")
        assert isinstance(adapter, EpicSyncAdapter)

    def test_get_unsupported_adapter_raises(self):
        """Test that unsupported source raises ValueError."""
        with pytest.raises(ValueError, match="Unsupported sync source"):
            get_sync_adapter("unsupported")


class TestSteamSyncAdapter:
    """Tests for SteamSyncAdapter."""

    def test_source_is_steam(self):
        """Test adapter has correct source."""
        adapter = SteamSyncAdapter()
        assert adapter.source == BackgroundJobSource.STEAM

    def test_get_credentials_returns_none_when_not_configured(self):
        """Test credentials return None when not configured."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {}

        assert adapter.get_credentials(user) is None

    def test_get_credentials_returns_none_when_preferences_none(self):
        """Test credentials return None when preferences is None."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = None

        assert adapter.get_credentials(user) is None

    def test_get_credentials_returns_none_when_not_verified(self):
        """Test credentials return None when not verified."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": False,
            }
        }

        assert adapter.get_credentials(user) is None

    def test_get_credentials_returns_none_when_missing_api_key(self):
        """Test credentials return None when API key is missing."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "steam": {
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        assert adapter.get_credentials(user) is None

    def test_get_credentials_returns_none_when_missing_steam_id(self):
        """Test credentials return None when Steam ID is missing."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "is_verified": True,
            }
        }

        assert adapter.get_credentials(user) is None

    def test_get_credentials_returns_credentials_when_valid(self):
        """Test credentials return dict when valid."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        creds = adapter.get_credentials(user)
        assert creds == {"api_key": "test_key", "steam_id": "12345678901234567"}

    def test_is_configured_true_when_credentials_valid(self):
        """Test is_configured returns True when credentials are valid."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        assert adapter.is_configured(user) is True

    def test_is_configured_false_when_not_configured(self):
        """Test is_configured returns False when not configured."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {}

        assert adapter.is_configured(user) is False

    @pytest.mark.asyncio
    async def test_fetch_games_raises_when_not_configured(self):
        """Test fetch_games raises when credentials not configured."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.preferences = {}

        # Mock session
        mock_session = MagicMock()

        with pytest.raises(ValueError, match="Steam credentials not configured"):
            await adapter.fetch_games(user, mock_session)

    @pytest.mark.asyncio
    async def test_fetch_games_returns_external_games(self):
        """Test fetch_games returns list of ExternalLibraryEntry objects."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.id = "user123"
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        # Mock session
        mock_session = MagicMock()

        # Mock Steam game response
        mock_steam_game = MagicMock()
        mock_steam_game.appid = 12345
        mock_steam_game.name = "Test Game"

        with patch("app.worker.tasks.sync.adapters.steam.SteamService") as mock_service:
            mock_instance = AsyncMock()
            mock_instance.get_owned_games = AsyncMock(return_value=[mock_steam_game])
            mock_service.return_value = mock_instance

            games = await adapter.fetch_games(user, mock_session)

        assert len(games) == 1
        assert games[0].external_id == "12345"
        assert games[0].title == "Test Game"
        assert games[0].platform == "pc-windows"
        assert games[0].storefront == "steam"
        assert games[0].metadata["appid"] == 12345

    @pytest.mark.asyncio
    async def test_fetch_games_returns_empty_list_when_no_games(self):
        """Test fetch_games returns empty list when user has no games."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.id = "user123"
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        # Mock session
        mock_session = MagicMock()

        with patch("app.worker.tasks.sync.adapters.steam.SteamService") as mock_service:
            mock_instance = AsyncMock()
            mock_instance.get_owned_games = AsyncMock(return_value=[])
            mock_service.return_value = mock_instance

            games = await adapter.fetch_games(user, mock_session)

        assert games == []

    @pytest.mark.asyncio
    async def test_fetch_games_handles_multiple_games(self):
        """Test fetch_games handles multiple games correctly."""
        adapter = SteamSyncAdapter()
        user = MagicMock()
        user.id = "user123"
        user.preferences = {
            "steam": {
                "web_api_key": "test_key",
                "steam_id": "12345678901234567",
                "is_verified": True,
            }
        }

        # Mock session
        mock_session = MagicMock()

        # Mock multiple Steam games
        mock_game1 = MagicMock()
        mock_game1.appid = 111
        mock_game1.name = "Game One"

        mock_game2 = MagicMock()
        mock_game2.appid = 222
        mock_game2.name = "Game Two"

        mock_game3 = MagicMock()
        mock_game3.appid = 333
        mock_game3.name = "Game Three"

        with patch("app.worker.tasks.sync.adapters.steam.SteamService") as mock_service:
            mock_instance = AsyncMock()
            mock_instance.get_owned_games = AsyncMock(
                return_value=[mock_game1, mock_game2, mock_game3]
            )
            mock_service.return_value = mock_instance

            games = await adapter.fetch_games(user, mock_session)

        assert len(games) == 3
        assert games[0].external_id == "111"
        assert games[0].title == "Game One"
        assert games[1].external_id == "222"
        assert games[1].title == "Game Two"
        assert games[2].external_id == "333"
        assert games[2].title == "Game Three"

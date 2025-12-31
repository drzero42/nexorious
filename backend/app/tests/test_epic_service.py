"""Tests for Epic Games Store service using legendary Python library."""

import pytest
from unittest.mock import MagicMock, patch

from app.services.epic import (
    EpicService,
    EpicAccountInfo,
    EpicAuthenticationError,
    EpicAuthExpiredError,
    EpicAPIError,
    EpicGame,
)


class TestEpicService:
    """Test EpicService initialization and setup."""

    @patch('app.services.epic.LegendaryCore')
    def test_service_initialization(self, mock_legendary_core):
        """Test EpicService creates with correct user config path."""
        user_id = "test-user-123"
        service = EpicService(user_id)

        assert service.user_id == user_id
        assert service.config_path == f"/var/lib/nexorious/legendary-configs/{user_id}"
        # Verify LegendaryCore was initialized
        mock_legendary_core.assert_called_once()

    @patch('app.services.epic.LegendaryCore')
    def test_service_initialization_failure(self, mock_legendary_core):
        """Test EpicService handles LegendaryCore initialization failure."""
        mock_legendary_core.side_effect = Exception("Initialization failed")

        with pytest.raises(EpicAPIError, match="Failed to initialize Epic service"):
            EpicService("test-user")


class TestEpicAuthentication:
    """Test Epic authentication flow."""

    @patch('app.services.epic.LegendaryCore')
    def test_get_auth_url(self, mock_legendary_core):
        """Test getting Epic OAuth URL."""
        mock_core_instance = MagicMock()
        mock_core_instance.egs.get_auth_url.return_value = "https://epicgames.com/auth"
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")
        url = service.get_auth_url()

        assert url == "https://epicgames.com/auth"
        mock_core_instance.egs.get_auth_url.assert_called_once()

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_start_device_auth(self, mock_legendary_core):
        """Test starting device auth returns OAuth URL."""
        mock_core_instance = MagicMock()
        mock_core_instance.egs.get_auth_url.return_value = "https://epicgames.com/auth"
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")
        url = await service.start_device_auth()

        assert url == "https://epicgames.com/auth"

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_complete_auth_success(self, mock_legendary_core):
        """Test completing authentication with valid code."""
        mock_core_instance = MagicMock()
        mock_core_instance.auth_code.return_value = True
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")
        result = await service.complete_auth("valid-code")

        assert result is True
        mock_core_instance.auth_code.assert_called_once_with("valid-code")

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_complete_auth_failure(self, mock_legendary_core):
        """Test completing authentication with invalid code."""
        mock_core_instance = MagicMock()
        mock_core_instance.auth_code.return_value = False
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")

        with pytest.raises(EpicAuthenticationError):
            await service.complete_auth("invalid-code")

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_verify_auth_authenticated(self, mock_legendary_core):
        """Test verify_auth returns True when authenticated."""
        mock_core_instance = MagicMock()
        mock_core_instance.login.return_value = True
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")
        result = await service.verify_auth()

        assert result is True

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_verify_auth_not_authenticated(self, mock_legendary_core):
        """Test verify_auth returns False when not authenticated."""
        mock_core_instance = MagicMock()
        mock_core_instance.login.return_value = False
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")
        result = await service.verify_auth()

        assert result is False


class TestEpicAccountInfo:
    """Test Epic account information retrieval."""

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_get_account_info_success(self, mock_legendary_core):
        """Test getting account info when authenticated."""
        mock_core_instance = MagicMock()
        mock_core_instance.login.return_value = True
        mock_core_instance.egs.user = {
            'displayName': 'TestUser',
            'account_id': 'test-account-123'
        }
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")
        info = await service.get_account_info()

        assert isinstance(info, EpicAccountInfo)
        assert info.display_name == 'TestUser'
        assert info.account_id == 'test-account-123'

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_get_account_info_not_authenticated(self, mock_legendary_core):
        """Test getting account info when not authenticated."""
        mock_core_instance = MagicMock()
        mock_core_instance.login.return_value = False
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")

        with pytest.raises(EpicAuthExpiredError):
            await service.get_account_info()


class TestEpicLibrary:
    """Test Epic library retrieval."""

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_get_library_success(self, mock_legendary_core):
        """Test getting library when authenticated."""
        mock_game = MagicMock()
        mock_game.app_name = 'TestGame'
        mock_game.app_title = 'Test Game'
        mock_game.namespace = 'test-namespace'
        mock_game.catalog_item_id = 'test-catalog-id'

        mock_core_instance = MagicMock()
        mock_core_instance.login.return_value = True
        mock_core_instance.get_game_list.return_value = [mock_game]
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")
        games = await service.get_library()

        assert len(games) == 1
        assert isinstance(games[0], EpicGame)
        assert games[0].app_name == 'TestGame'
        assert games[0].title == 'Test Game'
        mock_core_instance.get_game_list.assert_called_once_with(update_assets=True)

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_get_library_not_authenticated(self, mock_legendary_core):
        """Test getting library when not authenticated."""
        mock_core_instance = MagicMock()
        mock_core_instance.login.return_value = False
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")

        with pytest.raises(EpicAuthExpiredError):
            await service.get_library()


class TestEpicDisconnect:
    """Test Epic account disconnection."""

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_disconnect_success(self, mock_legendary_core):
        """Test disconnecting Epic account."""
        mock_core_instance = MagicMock()
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")
        await service.disconnect()

        mock_core_instance.lgd.invalidate_userdata.assert_called_once()

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_disconnect_failure(self, mock_legendary_core):
        """Test disconnect handles errors."""
        mock_core_instance = MagicMock()
        mock_core_instance.lgd.invalidate_userdata.side_effect = Exception("Disconnect failed")
        mock_legendary_core.return_value = mock_core_instance

        service = EpicService("test-user")

        with pytest.raises(EpicAPIError, match="Failed to disconnect Epic account"):
            await service.disconnect()

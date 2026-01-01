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

    @patch('app.services.epic.LegendaryCore')
    def test_get_user_json_path(self, mock_legendary_core):
        """Test _get_user_json_path returns correct path."""
        service = EpicService("test-user-123")

        expected_path = "/var/lib/nexorious/legendary-configs/test-user-123/legendary/user.json"
        assert service._get_user_json_path() == expected_path


class TestEpicCredentialStorage:
    """Test Epic credential storage and loading from database."""

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_load_credentials_from_db_success(self, mock_legendary_core):
        """Test _load_credentials_from_db loads credentials from DB and writes to filesystem."""
        from unittest.mock import MagicMock, patch, mock_open
        from app.models.user_sync_config import UserSyncConfig

        # Setup mock session with Epic credentials
        mock_config = UserSyncConfig(
            user_id="test-user",
            platform="epic",
            platform_credentials='{"displayName": "TestUser", "account_id": "test-account-123"}'
        )

        mock_result = MagicMock()
        mock_result.first.return_value = mock_config

        mock_session = MagicMock()
        mock_session.exec.return_value = mock_result

        service = EpicService("test-user")

        # Mock filesystem operations
        with patch('builtins.open', mock_open()) as mock_file, \
             patch('os.makedirs') as mock_makedirs, \
             patch('json.dump') as mock_json_dump:

            await service._load_credentials_from_db(mock_session)

            # Verify database query was made
            mock_session.exec.assert_called_once()

            # Verify directory creation was called
            expected_dir = "/var/lib/nexorious/legendary-configs/test-user/legendary"
            mock_makedirs.assert_called_once_with(expected_dir, exist_ok=True)

            # Verify file open was called
            expected_path = "/var/lib/nexorious/legendary-configs/test-user/legendary/user.json"
            mock_file.assert_called_once_with(expected_path, 'w')

            # Verify json.dump was called with parsed credentials
            mock_json_dump.assert_called_once()
            call_args = mock_json_dump.call_args
            written_data = call_args[0][0]
            assert written_data == {"displayName": "TestUser", "account_id": "test-account-123"}

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_load_credentials_from_db_no_config(self, mock_legendary_core):
        """Test _load_credentials_from_db handles missing config gracefully."""
        from unittest.mock import MagicMock, patch

        # Setup mock session with no config found
        mock_result = MagicMock()
        mock_result.first.return_value = None

        mock_session = MagicMock()
        mock_session.exec.return_value = mock_result

        service = EpicService("test-user")

        # Mock filesystem operations
        with patch('builtins.open') as mock_file, \
             patch('os.makedirs') as mock_makedirs:

            await service._load_credentials_from_db(mock_session)

            # Verify database query was made
            mock_session.exec.assert_called_once()

            # Verify no filesystem operations occurred
            mock_makedirs.assert_not_called()
            mock_file.assert_not_called()

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_load_credentials_from_db_empty_credentials(self, mock_legendary_core):
        """Test _load_credentials_from_db handles None credentials gracefully."""
        from unittest.mock import MagicMock, patch
        from app.models.user_sync_config import UserSyncConfig

        # Setup mock session with config but no credentials
        mock_config = UserSyncConfig(
            user_id="test-user",
            platform="epic",
            platform_credentials=None
        )

        mock_result = MagicMock()
        mock_result.first.return_value = mock_config

        mock_session = MagicMock()
        mock_session.exec.return_value = mock_result

        service = EpicService("test-user")

        # Mock filesystem operations
        with patch('builtins.open') as mock_file, \
             patch('os.makedirs') as mock_makedirs:

            await service._load_credentials_from_db(mock_session)

            # Verify database query was made
            mock_session.exec.assert_called_once()

            # Verify no filesystem operations occurred
            mock_makedirs.assert_not_called()
            mock_file.assert_not_called()

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_load_credentials_from_db_malformed_json(self, mock_legendary_core):
        """Test _load_credentials_from_db with malformed JSON raises EpicAPIError."""
        from unittest.mock import MagicMock
        from app.models.user_sync_config import UserSyncConfig

        # Setup mock session with malformed JSON credentials
        mock_config = UserSyncConfig(
            user_id="test-user",
            platform="epic",
            platform_credentials="{invalid json here"
        )

        mock_result = MagicMock()
        mock_result.first.return_value = mock_config

        mock_session = MagicMock()
        mock_session.exec.return_value = mock_result

        service = EpicService("test-user")

        # Verify that malformed JSON raises EpicAPIError with "corrupted" in message
        with pytest.raises(EpicAPIError, match="corrupted"):
            await service._load_credentials_from_db(mock_session)

        # Verify database query was made
        mock_session.exec.assert_called_once()

    @patch('app.services.epic.LegendaryCore')
    @pytest.mark.asyncio
    async def test_load_credentials_from_db_filesystem_error(self, mock_legendary_core):
        """Test _load_credentials_from_db with filesystem error raises EpicAPIError."""
        from unittest.mock import MagicMock, patch
        from app.models.user_sync_config import UserSyncConfig

        # Setup mock session with valid JSON credentials
        mock_config = UserSyncConfig(
            user_id="test-user",
            platform="epic",
            platform_credentials='{"displayName": "TestUser", "account_id": "test-account-123"}'
        )

        mock_result = MagicMock()
        mock_result.first.return_value = mock_config

        mock_session = MagicMock()
        mock_session.exec.return_value = mock_result

        service = EpicService("test-user")

        # Mock filesystem operations to fail
        with patch('os.makedirs', side_effect=OSError("Permission denied")):
            # Verify that filesystem error raises EpicAPIError
            with pytest.raises(EpicAPIError, match="Failed to store Epic credentials"):
                await service._load_credentials_from_db(mock_session)


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

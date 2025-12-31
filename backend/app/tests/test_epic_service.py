"""Tests for Epic Games Store service using legendary CLI."""

import asyncio
from unittest.mock import AsyncMock, patch, MagicMock

import pytest

from app.services.epic import (
    EpicService,
    EpicAccountInfo,
    LegendaryNotFoundError,
    EpicAuthenticationError,
    EpicAuthExpiredError,
    EpicAPIError,
)


class TestEpicService:
    """Test EpicService initialization and config path setup."""

    def test_service_initialization(self):
        """Test EpicService creates with correct user config path."""
        user_id = "test-user-123"
        service = EpicService(user_id)

        assert service.user_id == user_id
        assert service.config_path == f"/var/lib/nexorious/legendary-configs/{user_id}"


class TestEpicExceptions:
    """Test Epic service custom exceptions."""

    def test_legendary_not_found_error(self):
        """Test LegendaryNotFoundError can be raised and caught."""
        with pytest.raises(LegendaryNotFoundError) as exc_info:
            raise LegendaryNotFoundError("legendary not found")
        assert "legendary not found" in str(exc_info.value)

    def test_epic_authentication_error(self):
        """Test EpicAuthenticationError can be raised and caught."""
        with pytest.raises(EpicAuthenticationError) as exc_info:
            raise EpicAuthenticationError("auth failed")
        assert "auth failed" in str(exc_info.value)

    def test_epic_auth_expired_error(self):
        """Test EpicAuthExpiredError can be raised and caught."""
        with pytest.raises(EpicAuthExpiredError) as exc_info:
            raise EpicAuthExpiredError("auth expired")
        assert "auth expired" in str(exc_info.value)

    def test_epic_api_error(self):
        """Test EpicAPIError can be raised and caught."""
        with pytest.raises(EpicAPIError) as exc_info:
            raise EpicAPIError("api error")
        assert "api error" in str(exc_info.value)


class TestLegendarySubprocess:
    """Test legendary subprocess execution."""

    @pytest.mark.asyncio
    async def test_run_legendary_command_success(self):
        """Test successful legendary command execution."""
        service = EpicService("test-user")

        # Mock successful subprocess
        mock_process = MagicMock()
        mock_process.returncode = 0
        mock_process.stdout = b'{"status": "ok"}'
        mock_process.stderr = b''

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process
            mock_process.communicate = AsyncMock(return_value=(b'{"status": "ok"}', b''))

            result = await service._run_legendary_command(["status", "--json"])

            assert result == {"stdout": '{"status": "ok"}', "stderr": "", "returncode": 0}

            # Verify XDG_CONFIG_HOME was set
            call_args = mock_exec.call_args
            assert "XDG_CONFIG_HOME" in call_args[1]["env"]
            assert call_args[1]["env"]["XDG_CONFIG_HOME"] == service.config_path

    @pytest.mark.asyncio
    async def test_run_legendary_command_not_found(self):
        """Test legendary not found error."""
        service = EpicService("test-user")

        with patch('asyncio.create_subprocess_exec', side_effect=FileNotFoundError()):
            with pytest.raises(LegendaryNotFoundError) as exc_info:
                await service._run_legendary_command(["status"])

            assert "legendary CLI not found" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_run_legendary_command_auth_expired(self):
        """Test detection of expired authentication."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 1
        mock_process.communicate = AsyncMock(
            return_value=(b'', b'You are not authenticated')
        )

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            with pytest.raises(EpicAuthExpiredError) as exc_info:
                await service._run_legendary_command(["list"])

            assert "authentication expired" in str(exc_info.value).lower()

    @pytest.mark.asyncio
    async def test_run_legendary_command_generic_error(self):
        """Test generic command failure."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.returncode = 1
        mock_process.communicate = AsyncMock(
            return_value=(b'', b'Some other error')
        )

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            with pytest.raises(EpicAPIError) as exc_info:
                await service._run_legendary_command(["list"])

            assert "legendary command failed" in str(exc_info.value)

    @pytest.mark.asyncio
    async def test_run_legendary_command_timeout(self):
        """Test command timeout handling."""
        service = EpicService("test-user")

        mock_process = MagicMock()
        mock_process.communicate = AsyncMock(
            side_effect=asyncio.TimeoutError()
        )

        with patch('asyncio.create_subprocess_exec', new_callable=AsyncMock) as mock_exec:
            mock_exec.return_value = mock_process

            with pytest.raises(EpicAPIError) as exc_info:
                await service._run_legendary_command(["list"], timeout=1)

            assert "timed out" in str(exc_info.value)


class TestEpicAuthentication:
    """Test Epic authentication methods using device code flow."""

    @pytest.mark.asyncio
    async def test_start_device_auth_success(self):
        """Test starting device authentication flow."""
        service = EpicService("test-user")

        # Mock legendary output with device code URL
        mock_result = {
            "stdout": "Please visit: https://www.epicgames.com/activate?code=ABCD-1234\n",
            "stderr": "",
            "returncode": 0
        }

        with patch.object(service, '_run_legendary_command', new_callable=AsyncMock) as mock_cmd:
            mock_cmd.return_value = mock_result

            url = await service.start_device_auth()

            assert url == "https://www.epicgames.com/activate?code=ABCD-1234"
            mock_cmd.assert_called_once_with(["auth", "--json"])

    @pytest.mark.asyncio
    async def test_complete_auth_success(self):
        """Test completing authentication with valid code."""
        service = EpicService("test-user")

        # Mock successful auth completion
        mock_result = {
            "stdout": '{"status": "Logged in as TestUser"}',
            "stderr": "",
            "returncode": 0
        }

        with patch.object(service, '_run_legendary_command', new_callable=AsyncMock) as mock_cmd:
            mock_cmd.return_value = mock_result

            success = await service.complete_auth("ABCD-1234")

            assert success is True
            mock_cmd.assert_called_once_with(["auth", "--code", "ABCD-1234", "--json"])

    @pytest.mark.asyncio
    async def test_complete_auth_invalid_code(self):
        """Test authentication fails with invalid code."""
        service = EpicService("test-user")

        # Mock legendary command error
        with patch.object(
            service, '_run_legendary_command',
            side_effect=EpicAPIError("Invalid authorization code")
        ):
            with pytest.raises(EpicAuthenticationError) as exc_info:
                await service.complete_auth("INVALID")

            assert "authentication failed" in str(exc_info.value).lower()

    @pytest.mark.asyncio
    async def test_verify_auth_authenticated(self):
        """Test verify_auth returns True when authenticated."""
        service = EpicService("test-user")

        # Mock legendary status showing authenticated state
        mock_result = {
            "stdout": '{"account": "test@example.com", "status": "authenticated"}',
            "stderr": "",
            "returncode": 0
        }

        with patch.object(service, '_run_legendary_command', new_callable=AsyncMock) as mock_cmd:
            mock_cmd.return_value = mock_result

            is_authenticated = await service.verify_auth()

            assert is_authenticated is True
            mock_cmd.assert_called_once_with(["status", "--json"])

    @pytest.mark.asyncio
    async def test_verify_auth_not_authenticated(self):
        """Test verify_auth returns False when not authenticated."""
        service = EpicService("test-user")

        # Mock legendary command raising auth expired error
        with patch.object(
            service, '_run_legendary_command',
            side_effect=EpicAuthExpiredError("Not authenticated")
        ):
            is_authenticated = await service.verify_auth()

            assert is_authenticated is False

    @pytest.mark.asyncio
    async def test_get_account_info_success(self):
        """Test getting Epic account information."""
        service = EpicService("test-user")

        # Mock legendary status output with account info
        mock_result = {
            "stdout": '{"account": {"displayName": "TestPlayer", "id": "abc123xyz"}}',
            "stderr": "",
            "returncode": 0
        }

        with patch.object(service, '_run_legendary_command', new_callable=AsyncMock) as mock_cmd:
            mock_cmd.return_value = mock_result

            account = await service.get_account_info()

            assert isinstance(account, EpicAccountInfo)
            assert account.display_name == "TestPlayer"
            assert account.account_id == "abc123xyz"
            mock_cmd.assert_called_once_with(["status", "--json"])

    @pytest.mark.asyncio
    async def test_disconnect_success(self):
        """Test disconnecting Epic account."""
        service = EpicService("test-user")

        # Mock successful logout
        mock_result = {
            "stdout": "Logged out successfully",
            "stderr": "",
            "returncode": 0
        }

        with patch.object(service, '_run_legendary_command', new_callable=AsyncMock) as mock_cmd:
            mock_cmd.return_value = mock_result

            await service.disconnect()

            mock_cmd.assert_called_once_with(["auth", "--delete"])


class TestEpicLibrary:
    """Test Epic Games library fetching."""

    @pytest.mark.asyncio
    async def test_get_library_success(self):
        """Test fetching Epic library with games."""
        service = EpicService("test-user")

        # Mock legendary list output
        mock_result = {
            "stdout": '''[
                {
                    "app_name": "Fortnite",
                    "app_title": "Fortnite",
                    "app_version": "1.0",
                    "metadata": {"genre": "Battle Royale"}
                },
                {
                    "app_name": "RocketLeague",
                    "app_title": "Rocket League",
                    "app_version": "2.0",
                    "metadata": {"genre": "Sports"}
                }
            ]''',
            "stderr": "",
            "returncode": 0
        }

        with patch.object(service, '_run_legendary_command', new_callable=AsyncMock) as mock_cmd:
            mock_cmd.return_value = mock_result

            games = await service.get_library()

            assert len(games) == 2
            assert games[0].app_name == "Fortnite"
            assert games[0].title == "Fortnite"
            assert games[0].metadata == {"genre": "Battle Royale"}
            assert games[1].app_name == "RocketLeague"
            assert games[1].title == "Rocket League"
            assert games[1].metadata == {"genre": "Sports"}
            mock_cmd.assert_called_once_with(["list", "--json"])

    @pytest.mark.asyncio
    async def test_get_library_empty(self):
        """Test fetching Epic library when no games owned."""
        service = EpicService("test-user")

        # Mock legendary list returning empty array
        mock_result = {
            "stdout": "[]",
            "stderr": "",
            "returncode": 0
        }

        with patch.object(service, '_run_legendary_command', new_callable=AsyncMock) as mock_cmd:
            mock_cmd.return_value = mock_result

            games = await service.get_library()

            assert games == []
            mock_cmd.assert_called_once_with(["list", "--json"])

    @pytest.mark.asyncio
    async def test_get_library_auth_expired(self):
        """Test get_library raises EpicAuthExpiredError when auth expired."""
        service = EpicService("test-user")

        # Mock legendary command raising auth expired error
        with patch.object(
            service, '_run_legendary_command',
            side_effect=EpicAuthExpiredError("Authentication expired")
        ):
            with pytest.raises(EpicAuthExpiredError) as exc_info:
                await service.get_library()

            assert "authentication expired" in str(exc_info.value).lower()

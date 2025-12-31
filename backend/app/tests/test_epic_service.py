"""Tests for Epic Games Store service using legendary CLI."""

import asyncio
from unittest.mock import AsyncMock, patch, MagicMock

import pytest

from app.services.epic import (
    EpicService,
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

"""Tests for PSN service."""

import pytest
from unittest.mock import Mock, patch


def test_psn_service_imports():
    """Test that PSN service imports successfully."""
    from app.services.psn import (
        PSNService,
        PSNAccountInfo,
        PSNGame,
        PSNAPIError,
        PSNAuthenticationError,
        PSNTokenExpiredError,
    )

    assert PSNService is not None
    assert PSNAccountInfo is not None
    assert PSNGame is not None


def test_psn_service_init_success():
    """Test PSNService initializes PSNAWP successfully."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        from app.services.psn import PSNService

        service = PSNService("a" * 64)

        mock_psnawp.assert_called_once_with("a" * 64)
        assert service.npsso_token == "a" * 64
        assert service.psnawp is not None


def test_psn_service_init_failure():
    """Test PSNService handles PSNAWP initialization failure."""
    with patch('psnawp_api.PSNAWP', side_effect=Exception("Init failed")):
        from app.services.psn import PSNService, PSNAuthenticationError

        with pytest.raises(PSNAuthenticationError) as exc_info:
            PSNService("a" * 64)

        assert "Failed to initialize PSN service" in str(exc_info.value)


@pytest.mark.asyncio
async def test_verify_token_success():
    """Test token verification succeeds with valid token."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_client = Mock()
        mock_client.online_id = "test_user"
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.verify_token()

        assert result is True
        mock_psnawp.return_value.me.assert_called_once()


@pytest.mark.asyncio
async def test_verify_token_failure():
    """Test token verification fails with invalid token."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_psnawp.return_value.me.side_effect = Exception("Invalid token")

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.verify_token()

        assert result is False

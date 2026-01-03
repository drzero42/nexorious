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

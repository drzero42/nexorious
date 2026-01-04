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


@pytest.mark.asyncio
async def test_get_account_info_success():
    """Test getting account info succeeds."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_client = Mock()
        mock_client.online_id = "test_user"
        mock_client.account_id = "account123"
        # get_region returns a Country object with alpha_2 attribute
        mock_region = Mock()
        mock_region.alpha_2 = "US"
        mock_client.get_region.return_value = mock_region
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.get_account_info()

        assert result.online_id == "test_user"
        assert result.account_id == "account123"
        assert result.region == "US"


@pytest.mark.asyncio
async def test_get_account_info_expired_token():
    """Test getting account info fails with expired token."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_psnawp.return_value.me.side_effect = Exception("Token expired")

        from app.services.psn import PSNService, PSNTokenExpiredError
        service = PSNService("a" * 64)

        with pytest.raises(PSNTokenExpiredError) as exc_info:
            await service.get_account_info()

        assert "NPSSO token has expired" in str(exc_info.value)


@pytest.mark.asyncio
async def test_get_account_info_auth_error():
    """Test getting account info fails with auth error."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_psnawp.return_value.me.side_effect = Exception("Invalid credentials")

        from app.services.psn import PSNService, PSNAuthenticationError
        service = PSNService("a" * 64)

        with pytest.raises(PSNAuthenticationError) as exc_info:
            await service.get_account_info()

        assert "Failed to get account info" in str(exc_info.value)


@pytest.mark.asyncio
async def test_get_library_success():
    """Test getting library returns PSN games with platform detection."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        # game_entitlements returns a list of dicts
        entitlement1 = {
            "productId": "PROD001",
            "titleMeta": {"titleId": "GAME001", "name": "Test Game 1"},
            "entitlementAttributes": [{"platformId": "ps5"}]
        }
        entitlement2 = {
            "productId": "PROD002",
            "titleMeta": {"titleId": "GAME002", "name": "Test Game 2 (PS4+PS5)"},
            "entitlementAttributes": [{"platformId": "ps5"}, {"platformId": "ps4"}]
        }

        mock_client = Mock()
        mock_client.game_entitlements.return_value = [entitlement1, entitlement2]
        mock_client.title_stats.return_value = []
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.get_library()

        assert len(result) == 2
        assert result[0].product_id == "GAME001"
        assert result[0].name == "Test Game 1"
        assert result[0].platforms == ["playstation-5"]
        assert result[1].product_id == "GAME002"
        assert result[1].platforms == ["playstation-5", "playstation-4"]


@pytest.mark.asyncio
async def test_get_library_fallback_platform():
    """Test library falls back to PS5 when no platform info."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        # Entitlement with no entitlementAttributes (should fallback to PS5)
        entitlement = {
            "productId": "PROD003",
            "titleMeta": {"titleId": "GAME003", "name": "Test Game 3"},
            "entitlementAttributes": []  # Empty - no platform info
        }

        mock_client = Mock()
        mock_client.game_entitlements.return_value = [entitlement]
        mock_client.title_stats.return_value = []
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.get_library()

        assert len(result) == 1
        assert result[0].platforms == ["playstation-5"]


@pytest.mark.asyncio
async def test_get_library_expired_token():
    """Test library fetch fails with expired token."""
    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        mock_psnawp.return_value.me.return_value.game_entitlements.side_effect = Exception("Token expired")

        from app.services.psn import PSNService, PSNTokenExpiredError
        service = PSNService("a" * 64)

        with pytest.raises(PSNTokenExpiredError):
            await service.get_library()


@pytest.mark.asyncio
async def test_get_library_with_playtime():
    """Test getting library includes playtime from title_stats."""
    from datetime import timedelta

    with patch('psnawp_api.PSNAWP') as mock_psnawp:
        # Mock game_entitlements response
        mock_entitlement = {
            "productId": "PROD001",
            "titleMeta": {"titleId": "CUSA12345", "name": "Test Game"},
            "entitlementAttributes": [{"platformId": "ps5"}]
        }

        # Mock title_stats response with playtime
        mock_stats = Mock()
        mock_stats.title_id = "CUSA12345"
        mock_stats.play_duration = timedelta(hours=42, minutes=30)

        mock_client = Mock()
        mock_client.game_entitlements.return_value = [mock_entitlement]
        mock_client.title_stats.return_value = [mock_stats]
        mock_psnawp.return_value.me.return_value = mock_client

        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        result = await service.get_library()

        assert len(result) == 1
        assert result[0].product_id == "CUSA12345"
        assert result[0].playtime_hours == 42  # 42h30m truncates to 42h


@pytest.mark.asyncio
async def test_disconnect():
    """Test disconnect is a no-op for stateless PSNAWP."""
    with patch('psnawp_api.PSNAWP'):
        from app.services.psn import PSNService
        service = PSNService("a" * 64)

        # Should not raise
        await service.disconnect()

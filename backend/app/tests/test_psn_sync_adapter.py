"""Tests for PSN sync adapter."""

import pytest
from unittest.mock import Mock, AsyncMock, patch


def test_psn_adapter_imports():
    """Test PSN adapter imports successfully."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    assert PSNSyncAdapter is not None
    assert hasattr(PSNSyncAdapter, 'source')
    assert hasattr(PSNSyncAdapter, 'fetch_games')
    assert hasattr(PSNSyncAdapter, 'get_credentials')
    assert hasattr(PSNSyncAdapter, 'is_configured')


def test_get_credentials_valid():
    """Test get_credentials returns token when configured."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    user = Mock()
    user.preferences = {
        "psn": {
            "npsso_token": "a" * 64,
            "is_verified": True
        }
    }

    adapter = PSNSyncAdapter()
    result = adapter.get_credentials(user)

    assert result is not None
    assert result["npsso_token"] == "a" * 64


def test_get_credentials_not_verified():
    """Test get_credentials returns None when not verified."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    user = Mock()
    user.preferences = {
        "psn": {
            "npsso_token": "a" * 64,
            "is_verified": False
        }
    }

    adapter = PSNSyncAdapter()
    result = adapter.get_credentials(user)

    assert result is None


def test_get_credentials_no_config():
    """Test get_credentials returns None when not configured."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    user = Mock()
    user.preferences = {}

    adapter = PSNSyncAdapter()
    result = adapter.get_credentials(user)

    assert result is None


def test_is_configured_true():
    """Test is_configured returns True when credentials valid."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    user = Mock()
    user.preferences = {
        "psn": {
            "npsso_token": "a" * 64,
            "is_verified": True
        }
    }

    adapter = PSNSyncAdapter()
    result = adapter.is_configured(user)

    assert result is True


def test_is_configured_false():
    """Test is_configured returns False when not configured."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    user = Mock()
    user.preferences = {}

    adapter = PSNSyncAdapter()
    result = adapter.is_configured(user)

    assert result is False


def test_mark_token_expired():
    """Test _mark_token_expired marks token as invalid."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    user = Mock()
    user.preferences = {
        "psn": {
            "npsso_token": "a" * 64,
            "is_verified": True
        }
    }
    session = Mock()

    adapter = PSNSyncAdapter()
    adapter._mark_token_expired(user, session)

    assert user.preferences["psn"]["is_verified"] is False
    assert "token_expired_at" in user.preferences["psn"]
    session.commit.assert_called_once()


@pytest.mark.asyncio
async def test_fetch_games_success():
    """Test fetch_games converts PSN games to ExternalGame format."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter
    from app.services.psn import PSNGame

    user = Mock()
    user.id = "user123"
    user.preferences = {
        "psn": {
            "npsso_token": "a" * 64,
            "is_verified": True
        }
    }
    session = Mock()

    # Mock PSNService
    mock_game1 = PSNGame(
        product_id="GAME001",
        name="Test Game 1",
        platforms=["playstation-5"],
        metadata={"product_id": "GAME001"}
    )
    mock_game2 = PSNGame(
        product_id="GAME002",
        name="Test Game 2",
        platforms=["playstation-5", "playstation-4"],
        metadata={"product_id": "GAME002"}
    )

    with patch('app.worker.tasks.sync.adapters.psn.PSNService') as mock_service_class:
        mock_service = AsyncMock()
        mock_service.get_library.return_value = [mock_game1, mock_game2]
        mock_service_class.return_value = mock_service

        adapter = PSNSyncAdapter()
        result = await adapter.fetch_games(user, session)

    # Should create 3 ExternalGame objects (1 for game1, 2 for game2)
    assert len(result) == 3
    assert result[0].external_id == "GAME001"
    assert result[0].platform == "playstation-5"
    assert result[0].storefront == "playstation-store"
    assert result[1].external_id == "GAME002"
    assert result[1].platform == "playstation-5"
    assert result[2].external_id == "GAME002"
    assert result[2].platform == "playstation-4"


@pytest.mark.asyncio
async def test_fetch_games_not_configured():
    """Test fetch_games raises ValueError when not configured."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    user = Mock()
    user.preferences = {}
    session = Mock()

    adapter = PSNSyncAdapter()

    with pytest.raises(ValueError) as exc_info:
        await adapter.fetch_games(user, session)

    assert "PSN credentials not configured" in str(exc_info.value)


@pytest.mark.asyncio
async def test_fetch_games_token_expired():
    """Test fetch_games handles token expiration."""
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter
    from app.services.psn import PSNTokenExpiredError

    user = Mock()
    user.id = "user123"
    user.preferences = {
        "psn": {
            "npsso_token": "a" * 64,
            "is_verified": True
        }
    }
    session = Mock()

    with patch('app.worker.tasks.sync.adapters.psn.PSNService') as mock_service_class:
        mock_service = AsyncMock()
        mock_service.get_library.side_effect = PSNTokenExpiredError("Token expired")
        mock_service_class.return_value = mock_service

        adapter = PSNSyncAdapter()

        with pytest.raises(PSNTokenExpiredError):
            await adapter.fetch_games(user, session)

        # Should mark token as expired
        assert user.preferences["psn"]["is_verified"] is False


def test_psn_adapter_registered():
    """Test PSN adapter is registered in get_sync_adapter."""
    from app.worker.tasks.sync.adapters.base import get_sync_adapter
    from app.worker.tasks.sync.adapters.psn import PSNSyncAdapter

    adapter = get_sync_adapter("psn")

    assert isinstance(adapter, PSNSyncAdapter)

"""Tests for sync schemas."""

import pytest


def test_sync_platform_psn_exists():
    """Test PSN platform exists in SyncPlatform enum."""
    from app.schemas.sync import SyncPlatform

    assert hasattr(SyncPlatform, 'PSN')
    assert SyncPlatform.PSN.value == "psn"


def test_psn_configure_request_schema():
    """Test PSNConfigureRequest validates token length."""
    from app.schemas.sync import PSNConfigureRequest
    from pydantic import ValidationError

    # Valid 64-char token
    valid_request = PSNConfigureRequest(npsso_token="a" * 64)
    assert valid_request.npsso_token == "a" * 64

    # Invalid short token
    with pytest.raises(ValidationError):
        PSNConfigureRequest(npsso_token="short")

    # Invalid long token
    with pytest.raises(ValidationError):
        PSNConfigureRequest(npsso_token="a" * 100)


def test_psn_configure_response_schema():
    """Test PSNConfigureResponse schema."""
    from app.schemas.sync import PSNConfigureResponse

    response = PSNConfigureResponse(
        success=True,
        online_id="test_user",
        account_id="account123",
        region="us",
        message="Success"
    )

    assert response.success is True
    assert response.online_id == "test_user"


def test_psn_status_response_schema():
    """Test PSNStatusResponse schema."""
    from app.schemas.sync import PSNStatusResponse

    response = PSNStatusResponse(
        is_configured=True,
        online_id="test_user",
        account_id="account123",
        region="us",
        token_expired=False
    )

    assert response.is_configured is True
    assert response.token_expired is False

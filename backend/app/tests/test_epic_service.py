"""Tests for Epic Games Store service using legendary CLI."""

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

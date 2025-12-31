"""Tests for Epic Games Store service using legendary CLI."""

from app.services.epic import EpicService


class TestEpicService:
    """Test EpicService initialization and config path setup."""

    def test_service_initialization(self):
        """Test EpicService creates with correct user config path."""
        user_id = "test-user-123"
        service = EpicService(user_id)

        assert service.user_id == user_id
        assert service.config_path == f"/var/lib/nexorious/legendary-configs/{user_id}"

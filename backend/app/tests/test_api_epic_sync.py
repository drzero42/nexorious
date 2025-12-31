"""
Tests for Epic Games Store authentication API endpoints.

Tests cover:
- Starting Epic device authentication flow
- Completing authentication with authorization code
- Checking authentication status
- Disconnecting Epic account
- Error handling for Epic API errors and authentication failures
"""

from unittest.mock import patch, AsyncMock, MagicMock
from fastapi.testclient import TestClient
from sqlmodel import Session

from app.models.user import User
from app.services.epic import (
    EpicAccountInfo,
    EpicAuthenticationError,
    EpicAPIError,
)


class TestEpicAuthStart:
    """Tests for POST /sync/epic/auth/start endpoint."""

    def test_start_epic_auth_success(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test starting Epic authentication returns auth URL and instructions."""
        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.start_device_auth = AsyncMock(
                return_value="https://www.epicgames.com/activate?code=TEST123"
            )

            response = client.post(
                "/api/sync/epic/auth/start",
                headers=auth_headers,
            )

            assert response.status_code == 200
            data = response.json()
            assert data["auth_url"] == "https://www.epicgames.com/activate?code=TEST123"
            assert "instructions" in data
            assert len(data["instructions"]) > 0

            # Verify service was called
            mock_service.start_device_auth.assert_called_once()

    def test_start_epic_auth_error(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test starting Epic authentication handles API errors."""
        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.start_device_auth = AsyncMock(
                side_effect=EpicAPIError("Network error")
            )

            response = client.post(
                "/api/sync/epic/auth/start",
                headers=auth_headers,
            )

            assert response.status_code == 500
            data = response.json()
            # Check for either "detail" or "error" key (depends on error handler)
            assert "detail" in data or "error" in data


class TestEpicAuthComplete:
    """Tests for POST /sync/epic/auth/complete endpoint."""

    def test_complete_epic_auth_success(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        test_user: User,
        session: Session,
    ) -> None:
        """Test completing Epic authentication with valid code updates preferences."""
        mock_account_info = EpicAccountInfo(
            display_name="TestEpicUser",
            account_id="epic123456"
        )

        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.complete_auth = AsyncMock(return_value=True)
            mock_service.get_account_info = AsyncMock(return_value=mock_account_info)

            response = client.post(
                "/api/sync/epic/auth/complete",
                headers=auth_headers,
                json={"code": "valid_code_123"},
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is True
            assert data["display_name"] == "TestEpicUser"
            assert data["error"] is None

            # Verify service calls
            mock_service.complete_auth.assert_called_once_with("valid_code_123")
            mock_service.get_account_info.assert_called_once()

            # Verify preferences were updated
            session.refresh(test_user)
            preferences = test_user.preferences
            assert "epic" in preferences
            assert preferences["epic"]["is_verified"] is True
            assert preferences["epic"]["display_name"] == "TestEpicUser"
            assert preferences["epic"]["account_id"] == "epic123456"

    def test_complete_epic_auth_invalid_code(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test completing Epic authentication with invalid code returns error."""
        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.complete_auth = AsyncMock(
                side_effect=EpicAuthenticationError("Invalid code")
            )

            response = client.post(
                "/api/sync/epic/auth/complete",
                headers=auth_headers,
                json={"code": "invalid_code"},
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "invalid_code"
            assert data["display_name"] is None

    def test_complete_epic_auth_network_error(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test completing Epic authentication handles network errors."""
        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.complete_auth = AsyncMock(
                side_effect=EpicAPIError("Network timeout")
            )

            response = client.post(
                "/api/sync/epic/auth/complete",
                headers=auth_headers,
                json={"code": "test_code"},
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "network_error"


class TestEpicAuthCheck:
    """Tests for GET /sync/epic/auth/check endpoint."""

    def test_check_epic_auth_authenticated(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test checking Epic authentication status when authenticated."""
        mock_account_info = EpicAccountInfo(
            display_name="TestEpicUser",
            account_id="epic123456"
        )

        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_auth = AsyncMock(return_value=True)
            mock_service.get_account_info = AsyncMock(return_value=mock_account_info)

            response = client.get(
                "/api/sync/epic/auth/check",
                headers=auth_headers,
            )

            assert response.status_code == 200
            data = response.json()
            assert data["is_authenticated"] is True
            assert data["display_name"] == "TestEpicUser"

            # Verify service calls
            mock_service.verify_auth.assert_called_once()
            mock_service.get_account_info.assert_called_once()

    def test_check_epic_auth_not_authenticated(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test checking Epic authentication status when not authenticated."""
        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_auth = AsyncMock(return_value=False)

            response = client.get(
                "/api/sync/epic/auth/check",
                headers=auth_headers,
            )

            assert response.status_code == 200
            data = response.json()
            assert data["is_authenticated"] is False
            assert data["display_name"] is None

            # Verify verify_auth was called but get_account_info was not
            mock_service.verify_auth.assert_called_once()
            mock_service.get_account_info.assert_not_called()

    def test_check_epic_auth_handles_exceptions(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test checking Epic authentication handles exceptions gracefully."""
        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_auth = AsyncMock(
                side_effect=EpicAPIError("Connection failed")
            )

            response = client.get(
                "/api/sync/epic/auth/check",
                headers=auth_headers,
            )

            assert response.status_code == 200
            data = response.json()
            assert data["is_authenticated"] is False
            assert data["display_name"] is None


class TestEpicDisconnect:
    """Tests for DELETE /sync/epic/connection endpoint."""

    def test_disconnect_epic_success(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        test_user: User,
        session: Session,
    ) -> None:
        """Test disconnecting Epic account clears preferences."""
        # Set up Epic credentials
        test_user.preferences_json = '{"epic": {"is_verified": true, "display_name": "TestUser", "account_id": "123"}}'
        session.commit()

        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.disconnect = AsyncMock()

            response = client.delete(
                "/api/sync/epic/connection",
                headers=auth_headers,
            )

            assert response.status_code == 200
            data = response.json()
            assert data["success"] is True

            # Verify service was called
            mock_service.disconnect.assert_called_once()

            # Verify preferences were cleared
            session.refresh(test_user)
            preferences = test_user.preferences
            assert "epic" not in preferences

    def test_disconnect_epic_works_without_existing_config(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test disconnecting Epic works even if no config exists."""
        with patch("app.api.sync.EpicService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.disconnect = AsyncMock()

            response = client.delete(
                "/api/sync/epic/connection",
                headers=auth_headers,
            )

            assert response.status_code == 200
            data = response.json()
            assert data["success"] is True

    def test_disconnect_epic_requires_auth(
        self,
        client: TestClient,
    ) -> None:
        """Test disconnecting Epic requires authentication."""
        response = client.delete("/api/sync/epic/connection")
        assert response.status_code == 403

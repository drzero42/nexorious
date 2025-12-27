"""
Tests for the Steam credential verification endpoint.

Tests cover:
- Format validation for Steam ID and API key
- Successful verification with mocked Steam service
- Various error scenarios (invalid API key, invalid Steam ID, private profile)
- Authentication requirements
"""

import pytest
from unittest.mock import patch, AsyncMock, MagicMock
from fastapi.testclient import TestClient
from sqlmodel import Session
from datetime import datetime, timezone, timedelta
import uuid

from app.models.user import User, UserSession
from app.core.security import create_access_token, hash_token
from app.services.steam import SteamUserInfo, SteamAPIError, SteamAuthenticationError


@pytest.fixture
def auth_headers(test_user: User, session: Session) -> dict[str, str]:
    """Create authentication headers for test user."""
    access_token = create_access_token(data={"sub": test_user.id})

    # Create session record for the token
    session_record = UserSession(
        id=str(uuid.uuid4()),
        user_id=test_user.id,
        token_hash=hash_token(access_token),
        refresh_token_hash=hash_token("test_refresh_token"),
        expires_at=datetime.now(timezone.utc) + timedelta(days=30),
        user_agent="test-client",
        ip_address="127.0.0.1",
    )
    session.add(session_record)
    session.commit()

    return {"Authorization": f"Bearer {access_token}"}


@pytest.fixture
def valid_steam_id() -> str:
    """Return a valid format Steam ID."""
    return "76561197960435530"


@pytest.fixture
def valid_api_key() -> str:
    """Return a valid format API key (32 hex characters)."""
    return "ABCDEF1234567890ABCDEF1234567890"


@pytest.fixture
def mock_steam_user_info() -> SteamUserInfo:
    """Return a mock SteamUserInfo object for a public profile."""
    return SteamUserInfo(
        steam_id="76561197960435530",
        persona_name="TestPlayer",
        profile_url="https://steamcommunity.com/id/testplayer/",
        avatar="https://avatar.url/small.jpg",
        avatar_medium="https://avatar.url/medium.jpg",
        avatar_full="https://avatar.url/full.jpg",
        persona_state=1,
        community_visibility_state=3,  # Public
        profile_state=1,
        last_logoff=1234567890,
    )


@pytest.fixture
def mock_private_profile_user_info() -> SteamUserInfo:
    """Return a mock SteamUserInfo object for a private profile."""
    return SteamUserInfo(
        steam_id="76561197960435530",
        persona_name="PrivatePlayer",
        profile_url="https://steamcommunity.com/id/privateplayer/",
        avatar="https://avatar.url/small.jpg",
        avatar_medium="https://avatar.url/medium.jpg",
        avatar_full="https://avatar.url/full.jpg",
        persona_state=0,
        community_visibility_state=1,  # Private
        profile_state=1,
    )


class TestSteamVerifyFormatValidation:
    """Tests for format validation of Steam ID and API key."""

    def test_invalid_steam_id_format_wrong_prefix(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_api_key: str,
    ) -> None:
        """Test that Steam ID with wrong prefix is rejected."""
        response = client.post(
            "/api/sync/steam/verify",
            headers=auth_headers,
            json={
                "steam_id": "12345678901234567",  # Wrong prefix
                "web_api_key": valid_api_key,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_steam_id"
        assert data["steam_username"] is None

    def test_invalid_steam_id_format_wrong_length(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_api_key: str,
    ) -> None:
        """Test that Steam ID with wrong length is rejected."""
        response = client.post(
            "/api/sync/steam/verify",
            headers=auth_headers,
            json={
                "steam_id": "7656119123",  # Too short
                "web_api_key": valid_api_key,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_steam_id"

    def test_invalid_steam_id_format_non_numeric(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_api_key: str,
    ) -> None:
        """Test that non-numeric Steam ID is rejected."""
        response = client.post(
            "/api/sync/steam/verify",
            headers=auth_headers,
            json={
                "steam_id": "7656119abcdefghij",
                "web_api_key": valid_api_key,
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_steam_id"

    def test_invalid_api_key_format_wrong_length(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
    ) -> None:
        """Test that API key with wrong length is rejected."""
        response = client.post(
            "/api/sync/steam/verify",
            headers=auth_headers,
            json={
                "steam_id": valid_steam_id,
                "web_api_key": "TOOSHORT",
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_api_key"

    def test_invalid_api_key_format_invalid_characters(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
    ) -> None:
        """Test that API key with invalid characters is rejected."""
        response = client.post(
            "/api/sync/steam/verify",
            headers=auth_headers,
            json={
                "steam_id": valid_steam_id,
                "web_api_key": "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",  # Z is not hex
            },
        )

        assert response.status_code == 200
        data = response.json()
        assert data["valid"] is False
        assert data["error"] == "invalid_api_key"


class TestSteamVerifySuccess:
    """Tests for successful Steam verification."""

    def test_successful_verification(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
        valid_api_key: str,
        mock_steam_user_info: SteamUserInfo,
    ) -> None:
        """Test successful verification returns valid=True and username."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_api_key = AsyncMock(return_value=True)
            mock_service.get_user_info = AsyncMock(return_value=mock_steam_user_info)

            response = client.post(
                "/api/sync/steam/verify",
                headers=auth_headers,
                json={
                    "steam_id": valid_steam_id,
                    "web_api_key": valid_api_key,
                },
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is True
            assert data["steam_username"] == "TestPlayer"
            assert data["error"] is None

            # Verify the service was called correctly
            mock_service_class.assert_called_once_with(valid_api_key)
            mock_service.verify_api_key.assert_called_once()
            mock_service.get_user_info.assert_called_once_with(valid_steam_id)


class TestSteamVerifyAPIErrors:
    """Tests for Steam API error scenarios."""

    def test_invalid_api_key_from_steam(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
        valid_api_key: str,
    ) -> None:
        """Test that invalid API key from Steam returns appropriate error."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_api_key = AsyncMock(return_value=False)

            response = client.post(
                "/api/sync/steam/verify",
                headers=auth_headers,
                json={
                    "steam_id": valid_steam_id,
                    "web_api_key": valid_api_key,
                },
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "invalid_api_key"

    def test_invalid_steam_id_from_steam(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
        valid_api_key: str,
    ) -> None:
        """Test that invalid Steam ID from Steam returns appropriate error."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_api_key = AsyncMock(return_value=True)
            mock_service.get_user_info = AsyncMock(return_value=None)

            response = client.post(
                "/api/sync/steam/verify",
                headers=auth_headers,
                json={
                    "steam_id": valid_steam_id,
                    "web_api_key": valid_api_key,
                },
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "invalid_steam_id"

    def test_private_profile(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
        valid_api_key: str,
        mock_private_profile_user_info: SteamUserInfo,
    ) -> None:
        """Test that private profile returns appropriate error."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_api_key = AsyncMock(return_value=True)
            mock_service.get_user_info = AsyncMock(return_value=mock_private_profile_user_info)

            response = client.post(
                "/api/sync/steam/verify",
                headers=auth_headers,
                json={
                    "steam_id": valid_steam_id,
                    "web_api_key": valid_api_key,
                },
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "private_profile"

    def test_steam_authentication_error(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
        valid_api_key: str,
    ) -> None:
        """Test that SteamAuthenticationError returns invalid_api_key error."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_api_key = AsyncMock(
                side_effect=SteamAuthenticationError("Invalid API key")
            )

            response = client.post(
                "/api/sync/steam/verify",
                headers=auth_headers,
                json={
                    "steam_id": valid_steam_id,
                    "web_api_key": valid_api_key,
                },
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "invalid_api_key"

    def test_steam_api_error_rate_limit(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
        valid_api_key: str,
    ) -> None:
        """Test that rate limit error from Steam returns rate_limited error."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_api_key = AsyncMock(
                side_effect=SteamAPIError("Rate limit exceeded")
            )

            response = client.post(
                "/api/sync/steam/verify",
                headers=auth_headers,
                json={
                    "steam_id": valid_steam_id,
                    "web_api_key": valid_api_key,
                },
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "rate_limited"

    def test_steam_api_error_network(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
        valid_api_key: str,
    ) -> None:
        """Test that network error from Steam returns network_error."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_api_key = AsyncMock(
                side_effect=SteamAPIError("Connection refused")
            )

            response = client.post(
                "/api/sync/steam/verify",
                headers=auth_headers,
                json={
                    "steam_id": valid_steam_id,
                    "web_api_key": valid_api_key,
                },
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "network_error"

    def test_unexpected_exception(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
        valid_steam_id: str,
        valid_api_key: str,
    ) -> None:
        """Test that unexpected exceptions return network_error."""
        with patch("app.api.sync.SteamService") as mock_service_class:
            mock_service = MagicMock()
            mock_service_class.return_value = mock_service
            mock_service.verify_api_key = AsyncMock(
                side_effect=RuntimeError("Unexpected error")
            )

            response = client.post(
                "/api/sync/steam/verify",
                headers=auth_headers,
                json={
                    "steam_id": valid_steam_id,
                    "web_api_key": valid_api_key,
                },
            )

            assert response.status_code == 200
            data = response.json()
            assert data["valid"] is False
            assert data["error"] == "network_error"


class TestSteamVerifyAuthentication:
    """Tests for authentication requirements."""

    def test_requires_authentication(self, client: TestClient) -> None:
        """Test that the endpoint requires authentication."""
        response = client.post(
            "/api/sync/steam/verify",
            json={
                "steam_id": "76561197960435530",
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
            },
        )

        assert response.status_code == 403

    def test_invalid_token_rejected(self, client: TestClient) -> None:
        """Test that invalid tokens are rejected."""
        response = client.post(
            "/api/sync/steam/verify",
            headers={"Authorization": "Bearer invalid_token"},
            json={
                "steam_id": "76561197960435530",
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
            },
        )

        assert response.status_code == 401


class TestSteamVerifyRequestValidation:
    """Tests for request body validation."""

    def test_missing_steam_id(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test that missing steam_id returns 422."""
        response = client.post(
            "/api/sync/steam/verify",
            headers=auth_headers,
            json={
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
            },
        )

        assert response.status_code == 422

    def test_missing_api_key(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test that missing web_api_key returns 422."""
        response = client.post(
            "/api/sync/steam/verify",
            headers=auth_headers,
            json={
                "steam_id": "76561197960435530",
            },
        )

        assert response.status_code == 422

    def test_empty_body(
        self,
        client: TestClient,
        auth_headers: dict[str, str],
    ) -> None:
        """Test that empty body returns 422."""
        response = client.post(
            "/api/sync/steam/verify",
            headers=auth_headers,
            json={},
        )

        assert response.status_code == 422

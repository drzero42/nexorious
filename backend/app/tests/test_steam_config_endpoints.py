"""
Tests for Steam configuration API endpoints.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from fastapi.testclient import TestClient
from sqlmodel import Session
import json

from app.models.user import User
from app.services.steam import SteamUserInfo, SteamAuthenticationError, SteamAPIError
from app.tests.integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    auth_headers_fixture as auth_headers,
    assert_api_error,
    assert_api_success
)


@pytest.fixture
def mock_steam_service():
    """Mock Steam service for testing."""
    with patch('app.api.steam_config.create_steam_service') as mock:
        yield mock


class TestSteamConfigEndpoints:
    """Test Steam configuration endpoints."""

    def test_get_steam_config_no_config(self, client: TestClient, auth_headers):
        """Test getting Steam config when user has no configuration."""
        response = client.get("/api/steam/config", headers=auth_headers)
        
        assert_api_success(response)
        data = response.json()
        assert data["has_api_key"] is False
        assert data["api_key_masked"] is None
        assert data["steam_id"] is None
        assert data["is_verified"] is False
        assert data["configured_at"] is None

    def test_get_steam_config_with_config(self, client: TestClient, auth_headers, session: Session, test_user: User):
        """Test getting Steam config when user has configuration."""
        # Set up user with Steam config
        user = session.get(User, test_user.id)
        steam_config = {
            "steam": {
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
                "steam_id": "76561197960435530",
                "is_verified": True,
                "configured_at": "2023-01-01T12:00:00+00:00"
            }
        }
        user.preferences_json = json.dumps(steam_config)
        session.commit()
        
        response = client.get("/api/steam/config", headers=auth_headers)
        
        assert_api_success(response)
        data = response.json()
        assert data["has_api_key"] is True
        assert data["api_key_masked"] == "ABCDEF12****7890"
        assert data["steam_id"] == "76561197960435530"
        assert data["is_verified"] is True
        assert data["configured_at"] is not None

    def test_get_steam_config_unauthorized(self, client: TestClient):
        """Test getting Steam config without authentication."""
        response = client.get("/api/steam/config")
        
        assert_api_error(response, 403)

    def test_set_steam_config_success(self, client: TestClient, auth_headers, mock_steam_service):
        """Test setting Steam config successfully."""
        # Mock Steam service verification
        mock_service = MagicMock()
        mock_service.verify_api_key = AsyncMock(return_value=True)
        mock_service.validate_steam_id = MagicMock(return_value=True)
        mock_service.get_user_info = AsyncMock(return_value=SteamUserInfo(
            steam_id="76561197960435530",
            persona_name="Test User",
            profile_url="https://steamcommunity.com/id/testuser/",
            avatar="small.jpg",
            avatar_medium="medium.jpg",
            avatar_full="full.jpg",
            persona_state=1,
            community_visibility_state=3
        ))
        mock_steam_service.return_value = mock_service
        
        config_data = {
            "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
            "steam_id": "76561197960435530"
        }
        
        response = client.put("/api/steam/config", json=config_data, headers=auth_headers)
        
        assert_api_success(response)
        data = response.json()
        assert data["has_api_key"] is True
        assert data["api_key_masked"] == "ABCDEF12****7890"
        assert data["steam_id"] == "76561197960435530"
        assert data["is_verified"] is True

    def test_set_steam_config_invalid_api_key(self, client: TestClient, auth_headers, mock_steam_service):
        """Test setting Steam config with invalid API key format."""
        config_data = {
            "web_api_key": "INVALID_API_KEY_WITH_SPECIAL_CHARS!"
        }
        
        response = client.put("/api/steam/config", json=config_data, headers=auth_headers)
        
        assert_api_error(response, 422)  # Pydantic validation error

    def test_set_steam_config_invalid_api_key_business_logic(self, client: TestClient, auth_headers, mock_steam_service):
        """Test setting Steam config with valid format but invalid API key."""
        # Mock Steam service verification to return False
        mock_service = MagicMock()
        mock_service.verify_api_key = AsyncMock(return_value=False)
        mock_steam_service.return_value = mock_service
        
        config_data = {
            "web_api_key": "ABCDEF1234567890ABCDEF1234567890"  # Valid format, invalid key
        }
        
        response = client.put("/api/steam/config", json=config_data, headers=auth_headers)
        
        assert_api_error(response, 400, "Invalid Steam Web API key")  # Business logic error

    def test_delete_steam_config_success(self, client: TestClient, auth_headers, session: Session, test_user: User):
        """Test deleting Steam config successfully."""
        # Set up user with Steam config
        user = session.get(User, test_user.id)
        steam_config = {
            "steam": {
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
                "steam_id": "76561197960435530"
            },
            "other_setting": "value"
        }
        user.preferences_json = json.dumps(steam_config)
        session.commit()
        
        response = client.delete("/api/steam/config", headers=auth_headers)
        
        assert_api_success(response)
        assert "removed successfully" in response.json()["message"]
        
        # Verify Steam config was removed but other preferences remain
        session.refresh(user)
        preferences = user.preferences
        assert "steam" not in preferences
        assert preferences.get("other_setting") == "value"

    def test_verify_steam_config_valid(self, client: TestClient, auth_headers, mock_steam_service):
        """Test verifying valid Steam config."""
        # Mock Steam service
        mock_service = MagicMock()
        mock_service.verify_api_key = AsyncMock(return_value=True)
        mock_service.validate_steam_id = MagicMock(return_value=True)
        mock_service.get_user_info = AsyncMock(return_value=SteamUserInfo(
            steam_id="76561197960435530",
            persona_name="Test User",
            profile_url="https://steamcommunity.com/id/testuser/",
            avatar="small.jpg",
            avatar_medium="medium.jpg",
            avatar_full="full.jpg",
            persona_state=1,
            community_visibility_state=3
        ))
        mock_steam_service.return_value = mock_service
        
        verification_data = {
            "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
            "steam_id": "76561197960435530"
        }
        
        response = client.post("/api/steam/verify", json=verification_data, headers=auth_headers)
        
        assert_api_success(response)
        data = response.json()
        assert data["is_valid"] is True
        assert data["error_message"] is None
        assert data["steam_user_info"] is not None
        assert data["steam_user_info"]["steam_id"] == "76561197960435530"

    def test_verify_steam_config_invalid_api_key_format(self, client: TestClient, auth_headers):
        """Test verifying invalid Steam API key format."""
        verification_data = {
            "web_api_key": "INVALID_API_KEY_WITH_SPECIAL_CHARS!"
        }
        
        response = client.post("/api/steam/verify", json=verification_data, headers=auth_headers)
        
        assert_api_error(response, 422)  # Pydantic validation error
        
    def test_verify_steam_config_invalid_api_key_business_logic(self, client: TestClient, auth_headers, mock_steam_service):
        """Test verifying valid format but invalid Steam API key."""
        # Mock Steam service
        mock_service = MagicMock()
        mock_service.verify_api_key = AsyncMock(return_value=False)
        mock_steam_service.return_value = mock_service
        
        verification_data = {
            "web_api_key": "ABCDEF1234567890ABCDEF1234567890"  # Valid format, invalid key
        }
        
        response = client.post("/api/steam/verify", json=verification_data, headers=auth_headers)
        
        assert_api_success(response)
        data = response.json()
        assert data["is_valid"] is False
        assert "Invalid Steam Web API key" in data["error_message"]


class TestSteamConfigHelpers:
    """Test Steam configuration helper functions."""

    def test_mask_api_key(self):
        """Test API key masking function."""
        from app.api.steam_config import _mask_api_key
        
        api_key = "ABCDEF1234567890ABCDEF1234567890"
        masked = _mask_api_key(api_key)
        
        assert masked == "ABCDEF12****7890"

    def test_mask_api_key_short(self):
        """Test API key masking with short key."""
        from app.api.steam_config import _mask_api_key
        
        api_key = "SHORT"
        masked = _mask_api_key(api_key)
        
        assert masked == "****"
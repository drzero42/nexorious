"""
Tests for Steam Games feature disable functionality.
"""

import pytest
import json
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool

from ..main import app
from ..core.database import get_session
from ..models.user import User


# Test database setup
@pytest.fixture(name="session")
def session_fixture():
    """Create a test database session."""
    engine = create_engine(
        "sqlite:///:memory:",
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
    )
    SQLModel.metadata.create_all(engine)
    with Session(engine) as session:
        yield session


@pytest.fixture(name="client")
def client_fixture(session: Session):
    """Create a test client with the test database session."""
    def get_session_override():
        return session

    app.dependency_overrides[get_session] = get_session_override
    client = TestClient(app)
    yield client
    app.dependency_overrides.clear()


def create_user_with_preferences(client: TestClient, username: str, password: str, preferences: dict = None):
    """Helper function to create a user with specific preferences."""
    # Register user
    register_response = client.post("/api/auth/register", json={
        "username": username,
        "password": password
    })
    assert register_response.status_code == 201
    
    # Login to get token
    login_response = client.post("/api/auth/login", json={
        "username": username,
        "password": password
    })
    assert login_response.status_code == 200
    token = login_response.json()["access_token"]
    
    # Update preferences if provided
    if preferences:
        prefs_response = client.put(
            "/api/auth/me",
            json={"preferences": preferences},
            headers={"Authorization": f"Bearer {token}"}
        )
        assert prefs_response.status_code == 200
    
    return token


class TestSteamGamesFeatureDisable:
    """Test Steam Games feature disable functionality."""
    
    def test_steam_disabled_user_cannot_list_games(self, client: TestClient):
        """Test that users with Steam Games disabled get 404 on list endpoint."""
        token = create_user_with_preferences(client, "disabled_user", "password123", {
            "ui": {"steam_games_visible": False}
        })
        response = client.get(
            "/api/steam-games",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert response.json()["error"] == "Steam Games feature is disabled"
    
    def test_steam_disabled_user_all_endpoints_blocked(self, client: TestClient):
        """Test that users with Steam Games disabled get 404 on all Steam Games endpoints."""
        token = create_user_with_preferences(client, "disabled_user2", "password123", {
            "ui": {"steam_games_visible": False}
        })
        
        # Test all Steam Games endpoints
        endpoints_and_methods = [
            ("GET", "/api/steam-games", {}),
            ("POST", "/api/steam-games/import", {}),
            ("PUT", "/api/steam-games/test-id/match", {"igdb_id": 123}),
            ("POST", "/api/steam-games/test-id/sync", {}),
            ("POST", "/api/steam-games/sync", {}),
            ("PUT", "/api/steam-games/test-id/ignore", {}),
            ("PUT", "/api/steam-games/unignore-all", {}),
            ("PUT", "/api/steam-games/unmatch-all", {}),
            ("PUT", "/api/steam-games/unsync-all", {}),
            ("POST", "/api/steam-games/test-id/unsync", {}),
            ("POST", "/api/steam-games/auto-match", {}),
            ("POST", "/api/steam-games/test-id/auto-match", {}),
        ]
        
        for method, url, json_data in endpoints_and_methods:
            if method == "GET":
                response = client.get(url, headers={"Authorization": f"Bearer {token}"})
            elif method == "POST":
                response = client.post(url, json=json_data, headers={"Authorization": f"Bearer {token}"})
            elif method == "PUT":
                response = client.put(url, json=json_data, headers={"Authorization": f"Bearer {token}"})
            
            assert response.status_code == 404, f"Expected 404 for {method} {url}, got {response.status_code}"
            assert response.json()["error"] == "Steam Games feature is disabled"
    
    def test_steam_enabled_user_can_access_endpoints(self, client: TestClient):
        """Test that users with Steam Games enabled can access endpoints (even if they fail for other reasons)."""
        token = create_user_with_preferences(client, "enabled_user", "password123", {
            "ui": {"steam_games_visible": True}
        })
        response = client.get(
            "/api/steam-games",
            headers={"Authorization": f"Bearer {token}"}
        )
        # Should not be 404 (feature disabled), might be other errors like Steam not configured
        assert response.status_code != 404 or response.json().get("error") != "Steam Games feature is disabled"
    
    def test_user_with_no_preferences_defaults_to_enabled(self, client: TestClient):
        """Test that users with no preferences default to Steam Games enabled."""
        token = create_user_with_preferences(client, "no_prefs_user", "password123")
        response = client.get(
            "/api/steam-games",
            headers={"Authorization": f"Bearer {token}"}
        )
        # Should not be 404 (feature disabled), might be other errors like Steam not configured
        assert response.status_code != 404 or response.json().get("error") != "Steam Games feature is disabled"


class TestPreferencesHandling:
    """Test preferences handling for Steam Games visibility."""
    
    def test_user_can_disable_steam_games(self, client: TestClient):
        """Test that users can disable Steam Games through preferences update."""
        token = create_user_with_preferences(client, "toggle_user1", "password123", {
            "ui": {"steam_games_visible": True}
        })
        
        # First verify Steam Games is accessible
        response = client.get(
            "/api/steam-games",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code != 404 or response.json().get("error") != "Steam Games feature is disabled"
        
        # Update preferences to disable Steam Games
        response = client.put(
            "/api/auth/me",
            json={
                "preferences": {
                    "ui": {
                        "steam_games_visible": False
                    }
                }
            },
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 200
        
        # Now Steam Games should be disabled
        response = client.get(
            "/api/steam-games",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert response.json()["error"] == "Steam Games feature is disabled"
    
    def test_user_can_enable_steam_games(self, client: TestClient):
        """Test that users can enable Steam Games through preferences update."""
        token = create_user_with_preferences(client, "toggle_user2", "password123", {
            "ui": {"steam_games_visible": False}
        })
        
        # First verify Steam Games is disabled
        response = client.get(
            "/api/steam-games",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert response.json()["error"] == "Steam Games feature is disabled"
        
        # Update preferences to enable Steam Games
        response = client.put(
            "/api/auth/me",
            json={
                "preferences": {
                    "ui": {
                        "steam_games_visible": True
                    }
                }
            },
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 200
        
        # Now Steam Games should be accessible
        response = client.get(
            "/api/steam-games",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code != 404 or response.json().get("error") != "Steam Games feature is disabled"
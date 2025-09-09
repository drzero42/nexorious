"""
Tests for Steam Games feature disable functionality.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool

from ..main import app
from ..core.database import get_session
from ..models.platform import Platform, Storefront


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
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert response.json()["error"] == "Steam Games feature is disabled"
    
    def test_steam_disabled_user_all_endpoints_blocked(self, client: TestClient):
        """Test that users with Steam Games disabled get 404 on all Steam Games endpoints."""
        token = create_user_with_preferences(client, "disabled_user2", "password123", {
            "ui": {"steam_games_visible": False}
        })
        
        # Test all Steam endpoints that use verify_steam_games_enabled dependency
        endpoints_and_methods = [
            # Configuration endpoints
            ("GET", "/api/import/sources/steam/config", {}),
            ("PUT", "/api/import/sources/steam/config", {"web_api_key": "test_key", "steam_id": "76561197960435530"}),
            ("DELETE", "/api/import/sources/steam/config", {}),
            ("POST", "/api/import/sources/steam/verify", {"web_api_key": "test_key", "steam_id": "76561197960435530"}),
            ("POST", "/api/import/sources/steam/resolve-vanity", {"vanity_url": "testuser"}),
            
            # Library and games endpoints
            ("GET", "/api/import/sources/steam/library", {}),
            ("GET", "/api/import/sources/steam/games", {}),
            ("POST", "/api/import/sources/steam/games/import", {}),
            
            # Individual game operations
            ("PUT", "/api/import/sources/steam/games/test-id/match", {"igdb_id": 123}),
            ("POST", "/api/import/sources/steam/games/test-id/auto-match", {}),
            ("POST", "/api/import/sources/steam/games/test-id/sync", {}),
            ("POST", "/api/import/sources/steam/games/test-id/unsync", {}),
            ("PUT", "/api/import/sources/steam/games/test-id/ignore", {}),
            
            # Bulk operations
            ("POST", "/api/import/sources/steam/games/auto-match", {}),
            ("POST", "/api/import/sources/steam/games/sync", {}),
            ("POST", "/api/import/sources/steam/games/unsync", {}),
            ("PUT", "/api/import/sources/steam/games/unignore-all", {}),
            ("PUT", "/api/import/sources/steam/games/unmatch-all", {}),
            
            # Modern batch operations
            ("POST", "/api/import/sources/steam/batch/auto-match/start", {"session_type": "steam"}),
            ("POST", "/api/import/sources/steam/batch/sync/start", {"session_type": "steam"}),
            ("GET", "/api/import/sources/steam/batch/test-session-id/status", {}),
            ("DELETE", "/api/import/sources/steam/batch/test-session-id", {}),
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
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        # Should not be 404 (feature disabled), might be other errors like Steam not configured
        assert response.status_code != 404 or response.json().get("error") != "Steam Games feature is disabled"
    
    def test_user_with_no_preferences_defaults_to_enabled(self, client: TestClient):
        """Test that users with no preferences default to Steam Games enabled."""
        token = create_user_with_preferences(client, "no_prefs_user", "password123")
        response = client.get(
            "/api/import/sources/steam/config",
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
            "/api/import/sources/steam/config",
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
            "/api/import/sources/steam/config",
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
            "/api/import/sources/steam/config",
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
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code != 404 or response.json().get("error") != "Steam Games feature is disabled"


def create_platform_and_storefront(session: Session, platform_active: bool = True, storefront_active: bool = True):
    """Helper function to create PC-Windows platform and Steam storefront for testing."""
    # Create PC-Windows platform
    platform = Platform(
        name="pc-windows",
        display_name="PC (Windows)",
        is_active=platform_active,
        source="official"
    )
    session.add(platform)
    
    # Create Steam storefront
    storefront = Storefront(
        name="steam",
        display_name="Steam",
        is_active=storefront_active,
        source="official"
    )
    session.add(storefront)
    
    session.commit()
    session.refresh(platform)
    session.refresh(storefront)
    
    return platform, storefront


class TestSteamGamesPlatformStorefrontValidation:
    """Test Steam Games platform and storefront dependency validation."""
    
    def test_steam_enabled_with_active_dependencies(self, client: TestClient, session: Session):
        """Test that Steam Games works when platform and storefront are active."""
        # Create active platform and storefront
        create_platform_and_storefront(session, platform_active=True, storefront_active=True)
        
        token = create_user_with_preferences(client, "active_deps_user", "password123", {
            "ui": {"steam_games_visible": True}
        })
        
        response = client.get(
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        # Should not be 404 due to platform/storefront issues
        assert response.status_code != 404 or "platform" not in response.json().get("error", "").lower()
        assert response.status_code != 404 or "storefront" not in response.json().get("error", "").lower()
    
    def test_steam_disabled_inactive_platform(self, client: TestClient, session: Session):
        """Test that Steam Games is disabled when PC-Windows platform is inactive."""
        # Create inactive platform, active storefront
        create_platform_and_storefront(session, platform_active=False, storefront_active=True)
        
        token = create_user_with_preferences(client, "inactive_platform_user", "password123", {
            "ui": {"steam_games_visible": True}
        })
        
        response = client.get(
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert "PC-Windows platform is inactive" in response.json()["error"]
    
    def test_steam_disabled_inactive_storefront(self, client: TestClient, session: Session):
        """Test that Steam Games is disabled when Steam storefront is inactive."""
        # Create active platform, inactive storefront
        create_platform_and_storefront(session, platform_active=True, storefront_active=False)
        
        token = create_user_with_preferences(client, "inactive_storefront_user", "password123", {
            "ui": {"steam_games_visible": True}
        })
        
        response = client.get(
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert "Steam storefront is inactive" in response.json()["error"]
    
    def test_steam_disabled_both_dependencies_inactive(self, client: TestClient, session: Session):
        """Test that Steam Games is disabled when both platform and storefront are inactive."""
        # Create inactive platform and storefront
        create_platform_and_storefront(session, platform_active=False, storefront_active=False)
        
        token = create_user_with_preferences(client, "both_inactive_user", "password123", {
            "ui": {"steam_games_visible": True}
        })
        
        response = client.get(
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        # Should fail on platform check first
        assert "PC-Windows platform is inactive" in response.json()["error"]
    
    def test_steam_disabled_missing_platform(self, client: TestClient, session: Session):
        """Test that Steam Games is disabled when PC-Windows platform doesn't exist."""
        # Create only storefront, no platform
        storefront = Storefront(
            name="steam",
            display_name="Steam",
            is_active=True,
            source="official"
        )
        session.add(storefront)
        session.commit()
        
        token = create_user_with_preferences(client, "missing_platform_user", "password123", {
            "ui": {"steam_games_visible": True}
        })
        
        response = client.get(
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert "PC-Windows platform not found" in response.json()["error"]
    
    def test_steam_disabled_missing_storefront(self, client: TestClient, session: Session):
        """Test that Steam Games is disabled when Steam storefront doesn't exist."""
        # Create only platform, no storefront
        platform = Platform(
            name="pc-windows",
            display_name="PC (Windows)",
            is_active=True,
            source="official"
        )
        session.add(platform)
        session.commit()
        
        token = create_user_with_preferences(client, "missing_storefront_user", "password123", {
            "ui": {"steam_games_visible": True}
        })
        
        response = client.get(
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert "Steam storefront not found" in response.json()["error"]
    
    def test_steam_disabled_ui_preference_overrides_active_dependencies(self, client: TestClient, session: Session):
        """Test that UI preference takes precedence over active platform/storefront."""
        # Create active platform and storefront
        create_platform_and_storefront(session, platform_active=True, storefront_active=True)
        
        token = create_user_with_preferences(client, "ui_disabled_user", "password123", {
            "ui": {"steam_games_visible": False}
        })
        
        response = client.get(
            "/api/import/sources/steam/config",
            headers={"Authorization": f"Bearer {token}"}
        )
        assert response.status_code == 404
        assert response.json()["error"] == "Steam Games feature is disabled"
    
    def test_all_steam_endpoints_respect_platform_storefront_validation(self, client: TestClient, session: Session):
        """Test that all Steam Games endpoints respect platform/storefront validation."""
        # Create inactive platform
        create_platform_and_storefront(session, platform_active=False, storefront_active=True)
        
        token = create_user_with_preferences(client, "all_endpoints_user", "password123", {
            "ui": {"steam_games_visible": True}
        })
        
        # Test key Steam endpoints that use verify_steam_games_enabled dependency - should all fail with platform validation
        endpoints_and_methods = [
            ("GET", "/api/import/sources/steam/config", {}),
            ("GET", "/api/import/sources/steam/games", {}),
            ("POST", "/api/import/sources/steam/games/auto-match", {}),
            ("POST", "/api/import/sources/steam/batch/auto-match/start", {"session_type": "steam"}),
        ]
        
        for method, url, json_data in endpoints_and_methods:
            if method == "GET":
                response = client.get(url, headers={"Authorization": f"Bearer {token}"})
            elif method == "POST":
                response = client.post(url, json=json_data, headers={"Authorization": f"Bearer {token}"})
            elif method == "PUT":
                response = client.put(url, json=json_data, headers={"Authorization": f"Bearer {token}"})
            
            assert response.status_code == 404, f"Expected 404 for {method} {url}, got {response.status_code}"
            assert "PC-Windows platform is inactive" in response.json()["error"]
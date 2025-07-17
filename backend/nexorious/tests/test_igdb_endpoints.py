"""
Tests for IGDB API endpoints.
"""

import pytest
from unittest.mock import AsyncMock
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from sqlalchemy.pool import StaticPool

from nexorious.main import app
from nexorious.core.database import get_session
from nexorious.core.security import get_current_user
from nexorious.api.dependencies import get_igdb_service_dependency
from nexorious.services.igdb import GameMetadata, IGDBService, TwitchAuthError, IGDBError


@pytest.fixture
def client():
    """Create test client with in-memory database."""
    # Create in-memory SQLite database for testing with threading support
    engine = create_engine(
        "sqlite:///:memory:", 
        echo=False,
        connect_args={"check_same_thread": False},
        poolclass=StaticPool
    )
    SQLModel.metadata.create_all(engine)
    
    def get_test_session():
        with Session(engine) as session:
            yield session
    
    # Override the database dependency
    app.dependency_overrides[get_session] = get_test_session
    
    client = TestClient(app)
    yield client
    
    # Clean up
    app.dependency_overrides.clear()


def get_mock_user():
    """Create a mock user for testing."""
    from nexorious.models.user import User
    return User(
        id="test-user-id",
        username="testuser",
        email="test@example.com",
        password_hash="hashed_password",
        is_active=True,
        is_admin=False
    )


def create_mock_igdb_service(search_results=None, get_game_result=None, error=None):
    """Create a mock IGDB service with configurable responses."""
    mock_igdb = AsyncMock()
    
    if error:
        mock_igdb.search_games.side_effect = error
        mock_igdb.get_game_by_id.side_effect = error
    else:
        mock_igdb.search_games.return_value = search_results or []
        mock_igdb.get_game_by_id.return_value = get_game_result
    
    return mock_igdb


class TestIGDBSearchEndpoint:
    """Test cases for IGDB search endpoint."""
    
    def test_search_igdb_success(self, client):
        """Test successful IGDB search."""
        search_results = [
            GameMetadata(
                igdb_id="123",
                title="Test Game",
                description="A test game for PC and PlayStation",
                genre="Action",
                developer="Test Studio",
                publisher="Test Publisher",
                release_date="2023-01-01",
                cover_art_url="https://example.com/cover.jpg"
            )
        ]
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(search_results=search_results)
            
            response = client.post(
                "/api/games/search/igdb",
                json={"query": "Test Game", "limit": 10}
            )
            
            assert response.status_code == 200
            data = response.json()
            assert data["total"] == 1
            assert len(data["games"]) == 1
            
            candidate = data["games"][0]
            assert candidate["igdb_id"] == "123"
            assert candidate["title"] == "Test Game"
            assert candidate["description"] == "A test game for PC and PlayStation"
            assert "PC" in candidate["platforms"]
            assert "PlayStation" in candidate["platforms"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_search_igdb_authentication_error(self, client):
        """Test IGDB search with authentication error."""
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(error=TwitchAuthError("Authentication failed"))
            
            response = client.post(
                "/api/games/search/igdb",
                json={"query": "Test Game"}
            )
            
            assert response.status_code == 503
            assert "IGDB authentication failed" in response.json()["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_search_igdb_api_error(self, client):
        """Test IGDB search with API error."""
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(error=IGDBError("API error"))
            
            response = client.post(
                "/api/games/search/igdb",
                json={"query": "Test Game"}
            )
            
            assert response.status_code == 502
            assert "IGDB API error" in response.json()["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_search_igdb_no_results(self, client):
        """Test IGDB search with no results."""
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(search_results=[])
            
            response = client.post(
                "/api/games/search/igdb",
                json={"query": "Nonexistent Game"}
            )
            
            assert response.status_code == 200
            data = response.json()
            assert data["total"] == 0
            assert len(data["games"]) == 0
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_search_igdb_platform_extraction(self, client):
        """Test platform extraction from game description."""
        search_results = [
            GameMetadata(
                igdb_id="123",
                title="Test Game",
                description="Available on PC, PlayStation 5, Xbox Series X/S and Nintendo Switch",
                genre="Action"
            )
        ]
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(search_results=search_results)
            
            response = client.post(
                "/api/games/search/igdb",
                json={"query": "Test Game"}
            )
            
            assert response.status_code == 200
            data = response.json()
            
            candidate = data["games"][0]
            platforms = candidate["platforms"]
            
            # Should extract multiple platforms from description
            assert "PC" in platforms
            assert "PlayStation" in platforms
            assert "Xbox" in platforms
            assert "Nintendo" in platforms
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)


class TestIGDBImportEndpoint:
    """Test cases for IGDB import endpoint."""
    
    def test_import_from_igdb_success(self, client):
        """Test successful game import from IGDB."""
        game_metadata = GameMetadata(
            igdb_id="123",
            title="Test Game",
            description="A test game",
            genre="Action",
            developer="Test Studio",
            publisher="Test Publisher",
            release_date="2023-01-01",
            cover_art_url="https://example.com/cover.jpg",
            rating_average=8.5,
            rating_count=100,
            estimated_playtime_hours=40
        )
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(get_game_result=game_metadata)
            
            response = client.post(
                "/api/games/igdb-import",
                json={
                    "igdb_id": "123",
                    "accept_metadata": True,
                    "custom_overrides": {}
                }
            )
            
            assert response.status_code == 201
            data = response.json()
            assert data["title"] == "Test Game"
            assert data["igdb_id"] == "123"
            assert data["is_verified"] is True
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_import_from_igdb_not_found(self, client):
        """Test import when game not found in IGDB."""
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(get_game_result=None)
            
            response = client.post(
                "/api/games/igdb-import",
                json={
                    "igdb_id": "nonexistent",
                    "accept_metadata": True
                }
            )
            
            print(f"Debug response: {response.status_code} - {response.json()}")
            assert response.status_code == 404
            assert "Game not found in IGDB" in response.json()["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_import_from_igdb_with_overrides(self, client):
        """Test import with custom overrides."""
        game_metadata = GameMetadata(
            igdb_id="123",
            title="Test Game",
            description="Original description",
            genre="Action"
        )
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(get_game_result=game_metadata)
            
            response = client.post(
                "/api/games/igdb-import",
                json={
                    "igdb_id": "123",
                    "accept_metadata": True,
                    "custom_overrides": {
                        "description": "Custom description",
                        "genre": "Custom Genre"
                    }
                }
            )
            
            assert response.status_code == 201
            data = response.json()
            assert data["title"] == "Test Game"
            assert data["description"] == "Custom description"
            assert data["genre"] == "Custom Genre"
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
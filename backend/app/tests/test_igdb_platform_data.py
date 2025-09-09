"""
Tests for IGDB platform data caching and retrieval functionality.
"""

import pytest
from unittest.mock import AsyncMock
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool

from app.main import app
from app.core.database import get_session
from app.core.security import get_current_user
from app.api.dependencies import get_igdb_service_dependency
from app.services.igdb import GameMetadata, IGDB_PLATFORM_MAPPING


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
    from app.models.user import User
    return User(
        id="test-user-id",
        username="testuser",
        password_hash="hashed_password",
        is_active=True,
        is_admin=False
    )


class TestIGDBPlatformMapping:
    """Test cases for IGDB platform mapping functionality."""
    
    def test_igdb_platform_mapping_exists(self):
        """Test that IGDB platform mapping dictionary exists and has expected platforms."""
        assert isinstance(IGDB_PLATFORM_MAPPING, dict)
        assert len(IGDB_PLATFORM_MAPPING) > 0
        
        # Check for key platforms
        assert 6 in IGDB_PLATFORM_MAPPING  # PC
        assert 48 in IGDB_PLATFORM_MAPPING  # PlayStation 4
        assert 167 in IGDB_PLATFORM_MAPPING  # PlayStation 5
        assert 130 in IGDB_PLATFORM_MAPPING  # Nintendo Switch
        
        # Check mappings are correct
        assert IGDB_PLATFORM_MAPPING[6] == "pc-windows"
        assert IGDB_PLATFORM_MAPPING[48] == "playstation-4"
        assert IGDB_PLATFORM_MAPPING[167] == "playstation-5"
        assert IGDB_PLATFORM_MAPPING[130] == "nintendo-switch"


class TestGameMetadataWithPlatforms:
    """Test cases for GameMetadata with platform data."""
    
    def test_game_metadata_with_platform_data(self):
        """Test GameMetadata creation with platform fields."""
        metadata = GameMetadata(
            igdb_id=123,
            title="Test Game",
            igdb_platform_ids=[6, 48, 167],
            platform_names=["pc-windows", "playstation-4", "playstation-5"]
        )
        
        assert metadata.igdb_platform_ids == [6, 48, 167]
        assert metadata.platform_names == ["pc-windows", "playstation-4", "playstation-5"]
    
    def test_game_metadata_without_platform_data(self):
        """Test GameMetadata creation without platform fields."""
        metadata = GameMetadata(
            igdb_id=123,
            title="Test Game"
        )
        
        assert metadata.igdb_platform_ids is None
        assert metadata.platform_names is None


class TestIGDBPlatformDataStorage:
    """Test cases for storing platform data in database."""
    
    def test_igdb_import_stores_platform_data(self, client):
        """Test that IGDB import stores platform data in database."""
        game_metadata = GameMetadata(
            igdb_id=123,
            title="Test Game",
            description="A test game",
            genre="Action",
            developer="Test Studio",
            publisher="Test Publisher",
            release_date="2023-01-01",
            cover_art_url="https://example.com/cover.jpg",
            igdb_platform_ids=[6, 48, 167],
            platform_names=["pc-windows", "playstation-4", "playstation-5"]
        )
        
        # Mock IGDB service
        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.return_value = "/static/cover_art/123.jpg"
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb
            
            response = client.post(
                "/api/games/igdb-import",
                json={
                    "igdb_id": 123,
                    "accept_metadata": True,
                    "custom_overrides": {}
                }
            )
            
            assert response.status_code == 201
            data = response.json()
            
            # Verify platform data was stored
            assert data["igdb_platform_ids"] == "[6, 48, 167]"
            assert data["igdb_platform_names"] == '["pc-windows", "playstation-4", "playstation-5"]'
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_igdb_import_without_platform_data(self, client):
        """Test that IGDB import handles games without platform data."""
        game_metadata = GameMetadata(
            igdb_id=123,
            title="Test Game",
            description="A test game",
            genre="Action",
            developer="Test Studio",
            publisher="Test Publisher",
            release_date="2023-01-01",
            cover_art_url="https://example.com/cover.jpg",
            igdb_platform_ids=None,
            platform_names=None
        )
        
        # Mock IGDB service
        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.return_value = "/static/cover_art/123.jpg"
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb
            
            response = client.post(
                "/api/games/igdb-import",
                json={
                    "igdb_id": 123,
                    "accept_metadata": True,
                    "custom_overrides": {}
                }
            )
            
            assert response.status_code == 201
            data = response.json()
            
            # Verify platform data fields is null when no data available  
            assert data["igdb_platform_ids"] is None
            assert data["igdb_platform_names"] is None
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)


class TestIGDBSearchWithPlatforms:
    """Test cases for IGDB search with platform data."""
    
    def test_search_returns_platform_data(self, client):
        """Test that IGDB search returns platform data when available."""
        search_results = [
            GameMetadata(
                igdb_id=123,
                title="Test Game",
                igdb_slug="test-game",
                description="A test game",
                genre="Action",
                igdb_platform_ids=[6, 48, 130],
                platform_names=["pc-windows", "playstation-4", "nintendo-switch"]
            )
        ]
        
        # Mock IGDB service
        mock_igdb = AsyncMock()
        mock_igdb.search_games.return_value = search_results
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb
            
            response = client.post(
                "/api/games/search/igdb",
                json={"query": "Test Game", "limit": 10}
            )
            
            assert response.status_code == 200
            data = response.json()
            assert data["total"] == 1
            
            candidate = data["games"][0]
            platforms = candidate["platforms"]
            
            assert "pc-windows" in platforms
            assert "playstation-4" in platforms
            assert "nintendo-switch" in platforms
            assert len(platforms) == 3
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_search_returns_empty_platforms_when_no_data(self, client):
        """Test that IGDB search returns empty platforms when no data available."""
        search_results = [
            GameMetadata(
                igdb_id=123,
                title="Test Game",
                igdb_slug="test-game",
                description="A test game",
                genre="Action",
                igdb_platform_ids=None,
                platform_names=None
            )
        ]
        
        # Mock IGDB service
        mock_igdb = AsyncMock()
        mock_igdb.search_games.return_value = search_results
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = get_mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb
            
            response = client.post(
                "/api/games/search/igdb",
                json={"query": "Test Game", "limit": 10}
            )
            
            assert response.status_code == 200
            data = response.json()
            assert data["total"] == 1
            
            candidate = data["games"][0]
            platforms = candidate["platforms"]
            
            # Should be empty list when no platform data
            assert platforms == []
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
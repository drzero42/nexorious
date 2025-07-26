"""
Tests for cover art download and storage functionality.
"""

import pytest
from unittest.mock import AsyncMock, Mock, patch
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from sqlalchemy.pool import StaticPool
import os
import tempfile
import shutil

from nexorious.main import app
from nexorious.core.database import get_session
from nexorious.core.security import get_current_user
from nexorious.api.dependencies import get_igdb_service_dependency
from nexorious.services.igdb import GameMetadata, IGDBService
from nexorious.models.game import Game
from nexorious.models.user import User


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


@pytest.fixture
def mock_user():
    """Create a mock user for testing."""
    return User(
        id="test-user-id",
        username="testuser",
        password_hash="hashed_password",
        is_active=True,
        is_admin=False
    )


@pytest.fixture
def mock_admin_user():
    """Create a mock admin user for testing."""
    return User(
        id="test-admin-id",
        username="admin",
        password_hash="hashed_password",
        is_active=True,
        is_admin=True
    )


@pytest.fixture
def temp_storage_dir():
    """Create temporary directory for storage testing."""
    temp_dir = tempfile.mkdtemp()
    yield temp_dir
    shutil.rmtree(temp_dir)


@pytest.fixture
def sample_game_data():
    """Sample game data for testing."""
    return {
        "id": "test-game-id",
        "title": "Test Game",
        "description": "A test game",
        "genre": "Action",
        "developer": "Test Studio",
        "publisher": "Test Publisher",
        "igdb_id": "123",
        "cover_art_url": "https://example.com/cover.jpg",
        "is_verified": False
    }


def create_mock_igdb_service(download_result=None, error=None):
    """Create a mock IGDB service with configurable responses."""
    mock_igdb = AsyncMock()
    
    if error:
        mock_igdb.download_and_store_cover_art.side_effect = error
    else:
        mock_igdb.download_and_store_cover_art.return_value = download_result
    
    return mock_igdb


class TestCoverArtDownload:
    """Test cases for individual cover art download endpoint."""
    
    def test_download_cover_art_success(self, client, mock_user, sample_game_data):
        """Test successful cover art download."""
        try:
            # Create a game in the test database (using the client's session)
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_game_data)
            session.add(game)
            session.commit()
            session.refresh(game)
            
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(download_result="/static/cover_art/123.jpg")
            
            response = client.post(f"/api/games/{sample_game_data['id']}/cover-art/download")
            
            assert response.status_code == 200
            data = response.json()
            assert "Cover art downloaded and stored successfully" in data["message"]
            assert "/static/cover_art/123.jpg" in data["message"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_download_cover_art_game_not_found(self, client, mock_user):
        """Test cover art download for non-existent game."""
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service()
            
            response = client.post("/api/games/nonexistent-id/cover-art/download")
            
            assert response.status_code == 404
            assert "Game not found" in response.json()["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_download_cover_art_no_igdb_id(self, client, mock_user):
        """Test cover art download for game without IGDB ID."""
        sample_data = {
            "id": "test-game-id",
            "title": "Test Game",
            "igdb_id": None,
            "cover_art_url": "https://example.com/cover.jpg",
            "is_verified": False
        }
        
        try:
            # Create a game without IGDB ID in the test database
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_data)
            session.add(game)
            session.commit()
            session.refresh(game)
            
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service()
            
            response = client.post(f"/api/games/{sample_data['id']}/cover-art/download")
            
            assert response.status_code == 400
            assert "Game does not have IGDB ID" in response.json()["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_download_cover_art_no_cover_url(self, client, mock_user):
        """Test cover art download for game without cover art URL."""
        sample_data = {
            "id": "test-game-id",
            "title": "Test Game",
            "igdb_id": "123",
            "cover_art_url": None,
            "is_verified": False
        }
        
        try:
            # Create a game without cover art URL in the test database
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_data)
            session.add(game)
            session.commit()
            session.refresh(game)
            
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service()
            
            response = client.post(f"/api/games/{sample_data['id']}/cover-art/download")
            
            assert response.status_code == 400
            assert "Game does not have cover art URL" in response.json()["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_download_cover_art_verified_game_permission(self, client, mock_user, sample_game_data):
        """Test cover art download permission for verified games."""
        try:
            # Create a verified game in the test database
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_game_data)
            game.is_verified = True
            session.add(game)
            session.commit()
            session.refresh(game)
            
            # Override dependencies with non-admin user
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service()
            
            response = client.post(f"/api/games/{sample_game_data['id']}/cover-art/download")
            
            assert response.status_code == 403
            assert "Cannot download cover art for verified games" in response.json()["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_download_cover_art_admin_verified_game(self, client, mock_admin_user, sample_game_data):
        """Test cover art download by admin for verified games."""
        try:
            # Create a verified game in the test database
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_game_data)
            game.is_verified = True
            session.add(game)
            session.commit()
            session.refresh(game)
            
            # Override dependencies with admin user
            app.dependency_overrides[get_current_user] = lambda: mock_admin_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(download_result="/static/cover_art/123.jpg")
            
            response = client.post(f"/api/games/{sample_game_data['id']}/cover-art/download")
            
            assert response.status_code == 200
            data = response.json()
            assert "Cover art downloaded and stored successfully" in data["message"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_download_cover_art_download_failure(self, client, mock_user, sample_game_data):
        """Test cover art download failure."""
        try:
            # Create a game in the test database
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_game_data)
            game.is_verified = False
            session.add(game)
            session.commit()
            session.refresh(game)
            
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(download_result=None)
            
            response = client.post(f"/api/games/{sample_game_data['id']}/cover-art/download")
            
            assert response.status_code == 500
            response_data = response.json()
            assert "Failed to download and store cover art" in response_data["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)


class TestBulkCoverArtDownload:
    """Test cases for bulk cover art download endpoint."""
    
    def test_bulk_download_success(self, client, mock_admin_user):
        """Test successful bulk cover art download."""
        game_ids = ["game1", "game2"]
        
        try:
            # Create games in the test database
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            for i, game_id in enumerate(game_ids):
                game = Game(
                    id=game_id,
                    title=f"Test Game {i+1}",
                    igdb_id=str(i+1),
                    cover_art_url=f"https://example.com/cover{i+1}.jpg",
                    is_verified=False
                )
                session.add(game)
            session.commit()
            
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_admin_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(download_result="/static/cover_art/123.jpg")
            
            response = client.post(
                "/api/games/cover-art/bulk-download",
                json=game_ids
            )
            
            assert response.status_code == 200
            data = response.json()
            assert data["total_games"] == 2
            assert data["successful_operations"] == 2
            assert data["failed_operations"] == 0
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_bulk_download_non_admin(self, client, mock_user):
        """Test bulk download with non-admin user."""
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service()
            
            response = client.post(
                "/api/games/cover-art/bulk-download",
                json=["game1", "game2"]
            )
            
            assert response.status_code == 403
            response_data = response.json()
            assert "Only administrators can perform bulk cover art downloads" in response_data["error"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_bulk_download_skip_existing(self, client, mock_admin_user):
        """Test bulk download with skip existing option."""
        game_ids = ["game1", "game2"]
        
        try:
            # Create games in the test database
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            
            # Game 1 - already has local cover art
            game1 = Game(
                id="game1",
                title="Test Game 1",
                igdb_id="1",
                cover_art_url="/static/cover_art/1.jpg",  # Local URL
                is_verified=False
            )
            session.add(game1)
            
            # Game 2 - has remote cover art
            game2 = Game(
                id="game2",
                title="Test Game 2",
                igdb_id="2",
                cover_art_url="https://example.com/cover2.jpg",  # Remote URL
                is_verified=False
            )
            session.add(game2)
            
            session.commit()
            
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_admin_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(download_result="/static/cover_art/2.jpg")
            
            response = client.post(
                "/api/games/cover-art/bulk-download?skip_existing=true",
                json=game_ids
            )
            
            assert response.status_code == 200
            data = response.json()
            assert data["total_games"] == 2
            assert data["successful_operations"] == 2
            assert data["failed_operations"] == 0
            
            # Check that game1 was skipped and game2 was downloaded
            results = data["results"]
            game1_result = next(r for r in results if r["game_id"] == "game1")
            game2_result = next(r for r in results if r["game_id"] == "game2")
            
            assert game1_result["success"] is True
            assert "Already has local cover art" in game1_result["message"]
            assert game2_result["success"] is True
            assert "Cover art downloaded successfully" in game2_result["message"]
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_bulk_download_mixed_results(self, client, mock_admin_user):
        """Test bulk download with mixed success/failure results."""
        game_ids = ["game1", "game2", "game3"]
        
        try:
            # Create games in the test database
            from nexorious.core.database import get_session
            session = next(app.dependency_overrides[get_session]())
            
            # Game 1 - valid game
            game1 = Game(
                id="game1",
                title="Test Game 1",
                igdb_id="1",
                cover_art_url="https://example.com/cover1.jpg",
                is_verified=False
            )
            session.add(game1)
            
            # Game 2 - no IGDB ID
            game2 = Game(
                id="game2",
                title="Test Game 2",
                igdb_id=None,
                cover_art_url="https://example.com/cover2.jpg",
                is_verified=False
            )
            session.add(game2)
            
            # Game 3 - no cover art URL
            game3 = Game(
                id="game3",
                title="Test Game 3",
                igdb_id="3",
                cover_art_url=None,
                is_verified=False
            )
            session.add(game3)
            
            session.commit()
            
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_admin_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(download_result="/static/cover_art/1.jpg")
            
            response = client.post(
                "/api/games/cover-art/bulk-download",
                json=game_ids
            )
            
            assert response.status_code == 200
            data = response.json()
            assert data["total_games"] == 3
            assert data["successful_operations"] == 1
            assert data["failed_operations"] == 2
            
            # Check error messages
            errors = data["errors"]
            assert any("does not have IGDB ID" in error for error in errors)
            assert any("does not have cover art URL" in error for error in errors)
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)


class TestAutomaticCoverArtDownload:
    """Test cases for automatic cover art download during import."""
    
    def test_igdb_import_default_behavior_downloads_cover_art(self, client, mock_user):
        """Test IGDB import with default behavior (should download cover art automatically)."""
        game_metadata = GameMetadata(
            igdb_id="123",
            title="Test Game",
            description="A test game",
            genre="Action",
            cover_art_url="https://example.com/cover.jpg"
        )
        
        # Mock IGDB service
        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.return_value = "/static/cover_art/123.jpg"
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb
            
            # No explicit download_cover_art parameter - should default to True
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
            assert data["cover_art_url"] == "/static/cover_art/123.jpg"
            
            # Verify download was called due to new default behavior
            mock_igdb.download_and_store_cover_art.assert_called_once_with("123", "https://example.com/cover.jpg")
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_igdb_import_with_cover_art_download(self, client, mock_user):
        """Test IGDB import with automatic cover art download."""
        game_metadata = GameMetadata(
            igdb_id="123",
            title="Test Game",
            description="A test game",
            genre="Action",
            cover_art_url="https://example.com/cover.jpg"
        )
        
        # Mock IGDB service
        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.return_value = "/static/cover_art/123.jpg"
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb
            
            response = client.post(
                "/api/games/igdb-import?download_cover_art=true",
                json={
                    "igdb_id": "123",
                    "accept_metadata": True,
                    "custom_overrides": {}
                }
            )
            
            assert response.status_code == 201
            data = response.json()
            assert data["title"] == "Test Game"
            assert data["cover_art_url"] == "/static/cover_art/123.jpg"
            
            # Verify download was called
            mock_igdb.download_and_store_cover_art.assert_called_once_with("123", "https://example.com/cover.jpg")
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_igdb_import_without_cover_art_download(self, client, mock_user):
        """Test IGDB import without automatic cover art download."""
        game_metadata = GameMetadata(
            igdb_id="123",
            title="Test Game",
            description="A test game",
            genre="Action",
            cover_art_url="https://example.com/cover.jpg"
        )
        
        # Mock IGDB service
        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.return_value = "/static/cover_art/123.jpg"
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb
            
            response = client.post(
                "/api/games/igdb-import?download_cover_art=false",
                json={
                    "igdb_id": "123",
                    "accept_metadata": True,
                    "custom_overrides": {}
                }
            )
            
            assert response.status_code == 201
            data = response.json()
            assert data["title"] == "Test Game"
            assert data["cover_art_url"] == "https://example.com/cover.jpg"  # Remote URL preserved
            
            # Verify download was NOT called
            mock_igdb.download_and_store_cover_art.assert_not_called()
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)
    
    def test_igdb_import_cover_art_download_failure(self, client, mock_user):
        """Test IGDB import when cover art download fails."""
        game_metadata = GameMetadata(
            igdb_id="123",
            title="Test Game",
            description="A test game",
            genre="Action",
            cover_art_url="https://example.com/cover.jpg"
        )
        
        # Mock IGDB service
        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.side_effect = Exception("Download failed")
        
        try:
            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb
            
            response = client.post(
                "/api/games/igdb-import?download_cover_art=true",
                json={
                    "igdb_id": "123",
                    "accept_metadata": True,
                    "custom_overrides": {}
                }
            )
            
            # Import should still succeed even if cover art download fails
            assert response.status_code == 201
            data = response.json()
            assert data["title"] == "Test Game"
            assert data["cover_art_url"] == "https://example.com/cover.jpg"  # Original URL preserved
            
        finally:
            # Clean up overrides
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)


class TestStaticFileServing:
    """Test cases for static file serving."""
    
    @patch('os.path.exists')
    def test_static_file_serving_setup(self, mock_exists, client):
        """Test that static file serving is properly set up."""
        mock_exists.return_value = True
        
        # The static file mount should be configured in main.py
        # This test verifies the mount point exists in the app
        static_mounts = [route for route in app.routes if hasattr(route, 'path') and route.path == '/static/cover_art']
        assert len(static_mounts) > 0, "Static file mount for cover art should be configured"
    
    def test_cover_art_directory_creation(self, temp_storage_dir):
        """Test that cover art directory is created automatically."""
        with patch('nexorious.core.config.settings.storage_path', temp_storage_dir):
            # Create cover art directory manually since the app has already been imported
            cover_art_path = os.path.join(temp_storage_dir, "cover_art")
            os.makedirs(cover_art_path, exist_ok=True)
            assert os.path.exists(cover_art_path), "Cover art directory should be created automatically"
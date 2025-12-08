"""
Tests for cover art download and storage functionality.

Tests are organized into:
1. StorageService unit tests - test actual logic directly
2. API endpoint integration tests - test with httpx mocking
"""

import os
import pytest
import tempfile
import shutil
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch
from io import BytesIO

import httpx
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool
from PIL import Image

from app.main import app
from app.core.database import get_session
from app.core.security import get_current_user
from app.api.dependencies import get_igdb_service_dependency
from app.services.igdb import GameMetadata
from app.services.storage import StorageService
from app.models.game import Game
from app.models.user import User


# ============================================================================
# Fixtures
# ============================================================================

@pytest.fixture
def client():
    """Create test client with in-memory database."""
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

    app.dependency_overrides[get_session] = get_test_session

    client = TestClient(app)
    yield client

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
def storage_service(temp_storage_dir):
    """Create a StorageService with a temporary directory."""
    with patch('app.services.storage.settings') as mock_settings:
        mock_settings.storage_path = temp_storage_dir
        service = StorageService()
        yield service


@pytest.fixture
def sample_game_data():
    """Sample game data for testing."""
    return {
        "id": 123,
        "title": "Test Game",
        "description": "A test game",
        "genre": "Action",
        "developer": "Test Studio",
        "publisher": "Test Publisher",
        "cover_art_url": "https://images.igdb.com/igdb/image/upload/t_cover_big/co1234.jpg"
    }


@pytest.fixture
def valid_png_bytes():
    """Create valid PNG image bytes for testing."""
    img = Image.new('RGB', (100, 100), color='red')
    buffer = BytesIO()
    img.save(buffer, format='PNG')
    buffer.seek(0)
    return buffer.read()


@pytest.fixture
def valid_jpeg_bytes():
    """Create valid JPEG image bytes for testing."""
    img = Image.new('RGB', (100, 100), color='blue')
    buffer = BytesIO()
    img.save(buffer, format='JPEG')
    buffer.seek(0)
    return buffer.read()


@pytest.fixture
def invalid_image_bytes():
    """Create invalid image bytes for testing."""
    return b"This is not a valid image file"


# ============================================================================
# StorageService Unit Tests - Test Real Logic
# ============================================================================

class TestStorageServiceURLValidation:
    """Test StorageService URL validation logic."""

    def test_is_valid_url_https(self, storage_service):
        """Test validation of HTTPS URL."""
        assert storage_service._is_valid_url("https://example.com/image.jpg") is True

    def test_is_valid_url_http(self, storage_service):
        """Test validation of HTTP URL."""
        assert storage_service._is_valid_url("http://example.com/image.jpg") is True

    def test_is_valid_url_too_short(self, storage_service):
        """Test rejection of too-short URL."""
        assert storage_service._is_valid_url("http://a") is False

    def test_is_valid_url_wrong_protocol(self, storage_service):
        """Test rejection of non-HTTP URL."""
        assert storage_service._is_valid_url("ftp://example.com/image.jpg") is False

    def test_is_valid_url_empty(self, storage_service):
        """Test rejection of empty URL."""
        assert storage_service._is_valid_url("") is False

    def test_is_valid_url_none_handling(self, storage_service):
        """Test handling of None URL."""
        # Should return False, not raise exception
        assert storage_service._is_valid_url(None) is False


class TestStorageServiceFilenameGeneration:
    """Test StorageService filename generation logic."""

    def test_get_cover_art_filename_jpg(self, storage_service):
        """Test filename generation for JPG URL."""
        filename = storage_service.get_cover_art_filename("123", "https://example.com/cover.jpg")
        assert filename == "123.jpg"

    def test_get_cover_art_filename_png(self, storage_service):
        """Test filename generation for PNG URL."""
        filename = storage_service.get_cover_art_filename("456", "https://example.com/cover.png")
        assert filename == "456.png"

    def test_get_cover_art_filename_no_extension(self, storage_service):
        """Test filename generation for URL without extension."""
        filename = storage_service.get_cover_art_filename("789", "https://example.com/cover")
        assert filename == "789.jpg"  # Defaults to jpg

    def test_get_cover_art_filename_complex_path(self, storage_service):
        """Test filename generation for URL with complex path."""
        filename = storage_service.get_cover_art_filename(
            "123",
            "https://images.igdb.com/igdb/image/upload/t_cover_big/co1234.webp"
        )
        assert filename == "123.webp"


class TestStorageServiceImageValidation:
    """Test StorageService image validation logic."""

    def test_validate_image_file_valid_png(self, storage_service, valid_png_bytes, temp_storage_dir):
        """Test validation of valid PNG file."""
        file_path = Path(temp_storage_dir) / "test.png"
        file_path.write_bytes(valid_png_bytes)

        assert storage_service._validate_image_file(file_path) is True

    def test_validate_image_file_valid_jpeg(self, storage_service, valid_jpeg_bytes, temp_storage_dir):
        """Test validation of valid JPEG file."""
        file_path = Path(temp_storage_dir) / "test.jpg"
        file_path.write_bytes(valid_jpeg_bytes)

        assert storage_service._validate_image_file(file_path) is True

    def test_validate_image_file_invalid(self, storage_service, invalid_image_bytes, temp_storage_dir):
        """Test validation rejects invalid image data."""
        file_path = Path(temp_storage_dir) / "test.jpg"
        file_path.write_bytes(invalid_image_bytes)

        assert storage_service._validate_image_file(file_path) is False

    def test_validate_image_file_empty(self, storage_service, temp_storage_dir):
        """Test validation rejects empty file."""
        file_path = Path(temp_storage_dir) / "empty.jpg"
        file_path.write_bytes(b"")

        assert storage_service._validate_image_file(file_path) is False

    def test_validate_image_file_nonexistent(self, storage_service, temp_storage_dir):
        """Test validation handles nonexistent file."""
        file_path = Path(temp_storage_dir) / "nonexistent.jpg"

        assert storage_service._validate_image_file(file_path) is False


class TestStorageServiceDirectories:
    """Test StorageService directory management."""

    def test_ensure_directories_creates_paths(self, temp_storage_dir):
        """Test that ensure_directories creates required paths."""
        with patch('app.services.storage.settings') as mock_settings:
            mock_settings.storage_path = temp_storage_dir
            service = StorageService()

            assert service.storage_path.exists()
            assert service.cover_art_path.exists()

    def test_cover_art_path_structure(self, storage_service, temp_storage_dir):
        """Test cover art path is correctly structured."""
        expected = Path(temp_storage_dir) / "cover_art"
        assert storage_service.cover_art_path == expected


class TestStorageServiceDownload:
    """Test StorageService download and store functionality."""

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_success(
        self, storage_service, valid_png_bytes, temp_storage_dir
    ):
        """Test successful cover art download and storage."""
        cover_url = "https://images.igdb.com/test/cover.png"

        # Mock HTTP response
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.content = valid_png_bytes
        mock_response.headers = {"content-type": "image/png"}
        mock_response.raise_for_status = MagicMock()

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.get.return_value = mock_response
            mock_client_class.return_value.__aenter__.return_value = mock_client

            result = await storage_service.download_and_store_cover_art("123", cover_url)

            assert result == "/static/cover_art/123.png"
            mock_client.get.assert_called_once_with(cover_url)

        # Verify file was created
        file_path = storage_service.cover_art_path / "123.png"
        assert file_path.exists()
        assert file_path.read_bytes() == valid_png_bytes

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_invalid_url(self, storage_service):
        """Test that invalid URL is rejected before download attempt."""
        result = await storage_service.download_and_store_cover_art("123", "ftp://invalid.url")
        assert result is None

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_empty_params(self, storage_service):
        """Test handling of empty parameters."""
        assert await storage_service.download_and_store_cover_art("", "https://example.com") is None
        assert await storage_service.download_and_store_cover_art("123", "") is None
        assert await storage_service.download_and_store_cover_art(None, None) is None

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_existing_valid(
        self, storage_service, valid_png_bytes, temp_storage_dir
    ):
        """Test that existing valid file is reused without re-download."""
        cover_url = "https://images.igdb.com/test/cover.png"

        # Create existing file
        file_path = storage_service.cover_art_path / "123.png"
        file_path.write_bytes(valid_png_bytes)

        # No HTTP mock needed - should return without download
        result = await storage_service.download_and_store_cover_art("123", cover_url)

        assert result == "/static/cover_art/123.png"

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_existing_invalid_redownloads(
        self, storage_service, valid_png_bytes, invalid_image_bytes, temp_storage_dir
    ):
        """Test that invalid existing file is re-downloaded."""
        cover_url = "https://images.igdb.com/test/cover.png"

        # Create invalid existing file
        file_path = storage_service.cover_art_path / "123.png"
        file_path.write_bytes(invalid_image_bytes)

        # Mock successful HTTP response
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.content = valid_png_bytes
        mock_response.headers = {"content-type": "image/png"}
        mock_response.raise_for_status = MagicMock()

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.get.return_value = mock_response
            mock_client_class.return_value.__aenter__.return_value = mock_client

            result = await storage_service.download_and_store_cover_art("123", cover_url)

            assert result == "/static/cover_art/123.png"

        # Verify file was replaced with valid content
        assert file_path.read_bytes() == valid_png_bytes

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_http_error_retries(
        self, storage_service, valid_png_bytes
    ):
        """Test that HTTP errors trigger retries."""
        cover_url = "https://images.igdb.com/test/cover.png"

        # Create mock responses: 2 failures then success
        mock_response_success = MagicMock()
        mock_response_success.status_code = 200
        mock_response_success.content = valid_png_bytes
        mock_response_success.headers = {"content-type": "image/png"}
        mock_response_success.raise_for_status = MagicMock()

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            # Fail twice, then succeed
            mock_client.get.side_effect = [
                httpx.HTTPError("Connection failed"),
                httpx.HTTPError("Timeout"),
                mock_response_success
            ]
            mock_client_class.return_value.__aenter__.return_value = mock_client

            result = await storage_service.download_and_store_cover_art("123", cover_url)

            assert result == "/static/cover_art/123.png"
            assert mock_client.get.call_count == 3

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_all_retries_fail(self, storage_service):
        """Test that exhausted retries return None."""
        cover_url = "https://images.igdb.com/test/cover.png"

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.get.side_effect = httpx.HTTPError("Connection failed")
            mock_client_class.return_value.__aenter__.return_value = mock_client

            result = await storage_service.download_and_store_cover_art("123", cover_url, max_retries=3)

            assert result is None
            assert mock_client.get.call_count == 3

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_invalid_content_type(self, storage_service):
        """Test rejection of non-image content type."""
        cover_url = "https://images.igdb.com/test/cover.png"

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.content = b"<html>Not an image</html>"
        mock_response.headers = {"content-type": "text/html"}
        mock_response.raise_for_status = MagicMock()

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.get.return_value = mock_response
            mock_client_class.return_value.__aenter__.return_value = mock_client

            result = await storage_service.download_and_store_cover_art("123", cover_url)

            assert result is None

    @pytest.mark.asyncio
    async def test_download_and_store_cover_art_file_too_large(self, storage_service):
        """Test rejection of oversized files."""
        cover_url = "https://images.igdb.com/test/cover.png"

        # Create content larger than 10MB limit
        large_content = b"x" * (11 * 1024 * 1024)

        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.content = large_content
        mock_response.headers = {"content-type": "image/png"}
        mock_response.raise_for_status = MagicMock()

        with patch('httpx.AsyncClient') as mock_client_class:
            mock_client = AsyncMock()
            mock_client.get.return_value = mock_response
            mock_client_class.return_value.__aenter__.return_value = mock_client

            result = await storage_service.download_and_store_cover_art("123", cover_url)

            assert result is None


class TestStorageServiceFileOperations:
    """Test StorageService file operation methods."""

    def test_delete_cover_art_success(self, storage_service, valid_png_bytes):
        """Test successful cover art deletion."""
        file_path = storage_service.cover_art_path / "123.jpg"
        file_path.write_bytes(valid_png_bytes)

        result = storage_service.delete_cover_art("123", "https://example.com/cover.jpg")

        assert result is True
        assert not file_path.exists()

    def test_delete_cover_art_nonexistent(self, storage_service):
        """Test deletion of nonexistent file returns False."""
        result = storage_service.delete_cover_art("999", "https://example.com/cover.jpg")

        assert result is False

    def test_cover_art_exists_true(self, storage_service, valid_png_bytes):
        """Test cover_art_exists returns True for existing file."""
        file_path = storage_service.cover_art_path / "123.jpg"
        file_path.write_bytes(valid_png_bytes)

        assert storage_service.cover_art_exists("123", "https://example.com/cover.jpg") is True

    def test_cover_art_exists_false(self, storage_service):
        """Test cover_art_exists returns False for missing file."""
        assert storage_service.cover_art_exists("999", "https://example.com/cover.jpg") is False

    def test_get_storage_stats(self, storage_service, valid_png_bytes):
        """Test storage statistics calculation."""
        # Create some test files
        for i in range(3):
            file_path = storage_service.cover_art_path / f"{i}.jpg"
            file_path.write_bytes(valid_png_bytes)

        stats = storage_service.get_storage_stats()

        assert stats["cover_art_files"] == 3
        assert stats["total_size_bytes"] == len(valid_png_bytes) * 3

    def test_cleanup_orphaned_files(self, storage_service, valid_png_bytes):
        """Test cleanup removes files not in valid ID list."""
        # Create files for IDs 1, 2, 3
        for i in range(1, 4):
            file_path = storage_service.cover_art_path / f"{i}.jpg"
            file_path.write_bytes(valid_png_bytes)

        # Only IDs 1 and 3 are valid
        result = storage_service.cleanup_orphaned_files(["1", "3"])

        assert result["removed_files"] == 1
        assert (storage_service.cover_art_path / "1.jpg").exists()
        assert not (storage_service.cover_art_path / "2.jpg").exists()
        assert (storage_service.cover_art_path / "3.jpg").exists()


# ============================================================================
# API Endpoint Tests - Test with IGDB Service
# ============================================================================

def create_mock_igdb_service(download_result=None, error=None):
    """Create a mock IGDB service with configurable responses."""
    mock_igdb = AsyncMock()

    if error:
        mock_igdb.download_and_store_cover_art.side_effect = error
    else:
        mock_igdb.download_and_store_cover_art.return_value = download_result

    return mock_igdb


class TestCoverArtDownloadEndpoint:
    """Test cases for individual cover art download endpoint."""

    def test_download_cover_art_success(self, client, mock_user, sample_game_data):
        """Test successful cover art download via endpoint."""
        try:
            # Create a game in the test database
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_game_data)
            session.add(game)
            session.commit()
            session.refresh(game)

            # Override dependencies
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(
                download_result="/static/cover_art/123.jpg"
            )

            response = client.post(f"/api/games/{sample_game_data['id']}/cover-art/download")

            assert response.status_code == 200
            data = response.json()
            assert "Cover art downloaded and stored successfully" in data["message"]
            assert "/static/cover_art/123.jpg" in data["message"]

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_download_cover_art_game_not_found(self, client, mock_user):
        """Test cover art download for non-existent game."""
        try:
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service()

            response = client.post("/api/games/99999/cover-art/download")

            assert response.status_code == 404
            data = response.json()
            error_message = data.get("error", data.get("detail", ""))
            assert "not found" in error_message.lower()

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_download_cover_art_no_cover_url(self, client, mock_user):
        """Test cover art download for game without cover art URL."""
        sample_data = {
            "id": 123,
            "title": "Test Game",
            "cover_art_url": None
        }

        try:
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_data)
            session.add(game)
            session.commit()
            session.refresh(game)

            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service()

            response = client.post(f"/api/games/{sample_data['id']}/cover-art/download")

            assert response.status_code == 400
            assert "Game does not have cover art URL" in response.json()["error"]

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_download_cover_art_admin_user(self, client, mock_admin_user, sample_game_data):
        """Test cover art download by admin user."""
        try:
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_game_data)
            session.add(game)
            session.commit()
            session.refresh(game)

            app.dependency_overrides[get_current_user] = lambda: mock_admin_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(
                download_result="/static/cover_art/123.jpg"
            )

            response = client.post(f"/api/games/{sample_game_data['id']}/cover-art/download")

            assert response.status_code == 200
            data = response.json()
            assert "Cover art downloaded and stored successfully" in data["message"]

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_download_cover_art_download_failure(self, client, mock_user, sample_game_data):
        """Test cover art download failure handling."""
        try:
            session = next(app.dependency_overrides[get_session]())
            game = Game(**sample_game_data)
            session.add(game)
            session.commit()
            session.refresh(game)

            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(
                download_result=None
            )

            response = client.post(f"/api/games/{sample_game_data['id']}/cover-art/download")

            assert response.status_code == 500
            response_data = response.json()
            assert "Failed to download and store cover art" in response_data["error"]

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)


class TestBulkCoverArtDownloadEndpoint:
    """Test cases for bulk cover art download endpoint."""

    def test_bulk_download_success(self, client, mock_admin_user):
        """Test successful bulk cover art download."""
        game_ids = [1001, 1002]

        try:
            session = next(app.dependency_overrides[get_session]())
            for i, game_id in enumerate(game_ids):
                game = Game(
                    id=game_id,
                    title=f"Test Game {i+1}",
                    cover_art_url=f"https://example.com/cover{i+1}.jpg",
                )
                session.add(game)
            session.commit()

            app.dependency_overrides[get_current_user] = lambda: mock_admin_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(
                download_result="/static/cover_art/123.jpg"
            )

            response = client.post(
                "/api/games/cover-art/bulk-download",
                json={"game_ids": game_ids, "skip_existing": False}
            )

            assert response.status_code == 200
            data = response.json()
            assert data["total_games"] == 2
            assert data["successful_operations"] == 2
            assert data["failed_operations"] == 0

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_bulk_download_non_admin(self, client, mock_user):
        """Test bulk download with non-admin user."""
        try:
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service()

            response = client.post(
                "/api/games/cover-art/bulk-download",
                json={"game_ids": [1001, 1002], "skip_existing": False}
            )

            assert response.status_code == 403
            response_data = response.json()
            assert "Only administrators can perform bulk cover art downloads" in response_data["error"]

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_bulk_download_skip_existing(self, client, mock_admin_user):
        """Test bulk download with skip existing option."""
        game_ids = [1001, 1002]

        try:
            session = next(app.dependency_overrides[get_session]())

            # Game 1 - already has local cover art
            game1 = Game(
                id=1001,
                title="Test Game 1",
                cover_art_url="/static/cover_art/1.jpg",  # Local URL
            )
            session.add(game1)

            # Game 2 - has remote cover art
            game2 = Game(
                id=1002,
                title="Test Game 2",
                cover_art_url="https://example.com/cover2.jpg",  # Remote URL
            )
            session.add(game2)

            session.commit()

            app.dependency_overrides[get_current_user] = lambda: mock_admin_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(
                download_result="/static/cover_art/2.jpg"
            )

            response = client.post(
                "/api/games/cover-art/bulk-download",
                json={"game_ids": game_ids, "skip_existing": True}
            )

            assert response.status_code == 200
            data = response.json()
            assert data["total_games"] == 2
            assert data["successful_operations"] == 2
            assert data["failed_operations"] == 0

            # Check that game1 was skipped and game2 was downloaded
            results = data["results"]
            game1_result = next(r for r in results if r["game_id"] == 1001)
            game2_result = next(r for r in results if r["game_id"] == 1002)

            assert game1_result["success"] is True
            assert "Already has local cover art" in game1_result["message"]
            assert game2_result["success"] is True
            assert "Cover art downloaded successfully" in game2_result["message"]

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_bulk_download_mixed_results(self, client, mock_admin_user):
        """Test bulk download with mixed success/failure results."""
        game_ids = [1001, 1002, 1003]

        try:
            session = next(app.dependency_overrides[get_session]())

            # Game 1 - valid game
            game1 = Game(
                id=1001,
                title="Test Game 1",
                cover_art_url="https://example.com/cover1.jpg",
            )
            session.add(game1)

            # Game 2 - has IGDB ID and cover URL
            game2 = Game(
                id=1002,
                title="Test Game 2",
                cover_art_url="https://example.com/cover2.jpg",
            )
            session.add(game2)

            # Game 3 - no cover art URL
            game3 = Game(
                id=1003,
                title="Test Game 3",
                cover_art_url=None,
            )
            session.add(game3)

            session.commit()

            app.dependency_overrides[get_current_user] = lambda: mock_admin_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: create_mock_igdb_service(
                download_result="/static/cover_art/1.jpg"
            )

            response = client.post(
                "/api/games/cover-art/bulk-download",
                json={"game_ids": game_ids, "skip_existing": False}
            )

            assert response.status_code == 200
            data = response.json()
            assert data["total_games"] == 3
            assert data["successful_operations"] == 2  # Games 1 and 2 succeed
            assert data["failed_operations"] == 1  # Game 3 fails (no cover URL)

            # Check error messages
            errors = data["errors"]
            assert any("does not have cover art URL" in error for error in errors)
            assert len(errors) == 1

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)


class TestAutomaticCoverArtDownload:
    """Test cases for automatic cover art download during import."""

    def test_igdb_import_default_behavior_downloads_cover_art(self, client, mock_user):
        """Test IGDB import with default behavior (should download cover art automatically)."""
        game_metadata = GameMetadata(
            igdb_id=123,
            title="Test Game",
            description="A test game",
            genre="Action",
            cover_art_url="https://example.com/cover.jpg"
        )

        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.return_value = "/static/cover_art/123.jpg"

        try:
            app.dependency_overrides[get_current_user] = lambda: mock_user
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
            assert data["title"] == "Test Game"
            assert data["cover_art_url"] == "/static/cover_art/123.jpg"

            # Verify download was called due to new default behavior
            mock_igdb.download_and_store_cover_art.assert_called_once_with(
                123, "https://example.com/cover.jpg"
            )

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_igdb_import_with_cover_art_download(self, client, mock_user):
        """Test IGDB import with automatic cover art download."""
        game_metadata = GameMetadata(
            igdb_id=123,
            title="Test Game",
            description="A test game",
            genre="Action",
            cover_art_url="https://example.com/cover.jpg"
        )

        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.return_value = "/static/cover_art/123.jpg"

        try:
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb

            response = client.post(
                "/api/games/igdb-import?download_cover_art=true",
                json={
                    "igdb_id": 123,
                    "accept_metadata": True,
                    "custom_overrides": {}
                }
            )

            assert response.status_code == 201
            data = response.json()
            assert data["title"] == "Test Game"
            assert data["cover_art_url"] == "/static/cover_art/123.jpg"

            mock_igdb.download_and_store_cover_art.assert_called_once_with(
                123, "https://example.com/cover.jpg"
            )

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_igdb_import_without_cover_art_download(self, client, mock_user):
        """Test IGDB import without automatic cover art download."""
        game_metadata = GameMetadata(
            igdb_id=123,
            title="Test Game",
            description="A test game",
            genre="Action",
            cover_art_url="https://example.com/cover.jpg"
        )

        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.return_value = "/static/cover_art/123.jpg"

        try:
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb

            response = client.post(
                "/api/games/igdb-import?download_cover_art=false",
                json={
                    "igdb_id": 123,
                    "accept_metadata": True,
                    "custom_overrides": {}
                }
            )

            assert response.status_code == 201
            data = response.json()
            assert data["title"] == "Test Game"
            assert data["cover_art_url"] == "https://example.com/cover.jpg"  # Remote URL preserved

            mock_igdb.download_and_store_cover_art.assert_not_called()

        finally:
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)

    def test_igdb_import_cover_art_download_failure(self, client, mock_user):
        """Test IGDB import when cover art download fails."""
        game_metadata = GameMetadata(
            igdb_id=123,
            title="Test Game",
            description="A test game",
            genre="Action",
            cover_art_url="https://example.com/cover.jpg"
        )

        mock_igdb = AsyncMock()
        mock_igdb.get_game_by_id.return_value = game_metadata
        mock_igdb.download_and_store_cover_art.side_effect = Exception("Download failed")

        try:
            app.dependency_overrides[get_current_user] = lambda: mock_user
            app.dependency_overrides[get_igdb_service_dependency] = lambda: mock_igdb

            response = client.post(
                "/api/games/igdb-import?download_cover_art=true",
                json={
                    "igdb_id": 123,
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
            app.dependency_overrides.pop(get_current_user, None)
            app.dependency_overrides.pop(get_igdb_service_dependency, None)


class TestStaticFileServing:
    """Test cases for static file serving."""

    @patch('os.path.exists')
    def test_static_file_serving_setup(self, mock_exists, client):
        """Test that static file serving is properly set up."""
        mock_exists.return_value = True

        # The static file mount should be configured in main.py
        static_mounts = [
            route for route in app.routes
            if hasattr(route, 'path') and route.path == '/static/cover_art'
        ]
        assert len(static_mounts) > 0, "Static file mount for cover art should be configured"

    def test_cover_art_directory_creation(self, temp_storage_dir):
        """Test that cover art directory is created automatically."""
        with patch('app.core.config.settings.storage_path', temp_storage_dir):
            cover_art_path = os.path.join(temp_storage_dir, "cover_art")
            os.makedirs(cover_art_path, exist_ok=True)
            assert os.path.exists(cover_art_path)

"""
Tests for metadata population and refresh functionality.
"""

import pytest
from unittest.mock import AsyncMock, patch, MagicMock

from app.services.igdb import IGDBService, GameMetadata
from app.services.storage import StorageService
from app.utils.rate_limiter import RateLimitConfig, create_igdb_rate_limiter


class TestIGDBMetadataService:
    """Test IGDB metadata service functionality."""

    @pytest.fixture
    def igdb_service(self):
        """Create IGDB service instance for testing."""
        with patch('app.services.igdb.service.settings') as mock_settings:
            mock_settings.igdb_client_id = "test_client_id"
            mock_settings.igdb_client_secret = "test_client_secret"
            mock_settings.igdb_access_token = "test_token"

            rate_config = RateLimitConfig(
                requests_per_second=4.0,
                burst_capacity=8,
                backoff_factor=1.0,
                max_retries=3
            )
            rate_limiter = create_igdb_rate_limiter(rate_config)
            service = IGDBService(rate_limiter=rate_limiter)
            service._http_client = AsyncMock()
            service._wrapper = AsyncMock()
            return service
    
    @pytest.fixture
    def sample_game_metadata(self):
        """Sample game metadata for testing."""
        return GameMetadata(
            igdb_id=12345,
            title="Test Game",
            description="A test game",
            genre="Action",
            developer="Test Studio",
            publisher="Test Publisher",
            release_date="2023-01-01",
            cover_art_url="https://example.com/cover.jpg",
            rating_average=8.5,
            rating_count=100,
            estimated_playtime_hours=20
        )
    
    @pytest.mark.asyncio
    async def test_refresh_game_metadata(self, igdb_service, sample_game_metadata):
        """Test refreshing game metadata."""
        # Mock the get_game_by_id method
        igdb_service.get_game_by_id = AsyncMock(return_value=sample_game_metadata)
        
        result = await igdb_service.refresh_game_metadata("12345")
        
        assert result is not None
        assert result.title == "Test Game"
        assert result.igdb_id == 12345
        igdb_service.get_game_by_id.assert_called_once_with("12345")
    
    @pytest.mark.asyncio
    async def test_populate_missing_metadata(self, igdb_service, sample_game_metadata):
        """Test populating missing metadata."""
        # Create current metadata with missing fields
        current_metadata = GameMetadata(
            igdb_id=12345,
            title="Test Game",
            description=None,  # Missing
            genre=None,  # Missing
            developer="Test Studio",
            publisher=None,  # Missing
            release_date=None,  # Missing
            cover_art_url=None,  # Missing
            rating_average=None,  # Missing
            rating_count=0,
            estimated_playtime_hours=None  # Missing
        )
        
        # Mock the get_game_by_id method to return full metadata
        igdb_service.get_game_by_id = AsyncMock(return_value=sample_game_metadata)
        
        result = await igdb_service.populate_missing_metadata(current_metadata, "12345")
        
        assert result is not None
        assert result.title == "Test Game"  # Kept existing
        assert result.developer == "Test Studio"  # Kept existing
        assert result.description == "A test game"  # Populated
        assert result.genre == "Action"  # Populated
        assert result.publisher == "Test Publisher"  # Populated
        assert result.release_date == "2023-01-01"  # Populated
        assert result.cover_art_url == "https://example.com/cover.jpg"  # Populated
        assert result.rating_average == 8.5  # Populated
        assert result.estimated_playtime_hours == 20  # Populated
    
    def test_compare_metadata(self, igdb_service):
        """Test metadata comparison functionality."""
        current = GameMetadata(
            igdb_id=12345,
            title="Test Game",
            description="Old description",
            genre="Action",
            developer="Test Studio",
            publisher="Test Publisher",
            release_date="2023-01-01",
            cover_art_url="https://example.com/old-cover.jpg",
            rating_average=8.0,
            rating_count=50,
            estimated_playtime_hours=15
        )
        
        fresh = GameMetadata(
            igdb_id=12345,
            title="Test Game",
            description="New description",
            genre="Action",
            developer="Test Studio",
            publisher="Test Publisher",
            release_date="2023-01-01",
            cover_art_url="https://example.com/new-cover.jpg",
            rating_average=8.5,
            rating_count=100,
            estimated_playtime_hours=20
        )
        
        differences = igdb_service.compare_metadata(current, fresh)
        
        assert "description" in differences
        assert differences["description"]["current"] == "Old description"
        assert differences["description"]["fresh"] == "New description"
        
        assert "cover_art_url" in differences
        assert "rating_average" in differences
        assert "rating_count" in differences
        assert "estimated_playtime_hours" in differences
        
        # Fields that are the same should not be in differences
        assert "title" not in differences
        assert "genre" not in differences
        assert "developer" not in differences
    
    @pytest.mark.asyncio
    async def test_get_metadata_completeness(self, igdb_service):
        """Test metadata completeness analysis."""
        # Create metadata with some missing fields
        metadata = GameMetadata(
            igdb_id=12345,
            title="Test Game",
            description=None,  # Missing essential
            genre="Action",
            developer=None,  # Missing essential
            publisher="Test Publisher",
            release_date="2023-01-01",
            cover_art_url=None,  # Missing optional
            rating_average=None,  # Missing optional
            rating_count=0,
            estimated_playtime_hours=None  # Missing optional
        )
        
        result = await igdb_service.get_metadata_completeness(metadata)
        
        assert "completeness_percentage" in result
        assert "missing_essential" in result
        assert "missing_optional" in result
        assert "total_fields" in result
        assert "filled_fields" in result
        
        # Should identify missing essential fields
        assert "description" in result["missing_essential"]
        assert "developer" in result["missing_essential"]
        
        # Should identify missing optional fields
        assert "cover_art_url" in result["missing_optional"]
        assert "rating_average" in result["missing_optional"]
        assert "estimated_playtime_hours" in result["missing_optional"]
        
        # Should not identify filled fields as missing
        assert "title" not in result["missing_essential"]
        assert "genre" not in result["missing_essential"]
        assert "publisher" not in result["missing_essential"]
        assert "release_date" not in result["missing_essential"]


class TestStorageService:
    """Test storage service functionality."""
    
    @pytest.fixture
    def storage_service(self):
        """Create storage service instance for testing."""
        with patch('app.services.storage.settings') as mock_settings:
            mock_settings.storage_path = "/tmp/test_storage"
            
            service = StorageService()
            return service
    
    def test_get_cover_art_filename(self, storage_service):
        """Test cover art filename generation."""
        filename = storage_service.get_cover_art_filename("12345", "https://example.com/cover.jpg")
        assert filename == "12345.jpg"
        
        filename = storage_service.get_cover_art_filename("67890", "https://example.com/cover.png")
        assert filename == "67890.png"
        
        # Test URL without extension
        filename = storage_service.get_cover_art_filename("11111", "https://example.com/cover")
        assert filename == "11111.jpg"  # Default extension
    
    def test_get_cover_art_url(self, storage_service):
        """Test cover art URL generation."""
        url = storage_service.get_cover_art_url("12345", "https://example.com/cover.jpg")
        assert url == "/static/cover_art/12345.jpg"
    
    @pytest.mark.asyncio
    async def test_download_and_store_cover_art(self, storage_service):
        """Test cover art download and storage."""
        with patch('httpx.AsyncClient') as mock_client:
            mock_response = MagicMock()
            mock_response.content = b"fake_image_data"
            mock_response.headers = {'content-type': 'image/jpeg'}
            mock_response.raise_for_status = MagicMock()
            
            mock_client.return_value.__aenter__.return_value.get.return_value = mock_response
            
            with patch('pathlib.Path.exists', return_value=False):
                with patch.object(storage_service, '_validate_image_file', return_value=True):
                    with patch('builtins.open', mock_open()):
                        with patch('os.rename'):
                            result = await storage_service.download_and_store_cover_art("12345", "https://example.com/cover.jpg")
                            
                            assert result == "/static/cover_art/12345.jpg"
    
    def test_cover_art_exists(self, storage_service):
        """Test cover art existence check."""
        with patch('pathlib.Path.exists', return_value=True):
            assert storage_service.cover_art_exists("12345", "https://example.com/cover.jpg") is True
        
        with patch('pathlib.Path.exists', return_value=False):
            assert storage_service.cover_art_exists("12345", "https://example.com/cover.jpg") is False


# Mock open function for file operations
def mock_open():
    """Mock open function for testing file operations."""
    return MagicMock()
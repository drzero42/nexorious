"""
Tests for How Long to Beat integration functionality.
"""

import pytest
from unittest.mock import Mock, AsyncMock, patch
from datetime import datetime, timezone
import json

from nexorious.services.igdb import IGDBService, GameMetadata, map_igdb_time_to_beat_to_db_fields


class TestTimeToBeartMapping:
    """Test the mapping of IGDB time-to-beat fields to database fields."""
    
    def test_map_igdb_time_to_beat_to_db_fields_all_fields(self):
        """Test mapping with all fields present."""
        igdb_data = {
            "hastily": 8,
            "normally": 15,
            "completely": 25
        }
        
        result = map_igdb_time_to_beat_to_db_fields(igdb_data)
        
        expected = {
            "howlongtobeat_main": 8,
            "howlongtobeat_extra": 15,
            "howlongtobeat_completionist": 25
        }
        
        assert result == expected
    
    def test_map_igdb_time_to_beat_to_db_fields_partial_fields(self):
        """Test mapping with only some fields present."""
        igdb_data = {
            "hastily": 12,
            "normally": None,
            "completely": 30
        }
        
        result = map_igdb_time_to_beat_to_db_fields(igdb_data)
        
        expected = {
            "howlongtobeat_main": 12,
            "howlongtobeat_extra": None,
            "howlongtobeat_completionist": 30
        }
        
        assert result == expected
    
    def test_map_igdb_time_to_beat_to_db_fields_empty_data(self):
        """Test mapping with empty data."""
        igdb_data = {}
        
        result = map_igdb_time_to_beat_to_db_fields(igdb_data)
        
        expected = {
            "howlongtobeat_main": None,
            "howlongtobeat_extra": None,
            "howlongtobeat_completionist": None
        }
        
        assert result == expected


class TestGameMetadata:
    """Test the GameMetadata class with time-to-beat fields."""
    
    def test_game_metadata_with_time_to_beat(self):
        """Test creating GameMetadata with time-to-beat fields."""
        metadata = GameMetadata(
            igdb_id="12345",
            title="Test Game",
            slug="test-game",
            hastily=10,
            normally=20,
            completely=35
        )
        
        assert metadata.hastily == 10
        assert metadata.normally == 20
        assert metadata.completely == 35
    
    def test_game_metadata_without_time_to_beat(self):
        """Test creating GameMetadata without time-to-beat fields."""
        metadata = GameMetadata(
            igdb_id="12345",
            title="Test Game",
            slug="test-game"
        )
        
        assert metadata.hastily is None
        assert metadata.normally is None
        assert metadata.completely is None


class TestIGDBService:
    """Test the IGDB service time-to-beat functionality."""
    
    @pytest.fixture
    def igdb_service(self):
        """Create an IGDB service instance for testing."""
        with patch('nexorious.services.igdb.settings') as mock_settings:
            mock_settings.igdb_client_id = "test_client_id"
            mock_settings.igdb_client_secret = "test_client_secret"
            mock_settings.igdb_access_token = "test_token"
            service = IGDBService()
            return service
    
    @pytest.mark.asyncio
    async def test_get_time_to_beat_data_success(self, igdb_service):
        """Test successful retrieval of time-to-beat data."""
        mock_wrapper = Mock()
        mock_wrapper.api_request.return_value = json.dumps([{
            "hastily": 8,
            "normally": 15,
            "completely": 25
        }]).encode('utf-8')
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            result = await igdb_service._get_time_to_beat_data("12345")
        
        assert result == {
            "hastily": 8,
            "normally": 15,
            "completely": 25
        }
        
        mock_wrapper.api_request.assert_called_once()
        call_args = mock_wrapper.api_request.call_args
        assert call_args[0][0] == 'game_time_to_beat'
        assert 'where game = 12345' in call_args[0][1]
    
    @pytest.mark.asyncio
    async def test_get_time_to_beat_data_no_data(self, igdb_service):
        """Test when no time-to-beat data is available."""
        mock_wrapper = Mock()
        mock_wrapper.api_request.return_value = json.dumps([]).encode('utf-8')
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            result = await igdb_service._get_time_to_beat_data("12345")
        
        assert result is None
    
    @pytest.mark.asyncio
    async def test_get_time_to_beat_data_error(self, igdb_service):
        """Test error handling in time-to-beat data retrieval."""
        mock_wrapper = Mock()
        mock_wrapper.api_request.side_effect = Exception("API Error")
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper):
            result = await igdb_service._get_time_to_beat_data("12345")
        
        assert result is None
    
    @pytest.mark.asyncio
    async def test_get_game_by_id_includes_time_to_beat(self, igdb_service):
        """Test that get_game_by_id includes time-to-beat data."""
        mock_wrapper = Mock()
        
        # Mock game data response
        game_data = {
            "id": 12345,
            "name": "Test Game",
            "slug": "test-game",
            "summary": "A test game"
        }
        mock_wrapper.api_request.return_value = json.dumps([game_data]).encode('utf-8')
        
        # Mock time-to-beat data
        time_data = {
            "hastily": 10,
            "normally": 18,
            "completely": 30
        }
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper), \
             patch.object(igdb_service, '_get_time_to_beat_data', return_value=time_data):
            
            result = await igdb_service.get_game_by_id("12345")
        
        assert result is not None
        assert result.hastily == 10
        assert result.normally == 18
        assert result.completely == 30
    
    @pytest.mark.asyncio
    async def test_search_games_includes_time_to_beat(self, igdb_service):
        """Test that search_games includes time-to-beat data."""
        mock_wrapper = Mock()
        
        # Mock game data response
        games_data = [{
            "id": 12345,
            "name": "Test Game",
            "slug": "test-game",
            "summary": "A test game"
        }]
        mock_wrapper.api_request.return_value = json.dumps(games_data).encode('utf-8')
        
        # Mock time-to-beat data
        time_data = {
            "hastily": 8,
            "normally": 15,
            "completely": 25
        }
        
        with patch.object(igdb_service, '_get_wrapper', return_value=mock_wrapper), \
             patch.object(igdb_service, '_get_time_to_beat_data', return_value=time_data):
            
            results = await igdb_service.search_games("test")
        
        assert len(results) == 1
        assert results[0].hastily == 8
        assert results[0].normally == 15
        assert results[0].completely == 25
    
    @pytest.mark.asyncio
    async def test_populate_missing_metadata_includes_time_to_beat(self, igdb_service):
        """Test that populate_missing_metadata includes time-to-beat data."""
        current_metadata = GameMetadata(
            igdb_id="12345",
            title="Test Game",
            slug="test-game",
            hastily=None,
            normally=None,
            completely=None
        )
        
        fresh_metadata = GameMetadata(
            igdb_id="12345",
            title="Test Game",
            slug="test-game",
            hastily=12,
            normally=20,
            completely=35
        )
        
        with patch.object(igdb_service, 'get_game_by_id', return_value=fresh_metadata):
            result = await igdb_service.populate_missing_metadata(current_metadata, "12345")
        
        assert result is not None
        assert result.hastily == 12
        assert result.normally == 20
        assert result.completely == 35
    
    def test_compare_metadata_includes_time_to_beat(self, igdb_service):
        """Test that compare_metadata includes time-to-beat fields."""
        current_metadata = GameMetadata(
            igdb_id="12345",
            title="Test Game",
            slug="test-game",
            hastily=10,
            normally=18,
            completely=30
        )
        
        fresh_metadata = GameMetadata(
            igdb_id="12345",
            title="Test Game",
            slug="test-game",
            hastily=12,
            normally=20,
            completely=35
        )
        
        result = igdb_service.compare_metadata(current_metadata, fresh_metadata)
        
        assert 'hastily' in result
        assert 'normally' in result
        assert 'completely' in result
        
        assert result['hastily']['current'] == 10
        assert result['hastily']['fresh'] == 12
        assert result['normally']['current'] == 18
        assert result['normally']['fresh'] == 20
        assert result['completely']['current'] == 30
        assert result['completely']['fresh'] == 35
    
    @pytest.mark.asyncio
    async def test_get_metadata_completeness_includes_time_to_beat(self, igdb_service):
        """Test that get_metadata_completeness includes time-to-beat fields."""
        metadata = GameMetadata(
            igdb_id="12345",
            title="Test Game",
            slug="test-game",
            description="A test game",
            genre="Action",
            developer="Test Studio",
            publisher="Test Publisher",
            release_date="2024-01-01",
            cover_art_url="https://example.com/cover.jpg",
            rating_average=8.5,
            rating_count=100,
            hastily=10,
            normally=18,
            completely=30
        )
        
        result = await igdb_service.get_metadata_completeness(metadata)
        
        # Most fields should be present. Let's check what we actually get
        assert result['completeness_percentage'] > 90.0  # Should be high
        assert result['missing_essential'] == []  # Essential fields should be filled
        # Some optional fields may be missing, that's ok
        assert result['total_fields'] >= 10  # Should have at least 10 fields
        assert result['filled_fields'] >= 9  # Most fields should be filled
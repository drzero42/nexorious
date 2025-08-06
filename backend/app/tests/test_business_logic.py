"""
Unit tests for business logic functions across the application.
Tests core utility functions, data processing, and business rules.
"""

import pytest
from unittest.mock import patch

from app.services.igdb import IGDBService, GameMetadata
from app.services.storage import StorageService


class TestIGDBServiceBusinessLogic:
    """Test business logic in IGDB service."""
    
    def test_rank_games_by_fuzzy_match_scoring(self):
        """Test the scoring logic of fuzzy matching."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id="1", title="The Witcher 3: Wild Hunt"),
            GameMetadata(igdb_id="2", title="The Witcher"),
            GameMetadata(igdb_id="3", title="Witcher 2"),
            GameMetadata(igdb_id="4", title="Some Other Game")
        ]
        
        # Test exact match gets highest score
        result = service._rank_games_by_fuzzy_match(games, "The Witcher 3: Wild Hunt", threshold=0.5)
        assert result[0].title == "The Witcher 3: Wild Hunt"
        
        # Test partial match ordering
        result = service._rank_games_by_fuzzy_match(games, "Witcher", threshold=0.5)
        # Should rank by relevance, check that we get at least one result
        assert len(result) >= 1
        # The exact match "The Witcher" should be in results
        titles = [game.title for game in result]
        assert "The Witcher" in titles
    
    def test_rank_games_empty_list(self):
        """Test ranking with empty game list."""
        service = IGDBService()
        result = service._rank_games_by_fuzzy_match([], "Any Query", threshold=0.5)
        assert result == []
    
    def test_rank_games_threshold_filtering(self):
        """Test that threshold properly filters results."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id="1", title="The Witcher 3"),
            GameMetadata(igdb_id="2", title="Completely Different Game")
        ]
        
        # High threshold should filter out non-matches
        result = service._rank_games_by_fuzzy_match(games, "Witcher", threshold=0.8)
        assert len(result) == 1
        assert result[0].title == "The Witcher 3"
        
        # Low threshold should include more matches
        result = service._rank_games_by_fuzzy_match(games, "Witcher", threshold=0.1)
        assert len(result) >= 1


class TestStorageServiceBusinessLogic:
    """Test business logic in storage service."""
    
    @patch('app.services.storage.Path.mkdir')
    def test_ensure_directories_creation(self, mock_mkdir):
        """Test that directories are created during initialization."""
        StorageService()
        
        # mkdir should be called for both storage_path and cover_art_path
        assert mock_mkdir.call_count >= 2
    
    def test_get_cover_art_filename_generation(self):
        """Test cover art filename generation logic."""
        service = StorageService()
        
        # Test with URL that has extension
        filename = service.get_cover_art_filename("game-123", "https://example.com/image.jpg")
        assert filename == "game-123.jpg"
        
        # Test with URL without extension
        filename2 = service.get_cover_art_filename("game-456", "https://example.com/image")
        assert filename2 == "game-456.jpg"  # Default extension
        
        # Test with different extension
        filename3 = service.get_cover_art_filename("game-789", "https://example.com/image.png")
        assert filename3 == "game-789.png"
    
    def test_get_cover_art_path_generation(self):
        """Test cover art path generation logic."""
        service = StorageService()
        
        # Test with game ID and URL
        path = service.get_cover_art_path("game-123", "https://example.com/image.jpg")
        assert "game-123.jpg" in str(path)
        
        # Test with different game ID and URL
        path2 = service.get_cover_art_path("different-game", "https://example.com/image.png")
        assert "different-game.png" in str(path2)
        
        # Paths should be different for different games
        assert path != path2
    
    def test_get_cover_art_path_consistency(self):
        """Test that cover art path generation is consistent."""
        service = StorageService()
        
        igdb_id = "test-game-123"
        url = "https://example.com/image.jpg"
        path1 = service.get_cover_art_path(igdb_id, url)
        path2 = service.get_cover_art_path(igdb_id, url)
        
        # Same parameters should generate same path
        assert path1 == path2


class TestGameMetadataBusinessLogic:
    """Test business logic related to game metadata."""
    
    def test_game_metadata_creation(self):
        """Test GameMetadata object creation and validation."""
        metadata = GameMetadata(
            igdb_id="12345",
            title="Test Game",
            description="A test game",
            release_date="2023-01-01",
            genre="Action",
            developer="Test Studio",
            publisher="Test Publisher",
            rating_average=85.5,
            cover_art_url="https://example.com/cover.jpg",
            hastily=15,
            normally=25,
            completely=40
        )
        
        assert metadata.igdb_id == "12345"
        assert metadata.title == "Test Game"
        assert metadata.rating_average == 85.5
        assert metadata.genre == "Action"
        assert metadata.hastily == 15
        assert metadata.normally == 25
        assert metadata.completely == 40
    
    def test_game_metadata_minimal_creation(self):
        """Test GameMetadata with minimal required fields."""
        metadata = GameMetadata(
            igdb_id="123",
            title="Minimal Game"
        )
        
        assert metadata.igdb_id == "123"
        assert metadata.title == "Minimal Game"
        assert metadata.description is None
        assert metadata.genre is None
        assert metadata.developer is None
    
    def test_game_metadata_string_representation(self):
        """Test string representation of GameMetadata."""
        metadata = GameMetadata(
            igdb_id="456",
            title="String Test Game"
        )
        
        str_repr = str(metadata)
        assert "String Test Game" in str_repr
        assert "456" in str_repr


class TestBusinessRulesAndValidation:
    """Test business rules and validation logic."""
    
    def test_play_status_validation(self):
        """Test play status validation logic."""
        valid_statuses = [
            'not_started', 'in_progress', 'completed', 'mastered', 
            'dominated', 'shelved', 'dropped', 'replay'
        ]
        
        # This would normally be in a model or service
        def validate_play_status(status: str) -> bool:
            return status in valid_statuses
        
        # Test valid statuses
        for status in valid_statuses:
            assert validate_play_status(status) is True
        
        # Test invalid statuses
        invalid_statuses = ['invalid', 'unknown', 'finished', 'playing']
        for status in invalid_statuses:
            assert validate_play_status(status) is False
    
    def test_rating_validation(self):
        """Test rating validation logic."""
        def validate_rating(rating: float) -> bool:
            return 1.0 <= rating <= 5.0
        
        # Test valid ratings
        assert validate_rating(1.0) is True
        assert validate_rating(3.5) is True
        assert validate_rating(5.0) is True
        
        # Test invalid ratings
        assert validate_rating(0.9) is False
        assert validate_rating(5.1) is False
        assert validate_rating(-1.0) is False
    
    def test_completion_percentage_calculation(self):
        """Test completion percentage calculation logic."""
        def calculate_completion_percentage(play_status: str, hours_played: int, estimated_hours: int) -> float:
            if play_status in ['completed', 'mastered', 'dominated']:
                return 100.0
            elif play_status == 'not_started':
                return 0.0
            elif play_status in ['in_progress', 'replay'] and estimated_hours > 0:
                return min(100.0, (hours_played / estimated_hours) * 100.0)
            else:
                return 0.0
        
        # Test completed games
        assert calculate_completion_percentage('completed', 20, 30) == 100.0
        assert calculate_completion_percentage('mastered', 50, 30) == 100.0
        
        # Test not started
        assert calculate_completion_percentage('not_started', 0, 30) == 0.0
        
        # Test in progress
        assert calculate_completion_percentage('in_progress', 15, 30) == 50.0
        assert calculate_completion_percentage('in_progress', 30, 30) == 100.0
        assert calculate_completion_percentage('in_progress', 45, 30) == 100.0
        
        # Test with zero estimated hours
        assert calculate_completion_percentage('in_progress', 10, 0) == 0.0
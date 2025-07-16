"""
Unit tests for business logic functions across the application.
Tests core utility functions, data processing, and business rules.
"""

import pytest
from unittest.mock import patch

from nexorious.api.games import create_slug
from nexorious.services.igdb import IGDBService, GameMetadata
from nexorious.services.storage import StorageService


class TestCreateSlug:
    """Test the create_slug utility function."""
    
    def test_create_slug_basic(self):
        """Test basic slug creation."""
        assert create_slug("The Witcher 3") == "the-witcher-3"
        assert create_slug("Grand Theft Auto V") == "grand-theft-auto-v"
        assert create_slug("Super Mario Bros") == "super-mario-bros"
    
    def test_create_slug_special_characters(self):
        """Test slug creation with special characters."""
        assert create_slug("Mass Effect: Legendary Edition") == "mass-effect-legendary-edition"
        assert create_slug("Assassin's Creed: Odyssey") == "assassins-creed-odyssey"
        assert create_slug("Call of Duty: Modern Warfare (2019)") == "call-of-duty-modern-warfare-2019"
        assert create_slug("Divinity: Original Sin 2") == "divinity-original-sin-2"
    
    def test_create_slug_punctuation(self):
        """Test slug creation with various punctuation."""
        assert create_slug("It's a Wonderful Life") == "its-a-wonderful-life"
        assert create_slug("Don't Starve Together") == "dont-starve-together"
        assert create_slug("Rock & Roll Racing") == "rock-roll-racing"
        assert create_slug("Sid Meier's Civilization VI") == "sid-meiers-civilization-vi"
    
    def test_create_slug_numbers_and_roman_numerals(self):
        """Test slug creation with numbers and Roman numerals."""
        assert create_slug("Final Fantasy VII") == "final-fantasy-vii"
        assert create_slug("World War Z") == "world-war-z"
        assert create_slug("Battlefield 2042") == "battlefield-2042"
        assert create_slug("Counter-Strike 2") == "counter-strike-2"
    
    def test_create_slug_multiple_spaces(self):
        """Test slug creation with multiple spaces."""
        assert create_slug("Game    with    spaces") == "game-with-spaces"
        assert create_slug("Too   many  spaces") == "too-many-spaces"
        assert create_slug("  Leading and trailing  ") == "leading-and-trailing"
    
    def test_create_slug_hyphens_and_underscores(self):
        """Test slug creation with existing hyphens and underscores."""
        assert create_slug("Half-Life") == "half-life"
        assert create_slug("Left 4 Dead") == "left-4-dead"
        assert create_slug("Team_Fortress_2") == "team_fortress_2"  # Underscores are preserved
        assert create_slug("Portal-2") == "portal-2"
    
    def test_create_slug_edge_cases(self):
        """Test slug creation with edge cases."""
        assert create_slug("") == ""
        assert create_slug("   ") == ""
        assert create_slug("!!!") == ""
        assert create_slug("@#$%^&*()") == ""
        assert create_slug("123") == "123"
        assert create_slug("a") == "a"
    
    def test_create_slug_unicode_characters(self):
        """Test slug creation with unicode characters."""
        assert create_slug("Pokémon Red") == "pokémon-red"  # Unicode is preserved
        assert create_slug("Café") == "café"
        assert create_slug("Señor") == "señor"
        assert create_slug("Naïve") == "naïve"
    
    def test_create_slug_very_long_titles(self):
        """Test slug creation with very long titles."""
        long_title = "This is a very long game title that should be handled properly by the slug function"
        expected = "this-is-a-very-long-game-title-that-should-be-handled-properly-by-the-slug-function"
        assert create_slug(long_title) == expected
    
    def test_create_slug_mixed_case(self):
        """Test slug creation with mixed case."""
        assert create_slug("ThE WiTcHeR 3") == "the-witcher-3"
        assert create_slug("SHOUTING GAME TITLE") == "shouting-game-title"
        assert create_slug("mIxEd CaSe") == "mixed-case"
    
    def test_create_slug_consecutive_special_chars(self):
        """Test slug creation with consecutive special characters."""
        assert create_slug("Game!!!Title") == "gametitle"  # Punctuation removed
        assert create_slug("Multiple---Hyphens") == "multiple-hyphens"
        assert create_slug("Dots...and...More") == "dotsandmore"
        assert create_slug("Mixed!!--__Title") == "mixed-__title"


class TestIGDBServiceBusinessLogic:
    """Test business logic in IGDB service."""
    
    def test_rank_games_by_fuzzy_match_scoring(self):
        """Test the scoring logic of fuzzy matching."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id="1", title="The Witcher 3: Wild Hunt", slug="witcher-3"),
            GameMetadata(igdb_id="2", title="The Witcher", slug="witcher"),
            GameMetadata(igdb_id="3", title="Witcher 2", slug="witcher-2"),
            GameMetadata(igdb_id="4", title="Some Other Game", slug="other-game")
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
            GameMetadata(igdb_id="1", title="The Witcher 3", slug="witcher-3"),
            GameMetadata(igdb_id="2", title="Completely Different Game", slug="different")
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
    
    @patch('nexorious.services.storage.Path.mkdir')
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
            slug="test-game",
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
        assert metadata.slug == "test-game"
        assert metadata.rating_average == 85.5
        assert metadata.genre == "Action"
        assert metadata.hastily == 15
        assert metadata.normally == 25
        assert metadata.completely == 40
    
    def test_game_metadata_minimal_creation(self):
        """Test GameMetadata with minimal required fields."""
        metadata = GameMetadata(
            igdb_id="123",
            title="Minimal Game",
            slug="minimal-game"
        )
        
        assert metadata.igdb_id == "123"
        assert metadata.title == "Minimal Game"
        assert metadata.slug == "minimal-game"
        assert metadata.description is None
        assert metadata.genre is None
        assert metadata.developer is None
    
    def test_game_metadata_string_representation(self):
        """Test string representation of GameMetadata."""
        metadata = GameMetadata(
            igdb_id="456",
            title="String Test Game",
            slug="string-test-game"
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
    
    def test_slug_uniqueness_logic(self):
        """Test slug uniqueness validation logic."""
        def generate_unique_slug(base_slug: str, existing_slugs: list) -> str:
            if base_slug not in existing_slugs:
                return base_slug
            
            counter = 1
            while f"{base_slug}-{counter}" in existing_slugs:
                counter += 1
            return f"{base_slug}-{counter}"
        
        existing = ["game-1", "game-2", "test-game", "test-game-1"]
        
        # Test new slug
        assert generate_unique_slug("new-game", existing) == "new-game"
        
        # Test existing slug
        assert generate_unique_slug("game-1", existing) == "game-1-1"
        
        # Test existing slug with counter
        assert generate_unique_slug("test-game", existing) == "test-game-2"
    
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
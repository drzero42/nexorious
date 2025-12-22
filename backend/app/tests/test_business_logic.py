"""
Unit tests for business logic functions across the application.
Tests core utility functions, data processing, and business rules.
"""

from app.services.igdb import IGDBService, GameMetadata
from app.services.storage import StorageService


class TestIGDBServiceBusinessLogic:
    """Test business logic in IGDB service."""
    
    def test_rank_games_by_fuzzy_match_scoring(self):
        """Test the scoring logic of fuzzy matching."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id=1, title="The Witcher 3: Wild Hunt"),
            GameMetadata(igdb_id=2, title="The Witcher"),
            GameMetadata(igdb_id=3, title="Witcher 2"),
            GameMetadata(igdb_id=4, title="Some Other Game")
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
            GameMetadata(igdb_id=1, title="The Witcher 3"),
            GameMetadata(igdb_id=2, title="Completely Different Game")
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

    def test_ensure_directories_creation(self, tmp_path, monkeypatch):
        """Test that directories are actually created during initialization."""
        # Use a temporary directory for the storage path
        test_storage_path = tmp_path / "test_storage"

        # Patch the settings to use our temporary path
        monkeypatch.setattr('app.services.storage.settings.storage_path', str(test_storage_path))

        # Directories should not exist before initialization
        assert not test_storage_path.exists()
        assert not (test_storage_path / "cover_art").exists()

        # Create the service - this should create the directories
        service = StorageService()

        # Verify directories were actually created
        assert test_storage_path.exists()
        assert test_storage_path.is_dir()
        assert (test_storage_path / "cover_art").exists()
        assert (test_storage_path / "cover_art").is_dir()

        # Verify the service paths are set correctly
        assert service.storage_path == test_storage_path
        assert service.cover_art_path == test_storage_path / "cover_art"
    
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
        
        igdb_id = "12300"
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
            igdb_id=12345,
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
        
        assert metadata.igdb_id == 12345
        assert metadata.title == "Test Game"
        assert metadata.rating_average == 85.5
        assert metadata.genre == "Action"
        assert metadata.hastily == 15
        assert metadata.normally == 25
        assert metadata.completely == 40
    
    def test_game_metadata_minimal_creation(self):
        """Test GameMetadata with minimal required fields."""
        metadata = GameMetadata(
            igdb_id=123,
            title="Minimal Game"
        )
        
        assert metadata.igdb_id == 123
        assert metadata.title == "Minimal Game"
        assert metadata.description is None
        assert metadata.genre is None
        assert metadata.developer is None
    
    def test_game_metadata_string_representation(self):
        """Test string representation of GameMetadata."""
        metadata = GameMetadata(
            igdb_id=456,
            title="String Test Game"
        )
        
        str_repr = str(metadata)
        assert "String Test Game" in str_repr
        assert "456" in str_repr


class TestPlayStatusEnum:
    """Test PlayStatus enum used for game play status tracking.

    Tests the actual PlayStatus enum from the models to verify
    all expected statuses exist and can be created properly.
    """

    def test_play_status_enum_values(self):
        """Test that PlayStatus enum has all expected values."""
        from app.models.user_game import PlayStatus

        # Verify all expected status values exist
        expected_statuses = [
            'not_started', 'in_progress', 'completed', 'mastered',
            'dominated', 'shelved', 'dropped', 'replay'
        ]

        enum_values = [status.value for status in PlayStatus]

        for expected in expected_statuses:
            assert expected in enum_values, f"Expected status '{expected}' not found in PlayStatus enum"

    def test_play_status_enum_creation(self):
        """Test PlayStatus enum can be created from string values."""
        from app.models.user_game import PlayStatus

        # Test creating enum from valid values
        assert PlayStatus('not_started') == PlayStatus.NOT_STARTED
        assert PlayStatus('in_progress') == PlayStatus.IN_PROGRESS
        assert PlayStatus('completed') == PlayStatus.COMPLETED
        assert PlayStatus('mastered') == PlayStatus.MASTERED
        assert PlayStatus('dominated') == PlayStatus.DOMINATED
        assert PlayStatus('shelved') == PlayStatus.SHELVED
        assert PlayStatus('dropped') == PlayStatus.DROPPED
        assert PlayStatus('replay') == PlayStatus.REPLAY

    def test_play_status_invalid_value_raises(self):
        """Test that invalid play status raises ValueError."""
        from app.models.user_game import PlayStatus
        import pytest

        invalid_statuses = ['invalid', 'unknown', 'finished', 'playing']

        for invalid in invalid_statuses:
            with pytest.raises(ValueError):
                PlayStatus(invalid)


class TestPydanticRatingValidation:
    """Test rating validation through Pydantic schemas.

    Tests the actual Pydantic Field validation for personal_rating
    to ensure the ge=1, le=5 constraints work correctly.
    """

    def test_valid_rating_in_schema(self):
        """Test that valid ratings are accepted by the schema."""
        from app.schemas.user_game import UserGameCreateRequest

        # Valid ratings should not raise
        for rating in [1.0, 2.5, 3.0, 4.5, 5.0]:
            # Create with minimal required fields plus rating
            schema = UserGameCreateRequest(
                game_id=1,
                personal_rating=rating
            )
            assert schema.personal_rating == rating

    def test_invalid_rating_too_low(self):
        """Test that rating below 1 is rejected."""
        from app.schemas.user_game import UserGameCreateRequest
        from pydantic import ValidationError
        import pytest

        with pytest.raises(ValidationError) as exc_info:
            UserGameCreateRequest(
                game_id=1,
                personal_rating=0.5
            )

        # Check that the error is about the rating being too low
        errors = exc_info.value.errors()
        assert any('personal_rating' in str(e) for e in errors)

    def test_invalid_rating_too_high(self):
        """Test that rating above 5 is rejected."""
        from app.schemas.user_game import UserGameCreateRequest
        from pydantic import ValidationError
        import pytest

        with pytest.raises(ValidationError) as exc_info:
            UserGameCreateRequest(
                game_id=1,
                personal_rating=5.5
            )

        # Check that the error is about the rating being too high
        errors = exc_info.value.errors()
        assert any('personal_rating' in str(e) for e in errors)

    def test_null_rating_is_valid(self):
        """Test that None/null rating is accepted (rating is optional)."""
        from app.schemas.user_game import UserGameCreateRequest

        schema = UserGameCreateRequest(
            game_id=1,
            personal_rating=None
        )
        assert schema.personal_rating is None
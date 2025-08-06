"""
Tests for IGDB service functionality including fuzzy matching.
"""

import pytest
from unittest.mock import Mock, patch, AsyncMock
from app.services.igdb import IGDBService, GameMetadata, TwitchAuthError, IGDBError


class TestIGDBService:
    """Test cases for IGDB service."""

    def test_rank_games_by_fuzzy_match_exact_match(self):
        """Test that exact matches get highest priority."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id="1", title="The Witcher 3"),
            GameMetadata(igdb_id="2", title="Witcher 2"),
            GameMetadata(igdb_id="3", title="Some Other Game")
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "The Witcher 3", threshold=0.5)
        
        # Exact match should be first
        assert result[0].title == "The Witcher 3"
        assert len(result) >= 1
    
    def test_rank_games_by_fuzzy_match_partial_match(self):
        """Test that partial matches work correctly."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id="1", title="The Witcher 3: Wild Hunt"),
            GameMetadata(igdb_id="2", title="Witcher 2: Assassins of Kings"),
            GameMetadata(igdb_id="3", title="Some Other Game")
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "Witcher 3", threshold=0.5)
        
        # Should find Witcher 3 first due to partial match
        assert result[0].title == "The Witcher 3: Wild Hunt"
        assert len(result) >= 1
    
    def test_rank_games_by_fuzzy_match_threshold_filtering(self):
        """Test that threshold filtering works correctly."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id="1", title="The Witcher 3"),
            GameMetadata(igdb_id="2", title="Completely Different Game")
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "Witcher", threshold=0.8)
        
        # Only Witcher should pass high threshold
        assert len(result) == 1
        assert result[0].title == "The Witcher 3"
    
    def test_rank_games_by_fuzzy_match_empty_input(self):
        """Test handling of empty input."""
        service = IGDBService()
        
        result = service._rank_games_by_fuzzy_match([], "test", threshold=0.5)
        assert result == []
        
        games = [GameMetadata(igdb_id="1", title="Test Game")]
        result = service._rank_games_by_fuzzy_match(games, "", threshold=0.5)
        assert result == games
    
    def test_rank_games_by_fuzzy_match_case_insensitive(self):
        """Test that matching is case insensitive."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id="1", title="The Witcher 3"),
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "THE WITCHER 3", threshold=0.5)
        
        assert len(result) == 1
        assert result[0].title == "The Witcher 3"
    
    def test_rank_games_by_fuzzy_match_token_sorting(self):
        """Test that token sorting works for reordered words."""
        service = IGDBService()
        
        games = [
            GameMetadata(igdb_id="1", title="Grand Theft Auto V"),
            GameMetadata(igdb_id="2", title="Another Game")
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "Auto Grand Theft V", threshold=0.5)
        
        # Should still match despite word reordering
        assert len(result) >= 1
        assert result[0].title == "Grand Theft Auto V"

    @pytest.mark.asyncio
    async def test_search_games_calls_fuzzy_matching(self):
        """Test that search_games calls fuzzy matching."""
        service = IGDBService()
        
        # Mock the IGDB wrapper and API response
        mock_wrapper = Mock()
        mock_wrapper.api_request.return_value = b'[{"id": 1, "name": "Test Game"}]'
        
        with patch.object(service, '_get_wrapper', return_value=mock_wrapper):
            with patch.object(service, '_rank_games_by_fuzzy_match') as mock_rank:
                mock_rank.return_value = [
                    GameMetadata(igdb_id="1", title="Test Game")
                ]
                
                result = await service.search_games("test", limit=10)
                
                # Verify fuzzy matching was called
                mock_rank.assert_called_once()
                args, kwargs = mock_rank.call_args
                assert args[1] == "test"  # query parameter
                assert args[2] == 0.6  # default threshold
    
    @pytest.mark.asyncio
    async def test_search_games_error_handling(self):
        """Test error handling in search_games."""
        service = IGDBService()
        
        # Mock authentication error
        with patch.object(service, '_get_wrapper', side_effect=TwitchAuthError("Auth failed")):
            with pytest.raises(IGDBError):
                await service.search_games("test")
    
    @pytest.mark.asyncio
    async def test_get_game_by_id_success(self):
        """Test successful game retrieval by ID."""
        service = IGDBService()
        
        mock_wrapper = Mock()
        mock_wrapper.api_request.return_value = b'[{"id": 1, "name": "Test Game"}]'
        
        with patch.object(service, '_get_wrapper', return_value=mock_wrapper):
            with patch.object(service, '_parse_game_data') as mock_parse:
                mock_parse.return_value = GameMetadata(igdb_id="1", title="Test Game")
                
                result = await service.get_game_by_id("1")
                
                assert result is not None
                assert result.title == "Test Game"
                mock_parse.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_get_game_by_id_not_found(self):
        """Test handling of game not found."""
        service = IGDBService()
        
        mock_wrapper = Mock()
        mock_wrapper.api_request.return_value = b'[]'
        
        with patch.object(service, '_get_wrapper', return_value=mock_wrapper):
            result = await service.get_game_by_id("nonexistent")
            assert result is None


class TestGameMetadata:
    """Test cases for GameMetadata dataclass."""
    
    def test_game_metadata_creation(self):
        """Test GameMetadata creation with required fields."""
        metadata = GameMetadata(
            igdb_id="123",
            title="Test Game"
        )
        
        assert metadata.igdb_id == "123"
        assert metadata.title == "Test Game"
        assert metadata.description is None
        assert metadata.genre is None
    
    def test_game_metadata_with_optional_fields(self):
        """Test GameMetadata with optional fields."""
        metadata = GameMetadata(
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
        
        assert metadata.description == "A test game"
        assert metadata.genre == "Action"
        assert metadata.developer == "Test Studio"
        assert metadata.publisher == "Test Publisher"
        assert metadata.release_date == "2023-01-01"
        assert metadata.cover_art_url == "https://example.com/cover.jpg"
        assert metadata.rating_average == 8.5
        assert metadata.rating_count == 100
        assert metadata.estimated_playtime_hours == 40
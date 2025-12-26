"""
Tests for IGDB service functionality including fuzzy matching.
"""

import pytest
from unittest.mock import Mock, patch
from app.services.igdb import IGDBService, GameMetadata, TwitchAuthError, IGDBError
from app.utils.rate_limiter import RateLimitConfig, create_igdb_rate_limiter


def create_test_igdb_service() -> IGDBService:
    """Create an IGDBService with a local rate limiter for testing."""
    rate_config = RateLimitConfig(
        requests_per_second=4.0,
        burst_capacity=8,
        backoff_factor=1.0,
        max_retries=3
    )
    rate_limiter = create_igdb_rate_limiter(rate_config)
    return IGDBService(rate_limiter=rate_limiter)


class TestIGDBService:
    """Test cases for IGDB service."""

    def test_rank_games_by_fuzzy_match_exact_match(self):
        """Test that exact matches get highest priority."""
        service = create_test_igdb_service()
        
        games = [
            GameMetadata(igdb_id=1, title="The Witcher 3"),
            GameMetadata(igdb_id=2, title="Witcher 2"),
            GameMetadata(igdb_id=3, title="Some Other Game")
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "The Witcher 3", threshold=0.5)
        
        # Exact match should be first
        assert result[0].title == "The Witcher 3"
        assert len(result) >= 1
    
    def test_rank_games_by_fuzzy_match_partial_match(self):
        """Test that partial matches work correctly."""
        service = create_test_igdb_service()
        
        games = [
            GameMetadata(igdb_id=1, title="The Witcher 3: Wild Hunt"),
            GameMetadata(igdb_id=2, title="Witcher 2: Assassins of Kings"),
            GameMetadata(igdb_id=3, title="Some Other Game")
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "Witcher 3", threshold=0.5)
        
        # Should find Witcher 3 first due to partial match
        assert result[0].title == "The Witcher 3: Wild Hunt"
        assert len(result) >= 1
    
    def test_rank_games_by_fuzzy_match_threshold_filtering(self):
        """Test that threshold filtering works correctly."""
        service = create_test_igdb_service()
        
        games = [
            GameMetadata(igdb_id=1, title="The Witcher 3"),
            GameMetadata(igdb_id=2, title="Completely Different Game")
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "Witcher", threshold=0.8)
        
        # Only Witcher should pass high threshold
        assert len(result) == 1
        assert result[0].title == "The Witcher 3"
    
    def test_rank_games_by_fuzzy_match_empty_input(self):
        """Test handling of empty input."""
        service = create_test_igdb_service()
        
        result = service._rank_games_by_fuzzy_match([], "test", threshold=0.5)
        assert result == []
        
        games = [GameMetadata(igdb_id=1, title="Test Game")]
        result = service._rank_games_by_fuzzy_match(games, "", threshold=0.5)
        assert result == games
    
    def test_rank_games_by_fuzzy_match_case_insensitive(self):
        """Test that matching is case insensitive."""
        service = create_test_igdb_service()
        
        games = [
            GameMetadata(igdb_id=1, title="The Witcher 3"),
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "THE WITCHER 3", threshold=0.5)
        
        assert len(result) == 1
        assert result[0].title == "The Witcher 3"
    
    def test_rank_games_by_fuzzy_match_token_sorting(self):
        """Test that token sorting works for reordered words."""
        service = create_test_igdb_service()
        
        games = [
            GameMetadata(igdb_id=1, title="Grand Theft Auto V"),
            GameMetadata(igdb_id=2, title="Another Game")
        ]
        
        result = service._rank_games_by_fuzzy_match(games, "Auto Grand Theft V", threshold=0.5)
        
        # Should still match despite word reordering
        assert len(result) >= 1
        assert result[0].title == "Grand Theft Auto V"

    @pytest.mark.asyncio
    async def test_search_games_calls_fuzzy_matching(self):
        """Test that search_games calls fuzzy matching."""
        service = create_test_igdb_service()

        # Mock the IGDB wrapper and API response
        mock_wrapper = Mock()
        mock_wrapper.api_request.return_value = b'[{"id": 1, "name": "Test Game"}]'

        # _get_wrapper is now async, so we need AsyncMock
        async def mock_get_wrapper():
            return mock_wrapper

        with patch.object(service, '_get_wrapper', mock_get_wrapper):
            with patch.object(service, '_rank_games_by_fuzzy_match') as mock_rank:
                mock_rank.return_value = [
                    GameMetadata(igdb_id=1, title="Test Game")
                ]

                await service.search_games("test", limit=10)

                # Verify fuzzy matching was called
                mock_rank.assert_called_once()
                args, kwargs = mock_rank.call_args
                assert args[1] == "test"  # query parameter
                assert args[2] == 0.6  # default threshold
    
    @pytest.mark.asyncio
    async def test_search_games_error_handling(self):
        """Test error handling in search_games."""
        service = create_test_igdb_service()

        # Mock authentication error - _get_wrapper is now async
        async def mock_get_wrapper():
            raise TwitchAuthError("Auth failed")

        with patch.object(service, '_get_wrapper', mock_get_wrapper):
            with pytest.raises(IGDBError):
                await service.search_games("test")
    
    @pytest.mark.asyncio
    async def test_get_game_by_id_success(self):
        """Test successful game retrieval by ID."""
        service = create_test_igdb_service()

        mock_wrapper = Mock()
        mock_wrapper.api_request.return_value = b'[{"id": 1, "name": "Test Game"}]'

        async def mock_get_wrapper():
            return mock_wrapper

        with patch.object(service, '_get_wrapper', mock_get_wrapper):
            with patch('app.services.igdb.service.parse_game_data') as mock_parse:
                mock_parse.return_value = GameMetadata(igdb_id=1, title="Test Game")

                result = await service.get_game_by_id(1)

                assert result is not None
                assert result.title == "Test Game"
                mock_parse.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_game_by_id_not_found(self):
        """Test handling of game not found."""
        service = create_test_igdb_service()

        mock_wrapper = Mock()
        mock_wrapper.api_request.return_value = b'[]'

        async def mock_get_wrapper():
            return mock_wrapper

        with patch.object(service, '_get_wrapper', mock_get_wrapper):
            result = await service.get_game_by_id(999999)
            assert result is None


class TestGameMetadata:
    """Test cases for GameMetadata dataclass."""
    
    def test_game_metadata_creation(self):
        """Test GameMetadata creation with required fields."""
        metadata = GameMetadata(
            igdb_id=123,
            title="Test Game"
        )
        
        assert metadata.igdb_id == 123
        assert metadata.title == "Test Game"
        assert metadata.description is None
        assert metadata.genre is None
    
    def test_game_metadata_with_optional_fields(self):
        """Test GameMetadata with optional fields."""
        metadata = GameMetadata(
            igdb_id=123,
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


class TestKeywordExpansion:
    """Test cases for keyword expansion functionality."""
    
    def test_detect_keywords_goty(self):
        """Test detection of GOTY keyword."""
        service = create_test_igdb_service()
        
        # Test various forms of GOTY
        test_cases = [
            ("GOTY 2023", {"goty": "Game of the Year"}),
            ("Best GOTY games", {"goty": "Game of the Year"}),
            ("goty edition", {"goty": "Game of the Year"}),
            ("Goty nominees", {"goty": "Game of the Year"}),
            ("What is the GOTY?", {"goty": "Game of the Year"}),
        ]
        
        for query, expected in test_cases:
            result = service._detect_keywords(query)
            assert result == expected, f"Failed for query: '{query}'"
    
    def test_detect_keywords_no_match(self):
        """Test that non-keywords are not detected."""
        service = create_test_igdb_service()
        
        # Test cases that should NOT match
        test_cases = [
            "great games",
            "gothic game",  # Contains 'got' but not 'goty'
            "mythology game",  # Contains 'oty' but not 'goty'
            "got you",  # Contains 'got' but not 'goty'
            "",  # Empty string
        ]
        
        for query in test_cases:
            result = service._detect_keywords(query)
            assert result == {}, f"False positive for query: '{query}'"
    
    def test_detect_keywords_word_boundaries(self):
        """Test that keyword detection respects word boundaries."""
        service = create_test_igdb_service()
        
        # These should NOT match because goty is part of a larger word
        no_match_cases = [
            "ergoty game",  # goty at end of word
            "gotyness",     # goty at start of word
            "ergotycool",   # goty in middle of word
        ]
        
        for query in no_match_cases:
            result = service._detect_keywords(query)
            assert result == {}, f"Should not match word boundary case: '{query}'"
    
    def test_generate_expanded_queries(self):
        """Test generation of expanded queries."""
        service = create_test_igdb_service()
        
        test_cases = [
            ("GOTY 2023", {"goty": "Game of the Year"}, ["Game of the Year 2023"]),
            ("Best GOTY games", {"goty": "Game of the Year"}, ["Best Game of the Year games"]),
            ("goty edition", {"goty": "Game of the Year"}, ["Game of the Year edition"]),
        ]
        
        for original, keywords, expected in test_cases:
            result = service._generate_expanded_queries(original, keywords)
            assert result == expected, f"Failed for query: '{original}'"
    
    def test_generate_expanded_queries_case_preservation(self):
        """Test that case is handled correctly in expanded queries."""
        service = create_test_igdb_service()
        
        # Test mixed case scenarios
        keywords = {"goty": "Game of the Year"}
        test_cases = [
            ("GOTY winners", ["Game of the Year winners"]),
            ("Goty nominees", ["Game of the Year nominees"]),
            ("best goty", ["best Game of the Year"]),
        ]
        
        for original, expected in test_cases:
            result = service._generate_expanded_queries(original, keywords)
            assert result == expected, f"Case handling failed for: '{original}'"
    
    def test_detect_keywords_telltale_series(self):
        """Test detection of 'The Telltale Series' keyword."""
        service = create_test_igdb_service()
        
        # Test various forms of The Telltale Series
        test_cases = [
            ("The Walking Dead: The Telltale Series", {":": " ", "The Telltale Series": ""}),
            ("Batman: The Telltale Series Episode 1", {":": " ", "The Telltale Series": ""}),
            ("The Wolf Among Us: The Telltale Series", {":": " ", "The Telltale Series": ""}),
            ("the telltale series game", {"The Telltale Series": ""}),  # Case insensitive
            ("The Telltale Series - Season 1", {"The Telltale Series": ""}),
        ]
        
        for query, expected in test_cases:
            result = service._detect_keywords(query)
            assert result == expected, f"Failed to detect Telltale Series in query: '{query}'"
    
    def test_detect_keywords_telltale_series_no_match(self):
        """Test that partial telltale matches don't trigger false positives."""
        service = create_test_igdb_service()
        
        # Test cases that should NOT match
        test_cases = [
            "Telltale Games",  # Just the company name
            "A Telltale",      # Part of the phrase
            "Series finale",   # Just "Series"
            "The series",      # Just "The" and "series" separately
            "Tell tale story", # "tell tale" as separate words
        ]
        
        for query in test_cases:
            result = service._detect_keywords(query)
            # Should not contain The Telltale Series key
            assert "The Telltale Series" not in result, f"False positive for query: '{query}'"
    
    def test_generate_expanded_queries_telltale_removal(self):
        """Test generation of queries with Telltale Series removal."""
        service = create_test_igdb_service()
        
        test_cases = [
            # Standard cases
            ("The Walking Dead: The Telltale Series", {"The Telltale Series": ""}, ["The Walking Dead"]),
            ("Batman: The Telltale Series Episode 1", {"The Telltale Series": ""}, ["Batman: Episode 1"]),
            
            # At the beginning
            ("The Telltale Series Walking Dead", {"The Telltale Series": ""}, ["Walking Dead"]),
            
            # At the end
            ("Walking Dead The Telltale Series", {"The Telltale Series": ""}, ["Walking Dead"]),
            
            # With multiple spaces
            ("Batman:   The Telltale Series  Episode", {"The Telltale Series": ""}, ["Batman: Episode"]),
            
            # Case variations
            ("the telltale series batman", {"The Telltale Series": ""}, ["batman"]),
        ]
        
        for original, keywords, expected in test_cases:
            result = service._generate_expanded_queries(original, keywords)
            assert result == expected, f"Failed removal for query: '{original}', got: {result}"
    
    def test_generate_expanded_queries_telltale_cleanup(self):
        """Test whitespace cleanup after Telltale Series removal."""
        service = create_test_igdb_service()
        
        # Test edge cases for whitespace cleanup
        keywords = {"The Telltale Series": ""}
        test_cases = [
            # Trailing colon cleanup
            ("Game: The Telltale Series", ["Game"]),
            ("Game: The Telltale Series :", ["Game"]),
            
            # Multiple spaces cleanup  
            ("Before  The Telltale Series  After", ["Before After"]),
            
            # Leading/trailing whitespace
            ("  The Telltale Series Game  ", ["Game"]),
            ("Game The Telltale Series  ", ["Game"]),
            ("  The Telltale Series", [""]),
        ]
        
        for original, expected in test_cases:
            result = service._generate_expanded_queries(original, keywords)
            assert result == expected, f"Cleanup failed for: '{original}', got: {result}"
    
    def test_detect_keywords_trademark_symbol(self):
        """Test detection of ® registered trademark symbol."""
        service = create_test_igdb_service()
        
        # Test various cases with trademark symbol
        test_cases = [
            ("Rocket League®", {"®": ""}),
            ("FIFA® 24", {"®": ""}),
            ("Call of Duty®: Modern Warfare", {":": " ", "®": ""}),
            ("®Game Title", {"®": ""}),
            ("Game® Title® Here", {"®": ""}),  # Multiple symbols
            ("Pokémon® Go", {"®": ""}),
        ]
        
        for query, expected in test_cases:
            result = service._detect_keywords(query)
            assert result == expected, f"Failed to detect ® symbol in query: '{query}'"
    
    def test_detect_keywords_trademark_symbol_no_match(self):
        """Test that similar characters don't trigger false positives for ®."""
        service = create_test_igdb_service()
        
        # Test cases that should NOT match
        test_cases = [
            "Regular game",     # No symbol
            "Game title",       # No symbol
            "R game",          # Just the letter R
            "Game R rating",   # R as separate letter
        ]
        
        for query in test_cases:
            result = service._detect_keywords(query)
            # Should not contain ® key
            assert "®" not in result, f"False positive for query: '{query}'"
    
    def test_generate_expanded_queries_trademark_removal(self):
        """Test generation of queries with ® symbol removal."""
        service = create_test_igdb_service()
        
        test_cases = [
            # Standard cases
            ("Rocket League®", {"®": ""}, ["Rocket League"]),
            ("FIFA® 24", {"®": ""}, ["FIFA 24"]),
            ("Call of Duty®: Modern Warfare", {"®": ""}, ["Call of Duty: Modern Warfare"]),
            
            # At the beginning
            ("®Game Title", {"®": ""}, ["Game Title"]),
            
            # Multiple symbols
            ("Game® Title® Here", {"®": ""}, ["Game Title Here"]),
            
            # With other punctuation
            ("Pokémon® Go!", {"®": ""}, ["Pokémon Go!"]),
            
            # No spaces around symbol
            ("Game®Title", {"®": ""}, ["GameTitle"]),
        ]
        
        for original, keywords, expected in test_cases:
            result = service._generate_expanded_queries(original, keywords)
            assert result == expected, f"Failed removal for query: '{original}', got: {result}"
    
    def test_generate_expanded_queries_mixed_keywords_with_symbols(self):
        """Test that symbol keywords work alongside text keywords."""
        service = create_test_igdb_service()
        
        # Test mixed keywords: text + symbol
        test_query = "FIFA® GOTY Edition"
        detected = service._detect_keywords(test_query)
        expected_detected = {"®": "", "goty": "Game of the Year"}
        
        assert detected == expected_detected, f"Detection failed: {detected}"
        
        # Test expansion
        expanded = service._generate_expanded_queries(test_query, detected)
        expected_expanded = [
            "FIFA® Game of the Year Edition",  # GOTY expanded
            "FIFA GOTY Edition",               # ® removed
            "FIFA Game of the Year Edition"    # Combined: both ® removed AND GOTY expanded
        ]
        
        assert set(expanded) == set(expected_expanded), f"Mixed expansion failed: {expanded}"
    
    def test_detect_keywords_number_one(self):
        """Test detection of standalone number '1'."""
        service = create_test_igdb_service()
        
        # Test various cases with number 1
        test_cases = [
            ("Portal 1", {"1": ""}),
            ("Mass Effect 1", {"1": ""}),
            ("Game Title 1", {"1": ""}),
            ("Halo 1", {"1": ""}),
            ("FIFA 1", {"1": ""}),
        ]
        
        for query, expected in test_cases:
            result = service._detect_keywords(query)
            assert result == expected, f"Failed to detect '1' in query: '{query}'"
    
    def test_detect_keywords_number_one_no_match(self):
        """Test that '1' detection avoids false positives."""
        service = create_test_igdb_service()
        
        # Test cases that should NOT match
        test_cases = [
            "Counter-Strike 1.6",  # Part of version number
            "Level 11",           # Part of larger number  
            "1998 Game",         # At start but not standalone
            "Game 10",           # Different number
            "Team 1-2",          # Part of range
            "Part 1-3",          # Part of range
            "Volume 15",         # Different number
        ]
        
        for query in test_cases:
            result = service._detect_keywords(query)
            # Should not contain "1" key
            assert "1" not in result, f"False positive for query: '{query}'"
    
    def test_detect_keywords_year_parentheses(self):
        """Test detection of years in parentheses."""
        service = create_test_igdb_service()
        
        # Test various year formats
        test_cases = [
            ("Call of Duty (2003)", {"(2003)": ""}),
            ("FIFA 24 (2024)", {"(2024)": ""}),
            ("The Witcher 3 (2015)", {"(2015)": ""}),
            ("Game Title (1999)", {"(1999)": ""}),
            ("Old Game (1980)", {"(1980)": ""}),
            ("Future Game (2030)", {"(2030)": ""}),
        ]
        
        for query, expected in test_cases:
            result = service._detect_keywords(query)
            assert result == expected, f"Failed to detect year pattern in query: '{query}'"
    
    def test_detect_keywords_year_parentheses_no_match(self):
        """Test that year parentheses detection avoids false positives."""
        service = create_test_igdb_service()
        
        # Test cases that should NOT match
        test_cases = [
            "Game (2)",           # Too few digits
            "Game (99)",          # Too few digits
            "Game (12345)",       # Too many digits
            "Game 2023",          # No parentheses
            "Game (DLC)",         # Not a number
            "Game (v1.5)",        # Not just year
            "(End)",              # Not a year
        ]
        
        for query in test_cases:
            result = service._detect_keywords(query)
            # Should not contain any year patterns
            year_keys = [k for k in result.keys() if k.startswith('(') and k.endswith(')')]
            assert len(year_keys) == 0, f"False positive year detection for query: '{query}'"
    
    def test_generate_expanded_queries_number_removal(self):
        """Test generation of queries with number '1' removal."""
        service = create_test_igdb_service()
        
        test_cases = [
            # Standard cases - note that trailing spaces are cleaned up
            ("Portal 1", {"1": ""}, ["Portal"]),
            ("Mass Effect 1", {"1": ""}, ["Mass Effect"]),
            ("Game Title 1: Subtitle", {"1": ""}, ["Game Title: Subtitle"]),
            
            # With other text - space between "1" and "Legendary" is removed as one unit
            ("Halo 1 Legendary Edition", {"1 ": ""}, ["Halo Legendary Edition"]),
        ]
        
        for original, keywords, expected in test_cases:
            result = service._generate_expanded_queries(original, keywords)
            assert result == expected, f"Failed number removal for query: '{original}', got: {result}"
    
    def test_generate_expanded_queries_year_removal(self):
        """Test generation of queries with year parentheses removal."""
        service = create_test_igdb_service()
        
        test_cases = [
            # Standard cases
            ("Call of Duty (2003)", {"(2003)": ""}, ["Call of Duty"]),
            ("FIFA 24 (2024)", {"(2024)": ""}, ["FIFA 24"]),
            ("The Witcher 3 (2015)", {"(2015)": ""}, ["The Witcher 3"]),
            
            # With other punctuation
            ("Game: Title (1999)", {"(1999)": ""}, ["Game: Title"]),
            ("Game Title! (2020)", {"(2020)": ""}, ["Game Title!"]),
        ]
        
        for original, keywords, expected in test_cases:
            result = service._generate_expanded_queries(original, keywords)
            assert result == expected, f"Failed year removal for query: '{original}', got: {result}"
    
    def test_generate_expanded_queries_complex_mixed_patterns(self):
        """Test complex scenarios with multiple pattern types."""
        service = create_test_igdb_service()
        
        # Test mixed patterns: number + year + other keywords
        test_query = "Mass Effect 1 GOTY (2007)"
        detected = service._detect_keywords(test_query)
        expected_detected = {"1 ": "", "goty": "Game of the Year", "(2007)": ""}
        
        assert detected == expected_detected, f"Complex detection failed: {detected}"
        
        # Test expansion - should generate individual transformations plus combined
        expanded = service._generate_expanded_queries(test_query, detected)
        
        # Should include individual transformations
        assert "Mass Effect 1 Game of the Year (2007)" in expanded  # GOTY expanded
        assert "Mass Effect GOTY (2007)" in expanded              # "1 " removed  
        assert "Mass Effect 1 GOTY" in expanded                   # (2007) removed
        
        # Should also include combined transformation
        assert "Mass Effect Game of the Year" in expanded         # All transformations combined
        
        # Should have exactly 4 expansions (3 individual + 1 combined)
        assert len(expanded) == 4, f"Expected 4 expansions, got: {expanded}"
    
    def test_merge_and_deduplicate_results(self):
        """Test result merging and deduplication."""
        service = create_test_igdb_service()
        
        # Create test data with some overlapping IGDB IDs
        original_results = [
            GameMetadata(igdb_id=1, title="Game A"),
            GameMetadata(igdb_id=2, title="Game B"),
        ]
        
        expanded_results = [
            [
                GameMetadata(igdb_id=2, title="Game B"),  # Duplicate
                GameMetadata(igdb_id=3, title="Game C"),  # New
            ],
            [
                GameMetadata(igdb_id=1, title="Game A"),  # Duplicate
                GameMetadata(igdb_id=4, title="Game D"),  # New
            ]
        ]
        
        result = service._merge_and_deduplicate_results(original_results, expanded_results, limit=10)
        
        # Should have 4 unique games
        assert len(result) == 4
        
        # Original results should appear first
        assert result[0].igdb_id == 1  # Game A from original
        assert result[1].igdb_id == 2  # Game B from original
        
        # Check all IDs are unique
        seen_ids = set()
        for game in result:
            assert game.igdb_id not in seen_ids, f"Duplicate ID found: {game.igdb_id}"
            seen_ids.add(game.igdb_id)
    
    def test_merge_and_deduplicate_results_with_limit(self):
        """Test result merging respects limit."""
        service = create_test_igdb_service()
        
        original_results = [
            GameMetadata(igdb_id=1, title="Game A"),
            GameMetadata(igdb_id=2, title="Game B"),
        ]
        
        expanded_results = [
            [
                GameMetadata(igdb_id=3, title="Game C"),
                GameMetadata(igdb_id=4, title="Game D"),
                GameMetadata(igdb_id=5, title="Game E"),
            ]
        ]
        
        result = service._merge_and_deduplicate_results(original_results, expanded_results, limit=3)
        
        # Should respect limit of 3
        assert len(result) == 3
        
        # Original results should appear first
        assert result[0].igdb_id == 1
        assert result[1].igdb_id == 2
        assert result[2].igdb_id == 3  # First from expanded
    
    def test_merge_and_deduplicate_empty_results(self):
        """Test merging with empty results."""
        service = create_test_igdb_service()
        
        # Test with empty original results
        result = service._merge_and_deduplicate_results([], [[GameMetadata(igdb_id=1, title="Game A")]], limit=10)
        assert len(result) == 1
        assert result[0].igdb_id == 1
        
        # Test with empty expanded results
        original = [GameMetadata(igdb_id=1, title="Game A")]
        result = service._merge_and_deduplicate_results(original, [], limit=10)
        assert len(result) == 1
        assert result[0].igdb_id == 1
        
        # Test with both empty
        result = service._merge_and_deduplicate_results([], [], limit=10)
        assert len(result) == 0
    
    @pytest.mark.asyncio
    async def test_search_games_with_keyword_expansion(self):
        """Test end-to-end search with keyword expansion."""
        service = create_test_igdb_service()
        
        # Mock the single search method to return different results
        original_game = GameMetadata(igdb_id=1, title="GOTY Award Winner")
        expanded_game = GameMetadata(igdb_id=2, title="Game of the Year 2023")
        
        async def mock_single_search(query, limit):
            if "GOTY" in query:
                return [original_game]
            elif "Game of the Year" in query:
                return [expanded_game]
            return []
        
        # Mock fuzzy matching to return all games as-is
        def mock_fuzzy_match(games, query, threshold):
            return games
        
        with patch.object(service, '_perform_single_search', side_effect=mock_single_search):
            with patch.object(service, '_rank_games_by_fuzzy_match', side_effect=mock_fuzzy_match):
                result = await service.search_games("GOTY 2023", limit=10)
                
                # Should get both results merged
                assert len(result) == 2
                
                # Original result should appear first
                assert result[0].igdb_id == 1
                assert result[1].igdb_id == 2
    
    @pytest.mark.asyncio
    async def test_search_games_without_keywords(self):
        """Test search without keywords works normally."""
        service = create_test_igdb_service()
        
        game = GameMetadata(igdb_id=1, title="Regular Game")
        
        # Mock fuzzy matching to return all games as-is
        def mock_fuzzy_match(games, query, threshold):
            return games
        
        with patch.object(service, '_perform_single_search', return_value=[game]):
            with patch.object(service, '_rank_games_by_fuzzy_match', side_effect=mock_fuzzy_match):
                result = await service.search_games("Regular Game", limit=10)
                
                # Should get only the original search result
                assert len(result) == 1
                assert result[0].igdb_id == 1
    
    @pytest.mark.asyncio
    async def test_search_games_expansion_failure_fallback(self):
        """Test that expansion failures don't break the search."""
        service = create_test_igdb_service()
        
        original_game = GameMetadata(igdb_id=1, title="GOTY Winner")
        
        async def mock_single_search(query, limit):
            if "GOTY" in query:
                return [original_game]
            elif "Game of the Year" in query:
                raise IGDBError("Expanded search failed")
            return []
        
        # Mock fuzzy matching to return all games as-is
        def mock_fuzzy_match(games, query, threshold):
            return games
        
        with patch.object(service, '_perform_single_search', side_effect=mock_single_search):
            with patch.object(service, '_rank_games_by_fuzzy_match', side_effect=mock_fuzzy_match):
                # Should not raise exception, should return original results
                result = await service.search_games("GOTY 2023", limit=10)
                
                assert len(result) == 1
                assert result[0].igdb_id == 1
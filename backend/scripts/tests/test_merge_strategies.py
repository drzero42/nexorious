"""
Tests for merge strategy modules.

This module tests the three merge strategies (Interactive, Overwrite, Preserve)
for proper idempotency and conflict resolution behavior.
"""

import pytest
import asyncio
from unittest.mock import AsyncMock, MagicMock, patch
from pathlib import Path
from datetime import datetime
import json
import tempfile

from scripts.darkadia.merge_strategies import (
    MergeStrategy, InteractiveMerger, OverwriteMerger, PreserveMerger
)
from scripts.darkadia.api_client import NexoriousAPIClient, APIException
from rich.console import Console


class TestMergeStrategyBaseFunctionality:
    """Test cases for base MergeStrategy functionality."""
    
    @pytest.fixture
    def mock_api_client(self):
        """Create a mock API client."""
        return AsyncMock(spec=NexoriousAPIClient)
    
    @pytest.fixture
    def base_strategy(self, mock_api_client):
        """Create a concrete implementation of MergeStrategy for testing."""
        class TestMergeStrategy(MergeStrategy):
            async def process_games(self, games, user_id, batch_size=10):
                return self.results
            
            async def handle_conflict(self, existing_game, csv_game):
                return {'action': 'skip'}
        
        return TestMergeStrategy(mock_api_client, dry_run=False)
    
    @pytest.mark.asyncio
    async def test_find_existing_game_exact_match(self, base_strategy, mock_api_client):
        """Test finding existing game with exact title match."""
        # Mock search results with exact match
        mock_api_client.search_games.return_value = [
            {'id': '1', 'title': 'Test Game', 'rating': 8.5},
            {'id': '2', 'title': 'Test Game 2', 'rating': 7.0}
        ]
        
        result = await base_strategy.find_existing_game('Test Game', 'user123')
        
        assert result is not None
        assert result['id'] == '1'
        assert result['title'] == 'Test Game'
        
        # Should try high fuzzy threshold first
        mock_api_client.search_games.assert_called_with('Test Game', fuzzy_threshold=0.95)
    
    @pytest.mark.asyncio
    async def test_find_existing_game_case_insensitive(self, base_strategy, mock_api_client):
        """Test finding existing game with case insensitive matching."""
        mock_api_client.search_games.return_value = [
            {'id': '1', 'title': 'Test Game', 'rating': 8.5}
        ]
        
        result = await base_strategy.find_existing_game('test game', 'user123')
        
        assert result is not None
        assert result['id'] == '1'
    
    @pytest.mark.asyncio
    async def test_find_existing_game_fallback_threshold(self, base_strategy, mock_api_client):
        """Test fallback to lower fuzzy threshold when no high-confidence matches."""
        # First call (high threshold) returns empty, second call (lower threshold) returns result
        mock_api_client.search_games.side_effect = [[], [{'id': '1', 'title': 'Similar Game'}]]
        
        result = await base_strategy.find_existing_game('Test Game', 'user123')
        
        assert result is not None
        assert result['id'] == '1'
        
        # Should have called both thresholds
        assert mock_api_client.search_games.call_count == 2
        calls = mock_api_client.search_games.call_args_list
        assert calls[0][1]['fuzzy_threshold'] == 0.95
        assert calls[1][1]['fuzzy_threshold'] == 0.85
    
    @pytest.mark.asyncio
    async def test_find_existing_game_no_match(self, base_strategy, mock_api_client):
        """Test when no existing game is found."""
        mock_api_client.search_games.return_value = []
        
        result = await base_strategy.find_existing_game('Nonexistent Game', 'user123')
        
        assert result is None
    
    @pytest.mark.asyncio
    async def test_add_new_platforms_only(self, base_strategy, mock_api_client):
        """Test adding only new platforms, skipping duplicates."""
        existing_game = {
            'id': 'game123',
            'title': 'Test Game',
            'platforms': [
                {'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'},
                {'platform_name': 'PlayStation 4', 'storefront_name': 'PlayStation Store'}
            ]
        }
        
        new_platforms = [
            {'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'},  # Duplicate
            {'platform_name': 'PC (Windows)', 'storefront_name': 'Epic Games Store'},  # New
            {'platform_name': 'Xbox One', 'storefront_name': 'Microsoft Store'}  # New
        ]
        
        mock_api_client.add_platform_to_user_game.return_value = True
        
        await base_strategy._add_new_platforms_only(existing_game, new_platforms)
        
        # Should only call API for the 2 new platforms
        assert mock_api_client.add_platform_to_user_game.call_count == 2
        
        # Verify the correct platforms were added
        calls = mock_api_client.add_platform_to_user_game.call_args_list
        added_platforms = [call[0][1] for call in calls]
        
        assert any(p['storefront_name'] == 'Epic Games Store' for p in added_platforms)
        assert any(p['storefront_name'] == 'Microsoft Store' for p in added_platforms)
        assert not any(p['storefront_name'] == 'Steam' for p in added_platforms)
    
    def test_get_new_platforms_to_add(self, base_strategy):
        """Test filtering new platforms from existing ones."""
        existing_game = {
            'platforms': [
                {'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'},
                {'platform_name': 'PlayStation 4', 'storefront_name': 'PlayStation Store'}
            ]
        }
        
        new_platforms = [
            {'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'},  # Duplicate
            {'platform_name': 'PC (Windows)', 'storefront_name': 'GOG'},  # New
            {'platform_name': 'Nintendo Switch', 'storefront_name': 'Nintendo eShop'}  # New
        ]
        
        result = base_strategy._get_new_platforms_to_add(existing_game, new_platforms)
        
        assert len(result) == 2
        assert any(p['storefront_name'] == 'GOG' for p in result)
        assert any(p['storefront_name'] == 'Nintendo eShop' for p in result)
        assert not any(p['storefront_name'] == 'Steam' for p in result)


class TestInteractiveMerger:
    """Test cases for InteractiveMerger."""
    
    @pytest.fixture
    def mock_console(self):
        """Create a mock console."""
        return MagicMock(spec=Console)
    
    @pytest.fixture
    def mock_api_client(self):
        """Create a mock API client."""
        return AsyncMock(spec=NexoriousAPIClient)
    
    @pytest.fixture
    def temp_cache_dir(self):
        """Create a temporary directory for decision cache."""
        with tempfile.TemporaryDirectory() as temp_dir:
            yield Path(temp_dir)
    
    @pytest.fixture
    def interactive_merger(self, mock_console, mock_api_client, temp_cache_dir):
        """Create an InteractiveMerger with mocked dependencies."""
        merger = InteractiveMerger(mock_console, mock_api_client, dry_run=False)
        # Override the cache file path to use temp directory
        merger.decision_cache_file = temp_cache_dir / 'test_decisions.json'
        merger.persistent_decisions = {}
        return merger
    
    def test_decision_cache_loading(self, interactive_merger, temp_cache_dir):
        """Test loading decisions from cache file."""
        # Create a cache file with test decisions
        cache_file = temp_cache_dir / 'test_decisions.json'
        test_decisions = {
            'hash123': {
                'choice': '2',
                'choice_description': 'Use CSV data',
                'timestamp': '2023-01-01T00:00:00'
            }
        }
        
        with open(cache_file, 'w') as f:
            json.dump(test_decisions, f)
        
        # Reload decisions
        interactive_merger._load_persistent_decisions()
        
        assert len(interactive_merger.persistent_decisions) == 1
        assert 'hash123' in interactive_merger.persistent_decisions
        assert interactive_merger.persistent_decisions['hash123']['choice'] == '2'
    
    def test_decision_cache_saving(self, interactive_merger):
        """Test saving decisions to cache file."""
        # Add a decision
        interactive_merger.persistent_decisions['test_hash'] = {
            'choice': '3',
            'choice_description': 'Merge intelligently',
            'timestamp': datetime.now().isoformat()
        }
        
        # Save to file
        interactive_merger._save_persistent_decisions()
        
        # Verify file was created and contains correct data
        assert interactive_merger.decision_cache_file.exists()
        
        with open(interactive_merger.decision_cache_file, 'r') as f:
            saved_data = json.load(f)
        
        assert 'test_hash' in saved_data
        assert saved_data['test_hash']['choice'] == '3'
    
    def test_create_conflict_signature(self, interactive_merger):
        """Test conflict signature creation for caching."""
        existing_game = {
            'title': 'Test Game',
            'personal_rating': 4.0,
            'play_status': 'completed',
            'is_loved': True
        }
        
        csv_game = {
            'personal_rating': 5.0,
            'play_status': 'mastered',
            'is_loved': False
        }
        
        signature1 = interactive_merger._create_conflict_signature(existing_game, csv_game)
        
        # Same conflict should produce same signature
        signature2 = interactive_merger._create_conflict_signature(existing_game, csv_game)
        assert signature1 == signature2
        
        # Different conflict should produce different signature
        csv_game_different = csv_game.copy()
        csv_game_different['personal_rating'] = 3.0
        signature3 = interactive_merger._create_conflict_signature(existing_game, csv_game_different)
        assert signature1 != signature3
    
    def test_apply_cached_decision(self, interactive_merger):
        """Test applying a cached decision."""
        existing_game = {'title': 'Test Game', 'personal_rating': 4.0}
        csv_game = {'personal_rating': 5.0}
        
        # Test each cached decision type
        cached_decisions = [
            ({'choice': '1'}, {'action': 'skip'}),
            ({'choice': '2'}, {'action': 'update', 'data': csv_game}),
            ({'choice': '4'}, {'action': 'skip'}),
            ({'choice': '5', 'batch_choice': '2'}, {'action': 'update', 'data': csv_game})
        ]
        
        for cached_decision, expected_result in cached_decisions:
            result = interactive_merger._apply_cached_decision(existing_game, csv_game, cached_decision)
            
            if 'data' in expected_result:
                assert result['action'] == expected_result['action']
                assert result['data'] == expected_result['data']
            else:
                assert result == expected_result


class TestOverwriteMerger:
    """Test cases for OverwriteMerger."""
    
    @pytest.fixture
    def mock_api_client(self):
        """Create a mock API client."""
        return AsyncMock(spec=NexoriousAPIClient)
    
    @pytest.fixture
    def overwrite_merger(self, mock_api_client):
        """Create an OverwriteMerger with mocked dependencies."""
        return OverwriteMerger(mock_api_client, dry_run=False)
    
    @pytest.mark.asyncio
    async def test_process_new_game(self, overwrite_merger, mock_api_client):
        """Test processing a completely new game."""
        games = [{'Name': 'New Game', 'Rating': '4', 'Status': 'Completed'}]
        
        # Mock mapper
        overwrite_merger.mapper = MagicMock()
        overwrite_merger.mapper.convert_to_nexorious_game.return_value = {
            'title': 'New Game',
            'personal_rating': 4.0,
            'play_status': 'completed',
            'platforms': [{'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'}]
        }
        
        # Mock no existing game found
        mock_api_client.search_games.return_value = []
        mock_api_client.create_user_game.return_value = {'id': 'new_game_123'}
        
        results = await overwrite_merger.process_games(games, 'user123')
        
        assert results['new_games'] == 1
        assert results['updated_games'] == 0
        assert results['errors'] == 0
        
        # Verify API calls
        mock_api_client.create_user_game.assert_called_once()
        mock_api_client.update_user_game.assert_not_called()
    
    def test_handle_conflict_always_overwrites(self, overwrite_merger):
        """Test that overwrite merger always chooses CSV data."""
        existing_game = {'title': 'Test Game', 'personal_rating': 3.0}
        csv_game = {'personal_rating': 5.0}
        
        result = asyncio.run(overwrite_merger.handle_conflict(existing_game, csv_game))
        
        assert result['action'] == 'update'
        assert result['data'] == csv_game


class TestPreserveMerger:
    """Test cases for PreserveMerger."""
    
    @pytest.fixture
    def mock_api_client(self):
        """Create a mock API client."""
        return AsyncMock(spec=NexoriousAPIClient)
    
    @pytest.fixture
    def preserve_merger(self, mock_api_client):
        """Create a PreserveMerger with mocked dependencies."""
        return PreserveMerger(mock_api_client, dry_run=False)
    
    @pytest.mark.asyncio
    async def test_process_existing_game_with_new_platforms(self, preserve_merger, mock_api_client):
        """Test processing existing game with new platforms (should add platforms only)."""
        games = [{'Name': 'Existing Game', 'Platform': 'Xbox One'}]
        
        # Mock mapper
        preserve_merger.mapper = MagicMock()
        preserve_merger.mapper.convert_to_nexorious_game.return_value = {
            'title': 'Existing Game',
            'platforms': [{'platform_name': 'Xbox One', 'storefront_name': 'Microsoft Store'}]
        }
        
        # Mock existing game found
        existing_game = {
            'id': 'existing_123',
            'title': 'Existing Game',
            'personal_rating': 4.0,
            'platforms': [{'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'}]
        }
        mock_api_client.search_games.return_value = [existing_game]
        mock_api_client.add_platform_to_user_game.return_value = True
        
        results = await preserve_merger.process_games(games, 'user123')
        
        assert results['updated_games'] == 1  # Game was updated with new platform
        assert results['new_games'] == 0
        assert results['errors'] == 0
        
        # Should add the new platform
        mock_api_client.add_platform_to_user_game.assert_called_once()
        # Should not update game data
        mock_api_client.update_user_game.assert_not_called()
    
    @pytest.mark.asyncio
    async def test_process_existing_game_no_new_platforms(self, preserve_merger, mock_api_client):
        """Test processing existing game with no new platforms (should skip)."""
        games = [{'Name': 'Existing Game', 'Platform': 'PC'}]
        
        # Mock mapper
        preserve_merger.mapper = MagicMock()
        preserve_merger.mapper.convert_to_nexorious_game.return_value = {
            'title': 'Existing Game',
            'platforms': [{'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'}]
        }
        
        # Mock existing game with same platform
        existing_game = {
            'id': 'existing_123',
            'title': 'Existing Game',
            'platforms': [{'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'}]
        }
        mock_api_client.search_games.return_value = [existing_game]
        
        results = await preserve_merger.process_games(games, 'user123')
        
        assert results['skipped_games'] == 1  # Game was skipped (no new platforms)
        assert results['updated_games'] == 0
        assert results['new_games'] == 0
        
        # Should not add platforms or update game
        mock_api_client.add_platform_to_user_game.assert_not_called()
        mock_api_client.update_user_game.assert_not_called()
    
    def test_handle_conflict_always_preserves(self, preserve_merger):
        """Test that preserve merger always skips conflicts."""
        existing_game = {'title': 'Test Game', 'personal_rating': 3.0}
        csv_game = {'personal_rating': 5.0}
        
        result = asyncio.run(preserve_merger.handle_conflict(existing_game, csv_game))
        
        assert result['action'] == 'skip'


class TestIdempotency:
    """Test cases for idempotent behavior across all merge strategies."""
    
    @pytest.fixture
    def sample_games(self):
        """Sample games data for testing."""
        return [
            {
                'Name': 'Test Game 1',
                'Rating': '4',
                'Status': 'Completed',
                'Platform': 'PC'
            },
            {
                'Name': 'Test Game 2', 
                'Rating': '5',
                'Status': 'Mastered',
                'Platform': 'PlayStation 4'
            }
        ]
    
    @pytest.mark.asyncio
    async def test_overwrite_merger_idempotency(self, sample_games):
        """Test that OverwriteMerger produces identical results on re-run."""
        mock_api_client = AsyncMock(spec=NexoriousAPIClient)
        merger = OverwriteMerger(mock_api_client, dry_run=True)  # Use dry_run for testing
        
        # Mock mapper and API responses
        merger.mapper = MagicMock()
        merger.mapper.convert_to_nexorious_game.side_effect = [
            {'title': 'Test Game 1', 'personal_rating': 4.0, 'platforms': []},
            {'title': 'Test Game 2', 'personal_rating': 5.0, 'platforms': []}
        ]
        
        # No existing games found
        mock_api_client.search_games.return_value = []
        
        # Run import twice
        results1 = await merger.process_games(sample_games, 'user123')
        
        # Reset for second run
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': [],
            'structured_errors': []
        }
        merger.mapper.convert_to_nexorious_game.side_effect = [
            {'title': 'Test Game 1', 'personal_rating': 4.0, 'platforms': []},
            {'title': 'Test Game 2', 'personal_rating': 5.0, 'platforms': []}
        ]
        
        results2 = await merger.process_games(sample_games, 'user123')
        
        # Results should be identical
        assert results1 == results2
    
    @pytest.mark.asyncio 
    async def test_preserve_merger_idempotency(self, sample_games):
        """Test that PreserveMerger doesn't add duplicate platforms on re-run."""
        mock_api_client = AsyncMock(spec=NexoriousAPIClient)
        merger = PreserveMerger(mock_api_client, dry_run=False)
        
        # Mock mapper
        merger.mapper = MagicMock()
        merger.mapper.convert_to_nexorious_game.return_value = {
            'title': 'Test Game 1',
            'platforms': [{'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'}]
        }
        
        # Mock existing game
        existing_game = {
            'id': 'game123',
            'title': 'Test Game 1',
            'platforms': []  # No platforms initially
        }
        
        # First run - should add platform
        mock_api_client.search_games.return_value = [existing_game]
        mock_api_client.add_platform_to_user_game.return_value = True
        
        results1 = await merger.process_games([sample_games[0]], 'user123')
        assert results1['updated_games'] == 1
        assert mock_api_client.add_platform_to_user_game.call_count == 1
        
        # Reset for second run - existing game now has the platform
        existing_game['platforms'] = [{'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'}]
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': []
        }
        mock_api_client.add_platform_to_user_game.reset_mock()
        
        results2 = await merger.process_games([sample_games[0]], 'user123')
        
        # Second run should skip (no new platforms to add)
        assert results2['skipped_games'] == 1
        assert results2['updated_games'] == 0
        assert mock_api_client.add_platform_to_user_game.call_count == 0
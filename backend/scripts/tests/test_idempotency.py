"""
Idempotency validation tests for the Darkadia CSV import system.

These tests verify that running the same import multiple times produces
identical results without creating duplicates or unintended side effects.
"""

import pytest
import asyncio
from unittest.mock import AsyncMock, MagicMock, patch
from pathlib import Path
import tempfile
import json
from datetime import datetime

from scripts.darkadia.merge_strategies import InteractiveMerger, OverwriteMerger, PreserveMerger
from scripts.darkadia.parser import DarkadiaCSVParser
from scripts.darkadia.api_client import NexoriousAPIClient
from rich.console import Console


class TestImportIdempotency:
    """End-to-end idempotency tests for the complete import process."""
    
    @pytest.fixture
    def sample_csv_data(self):
        """Sample CSV data representing a typical Darkadia export."""
        return [
            {
                'Name': 'The Witcher 3: Wild Hunt',
                'Rating': '5',
                'Status': 'Completed',
                'Platform': 'PC',
                'Storefront': 'Steam',
                'Date Acquired': '2023-01-15',
                'Hours Played': '120',
                'Notes': 'Amazing RPG, loved the story',
                'Loved': 'Yes'
            },
            {
                'Name': 'God of War',
                'Rating': '4',
                'Status': 'In Progress', 
                'Platform': 'PlayStation 4',
                'Storefront': 'PlayStation Store',
                'Date Acquired': '2023-02-01',
                'Hours Played': '25',
                'Notes': 'Great combat system',
                'Loved': 'No'
            },
            {
                'Name': 'Hades',
                'Rating': '5',
                'Status': 'Mastered',
                'Platform': 'PC',
                'Storefront': 'Epic Games Store',
                'Date Acquired': '2023-03-10',
                'Hours Played': '80',
                'Notes': 'Perfect roguelike',
                'Loved': 'Yes'
            }
        ]
    
    @pytest.fixture
    def mock_api_client(self):
        """Create a comprehensive mock API client."""
        client = AsyncMock(spec=NexoriousAPIClient)
        
        # Mock platform/storefront data
        client.get_platforms.return_value = [
            {'name': 'PC (Windows)', 'display_name': 'PC (Windows)'},
            {'name': 'PlayStation 4', 'display_name': 'PlayStation 4'}
        ]
        client.get_storefronts.return_value = [
            {'name': 'Steam', 'display_name': 'Steam'},
            {'name': 'PlayStation Store', 'display_name': 'PlayStation Store'},
            {'name': 'Epic Games Store', 'display_name': 'Epic Games Store'}
        ]
        
        return client
    
    @pytest.fixture
    def mock_parser(self, sample_csv_data):
        """Create a mock parser that returns consistent data."""
        parser = MagicMock(spec=DarkadiaCSVParser)
        parser.parse_csv.return_value = sample_csv_data
        parser.group_duplicates.return_value = sample_csv_data  # No duplicates in sample
        return parser
    
    @pytest.mark.asyncio
    async def test_overwrite_strategy_complete_idempotency(self, sample_csv_data, mock_api_client, mock_parser):
        """Test complete idempotency of overwrite strategy across multiple runs."""
        
        # Setup merger
        merger = OverwriteMerger(mock_api_client, dry_run=False)
        
        # Mock the mapper to return consistent Nexorious format
        merger.mapper = MagicMock()
        def mock_convert(darkadia_game):
            return {
                'title': darkadia_game['Name'],
                'personal_rating': int(darkadia_game['Rating']),
                'play_status': darkadia_game['Status'].lower().replace(' ', '_'),
                'is_loved': darkadia_game['Loved'].lower() == 'yes',
                'personal_notes': darkadia_game['Notes'],
                'hours_played': int(darkadia_game['Hours Played']),
                'platforms': [{
                    'platform_name': darkadia_game['Platform'],
                    'storefront_name': darkadia_game['Storefront'],
                    'is_available': True
                }]
            }
        merger.mapper.convert_to_nexorious_game.side_effect = mock_convert
        
        # First run - no existing games
        mock_api_client.search_games.return_value = []
        created_games = []
        
        def mock_create_game(user_id, game_data):
            game_id = f"game_{len(created_games) + 1}"
            created_game = {'id': game_id, **game_data}
            created_games.append(created_game)
            return created_game
        
        mock_api_client.create_user_game.side_effect = mock_create_game
        
        # Run first import
        results1 = await merger.process_games(sample_csv_data, 'user123')
        
        assert results1['new_games'] == 3
        assert results1['updated_games'] == 0
        assert results1['errors'] == 0
        assert len(created_games) == 3
        
        # Second run - games now exist
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': []
        }
        
        # Mock search to return the created games
        def mock_search_games(query, fuzzy_threshold=0.8):
            # Find matching game by title
            for game in created_games:
                if game['title'].lower() in query.lower():
                    return [game]
            return []
        
        mock_api_client.search_games.side_effect = mock_search_games
        mock_api_client.update_user_game.return_value = {'status': 'success'}
        mock_api_client.add_platform_to_user_game.return_value = True
        
        # Run second import
        results2 = await merger.process_games(sample_csv_data, 'user123')
        
        # Should update existing games, not create new ones
        assert results2['new_games'] == 0
        assert results2['updated_games'] == 3
        assert results2['errors'] == 0
        
        # Verify identical processing
        assert results1['total_processed'] == results2['total_processed']
        
        # Third run should produce identical results to second run
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': []
        }
        
        results3 = await merger.process_games(sample_csv_data, 'user123')
        
        assert results3 == results2  # Should be completely identical
    
    @pytest.mark.asyncio
    async def test_preserve_strategy_platform_idempotency(self, sample_csv_data, mock_api_client):
        """Test that preserve strategy doesn't add duplicate platforms across runs."""
        
        merger = PreserveMerger(mock_api_client, dry_run=False)
        
        # Mock the mapper
        merger.mapper = MagicMock()
        merger.mapper.convert_to_nexorious_game.return_value = {
            'title': 'Test Game',
            'platforms': [
                {'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'},
                {'platform_name': 'PC (Windows)', 'storefront_name': 'Epic Games Store'}
            ]
        }
        
        # Mock existing game with one platform
        existing_game = {
            'id': 'game123',
            'title': 'Test Game',
            'platforms': [
                {'platform_name': 'PC (Windows)', 'storefront_name': 'Steam'}
            ]
        }
        
        mock_api_client.search_games.return_value = [existing_game]
        platform_add_calls = []
        
        def mock_add_platform(game_id, platform_data):
            platform_add_calls.append((game_id, platform_data))
            # Simulate adding to existing game
            existing_game['platforms'].append(platform_data)
            return True
        
        mock_api_client.add_platform_to_user_game.side_effect = mock_add_platform
        
        # First run - should add Epic Games Store platform
        results1 = await merger.process_games([sample_csv_data[0]], 'user123')
        
        assert results1['updated_games'] == 1
        assert len(platform_add_calls) == 1
        assert platform_add_calls[0][1]['storefront_name'] == 'Epic Games Store'
        
        # Second run - existing game now has both platforms
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': []
        }
        platform_add_calls.clear()
        
        results2 = await merger.process_games([sample_csv_data[0]], 'user123')
        
        # Should skip - no new platforms to add
        assert results2['skipped_games'] == 1
        assert results2['updated_games'] == 0
        assert len(platform_add_calls) == 0  # No API calls to add platforms
        
        # Third run should be identical to second
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': []
        }
        
        results3 = await merger.process_games([sample_csv_data[0]], 'user123')
        assert results3 == results2
    
    @pytest.mark.asyncio
    async def test_interactive_strategy_decision_persistence(self, sample_csv_data, mock_api_client):
        """Test that interactive strategy persists and reuses decisions across runs."""
        
        with tempfile.TemporaryDirectory() as temp_dir:
            console = MagicMock(spec=Console)
            merger = InteractiveMerger(console, mock_api_client, dry_run=False)
            
            # Override cache file location
            merger.decision_cache_file = Path(temp_dir) / 'decisions.json'
            merger.persistent_decisions = {}
            
            # Mock the mapper
            merger.mapper = MagicMock()
            merger.mapper.convert_to_nexorious_game.return_value = {
                'title': 'Test Game',
                'personal_rating': 5.0,
                'play_status': 'completed'
            }
            
            # Mock existing game with different data (conflict)
            existing_game = {
                'id': 'game123',
                'title': 'Test Game',
                'personal_rating': 3.0,
                'play_status': 'in_progress',
                'platforms': []
            }
            
            mock_api_client.search_games.return_value = [existing_game]
            
            # Create a cached decision
            conflict_signature = merger._create_conflict_signature(
                existing_game, 
                merger.mapper.convert_to_nexorious_game.return_value
            )
            
            cached_decision = {
                'choice': '2',  # Use CSV data
                'choice_description': 'Use CSV data',
                'timestamp': datetime.now().isoformat()
            }
            
            merger.persistent_decisions[conflict_signature] = cached_decision
            merger._save_persistent_decisions()
            
            # Mock API responses
            mock_api_client.update_user_game.return_value = {'status': 'success'}
            mock_api_client.add_platform_to_user_game.return_value = True
            
            # First run - should use cached decision without prompting
            results1 = await merger.process_games([sample_csv_data[0]], 'user123')
            
            assert results1['updated_games'] == 1
            assert results1['errors'] == 0
            
            # Verify decision was applied (update was called)
            mock_api_client.update_user_game.assert_called_once()
            
            # Second run - should use same cached decision
            merger.results = {
                'total_processed': 0, 'new_games': 0, 'updated_games': 0,
                'skipped_games': 0, 'errors': 0, 'error_details': []
            }
            mock_api_client.update_user_game.reset_mock()
            
            results2 = await merger.process_games([sample_csv_data[0]], 'user123')
            
            # Should produce identical results
            assert results2['updated_games'] == 1
            assert results2['errors'] == 0
            
            # Verify cached decision was reused
            mock_api_client.update_user_game.assert_called_once()
    
    @pytest.mark.asyncio
    async def test_interrupted_import_recovery(self, sample_csv_data, mock_api_client):
        """Test that import can recover gracefully from interruptions."""
        
        merger = OverwriteMerger(mock_api_client, dry_run=False)
        
        # Mock the mapper
        merger.mapper = MagicMock()
        merger.mapper.convert_to_nexorious_game.return_value = {
            'title': 'Test Game',
            'personal_rating': 4.0,
            'platforms': []
        }
        
        # Simulate partial import - first game created, second fails, third not processed
        created_games = []
        api_call_count = 0
        
        def mock_create_game(user_id, game_data):
            nonlocal api_call_count
            api_call_count += 1
            
            if api_call_count == 1:
                # First game succeeds
                game = {'id': f'game_{api_call_count}', **game_data}
                created_games.append(game)
                return game
            elif api_call_count == 2:
                # Second game fails
                raise Exception("Network error")
            else:
                # Third game succeeds
                game = {'id': f'game_{api_call_count}', **game_data}
                created_games.append(game)
                return game
        
        mock_api_client.create_user_game.side_effect = mock_create_game
        mock_api_client.search_games.return_value = []  # No existing games initially
        
        # First run - partial failure
        results1 = await merger.process_games(sample_csv_data, 'user123')
        
        assert results1['new_games'] == 2  # Two succeeded
        assert results1['errors'] == 1    # One failed
        assert len(created_games) == 2
        
        # Second run - resume with existing games
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': []
        }
        
        # Mock search to find existing games
        def mock_search_existing(query, fuzzy_threshold=0.8):
            # Return existing games for first two queries
            if api_call_count <= len(created_games) + 1:  # Adjust for failed call
                return [created_games[0]] if created_games else []
            return []
        
        # Reset and configure mocks for second run
        api_call_count = 0
        mock_api_client.search_games.side_effect = mock_search_existing
        mock_api_client.update_user_game.return_value = {'status': 'success'}
        mock_api_client.create_user_game.side_effect = mock_create_game
        
        results2 = await merger.process_games(sample_csv_data, 'user123')
        
        # Should handle existing games properly and succeed with remaining
        assert results2['errors'] <= results1['errors']  # No worse than first run
        assert results2['total_processed'] == len(sample_csv_data)


class TestScalabilityIdempotency:
    """Test idempotency with larger datasets and edge cases."""
    
    @pytest.mark.asyncio
    async def test_large_dataset_idempotency(self, mock_api_client):
        """Test idempotency with a large number of games."""
        
        # Generate large dataset
        large_dataset = []
        for i in range(100):
            large_dataset.append({
                'Name': f'Game {i:03d}',
                'Rating': str((i % 5) + 1),
                'Status': ['Not Started', 'In Progress', 'Completed'][i % 3],
                'Platform': ['PC', 'PlayStation 4', 'Xbox One'][i % 3],
                'Storefront': ['Steam', 'PlayStation Store', 'Microsoft Store'][i % 3]
            })
        
        merger = OverwriteMerger(mock_api_client, dry_run=True)  # Use dry_run for speed
        
        # Mock mapper
        merger.mapper = MagicMock()
        def mock_convert(game):
            return {
                'title': game['Name'],
                'personal_rating': int(game['Rating']),
                'platforms': []
            }
        merger.mapper.convert_to_nexorious_game.side_effect = mock_convert
        
        # Mock no existing games
        mock_api_client.search_games.return_value = []
        
        # First run
        results1 = await merger.process_games(large_dataset, 'user123')
        
        # Second run
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': []
        }
        
        results2 = await merger.process_games(large_dataset, 'user123')
        
        # Results should be identical
        assert results1 == results2
        assert results1['total_processed'] == 100
        assert results1['new_games'] == 100
    
    @pytest.mark.asyncio
    async def test_duplicate_titles_idempotency(self, mock_api_client):
        """Test idempotency when CSV contains duplicate game titles."""
        
        duplicate_dataset = [
            {'Name': 'Duplicate Game', 'Rating': '4', 'Platform': 'PC', 'Storefront': 'Steam'},
            {'Name': 'Duplicate Game', 'Rating': '5', 'Platform': 'PlayStation 4', 'Storefront': 'PlayStation Store'},
            {'Name': 'Duplicate Game', 'Rating': '3', 'Platform': 'Xbox One', 'Storefront': 'Microsoft Store'}
        ]
        
        merger = PreserveMerger(mock_api_client, dry_run=False)
        
        # Mock mapper
        merger.mapper = MagicMock()
        def mock_convert(game):
            return {
                'title': game['Name'],
                'personal_rating': int(game['Rating']),
                'platforms': [{
                    'platform_name': game['Platform'],
                    'storefront_name': game['Storefront']
                }]
            }
        merger.mapper.convert_to_nexorious_game.side_effect = mock_convert
        
        # Mock API responses
        created_game = None
        
        def mock_search(query, fuzzy_threshold=0.8):
            if created_game and 'duplicate game' in query.lower():
                return [created_game]
            return []
        
        def mock_create(user_id, game_data):
            nonlocal created_game
            created_game = {'id': 'game123', 'platforms': [], **game_data}
            return created_game
        
        def mock_add_platform(game_id, platform_data):
            if created_game:
                created_game['platforms'].append(platform_data)
            return True
        
        mock_api_client.search_games.side_effect = mock_search
        mock_api_client.create_user_game.side_effect = mock_create
        mock_api_client.add_platform_to_user_game.side_effect = mock_add_platform
        
        # First run
        results1 = await merger.process_games(duplicate_dataset, 'user123')
        
        # Should create one game and add platforms for duplicates
        assert results1['new_games'] == 1
        assert results1['updated_games'] == 2  # Two additional platform additions
        assert len(created_game['platforms']) == 3
        
        # Second run
        merger.results = {
            'total_processed': 0, 'new_games': 0, 'updated_games': 0,
            'skipped_games': 0, 'errors': 0, 'error_details': []
        }
        mock_api_client.create_user_game.reset_mock()
        mock_api_client.add_platform_to_user_game.reset_mock()
        
        results2 = await merger.process_games(duplicate_dataset, 'user123')
        
        # Should skip all - no new platforms to add
        assert results2['new_games'] == 0
        assert results2['updated_games'] == 0
        assert results2['skipped_games'] == 3
        
        # No API calls should be made for creating or adding platforms
        mock_api_client.create_user_game.assert_not_called()
        mock_api_client.add_platform_to_user_game.assert_not_called()
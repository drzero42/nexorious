"""
Tests for the Copy Consolidation System.

This module tests the consolidation of multiple CSV rows representing
different copies of the same game.
"""

import pytest

from app.services.import_sources.copy_consolidation import (
    CopyConsolidationProcessor, 
    CopyData, 
    ConsolidatedGame
)


class TestCopyConsolidationProcessor:
    """Test cases for CopyConsolidationProcessor."""
    
    @pytest.fixture
    def processor(self):
        """Create a processor instance for testing."""
        return CopyConsolidationProcessor()
    
    @pytest.fixture
    def sample_single_game_data(self):
        """Sample CSV data for a single game with one copy."""
        return [
            {
                'Name': 'Test Game',
                'Added': '2023-01-15',
                'Loved': True,
                'Owned': True,
                'Played': True,
                'Playing': False,
                'Finished': True,
                'Mastered': False,
                'Dominated': False,
                'Shelved': False,
                'Rating': 4.5,
                'Copy platform': 'PC',
                'Copy source': 'Steam',
                'Copy media': 'Digital',
                'Copy label': 'Steam Edition',
                'Notes': 'Great game!',
                '_csv_row_number': 1
            }
        ]
    
    @pytest.fixture
    def sample_multi_copy_data(self):
        """Sample CSV data for a game with multiple copies."""
        return [
            {
                'Name': 'Multi-Copy Game',
                'Added': '2023-01-15',
                'Loved': True,
                'Owned': True,
                'Played': True,
                'Playing': False,
                'Finished': True,
                'Mastered': False,
                'Dominated': False,
                'Shelved': False,
                'Rating': 4.0,
                'Copy platform': 'PC',
                'Copy source': 'Steam',
                'Copy media': 'Digital',
                'Notes': 'Steam version',
                '_csv_row_number': 1
            },
            {
                'Name': 'Multi-Copy Game',
                'Added': '2023-02-20',
                'Loved': True,
                'Owned': True,
                'Played': False,
                'Playing': True,
                'Finished': False,
                'Mastered': False,
                'Dominated': False,
                'Shelved': False,
                'Rating': 4.5,
                'Copy platform': 'PlayStation 4',
                'Copy source': 'PSN',
                'Copy media': 'Digital',
                'Notes': 'PS4 version',
                '_csv_row_number': 2
            }
        ]
    
    @pytest.fixture
    def sample_fallback_platform_data(self):
        """Sample CSV data for a game with only fallback platform data."""
        return [
            {
                'Name': 'Fallback Game',
                'Added': '2023-01-15',
                'Loved': False,
                'Owned': True,
                'Played': True,
                'Playing': False,
                'Finished': False,
                'Mastered': False,
                'Dominated': False,
                'Shelved': True,
                'Rating': 3.0,
                'Copy platform': '',  # No copy data
                'Copy source': '',
                'Copy media': '',
                'Platforms': 'PC, Nintendo Switch',  # Fallback platforms
                'Notes': 'Owned on multiple platforms',
                '_csv_row_number': 1
            }
        ]
    
    @pytest.mark.asyncio
    async def test_consolidate_single_game(self, processor, sample_single_game_data):
        """Test consolidating a single game with one copy."""
        consolidated_games = await processor.consolidate_games(sample_single_game_data)
        
        assert len(consolidated_games) == 1
        game = consolidated_games[0]
        
        assert game.name == 'Test Game'
        assert len(game.copies) == 1
        assert game.copies[0].platform == 'PC'
        assert game.copies[0].storefront == 'Steam'
        assert game.copies[0].is_real_copy
        assert game.base_data['Rating'] == 4.5
        assert game.base_data['Finished']
    
    @pytest.mark.asyncio
    async def test_consolidate_multi_copy_game(self, processor, sample_multi_copy_data):
        """Test consolidating a game with multiple copies."""
        consolidated_games = await processor.consolidate_games(sample_multi_copy_data)
        
        assert len(consolidated_games) == 1
        game = consolidated_games[0]
        
        assert game.name == 'Multi-Copy Game'
        assert len(game.copies) == 2
        
        # Check copies
        pc_copy = next((c for c in game.copies if c.platform == 'PC'), None)
        ps4_copy = next((c for c in game.copies if c.platform == 'PlayStation 4'), None)
        
        assert pc_copy is not None
        assert pc_copy.storefront == 'Steam'
        assert pc_copy.csv_row_number == 1
        
        assert ps4_copy is not None
        assert ps4_copy.storefront == 'PSN'
        assert ps4_copy.csv_row_number == 2
        
        # Check merged base data (should use highest rating, OR logic for booleans)
        assert game.base_data['Rating'] == 4.5  # max of 4.0 and 4.5
        assert game.base_data['Played']  # True OR False
        assert game.base_data['Playing']  # False OR True
        
        # Check that both row numbers are tracked
        assert 1 in game.csv_row_numbers
        assert 2 in game.csv_row_numbers
    
    @pytest.mark.asyncio
    async def test_consolidate_fallback_platforms(self, processor, sample_fallback_platform_data):
        """Test consolidating games with fallback platform data."""
        consolidated_games = await processor.consolidate_games(sample_fallback_platform_data)
        
        assert len(consolidated_games) == 1
        game = consolidated_games[0]
        
        assert game.name == 'Fallback Game'
        assert len(game.copies) == 2  # PC and Nintendo Switch
        
        # Check that all copies are fallback copies
        for copy in game.copies:
            assert not copy.is_real_copy
            assert copy.requires_storefront_resolution
            assert copy.media == 'Digital'  # Default for fallback
        
        # Check platform names
        platform_names = [c.platform for c in game.copies]
        assert 'PC' in platform_names
        assert 'Nintendo Switch' in platform_names
    
    @pytest.mark.asyncio
    async def test_empty_input(self, processor):
        """Test handling of empty input."""
        consolidated_games = await processor.consolidate_games([])
        assert len(consolidated_games) == 0
    
    @pytest.mark.asyncio
    async def test_games_with_empty_names(self, processor):
        """Test handling of games with empty names."""
        data_with_empty_names = [
            {'Name': '', 'Copy platform': 'PC', '_csv_row_number': 1},
            {'Name': 'Valid Game', 'Copy platform': 'PC', '_csv_row_number': 2}
        ]
        
        consolidated_games = await processor.consolidate_games(data_with_empty_names)
        
        # Should only process the game with a valid name
        assert len(consolidated_games) == 1
        assert consolidated_games[0].name == 'Valid Game'
    
    def test_copy_identifier_generation(self, processor):
        """Test copy identifier generation."""
        # Test with platform and storefront
        identifier = processor._generate_copy_identifier('PC', 'Steam', 'Digital')
        assert 'plt:PC' in identifier
        assert 'str:Steam' in identifier
        assert 'med:Digital' in identifier
        assert '#' in identifier  # Hash suffix
        
        # Test with platform only
        identifier = processor._generate_copy_identifier('PC', '', 'Physical')
        assert 'plt:PC' in identifier
        assert 'med:Physical' in identifier
        
        # Test with empty values
        identifier = processor._generate_copy_identifier('', '', '')
        assert identifier.startswith('unknown#')  # Should have hash suffix
        assert len(identifier) > 8  # unknown plus hash
    
    def test_merge_base_data_ratings(self, processor):
        """Test rating merging logic."""
        rows = [
            {'Name': 'Test', 'Rating': 3.0, '_csv_row_number': 1},
            {'Name': 'Test', 'Rating': 4.5, '_csv_row_number': 2},
            {'Name': 'Test', 'Rating': 2.0, '_csv_row_number': 3}
        ]
        
        merged = processor._merge_base_data(rows)
        assert merged['Rating'] == 4.5  # Should take highest rating
    
    def test_merge_base_data_dates(self, processor):
        """Test date merging logic."""
        rows = [
            {'Name': 'Test', 'Added': '2023-01-01', '_csv_row_number': 1},
            {'Name': 'Test', 'Added': '2023-03-15', '_csv_row_number': 2},
            {'Name': 'Test', 'Added': '2023-02-10', '_csv_row_number': 3}
        ]
        
        merged = processor._merge_base_data(rows)
        assert merged['Added'] == '2023-03-15'  # Should take most recent
    
    def test_merge_base_data_booleans(self, processor):
        """Test boolean field merging with OR logic."""
        rows = [
            {'Name': 'Test', 'Played': False, 'Finished': True, 'Loved': False, '_csv_row_number': 1},
            {'Name': 'Test', 'Played': True, 'Finished': False, 'Loved': False, '_csv_row_number': 2}
        ]
        
        merged = processor._merge_base_data(rows)
        assert merged['Played']   # False OR True
        assert merged['Finished'] # True OR False
        assert not merged['Loved']   # False OR False
    
    def test_merge_base_data_notes(self, processor):
        """Test notes concatenation."""
        rows = [
            {'Name': 'Test', 'Notes': 'First note', '_csv_row_number': 1},
            {'Name': 'Test', 'Notes': 'Second note', '_csv_row_number': 2},
            {'Name': 'Test', 'Notes': 'First note', '_csv_row_number': 3}  # Duplicate
        ]
        
        merged = processor._merge_base_data(rows)
        # Should concatenate unique notes
        assert 'First note' in merged['Notes']
        assert 'Second note' in merged['Notes']
        assert merged['Notes'].count('First note') == 1  # No duplicates
    
    def test_extract_copy_data_real_copy(self, processor):
        """Test extraction of real copy data."""
        row = {
            'Copy platform': 'PC',
            'Copy source': 'Steam',
            'Copy media': 'Digital',
            'Copy label': 'GOTY Edition',
            'Copy Release': '2023',
            'Copy box': 'Yes',
            '_csv_row_number': 1
        }
        
        copy_list = processor._extract_copy_data(row, 1)
        assert len(copy_list) == 1
        
        copy = copy_list[0]
        assert copy.platform == 'PC'
        assert copy.storefront == 'Steam'
        assert copy.media == 'Digital'
        assert copy.label == 'GOTY Edition'
        assert copy.is_real_copy
        assert not copy.requires_storefront_resolution
    
    def test_extract_copy_data_storefront_other(self, processor):
        """Test extraction when using 'Copy source other' field."""
        row = {
            'Copy platform': 'PC',
            'Copy source': 'Other',
            'Copy source other': 'Custom Storefront',
            'Copy media': 'Digital',
            '_csv_row_number': 1
        }
        
        copy_list = processor._extract_copy_data(row, 1)
        copy = copy_list[0]
        
        assert copy.storefront is None  # 'Other' should be None
        assert copy.storefront_other == 'Custom Storefront'
    
    def test_extract_copy_data_no_copy(self, processor):
        """Test extraction when no copy data exists."""
        row = {
            'Copy platform': '',
            'Copy source': '',
            'Copy source other': '',
            '_csv_row_number': 1
        }
        
        copy_list = processor._extract_copy_data(row, 1)
        assert len(copy_list) == 0
    
    def test_extract_copy_data_missing_storefront(self, processor):
        """Test copy that has platform but no storefront."""
        row = {
            'Copy platform': 'PC',
            'Copy source': '',
            'Copy source other': '',
            'Copy media': 'Physical',
            '_csv_row_number': 1
        }
        
        copy_list = processor._extract_copy_data(row, 1)
        copy = copy_list[0]
        
        assert copy.platform == 'PC'
        assert copy.storefront is None
        assert copy.requires_storefront_resolution
    
    def test_create_fallback_copies(self, processor):
        """Test creation of fallback copies."""
        row = {
            'Platforms': 'PC, PlayStation 4, Nintendo Switch',
            '_csv_row_number': 1
        }
        
        fallback_copies = processor._create_fallback_copies(row)
        assert len(fallback_copies) == 3
        
        platforms = [c.platform for c in fallback_copies]
        assert 'PC' in platforms
        assert 'PlayStation 4' in platforms
        assert 'Nintendo Switch' in platforms
        
        # All should be fallback copies
        for copy in fallback_copies:
            assert not copy.is_real_copy
            assert copy.requires_storefront_resolution
            assert copy.media == 'Digital'
    
    def test_create_fallback_copies_empty_platforms(self, processor):
        """Test fallback creation with empty platforms field."""
        row = {'Platforms': '', '_csv_row_number': 1}
        
        fallback_copies = processor._create_fallback_copies(row)
        assert len(fallback_copies) == 0
    
    @pytest.mark.asyncio
    async def test_consolidation_stats(self, processor, sample_multi_copy_data):
        """Test consolidation statistics tracking."""
        # Add single game data to test consolidation stats
        mixed_data = sample_multi_copy_data + [
            {'Name': 'Single Game', 'Copy platform': 'PC', '_csv_row_number': 3}
        ]
        
        await processor.consolidate_games(mixed_data)
        stats = processor.get_consolidation_stats()
        
        assert stats['processed_games'] == 2  # Two unique games
        assert stats['total_copies'] == 3     # 2 copies + 1 copy
        assert stats['consolidated_games'] == 1  # One game was consolidated
        assert stats['average_copies_per_game'] == 1.5
    
    def test_has_real_copies_method(self):
        """Test the has_real_copies method of ConsolidatedGame."""
        # Game with real copies
        real_copy = CopyData(
            platform='PC', storefront='Steam', storefront_other=None, 
            media='Digital', copy_identifier='test', csv_row_number=1, is_real_copy=True
        )
        game_with_real = ConsolidatedGame('Test', {}, [real_copy], [1])
        assert game_with_real.has_real_copies()
        
        # Game with only fallback copies
        fallback_copy = CopyData(
            platform='PC', storefront=None, storefront_other=None,
            media='Digital', copy_identifier='fallback:PC', csv_row_number=1, is_real_copy=False
        )
        game_with_fallback = ConsolidatedGame('Test', {}, [fallback_copy], [1])
        assert not game_with_fallback.has_real_copies()
    
    def test_get_copy_count_method(self):
        """Test the get_copy_count method of ConsolidatedGame."""
        copies = [
            CopyData('PC', 'Steam', None, 'Digital', 'test1', 1, is_real_copy=True),
            CopyData('PS4', 'PSN', None, 'Digital', 'test2', 2, is_real_copy=True)
        ]
        game = ConsolidatedGame('Test', {}, copies, [1, 2])
        assert game.get_copy_count() == 2
        
        # Empty copies
        empty_game = ConsolidatedGame('Empty', {}, [], [])
        assert empty_game.get_copy_count() == 0


class TestCopyConsolidationIntegration:
    """Integration tests for copy consolidation with realistic data."""
    
    @pytest.mark.asyncio
    async def test_realistic_darkadia_export(self):
        """Test with realistic Darkadia export data."""
        processor = CopyConsolidationProcessor()
        
        # Simulate real Darkadia export with various scenarios
        realistic_data = [
            # Game with multiple digital copies
            {
                'Name': 'The Witcher 3: Wild Hunt',
                'Added': '2022-05-15',
                'Loved': True, 'Owned': True, 'Played': True, 'Playing': False,
                'Finished': True, 'Mastered': False, 'Dominated': False, 'Shelved': False,
                'Rating': 5.0,
                'Copy platform': 'PC', 'Copy source': 'Steam', 'Copy media': 'Digital',
                'Copy label': 'Game of the Year Edition',
                'Notes': 'Amazing RPG',
                '_csv_row_number': 1
            },
            {
                'Name': 'The Witcher 3: Wild Hunt',
                'Added': '2023-01-10',
                'Loved': True, 'Owned': True, 'Played': False, 'Playing': False,
                'Finished': False, 'Mastered': False, 'Dominated': False, 'Shelved': False,
                'Rating': 5.0,
                'Copy platform': 'PlayStation 4', 'Copy source': 'PlayStation Store', 'Copy media': 'Digital',
                'Copy label': 'Complete Edition',
                'Notes': 'Backup copy',
                '_csv_row_number': 2
            },
            # Game with physical copy
            {
                'Name': 'Zelda: Breath of the Wild',
                'Added': '2017-03-03',
                'Loved': True, 'Owned': True, 'Played': True, 'Playing': False,
                'Finished': True, 'Mastered': True, 'Dominated': False, 'Shelved': False,
                'Rating': 4.8,
                'Copy platform': 'Nintendo Switch', 'Copy source': 'Physical', 'Copy media': 'Physical',
                'Copy box': 'Perfect', 'Copy manual': 'Yes', 'Copy complete': 'Yes',
                'Notes': 'Launch day purchase',
                '_csv_row_number': 3
            },
            # Game with only generic platform info
            {
                'Name': 'Portal 2',
                'Added': '2011-04-19',
                'Loved': True, 'Owned': True, 'Played': True, 'Playing': False,
                'Finished': True, 'Mastered': False, 'Dominated': False, 'Shelved': False,
                'Rating': 4.9,
                'Copy platform': '', 'Copy source': '', 'Copy media': '',
                'Platforms': 'PC, Xbox 360',
                'Notes': 'Classic puzzle game',
                '_csv_row_number': 4
            }
        ]
        
        consolidated_games = await processor.consolidate_games(realistic_data)
        
        # Should have 3 unique games
        assert len(consolidated_games) == 3
        
        # Find each game
        witcher = next(g for g in consolidated_games if 'Witcher' in g.name)
        zelda = next(g for g in consolidated_games if 'Zelda' in g.name)
        portal = next(g for g in consolidated_games if 'Portal' in g.name)
        
        # Check Witcher (multi-copy)
        assert len(witcher.copies) == 2
        assert witcher.base_data['Rating'] == 5.0
        assert any(c.platform == 'PC' and c.storefront == 'Steam' for c in witcher.copies)
        assert any(c.platform == 'PlayStation 4' for c in witcher.copies)
        
        # Check Zelda (physical copy)
        assert len(zelda.copies) == 1
        assert zelda.copies[0].media == 'Physical'
        assert zelda.copies[0].box == 'Perfect'
        assert zelda.base_data['Mastered']
        
        # Check Portal (fallback platforms)
        assert len(portal.copies) == 2  # PC and Xbox 360
        assert all(not c.is_real_copy for c in portal.copies)
        assert all(c.requires_storefront_resolution for c in portal.copies)
        
        # Check stats
        stats = processor.get_consolidation_stats()
        assert stats['processed_games'] == 3
        assert stats['consolidated_games'] == 1  # Only Witcher was consolidated
        assert stats['total_copies'] == 5  # 2 + 1 + 2
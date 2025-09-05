"""
Tests for Darkadia Data Mapper module.
"""

import pytest
from datetime import datetime, date
import pandas as pd

from scripts.darkadia.mapper import DarkadiaDataMapper, PlayStatus, OwnershipStatus


class TestDarkadiaDataMapper:
    """Test cases for DarkadiaDataMapper."""
    
    @pytest.fixture
    def mapper(self):
        """Create a mapper instance for testing."""
        return DarkadiaDataMapper(verbose=False)
    
    @pytest.fixture
    def sample_darkadia_game(self):
        """Sample Darkadia game data for testing."""
        return {
            'Name': 'Test Game',
            'Added': '2023-01-15',
            'Loved': 1,
            'Owned': 1,
            'Played': 1,
            'Playing': 0,
            'Finished': 1,
            'Mastered': 0,
            'Dominated': 0,
            'Shelved': 0,
            'Rating': 4.5,
            'Notes': 'Great game!',
            '_platforms': [
                {
                    'platform': 'PC',
                    'storefront': 'Steam',
                    'media': 'Digital'
                }
            ]
        }
    
    def test_convert_to_nexorious_game(self, mapper, sample_darkadia_game):
        """Test conversion of Darkadia game to Nexorious format."""
        nexorious_game = mapper.convert_to_nexorious_game(sample_darkadia_game)
        
        assert nexorious_game['title'] == 'Test Game'
        assert nexorious_game['personal_rating'] == 4.5
        assert nexorious_game['is_loved'] is True
        assert nexorious_game['personal_notes'] == 'Great game!'
        assert nexorious_game['acquired_date'] == '2023-01-15'
        assert nexorious_game['ownership_status'] == OwnershipStatus.OWNED.value
        assert nexorious_game['play_status'] == PlayStatus.COMPLETED.value
        assert nexorious_game['hours_played'] == 0
        assert len(nexorious_game['platforms']) == 1
    
    def test_convert_rating_valid(self, mapper):
        """Test rating conversion with valid values."""
        assert mapper._convert_rating(4.5) == 4.5
        assert mapper._convert_rating(0) == 0.0
        assert mapper._convert_rating(5) == 5.0
        assert mapper._convert_rating('3.5') == 3.5
    
    def test_convert_rating_invalid(self, mapper):
        """Test rating conversion with invalid values."""
        assert mapper._convert_rating(6.0) is None  # Above range
        assert mapper._convert_rating(-1.0) is None  # Below range
        assert mapper._convert_rating('invalid') is None
        assert mapper._convert_rating(None) is None
        assert mapper._convert_rating(pd.NA) is None
    
    def test_convert_date_valid(self, mapper):
        """Test date conversion with valid values."""
        assert mapper._convert_date('2023-01-15') == '2023-01-15'
        assert mapper._convert_date(datetime(2023, 1, 15)) == '2023-01-15'
        assert mapper._convert_date(date(2023, 1, 15)) == '2023-01-15'
        
        # Test pandas Timestamp
        pd_timestamp = pd.Timestamp('2023-01-15')
        assert mapper._convert_date(pd_timestamp) == '2023-01-15'
    
    def test_convert_date_invalid(self, mapper):
        """Test date conversion with invalid values."""
        assert mapper._convert_date(None) is None
        assert mapper._convert_date(pd.NA) is None
        assert mapper._convert_date('invalid-date') is None
        assert mapper._convert_date('') is None
    
    def test_convert_ownership_status(self, mapper):
        """Test ownership status conversion."""
        owned_game = {'Owned': 1}
        not_owned_game = {'Owned': 0}
        
        assert mapper._convert_ownership_status(owned_game) == OwnershipStatus.OWNED.value
        assert mapper._convert_ownership_status(not_owned_game) == OwnershipStatus.NO_LONGER_OWNED.value
    
    def test_convert_play_status_priority(self, mapper):
        """Test play status conversion priority order."""
        # Test priority: Dominated > Mastered > Finished > Playing > Shelved > Played > Not Started
        
        dominated_game = {'Dominated': 1, 'Mastered': 1, 'Finished': 1}
        assert mapper._convert_play_status(dominated_game) == PlayStatus.DOMINATED.value
        
        mastered_game = {'Mastered': 1, 'Finished': 1, 'Playing': 1}
        assert mapper._convert_play_status(mastered_game) == PlayStatus.MASTERED.value
        
        finished_game = {'Finished': 1, 'Playing': 1, 'Played': 1}
        assert mapper._convert_play_status(finished_game) == PlayStatus.COMPLETED.value
        
        playing_game = {'Playing': 1, 'Shelved': 1, 'Played': 1}
        assert mapper._convert_play_status(playing_game) == PlayStatus.IN_PROGRESS.value
        
        shelved_game = {'Shelved': 1, 'Played': 1}
        assert mapper._convert_play_status(shelved_game) == PlayStatus.SHELVED.value
        
        played_game = {'Played': 1}
        assert mapper._convert_play_status(played_game) == PlayStatus.COMPLETED.value
        
        not_started_game = {}
        assert mapper._convert_play_status(not_started_game) == PlayStatus.NOT_STARTED.value
    
    def test_map_platform_name_direct(self, mapper):
        """Test direct platform name mapping."""
        assert mapper._map_platform_name('PC') == 'PC (Windows)'
        assert mapper._map_platform_name('PlayStation 4') == 'PlayStation 4'
        assert mapper._map_platform_name('Nintendo Switch') == 'Nintendo Switch'
        assert mapper._map_platform_name('Xbox 360') == 'Xbox 360'
        assert mapper._map_platform_name('Mac') == 'PC (Windows)'  # Maps to PC
    
    def test_map_platform_name_fuzzy(self, mapper):
        """Test fuzzy platform name mapping."""
        assert mapper._map_platform_name('playstation 4') == 'PlayStation 4'
        assert mapper._map_platform_name('PS4') == 'PlayStation 4'
        assert mapper._map_platform_name('xbox one') == 'Xbox One'
        assert mapper._map_platform_name('nintendo switch') == 'Nintendo Switch'
        assert mapper._map_platform_name('windows') == 'PC (Windows)'
    
    def test_map_platform_name_unknown(self, mapper):
        """Test unknown platform name handling."""
        result = mapper._map_platform_name('Unknown Platform')
        assert result is None
        assert 'Unknown Platform' in mapper.unknown_platforms
    
    def test_map_storefront_name_direct(self, mapper):
        """Test direct storefront name mapping."""
        assert mapper._map_storefront_name('Steam', 'PC (Windows)') == 'Steam'
        assert mapper._map_storefront_name('Epic Games Store', 'PC (Windows)') == 'Epic Games Store'
        assert mapper._map_storefront_name('PSN', 'PlayStation 4') == 'PlayStation Store'
        assert mapper._map_storefront_name('Nintendo eShop', 'Nintendo Switch') == 'Nintendo eShop'
    
    def test_map_storefront_name_fuzzy(self, mapper):
        """Test fuzzy storefront name mapping."""
        # Epic variants should all map to Epic Games Store
        assert mapper._map_storefront_name('epic', 'PC (Windows)') == 'Epic Games Store'
        assert mapper._map_storefront_name('Epic', 'PC (Windows)') == 'Epic Games Store'
        assert mapper._map_storefront_name('Epic Game Store', 'PC (Windows)') == 'Epic Games Store'
        assert mapper._map_storefront_name('Epic Games', 'PC (Windows)') == 'Epic Games Store'
        
        # Other fuzzy matches should still work
        assert mapper._map_storefront_name('playstation store', 'PlayStation 4') == 'PlayStation Store'
        assert mapper._map_storefront_name('humble bundle', 'PC (Windows)') == 'Humble Bundle'
        assert mapper._map_storefront_name('microsoft store', 'Xbox One') == 'Microsoft Store'
    
    def test_map_storefront_name_default(self, mapper):
        """Test default storefront assignment."""
        # Empty storefront should use platform default
        assert mapper._map_storefront_name('', 'PC (Windows)') == 'Steam'
        assert mapper._map_storefront_name('', 'PlayStation 4') == 'PlayStation Store'
        assert mapper._map_storefront_name('', 'Nintendo Switch') == 'Nintendo eShop'
        
        # Unknown storefront should return None for resolution workflow
        assert mapper._map_storefront_name('Unknown Store', 'PC (Windows)') is None
    
    def test_convert_single_platform(self, mapper):
        """Test single platform conversion."""
        platform_info = {
            'platform': 'PC',
            'storefront': 'Steam',
            'media': 'Digital',
            'label': 'Steam Edition',
            'purchase_date': '2023-01-15'
        }
        
        result = mapper._convert_single_platform(platform_info)
        
        assert result is not None
        assert result['platform_name'] == 'PC (Windows)'
        assert result['storefront_name'] == 'Steam'
        assert result['is_available'] is True
        assert result['is_physical'] is False
        assert result['metadata']['original_platform'] == 'PC'
        assert result['metadata']['label'] == 'Steam Edition'
    
    def test_convert_single_platform_physical(self, mapper):
        """Test physical platform conversion."""
        platform_info = {
            'platform': 'PlayStation 4',
            'storefront': 'Physical',
            'media': 'Physical'
        }
        
        result = mapper._convert_single_platform(platform_info)
        
        assert result is not None
        assert result['is_physical'] is True
        assert result['storefront_name'] == 'Physical'
    
    def test_convert_single_platform_empty(self, mapper):
        """Test empty platform handling."""
        platform_info = {'platform': '', 'storefront': 'Steam'}
        result = mapper._convert_single_platform(platform_info)
        assert result is None
        
        platform_info = {'platform': 'nan', 'storefront': 'Steam'}
        result = mapper._convert_single_platform(platform_info)
        assert result is None
    
    def test_convert_platforms_from_merged(self, mapper):
        """Test platform conversion from merged game data."""
        game_data = {
            '_platforms': [
                {'platform': 'PC', 'storefront': 'Steam', 'media': 'Digital'},
                {'platform': 'PlayStation 4', 'storefront': 'PSN', 'media': 'Digital'}
            ]
        }
        
        platforms = mapper._convert_platforms(game_data)
        
        assert len(platforms) == 2
        assert platforms[0]['platform_name'] == 'PC (Windows)'
        assert platforms[0]['storefront_name'] == 'Steam'
        assert platforms[1]['platform_name'] == 'PlayStation 4'
        assert platforms[1]['storefront_name'] == 'PlayStation Store'
    
    def test_convert_platforms_fallback(self, mapper):
        """Test platform conversion fallback to individual fields."""
        game_data = {
            'Copy platform': 'PC',
            'Copy source': 'Steam',
            'Copy media': 'Digital'
        }
        
        platforms = mapper._convert_platforms(game_data)
        
        assert len(platforms) == 1
        assert platforms[0]['platform_name'] == 'PC (Windows)'
        assert platforms[0]['storefront_name'] == 'Steam'
    
    def test_clean_string(self, mapper):
        """Test string cleaning functionality."""
        assert mapper._clean_string('  test  ') == 'test'
        assert mapper._clean_string('nan') == ''
        assert mapper._clean_string('NaN') == ''
        assert mapper._clean_string(None) == ''
        assert mapper._clean_string(pd.NA) == ''
        assert mapper._clean_string(123) == '123'
    
    def test_get_mapping_summary(self, mapper):
        """Test mapping summary generation."""
        # Add some unknown platforms and storefronts
        mapper.unknown_platforms.add('Unknown Platform')
        mapper.unknown_storefronts.add('Unknown Store')
        
        summary = mapper.get_mapping_summary()
        
        assert summary['unknown_platform_count'] == 1
        assert summary['unknown_storefront_count'] == 1
        assert 'Unknown Platform' in summary['unknown_platforms']
        assert 'Unknown Store' in summary['unknown_storefronts']
    
    def test_unknown_storefront_resolution_workflow(self, mapper):
        """Test that unknown storefronts properly flow into resolution workflow."""
        # Test the complete workflow from unknown storefront to platform data
        platform_info = {
            'platform': 'PC',
            'storefront': 'BattleDotNet',  # Unknown storefront
            'media': 'Digital',
            'label': 'Battle.net Edition',
        }
        
        # Convert the platform - this should mark the storefront as unknown
        result = mapper._convert_single_platform(platform_info)
        
        # The result should have platform data but with fallback storefront
        assert result is not None
        assert result['platform_name'] == 'PC (Windows)'
        assert result['storefront_name'] == 'Steam'  # Fallback for PC platform
        
        # But the metadata should indicate the storefront was unknown
        assert result['metadata']['original_storefront'] == 'BattleDotNet'
        assert result['metadata']['storefront_was_unknown'] is True
        
        # The unknown storefront should be tracked
        assert 'BattleDotNet' in mapper.unknown_storefronts
"""
Comprehensive tests for Darkadia data transformation pipeline.

Tests cover validation, platform mapping, boolean flag resolution,
and batch processing functionality.
"""

import pytest
from typing import Dict, Any, List
from datetime import datetime

from app.services.import_sources.darkadia_transformer import (
    DarkadiaTransformationPipeline,
    ValidationStage,
    MappingStage,
    PersistenceStage,
    TransformationContext,
    ValidationIssue
)
from app.models.user_game import PlayStatus


class TestValidationStage:
    """Test validation stage functionality."""
    
    @pytest.fixture
    def validation_stage(self):
        return ValidationStage()
    
    @pytest.fixture
    def sample_valid_row(self):
        return {
            'Name': 'The Witcher 3',
            'Added': '2024-01-15',
            'Loved': '0',
            'Owned': '1',
            'Played': '0',
            'Playing': '0',
            'Finished': '1',
            'Mastered': '0',
            'Dominated': '0',
            'Shelved': '0',
            'Rating': '4.5',
            'Copy platform': 'PC',
            'Copy source': 'Steam',
            'Notes': 'Great game!'
        }
    
    @pytest.mark.asyncio
    async def test_validates_required_fields(self, validation_stage):
        """Test that rows without required fields are rejected."""
        context = TransformationContext()
        
        # Row without Name field
        invalid_row = {'Rating': '5', 'Played': '1'}
        data = [invalid_row]
        
        result = await validation_stage.process(data, context)
        
        assert len(result) == 0  # Row should be rejected
        assert context.processed_rows == 1
        assert context.successful_rows == 0
        assert len([issue for issue in context.issues if issue.severity == 'error']) > 0
    
    @pytest.mark.asyncio
    async def test_validates_boolean_fields(self, validation_stage, sample_valid_row):
        """Test boolean field validation and conversion."""
        context = TransformationContext()
        
        # Test various boolean representations
        test_row = sample_valid_row.copy()
        test_row.update({
            'Loved': 'true',
            'Played': 'yes', 
            'Playing': '2',  # Should convert to True with warning
            'Finished': 'invalid'  # Should default to False with warning
        })
        
        result = await validation_stage.process([test_row], context)
        
        assert len(result) == 1
        row = result[0]
        assert row['Loved'] == True
        assert row['Played'] == True 
        assert row['Playing'] == True  # 2 converts to True
        assert row['Finished'] == False  # invalid converts to False
        
        # Should have warnings for unusual conversions
        warnings = [issue for issue in context.issues if issue.severity == 'warning']
        assert len(warnings) >= 2
    
    @pytest.mark.asyncio
    async def test_validates_flag_combinations(self, validation_stage, sample_valid_row):
        """Test validation of impossible boolean flag combinations."""
        context = TransformationContext()
        
        # Test Playing + Shelved (impossible)
        test_row = sample_valid_row.copy()
        test_row.update({
            'Playing': '1',
            'Shelved': '1'
        })
        
        result = await validation_stage.process([test_row], context)
        
        assert len(result) == 1
        row = result[0]
        assert row['Playing'] == False  # Should be corrected
        assert row['Shelved'] == True
        
        # Should have warning about impossible combination
        warnings = [issue for issue in context.issues if 'Playing' in issue.message and 'Shelved' in issue.message]
        assert len(warnings) > 0
    
    @pytest.mark.asyncio
    async def test_validates_hierarchy_flags(self, validation_stage, sample_valid_row):
        """Test validation of hierarchical flag relationships."""
        context = TransformationContext()
        
        # Test Dominated without Mastered/Finished
        test_row = sample_valid_row.copy()
        test_row.update({
            'Dominated': '1',
            'Mastered': '0',
            'Finished': '0'
        })
        
        result = await validation_stage.process([test_row], context)
        
        assert len(result) == 1
        row = result[0]
        assert row['Dominated'] == True
        assert row['Mastered'] == True  # Should be auto-corrected
        assert row['Finished'] == True  # Should be auto-corrected
        
        # Should have warnings about auto-correction
        warnings = [issue for issue in context.issues if issue.severity == 'warning']
        assert len(warnings) >= 2
    
    @pytest.mark.asyncio 
    async def test_validates_rating_values(self, validation_stage, sample_valid_row):
        """Test rating value validation and range checking."""
        context = TransformationContext()
        
        test_cases = [
            ('4.5', 4.5),      # Valid rating
            ('0', 0.0),        # Minimum valid
            ('5.0', 5.0),      # Maximum valid  
            ('6.5', None),     # Out of range
            ('invalid', None), # Invalid format
            ('', None),        # Empty
        ]
        
        for rating_input, expected in test_cases:
            context = TransformationContext()  # Reset context
            test_row = sample_valid_row.copy()
            test_row['Rating'] = rating_input
            
            result = await validation_stage.process([test_row], context)
            
            assert len(result) == 1
            assert result[0]['Rating'] == expected
    
    @pytest.mark.asyncio
    async def test_validates_date_formats(self, validation_stage, sample_valid_row):
        """Test date validation with various formats."""
        context = TransformationContext()
        
        test_cases = [
            ('2024-01-15', '2024-01-15'),         # ISO format
            ('01/15/2024', '2024-01-15'),         # US format
            ('15/01/2024', '2024-01-15'),         # EU format
            ('2024-01-15 12:30:45', '2024-01-15'), # With time
            ('invalid-date', None),               # Invalid
            ('', None),                           # Empty
        ]
        
        for date_input, expected in test_cases:
            context = TransformationContext()  # Reset context  
            test_row = sample_valid_row.copy()
            test_row['Added'] = date_input
            
            result = await validation_stage.process([test_row], context)
            
            assert len(result) == 1
            assert result[0]['Added'] == expected


class TestMappingStage:
    """Test platform/storefront mapping functionality."""
    
    @pytest.fixture
    def mapping_stage(self):
        return MappingStage()
    
    @pytest.fixture
    def sample_row(self):
        return {
            'Name': 'Test Game',
            'Copy platform': 'PC',
            'Copy source': 'Steam',
            'Copy source other': ''
        }
    
    @pytest.mark.asyncio
    async def test_maps_known_platforms(self, mapping_stage, sample_row):
        """Test mapping of known platforms."""
        context = TransformationContext()
        
        test_cases = [
            ('PC', 'PC (Windows)'),
            ('PlayStation 4', 'PlayStation 4'),
            ('PS4', 'PlayStation 4'),
            ('Nintendo Switch', 'Nintendo Switch')
        ]
        
        for input_platform, expected in test_cases:
            context = TransformationContext()  # Reset context
            test_row = sample_row.copy()
            test_row['Copy platform'] = input_platform
            
            result = await mapping_stage.process([test_row], context)
            
            assert len(result) == 1
            assert result[0]['_mapped_platform'] == expected
            assert result[0]['_original_platform'] == input_platform
    
    @pytest.mark.asyncio
    async def test_maps_known_storefronts(self, mapping_stage, sample_row):
        """Test mapping of known storefronts."""
        context = TransformationContext()
        
        test_cases = [
            ('Steam', 'steam'),
            ('Epic', 'epic-games-store'),
            ('GOG', 'gog'),
            ('PlayStation Store', 'playstation-store'),
            ('PSN', 'playstation-store')
        ]
        
        for input_storefront, expected in test_cases:
            context = TransformationContext()  # Reset context
            test_row = sample_row.copy()
            test_row['Copy source'] = input_storefront
            
            result = await mapping_stage.process([test_row], context)
            
            assert len(result) == 1
            assert result[0]['_mapped_storefront'] == expected
    
    @pytest.mark.asyncio
    async def test_handles_other_storefront_field(self, mapping_stage, sample_row):
        """Test handling of 'Copy source other' field."""
        context = TransformationContext()
        
        test_row = sample_row.copy()
        test_row.update({
            'Copy source': 'Other',
            'Copy source other': 'GOG'
        })
        
        result = await mapping_stage.process([test_row], context)
        
        assert len(result) == 1
        assert result[0]['_mapped_storefront'] == 'gog'
        assert result[0]['_original_storefront'] == 'GOG'  # Should use the 'other' field
    
    @pytest.mark.asyncio
    async def test_fuzzy_matching_platforms(self, mapping_stage, sample_row):
        """Test fuzzy matching for similar platform names."""
        context = TransformationContext()
        
        # Test slight variations that should fuzzy match
        test_cases = [
            'playstation 4',  # Case variation
            'PlayStation4',   # Missing space
            'PS 4',          # Different formatting
        ]
        
        for input_platform in test_cases:
            context = TransformationContext()  # Reset context
            test_row = sample_row.copy()
            test_row['Copy platform'] = input_platform
            
            result = await mapping_stage.process([test_row], context)
            
            assert len(result) == 1
            # Should fuzzy match to PlayStation 4
            assert result[0]['_mapped_platform'] == 'PlayStation 4'
            
            # Should have info message about fuzzy match
            info_issues = [issue for issue in context.issues if issue.severity == 'info']
            assert len(info_issues) > 0
    
    @pytest.mark.asyncio
    async def test_tracks_unknown_platforms(self, mapping_stage, sample_row):
        """Test tracking of unknown platforms."""
        context = TransformationContext()
        
        test_row = sample_row.copy()
        test_row['Copy platform'] = 'Fake Gaming System XYZ'
        
        result = await mapping_stage.process([test_row], context)
        
        assert len(result) == 1
        assert result[0]['_mapped_platform'] is None
        assert 'Fake Gaming System XYZ' in context.unknown_platforms
        
        # Should have warning about unknown platform
        warnings = [issue for issue in context.issues if issue.severity == 'warning']
        assert len(warnings) > 0
    
    @pytest.mark.asyncio
    async def test_no_default_storefront_when_empty(self, mapping_stage, sample_row):
        """Test that no default storefront is assigned when none specified - user must choose."""
        context = TransformationContext()
        
        test_row = sample_row.copy()
        test_row.update({
            'Copy platform': 'PC',
            'Copy source': ''  # No storefront specified
        })
        
        result = await mapping_stage.process([test_row], context)
        
        assert len(result) == 1
        assert result[0]['_mapped_platform'] == 'PC (Windows)'
        assert result[0]['_mapped_storefront'] is None  # No default - user must choose


class TestPersistenceStage:
    """Test persistence stage functionality."""
    
    @pytest.fixture
    def persistence_stage(self):
        return PersistenceStage()
    
    @pytest.fixture
    def sample_row(self):
        return {
            'Name': 'Test Game',
            'Loved': False,
            'Played': True,
            'Playing': False,
            'Finished': False,
            'Mastered': False,
            'Dominated': False,
            'Shelved': False,
            'Rating': 4.5,
            'Copy label': 'GOTY Edition',
            'Copy platform': 'PC',
            'Copy media': 'Digital'
        }
    
    @pytest.mark.asyncio
    async def test_resolves_play_status_correctly(self, persistence_stage, sample_row):
        """Test play status resolution with different flag combinations."""
        context = TransformationContext()
        
        test_cases = [
            # (flags, expected_status)
            ({'Dominated': True}, PlayStatus.DOMINATED.value),
            ({'Mastered': True}, PlayStatus.MASTERED.value),
            ({'Finished': True}, PlayStatus.COMPLETED.value),
            ({'Shelved': True}, PlayStatus.DROPPED.value),  # Shelved = Dropped
            ({'Playing': True}, PlayStatus.IN_PROGRESS.value),
            ({'Played': True}, PlayStatus.SHELVED.value),   # Played only = Shelved
            ({}, PlayStatus.NOT_STARTED.value),
        ]
        
        for flags, expected_status in test_cases:
            context = TransformationContext()  # Reset context
            test_row = sample_row.copy()
            
            # Reset all flags to False, then set the test flags
            for flag in ['Played', 'Playing', 'Finished', 'Mastered', 'Dominated', 'Shelved']:
                test_row[flag] = flags.get(flag, False)
            
            result = await persistence_stage.process([test_row], context)
            
            assert len(result) == 1
            assert result[0]['_resolved_play_status'] == expected_status
    
    @pytest.mark.asyncio
    async def test_extracts_copy_metadata(self, persistence_stage, sample_row):
        """Test extraction of physical copy metadata."""
        context = TransformationContext()
        
        test_row = sample_row.copy()
        test_row.update({
            'Copy label': 'GOTY Edition',
            'Copy Release': '2015',
            'Copy platform': 'PlayStation 4',
            'Copy media': 'Physical',
            'Copy box': 'Excellent',
            'Copy manual': 'Good',
            'Copy complete': 'Yes'
        })
        
        result = await persistence_stage.process([test_row], context)
        
        assert len(result) == 1
        copy_metadata = result[0]['_copy_metadata']
        
        assert copy_metadata is not None
        assert copy_metadata['label'] == 'GOTY Edition'
        assert copy_metadata['release'] == '2015'
        assert copy_metadata['platform'] == 'PlayStation 4'
        assert copy_metadata['media'] == 'Physical'
        assert copy_metadata['box'] == 'Excellent'
    
    @pytest.mark.asyncio
    async def test_handles_empty_copy_metadata(self, persistence_stage, sample_row):
        """Test handling when no copy metadata is present."""
        context = TransformationContext()
        
        test_row = sample_row.copy()
        # Remove all copy fields
        for key in list(test_row.keys()):
            if key.startswith('Copy'):
                del test_row[key]
        
        result = await persistence_stage.process([test_row], context)
        
        assert len(result) == 1
        assert '_copy_metadata' not in result[0] or result[0]['_copy_metadata'] is None


class TestDarkadiaTransformationPipeline:
    """Test the complete transformation pipeline."""
    
    @pytest.fixture
    def pipeline(self):
        return DarkadiaTransformationPipeline()
    
    @pytest.fixture
    def sample_csv_data(self):
        return [
            {
                'Name': 'The Witcher 3: Wild Hunt',
                'Added': '2024-01-15',
                'Loved': '0',
                'Owned': '1', 
                'Played': '0',
                'Playing': '0',
                'Finished': '1',
                'Mastered': '0',
                'Dominated': '0',
                'Shelved': '0',
                'Rating': '4.5',
                'Copy platform': 'PC',
                'Copy source': 'Steam',
                'Copy label': 'GOTY Edition',
                'Notes': 'Excellent RPG'
            },
            {
                'Name': 'Cyberpunk 2077',
                'Added': '2023-12-01',
                'Loved': '1',
                'Owned': '1',
                'Played': '1',
                'Playing': '0', 
                'Finished': '0',
                'Mastered': '0',
                'Dominated': '0',
                'Shelved': '1',  # Played + Shelved = DROPPED
                'Rating': '3.0',
                'Copy platform': 'PlayStation 4',
                'Copy source': 'PSN',
                'Notes': 'Disappointing'
            }
        ]
    
    @pytest.mark.asyncio
    async def test_complete_pipeline_transformation(self, pipeline, sample_csv_data):
        """Test complete pipeline transformation of sample data."""
        transformed_data, context = await pipeline.transform(sample_csv_data)
        
        assert len(transformed_data) == 2
        assert context.total_rows == 2
        assert context.successful_rows == 2
        
        # Check first game transformation
        game1 = transformed_data[0]
        assert game1['Name'] == 'The Witcher 3: Wild Hunt'
        assert game1['_resolved_play_status'] == PlayStatus.COMPLETED.value
        assert game1['_mapped_platform'] == 'PC (Windows)'
        assert game1['_mapped_storefront'] == 'steam'
        assert game1['Rating'] == 4.5
        
        # Check second game transformation
        game2 = transformed_data[1]
        assert game2['Name'] == 'Cyberpunk 2077'
        assert game2['_resolved_play_status'] == PlayStatus.DROPPED.value  # Shelved = Dropped
        assert game2['_mapped_platform'] == 'PlayStation 4'
        assert game2['_mapped_storefront'] == 'playstation-store'  # PSN mapped to database name
        assert game2['Rating'] == 3.0
    
    @pytest.mark.asyncio
    async def test_handles_invalid_data_gracefully(self, pipeline):
        """Test pipeline handling of invalid/problematic data."""
        invalid_data = [
            {
                # Missing required Name field
                'Rating': '5',
                'Played': '1'
            },
            {
                'Name': 'Valid Game',
                'Playing': '1',
                'Shelved': '1',  # Impossible combination
                'Rating': '10',  # Out of range
                'Copy platform': 'Unknown Platform',
                'Added': 'invalid-date'
            }
        ]
        
        transformed_data, context = await pipeline.transform(invalid_data)
        
        # First row should be rejected due to missing Name
        # Second row should be processed with corrections/warnings
        assert len(transformed_data) == 1
        assert context.total_rows == 2
        assert context.successful_rows == 1
        
        # Check the valid game was processed with corrections
        game = transformed_data[0]
        assert game['Name'] == 'Valid Game'
        assert game['Playing'] == False  # Corrected from impossible combination
        assert game['Shelved'] == True
        assert game['Rating'] is None  # Out of range rating cleared
        assert game['Added'] is None   # Invalid date cleared
        
        # Should have warnings for corrections and unknown platform
        warnings = [issue for issue in context.issues if issue.severity == 'warning']
        assert len(warnings) >= 3  # Playing/Shelved, rating, unknown platform, date
        assert len(context.unknown_platforms) == 1
        assert 'Unknown Platform' in context.unknown_platforms
    
    @pytest.mark.asyncio
    async def test_context_summary_generation(self, pipeline, sample_csv_data):
        """Test transformation context summary generation."""
        transformed_data, context = await pipeline.transform(sample_csv_data)
        
        summary = context.get_summary()
        
        assert summary['total_rows'] == 2
        assert summary['processed_rows'] == 2
        assert summary['successful_rows'] == 2
        assert 'error_count' in summary
        assert 'warning_count' in summary
        assert 'unknown_platforms' in summary
        assert 'unknown_storefronts' in summary
        assert isinstance(summary['issues'], list)
    
    @pytest.mark.asyncio
    async def test_empty_data_handling(self, pipeline):
        """Test pipeline handling of empty data."""
        transformed_data, context = await pipeline.transform([])
        
        assert len(transformed_data) == 0
        assert context.total_rows == 0
        assert context.successful_rows == 0
        assert len(context.issues) == 0
    
    @pytest.mark.asyncio
    async def test_batch_processing_large_dataset(self, pipeline):
        """Test pipeline with large dataset (simulated)."""
        # Create a moderately large dataset
        large_data = []
        for i in range(50):
            game_data = {
                'Name': f'Game {i}',
                'Added': '2024-01-01',
                'Owned': '1',
                'Finished': '1' if i % 2 == 0 else '0',
                'Playing': '1' if i % 3 == 0 else '0',
                'Rating': str(float(i % 5)),
                'Copy platform': 'PC' if i % 2 == 0 else 'PlayStation 4',
                'Copy source': 'Steam' if i % 2 == 0 else 'PlayStation Store'
            }
            large_data.append(game_data)
        
        transformed_data, context = await pipeline.transform(large_data)
        
        assert len(transformed_data) == 50
        assert context.total_rows == 50
        assert context.successful_rows == 50
    
    @pytest.mark.asyncio
    async def test_platforms_metadata_creation(self, pipeline):
        """Test that _platforms metadata is correctly created from transformation data."""
        sample_data = [
            {
                "Name": "Test Game with Copy Data",
                "Copy platform": "PC",
                "Copy source": "Steam",
                "Copy media": "Digital",
                "Copy label": "GOTY Edition",
                "Copy Release": "2024",
                "Played": True,
                "Rating": "4.5"
            },
            {
                "Name": "Test Game with Other Source",
                "Copy platform": "PlayStation 4", 
                "Copy source": "Other",
                "Copy source other": "PSN",
                "Copy media": "Digital",
                "Played": False,
                "Rating": ""
            },
            {
                "Name": "Test Game with Fallback Platform",
                "Platforms": "Xbox One, PC",
                "Played": True,
                "Rating": "3.0"
            }
        ]
        
        result, context = await pipeline.transform(sample_data)
        
        # Test game 1: Should have copy data with platform and storefront
        game1 = result[0]
        assert '_platforms' in game1
        platforms1 = game1['_platforms']
        assert len(platforms1) == 1
        platform1 = platforms1[0]
        assert platform1['platform'] == 'PC (Windows)'  # Should be mapped
        assert platform1['original_platform'] == 'PC'
        assert platform1['storefront'] == 'steam'  # Should be mapped to database name
        assert platform1['original_storefront'] == 'Steam'
        assert platform1['is_real_copy'] == True
        assert platform1['copy_identifier'] == 'plt:PC|str:Steam'
        
        # Test game 2: Should handle "Other" storefront with fallback to Copy source other
        game2 = result[1]
        assert '_platforms' in game2
        platforms2 = game2['_platforms']
        assert len(platforms2) == 1
        platform2 = platforms2[0]
        assert platform2['platform'] == 'PlayStation 4'  # Should be mapped
        assert platform2['original_platform'] == 'PlayStation 4'
        assert platform2['storefront'] == 'playstation-store'  # PSN should map to playstation-store database name
        assert platform2['original_storefront'] == 'PSN'
        assert platform2['is_real_copy'] == True
        
        # Test game 3: Should have fallback platform data
        game3 = result[2]
        assert '_platforms' in game3
        platforms3 = game3['_platforms']
        assert len(platforms3) == 2  # Should create entries for both Xbox One and PC
        
        # Check first fallback platform (Xbox One)
        xbox_platform = platforms3[0]
        assert xbox_platform['platform'] == 'Xbox One'  # Should be mapped (first platform gets mapping)
        assert xbox_platform['original_platform'] == 'Xbox One'
        assert xbox_platform['storefront'] is None
        assert xbox_platform['original_storefront'] is None
        assert xbox_platform['is_real_copy'] == False
        assert xbox_platform['requires_storefront_resolution'] == True
        assert xbox_platform['copy_identifier'] == 'fallback:Xbox One'
        
        # Check second fallback platform (PC)
        pc_platform = platforms3[1] 
        assert pc_platform['platform'] == 'PC'  # Original name (only first platform gets mapping)
        assert pc_platform['original_platform'] == 'PC'
        assert pc_platform['storefront'] is None
        assert pc_platform['original_storefront'] is None
        assert pc_platform['is_real_copy'] == False
        assert pc_platform['requires_storefront_resolution'] == True
        assert pc_platform['copy_identifier'] == 'fallback:PC'
        
        # Verify all games have _platforms metadata now
        assert all('_platforms' in game for game in result)
        assert context.successful_rows == 3


if __name__ == "__main__":
    pytest.main([__file__])
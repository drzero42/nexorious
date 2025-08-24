"""
Tests for Darkadia CSV Parser module.
"""

import pytest
import asyncio
from pathlib import Path
import tempfile
import csv
from datetime import datetime

from scripts.darkadia.parser import DarkadiaCSVParser


class TestDarkadiaCSVParser:
    """Test cases for DarkadiaCSVParser."""
    
    @pytest.fixture
    def parser(self):
        """Create a parser instance for testing."""
        return DarkadiaCSVParser(verbose=False)
    
    @pytest.fixture
    def sample_csv_data(self):
        """Sample CSV data for testing."""
        return [
            {
                'Name': 'Test Game 1',
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
                'Copy platform': 'PC',
                'Copy source': 'Steam',
                'Copy media': 'Digital',
                'Notes': 'Great game!'
            },
            {
                'Name': 'Test Game 2',
                'Added': '2023-02-20',
                'Loved': 0,
                'Owned': 1,
                'Played': 0,
                'Playing': 1,
                'Finished': 0,
                'Mastered': 0,
                'Dominated': 0,
                'Shelved': 0,
                'Rating': 3.0,
                'Copy platform': 'PlayStation 4',
                'Copy source': 'PSN',
                'Copy media': 'Digital',
                'Notes': ''
            }
        ]
    
    def create_temp_csv(self, data):
        """Create a temporary CSV file with the given data."""
        temp_file = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False)
        
        if data:
            writer = csv.DictWriter(temp_file, fieldnames=data[0].keys())
            writer.writeheader()
            writer.writerows(data)
        
        temp_file.close()
        return Path(temp_file.name)
    
    @pytest.mark.asyncio
    async def test_parse_valid_csv(self, parser, sample_csv_data):
        """Test parsing a valid CSV file."""
        csv_file = self.create_temp_csv(sample_csv_data)
        
        try:
            games_data = await parser.parse_csv(csv_file)
            
            assert len(games_data) == 2
            assert games_data[0]['Name'] == 'Test Game 1'
            assert games_data[1]['Name'] == 'Test Game 2'
            
            # Check boolean conversion
            assert games_data[0]['Loved'] is True
            assert games_data[1]['Loved'] is False
            
            # Check numeric conversion
            assert games_data[0]['Rating'] == 4.5
            assert games_data[1]['Rating'] == 3.0
            
        finally:
            csv_file.unlink()  # Clean up
    
    @pytest.mark.asyncio
    async def test_parse_empty_csv(self, parser):
        """Test parsing an empty CSV file."""
        csv_file = self.create_temp_csv([])
        
        try:
            with pytest.raises(ValueError, match="CSV file is empty"):
                await parser.parse_csv(csv_file)
        finally:
            csv_file.unlink()
    
    @pytest.mark.asyncio
    async def test_parse_missing_critical_columns(self, parser):
        """Test parsing CSV missing critical columns."""
        data = [{'NotName': 'Test', 'Other': 'Data'}]
        csv_file = self.create_temp_csv(data)
        
        try:
            with pytest.raises(ValueError, match="Missing critical columns"):
                await parser.parse_csv(csv_file)
        finally:
            csv_file.unlink()
    
    
    @pytest.mark.asyncio
    async def test_handle_continuation_rows(self, parser):
        """Test handling of continuation rows in CSV."""
        import pandas as pd
        
        # Create DataFrame with continuation rows (empty Name cells)
        df = pd.DataFrame([
            {'Name': 'Multi-Platform Game', 'Copy platform': 'PC'},
            {'Name': '', 'Copy platform': 'PlayStation 4'},  # Continuation row
            {'Name': 'Another Game', 'Copy platform': 'Xbox One'}
        ])
        
        cleaned_df = await parser._handle_continuation_rows(df)
        
        # All rows should have the game name filled
        assert cleaned_df.iloc[0]['Name'] == 'Multi-Platform Game'
        assert cleaned_df.iloc[1]['Name'] == 'Multi-Platform Game'
        assert cleaned_df.iloc[2]['Name'] == 'Another Game'
    
    
    @pytest.mark.asyncio
    async def test_invalid_ratings_cleaned(self, parser):
        """Test that invalid ratings are cleaned during processing."""
        data = [
            {'Name': 'Game 1', 'Rating': 6.0},  # Invalid (>5)
            {'Name': 'Game 2', 'Rating': -1.0}, # Invalid (<0)
            {'Name': 'Game 3', 'Rating': 4.5},  # Valid
        ]
        
        csv_file = self.create_temp_csv(data)
        
        try:
            games_data = await parser.parse_csv(csv_file)
            
            # Invalid ratings should be converted to None
            assert games_data[0]['Rating'] != 6.0  # Should be cleaned
            assert games_data[1]['Rating'] != -1.0  # Should be cleaned
            assert games_data[2]['Rating'] == 4.5   # Should remain valid
            
            # Should have validation errors recorded
            assert len(parser.validation_errors) > 0
            
        finally:
            csv_file.unlink()
    
    def test_get_validation_summary(self, parser):
        """Test validation summary generation."""
        # Add some mock errors and warnings
        parser.validation_errors = ["Error 1", "Error 2"]
        parser.warnings = ["Warning 1"]
        
        summary = parser.get_validation_summary()
        
        assert summary['error_count'] == 2
        assert summary['warning_count'] == 1
        assert len(summary['errors']) == 2
        assert len(summary['warnings']) == 1
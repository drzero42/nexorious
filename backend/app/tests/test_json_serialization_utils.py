"""
Tests for enhanced JSON serialization utilities.

This test suite verifies that the enhanced JSON serialization utilities
properly handle pandas types, datetime objects, and complex nested structures
without throwing serialization errors.
"""

import json
import pytest
import pandas as pd
from datetime import datetime, date
from dataclasses import dataclass
from decimal import Decimal
from typing import Any, Dict, List

from app.utils.json_serialization import (
    PandasJSONEncoder,
    make_json_serializable,
    safe_json_dumps,
    enhanced_safe_json_dumps,
    debug_non_serializable_fields,
    deep_debug_serialization_issues
)


@dataclass
class SampleDataClass:
    """Sample dataclass for serialization testing."""
    name: str
    timestamp: pd.Timestamp
    value: int


class TestPandasJSONEncoder:
    """Test the custom pandas JSON encoder."""
    
    def test_encodes_pandas_timestamp(self):
        """Test encoding of pandas Timestamp objects."""
        encoder = PandasJSONEncoder()
        timestamp = pd.Timestamp('2023-01-15')
        result = encoder.default(timestamp)
        assert result == '2023-01-15'
    
    def test_encodes_pandas_nat(self):
        """Test encoding of pandas NaT objects."""
        encoder = PandasJSONEncoder()
        result = encoder.default(pd.NaT)
        assert result is None
    
    def test_encodes_python_datetime(self):
        """Test encoding of Python datetime objects."""
        encoder = PandasJSONEncoder()
        dt = datetime(2023, 1, 15, 10, 30)
        result = encoder.default(dt)
        assert result == '2023-01-15'
    
    def test_encodes_python_date(self):
        """Test encoding of Python date objects."""
        encoder = PandasJSONEncoder()
        dt = date(2023, 1, 15)
        result = encoder.default(dt)
        assert result == '2023-01-15'
    
    def test_encodes_decimal(self):
        """Test encoding of Decimal objects."""
        encoder = PandasJSONEncoder()
        decimal_val = Decimal('3.14159')
        result = encoder.default(decimal_val)
        assert result == 3.14159


class TestMakeJsonSerializable:
    """Test the make_json_serializable function."""
    
    def test_handles_none(self):
        """Test handling of None values."""
        result = make_json_serializable(None)
        assert result is None
    
    def test_handles_basic_types(self):
        """Test handling of basic JSON-serializable types."""
        test_cases = [
            ('hello', 'hello'),
            (42, 42),
            (3.14, 3.14),
            (True, True),
            (False, False)
        ]
        
        for input_val, expected in test_cases:
            result = make_json_serializable(input_val)
            assert result == expected
    
    def test_handles_pandas_timestamp(self):
        """Test handling of pandas Timestamp objects."""
        timestamp = pd.Timestamp('2023-01-15')
        result = make_json_serializable(timestamp)
        assert result == '2023-01-15'
    
    def test_handles_pandas_nat(self):
        """Test handling of pandas NaT objects."""
        result = make_json_serializable(pd.NaT)
        assert result is None
    
    def test_handles_python_datetime(self):
        """Test handling of Python datetime objects."""
        dt = datetime(2023, 1, 15, 10, 30)
        result = make_json_serializable(dt)
        assert result == '2023-01-15'
    
    def test_handles_lists(self):
        """Test handling of lists with mixed types."""
        test_list = [
            'string',
            42,
            pd.Timestamp('2023-01-15'),
            pd.NaT,
            datetime(2023, 2, 1)
        ]
        
        result = make_json_serializable(test_list)
        expected = ['string', 42, '2023-01-15', None, '2023-02-01']
        assert result == expected
    
    def test_handles_dictionaries(self):
        """Test handling of dictionaries with mixed types."""
        test_dict = {
            'string': 'hello',
            'number': 42,
            'timestamp': pd.Timestamp('2023-01-15'),
            'nat': pd.NaT,
            'datetime': datetime(2023, 2, 1),
            'nested': {
                'inner_timestamp': pd.Timestamp('2023-03-01')
            }
        }
        
        result = make_json_serializable(test_dict)
        expected = {
            'string': 'hello',
            'number': 42,
            'timestamp': '2023-01-15',
            'nat': None,
            'datetime': '2023-02-01',
            'nested': {
                'inner_timestamp': '2023-03-01'
            }
        }
        assert result == expected
    
    def test_handles_dataclasses(self):
        """Test handling of dataclass objects."""
        test_obj = SampleDataClass(
            name="test",
            timestamp=pd.Timestamp('2023-01-15'),
            value=42
        )
        
        result = make_json_serializable(test_obj)
        expected = {
            'name': 'test',
            'timestamp': '2023-01-15',
            'value': 42
        }
        assert result == expected
    
    def test_handles_decimal(self):
        """Test handling of Decimal objects."""
        decimal_val = Decimal('3.14159')
        result = make_json_serializable(decimal_val)
        assert result == 3.14159


class TestSafeJsonDumps:
    """Test the safe_json_dumps function."""
    
    def test_serializes_clean_data(self):
        """Test serialization of already clean data."""
        clean_data = {'string': 'hello', 'number': 42}
        result = safe_json_dumps(clean_data)
        parsed = json.loads(result)
        assert parsed == clean_data
    
    def test_serializes_pandas_types(self):
        """Test serialization of data with pandas types."""
        data_with_pandas = {
            'timestamp': pd.Timestamp('2023-01-15'),
            'nat': pd.NaT,
            'list': [pd.Timestamp('2023-02-01'), 'string', pd.NaT]
        }
        
        result = safe_json_dumps(data_with_pandas)
        parsed = json.loads(result)
        expected = {
            'timestamp': '2023-01-15',
            'nat': None,
            'list': ['2023-02-01', 'string', None]
        }
        assert parsed == expected
    
    def test_serializes_complex_nested_structure(self):
        """Test serialization of complex nested structures."""
        complex_data = {
            'level1': {
                'level2': {
                    'timestamps': [
                        pd.Timestamp('2023-01-01'),
                        pd.Timestamp('2023-01-02'),
                        pd.NaT
                    ],
                    'mixed': {
                        'date': datetime(2023, 1, 15),
                        'string': 'test',
                        'number': 42
                    }
                }
            }
        }
        
        result = safe_json_dumps(complex_data)
        parsed = json.loads(result)
        
        # Verify the structure was properly converted
        assert parsed['level1']['level2']['timestamps'] == ['2023-01-01', '2023-01-02', None]
        assert parsed['level1']['level2']['mixed']['date'] == '2023-01-15'
        assert parsed['level1']['level2']['mixed']['string'] == 'test'
        assert parsed['level1']['level2']['mixed']['number'] == 42


class TestEnhancedSafeJsonDumps:
    """Test the enhanced_safe_json_dumps function."""
    
    def test_basic_functionality(self):
        """Test basic functionality with clean data."""
        clean_data = {'test': 'value'}
        result = enhanced_safe_json_dumps(clean_data, context="test")
        parsed = json.loads(result)
        assert parsed == clean_data
    
    def test_handles_problematic_data(self):
        """Test handling of problematic data with pandas types."""
        problematic_data = {
            'timestamps': [pd.Timestamp('2023-01-01'), pd.NaT],
            'datetime': datetime(2023, 1, 15),
            'nested': {
                'deep_timestamp': pd.Timestamp('2023-02-01')
            }
        }
        
        result = enhanced_safe_json_dumps(problematic_data, context="problematic_test")
        parsed = json.loads(result)
        
        assert parsed['timestamps'] == ['2023-01-01', None]
        assert parsed['datetime'] == '2023-01-15'
        assert parsed['nested']['deep_timestamp'] == '2023-02-01'


class TestDebugUtilities:
    """Test debugging utilities for serialization issues."""
    
    def test_debug_non_serializable_fields_clean_data(self):
        """Test debugging with clean, serializable data."""
        clean_data = {'string': 'hello', 'number': 42}
        problems = debug_non_serializable_fields(clean_data)
        assert len(problems) == 0
    
    def test_debug_non_serializable_fields_problematic_data(self):
        """Test debugging with problematic data."""
        problematic_data = {
            'timestamp': pd.Timestamp('2023-01-15'),
            'nat': pd.NaT,
            'datetime': datetime(2023, 1, 15)
        }
        
        problems = debug_non_serializable_fields(problematic_data)
        assert len(problems) > 0
        
        # Check that pandas types are identified
        problem_types = [p.split(':')[1].strip() for p in problems]
        assert any('pandas' in ptype.lower() for ptype in problem_types)
    
    def test_deep_debug_serialization_issues(self):
        """Test deep debugging analysis."""
        problematic_data = {
            'timestamp1': pd.Timestamp('2023-01-15'),
            'timestamp2': pd.Timestamp('2023-01-16'),
            'nat': pd.NaT,
            'datetime': datetime(2023, 1, 15)
        }
        
        analysis = deep_debug_serialization_issues(problematic_data, "test_data")
        
        assert analysis['name'] == 'test_data'
        assert analysis['is_json_safe'] is False
        assert analysis['total_issues'] > 0
        assert analysis['pandas_timestamps_found'] >= 2
        assert analysis['pandas_nat_found'] >= 1
        assert analysis['python_datetimes_found'] >= 1


class TestIntegrationScenarios:
    """Test integration scenarios that simulate real-world usage."""
    
    def test_darkadia_base_data_simulation(self):
        """Test serialization of data structure similar to Darkadia base_data."""
        base_data = {
            'Game': 'Test Game',
            'Rating': 4.5,
            'Notes': 'Great game!',
            'Loved': True,
            'Owned': True,
            'Played': False,
            'Playing': False,
            'Finished': False,
            'Mastered': False,
            'Dominated': False,
            'Shelved': False,
            'Added': pd.Timestamp('2023-01-10'),
            'Some_Date_Field': pd.NaT,
            'Another_Date': datetime(2023, 1, 15),
        }
        
        # Test that it can be serialized without errors
        result = enhanced_safe_json_dumps(base_data, "base_data_simulation")
        parsed = json.loads(result)
        
        # Verify key conversions
        assert parsed['Added'] == '2023-01-10'
        assert parsed['Some_Date_Field'] is None
        assert parsed['Another_Date'] == '2023-01-15'
        assert parsed['Game'] == 'Test Game'
        assert parsed['Rating'] == 4.5
    
    def test_darkadia_copy_data_simulation(self):
        """Test serialization of data structure similar to Darkadia copy data."""
        copy_data_dict = {
            'platform': 'PlayStation 4',
            'storefront': 'PlayStation Store',
            'storefront_other': '',
            'media': 'Digital',
            'label': 'Standard Edition',
            'release': '2023-01-15',
            'purchase_date': pd.Timestamp('2023-02-01'),  # This could be a pandas Timestamp
            'box': 'N/A',
            'box_condition': '',
            'box_notes': '',
            'manual': 'N/A',
            'manual_condition': '',
            'manual_notes': '',
            'complete': 'N/A',
            'complete_notes': ''
        }
        
        # Test that it can be serialized without errors
        result = enhanced_safe_json_dumps(copy_data_dict, "copy_data_simulation")
        parsed = json.loads(result)
        
        # Verify key conversions
        assert parsed['purchase_date'] == '2023-02-01'
        assert parsed['platform'] == 'PlayStation 4'
        assert parsed['storefront'] == 'PlayStation Store'
    
    def test_darkadia_merged_data_simulation(self):
        """Test serialization of merged CSV data (base_data + copy_data)."""
        # Simulate what happens in the original_csv_data_json creation
        base_data = {
            'Game': 'Test Game',
            'Added': pd.Timestamp('2023-01-10'),
            'Rating': 4.5,
            'Notes': 'Great game!'
        }
        
        copy_data = {
            'platform': 'PC',
            'storefront': 'Steam',
            'purchase_date': pd.Timestamp('2023-02-01')
        }
        
        merged_data = {
            **base_data,
            'Copy platform': copy_data['platform'] or '',
            'Copy source': copy_data['storefront'] or '',
            'Copy purchase date': copy_data['purchase_date'],
        }
        
        # This is the critical test - can we serialize the merged data?
        result = enhanced_safe_json_dumps(merged_data, "merged_data_simulation")
        parsed = json.loads(result)
        
        # Verify all conversions worked
        assert parsed['Added'] == '2023-01-10'
        assert parsed['Copy purchase date'] == '2023-02-01'
        assert parsed['Game'] == 'Test Game'
        assert parsed['Copy platform'] == 'PC'
        assert parsed['Copy source'] == 'Steam'


if __name__ == "__main__":
    pytest.main([__file__])
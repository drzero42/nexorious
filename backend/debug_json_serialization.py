#!/usr/bin/env python3
"""
Comprehensive debug script for identifying pandas Timestamp serialization issues.

This script provides detailed analysis of data structures to identify exactly 
where non-JSON-serializable objects (particularly pandas Timestamps) are hiding.
"""

import json
import pandas as pd
from datetime import datetime
from typing import Any, Dict, List, Optional, Union
from dataclasses import dataclass, asdict, is_dataclass
import logging

# Configure logging for detailed debugging
logging.basicConfig(
    level=logging.DEBUG,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

def deep_type_analysis(data: Any, path: str = "root", max_depth: int = 10) -> List[str]:
    """
    Perform deep analysis of data structure types with exact paths.
    
    Args:
        data: Data structure to analyze
        path: Current path in the structure
        max_depth: Maximum recursion depth to prevent infinite loops
        
    Returns:
        List of detailed type information for all nested values
    """
    type_info = []
    
    if max_depth <= 0:
        type_info.append(f"{path}: MAX_DEPTH_REACHED - {type(data)}")
        return type_info
    
    # Add current level type info
    type_info.append(f"{path}: {type(data)} = {repr(data) if len(str(data)) < 100 else str(type(data))}")
    
    # Recurse based on data type
    if isinstance(data, dict):
        for key, value in data.items():
            new_path = f"{path}['{key}']"
            type_info.extend(deep_type_analysis(value, new_path, max_depth - 1))
    
    elif isinstance(data, (list, tuple)):
        for i, value in enumerate(data):
            new_path = f"{path}[{i}]"
            type_info.extend(deep_type_analysis(value, new_path, max_depth - 1))
    
    elif is_dataclass(data):
        for field_name, field_value in asdict(data).items():
            new_path = f"{path}.{field_name}"
            type_info.extend(deep_type_analysis(field_value, new_path, max_depth - 1))
    
    elif hasattr(data, '__dict__'):
        for attr_name, attr_value in data.__dict__.items():
            if not attr_name.startswith('_'):  # Skip private attributes
                new_path = f"{path}.{attr_name}"
                type_info.extend(deep_type_analysis(attr_value, new_path, max_depth - 1))
    
    return type_info

def find_json_problematic_fields(data: Any, path: str = "root") -> List[Dict[str, Any]]:
    """
    Find fields that will cause JSON serialization to fail.
    
    Returns detailed information about each problematic field including
    its path, type, value, and why it's problematic.
    """
    problems = []
    
    def is_json_safe(value: Any) -> bool:
        """Test if a value is JSON serializable."""
        try:
            json.dumps(value)
            return True
        except (TypeError, ValueError):
            return False
    
    def safe_pd_isna(value: Any) -> bool:
        """Safely check if a value is pandas NaT/NaN."""
        try:
            if hasattr(value, '__len__') and not isinstance(value, str):
                # For arrays/sequences, we don't want to check isna
                return False
            return pd.isna(value)
        except (ValueError, TypeError, AttributeError):
            return False
    
    def check_value(val: Any, current_path: str):
        # Check if current value is JSON safe
        if not is_json_safe(val):
            problem_info = {
                'path': current_path,
                'type': str(type(val)),
                'value': repr(val) if len(str(val)) < 200 else f"{str(val)[:200]}...",
                'is_pandas_timestamp': isinstance(val, pd.Timestamp),
                'is_pandas_nat': safe_pd_isna(val),
                'is_datetime': isinstance(val, datetime),
                'reason': 'Unknown'
            }
            
            # Determine the specific reason
            if isinstance(val, pd.Timestamp):
                problem_info['reason'] = 'pandas Timestamp'
            elif safe_pd_isna(val):
                problem_info['reason'] = 'pandas NaT/NaN'
            elif isinstance(val, datetime):
                problem_info['reason'] = 'Python datetime'
            elif is_dataclass(val):
                problem_info['reason'] = 'Dataclass instance'
            elif hasattr(val, '__dict__'):
                problem_info['reason'] = 'Custom object with __dict__'
            else:
                problem_info['reason'] = f'Non-serializable type: {type(val)}'
            
            problems.append(problem_info)
        
        # Recurse into data structures
        if isinstance(val, dict):
            for k, v in val.items():
                check_value(v, f"{current_path}['{k}']")
        elif isinstance(val, (list, tuple)):
            for i, v in enumerate(val):
                check_value(v, f"{current_path}[{i}]")
        elif is_dataclass(val):
            for field_name, field_value in asdict(val).items():
                check_value(field_value, f"{current_path}.{field_name}")
    
    check_value(data, path)
    return problems

def enhanced_pandas_safe_converter(data: Any) -> Any:
    """
    Enhanced recursive converter that handles ALL pandas types and nested structures.
    
    This is more comprehensive than the existing make_json_serializable function.
    """
    # Handle None and basic types first
    if data is None:
        return None
    
    # Handle pandas types with comprehensive coverage
    if hasattr(pd, 'api') and hasattr(pd.api, 'types'):
        # Use pandas API for comprehensive type checking, but avoid arrays
        try:
            if hasattr(data, '__len__') and not isinstance(data, str):
                # Skip pandas array checking for sequences
                pass
            elif pd.api.types.is_scalar(data):
                if pd.isna(data):
                    return None
                elif isinstance(data, pd.Timestamp):
                    return data.strftime('%Y-%m-%d')
                elif isinstance(data, (pd.Timedelta, pd.Period)):
                    return str(data)
                elif isinstance(data, pd.Categorical):
                    return str(data)
        except (ValueError, TypeError):
            # If pandas API fails, continue with fallback checks
            pass
    
    # Fallback pandas checks
    if isinstance(data, pd.Timestamp):
        try:
            if pd.isna(data):
                return None
        except (ValueError, TypeError):
            pass
        return data.strftime('%Y-%m-%d')
    
    try:
        if pd.isna(data):
            return None
    except (ValueError, TypeError):
        # Not a pandas type or array issue
        pass
    
    # Handle Python datetime objects
    if isinstance(data, datetime):
        return data.strftime('%Y-%m-%d')
    
    # Handle dataclasses
    if is_dataclass(data):
        return enhanced_pandas_safe_converter(asdict(data))
    
    # Handle dictionaries recursively
    if isinstance(data, dict):
        return {str(k): enhanced_pandas_safe_converter(v) for k, v in data.items()}
    
    # Handle lists/tuples recursively
    if isinstance(data, (list, tuple)):
        return [enhanced_pandas_safe_converter(item) for item in data]
    
    # Handle objects with __dict__
    if hasattr(data, '__dict__') and not isinstance(data, (str, int, float, bool)):
        try:
            return enhanced_pandas_safe_converter(data.__dict__)
        except:
            # If __dict__ fails, try string conversion
            return str(data)
    
    # Handle basic JSON-safe types
    if isinstance(data, (str, int, float, bool)):
        return data
    
    # Final fallback: convert to string
    logger.warning(f"Converting unknown type to string: {type(data)} = {data}")
    return str(data)

def debug_data_structure(data: Any, name: str = "data"):
    """
    Comprehensive debug analysis of a data structure.
    
    Args:
        data: The data structure to analyze
        name: Name for the data structure (for logging)
    """
    print(f"\n{'='*60}")
    print(f"DEBUG ANALYSIS: {name}")
    print(f"{'='*60}")
    
    print(f"\n1. BASIC INFO:")
    print(f"   Type: {type(data)}")
    print(f"   Length: {len(data) if hasattr(data, '__len__') else 'N/A'}")
    
    print(f"\n2. TYPE ANALYSIS:")
    types = deep_type_analysis(data, name)
    for type_info in types[:20]:  # Show first 20 for readability
        print(f"   {type_info}")
    if len(types) > 20:
        print(f"   ... and {len(types) - 20} more entries")
    
    print(f"\n3. JSON SERIALIZATION PROBLEMS:")
    problems = find_json_problematic_fields(data, name)
    if not problems:
        print("   ✓ No JSON serialization problems found!")
    else:
        print(f"   ✗ Found {len(problems)} problematic fields:")
        for problem in problems:
            print(f"     - {problem['path']}: {problem['reason']}")
            print(f"       Type: {problem['type']}")
            print(f"       Value: {problem['value'][:100]}...")
    
    print(f"\n4. STANDARD JSON TEST:")
    try:
        json.dumps(data)
        print("   ✓ Standard json.dumps() works!")
    except Exception as e:
        print(f"   ✗ Standard json.dumps() fails: {e}")
    
    print(f"\n5. ENHANCED CONVERSION TEST:")
    try:
        converted = enhanced_pandas_safe_converter(data)
        json.dumps(converted)
        print("   ✓ Enhanced conversion works!")
    except Exception as e:
        print(f"   ✗ Enhanced conversion fails: {e}")
    
    print(f"\n{'='*60}")
    return problems

if __name__ == "__main__":
    # Test with sample data that might contain Timestamps
    test_data = {
        'simple_string': 'hello',
        'simple_int': 42,
        'pandas_timestamp': pd.Timestamp('2023-01-15'),
        'pandas_nat': pd.NaT,
        'python_datetime': datetime(2023, 1, 15),
        'nested_dict': {
            'inner_timestamp': pd.Timestamp('2023-02-01'),
            'inner_list': [pd.Timestamp('2023-03-01'), 'string', 123]
        },
        'list_with_timestamps': [
            pd.Timestamp('2023-04-01'),
            'normal_string',
            {'nested': pd.Timestamp('2023-05-01')}
        ]
    }
    
    debug_data_structure(test_data, "test_data")
"""
JSON serialization utilities for handling pandas and other non-serializable types.

This module provides utilities to safely convert data structures containing
pandas types, datetime objects, and other non-JSON-serializable types to
JSON-serializable formats.
"""

import json
import logging
import pandas as pd
from datetime import datetime, date
from typing import Any, Dict, List
from decimal import Decimal

logger = logging.getLogger(__name__)


class PandasJSONEncoder(json.JSONEncoder):
    """
    Custom JSON encoder that handles pandas types and other non-serializable objects.
    
    This encoder converts:
    - pandas Timestamp -> ISO date string (YYYY-MM-DD)
    - pandas NaT/NaN -> None
    - datetime objects -> ISO date string
    - Decimal -> float
    - Other non-serializable -> string representation
    """
    
    def default(self, o):
        # Handle pandas Timestamp
        if isinstance(o, pd.Timestamp):
            if pd.isna(o):
                return None
            return o.strftime('%Y-%m-%d')

        # Handle pandas NaT and NaN
        if pd.isna(o):
            return None

        # Handle Python datetime objects
        if isinstance(o, (datetime, date)):
            return o.strftime('%Y-%m-%d')

        # Handle Decimal
        if isinstance(o, Decimal):
            return float(o)

        # For any other non-serializable type, convert to string
        try:
            # Try the default encoder first
            return super().default(o)
        except TypeError:
            logger.warning(f"Converting non-serializable object to string: {type(o)} = {o}")
            return str(o)


def make_json_serializable(data: Any) -> Any:
    """
    Recursively convert a data structure to be JSON-serializable.
    
    This function walks through nested dictionaries, lists, and other
    data structures to convert pandas types and other non-serializable
    objects to JSON-compatible types.
    
    Enhanced version with comprehensive pandas type handling and better
    error safety for complex data structures.
    
    Args:
        data: The data structure to convert
        
    Returns:
        JSON-serializable version of the data
        
    Examples:
        >>> make_json_serializable({'date': pd.Timestamp('2023-01-15')})
        {'date': '2023-01-15'}
        >>> make_json_serializable([pd.NaT, pd.Timestamp('2023-01-15')])
        [None, '2023-01-15']
    """
    if data is None:
        return None
    
    # Handle pandas types with comprehensive coverage and error safety
    if hasattr(pd, 'api') and hasattr(pd.api, 'types'):
        try:
            # Skip pandas array/sequence checking to avoid "truth value" errors
            if not (hasattr(data, '__len__') and not isinstance(data, str)):
                if pd.api.types.is_scalar(data):
                    if pd.isna(data):
                        return None
                    elif isinstance(data, pd.Timestamp):
                        return data.strftime('%Y-%m-%d')
                    elif isinstance(data, (pd.Timedelta, pd.Period)):
                        return str(data)
                    elif isinstance(data, pd.Categorical):
                        return str(data)
        except (ValueError, TypeError, AttributeError):
            # If pandas API fails, continue with fallback checks
            pass
    
    # Fallback pandas checks with error safety
    if isinstance(data, pd.Timestamp):
        try:
            if pd.isna(data):
                return None
        except (ValueError, TypeError):
            pass
        return data.strftime('%Y-%m-%d')
    
    # Safe pandas isna check for other types
    try:
        if hasattr(data, '__len__') and not isinstance(data, str):
            # Avoid checking isna on sequences/arrays
            pass
        elif pd.isna(data):
            return None
    except (ValueError, TypeError, AttributeError):
        # Not a pandas type or array issue
        pass
    
    # Handle Python datetime objects
    if isinstance(data, (datetime, date)):
        return data.strftime('%Y-%m-%d')
    
    # Handle Decimal
    if isinstance(data, Decimal):
        return float(data)
    
    # Handle dataclasses (convert to dict first)
    from dataclasses import is_dataclass, asdict
    if is_dataclass(data) and not isinstance(data, type):
        try:
            return make_json_serializable(asdict(data))
        except Exception:
            # If dataclass conversion fails, treat as regular object
            pass
    
    # Handle dictionaries recursively
    if isinstance(data, dict):
        result = {}
        for key, value in data.items():
            # Ensure keys are strings for JSON compatibility
            json_key = str(key) if not isinstance(key, str) else key
            result[json_key] = make_json_serializable(value)
        return result
    
    # Handle lists/tuples recursively
    if isinstance(data, (list, tuple)):
        return [make_json_serializable(item) for item in data]
    
    # Handle basic JSON-serializable types
    if isinstance(data, (str, int, float, bool)):
        return data
    
    # Handle objects with __dict__ (custom classes)
    if hasattr(data, '__dict__') and not isinstance(data, type):
        try:
            return make_json_serializable(data.__dict__)
        except Exception:
            # If __dict__ conversion fails, fall back to string
            pass
    
    # For any other type, convert to string as a fallback
    logger.warning(f"Converting non-serializable object to string: {type(data)} = {data}")
    return str(data)


def safe_json_dumps(data: Any, **kwargs) -> str:
    """
    Safely serialize data to JSON string, handling pandas types and other non-serializable objects.
    
    This function first converts the data to be JSON-serializable using make_json_serializable(),
    then uses the standard json.dumps() function.
    
    Args:
        data: The data to serialize
        **kwargs: Additional arguments passed to json.dumps()
        
    Returns:
        JSON string representation of the data
        
    Raises:
        ValueError: If the data cannot be serialized even after conversion
    """
    try:
        # First pass: make data JSON-serializable
        serializable_data = make_json_serializable(data)
        
        # Second pass: serialize with standard json.dumps
        return json.dumps(serializable_data, **kwargs)
    
    except Exception as e:
        # Fallback: use custom encoder
        logger.warning(f"Standard serialization failed, using custom encoder: {str(e)}")
        try:
            return json.dumps(data, cls=PandasJSONEncoder, **kwargs)
        except Exception as fallback_error:
            logger.error(f"JSON serialization failed completely: {str(fallback_error)}")
            raise ValueError(f"Cannot serialize data to JSON: {str(fallback_error)}")


def debug_non_serializable_fields(data: Any, path: str = "root") -> List[str]:
    """
    Debug utility to find non-JSON-serializable fields in a data structure.
    
    This function recursively walks through a data structure and identifies
    fields that cannot be serialized to JSON, returning their paths.
    
    Args:
        data: The data structure to analyze
        path: Current path in the data structure (for internal recursion)
        
    Returns:
        List of paths to non-serializable fields
        
    Examples:
        >>> debug_non_serializable_fields({'date': pd.Timestamp('2023-01-15')})
        ['root.date: <class 'pandas._libs.tslibs.timestamps.Timestamp'>']
    """
    problematic_fields = []
    
    def is_json_serializable(value):
        """Check if a single value is JSON-serializable."""
        try:
            json.dumps(value)
            return True
        except (TypeError, ValueError):
            return False
    
    def check_item(item, current_path):
        """Recursively check an item and its children."""
        if isinstance(item, dict):
            for key, value in item.items():
                new_path = f"{current_path}.{key}"
                if not is_json_serializable(value):
                    if isinstance(value, (dict, list, tuple)):
                        # Recurse into nested structures
                        check_item(value, new_path)
                    else:
                        # Found a problematic leaf value
                        problematic_fields.append(f"{new_path}: {type(value)}")
                        
        elif isinstance(item, (list, tuple)):
            for i, value in enumerate(item):
                new_path = f"{current_path}[{i}]"
                if not is_json_serializable(value):
                    if isinstance(value, (dict, list, tuple)):
                        # Recurse into nested structures
                        check_item(value, new_path)
                    else:
                        # Found a problematic leaf value
                        problematic_fields.append(f"{new_path}: {type(value)}")
        else:
            # Leaf value that's not JSON-serializable
            if not is_json_serializable(item):
                problematic_fields.append(f"{path}: {type(item)}")
    
    check_item(data, path)
    return problematic_fields


def log_serialization_debug(data: Any, context: str = "") -> None:
    """
    Log debug information about non-serializable fields in data.
    
    Enhanced version with more detailed analysis and error safety.
    
    Args:
        data: The data structure to debug
        context: Additional context for the log message
    """
    try:
        problematic_fields = debug_non_serializable_fields(data)
        if problematic_fields:
            logger.warning(f"Non-serializable fields found{' in ' + context if context else ''} ({len(problematic_fields)} issues):")
            for i, field in enumerate(problematic_fields[:10]):  # Limit to first 10 for readability
                logger.warning(f"  - {field}")
            if len(problematic_fields) > 10:
                logger.warning(f"  ... and {len(problematic_fields) - 10} more non-serializable fields")
        else:
            logger.debug(f"All fields are JSON-serializable{' in ' + context if context else ''}")
    except Exception as e:
        logger.error(f"Error during serialization debug{' for ' + context if context else ''}: {e}")


def deep_debug_serialization_issues(data: Any, name: str = "data") -> Dict[str, Any]:
    """
    Perform deep debugging of serialization issues with comprehensive analysis.

    Args:
        data: The data structure to analyze
        name: Name for the data structure (for logging)

    Returns:
        Dictionary with detailed analysis results
    """
    analysis: Dict[str, Any] = {
        'name': name,
        'type': str(type(data)),
        'is_json_safe': False,
        'problematic_fields': [],
        'pandas_timestamps_found': 0,
        'pandas_nat_found': 0,
        'python_datetimes_found': 0,
        'dataclass_objects_found': 0,
        'custom_objects_found': 0,
        'total_issues': 0
    }
    
    try:
        # Test if the data is JSON safe
        json.dumps(data)
        analysis['is_json_safe'] = True
    except (TypeError, ValueError):
        analysis['is_json_safe'] = False
    
    try:
        # Find all problematic fields
        problematic_fields = debug_non_serializable_fields(data)
        analysis['problematic_fields'] = problematic_fields
        analysis['total_issues'] = len(problematic_fields)
        
        # Count specific types of issues
        for field in problematic_fields:
            field_lower = field.lower()
            if 'pandas._libs.tslibs.timestamps.timestamp' in field_lower:
                analysis['pandas_timestamps_found'] += 1
            elif 'pandas._libs.tslibs.nattype.nattype' in field_lower or 'nat' in field_lower:
                analysis['pandas_nat_found'] += 1
            elif 'datetime.datetime' in field_lower:
                analysis['python_datetimes_found'] += 1
            elif 'dataclass' in field_lower:
                analysis['dataclass_objects_found'] += 1
            else:
                analysis['custom_objects_found'] += 1
                
    except Exception as e:
        logger.error(f"Error analyzing {name}: {e}")
        analysis['error'] = str(e)
    
    return analysis


def enhanced_safe_json_dumps(data: Any, context: str = "", **kwargs) -> str:
    """
    Enhanced safe JSON dumps with comprehensive debugging and error handling.
    
    This function provides detailed logging about what's being converted and why,
    making it easier to track down serialization issues in complex data structures.
    
    Args:
        data: The data to serialize
        context: Context description for debugging
        **kwargs: Additional arguments passed to json.dumps()
        
    Returns:
        JSON string representation of the data
        
    Raises:
        ValueError: If the data cannot be serialized even after conversion
    """
    # First, do a deep analysis if debugging is enabled
    if logger.isEnabledFor(logging.DEBUG):
        analysis = deep_debug_serialization_issues(data, context or "data")
        if analysis['total_issues'] > 0:
            logger.debug(f"Serialization analysis for {analysis['name']}:")
            logger.debug(f"  - Total issues: {analysis['total_issues']}")
            logger.debug(f"  - Pandas Timestamps: {analysis['pandas_timestamps_found']}")
            logger.debug(f"  - Pandas NaT: {analysis['pandas_nat_found']}")
            logger.debug(f"  - Python datetimes: {analysis['python_datetimes_found']}")
            logger.debug(f"  - Custom objects: {analysis['custom_objects_found']}")
    
    try:
        # First pass: make data JSON-serializable with enhanced converter
        logger.debug(f"Converting data to JSON-serializable format{' for ' + context if context else ''}")
        serializable_data = make_json_serializable(data)
        
        # Second pass: serialize with standard json.dumps
        logger.debug(f"Serializing converted data to JSON{' for ' + context if context else ''}")
        return json.dumps(serializable_data, **kwargs)
    
    except Exception as e:
        logger.warning(f"Enhanced serialization failed{' for ' + context if context else ''}, trying fallback encoder: {str(e)}")
        try:
            # Fallback: use custom encoder
            return json.dumps(data, cls=PandasJSONEncoder, **kwargs)
        except Exception as fallback_error:
            logger.error(f"JSON serialization failed completely{' for ' + context if context else ''}: {str(fallback_error)}")
            
            # Final debug attempt
            if logger.isEnabledFor(logging.ERROR):
                try:
                    analysis = deep_debug_serialization_issues(data, context or "failed_data")
                    logger.error(f"Final analysis: {analysis}")
                except Exception:
                    logger.error("Could not perform final analysis due to additional errors")
            
            raise ValueError(f"Cannot serialize data to JSON{' for ' + context if context else ''}: {str(fallback_error)}")
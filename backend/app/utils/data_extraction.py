"""
Data extraction utilities for handling mixed data types from CSV imports.

This module provides utilities to safely extract string values from data sources
that may contain various types including pandas Timestamps, strings, None/NaN values,
and numeric data.
"""

import pandas as pd
from typing import Any, Optional
from datetime import datetime


def safe_extract_string(value: Any, default: str = '') -> str:
    """
    Safely extract string value from mixed types (string, Timestamp, None, NaN).
    
    This utility function handles the common case where CSV data may contain
    various data types that need to be converted to clean string values.
    
    Args:
        value: The value to extract as a string. Can be:
            - pandas Timestamp objects (converted to YYYY-MM-DD format)
            - datetime objects (converted to YYYY-MM-DD format)  
            - strings (stripped of whitespace)
            - None/NaN values (converted to default)
            - numeric values (converted to string then stripped)
        default: Default value to return for None/NaN inputs
        
    Returns:
        Clean string value or default for invalid inputs
        
    Examples:
        >>> safe_extract_string("  hello  ")
        "hello"
        >>> safe_extract_string(pd.Timestamp("2023-01-15"))
        "2023-01-15"
        >>> safe_extract_string(None)
        ""
        >>> safe_extract_string(pd.NaT, "unknown")
        "unknown"
        >>> safe_extract_string(42.5)
        "42.5"
    """
    # Handle pandas NaN and None values
    if pd.isna(value) or value is None:
        return default
    
    # Handle pandas Timestamps
    if isinstance(value, pd.Timestamp):
        # Return empty string for invalid timestamps (NaT)
        if pd.isna(value):
            return default
        # Format as YYYY-MM-DD for date consistency
        return value.strftime('%Y-%m-%d')
    
    # Handle Python datetime objects
    if isinstance(value, datetime):
        return value.strftime('%Y-%m-%d')
        
    # Convert to string and strip whitespace
    str_value = str(value).strip()
    
    # Handle pandas 'nan' string representation
    if str_value.lower() == 'nan':
        return default
        
    return str_value


def safe_extract_numeric(value: Any, default: Optional[float] = None) -> Optional[float]:
    """
    Safely extract numeric value from mixed types.
    
    Args:
        value: The value to extract as a number
        default: Default value to return for invalid inputs
        
    Returns:
        Numeric value or default for invalid inputs
        
    Examples:
        >>> safe_extract_numeric("4.5")
        4.5
        >>> safe_extract_numeric("invalid")
        None
        >>> safe_extract_numeric(pd.NaT)
        None
    """
    if pd.isna(value) or value is None:
        return default
        
    if isinstance(value, (int, float)):
        return float(value)
        
    # Try to convert string to number
    try:
        return float(str(value).strip())
    except (ValueError, TypeError):
        return default


def safe_extract_date_string(value: Any, default: str = '') -> str:
    """
    Safely extract date as string from mixed types with better date handling.
    
    This is specialized for date fields that may need different formatting
    or parsing than the general string extraction.
    
    Args:
        value: The value to extract as a date string
        default: Default value to return for invalid inputs
        
    Returns:
        Date string in YYYY-MM-DD format or default
    """
    if pd.isna(value) or value is None:
        return default
    
    # Handle pandas Timestamps
    if isinstance(value, pd.Timestamp):
        if pd.isna(value):
            return default
        return value.strftime('%Y-%m-%d')
    
    # Handle Python datetime objects  
    if isinstance(value, datetime):
        return value.strftime('%Y-%m-%d')
    
    # For strings, try to parse and reformat for consistency
    str_value = str(value).strip()
    if not str_value or str_value.lower() == 'nan':
        return default
        
    # Try to parse various date formats and normalize to YYYY-MM-DD
    for date_format in ['%Y-%m-%d', '%m/%d/%Y', '%d/%m/%Y', '%Y-%m-%d %H:%M:%S']:
        try:
            parsed_date = datetime.strptime(str_value, date_format)
            return parsed_date.strftime('%Y-%m-%d')
        except ValueError:
            continue
    
    # If parsing fails, return the original string
    return str_value
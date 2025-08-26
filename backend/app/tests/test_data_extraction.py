"""
Tests for data extraction utilities.
"""

import pandas as pd
from datetime import datetime
from app.utils.data_extraction import (
    safe_extract_string,
    safe_extract_numeric,
    safe_extract_date_string
)


class TestSafeExtractString:
    """Test safe_extract_string function."""

    def test_extract_regular_strings(self):
        """Test extraction of regular string values."""
        assert safe_extract_string("hello") == "hello"
        assert safe_extract_string("  hello  ") == "hello"
        assert safe_extract_string("") == ""
        assert safe_extract_string("   ") == ""

    def test_extract_none_and_nan(self):
        """Test extraction of None and NaN values."""
        assert safe_extract_string(None) == ""
        assert safe_extract_string(None, "default") == "default"
        assert safe_extract_string(pd.NaT) == ""
        assert safe_extract_string(pd.NaT, "unknown") == "unknown"
        assert safe_extract_string(float('nan')) == ""

    def test_extract_pandas_timestamps(self):
        """Test extraction of pandas Timestamp objects."""
        timestamp = pd.Timestamp('2023-01-15')
        assert safe_extract_string(timestamp) == "2023-01-15"
        
        timestamp_with_time = pd.Timestamp('2023-01-15 14:30:00')
        assert safe_extract_string(timestamp_with_time) == "2023-01-15"
        
        # Test invalid timestamp (NaT)
        assert safe_extract_string(pd.NaT) == ""
        assert safe_extract_string(pd.NaT, "missing") == "missing"

    def test_extract_datetime_objects(self):
        """Test extraction of Python datetime objects."""
        dt = datetime(2023, 1, 15, 14, 30, 0)
        assert safe_extract_string(dt) == "2023-01-15"

    def test_extract_numeric_values(self):
        """Test extraction of numeric values."""
        assert safe_extract_string(42) == "42"
        assert safe_extract_string(42.5) == "42.5"
        assert safe_extract_string(0) == "0"

    def test_extract_nan_string(self):
        """Test extraction of 'nan' string representation."""
        assert safe_extract_string("nan") == ""
        assert safe_extract_string("NaN") == ""
        assert safe_extract_string("Nan") == ""
        assert safe_extract_string("nan", "missing") == "missing"


class TestSafeExtractNumeric:
    """Test safe_extract_numeric function."""

    def test_extract_numbers(self):
        """Test extraction of numeric values."""
        assert safe_extract_numeric(42) == 42.0
        assert safe_extract_numeric(42.5) == 42.5
        assert safe_extract_numeric("42.5") == 42.5
        assert safe_extract_numeric("  42.5  ") == 42.5

    def test_extract_invalid_numbers(self):
        """Test extraction of invalid numeric values."""
        assert safe_extract_numeric("invalid") is None
        assert safe_extract_numeric("") is None
        assert safe_extract_numeric(None) is None
        assert safe_extract_numeric(pd.NaT) is None

    def test_extract_with_default(self):
        """Test extraction with custom default values."""
        assert safe_extract_numeric("invalid", -1) == -1
        assert safe_extract_numeric(None, 0) == 0


class TestSafeExtractDateString:
    """Test safe_extract_date_string function."""

    def test_extract_timestamps(self):
        """Test extraction of pandas Timestamp objects."""
        timestamp = pd.Timestamp('2023-01-15')
        assert safe_extract_date_string(timestamp) == "2023-01-15"
        
        timestamp_with_time = pd.Timestamp('2023-01-15 14:30:00')
        assert safe_extract_date_string(timestamp_with_time) == "2023-01-15"

    def test_extract_datetime_objects(self):
        """Test extraction of Python datetime objects."""
        dt = datetime(2023, 1, 15, 14, 30, 0)
        assert safe_extract_date_string(dt) == "2023-01-15"

    def test_extract_date_strings(self):
        """Test extraction and normalization of date strings."""
        assert safe_extract_date_string("2023-01-15") == "2023-01-15"
        assert safe_extract_date_string("01/15/2023") == "2023-01-15"
        assert safe_extract_date_string("15/01/2023") == "2023-01-15"
        assert safe_extract_date_string("2023-01-15 14:30:00") == "2023-01-15"

    def test_extract_invalid_dates(self):
        """Test extraction of invalid date values."""
        assert safe_extract_date_string("invalid") == "invalid"  # Returns original string if unparseable
        assert safe_extract_date_string("") == ""
        assert safe_extract_date_string(None) == ""
        assert safe_extract_date_string(pd.NaT) == ""

    def test_extract_with_default(self):
        """Test extraction with custom default values."""
        assert safe_extract_date_string(None, "unknown") == "unknown"
        assert safe_extract_date_string(pd.NaT, "missing") == "missing"
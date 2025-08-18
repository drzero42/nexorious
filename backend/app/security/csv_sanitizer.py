"""
CSV Sanitizer for preventing injection attacks.

This module implements comprehensive CSV injection prevention following OWASP guidelines:
- Formula injection prevention (=, +, -, @, tab, carriage return prefixes)
- Script injection prevention (JavaScript, VBScript, HTML)
- DDE (Dynamic Data Exchange) attack prevention
- Control character sanitization

Reference:
- OWASP CSV Injection: https://owasp.org/www-community/attacks/CSV_Injection
- CWE-1236: Improper Neutralization of Formula Elements in CSV Files
"""

import re
import html
import logging
from typing import Any, List, Pattern, Dict, Optional
from dataclasses import dataclass, field

logger = logging.getLogger(__name__)

@dataclass
class SanitizationStats:
    """Track sanitization operations for audit logging and monitoring."""
    total_cells: int = 0
    formula_injections_prevented: int = 0
    script_injections_prevented: int = 0
    dde_injections_prevented: int = 0
    control_chars_removed: int = 0
    html_escaped: int = 0
    null_bytes_removed: int = 0
    
    def to_dict(self) -> Dict[str, int]:
        """Convert stats to dictionary for logging."""
        return {
            'total_cells': self.total_cells,
            'formula_injections_prevented': self.formula_injections_prevented,
            'script_injections_prevented': self.script_injections_prevented,
            'dde_injections_prevented': self.dde_injections_prevented,
            'control_chars_removed': self.control_chars_removed,
            'html_escaped': self.html_escaped,
            'null_bytes_removed': self.null_bytes_removed
        }


class CSVSanitizer:
    """
    Comprehensive CSV injection prevention following OWASP guidelines.
    
    This class provides static methods to sanitize CSV cell values against:
    - Formula injection attacks (Excel/LibreOffice formulas)
    - Script injection attacks (JavaScript, VBScript)
    - DDE (Dynamic Data Exchange) attacks
    - Control character injection
    - HTML/XML injection
    
    The sanitizer is designed to be conservative - it may over-sanitize 
    to prevent false negatives in security detection.
    """
    
    # Excel/LibreOffice/Google Sheets formula prefixes that can execute commands
    FORMULA_PREFIXES = ('=', '+', '-', '@', '\t', '\r')
    
    # Script injection patterns - matches various script execution contexts
    SCRIPT_PATTERNS: List[Pattern[str]] = [
        # HTML script tags with various attributes
        re.compile(r'<script[^>]*>.*?</script>', re.IGNORECASE | re.DOTALL),
        re.compile(r'<script[^>]*>', re.IGNORECASE),
        
        # JavaScript and VBScript URLs
        re.compile(r'javascript\s*:', re.IGNORECASE),
        re.compile(r'vbscript\s*:', re.IGNORECASE),
        
        # Data URLs that could contain HTML/JavaScript
        re.compile(r'data\s*:\s*text/html', re.IGNORECASE),
        re.compile(r'data\s*:\s*application/javascript', re.IGNORECASE),
        
        # Command injection patterns in spreadsheet formulas
        re.compile(r'cmd\s*\|', re.IGNORECASE),
        re.compile(r'\|\s*cmd', re.IGNORECASE),
        re.compile(r'powershell', re.IGNORECASE),
        re.compile(r'system\s*\(', re.IGNORECASE),
        
        # Other dangerous HTML elements
        re.compile(r'<iframe[^>]*>', re.IGNORECASE),
        re.compile(r'<embed[^>]*>', re.IGNORECASE),
        re.compile(r'<object[^>]*>', re.IGNORECASE),
        re.compile(r'<link[^>]*>', re.IGNORECASE),
        re.compile(r'<meta[^>]*>', re.IGNORECASE),
    ]
    
    # DDE (Dynamic Data Exchange) patterns - can execute system commands
    # Format: =cmd|'/c calc'!A1 or @SUM(cmd|'/c notepad'!A1:A2)
    DDE_PATTERNS: List[Pattern[str]] = [
        re.compile(r'=.*\|.*!', re.IGNORECASE),  # =cmd|'/c calc'!A1
        re.compile(r'@.*\|.*!', re.IGNORECASE),  # @SUM(cmd|'/c calc'!A1)
        re.compile(r'\+.*\|.*!', re.IGNORECASE), # +cmd|'/c calc'!A1
        re.compile(r'-.*\|.*!', re.IGNORECASE),  # -cmd|'/c calc'!A1
    ]
    
    # Control characters that should be removed (except safe whitespace)
    CONTROL_CHAR_PATTERN = re.compile(r'[\x00-\x08\x0B\x0C\x0E-\x1F\x7F-\x9F]')
    
    # Null byte patterns
    NULL_BYTE_PATTERN = re.compile(r'\x00')
    
    # Maximum cell content length to prevent memory exhaustion
    MAX_CELL_LENGTH = 10240  # 10KB per cell
    
    @staticmethod
    def sanitize_cell(value: Any, max_length: Optional[int] = None) -> str:
        """
        Sanitize a single cell value against all known injection attacks.
        
        Args:
            value: The cell value to sanitize (any type, will be converted to string)
            max_length: Maximum allowed length (defaults to MAX_CELL_LENGTH)
            
        Returns:
            Sanitized string value safe for CSV export and processing
            
        Examples:
            >>> CSVSanitizer.sanitize_cell("=1+1")
            "'=1+1"
            >>> CSVSanitizer.sanitize_cell("<script>alert('xss')</script>")
            "&lt;script&gt;alert('xss')&lt;/script&gt;"
            >>> CSVSanitizer.sanitize_cell("=cmd|'/c calc'!A1")
            "'=cmd|'/c calc'!A1"
        """
        # Handle None/NaN values
        if value is None or (hasattr(value, '__iter__') and len(str(value).strip()) == 0):
            return ""
            
        # Convert to string and strip whitespace
        str_value = str(value).strip()
        
        if not str_value:
            return ""
            
        # Truncate if too long (prevent memory exhaustion)
        if max_length is None:
            max_length = CSVSanitizer.MAX_CELL_LENGTH
            
        if len(str_value) > max_length:
            logger.warning(f"Cell value truncated from {len(str_value)} to {max_length} characters")
            str_value = str_value[:max_length]
        
        # 1. Remove null bytes (can bypass security filters)
        original_length = len(str_value)
        str_value = CSVSanitizer.NULL_BYTE_PATTERN.sub('', str_value)
        if len(str_value) != original_length:
            logger.warning("Removed null bytes from cell value")
        
        # 2. Check for and prevent formula injection
        if str_value and str_value[0] in CSVSanitizer.FORMULA_PREFIXES:
            # Check if it's just a number (positive or negative) - these are safe
            if str_value[0] in ['+', '-']:
                try:
                    float(str_value)
                    # It's a number, no need to escape
                except ValueError:
                    # Not a number, treat as dangerous formula
                    logger.info(f"Formula injection prevented: starts with '{str_value[0]}'")
                    str_value = "'" + str_value  # Prefix with quote to prevent execution
            else:
                # Other formula prefixes (=, @, tab, CR) are always dangerous
                logger.info(f"Formula injection prevented: starts with '{str_value[0]}'")
                str_value = "'" + str_value  # Prefix with quote to prevent execution
            
        # 3. Check for and prevent DDE injection patterns
        for pattern in CSVSanitizer.DDE_PATTERNS:
            if pattern.search(str_value):
                logger.warning("DDE injection pattern detected and prevented")
                str_value = "'" + str_value
                break
        
        # 4. Remove dangerous script patterns
        for pattern in CSVSanitizer.SCRIPT_PATTERNS:
            original_value = str_value
            str_value = pattern.sub('', str_value)
            if str_value != original_value:
                logger.warning("Script injection pattern detected and removed")
        
        # 5. Remove control characters (except safe whitespace: space, tab, newline, CR)
        original_length = len(str_value)
        str_value = CSVSanitizer.CONTROL_CHAR_PATTERN.sub('', str_value)
        if len(str_value) != original_length:
            logger.info("Control characters removed from cell value")
        
        # 6. HTML escape to prevent XSS in web contexts
        # This is the final step to ensure any remaining HTML is safe
        # Special handling: don't escape the leading quote if we added it for formula protection
        starts_with_protection_quote = str_value.startswith("'") and len(str_value) > 1
        
        if starts_with_protection_quote:
            # Escape everything except the leading protection quote
            escaped_content = html.escape(str_value[1:], quote=True)
            str_value = "'" + escaped_content
        else:
            # Normal HTML escaping
            str_value = html.escape(str_value, quote=True)
        
        return str_value
    
    @staticmethod
    def sanitize_row(row_data: Dict[str, Any], 
                     max_cell_length: Optional[int] = None) -> Dict[str, str]:
        """
        Sanitize all cells in a row of data.
        
        Args:
            row_data: Dictionary of column_name -> cell_value
            max_cell_length: Maximum allowed length per cell
            
        Returns:
            Dictionary with all values sanitized
            
        Example:
            >>> row = {"game": "Portal", "formula": "=1+1", "script": "<script>alert('xss')</script>"}
            >>> CSVSanitizer.sanitize_row(row)
            {"game": "Portal", "formula": "'=1+1", "script": "&lt;script&gt;alert('xss')&lt;/script&gt;"}
        """
        sanitized = {}
        
        for column, value in row_data.items():
            # Also sanitize column names to prevent header injection
            safe_column = CSVSanitizer.sanitize_cell(column, 100)  # Column names shorter
            sanitized[safe_column] = CSVSanitizer.sanitize_cell(value, max_cell_length)
            
        return sanitized
    
    @staticmethod
    def sanitize_csv_data(csv_data: List[Dict[str, Any]], 
                          max_cell_length: Optional[int] = None) -> tuple[List[Dict[str, str]], SanitizationStats]:
        """
        Sanitize an entire CSV dataset with statistics tracking.
        
        Args:
            csv_data: List of dictionaries representing CSV rows
            max_cell_length: Maximum allowed length per cell
            
        Returns:
            Tuple of (sanitized_data, sanitization_stats)
            
        Example:
            >>> data = [{"game": "Portal", "rating": "=1+1"}]
            >>> sanitized, stats = CSVSanitizer.sanitize_csv_data(data)
            >>> stats.formula_injections_prevented
            1
        """
        stats = SanitizationStats()
        sanitized_data = []
        
        for row_index, row in enumerate(csv_data):
            try:
                sanitized_row = {}
                
                for column, value in row.items():
                    original_value = str(value) if value is not None else ""
                    
                    # Sanitize column name
                    safe_column = CSVSanitizer.sanitize_cell(column, 100)
                    
                    # Sanitize cell value and track statistics
                    sanitized_value = CSVSanitizer.sanitize_cell(value, max_cell_length)
                    sanitized_row[safe_column] = sanitized_value
                    
                    # Update statistics
                    stats.total_cells += 1
                    
                    if original_value and len(original_value) > 0:
                        # Check what types of sanitization were performed
                        if original_value[0] in CSVSanitizer.FORMULA_PREFIXES:
                            stats.formula_injections_prevented += 1
                            
                        # Check for DDE patterns
                        for pattern in CSVSanitizer.DDE_PATTERNS:
                            if pattern.search(original_value):
                                stats.dde_injections_prevented += 1
                                break
                                
                        # Check for script patterns
                        for pattern in CSVSanitizer.SCRIPT_PATTERNS:
                            if pattern.search(original_value):
                                stats.script_injections_prevented += 1
                                break
                                
                        # Check for control characters
                        if CSVSanitizer.CONTROL_CHAR_PATTERN.search(original_value):
                            stats.control_chars_removed += 1
                            
                        # Check for null bytes
                        if CSVSanitizer.NULL_BYTE_PATTERN.search(original_value):
                            stats.null_bytes_removed += 1
                            
                        # Check if HTML escaping was needed
                        if html.escape(original_value, quote=True) != original_value:
                            stats.html_escaped += 1
                
                sanitized_data.append(sanitized_row)
                
            except Exception as e:
                logger.error(f"Error sanitizing row {row_index}: {e}")
                # Skip problematic rows rather than failing entirely
                continue
        
        # Log sanitization statistics
        if stats.total_cells > 0:
            logger.info(f"CSV sanitization completed: {stats.to_dict()}")
            
        return sanitized_data, stats
    
    @staticmethod
    def is_potentially_dangerous(value: Any) -> bool:
        """
        Check if a value contains potentially dangerous content without sanitizing.
        
        This is useful for validation and logging purposes.
        
        Args:
            value: The value to check
            
        Returns:
            True if the value contains potentially dangerous patterns
            
        Example:
            >>> CSVSanitizer.is_potentially_dangerous("=1+1")
            True
            >>> CSVSanitizer.is_potentially_dangerous("Portal")
            False
        """
        if value is None:
            return False
            
        str_value = str(value).strip()
        
        if not str_value:
            return False
        
        # Check for formula prefixes, but exclude simple numbers
        if str_value[0] in CSVSanitizer.FORMULA_PREFIXES:
            # Check if it's just a number (positive or negative)
            if str_value[0] in ['+', '-']:
                try:
                    # If it can be parsed as a number, it's safe
                    float(str_value)
                    return False
                except ValueError:
                    # Not a number, could be dangerous
                    return True
            else:
                # Other formula prefixes (=, @, tab, CR) are always dangerous
                return True
            
        # Check for DDE patterns
        for pattern in CSVSanitizer.DDE_PATTERNS:
            if pattern.search(str_value):
                return True
                
        # Check for script patterns
        for pattern in CSVSanitizer.SCRIPT_PATTERNS:
            if pattern.search(str_value):
                return True
                
        # Check for control characters or null bytes
        if (CSVSanitizer.CONTROL_CHAR_PATTERN.search(str_value) or 
            CSVSanitizer.NULL_BYTE_PATTERN.search(str_value)):
            return True
            
        return False
    
    @staticmethod
    def validate_csv_safety(csv_data: List[Dict[str, Any]], 
                           strict: bool = False) -> tuple[bool, List[str]]:
        """
        Validate CSV data for potential security issues without sanitizing.
        
        Args:
            csv_data: List of dictionaries representing CSV rows
            strict: If True, any potentially dangerous content fails validation
            
        Returns:
            Tuple of (is_safe, list_of_issues)
            
        Example:
            >>> data = [{"game": "Portal", "rating": "=1+1"}]
            >>> is_safe, issues = CSVSanitizer.validate_csv_safety(data, strict=True)
            >>> is_safe
            False
            >>> issues
            ['Formula injection detected in row 1, column: rating']
        """
        issues = []
        is_safe = True
        
        for row_index, row in enumerate(csv_data):
            for column, value in row.items():
                if CSVSanitizer.is_potentially_dangerous(value):
                    issue = f"Potentially dangerous content in row {row_index + 1}, column: {column}"
                    issues.append(issue)
                    
                    if strict:
                        is_safe = False
        
        return is_safe, issues
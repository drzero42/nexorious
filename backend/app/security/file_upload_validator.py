"""
Secure file upload validation for CSV imports.

This module implements comprehensive file upload security validation following OWASP guidelines:
- File size validation to prevent resource exhaustion
- MIME type validation to ensure only CSV files are accepted
- Filename sanitization to prevent path traversal attacks
- Content validation to detect malicious files
- Secure temporary file handling with proper permissions

Reference:
- OWASP File Upload Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html
- CWE-434: Unrestricted Upload of File with Dangerous Type
"""

import hashlib
import tempfile
import csv
import io
import re
import time
import uuid
import logging
from pathlib import Path
from typing import Optional, List, Tuple
from dataclasses import dataclass, field
from fastapi import UploadFile

logger = logging.getLogger(__name__)

@dataclass
class FileValidationResult:
    """Result of file validation with security details."""
    is_valid: bool
    file_path: Optional[Path] = None
    file_hash: Optional[str] = None
    file_size: int = 0
    mime_type: Optional[str] = None
    detected_encoding: str = 'utf-8'
    row_count: int = 0
    column_count: int = 0
    errors: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)


class SecureFileUploadValidator:
    """
    Comprehensive file upload security validator.
    
    Implements multiple layers of validation to protect against:
    - File size attacks (DoS via large files)
    - MIME type spoofing attacks
    - Path traversal attacks via filenames
    - Malicious file content
    - Encoding attacks
    
    All validations follow OWASP File Upload Cheat Sheet recommendations.
    """
    
    # Security limits
    MAX_FILE_SIZE = 10 * 1024 * 1024  # 10MB - prevent memory exhaustion
    MIN_FILE_SIZE = 10  # 10 bytes - must contain some content
    
    # Allowed file extensions and MIME types
    ALLOWED_EXTENSIONS = {'.csv'}
    ALLOWED_MIME_TYPES = {
        'text/csv', 
        'text/plain', 
        'application/csv',
        'text/comma-separated-values'
    }
    
    # Content-Type header validation (what browser/client claims)
    ALLOWED_CONTENT_TYPES = {
        'text/csv', 
        'text/plain', 
        'application/csv',
        'text/comma-separated-values'
    }
    
    # Filename security validation
    MAX_FILENAME_LENGTH = 255
    SAFE_FILENAME_PATTERN = re.compile(r'^[\w\-. ()]+$', re.UNICODE)
    
    # Dangerous filename patterns to reject
    DANGEROUS_FILENAME_PATTERNS = [
        # Path traversal
        re.compile(r'\.\.'),
        re.compile(r'/'),
        re.compile(r'\\'),
        re.compile(r'\x00'),  # Null byte
        
        # Executable extensions (even as part of filename)
        re.compile(r'\.exe', re.IGNORECASE),
        re.compile(r'\.bat', re.IGNORECASE),
        re.compile(r'\.cmd', re.IGNORECASE),
        re.compile(r'\.scr', re.IGNORECASE),
        re.compile(r'\.com', re.IGNORECASE),
        re.compile(r'\.pif', re.IGNORECASE),
        re.compile(r'\.js', re.IGNORECASE),
        re.compile(r'\.vbs', re.IGNORECASE),
        re.compile(r'\.jar', re.IGNORECASE),
        
        # Script injection in filenames
        re.compile(r'<script', re.IGNORECASE),
        re.compile(r'javascript:', re.IGNORECASE),
        re.compile(r'vbscript:', re.IGNORECASE),
    ]
    
    # CSV structure validation limits
    MAX_CSV_COLUMNS = 100  # Prevent memory exhaustion from wide CSVs
    MIN_CSV_COLUMNS = 2    # Must have at least some structure
    MAX_PREVIEW_ROWS = 10  # Only validate first 10 rows for performance
    
    # Encoding validation
    ALLOWED_ENCODINGS = ['utf-8', 'utf-8-sig', 'ascii', 'iso-8859-1']
    
    @staticmethod
    async def validate_upload(file: UploadFile, 
                            user_id: str,
                            temp_dir: Optional[Path] = None) -> FileValidationResult:
        """
        Comprehensive validation of uploaded file.
        
        Args:
            file: FastAPI UploadFile object
            user_id: ID of the user uploading (for audit logging)
            temp_dir: Optional custom temporary directory
            
        Returns:
            FileValidationResult with validation outcome and details
            
        Raises:
            HTTPException: For critical security violations that should be rejected immediately
        """
        result = FileValidationResult(is_valid=False)
        
        try:
            # 1. Validate file presence
            if not file or not file.filename:
                result.errors.append("No file provided")
                return result
            
            # 2. Validate filename security
            filename_validation = SecureFileUploadValidator._validate_filename(file.filename)
            if not filename_validation[0]:
                result.errors.extend(filename_validation[1])
                logger.warning(f"Dangerous filename rejected for user {user_id}: {file.filename}")
                return result
            
            # 3. Validate Content-Type header (what client claims)
            if file.content_type not in SecureFileUploadValidator.ALLOWED_CONTENT_TYPES:
                result.errors.append(f"Invalid content type: {file.content_type}. Only CSV files are allowed.")
                return result
            
            # 4. Read file content with size validation
            content = await file.read()
            result.file_size = len(content)
            
            # Size validation
            if result.file_size > SecureFileUploadValidator.MAX_FILE_SIZE:
                max_mb = SecureFileUploadValidator.MAX_FILE_SIZE / 1024 / 1024
                result.errors.append(f"File too large: {result.file_size} bytes. Maximum allowed: {max_mb}MB")
                return result
                
            if result.file_size < SecureFileUploadValidator.MIN_FILE_SIZE:
                result.errors.append("File too small or empty")
                return result
            
            # 5. Generate file hash for deduplication and integrity
            result.file_hash = hashlib.sha256(content).hexdigest()
            
            # 6. Validate encoding and detect charset
            encoding_result = SecureFileUploadValidator._detect_and_validate_encoding(content)
            if not encoding_result[0]:
                result.errors.extend(encoding_result[2])
                return result
            result.detected_encoding = encoding_result[1]
            
            # 7. Validate CSV structure
            csv_validation = SecureFileUploadValidator._validate_csv_structure(content, result.detected_encoding)
            if not csv_validation[0]:
                result.errors.extend(csv_validation[3])
                return result
            
            result.row_count = csv_validation[1]
            result.column_count = csv_validation[2]
            
            # 8. Create secure temporary file
            secure_temp_file = SecureFileUploadValidator._create_secure_temp_file(
                content, user_id, result.file_hash, temp_dir
            )
            result.file_path = secure_temp_file
            
            # If we get here, all validations passed
            result.is_valid = True
            
            logger.info(f"File upload validated successfully for user {user_id}: "
                       f"size={result.file_size}, rows={result.row_count}, cols={result.column_count}")
            
            return result
            
        except Exception as e:
            logger.error(f"File validation error for user {user_id}: {e}")
            result.errors.append("File validation failed due to internal error")
            return result
    
    @staticmethod
    def _validate_filename(filename: str) -> Tuple[bool, List[str]]:
        """
        Validate filename for security issues.
        
        Args:
            filename: The filename to validate
            
        Returns:
            Tuple of (is_valid, list_of_errors)
        """
        errors = []
        
        # Length check
        if len(filename) > SecureFileUploadValidator.MAX_FILENAME_LENGTH:
            errors.append(f"Filename too long: {len(filename)} characters. Maximum: {SecureFileUploadValidator.MAX_FILENAME_LENGTH}")
        
        # Extension check
        file_path = Path(filename)
        if file_path.suffix.lower() not in SecureFileUploadValidator.ALLOWED_EXTENSIONS:
            errors.append(f"Invalid file extension: {file_path.suffix}. Only .csv files are allowed")
        
        # Character whitelist check (allows Unicode letters/digits)
        if not SecureFileUploadValidator.SAFE_FILENAME_PATTERN.match(filename):
            errors.append("Filename contains invalid characters. Only Unicode letters, numbers, spaces, hyphens, underscores, dots, and parentheses are allowed")
        
        # Check for control characters explicitly (more dangerous than non-ASCII)
        if any(ord(c) < 32 for c in filename):
            errors.append("Filename contains control characters")
        
        # Dangerous pattern check
        for pattern in SecureFileUploadValidator.DANGEROUS_FILENAME_PATTERNS:
            if pattern.search(filename):
                errors.append(f"Filename contains dangerous pattern: {filename}")
                break
        
        return len(errors) == 0, errors
    
    @staticmethod
    def _detect_and_validate_encoding(content: bytes) -> Tuple[bool, str, List[str]]:
        """
        Detect and validate file encoding with content quality checks.
        
        Args:
            content: File content as bytes
            
        Returns:
            Tuple of (is_valid, detected_encoding, list_of_errors)
        """
        errors = []
        
        # Try UTF-8 first (most restrictive)
        try:
            decoded_text = content.decode('utf-8')
            if SecureFileUploadValidator._is_reasonable_csv_content(decoded_text):
                return True, 'utf-8', errors
        except UnicodeDecodeError:
            pass
        
        # Try other encodings with content validation
        for encoding in ['utf-8-sig', 'ascii']:
            try:
                decoded_text = content.decode(encoding)
                if SecureFileUploadValidator._is_reasonable_csv_content(decoded_text):
                    return True, encoding, errors
            except UnicodeDecodeError:
                continue
        
        # iso-8859-1 as last resort (with stricter validation)
        try:
            decoded_text = content.decode('iso-8859-1')
            if SecureFileUploadValidator._is_reasonable_csv_content(decoded_text, strict=True):
                return True, 'iso-8859-1', errors
            else:
                errors.append("File content appears to be corrupted or uses unsupported encoding")
        except UnicodeDecodeError:
            pass
        
        # If we get here, no valid encoding was found
        errors.append(f"File encoding not supported. Allowed encodings: {', '.join(SecureFileUploadValidator.ALLOWED_ENCODINGS)}")
        return False, 'utf-8', errors  # Default encoding for error case
    
    @staticmethod
    def _is_reasonable_csv_content(text: str, strict: bool = False) -> bool:
        """
        Check if decoded text looks like reasonable CSV content.
        
        Args:
            text: Decoded text content
            strict: If True, apply stricter validation (no control chars)
            
        Returns:
            True if content appears to be reasonable CSV data
        """
        if not text or len(text) < 5:  # Too short to be meaningful CSV
            return False
        
        # Check for excessive control characters (except common CSV ones)
        control_chars = sum(1 for c in text if ord(c) < 32 and c not in '\n\r\t')
        if strict and control_chars > 0:
            return False
        if control_chars > len(text) * 0.1:  # More than 10% control chars is suspicious
            return False
        
        # Check for suspicious high-byte patterns (common in corrupted/binary data)
        if strict:
            # Look for suspicious byte sequences that are common in binary data
            suspicious_chars = sum(1 for c in text if ord(c) in [0xff, 0xfe, 0xfd, 0xfc])
            if suspicious_chars > 0:
                return False
            
            # Check for excessive non-ASCII characters in strict mode
            non_ascii = sum(1 for c in text if ord(c) > 127)
            if non_ascii > len(text) * 0.3:  # More than 30% non-ASCII is suspicious
                return False
        
        # Check for common CSV indicators
        has_comma_or_separator = ',' in text or '\t' in text or ';' in text
        has_newlines = '\n' in text or '\r' in text
        
        # Content should look like structured data
        return has_comma_or_separator and has_newlines
    
    @staticmethod
    def _validate_csv_structure(content: bytes, encoding: str) -> Tuple[bool, int, int, List[str]]:
        """
        Validate CSV file structure and basic format.
        
        Args:
            content: File content as bytes
            encoding: Detected file encoding
            
        Returns:
            Tuple of (is_valid, row_count, column_count, list_of_errors)
        """
        errors = []
        row_count = 0
        column_count = 0
        
        try:
            # Decode content
            text_content = content.decode(encoding)
            
            # Parse CSV
            csv_reader = csv.reader(io.StringIO(text_content))
            
            # Read and validate header
            try:
                header = next(csv_reader)
                column_count = len(header)
                
                if column_count < SecureFileUploadValidator.MIN_CSV_COLUMNS:
                    errors.append(f"CSV must have at least {SecureFileUploadValidator.MIN_CSV_COLUMNS} columns")
                    return False, 0, 0, errors
                    
                if column_count > SecureFileUploadValidator.MAX_CSV_COLUMNS:
                    errors.append(f"CSV has too many columns: {column_count}. Maximum allowed: {SecureFileUploadValidator.MAX_CSV_COLUMNS}")
                    return False, 0, 0, errors
                
                # Validate header names
                for i, col_name in enumerate(header):
                    if not col_name or not col_name.strip():
                        errors.append(f"Empty column name at position {i + 1}")
                        return False, 0, 0, errors
                        
                    if len(col_name.strip()) > 100:  # Column names shouldn't be too long
                        errors.append(f"Column name too long at position {i + 1}: {len(col_name)} characters")
                        return False, 0, 0, errors
                
                row_count = 1  # Header counts as a row
                
            except StopIteration:
                errors.append("CSV file appears to be empty")
                return False, 0, 0, errors
            
            # Validate first few data rows for structure consistency
            preview_rows = 0
            for row in csv_reader:
                row_count += 1
                
                # Only validate structure for first few rows for performance
                if preview_rows < SecureFileUploadValidator.MAX_PREVIEW_ROWS:
                    if len(row) != column_count:
                        errors.append(f"Row {row_count} has {len(row)} columns, expected {column_count}")
                        return False, 0, 0, errors
                    preview_rows += 1
                # Continue counting all rows without structure validation
            
            # Ensure there's at least one data row
            if row_count < 2:
                errors.append("CSV file must contain at least one data row")
                return False, 0, 0, errors
            
            return True, row_count, column_count, errors
            
        except csv.Error as e:
            errors.append(f"Invalid CSV format: {str(e)}")
            return False, 0, 0, errors
        except Exception as e:
            errors.append(f"CSV validation error: {str(e)}")
            return False, 0, 0, errors
    
    @staticmethod
    def _create_secure_temp_file(content: bytes, 
                                user_id: str, 
                                file_hash: str,
                                temp_dir: Optional[Path] = None) -> Path:
        """
        Create a secure temporary file with proper permissions and naming.
        
        Args:
            content: File content to write
            user_id: User ID for audit trail
            file_hash: File hash for unique naming
            temp_dir: Optional custom temporary directory
            
        Returns:
            Path to the created temporary file
            
        Raises:
            RuntimeError: If temp file creation fails
        """
        try:
            # Create isolated temporary directory
            if temp_dir is None:
                temp_dir_path = Path(tempfile.mkdtemp(prefix=f"csv_import_{user_id}_", dir="/tmp"))
            else:
                temp_dir_path = temp_dir
                temp_dir_path.mkdir(parents=True, exist_ok=True)
            
            # Generate secure filename
            timestamp = int(time.time())
            unique_id = str(uuid.uuid4())[:8]
            safe_filename = f"darkadia_{timestamp}_{unique_id}_{file_hash[:16]}.csv"
            
            temp_file_path = temp_dir_path / safe_filename
            
            # Write file with restricted permissions
            temp_file_path.write_bytes(content)
            temp_file_path.chmod(0o600)  # Owner read/write only
            
            logger.info(f"Created secure temp file for user {user_id}: {temp_file_path}")
            
            return temp_file_path
            
        except Exception as e:
            logger.error(f"Failed to create secure temp file for user {user_id}: {e}")
            raise RuntimeError(f"Temporary file creation failed: {str(e)}")
    
    @staticmethod
    def cleanup_temp_file(file_path: Optional[Path]) -> bool:
        """
        Securely clean up a temporary file.
        
        Args:
            file_path: Path to the temporary file to delete
            
        Returns:
            True if cleanup was successful, False otherwise
        """
        if not file_path or not file_path.exists():
            return True
            
        try:
            # Delete the file
            file_path.unlink()
            
            # Try to remove the parent directory if it's empty
            try:
                file_path.parent.rmdir()
            except OSError:
                # Directory not empty or other issue, but that's ok
                pass
                
            logger.info(f"Cleaned up temp file: {file_path}")
            return True
            
        except Exception as e:
            logger.error(f"Failed to cleanup temp file {file_path}: {e}")
            return False
    
    @staticmethod
    def validate_file_permissions(file_path: Path) -> bool:
        """
        Validate that a temporary file has secure permissions.
        
        Args:
            file_path: Path to validate
            
        Returns:
            True if permissions are secure
        """
        try:
            stat = file_path.stat()
            # Check that only owner can read/write (octal 600)
            return (stat.st_mode & 0o777) == 0o600
        except Exception as e:
            logger.error(f"Failed to check file permissions for {file_path}: {e}")
            return False
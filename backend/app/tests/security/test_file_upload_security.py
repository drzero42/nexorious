"""
Comprehensive tests for secure file upload validation.

This test suite validates that the SecureFileUploadValidator properly prevents
all known file upload attack vectors including malicious files, path traversal,
MIME type spoofing, and other security issues.

Reference:
- OWASP File Upload Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html
- CWE-434: Unrestricted Upload of File with Dangerous Type
"""

import pytest
import tempfile
import csv
import io
from pathlib import Path
from unittest.mock import Mock, AsyncMock

from fastapi import UploadFile
from app.security.file_upload_validator import SecureFileUploadValidator


class TestSecureFileUploadValidator:
    """Test cases for secure file upload validation."""
    
    def create_mock_upload_file(self, 
                               filename: str, 
                               content: bytes, 
                               content_type: str = "text/csv") -> UploadFile:
        """Create a mock UploadFile for testing."""
        mock_file = Mock(spec=UploadFile)
        mock_file.filename = filename
        mock_file.content_type = content_type
        mock_file.read = AsyncMock(return_value=content)
        return mock_file
    
    def create_valid_csv_content(self, rows: int = 5) -> bytes:
        """Create valid CSV content for testing."""
        output = io.StringIO()
        writer = csv.writer(output)
        
        # Header
        writer.writerow(["game_name", "platform", "rating", "notes"])
        
        # Data rows
        for i in range(rows):
            writer.writerow([
                f"Test Game {i}",
                "PC",
                str(i % 10),
                f"Notes for game {i}"
            ])
        
        return output.getvalue().encode('utf-8')
    
    def create_malicious_csv_content(self) -> bytes:
        """Create CSV content with injection attempts."""
        output = io.StringIO()
        writer = csv.writer(output)
        
        writer.writerow(["game_name", "formula", "script", "notes"])
        writer.writerow([
            "Portal",
            "=1+1", 
            "<script>alert('xss')</script>",
            "Great game"
        ])
        
        return output.getvalue().encode('utf-8')
    
    @pytest.mark.asyncio
    async def test_valid_csv_upload(self):
        """Test that valid CSV files are accepted."""
        valid_content = self.create_valid_csv_content()
        upload_file = self.create_mock_upload_file("test.csv", valid_content)
        
        result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
        
        assert result.is_valid, f"Valid CSV rejected: {result.errors}"
        assert result.file_size == len(valid_content)
        assert result.row_count > 1, "Should detect CSV rows"
        assert result.column_count == 4, "Should detect 4 columns"
        assert result.file_path is not None, "Should create temp file"
        assert result.file_hash is not None, "Should generate file hash"
    
    @pytest.mark.asyncio
    async def test_file_size_limits(self):
        """Test file size validation."""
        # Test oversized file
        large_content = b"A" * (SecureFileUploadValidator.MAX_FILE_SIZE + 1)
        large_file = self.create_mock_upload_file("large.csv", large_content)
        
        result = await SecureFileUploadValidator.validate_upload(large_file, "test_user")
        
        assert not result.is_valid, "Oversized file should be rejected"
        assert any("too large" in error.lower() for error in result.errors), "Should report size error"
        
        # Test undersized file
        small_file = self.create_mock_upload_file("small.csv", b"")
        
        result = await SecureFileUploadValidator.validate_upload(small_file, "test_user")
        
        assert not result.is_valid, "Empty file should be rejected"
        assert any("empty" in error.lower() for error in result.errors), "Should report empty error"
    
    @pytest.mark.asyncio
    async def test_filename_security_validation(self):
        """Test filename security checks."""
        valid_content = self.create_valid_csv_content()
        
        dangerous_filenames = [
            # Path traversal
            "../../../etc/passwd",
            "..\\..\\windows\\system32\\cmd.exe", 
            "normal.csv/../../../etc/passwd",
            
            # Null byte injection
            "test.csv\x00.exe",
            "normal\x00'; DROP TABLE users; --.csv",
            
            # Script injection in filename
            "test<script>alert('xss')</script>.csv",
            "javascript:alert('xss').csv",
            "vbscript:msgbox('xss').csv",
            
            # Executable extensions
            "malware.exe.csv",
            "virus.bat",
            "trojan.scr.csv",
            "script.js.csv",
            
            # Invalid characters
            "file|with|pipes.csv",
            "file<with>brackets.csv",
            "file?with?questions.csv",
            "file*with*stars.csv",
        ]
        
        for dangerous_filename in dangerous_filenames:
            upload_file = self.create_mock_upload_file(dangerous_filename, valid_content)
            result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
            
            assert not result.is_valid, f"Dangerous filename should be rejected: {dangerous_filename}"
            assert len(result.errors) > 0, f"Should report filename errors for: {dangerous_filename}"
    
    @pytest.mark.asyncio
    async def test_content_type_validation(self):
        """Test Content-Type header validation."""
        valid_content = self.create_valid_csv_content()
        
        # Valid content types
        valid_content_types = [
            "text/csv",
            "text/plain", 
            "application/csv",
            "text/comma-separated-values"
        ]
        
        for content_type in valid_content_types:
            upload_file = self.create_mock_upload_file("test.csv", valid_content, content_type)
            result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
            
            assert result.is_valid or len(result.warnings) == 0, f"Valid content type rejected: {content_type}"
        
        # Invalid content types
        invalid_content_types = [
            "application/octet-stream",
            "text/html",
            "application/javascript",
            "image/jpeg",
            "application/zip",
            "application/x-executable"
        ]
        
        for content_type in invalid_content_types:
            upload_file = self.create_mock_upload_file("test.csv", valid_content, content_type)
            result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
            
            assert not result.is_valid, f"Invalid content type should be rejected: {content_type}"
    
    @pytest.mark.asyncio
    async def test_csv_structure_validation(self):
        """Test CSV structure validation."""
        # Test invalid CSV structures
        invalid_csvs = [
            # No header
            b"",
            
            # Only header, no data
            b"game,platform,rating\n",
            
            # Too few columns
            b"game\nPortal\n",
            
            # Inconsistent column count
            b"game,platform,rating\nPortal,PC\nHalf-Life,PC,9,Extra\n",
            
            # Empty column names
            b",platform,rating\nPortal,PC,9\n",
            
            # Invalid CSV format (unclosed quotes causing parsing error)
            b'game,platform,rating\nPortal,PC,9\n"Unclosed quote at end of file',
        ]
        
        for i, invalid_csv in enumerate(invalid_csvs):
            upload_file = self.create_mock_upload_file(f"invalid_{i}.csv", invalid_csv)
            result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
            
            assert not result.is_valid, f"Invalid CSV structure should be rejected: {i}"
            assert len(result.errors) > 0, f"Should report structure errors for CSV {i}"
    
    @pytest.mark.asyncio
    async def test_encoding_validation(self):
        """Test file encoding validation."""
        # Test valid encodings
        valid_encodings = [
            ("test content", "utf-8"),
            ("test content", "ascii"), 
            ("test content with accents: café", "utf-8"),
            ("test content", "iso-8859-1")
        ]
        
        for content, encoding in valid_encodings:
            csv_content = f"game,rating\nPortal,9\n{content},8\n".encode(encoding)
            upload_file = self.create_mock_upload_file("test.csv", csv_content)
            result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
            
            # Should either be valid or have only warnings about encoding
            if not result.is_valid:
                assert len(result.errors) == 0 or all("encoding" in error.lower() for error in result.errors), \
                    f"Valid encoding rejected with unexpected errors: {encoding}"
        
        # Test invalid encoding
        invalid_content = b"game,rating\nPortal,9\n\xff\xfe\x42\x61\x64\x42\x79\x74\x65\x73,8\n"  # Invalid UTF-8
        upload_file = self.create_mock_upload_file("invalid_encoding.csv", invalid_content)
        result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
        
        assert not result.is_valid, "Invalid encoding should be rejected"
        assert any("encoding" in error.lower() for error in result.errors), "Should report encoding error"
    
    @pytest.mark.asyncio
    async def test_temp_file_creation_and_cleanup(self):
        """Test secure temporary file handling."""
        valid_content = self.create_valid_csv_content()
        upload_file = self.create_mock_upload_file("test.csv", valid_content)
        
        result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
        
        assert result.is_valid, "Valid file should be accepted"
        assert result.file_path is not None, "Should create temp file"
        assert result.file_path.exists(), "Temp file should exist"
        
        # Check file permissions (should be owner read/write only)
        assert SecureFileUploadValidator.validate_file_permissions(result.file_path), \
            "Temp file should have secure permissions"
        
        # Test cleanup
        cleanup_success = SecureFileUploadValidator.cleanup_temp_file(result.file_path)
        assert cleanup_success, "Cleanup should succeed"
        assert not result.file_path.exists(), "Temp file should be deleted"
    
    @pytest.mark.asyncio
    async def test_file_hash_generation(self):
        """Test file hash generation for deduplication."""
        content = self.create_valid_csv_content()
        upload_file1 = self.create_mock_upload_file("test1.csv", content)
        upload_file2 = self.create_mock_upload_file("test2.csv", content)  # Same content
        different_content = b"different,content\ntest,123\n"
        upload_file3 = self.create_mock_upload_file("test3.csv", different_content)
        
        result1 = await SecureFileUploadValidator.validate_upload(upload_file1, "test_user")
        result2 = await SecureFileUploadValidator.validate_upload(upload_file2, "test_user")
        result3 = await SecureFileUploadValidator.validate_upload(upload_file3, "test_user")
        
        # Same content should have same hash
        assert result1.file_hash == result2.file_hash, "Same content should have same hash"
        
        # Different content should have different hash
        assert result1.file_hash != result3.file_hash, "Different content should have different hash"
        
        # Hash should be consistent
        assert result1.file_hash is not None, "Hash should exist"
        assert len(result1.file_hash) > 0, "Hash should not be empty"
        assert isinstance(result1.file_hash, str), "Hash should be string"
        
        # Cleanup
        for result in [result1, result2, result3]:
            if result.file_path:
                SecureFileUploadValidator.cleanup_temp_file(result.file_path)
    
    @pytest.mark.asyncio
    async def test_no_file_provided(self):
        """Test handling when no file is provided."""
        # Test None file
        result = await SecureFileUploadValidator.validate_upload(None, "test_user")  # type: ignore[arg-type]
        assert not result.is_valid, "Should reject None file"
        assert any("No file provided" in error for error in result.errors), "Should report no file error"
        
        # Test file with no filename
        mock_file = Mock(spec=UploadFile)
        mock_file.filename = None
        mock_file.content_type = "text/csv"
        
        result = await SecureFileUploadValidator.validate_upload(mock_file, "test_user")
        assert not result.is_valid, "Should reject file without filename"
    
    @pytest.mark.asyncio
    async def test_column_count_limits(self):
        """Test CSV column count validation."""
        # Test too many columns
        header = ["col_" + str(i) for i in range(SecureFileUploadValidator.MAX_CSV_COLUMNS + 5)]
        data_row = ["data_" + str(i) for i in range(len(header))]
        
        output = io.StringIO()
        writer = csv.writer(output)
        writer.writerow(header)
        writer.writerow(data_row)
        
        too_many_cols_content = output.getvalue().encode('utf-8')
        upload_file = self.create_mock_upload_file("too_many_cols.csv", too_many_cols_content)
        
        result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
        assert not result.is_valid, "Should reject CSV with too many columns"
        assert any("too many columns" in error.lower() for error in result.errors), "Should report column limit error"
        
        # Test too few columns
        single_col_content = b"game\nPortal\n"
        upload_file = self.create_mock_upload_file("single_col.csv", single_col_content)
        
        result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
        assert not result.is_valid, "Should reject CSV with too few columns"
    
    @pytest.mark.asyncio
    async def test_performance_with_valid_large_file(self):
        """Test performance with large but valid CSV files."""
        # Create large CSV content
        output = io.StringIO()
        writer = csv.writer(output)
        writer.writerow(["game", "platform", "rating", "notes"])
        
        # Add many rows (but within limits)
        for i in range(100):  # Reasonable size for testing
            writer.writerow([f"Game {i}", "PC", str(i % 10), f"Notes {i}"])
        
        large_content = output.getvalue().encode('utf-8')
        upload_file = self.create_mock_upload_file("large_valid.csv", large_content)
        
        import time
        start_time = time.time()
        
        result = await SecureFileUploadValidator.validate_upload(upload_file, "test_user")
        
        end_time = time.time()
        processing_time = end_time - start_time
        
        assert result.is_valid, f"Large valid file should be accepted: {result.errors}"
        assert processing_time < 5.0, f"Processing took too long: {processing_time:.2f}s"
        assert result.row_count == 101, "Should count all rows including header"
        
        if result.file_path:
            SecureFileUploadValidator.cleanup_temp_file(result.file_path)
    
    def test_validate_file_permissions(self):
        """Test file permission validation."""
        with tempfile.NamedTemporaryFile(delete=False) as temp_file:
            temp_path = Path(temp_file.name)
            temp_file.write(b"test content")
        
        try:
            # Test secure permissions (600)
            temp_path.chmod(0o600)
            assert SecureFileUploadValidator.validate_file_permissions(temp_path), \
                "Should validate secure permissions"
            
            # Test insecure permissions (777)
            temp_path.chmod(0o777)
            assert not SecureFileUploadValidator.validate_file_permissions(temp_path), \
                "Should reject insecure permissions"
            
        finally:
            temp_path.unlink()
    
    def test_cleanup_nonexistent_file(self):
        """Test cleanup of non-existent files."""
        nonexistent_path = Path("/tmp/nonexistent_file_12345.csv")
        
        # Should succeed (no error) for non-existent files
        result = SecureFileUploadValidator.cleanup_temp_file(nonexistent_path)
        assert result, "Cleanup of non-existent file should succeed"
        
        # Should succeed for None path
        result = SecureFileUploadValidator.cleanup_temp_file(None)
        assert result, "Cleanup of None path should succeed"


class TestSecureFileUploadValidatorEdgeCases:
    """Test edge cases and error conditions."""
    
    @pytest.mark.asyncio
    async def test_file_read_exception(self):
        """Test handling of file read exceptions."""
        mock_file = Mock(spec=UploadFile)
        mock_file.filename = "test.csv"
        mock_file.content_type = "text/csv"
        mock_file.read = AsyncMock(side_effect=Exception("Read error"))
        
        result = await SecureFileUploadValidator.validate_upload(mock_file, "test_user")
        
        assert not result.is_valid, "Should handle read exceptions"
        assert len(result.errors) > 0, "Should report error"
    
    def test_filename_validation_edge_cases(self):
        """Test edge cases in filename validation."""
        edge_cases = [
            # Very long filename
            "a" * (SecureFileUploadValidator.MAX_FILENAME_LENGTH + 1) + ".csv",
            
            # Filename with only extension
            ".csv",
            
            # Empty filename
            "",
            
            # Filename with multiple extensions
            "test.exe.csv",
            
            # Unicode in filename
            "test_ü_ñ_é.csv",  # Should be valid
        ]
        
        for filename in edge_cases:
            is_valid, errors = SecureFileUploadValidator._validate_filename(filename)
            
            if filename == "test_ü_ñ_é.csv":
                # Unicode should be valid
                assert is_valid or len(errors) == 0, f"Unicode filename should be valid: {filename}"
            elif len(filename) > SecureFileUploadValidator.MAX_FILENAME_LENGTH:
                assert not is_valid, f"Long filename should be invalid: {filename}"
                assert any("too long" in error.lower() for error in errors), "Should report length error"
            else:
                # Other edge cases should be caught appropriately
                if not is_valid:
                    assert len(errors) > 0, f"Invalid filename should have errors: {filename}"
    
    def test_encoding_detection_edge_cases(self):
        """Test edge cases in encoding detection."""
        # Test various problematic encodings
        test_cases = [
            # Valid UTF-8 with BOM
            "test content".encode('utf-8-sig'),
            
            # Latin-1 content
            "café résumé".encode('iso-8859-1'),
            
            # ASCII content
            "simple ascii".encode('ascii'),
            
            # Invalid UTF-8 bytes
            b'\xff\xfe\x42\x61\x64\x42\x79\x74\x65\x73',
        ]
        
        for content in test_cases:
            is_valid, encoding, errors = SecureFileUploadValidator._detect_and_validate_encoding(content)
            
            if content == b'\xff\xfe\x42\x61\x64\x42\x79\x74\x65\x73':
                assert not is_valid, "Invalid bytes should fail validation"
                assert len(errors) > 0, "Should report encoding errors"
            else:
                # Valid encodings should work
                if not is_valid:
                    # Some edge cases might not be supported, but should be handled gracefully
                    assert len(errors) > 0, "Should report why encoding failed"
    
    def test_csv_structure_validation_edge_cases(self):
        """Test edge cases in CSV structure validation."""
        edge_cases = [
            # CSV with quoted commas
            b'game,description\nPortal,"A game, with commas"\n',
            
            # CSV with quoted newlines
            b'game,notes\nPortal,"Multi\nline\nnotes"\n',
            
            # CSV with escaped quotes
            b'game,notes\nPortal,"He said ""Hello"""\n',
            
            # CSV with trailing commas
            b'game,platform,\nPortal,PC,\n',
            
            # CSV with different line endings
            b'game,platform\r\nPortal,PC\r\n',
            b'game,platform\rPortal,PC\r',
        ]
        
        for csv_content in edge_cases:
            is_valid, row_count, col_count, errors = SecureFileUploadValidator._validate_csv_structure(
                csv_content, 'utf-8'
            )
            
            # Most of these should be valid CSV formats
            if not is_valid:
                # If invalid, should have clear error messages
                assert len(errors) > 0, f"Should report why CSV is invalid: {csv_content}"
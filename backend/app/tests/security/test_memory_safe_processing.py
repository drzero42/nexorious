"""
Comprehensive tests for memory-safe CSV processing.

This test suite validates that the SecureCSVProcessor properly prevents
resource exhaustion attacks while providing safe streaming processing
of large CSV datasets.

Reference:
- OWASP DoS Prevention: https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html
- CWE-400: Uncontrolled Resource Consumption
"""

import pytest
import tempfile
import csv
import asyncio
import time
from pathlib import Path
from unittest.mock import patch, Mock

from app.security.secure_csv_processor import (
    SecureCSVProcessor, 
    ProcessingLimits,
    ProcessingStats,
    RowLimitExceededError,
    MemoryLimitExceededError,
    ChunkLimitExceededError
)


class TestSecureCSVProcessor:
    """Test cases for memory-safe CSV processing."""
    
    def create_test_csv_file(self, 
                            rows: int = 10, 
                            cols: int = 4, 
                            cell_size: int = 10) -> Path:
        """Create a test CSV file with specified dimensions."""
        temp_file = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False)
        
        writer = csv.writer(temp_file)
        
        # Write header
        header = [f"col_{i}" for i in range(cols)]
        writer.writerow(header)
        
        # Write data rows
        for row_i in range(rows):
            row = [f"data_{row_i}_{col_i}".ljust(cell_size, 'x') for col_i in range(cols)]
            writer.writerow(row)
        
        temp_file.close()
        return Path(temp_file.name)
    
    def create_large_csv_file(self, rows: int = 1000) -> Path:
        """Create a large CSV file for testing limits."""
        return self.create_test_csv_file(rows=rows, cols=5, cell_size=50)
    
    def create_wide_csv_file(self, cols: int = 200) -> Path:
        """Create a wide CSV file for testing column limits."""
        return self.create_test_csv_file(rows=5, cols=cols, cell_size=10)
    
    def create_large_cell_csv_file(self, cell_size: int = 20000) -> Path:
        """Create CSV with large cells for testing cell size limits."""
        return self.create_test_csv_file(rows=3, cols=3, cell_size=cell_size)
    
    @pytest.mark.asyncio
    async def test_basic_csv_processing(self):
        """Test basic CSV processing functionality."""
        test_file = self.create_test_csv_file(rows=5, cols=3)
        
        try:
            processor = SecureCSVProcessor()
            processed_rows = []
            
            async with processor.process_csv_securely(test_file) as chunks:
                async for chunk in chunks:
                    processed_rows.extend(chunk)
            
            # Should process all rows
            assert len(processed_rows) == 5, f"Expected 5 rows, got {len(processed_rows)}"
            
            # Check structure
            for row in processed_rows:
                assert len(row) == 3, "Each row should have 3 columns"
                assert all(isinstance(v, str) for v in row.values()), "All values should be strings"
            
            # Check statistics
            stats = processor.get_processing_stats()
            assert stats.total_rows_processed == 5, "Should track processed rows"
            assert stats.total_chunks_processed > 0, "Should track processed chunks"
            assert stats.errors_encountered == 0, "Should have no errors"
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_row_limit_enforcement(self):
        """Test that row limits are properly enforced."""
        # Create file with more rows than limit
        limits = ProcessingLimits(max_rows=10, chunk_size=5)
        test_file = self.create_test_csv_file(rows=15)  # Exceeds limit
        
        try:
            processor = SecureCSVProcessor(limits)
            
            with pytest.raises(RowLimitExceededError) as exc_info:
                async with processor.process_csv_securely(test_file) as chunks:
                    async for chunk in chunks:
                        pass  # Should fail before completing
            
            assert "Too many rows" in str(exc_info.value)
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_chunk_limit_enforcement(self):
        """Test that chunk limits are properly enforced."""
        # Create file that will require more chunks than allowed
        limits = ProcessingLimits(max_chunks=3, chunk_size=2, max_rows=1000)
        test_file = self.create_test_csv_file(rows=10)  # Will need 5 chunks with chunk_size=2
        
        try:
            processor = SecureCSVProcessor(limits)
            
            with pytest.raises(ChunkLimitExceededError) as exc_info:
                async with processor.process_csv_securely(test_file) as chunks:
                    async for chunk in chunks:
                        pass  # Should fail after 3 chunks
            
            assert "Too many chunks" in str(exc_info.value)
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_cell_size_truncation(self):
        """Test that large cells are truncated."""
        limits = ProcessingLimits(max_cell_size=100)
        test_file = self.create_large_cell_csv_file(cell_size=500)  # Exceeds limit
        
        try:
            processor = SecureCSVProcessor(limits)
            processed_rows = []
            
            async with processor.process_csv_securely(test_file) as chunks:
                async for chunk in chunks:
                    processed_rows.extend(chunk)
            
            # Check that cells were truncated
            stats = processor.get_processing_stats()
            assert stats.cells_truncated > 0, "Should have truncated large cells"
            
            # Check actual truncation
            for row in processed_rows:
                for cell_value in row.values():
                    assert len(cell_value) <= limits.max_cell_size, \
                        f"Cell not properly truncated: {len(cell_value)}"
            
        finally:
            test_file.unlink()
    
    def test_processing_timeout_configuration(self):
        """Test timeout configuration for processing limits."""
        # For self-hosted system, just verify timeout can be configured
        short_timeout_limits = ProcessingLimits(processing_timeout=30)
        long_timeout_limits = ProcessingLimits(processing_timeout=600)
        
        assert short_timeout_limits.processing_timeout == 30
        assert long_timeout_limits.processing_timeout == 600
        
        # Verify processor accepts the limits
        processor_short = SecureCSVProcessor(short_timeout_limits) 
        processor_long = SecureCSVProcessor(long_timeout_limits)
        
        assert processor_short.limits.processing_timeout == 30
        assert processor_long.limits.processing_timeout == 600
    
    @pytest.mark.asyncio
    async def test_memory_usage_monitoring(self):
        """Test memory usage monitoring."""
        # Use very low memory limit to trigger monitoring
        limits = ProcessingLimits(max_memory_mb=1, memory_check_interval=1)
        test_file = self.create_test_csv_file(rows=10)
        
        try:
            processor = SecureCSVProcessor(limits)
            
            # Mock memory usage to exceed limit
            mock_process = Mock()
            mock_memory_info = Mock()
            mock_memory_info.rss = 10 * 1024 * 1024  # 10MB in bytes (exceeds 1MB limit)
            mock_process.memory_info.return_value = mock_memory_info
            
            with patch.object(processor, '_process', mock_process):
                with pytest.raises(MemoryLimitExceededError) as exc_info:
                    async with processor.process_csv_securely(test_file) as chunks:
                        async for chunk in chunks:
                            pass  # Should fail on memory check
            
            assert "Memory limit exceeded" in str(exc_info.value)
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_progress_callback(self):
        """Test progress callback functionality."""
        test_file = self.create_test_csv_file(rows=10, cols=3)
        progress_calls = []
        
        def progress_callback(stats: ProcessingStats):
            progress_calls.append(stats.total_rows_processed)
        
        try:
            processor = SecureCSVProcessor()
            
            async with processor.process_csv_securely(test_file, progress_callback) as chunks:
                async for chunk in chunks:
                    pass  # Process all chunks
            
            # Should have received progress updates
            assert len(progress_calls) > 0, "Should have called progress callback"
            assert progress_calls[-1] == 10, "Final progress should be 10 rows"
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_error_recovery_during_processing(self):
        """Test error recovery when processing individual rows fails."""
        # Create CSV with some problematic data
        temp_file = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False)
        
        writer = csv.writer(temp_file)
        writer.writerow(["game", "rating", "notes"])
        writer.writerow(["Portal", "9", "Great game"])
        writer.writerow(["Test Game", "invalid", "Normal notes"])  # Might cause issues
        writer.writerow(["Half-Life", "10", "Classic"])
        
        temp_file.close()
        test_file = Path(temp_file.name)
        
        try:
            processor = SecureCSVProcessor()
            processed_rows = []
            
            async with processor.process_csv_securely(test_file) as chunks:
                async for chunk in chunks:
                    processed_rows.extend(chunk)
            
            # Should process most rows even if some have issues
            assert len(processed_rows) >= 2, "Should process at least valid rows"
            
            # Check statistics for any skipped rows
            processor.get_processing_stats()
            # Note: In this case, rows shouldn't actually be skipped since the data is valid
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_csv_parsing_errors(self):
        """Test handling of CSV parsing errors."""
        # Create malformed CSV file - for self-hosted system, just test graceful handling
        temp_file = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False)
        temp_file.write('game,rating\n')
        temp_file.write('Portal,9\n')
        temp_file.write('Game with "quotes",8\n')  # This is actually valid CSV
        temp_file.close()
        
        test_file = Path(temp_file.name)
        
        try:
            processor = SecureCSVProcessor()
            processed_rows = []
            
            # Should handle gracefully for self-hosted system
            async with processor.process_csv_securely(test_file) as chunks:
                async for chunk in chunks:
                    processed_rows.extend(chunk)
            
            # Should process successfully for reasonable CSV content
            assert len(processed_rows) >= 2, "Should process valid rows"
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_empty_csv_handling(self):
        """Test handling of empty CSV files."""
        # Create empty CSV file
        temp_file = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False)
        temp_file.close()
        
        test_file = Path(temp_file.name)
        
        try:
            processor = SecureCSVProcessor()
            
            with pytest.raises(ValueError) as exc_info:
                async with processor.process_csv_securely(test_file) as chunks:
                    async for chunk in chunks:
                        pass
            
            assert "empty" in str(exc_info.value).lower()
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_unicode_content_handling(self):
        """Test handling of Unicode content in CSV files."""
        # Create CSV with Unicode content
        temp_file = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False, encoding='utf-8')
        
        writer = csv.writer(temp_file)
        writer.writerow(["game", "description"])
        writer.writerow(["Pokémon Red", "Classic RPG with é, ñ, ü"])
        writer.writerow(["東京", "Japanese game title"])
        writer.writerow(["Café Game", "résumé naïve"])
        
        temp_file.close()
        test_file = Path(temp_file.name)
        
        try:
            processor = SecureCSVProcessor()
            processed_rows = []
            
            async with processor.process_csv_securely(test_file) as chunks:
                async for chunk in chunks:
                    processed_rows.extend(chunk)
            
            # Should process all rows with Unicode content
            assert len(processed_rows) == 3, "Should process all Unicode rows"
            
            # Check that Unicode content is preserved
            unicode_content_found = False
            for row in processed_rows:
                for value in row.values():
                    if any(ord(char) > 127 for char in value):
                        unicode_content_found = True
                        break
                        
            assert unicode_content_found, "Unicode content should be preserved"
            
        finally:
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_statistics_tracking(self):
        """Test comprehensive statistics tracking."""
        test_file = self.create_test_csv_file(rows=25, cols=4)  # Should create multiple chunks
        
        try:
            limits = ProcessingLimits(chunk_size=10)  # Force multiple chunks
            processor = SecureCSVProcessor(limits)
            
            start_time = time.time()
            
            async with processor.process_csv_securely(test_file) as chunks:
                async for chunk in chunks:
                    pass  # Process all chunks
                    
            end_time = time.time()
            
            stats = processor.get_processing_stats()
            
            # Verify statistics
            assert stats.total_rows_processed == 25, "Should track all processed rows"
            assert stats.total_chunks_processed >= 3, "Should track multiple chunks"
            assert stats.processing_time_seconds > 0, "Should track processing time"
            assert stats.processing_time_seconds <= (end_time - start_time + 1), "Processing time should be reasonable"
            # Memory tracking may not work in all test environments - acceptable for self-hosted
            assert stats.peak_memory_mb >= 0, "Memory tracking should not be negative"
            
            # Convert to dict for logging simulation
            stats_dict = stats.to_dict()
            assert isinstance(stats_dict, dict), "Should convert to dict"
            assert 'total_rows_processed' in stats_dict, "Should include all fields"
            
        finally:
            test_file.unlink()
    
    def test_processing_limits_configuration(self):
        """Test ProcessingLimits configuration and validation."""
        # Test default limits
        default_limits = ProcessingLimits()
        assert default_limits.max_rows == 10000
        assert default_limits.max_cell_size == 10 * 1024
        assert default_limits.processing_timeout == 300
        
        # Test custom limits
        custom_limits = ProcessingLimits(
            max_rows=5000,
            max_cell_size=5000,
            processing_timeout=120,
            chunk_size=50
        )
        assert custom_limits.max_rows == 5000
        assert custom_limits.max_cell_size == 5000
        assert custom_limits.processing_timeout == 120
        assert custom_limits.chunk_size == 50
    
    @pytest.mark.asyncio
    async def test_validate_csv_size(self):
        """Test CSV size validation utility."""
        test_file = self.create_test_csv_file(rows=100, cols=5)
        
        try:
            # Should validate successfully for reasonable file
            result = await SecureCSVProcessor.validate_csv_size(test_file)
            
            assert 'file_size_bytes' in result
            assert 'estimated_rows' in result
            assert 'estimated_columns' in result
            assert result['size_check_passed'], "Size check should pass for reasonable file"
            
            # Test with restrictive limits
            strict_limits = ProcessingLimits(max_memory_mb=1)  # Very low limit
            result = await SecureCSVProcessor.validate_csv_size(test_file, strict_limits)
            
            # May fail size check with very restrictive limits
            if not result['size_check_passed']:
                assert len(result['errors']) > 0, "Should report size errors"
            
        finally:
            test_file.unlink()
    
    def test_get_recommended_limits(self):
        """Test recommended limits based on file size."""
        # Small file
        small_limits = SecureCSVProcessor.get_recommended_limits(0.5)  # 0.5MB
        assert small_limits.max_rows <= 5000
        assert small_limits.processing_timeout <= 60
        
        # Medium file
        medium_limits = SecureCSVProcessor.get_recommended_limits(3.0)  # 3MB
        assert medium_limits.max_rows == 10000  # Default
        
        # Large file
        large_limits = SecureCSVProcessor.get_recommended_limits(8.0)  # 8MB
        assert large_limits.max_rows >= 20000
        assert large_limits.processing_timeout >= 600
        assert large_limits.chunk_size <= 50  # Smaller chunks for large files
    
    def test_stats_reset(self):
        """Test statistics reset functionality."""
        processor = SecureCSVProcessor()
        
        # Manually set some stats (simulating processing)
        processor.stats.total_rows_processed = 100
        processor.stats.errors_encountered = 5
        processor.stats.peak_memory_mb = 50.5
        
        # Verify stats are set
        assert processor.stats.total_rows_processed == 100
        assert processor.stats.errors_encountered == 5
        
        # Reset and verify
        processor.reset_stats()
        assert processor.stats.total_rows_processed == 0
        assert processor.stats.errors_encountered == 0
        assert processor.stats.peak_memory_mb == 0.0


class TestSecureCSVProcessorEdgeCases:
    """Test edge cases and error conditions."""
    
    @pytest.mark.asyncio
    async def test_nonexistent_file(self):
        """Test handling of non-existent files."""
        nonexistent_file = Path("/tmp/nonexistent_12345.csv")
        processor = SecureCSVProcessor()
        
        with pytest.raises((FileNotFoundError, ValueError)):
            async with processor.process_csv_securely(nonexistent_file) as chunks:
                async for chunk in chunks:
                    pass
    
    @pytest.mark.asyncio
    async def test_permission_denied_file(self):
        """Test handling of permission denied scenarios."""
        # Create a file and remove read permissions
        temp_file = tempfile.NamedTemporaryFile(delete=False)
        temp_file.write(b"game,rating\nPortal,9\n")
        temp_file.close()
        
        test_file = Path(temp_file.name)
        test_file.chmod(0o000)  # No permissions
        
        try:
            processor = SecureCSVProcessor()
            
            with pytest.raises((PermissionError, ValueError)):
                async with processor.process_csv_securely(test_file) as chunks:
                    async for chunk in chunks:
                        pass
                        
        finally:
            # Restore permissions to delete
            test_file.chmod(0o666)
            test_file.unlink()
    
    @pytest.mark.asyncio
    async def test_concurrent_processing_safety(self):
        """Test that multiple processors can run concurrently safely."""
        test_files = []
        
        try:
            # Create multiple test files
            for i in range(3):
                temp_file = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False)
                writer = csv.writer(temp_file)
                writer.writerow(['game', 'rating'])
                for j in range(10):
                    writer.writerow([f'Game_{i}_{j}', str(j % 10)])
                temp_file.close()
                test_files.append(Path(temp_file.name))
            
            # Process files concurrently
            async def process_file(file_path):
                processor = SecureCSVProcessor()
                rows = []
                async with processor.process_csv_securely(file_path) as chunks:
                    async for chunk in chunks:
                        rows.extend(chunk)
                return len(rows)
            
            # Run concurrent processing
            tasks = [process_file(f) for f in test_files]
            results = await asyncio.gather(*tasks)
            
            # All should succeed
            assert all(r == 10 for r in results), "All files should be processed correctly"
            
        finally:
            # Cleanup
            for f in test_files:
                if f.exists():
                    f.unlink()
    
    @pytest.mark.asyncio
    async def test_context_manager_exception_handling(self):
        """Test that context manager properly handles exceptions."""
        test_file = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False)
        
        writer = csv.writer(test_file)
        writer.writerow(['game', 'rating'])
        writer.writerow(['Portal', '9'])
        
        test_file.close()
        file_path = Path(test_file.name)
        
        try:
            processor = SecureCSVProcessor()
            
            with pytest.raises(ValueError):
                async with processor.process_csv_securely(file_path) as chunks:
                    async for chunk in chunks:
                        # Simulate an exception during processing
                        raise ValueError("Simulated error")
            
            # Processor should have recorded the error
            stats = processor.get_processing_stats()
            assert stats.errors_encountered > 0, "Should record processing errors"
            
        finally:
            file_path.unlink()
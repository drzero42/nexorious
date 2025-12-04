"""
Memory-safe CSV processor for preventing resource exhaustion attacks.

This module implements secure CSV processing with strict resource limits to prevent:
- Memory exhaustion attacks (DoS via large CSV files)
- CPU exhaustion attacks (DoS via processing timeouts)
- Storage exhaustion attacks (limits on row/cell counts)

The processor uses chunked streaming to handle large datasets without loading
everything into memory at once, while enforcing security limits at each stage.

Reference:
- OWASP DoS Prevention: https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html
- CWE-400: Uncontrolled Resource Consumption
"""

import asyncio
import psutil
import signal
import time
import logging
import pandas as pd
from contextlib import asynccontextmanager
from pathlib import Path
from typing import AsyncIterator, Dict, Any, List, Optional, Callable
from dataclasses import dataclass

logger = logging.getLogger(__name__)

@dataclass
class ProcessingLimits:
    """Configuration for processing limits and security constraints."""
    max_rows: int = 10000                    # Maximum number of rows to process
    max_cell_size: int = 10 * 1024          # Maximum size per cell (10KB)
    max_memory_mb: int = 512                 # Maximum memory usage (512MB)
    processing_timeout: int = 300            # Maximum processing time (5 minutes)
    chunk_size: int = 100                    # Rows per chunk for streaming
    max_chunks: int = 100                    # Maximum number of chunks
    memory_check_interval: int = 10          # Check memory every N chunks

@dataclass
class ProcessingStats:
    """Statistics tracked during CSV processing."""
    total_rows_processed: int = 0
    total_chunks_processed: int = 0
    peak_memory_mb: float = 0.0
    processing_time_seconds: float = 0.0
    cells_truncated: int = 0
    rows_skipped: int = 0
    errors_encountered: int = 0
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert stats to dictionary for logging."""
        return {
            'total_rows_processed': self.total_rows_processed,
            'total_chunks_processed': self.total_chunks_processed,
            'peak_memory_mb': self.peak_memory_mb,
            'processing_time_seconds': self.processing_time_seconds,
            'cells_truncated': self.cells_truncated,
            'rows_skipped': self.rows_skipped,
            'errors_encountered': self.errors_encountered
        }

class ProcessingTimeoutError(Exception):
    """Raised when CSV processing exceeds timeout limit."""
    pass

class RowLimitExceededError(Exception):
    """Raised when CSV contains more rows than allowed."""
    pass

class CellSizeExceededError(Exception):
    """Raised when a CSV cell exceeds size limit.""" 
    pass

class MemoryLimitExceededError(Exception):
    """Raised when processing exceeds memory limit."""
    pass

class ChunkLimitExceededError(Exception):
    """Raised when CSV requires more chunks than allowed."""
    pass

class SecureCSVProcessor:
    """
    Memory-safe CSV processor with comprehensive resource limiting.
    
    This processor implements multiple layers of protection against resource
    exhaustion attacks while providing streaming access to CSV data.
    
    Features:
    - Memory usage monitoring and limits
    - Row and cell count limits
    - Processing timeouts with cleanup
    - Chunked streaming for large files
    - Automatic resource cleanup
    - Comprehensive error handling
    """
    
    def __init__(self, limits: Optional[ProcessingLimits] = None):
        """
        Initialize the secure CSV processor.
        
        Args:
            limits: Custom processing limits (uses defaults if None)
        """
        self.limits = limits or ProcessingLimits()
        self.stats = ProcessingStats()
        self._start_time: Optional[float] = None
        self._process: Optional[psutil.Process] = None
        self._timeout_handler_set = False
        self._chunk_iterator: Optional[AsyncIterator[List[Dict[str, Any]]]] = None
        self._file_path: Optional[Path] = None
        self._progress_callback: Optional[Callable[[ProcessingStats], None]] = None
    
    @asynccontextmanager
    async def process_csv_securely(self, 
                                 file_path: Path,
                                 progress_callback: Optional[Callable[[ProcessingStats], None]] = None):
        """
        Async context manager for secure CSV processing with automatic cleanup.
        
        Args:
            file_path: Path to CSV file to process
            progress_callback: Optional callback to report progress
            
        Yields:
            Self as async iterator that can be used with 'async for'
            
        Example:
            async with processor.process_csv_securely(csv_path) as chunks:
                async for chunk in chunks:
                    # Process chunk safely
                    for row in chunk:
                        print(row)
        """
        self._start_time = time.time()
        self._process = psutil.Process()
        self._file_path = file_path
        self._progress_callback = progress_callback
        self._chunk_iterator = None
        
        # Set up timeout protection
        self._setup_timeout_protection()
        
        try:
            # Yield self as the async iterator
            yield self
                
        except Exception as e:
            logger.error(f"CSV processing error: {e}")
            self.stats.errors_encountered += 1
            raise
            
        finally:
            # Cleanup and final stats
            self._cleanup_timeout_protection()
            self.stats.processing_time_seconds = time.time() - (self._start_time or time.time())
            
            # Log final statistics
            logger.info(f"CSV processing completed: {self.stats.to_dict()}")
    
    def __aiter__(self):
        """Make this class async iterable."""
        return self
    
    async def __anext__(self):
        """Async iterator protocol - get next chunk."""
        if self._chunk_iterator is None:
            # Initialize the chunk iterator on first call
            assert self._file_path is not None, "File path must be set before iterating"
            self._chunk_iterator = self._process_csv_chunks(self._file_path, self._progress_callback)
        
        try:
            return await self._chunk_iterator.__anext__()
        except StopAsyncIteration:
            raise StopAsyncIteration
    
    async def _process_csv_chunks(self, 
                                file_path: Path,
                                progress_callback: Optional[Callable[[ProcessingStats], None]]) -> AsyncIterator[List[Dict[str, Any]]]:
        """
        Process CSV file in secure chunks with resource monitoring.
        
        Args:
            file_path: Path to CSV file
            progress_callback: Optional progress callback
            
        Yields:
            List of dictionaries representing CSV rows
        """
        chunk_count = 0
        total_rows = 0
        
        try:
            # Use pandas chunked reading for memory efficiency
            chunk_iterator = pd.read_csv(
                file_path,
                chunksize=self.limits.chunk_size,
                encoding='utf-8',
                low_memory=True,  # Use efficient dtypes
                na_filter=False,  # Don't convert to NaN (keep strings)
            )
            
            for chunk_df in chunk_iterator:
                # Check processing timeout
                self._check_timeout()
                
                # Check chunk limits
                chunk_count += 1
                if chunk_count > self.limits.max_chunks:
                    raise ChunkLimitExceededError(f"Too many chunks: {chunk_count} > {self.limits.max_chunks}")
                
                # Check row limits
                chunk_rows = len(chunk_df)
                total_rows += chunk_rows
                if total_rows > self.limits.max_rows:
                    raise RowLimitExceededError(f"Too many rows: {total_rows} > {self.limits.max_rows}")
                
                # Check memory usage periodically
                if chunk_count % self.limits.memory_check_interval == 0:
                    self._check_memory_usage()
                
                # Process chunk with cell size validation and sanitization
                processed_chunk = await self._process_chunk_safely(chunk_df)
                
                # Update statistics
                self.stats.total_chunks_processed = chunk_count
                self.stats.total_rows_processed = total_rows
                
                # Report progress if callback provided
                if progress_callback:
                    try:
                        progress_callback(self.stats)
                    except Exception as e:
                        logger.warning(f"Progress callback error: {e}")
                
                # Yield the processed chunk
                yield processed_chunk
                
                # Allow other tasks to run
                await asyncio.sleep(0)
                
        except pd.errors.ParserError as e:
            raise ValueError(f"CSV parsing failed: {str(e)}")
        except pd.errors.EmptyDataError:
            raise ValueError("CSV file is empty")
        except UnicodeDecodeError as e:
            raise ValueError(f"CSV encoding error: {str(e)}")
    
    async def _process_chunk_safely(self, chunk_df: pd.DataFrame) -> List[Dict[str, Any]]:
        """
        Process a single chunk with cell size validation and security checks.
        
        Args:
            chunk_df: Pandas DataFrame chunk
            
        Returns:
            List of dictionaries with validated and sanitized data
        """
        processed_rows = []
        
        # Convert DataFrame to list of dictionaries
        chunk_records = chunk_df.to_dict('records')
        
        for row_index, row in enumerate(chunk_records):
            try:
                processed_row = {}
                
                for column, value in row.items():
                    # Convert to string and validate size
                    str_value = str(value) if pd.notna(value) else ""
                    
                    # Check cell size limit
                    if len(str_value) > self.limits.max_cell_size:
                        logger.warning(f"Cell truncated: {len(str_value)} > {self.limits.max_cell_size}")
                        str_value = str_value[:self.limits.max_cell_size]
                        self.stats.cells_truncated += 1
                    
                    processed_row[str(column)] = str_value
                
                processed_rows.append(processed_row)
                
            except Exception as e:
                logger.error(f"Error processing row {row_index}: {e}")
                self.stats.rows_skipped += 1
                # Continue processing other rows
                continue
        
        return processed_rows
    
    def _setup_timeout_protection(self):
        """Set up timeout protection using signal alarm."""
        if not self._timeout_handler_set:
            def timeout_handler(signum, frame):
                raise ProcessingTimeoutError(f"CSV processing timeout after {self.limits.processing_timeout} seconds")
            
            signal.signal(signal.SIGALRM, timeout_handler)
            signal.alarm(self.limits.processing_timeout)
            self._timeout_handler_set = True
    
    def _cleanup_timeout_protection(self):
        """Clean up timeout protection."""
        if self._timeout_handler_set:
            signal.alarm(0)  # Cancel alarm
            self._timeout_handler_set = False
    
    def _check_timeout(self):
        """Check if processing has exceeded timeout limit."""
        if self._start_time:
            elapsed = time.time() - self._start_time
            if elapsed > self.limits.processing_timeout:
                raise ProcessingTimeoutError(f"Processing timeout: {elapsed:.1f}s > {self.limits.processing_timeout}s")
    
    def _check_memory_usage(self):
        """Check current memory usage against limits."""
        if self._process:
            try:
                memory_info = self._process.memory_info()
                memory_mb = memory_info.rss / 1024 / 1024  # Convert to MB
                
                # Track peak memory usage
                self.stats.peak_memory_mb = max(self.stats.peak_memory_mb, memory_mb)
                
                if memory_mb > self.limits.max_memory_mb:
                    raise MemoryLimitExceededError(f"Memory limit exceeded: {memory_mb:.1f}MB > {self.limits.max_memory_mb}MB")
                    
                logger.debug(f"Memory usage: {memory_mb:.1f}MB")
                
            except psutil.Error as e:
                logger.warning(f"Could not check memory usage: {e}")
    
    @staticmethod
    async def validate_csv_size(file_path: Path, limits: Optional[ProcessingLimits] = None) -> Dict[str, Any]:
        """
        Quick validation of CSV file size and basic structure without full processing.
        
        Args:
            file_path: Path to CSV file to validate
            limits: Processing limits to check against
            
        Returns:
            Dictionary with validation results
        """
        if limits is None:
            limits = ProcessingLimits()
        
        try:
            # Get file size
            file_size = file_path.stat().st_size
            max_file_size = limits.max_memory_mb * 1024 * 1024  # Convert to bytes
            
            warnings: List[str] = []
            errors: List[str] = []
            result: Dict[str, Any] = {
                'file_size_bytes': file_size,
                'estimated_rows': 0,
                'estimated_columns': 0,
                'size_check_passed': file_size <= max_file_size,
                'warnings': warnings,
                'errors': errors
            }
            
            if file_size > max_file_size:
                errors.append(f"File size {file_size} exceeds limit {max_file_size}")
                return result
            
            # Quick structure check - read just the first few lines
            try:
                sample_df = pd.read_csv(file_path, nrows=10, encoding='utf-8')
                result['estimated_columns'] = len(sample_df.columns)
                
                # Estimate total rows (rough calculation)
                if file_size > 0:
                    avg_row_size = file_size / len(sample_df) if len(sample_df) > 0 else 100
                    result['estimated_rows'] = int(file_size / avg_row_size)
                
                # Check against limits
                if result['estimated_rows'] > limits.max_rows:
                    warnings.append(f"Estimated rows {result['estimated_rows']} may exceed limit {limits.max_rows}")

            except Exception as e:
                warnings.append(f"Could not analyze CSV structure: {e}")
            
            return result
            
        except Exception as e:
            return {
                'file_size_bytes': 0,
                'estimated_rows': 0,
                'estimated_columns': 0,
                'size_check_passed': False,
                'warnings': [],
                'errors': [f"Validation failed: {e}"]
            }
    
    @staticmethod
    def get_recommended_limits(file_size_mb: float) -> ProcessingLimits:
        """
        Get recommended processing limits based on file size.
        
        Args:
            file_size_mb: File size in megabytes
            
        Returns:
            Recommended ProcessingLimits configuration
        """
        if file_size_mb < 1:
            # Small files - relaxed limits
            return ProcessingLimits(
                max_rows=5000,
                chunk_size=200,
                max_memory_mb=256,
                processing_timeout=60
            )
        elif file_size_mb < 5:
            # Medium files - default limits
            return ProcessingLimits()
        else:
            # Large files - strict limits
            return ProcessingLimits(
                max_rows=20000,
                chunk_size=50,
                max_memory_mb=1024,
                processing_timeout=600,  # 10 minutes
                memory_check_interval=5
            )
    
    def get_processing_stats(self) -> ProcessingStats:
        """Get current processing statistics."""
        return self.stats
    
    def reset_stats(self):
        """Reset processing statistics."""
        self.stats = ProcessingStats()
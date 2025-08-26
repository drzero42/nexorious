"""
Batch Session models for managing batched operations on Steam games.

This module provides models for tracking and managing long-running batch operations
like auto-matching and syncing Steam games in small batches with progress feedback.
"""

from datetime import datetime, timezone
from typing import List, Dict, Any
from enum import Enum
from dataclasses import dataclass, field
import uuid


class BatchOperationType(str, Enum):
    """Types of batch operations."""
    AUTO_MATCH = "auto_match"
    SYNC = "sync"


class BatchSessionStatus(str, Enum):
    """Status of a batch session."""
    ACTIVE = "active"
    COMPLETED = "completed"
    CANCELLED = "cancelled"
    FAILED = "failed"


@dataclass
class BatchSession:
    """
    Represents a batch processing session for Steam games operations.
    
    This tracks the overall progress of a batched operation, allowing
    the frontend to process games in small chunks with progress feedback.
    """
    id: str
    user_id: str
    operation_type: BatchOperationType
    total_items: int
    processed_items: int = 0
    successful_items: int = 0
    failed_items: int = 0
    status: BatchSessionStatus = BatchSessionStatus.ACTIVE
    created_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    errors: List[str] = field(default_factory=list)
    # Store IDs of items that have been processed to track progress
    processed_item_ids: List[str] = field(default_factory=list)
    # Store IDs of items that failed for potential retry
    failed_item_ids: List[str] = field(default_factory=list)

    @classmethod
    def create(
        cls,
        user_id: str,
        operation_type: BatchOperationType,
        total_items: int
    ) -> "BatchSession":
        """Create a new batch session with a unique ID."""
        return cls(
            id=str(uuid.uuid4()),
            user_id=user_id,
            operation_type=operation_type,
            total_items=total_items
        )

    def update_progress(
        self,
        processed_count: int,
        successful_count: int,
        failed_count: int,
        processed_ids: List[str],
        failed_ids: List[str],
        errors: List[str]
    ) -> None:
        """Update the session progress after processing a batch."""
        self.processed_items += processed_count
        self.successful_items += successful_count
        self.failed_items += failed_count
        self.processed_item_ids.extend(processed_ids)
        self.failed_item_ids.extend(failed_ids)
        self.errors.extend(errors)
        self.updated_at = datetime.now(timezone.utc)
        
        # Update status if completed
        if self.processed_items >= self.total_items:
            self.status = BatchSessionStatus.COMPLETED

    def cancel(self) -> None:
        """Cancel the batch session."""
        self.status = BatchSessionStatus.CANCELLED
        self.updated_at = datetime.now(timezone.utc)

    def fail(self, error_message: str) -> None:
        """Mark the batch session as failed."""
        self.status = BatchSessionStatus.FAILED
        self.errors.append(error_message)
        self.updated_at = datetime.now(timezone.utc)

    @property
    def is_active(self) -> bool:
        """Check if the session is still active."""
        return self.status == BatchSessionStatus.ACTIVE

    @property
    def is_complete(self) -> bool:
        """Check if the session is complete (successful or failed)."""
        return self.status in [BatchSessionStatus.COMPLETED, BatchSessionStatus.CANCELLED, BatchSessionStatus.FAILED]

    @property
    def remaining_items(self) -> int:
        """Get the number of items remaining to process."""
        return max(0, self.total_items - self.processed_items)

    @property
    def progress_percentage(self) -> float:
        """Get the progress as a percentage."""
        if self.total_items == 0:
            return 100.0
        return (self.processed_items / self.total_items) * 100.0

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary for API responses."""
        return {
            "session_id": self.id,
            "user_id": self.user_id,
            "operation_type": self.operation_type.value,
            "total_items": self.total_items,
            "processed_items": self.processed_items,
            "successful_items": self.successful_items,
            "failed_items": self.failed_items,
            "remaining_items": self.remaining_items,
            "progress_percentage": round(self.progress_percentage, 1),
            "status": self.status.value,
            "created_at": self.created_at.isoformat(),
            "updated_at": self.updated_at.isoformat(),
            "errors": self.errors,
            "is_complete": self.is_complete
        }


# Constants for batch processing
BATCH_SIZES = {
    BatchOperationType.AUTO_MATCH: 5,  # Small batches for more frequent progress updates
    BatchOperationType.SYNC: 5         # Small batches for more frequent progress updates
}

# Session timeout in minutes
SESSION_TIMEOUT_MINUTES = 30
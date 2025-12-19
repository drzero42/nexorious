"""
Unified Job model for tracking background tasks (sync, import, export).

This model consolidates all background task types into a single table,
replacing the separate ImportJob model and enabling unified job management.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import UniqueConstraint
from typing import Optional, Dict, Any, List, TYPE_CHECKING
from datetime import datetime, timezone
from enum import Enum
import uuid
import json

if TYPE_CHECKING:
    from .user import User


class BackgroundJobType(str, Enum):
    """Type of background job."""

    SYNC = "sync"
    IMPORT = "import"
    EXPORT = "export"


class BackgroundJobSource(str, Enum):
    """Source/platform for the job."""

    STEAM = "steam"
    EPIC = "epic"
    GOG = "gog"
    XBOX = "xbox"
    PLAYSTATION = "playstation"
    CSV = "csv"
    DARKADIA = "darkadia"
    NEXORIOUS = "nexorious"
    SYSTEM = "system"


class BackgroundJobStatus(str, Enum):
    """Status of a background job."""

    PENDING = "pending"
    PROCESSING = "processing"
    AWAITING_REVIEW = "awaiting_review"
    READY = "ready"
    FINALIZING = "finalizing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class BackgroundJobPriority(str, Enum):
    """Priority level for job queue."""

    HIGH = "high"
    LOW = "low"


class ImportJobSubtype(str, Enum):
    """Subtype for import jobs - specifies the kind of import operation."""

    LIBRARY_IMPORT = "library_import"
    AUTO_MATCH = "auto_match"
    BULK_SYNC = "bulk_sync"
    BULK_UNMATCH = "bulk_unmatch"
    BULK_UNSYNC = "bulk_unsync"
    BULK_UNIGNORE = "bulk_unignore"


class Job(SQLModel, table=True):
    """
    Unified job model for all background tasks.

    Tracks sync, import, and export operations with their progress,
    status, and results. Supports the taskiq-based background task system.
    """

    __tablename__ = "jobs"  # type: ignore[assignment]

    # Primary key
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)

    # User association
    user_id: str = Field(foreign_key="users.id", index=True)

    # Job classification
    job_type: BackgroundJobType = Field(index=True)
    source: BackgroundJobSource = Field(index=True)
    status: BackgroundJobStatus = Field(default=BackgroundJobStatus.PENDING, index=True)
    priority: BackgroundJobPriority = Field(default=BackgroundJobPriority.HIGH)
    import_subtype: Optional[ImportJobSubtype] = Field(
        default=None,
        index=True,
        description="Subtype for import jobs (library_import, auto_match, bulk_*)",
    )

    # Progress tracking
    progress_current: int = Field(default=0)
    progress_total: int = Field(default=0)
    successful_items: int = Field(
        default=0, description="Number of items processed successfully"
    )
    failed_items: int = Field(
        default=0, description="Number of items that failed processing"
    )

    # Results and errors (stored as JSON strings for compatibility)
    result_summary_json: str = Field(
        default="{}",
        sa_column_kwargs={"name": "result_summary"},
        description="JSON string containing result statistics",
    )
    error_log_json: str = Field(
        default="[]",
        sa_column_kwargs={"name": "error_log"},
        description="JSON array of error details from processing",
    )
    error_message: Optional[str] = Field(
        default=None, max_length=2000, description="Primary error message if job failed"
    )

    # Batch session tracking (for auto-match and sync operations)
    processed_item_ids_json: str = Field(
        default="[]",
        sa_column_kwargs={"name": "processed_item_ids"},
        description="JSON array of item IDs that have been processed",
    )
    failed_item_ids_json: str = Field(
        default="[]",
        sa_column_kwargs={"name": "failed_item_ids"},
        description="JSON array of item IDs that failed processing",
    )

    # File path for exports
    file_path: Optional[str] = Field(
        default=None, max_length=500, description="File path for export jobs"
    )

    # Taskiq task ID for tracking
    taskiq_task_id: Optional[str] = Field(
        default=None,
        max_length=100,
        index=True,
        description="Taskiq task ID for status tracking",
    )

    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    started_at: Optional[datetime] = Field(default=None)
    completed_at: Optional[datetime] = Field(default=None)

    # Relationships
    user: "User" = Relationship(back_populates="jobs")
    review_items: List["ReviewItem"] = Relationship(
        back_populates="job",
        sa_relationship_kwargs={"cascade": "all, delete-orphan"},
    )

    def get_result_summary(self) -> Dict[str, Any]:
        """Get result summary as a dictionary."""
        try:
            return json.loads(self.result_summary_json)
        except (json.JSONDecodeError, TypeError):
            return {}

    def set_result_summary(self, value: Dict[str, Any]) -> None:
        """Set result summary from a dictionary."""
        self.result_summary_json = json.dumps(value)

    def get_error_log(self) -> List[Dict[str, Any]]:
        """Get error log as a list of error details."""
        try:
            return json.loads(self.error_log_json)
        except (json.JSONDecodeError, TypeError):
            return []

    def set_error_log(self, value: List[Dict[str, Any]]) -> None:
        """Set error log from a list of error details."""
        self.error_log_json = json.dumps(value)

    def add_error(self, error: Dict[str, Any]) -> None:
        """Append an error to the error log."""
        errors = self.get_error_log()
        errors.append(error)
        self.set_error_log(errors)

    def get_processed_item_ids(self) -> List[str]:
        """Get processed item IDs as a list."""
        try:
            return json.loads(self.processed_item_ids_json)
        except (json.JSONDecodeError, TypeError):
            return []

    def set_processed_item_ids(self, value: List[str]) -> None:
        """Set processed item IDs from a list."""
        self.processed_item_ids_json = json.dumps(value)

    def add_processed_item_id(self, item_id: str) -> None:
        """Add an item ID to the processed list."""
        ids = self.get_processed_item_ids()
        ids.append(item_id)
        self.set_processed_item_ids(ids)

    def get_failed_item_ids(self) -> List[str]:
        """Get failed item IDs as a list."""
        try:
            return json.loads(self.failed_item_ids_json)
        except (json.JSONDecodeError, TypeError):
            return []

    def set_failed_item_ids(self, value: List[str]) -> None:
        """Set failed item IDs from a list."""
        self.failed_item_ids_json = json.dumps(value)

    def add_failed_item_id(self, item_id: str) -> None:
        """Add an item ID to the failed list."""
        ids = self.get_failed_item_ids()
        ids.append(item_id)
        self.set_failed_item_ids(ids)

    @property
    def progress_percent(self) -> int:
        """Calculate progress percentage."""
        if self.progress_total == 0:
            return 0
        return min(100, int((self.progress_current / self.progress_total) * 100))

    @property
    def remaining_items(self) -> int:
        """Calculate remaining items to process."""
        return max(0, self.progress_total - self.progress_current)

    @property
    def is_active(self) -> bool:
        """Check if job is in an active (non-terminal) state."""
        return self.status in (
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
            BackgroundJobStatus.AWAITING_REVIEW,
            BackgroundJobStatus.READY,
            BackgroundJobStatus.FINALIZING,
        )

    @property
    def is_terminal(self) -> bool:
        """Check if job is in a terminal state."""
        return self.status in (
            BackgroundJobStatus.COMPLETED,
            BackgroundJobStatus.FAILED,
            BackgroundJobStatus.CANCELLED,
        )

    @property
    def duration_seconds(self) -> Optional[float]:
        """Calculate job duration in seconds."""
        if self.started_at is None:
            return None
        end_time = self.completed_at or datetime.now(timezone.utc)
        # Ensure both datetimes have consistent timezone handling
        # PostgreSQL may return timezone-aware or naive datetimes depending on configuration
        if end_time.tzinfo is not None and self.started_at.tzinfo is None:
            # end_time is aware, started_at is naive - make started_at aware (assume UTC)
            started = self.started_at.replace(tzinfo=timezone.utc)
        elif end_time.tzinfo is None and self.started_at.tzinfo is not None:
            # end_time is naive, started_at is aware - make end_time aware (assume UTC)
            end_time = end_time.replace(tzinfo=timezone.utc)
            started = self.started_at
        else:
            started = self.started_at
        return (end_time - started).total_seconds()


class ReviewItemStatus(str, Enum):
    """Status of a review item."""

    PENDING = "pending"
    MATCHED = "matched"
    SKIPPED = "skipped"
    REMOVAL = "removal"


class ReviewItem(SQLModel, table=True):
    """
    Review item for games that need user matching decisions.

    Created during sync/import when automatic matching fails or
    when sync detects games removed from a platform library.
    """

    __tablename__ = "review_items"  # type: ignore[assignment]
    __table_args__ = (
        # Prevent duplicate ReviewItems for the same game title within a job
        UniqueConstraint("job_id", "source_title", name="uq_review_items_job_source_title"),
    )

    # Primary key
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)

    # Associations
    job_id: str = Field(foreign_key="jobs.id", index=True)
    user_id: str = Field(foreign_key="users.id", index=True)

    # Status
    status: ReviewItemStatus = Field(default=ReviewItemStatus.PENDING, index=True)

    # Source data
    source_title: str = Field(max_length=500, description="Game title from source")
    source_metadata_json: str = Field(
        default="{}",
        sa_column_kwargs={"name": "source_metadata"},
        description="JSON string with platform ID, release year, etc.",
    )

    # IGDB matching
    igdb_candidates_json: str = Field(
        default="[]",
        sa_column_kwargs={"name": "igdb_candidates"},
        description="JSON array of IGDB search results for user to pick from",
    )
    resolved_igdb_id: Optional[int] = Field(
        default=None, description="IGDB ID selected by user"
    )
    match_confidence: Optional[float] = Field(
        default=None,
        ge=0.0,
        le=1.0,
        description="Confidence score for auto-match (1.0 = exact match or single result)"
    )

    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    resolved_at: Optional[datetime] = Field(default=None)

    # Relationships
    job: Job = Relationship(back_populates="review_items")

    def get_source_metadata(self) -> Dict[str, Any]:
        """Get source metadata as a dictionary."""
        try:
            return json.loads(self.source_metadata_json)
        except (json.JSONDecodeError, TypeError):
            return {}

    def set_source_metadata(self, value: Dict[str, Any]) -> None:
        """Set source metadata from a dictionary."""
        self.source_metadata_json = json.dumps(value)

    def get_igdb_candidates(self) -> List[Dict[str, Any]]:
        """Get IGDB candidates as a list."""
        try:
            return json.loads(self.igdb_candidates_json)
        except (json.JSONDecodeError, TypeError):
            return []

    def set_igdb_candidates(self, value: List[Dict[str, Any]]) -> None:
        """Set IGDB candidates from a list."""
        self.igdb_candidates_json = json.dumps(value)

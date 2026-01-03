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
    MAINTENANCE = "maintenance"


class BackgroundJobSource(str, Enum):
    """Source/platform for the job."""

    STEAM = "steam"
    EPIC = "epic"
    GOG = "gog"
    XBOX = "xbox"
    PLAYSTATION = "playstation"
    CSV = "csv"
    NEXORIOUS = "nexorious"
    SYSTEM = "system"


class BackgroundJobStatus(str, Enum):
    """Status of a background job."""

    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class BackgroundJobPriority(str, Enum):
    """Priority level for job queue."""

    HIGH = "high"
    LOW = "low"


class JobItemStatus(str, Enum):
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    PENDING_REVIEW = "pending_review"
    SKIPPED = "skipped"
    FAILED = "failed"


class ImportJobSubtype(str, Enum):
    """Subtype for import jobs - specifies the kind of import operation."""

    LIBRARY_IMPORT = "library_import"
    WISHLIST_IMPORT = "wishlist_import"
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

    # File path for exports
    file_path: Optional[str] = Field(
        default=None, max_length=500, description="File path for export jobs"
    )

    # Progress tracking
    total_items: int = Field(default=0)

    # Error tracking
    error_message: Optional[str] = Field(
        default=None, max_length=2000, description="Primary error message if job failed"
    )

    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    started_at: Optional[datetime] = Field(default=None)
    completed_at: Optional[datetime] = Field(default=None)

    # Retry tracking
    auto_retry_done: bool = Field(default=False, description="Whether automatic retry has been performed")

    # Relationships
    user: "User" = Relationship(back_populates="jobs")
    items: list["JobItem"] = Relationship(
        back_populates="job",
        sa_relationship_kwargs={"cascade": "all, delete-orphan"},
    )

    @property
    def is_active(self) -> bool:
        """Check if job is in an active (non-terminal) state."""
        return self.status in (
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
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


class JobItem(SQLModel, table=True):
    __tablename__ = "job_items"

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    job_id: str = Field(foreign_key="jobs.id", index=True)
    user_id: str = Field(foreign_key="users.id", index=True)

    # Item identification
    item_key: str = Field(index=True)
    source_title: str = Field(max_length=500)
    source_metadata_json: str = Field(default="{}")

    # Processing outcome
    status: JobItemStatus = Field(default=JobItemStatus.PENDING, index=True)
    result_json: str = Field(default="{}")
    error_message: str | None = None

    # Review fields (when status=PENDING_REVIEW)
    igdb_candidates_json: str = Field(default="[]")
    resolved_igdb_id: int | None = None
    match_confidence: float | None = None

    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    processed_at: datetime | None = None
    resolved_at: datetime | None = None

    # Relationships
    job: "Job" = Relationship(back_populates="items")

    __table_args__ = (
        UniqueConstraint("job_id", "item_key", name="uq_job_item_key"),
    )

    def get_source_metadata(self) -> Dict[str, Any]:
        """Get source metadata as a dictionary."""
        return json.loads(self.source_metadata_json) if self.source_metadata_json else {}

    def set_source_metadata(self, metadata: Dict[str, Any]) -> None:
        """Set source metadata from a dictionary."""
        self.source_metadata_json = json.dumps(metadata)

    def get_igdb_candidates(self) -> List[Dict[str, Any]]:
        """Get IGDB candidates as a list."""
        return json.loads(self.igdb_candidates_json) if self.igdb_candidates_json else []

    def set_igdb_candidates(self, candidates: List[Dict[str, Any]]) -> None:
        """Set IGDB candidates from a list."""
        self.igdb_candidates_json = json.dumps(candidates)

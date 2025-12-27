"""
Pydantic schemas for Job management API.

Provides request/response models for job listing, detail retrieval,
cancellation, and deletion endpoints.
"""

from pydantic import BaseModel, Field, ConfigDict, computed_field
from typing import Optional, List, Dict, Any
from datetime import datetime
from enum import Enum


class JobType(str, Enum):
    """Type of background job."""

    SYNC = "sync"
    IMPORT = "import"
    EXPORT = "export"


class JobSource(str, Enum):
    """Source/platform for the job."""

    STEAM = "steam"
    EPIC = "epic"
    GOG = "gog"
    DARKADIA = "darkadia"
    NEXORIOUS = "nexorious"
    SYSTEM = "system"


class JobStatus(str, Enum):
    """Status of a background job."""

    PENDING = "pending"
    PROCESSING = "processing"
    READY = "ready"
    FINALIZING = "finalizing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class JobPriority(str, Enum):
    """Priority level for job queue."""

    HIGH = "high"
    LOW = "low"


class JobProgress(BaseModel):
    """Progress counts by status."""

    pending: int = 0
    processing: int = 0
    completed: int = 0
    pending_review: int = 0
    skipped: int = 0
    failed: int = 0

    @computed_field
    @property
    def total(self) -> int:
        return (
            self.pending
            + self.processing
            + self.completed
            + self.pending_review
            + self.skipped
            + self.failed
        )

    @computed_field
    @property
    def percent(self) -> int:
        if self.total == 0:
            return 0
        done = self.completed + self.pending_review + self.skipped + self.failed
        return int((done / self.total) * 100)


class JobResponse(BaseModel):
    """Response model for a single job."""

    model_config = ConfigDict(from_attributes=True)

    id: str
    user_id: str
    job_type: JobType
    source: JobSource
    status: JobStatus
    priority: JobPriority

    # Progress tracking
    progress: JobProgress
    total_items: int

    # Error tracking
    error_message: Optional[str] = None

    # File path for exports
    file_path: Optional[str] = None

    # Timestamps
    created_at: datetime
    started_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None

    # Computed fields
    is_terminal: bool
    duration_seconds: Optional[float] = None


class JobListResponse(BaseModel):
    """Response model for paginated job list."""

    jobs: List[JobResponse]
    total: int
    page: int
    per_page: int
    pages: int


class JobCancelResponse(BaseModel):
    """Response model for job cancellation."""

    success: bool
    message: str
    job: Optional[JobResponse] = None


class JobDeleteResponse(BaseModel):
    """Response model for job deletion."""

    success: bool
    message: str
    deleted_job_id: str


class JobConfirmResponse(BaseModel):
    """Response model for confirming an import job after review."""

    success: bool
    message: str
    job: Optional[JobResponse] = None
    games_added: int = 0
    games_skipped: int = 0
    games_removed: int = 0


class JobDiscardResponse(BaseModel):
    """Response model for discarding an import job."""

    success: bool
    message: str
    deleted_job_id: str
    deleted_review_items: int


class RetryFailedResponse(BaseModel):
    """Response for retry failed items endpoint."""

    success: bool
    message: str
    retried_count: int

"""
Import job model for tracking bulk data operations.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional
from datetime import datetime, timezone
from enum import Enum
import uuid


class ImportType(str, Enum):
    """Import type enumeration."""
    CSV = "csv"
    STEAM = "steam"
    EPIC = "epic"
    GOG = "gog"
    XBOX = "xbox"
    PLAYSTATION = "playstation"


class ImportStatus(str, Enum):
    """Import status enumeration."""
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"


class ImportJob(SQLModel, table=True):
    """Import job model for tracking bulk data operations."""
    
    __tablename__ = "import_jobs"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    import_type: ImportType
    status: ImportStatus = Field(default=ImportStatus.PENDING, index=True)
    total_records: int = Field(default=0)
    processed_records: int = Field(default=0)
    failed_records: int = Field(default=0)
    error_log: str = Field(default="[]")  # JSON string for error details
    job_metadata: str = Field(default="{}")  # JSON string for import-specific metadata
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    completed_at: Optional[datetime] = Field(default=None)
    
    # Relationships
    user: "User" = Relationship(back_populates="import_jobs")


# Import forward references
from .user import User
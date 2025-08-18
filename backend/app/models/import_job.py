"""
Import job model for tracking bulk data operations and source imports.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, Dict, Any
from datetime import datetime, timezone
from enum import Enum
import uuid
import json


class ImportType(str, Enum):
    """Import type enumeration."""
    CSV = "csv"
    STEAM = "steam"
    EPIC = "epic"
    GOG = "gog"
    XBOX = "xbox"
    PLAYSTATION = "playstation"
    DARKADIA = "darkadia"


class ImportStatus(str, Enum):
    """Import status enumeration."""
    PENDING = "pending"
    PROCESSING = "processing"
    RUNNING = "running"  # Added for new import framework
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"  # Added for new import framework


class JobType(str, Enum):
    """Job type enumeration for new import framework."""
    LIBRARY_IMPORT = "library_import"
    AUTO_MATCH = "auto_match" 
    BULK_SYNC = "bulk_sync"
    BULK_UNMATCH = "bulk_unmatch"
    BULK_UNSYNC = "bulk_unsync"
    BULK_UNIGNORE = "bulk_unignore"


class ImportJob(SQLModel, table=True):
    """Import job model for tracking bulk data operations and source imports."""
    
    __tablename__ = "import_jobs"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    import_type: ImportType
    status: ImportStatus = Field(default=ImportStatus.PENDING, index=True)
    
    # Legacy fields (maintain backward compatibility)
    total_records: int = Field(default=0)
    processed_records: int = Field(default=0)
    failed_records: int = Field(default=0)
    error_log: str = Field(default="[]")  # JSON string for error details
    job_metadata: str = Field(default="{}")  # JSON string for import-specific metadata
    
    # New framework fields
    job_type: Optional[JobType] = Field(default=None, description="Specific job type for new import framework")
    source: Optional[str] = Field(default=None, index=True, description="Import source identifier")
    started_at: Optional[datetime] = Field(default=None, description="When job processing started")
    progress: int = Field(default=0, description="Progress percentage 0-100")
    total_items: int = Field(default=0, description="Total number of items to process")
    processed_items: int = Field(default=0, description="Number of items processed")
    successful_items: int = Field(default=0, description="Number of items processed successfully")
    failed_items: int = Field(default=0, description="Number of items that failed processing")
    error_message: Optional[str] = Field(default=None, description="Primary error message if job failed")
    
    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    completed_at: Optional[datetime] = Field(default=None)
    
    # Relationships
    user: "User" = Relationship(back_populates="import_jobs")
    
    def get_metadata(self) -> Dict[str, Any]:
        """Get metadata as a dictionary."""
        try:
            return json.loads(self.job_metadata)
        except (json.JSONDecodeError, TypeError):
            return {}
    
    def set_metadata(self, value: Dict[str, Any]) -> None:
        """Set metadata from a dictionary."""
        self.job_metadata = json.dumps(value)
    
    def get_errors(self) -> list:
        """Get error log as a list."""
        try:
            return json.loads(self.error_log)
        except (json.JSONDecodeError, TypeError):
            return []
    
    def set_errors(self, value: list) -> None:
        """Set error log from a list."""
        self.error_log = json.dumps(value)


# Import forward references
from .user import User
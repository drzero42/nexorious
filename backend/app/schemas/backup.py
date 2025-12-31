"""
Schemas for backup/restore API endpoints.
"""

from pydantic import BaseModel, Field
from typing import Optional
from datetime import datetime
from enum import Enum


class BackupSchedule(str, Enum):
    """Backup schedule options."""
    MANUAL = "manual"
    DAILY = "daily"
    WEEKLY = "weekly"


class RetentionMode(str, Enum):
    """Retention policy mode."""
    DAYS = "days"
    COUNT = "count"


class BackupType(str, Enum):
    """Type of backup."""
    SCHEDULED = "scheduled"
    MANUAL = "manual"
    PRE_RESTORE = "pre_restore"


# Configuration schemas
class BackupConfigResponse(BaseModel):
    """Response schema for backup configuration."""
    schedule: BackupSchedule
    schedule_time: str
    schedule_day: Optional[int] = None
    retention_mode: RetentionMode
    retention_value: int
    updated_at: datetime


class BackupConfigUpdateRequest(BaseModel):
    """Request schema for updating backup configuration."""
    schedule: Optional[BackupSchedule] = None
    schedule_time: Optional[str] = Field(None, pattern=r"^\d{2}:\d{2}$")
    schedule_day: Optional[int] = Field(None, ge=0, le=6)
    retention_mode: Optional[RetentionMode] = None
    retention_value: Optional[int] = Field(None, ge=1)


# Backup info schemas
class BackupStats(BaseModel):
    """Statistics from backup manifest."""
    users: int
    games: int
    tags: int


class BackupInfo(BaseModel):
    """Information about a single backup."""
    id: str
    created_at: datetime
    backup_type: BackupType
    size_bytes: int
    stats: BackupStats


class BackupListResponse(BaseModel):
    """Response for listing backups."""
    backups: list[BackupInfo]
    total: int


# Backup operation schemas
class BackupCreateResponse(BaseModel):
    """Response after creating a backup."""
    job_id: str
    message: str


class BackupDeleteResponse(BaseModel):
    """Response after deleting a backup."""
    success: bool
    message: str


# Restore schemas
class RestoreRequest(BaseModel):
    """Request for restore confirmation."""
    confirm: bool = Field(..., description="Must be true to confirm restore")


class RestoreResponse(BaseModel):
    """Response after initiating restore."""
    success: bool
    message: str
    session_invalidated: bool = False


# Internal API schemas (worker-to-API communication)
class InternalBackupRequest(BaseModel):
    """Request for internal backup creation."""
    backup_type: BackupType = BackupType.MANUAL


class InternalBackupResponse(BaseModel):
    """Response from internal backup creation."""
    success: bool
    backup_id: Optional[str] = None
    error: Optional[str] = None

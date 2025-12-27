"""
Pydantic schemas for Sync configuration API.

Provides request/response models for sync configuration management
and manual sync triggering endpoints.
"""

from pydantic import BaseModel, Field, ConfigDict
from typing import Optional, List
from datetime import datetime
from enum import Enum


class SyncFrequency(str, Enum):
    """Frequency options for automatic platform syncing."""

    MANUAL = "manual"
    HOURLY = "hourly"
    DAILY = "daily"
    WEEKLY = "weekly"


class SyncPlatform(str, Enum):
    """Supported platforms for syncing."""

    STEAM = "steam"
    EPIC = "epic"
    GOG = "gog"


class SyncConfigResponse(BaseModel):
    """Response model for a single sync configuration."""

    model_config = ConfigDict(from_attributes=True)

    id: str
    user_id: str
    platform: str
    frequency: SyncFrequency
    auto_add: bool = Field(
        description="If True, matched games are added automatically. If False, queued for review."
    )
    enabled: bool = Field(description="Whether sync is enabled for this platform")
    last_synced_at: Optional[datetime] = Field(
        default=None, description="Timestamp of last successful sync"
    )
    created_at: datetime
    updated_at: datetime


class SyncConfigListResponse(BaseModel):
    """Response model for list of all sync configurations."""

    configs: List[SyncConfigResponse]
    total: int


class SyncConfigUpdateRequest(BaseModel):
    """Request model for updating sync configuration."""

    frequency: Optional[SyncFrequency] = Field(
        default=None, description="Sync frequency (manual, hourly, daily, weekly)"
    )
    auto_add: Optional[bool] = Field(
        default=None,
        description="If True, matched games are added automatically. If False, queued for review.",
    )
    enabled: Optional[bool] = Field(
        default=None, description="Whether sync is enabled for this platform"
    )


class SyncConfigCreateRequest(BaseModel):
    """Request model for creating sync configuration (if it doesn't exist)."""

    frequency: SyncFrequency = Field(
        default=SyncFrequency.MANUAL,
        description="Sync frequency (manual, hourly, daily, weekly)",
    )
    auto_add: bool = Field(
        default=False,
        description="If True, matched games are added automatically. If False, queued for review.",
    )
    enabled: bool = Field(default=False, description="Whether sync is enabled")


class ManualSyncTriggerResponse(BaseModel):
    """Response model for triggering a manual sync."""

    message: str
    job_id: str
    platform: str
    status: str = Field(default="queued", description="Initial job status")


class SyncStatusResponse(BaseModel):
    """Response model for sync status check."""

    platform: str
    is_syncing: bool = Field(
        description="Whether a sync is currently in progress for this platform"
    )
    last_synced_at: Optional[datetime] = None
    active_job_id: Optional[str] = Field(
        default=None, description="ID of the active sync job if syncing"
    )

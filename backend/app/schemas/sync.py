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
    PSN = "psn"


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
    last_synced_at: Optional[datetime] = Field(
        default=None, description="Timestamp of last successful sync"
    )
    created_at: datetime
    updated_at: datetime
    is_configured: bool = Field(
        default=False,
        description="Whether platform credentials have been verified"
    )


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


class SteamVerifyRequest(BaseModel):
    """Request model for verifying Steam credentials."""

    steam_id: str = Field(
        description="Steam ID (17 digits starting with 7656119)"
    )
    web_api_key: str = Field(
        description="Steam Web API key (32 alphanumeric characters)"
    )


class SteamVerifyResponse(BaseModel):
    """Response model for Steam verification result."""

    valid: bool = Field(description="Whether the credentials are valid")
    steam_username: Optional[str] = Field(
        default=None,
        description="Steam display name if verification succeeded"
    )
    error: Optional[str] = Field(
        default=None,
        description="Error code if verification failed"
    )


class EpicAuthStartResponse(BaseModel):
    """Response when starting Epic authentication."""

    auth_url: str = Field(description="URL for user to visit and authenticate")
    instructions: str = Field(description="Instructions for completing authentication")


class EpicAuthCompleteRequest(BaseModel):
    """Request to complete Epic authentication with code."""

    code: str = Field(description="Authorization code from Epic")


class EpicAuthCompleteResponse(BaseModel):
    """Response after completing Epic authentication."""

    valid: bool = Field(description="Whether authentication succeeded")
    display_name: Optional[str] = Field(
        default=None,
        description="Epic display name if authentication succeeded"
    )
    error: Optional[str] = Field(
        default=None,
        description="Error code if authentication failed"
    )


class EpicAuthCheckResponse(BaseModel):
    """Response for Epic authentication status check."""

    is_authenticated: bool = Field(description="Whether user is authenticated")
    display_name: Optional[str] = Field(
        default=None,
        description="Epic display name if authenticated"
    )


class PSNConfigureRequest(BaseModel):
    """Request to configure PSN sync with NPSSO token."""
    npsso_token: str = Field(
        ...,
        min_length=64,
        max_length=64,
        description="64-character NPSSO token from PlayStation.com"
    )


class PSNConfigureResponse(BaseModel):
    """Response after configuring PSN sync."""
    success: bool
    online_id: str
    account_id: str
    region: str
    message: str


class PSNStatusResponse(BaseModel):
    """PSN connection status."""
    is_configured: bool
    online_id: Optional[str] = None
    account_id: Optional[str] = None
    region: Optional[str] = None
    token_expired: bool = False

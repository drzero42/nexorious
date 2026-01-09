"""
Pydantic schemas for collection export API.

Provides request/response models for exporting user collections to JSON and CSV formats.
Export jobs are tracked using the unified Job model.
"""

from pydantic import BaseModel, Field, ConfigDict
from typing import Optional, List, Dict, Any
from datetime import datetime, date
from enum import Enum


class ExportFormat(str, Enum):
    """Supported export formats."""

    JSON = "json"
    CSV = "csv"


class ExportJobCreatedResponse(BaseModel):
    """Response when an export job is created."""

    model_config = ConfigDict(from_attributes=True)

    job_id: str = Field(..., description="ID of the created export job")
    status: str = Field(default="pending", description="Initial job status")
    message: str = Field(..., description="Success message")
    estimated_items: int = Field(default=0, description="Estimated number of items to export")


class ExportDownloadResponse(BaseModel):
    """Response for export download metadata."""

    model_config = ConfigDict(from_attributes=True)

    job_id: str
    status: str
    file_path: Optional[str] = None
    download_url: Optional[str] = None
    file_size: Optional[int] = None
    format: ExportFormat
    created_at: datetime
    completed_at: Optional[datetime] = None
    expires_at: Optional[datetime] = None


# Export data schemas (for JSON exports)


class ExportPlatformData(BaseModel):
    """Platform data in export format."""

    platform_id: Optional[str] = None
    platform_name: Optional[str] = None
    storefront_id: Optional[str] = None
    storefront_name: Optional[str] = None
    store_game_id: Optional[str] = None
    store_url: Optional[str] = None
    is_available: bool = True
    hours_played: int = 0
    ownership_status: str = "owned"
    acquired_date: Optional[date] = None


class ExportTagData(BaseModel):
    """Tag data in export format."""

    name: str
    color: Optional[str] = None


class ExportGameData(BaseModel):
    """Game data in export format (for JSON exports)."""

    # IGDB data
    igdb_id: int = Field(..., description="IGDB game ID for reliable re-import")
    title: str
    release_year: Optional[int] = None

    # User data
    play_status: str
    personal_rating: Optional[float] = None
    is_loved: bool = False
    hours_played: int = 0
    personal_notes: Optional[str] = None

    # Relationships
    platforms: List[ExportPlatformData] = Field(default_factory=list)
    tags: List[ExportTagData] = Field(default_factory=list)

    # Timestamps
    created_at: datetime
    updated_at: datetime


class ExportWishlistItem(BaseModel):
    """Wishlist item in export format (for JSON exports)."""

    igdb_id: int = Field(..., description="IGDB game ID for reliable re-import")
    title: str
    release_year: Optional[int] = None
    added_at: datetime = Field(..., description="When the game was added to wishlist")


class NexoriousExportData(BaseModel):
    """Complete Nexorious JSON export format."""

    export_version: str = Field(default="1.2", description="Export format version")
    export_date: datetime = Field(..., description="When the export was created")
    user_id: str = Field(..., description="User ID (for reference only)")

    # Statistics
    total_games: int
    total_wishlist: int = Field(default=0, description="Total wishlist items")
    export_stats: Dict[str, Any] = Field(
        default_factory=dict, description="Summary statistics about the export"
    )

    # Game data
    games: List[ExportGameData]
    wishlist: List[ExportWishlistItem] = Field(
        default_factory=list, description="Games on user's wishlist"
    )


# CSV export row schema


class CsvExportRow(BaseModel):
    """Single row for CSV export."""

    igdb_id: int
    title: str
    release_year: Optional[int] = None
    play_status: str
    personal_rating: Optional[float] = None
    is_loved: bool = False
    hours_played: int = 0
    personal_notes: Optional[str] = None
    platforms: str = ""  # Comma-separated platform names
    storefronts: str = ""  # Comma-separated storefront names
    ownership_statuses: str = ""  # Comma-separated ownership statuses per platform
    acquired_dates: str = ""  # Comma-separated acquired dates per platform
    tags: str = ""  # Comma-separated tag names
    created_at: str
    updated_at: str

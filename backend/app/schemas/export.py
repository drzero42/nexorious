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


class ExportScope(str, Enum):
    """What data to export."""

    COLLECTION = "collection"
    WISHLIST = "wishlist"


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
    scope: ExportScope
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
    ownership_status: str
    play_status: str
    personal_rating: Optional[float] = None
    is_loved: bool = False
    hours_played: int = 0
    personal_notes: Optional[str] = None
    acquired_date: Optional[date] = None

    # Relationships
    platforms: List[ExportPlatformData] = Field(default_factory=list)
    tags: List[ExportTagData] = Field(default_factory=list)

    # Timestamps
    created_at: datetime
    updated_at: datetime


class NexoriousExportData(BaseModel):
    """Complete Nexorious JSON export format."""

    export_version: str = Field(default="1.0", description="Export format version")
    export_date: datetime = Field(..., description="When the export was created")
    export_scope: ExportScope
    user_id: str = Field(..., description="User ID (for reference only)")

    # Statistics
    total_games: int
    export_stats: Dict[str, Any] = Field(
        default_factory=dict, description="Summary statistics about the export"
    )

    # Game data
    games: List[ExportGameData]


# CSV export row schema


class CsvExportRow(BaseModel):
    """Single row for CSV export."""

    igdb_id: int
    title: str
    release_year: Optional[int] = None
    ownership_status: str
    play_status: str
    personal_rating: Optional[float] = None
    is_loved: bool = False
    hours_played: int = 0
    personal_notes: Optional[str] = None
    acquired_date: Optional[str] = None
    platforms: str = ""  # Comma-separated platform names
    storefronts: str = ""  # Comma-separated storefront names
    tags: str = ""  # Comma-separated tag names
    created_at: str
    updated_at: str

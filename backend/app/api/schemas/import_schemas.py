"""
Schemas for import API endpoints.
"""

from pydantic import BaseModel, Field
from typing import List, Optional, Dict, Any
from datetime import datetime


class ImportSourceInfo(BaseModel):
    """Information about an import source."""
    name: str = Field(..., description="Source identifier (e.g., 'steam', 'epic')")
    display_name: str = Field(..., description="Human-readable source name")
    description: str = Field(..., description="Description of the import source")
    icon: str = Field(..., description="Icon identifier for the source")
    available: bool = Field(..., description="Whether the source is available for import")
    configured: bool = Field(..., description="Whether the user has configured this source")
    status: str = Field(..., description="Source status: 'available', 'coming_soon', 'disabled'")


class ImportSourcesResponse(BaseModel):
    """Response for listing available import sources."""
    sources: List[ImportSourceInfo] = Field(..., description="List of available import sources")


class ImportJobResponse(BaseModel):
    """Response for import job information."""
    id: str = Field(..., description="Job ID")
    source: str = Field(..., description="Import source")
    job_type: str = Field(..., description="Type of import job")
    status: str = Field(..., description="Job status")
    progress: int = Field(..., description="Progress percentage (0-100)")
    total_items: int = Field(..., description="Total items to process")
    processed_items: int = Field(..., description="Items processed so far")
    successful_items: int = Field(..., description="Items processed successfully")
    failed_items: int = Field(..., description="Items that failed processing")
    created_at: datetime = Field(..., description="When the job was created")
    started_at: Optional[datetime] = Field(None, description="When the job started processing")
    completed_at: Optional[datetime] = Field(None, description="When the job completed")
    error_message: Optional[str] = Field(None, description="Error message if job failed")
    metadata: Optional[Dict[str, Any]] = Field(None, description="Additional job metadata")


class ImportJobsListResponse(BaseModel):
    """Response for listing import jobs."""
    jobs: List[ImportJobResponse] = Field(..., description="List of import jobs")
    total: int = Field(..., description="Total number of jobs")
    offset: int = Field(..., description="Pagination offset")
    limit: int = Field(..., description="Pagination limit")


class ImportJobCancelResponse(BaseModel):
    """Response for cancelling an import job."""
    message: str = Field(..., description="Success message")
    job_id: str = Field(..., description="ID of the cancelled job")
    cancelled: bool = Field(..., description="Whether the job was successfully cancelled")


class ImportHistoryResponse(BaseModel):
    """Response for import history."""
    history: List[ImportJobResponse] = Field(..., description="List of completed import jobs")
    total: int = Field(..., description="Total number of completed jobs")
    offset: int = Field(..., description="Pagination offset")
    limit: int = Field(..., description="Pagination limit")


# Source-specific config schemas
class SourceConfigRequest(BaseModel):
    """Base request for source configuration."""
    pass


class SourceConfigResponse(BaseModel):
    """Base response for source configuration."""
    is_configured: bool = Field(..., description="Whether source is configured")
    is_verified: bool = Field(..., description="Whether source configuration is verified")
    configured_at: Optional[datetime] = Field(None, description="When source was configured")
    last_import: Optional[datetime] = Field(None, description="When last import was performed")


class VerificationRequest(BaseModel):
    """Base request for verifying source configuration."""
    pass


class VerificationResponse(BaseModel):
    """Base response for source verification."""
    is_valid: bool = Field(..., description="Whether configuration is valid")
    error_message: Optional[str] = Field(None, description="Error message if verification failed")
    additional_data: Optional[Dict[str, Any]] = Field(None, description="Additional verification data")


class LibraryPreviewResponse(BaseModel):
    """Base response for library preview."""
    total_games: int = Field(..., description="Total games found in library")
    preview_games: List[Dict[str, Any]] = Field(..., description="Sample of games from library")
    source_info: Optional[Dict[str, Any]] = Field(None, description="Additional source information")


class ImportGameResponse(BaseModel):
    """Response for imported game information."""
    id: str = Field(..., description="Game ID in import system")
    external_id: str = Field(..., description="External game ID (AppID, etc.)")
    name: str = Field(..., description="Game name")
    igdb_id: Optional[str] = Field(None, description="IGDB game ID if matched")
    igdb_title: Optional[str] = Field(None, description="IGDB game title if matched")
    game_id: Optional[str] = Field(None, description="Main games table ID if synced")
    user_game_id: Optional[str] = Field(None, description="User games table ID if synced")
    ignored: bool = Field(..., description="Whether game is ignored")
    created_at: datetime = Field(..., description="When game was imported")
    updated_at: datetime = Field(..., description="When game was last updated")


class ImportGamesList(BaseModel):
    """Response for listing imported games."""
    games: List[ImportGameResponse] = Field(..., description="List of imported games")
    total: int = Field(..., description="Total number of games")
    offset: int = Field(..., description="Pagination offset")
    limit: int = Field(..., description="Pagination limit")


class ImportStartResponse(BaseModel):
    """Response for starting library import."""
    message: str = Field(..., description="Success message")
    job_id: str = Field(..., description="ID of the import job")
    started: bool = Field(..., description="Whether import was started successfully")


class GameMatchRequest(BaseModel):
    """Request for matching game to IGDB."""
    igdb_id: Optional[str] = Field(None, description="IGDB ID to match to, or null to clear match")


class GameMatchResponse(BaseModel):
    """Response for game matching operation."""
    message: str = Field(..., description="Success message")
    game: ImportGameResponse = Field(..., description="Updated game information")


class GameSyncResponse(BaseModel):
    """Response for game sync operation."""
    message: str = Field(..., description="Success message")
    game: ImportGameResponse = Field(..., description="Updated game information")
    user_game_id: Optional[str] = Field(None, description="User game ID if synced")
    action: str = Field(..., description="Action performed: 'created_new', 'updated_existing'")


class GameIgnoreResponse(BaseModel):
    """Response for game ignore operation."""
    message: str = Field(..., description="Success message")
    game: ImportGameResponse = Field(..., description="Updated game information")
    ignored: bool = Field(..., description="New ignore status")


class BulkOperationResponse(BaseModel):
    """Response for bulk operations."""
    message: str = Field(..., description="Success message")
    total_processed: int = Field(..., description="Total items processed")
    successful_operations: int = Field(..., description="Successful operations")
    failed_operations: int = Field(..., description="Failed operations")
    skipped_items: int = Field(default=0, description="Items skipped")
    errors: List[str] = Field(default_factory=list, description="List of errors encountered")
"""
Darkadia CSV import configuration schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, field_validator
from typing import Optional, Dict, Any, List
from datetime import datetime


class DarkadiaConfigRequest(BaseModel):
    """Request schema for setting Darkadia CSV configuration."""
    csv_file_path: str = Field(..., min_length=1, description="Path to the Darkadia CSV file")
    
    @field_validator('csv_file_path')
    @classmethod
    def validate_file_path(cls, v):
        """Validate CSV file path format."""
        if not v.strip():
            raise ValueError('CSV file path cannot be empty')
        
        if not v.lower().endswith('.csv'):
            raise ValueError('File must have a .csv extension')
        
        return v.strip()


class DarkadiaConfigResponse(BaseModel):
    """Response schema for Darkadia configuration."""
    has_csv_file: bool = Field(..., description="Whether user has configured a CSV file")
    csv_file_path: Optional[str] = Field(None, description="Path to the configured CSV file")
    file_exists: bool = Field(default=False, description="Whether the CSV file exists and is accessible")
    file_hash: Optional[str] = Field(None, description="Hash of the CSV file for change detection")
    configured_at: Optional[datetime] = Field(None, description="When the Darkadia configuration was last updated")


class DarkadiaVerificationRequest(BaseModel):
    """Request schema for verifying Darkadia CSV file."""
    csv_file_path: str = Field(..., min_length=1, description="Path to the CSV file to verify")
    
    @field_validator('csv_file_path')
    @classmethod
    def validate_file_path(cls, v):
        """Validate CSV file path format."""
        if not v.strip():
            raise ValueError('CSV file path cannot be empty')
        
        if not v.lower().endswith('.csv'):
            raise ValueError('File must have a .csv extension')
        
        return v.strip()


class DarkadiaUploadResponse(BaseModel):
    """Response schema for CSV file upload."""
    message: str = Field(..., description="Status message about the upload")
    file_id: str = Field(..., description="Unique identifier for the uploaded file")
    total_games: int = Field(..., description="Estimated number of games in the CSV")
    file_path: str = Field(..., description="Path where the file was stored")
    file_size: int = Field(..., description="Size of the uploaded file in bytes")
    preview_games: List[Dict[str, Any]] = Field(..., description="Preview of first few games from CSV")


class DarkadiaGamePreview(BaseModel):
    """Schema for preview game information from CSV."""
    name: str = Field(..., description="Game name from CSV")
    platforms: str = Field(..., description="Platforms from CSV")
    rating: str = Field(..., description="Rating from CSV")
    played: bool = Field(..., description="Whether game has been played")
    finished: bool = Field(..., description="Whether game has been finished")


class DarkadiaLibraryPreview(BaseModel):
    """Response schema for Darkadia library preview."""
    total_games_estimate: int = Field(..., description="Estimated total number of games")
    preview_games: List[DarkadiaGamePreview] = Field(..., description="Preview of first few games")
    file_info: Dict[str, Any] = Field(..., description="Information about the CSV file")


class DarkadiaGameResponse(BaseModel):
    """Response schema for Darkadia game from database."""
    id: str = Field(..., description="Darkadia game UUID")
    external_id: str = Field(..., description="External ID (row number from CSV)")
    name: str = Field(..., description="Game name from CSV")
    igdb_id: Optional[str] = Field(None, description="IGDB ID when matched to games table")
    igdb_title: Optional[str] = Field(None, description="Game title from IGDB when matched")
    game_id: Optional[str] = Field(None, description="Game ID when synced to user collection")
    user_game_id: Optional[str] = Field(None, description="UserGame ID when synced to user collection")
    ignored: bool = Field(..., description="Whether user has marked this game as ignored")
    created_at: datetime = Field(..., description="When the Darkadia game was imported")
    updated_at: datetime = Field(..., description="When the Darkadia game was last updated")


class DarkadiaGamesListResponse(BaseModel):
    """Response schema for listing Darkadia games."""
    total: int = Field(..., description="Total number of Darkadia games")
    games: List[DarkadiaGameResponse] = Field(..., description="List of Darkadia games")


class DarkadiaImportStartResponse(BaseModel):
    """Response schema for starting Darkadia CSV import."""
    message: str = Field(..., description="Status message about the import")
    imported_count: int = Field(..., description="Number of games imported")
    skipped_count: int = Field(..., description="Number of games skipped (already imported)")
    auto_matched_count: int = Field(..., description="Number of games auto-matched to IGDB")
    total_games: int = Field(..., description="Total number of games in CSV")
    errors: List[str] = Field(default=[], description="List of errors encountered during import")


class DarkadiaGameMatchRequest(BaseModel):
    """Request schema for matching Darkadia game to IGDB game."""
    igdb_id: Optional[str] = Field(None, description="IGDB game ID to match to. Set to null to clear existing match.")


class DarkadiaGameMatchResponse(BaseModel):
    """Response schema for Darkadia game IGDB matching."""
    message: str = Field(..., description="Status message about the matching operation")
    game: DarkadiaGameResponse = Field(..., description="Updated Darkadia game information")


class DarkadiaGameSyncResponse(BaseModel):
    """Response schema for Darkadia game sync to main collection."""
    message: str = Field(..., description="Status message about the sync operation")
    game: DarkadiaGameResponse = Field(..., description="Updated Darkadia game information")
    user_game_id: str = Field(..., description="ID of the created or updated user game in main collection")
    action: str = Field(..., description="Action taken: 'created_new' or 'updated_existing'")


class DarkadiaGameIgnoreResponse(BaseModel):
    """Response schema for Darkadia game ignore/un-ignore operation."""
    message: str = Field(..., description="Status message about the ignore/un-ignore operation")
    game: DarkadiaGameResponse = Field(..., description="Updated Darkadia game information")
    ignored: bool = Field(..., description="The new ignored status for clarity")


class DarkadiaGamesBulkSyncResponse(BaseModel):
    """Response schema for bulk Darkadia games sync operation."""
    message: str = Field(..., description="Overall status message about the bulk sync operation")
    total_processed: int = Field(..., description="Total number of Darkadia games processed")
    successful_syncs: int = Field(..., description="Number of games successfully synced to collection")
    failed_syncs: int = Field(..., description="Number of games that failed to sync")
    errors: List[str] = Field(default=[], description="List of error messages for failed syncs")


class DarkadiaGamesBulkUnignoreResponse(BaseModel):
    """Response schema for bulk Darkadia games unignore operation."""
    message: str = Field(..., description="Overall status message about the bulk unignore operation")
    total_processed: int = Field(..., description="Total number of Darkadia games processed")
    successful_unignores: int = Field(..., description="Number of games successfully unignored")
    failed_unignores: int = Field(..., description="Number of games that failed to unignore")
    errors: List[str] = Field(default=[], description="List of error messages for failed unignores")


class DarkadiaGamesBulkUnmatchResponse(BaseModel):
    """Response schema for bulk Darkadia games unmatch operation."""
    message: str = Field(..., description="Overall status message about the bulk unmatch operation")
    total_processed: int = Field(..., description="Total number of Darkadia games processed")
    successful_unmatches: int = Field(..., description="Number of games successfully unmatched")
    failed_unmatches: int = Field(..., description="Number of games that failed to unmatch")
    errors: List[str] = Field(default=[], description="List of error messages for failed unmatches")


class DarkadiaGamesAutoMatchResponse(BaseModel):
    """Response schema for auto-matching operation."""
    message: str = Field(..., description="Overall status message about the auto-matching operation")
    total_processed: int = Field(..., description="Total number of Darkadia games processed for matching")
    successful_matches: int = Field(..., description="Number of games successfully matched to IGDB")
    failed_matches: int = Field(..., description="Number of games that failed to match")
    errors: List[str] = Field(default=[], description="List of error messages for failed matches")


class DarkadiaGameAutoMatchSingleResponse(BaseModel):
    """Response schema for single Darkadia game auto-matching operation."""
    message: str = Field(..., description="Status message about the auto-matching operation")
    game: DarkadiaGameResponse = Field(..., description="Updated Darkadia game information")
    matched: bool = Field(..., description="Whether the game was successfully auto-matched")
    confidence: Optional[float] = Field(None, description="Matching confidence score if matched")
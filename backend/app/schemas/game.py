"""
Game-related schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, ConfigDict
from typing import Optional, List, Dict, Any
from datetime import date
from .common import TimestampMixin






class GameResponse(BaseModel, TimestampMixin):
    """Response schema for game data."""
    id: int = Field(..., description="Game ID (IGDB ID as primary key)")
    title: str
    description: Optional[str]
    genre: Optional[str]
    developer: Optional[str]
    publisher: Optional[str]
    release_date: Optional[date]
    cover_art_url: Optional[str]
    rating_average: Optional[float]
    rating_count: int
    game_metadata: str
    estimated_playtime_hours: Optional[int]
    howlongtobeat_main: Optional[int]
    howlongtobeat_extra: Optional[int]
    howlongtobeat_completionist: Optional[int]
    igdb_slug: Optional[str]
    igdb_platform_ids: Optional[str]
    igdb_platform_names: Optional[str] = None
    game_modes: Optional[str] = None
    themes: Optional[str] = None
    player_perspectives: Optional[str] = None

    model_config = ConfigDict(from_attributes=True)


class GameSearchRequest(BaseModel):
    """Request schema for game search."""
    q: Optional[str] = Field(None, description="Search query")
    genre: Optional[str] = Field(None, description="Filter by genre")
    developer: Optional[str] = Field(None, description="Filter by developer")
    publisher: Optional[str] = Field(None, description="Filter by publisher")
    release_year: Optional[int] = Field(None, description="Filter by release year")


class GameListResponse(BaseModel):
    """Response schema for game list."""
    games: List[GameResponse]
    total: int
    page: int
    per_page: int
    pages: int



class IGDBSearchRequest(BaseModel):
    """Request schema for IGDB game search."""
    query: str = Field(..., min_length=1, description="Game title to search for")
    limit: Optional[int] = Field(default=10, ge=1, le=50, description="Maximum number of results")


class IGDBGameCandidate(BaseModel):
    """Schema for IGDB game search candidate.
    
    Note: Time-to-beat fields are null in search responses for performance optimization.
    Complete metadata including time-to-beat data is fetched during game import.
    """
    igdb_id: int = Field(..., gt=0, description="IGDB unique identifier for the game")
    igdb_slug: Optional[str] = Field(None, description="IGDB URL slug for generating game links")
    title: str = Field(..., description="Game title from IGDB")
    release_date: Optional[date] = Field(None, description="Game release date")
    cover_art_url: Optional[str] = Field(None, description="URL to game cover art image")
    description: Optional[str] = Field(None, description="Game description/summary")
    platforms: List[str] = Field(default_factory=list, description="List of platform names where the game is available")
    howlongtobeat_main: Optional[int] = Field(None, description="Main story completion time in hours (null in search results, populated during import for performance)")
    howlongtobeat_extra: Optional[int] = Field(None, description="Main story + extras completion time in hours (null in search results, populated during import for performance)")
    howlongtobeat_completionist: Optional[int] = Field(None, description="Completionist time in hours (null in search results, populated during import for performance)")


class IGDBSearchResponse(BaseModel):
    """Response schema for IGDB search results."""
    games: List[IGDBGameCandidate]
    total: int


class GameMetadataAcceptRequest(BaseModel):
    """Request schema for accepting IGDB metadata."""
    igdb_id: int = Field(..., gt=0, description="Selected IGDB game ID")
    accept_metadata: bool = Field(default=True, description="Whether to accept the metadata")
    custom_overrides: Optional[Dict[str, Any]] = Field(default_factory=dict, description="Custom field overrides")
    
    # Allow direct field overrides for convenience
    title: Optional[str] = Field(None, description="Override title")
    description: Optional[str] = Field(None, description="Override description")
    genre: Optional[str] = Field(None, description="Override genre")
    developer: Optional[str] = Field(None, description="Override developer")
    publisher: Optional[str] = Field(None, description="Override publisher")
    
    def model_post_init(self, __context):
        """Merge direct field overrides into custom_overrides."""
        if not self.custom_overrides:
            self.custom_overrides = {}
        
        # Add direct field overrides to custom_overrides
        for field_name in ['title', 'description', 'genre', 'developer', 'publisher']:
            field_value = getattr(self, field_name, None)
            if field_value is not None:
                self.custom_overrides[field_name] = field_value


class MetadataStatusResponse(BaseModel):
    """Response schema for metadata completeness status."""
    completeness_percentage: float
    missing_essential: List[str]
    missing_optional: List[str]
    total_fields: int
    filled_fields: int


class MetadataRefreshRequest(BaseModel):
    """Request schema for metadata refresh operations."""
    fields: Optional[List[str]] = Field(default=None, description="Specific fields to refresh (if None, refresh all)")
    force: bool = Field(default=False, description="Force refresh even if metadata is complete")


class MetadataRefreshResponse(BaseModel):
    """Response schema for metadata refresh operations."""
    success: bool
    updated_fields: List[str]
    errors: List[str]
    game: GameResponse


class MetadataPopulateRequest(BaseModel):
    """Request schema for metadata population operations."""
    populate_missing_only: bool = Field(default=True, description="Only populate missing fields")
    fields: Optional[List[str]] = Field(default=None, description="Specific fields to populate (if None, populate all missing)")


class MetadataPopulateResponse(BaseModel):
    """Response schema for metadata population operations."""
    success: bool
    populated_fields: List[str]
    errors: List[str]
    game: GameResponse


class MetadataComparisonResponse(BaseModel):
    """Response schema for metadata comparison."""
    has_differences: bool
    differences: Dict[str, Dict[str, Any]]
    recommendations: List[str]


class BulkMetadataRequest(BaseModel):
    """Request schema for bulk metadata operations."""
    game_ids: List[int] = Field(..., min_length=1, max_length=100, description="List of game IDs to process")
    operation: str = Field(..., pattern="^(refresh|populate)$", description="Operation type")
    options: Optional[Dict[str, Any]] = Field(default_factory=dict, description="Operation-specific options")


class CoverArtResult(BaseModel):
    """Result of a cover art download operation."""
    game_id: int
    success: bool = False
    fields: List[str] = Field(default_factory=list)
    message: Optional[str] = None

    model_config = ConfigDict(extra="forbid")


class BulkMetadataResponse(BaseModel):
    """Response schema for bulk metadata operations."""
    total_games: int
    processed_games: int
    successful_operations: int
    failed_operations: int
    results: List[Dict[str, Any]]
    errors: List[str]


class BulkCoverArtDownloadRequest(BaseModel):
    """Request schema for bulk cover art download operations."""
    game_ids: List[int] = Field(..., min_length=1, max_length=100, description="List of game IDs to process")
    skip_existing: bool = Field(default=True, description="Skip games that already have local cover art")
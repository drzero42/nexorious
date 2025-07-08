"""
Game-related schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, HttpUrl
from typing import Optional, List, Dict, Any
from datetime import date, datetime
from .common import TimestampMixin


class GameCreateRequest(BaseModel):
    """Request schema for creating a new game."""
    title: str = Field(..., max_length=500, description="Game title")
    description: Optional[str] = Field(None, description="Game description")
    genre: Optional[str] = Field(None, max_length=200, description="Game genre")
    developer: Optional[str] = Field(None, max_length=200, description="Game developer")
    publisher: Optional[str] = Field(None, max_length=200, description="Game publisher")
    release_date: Optional[date] = Field(None, description="Game release date")
    cover_art_url: Optional[HttpUrl] = Field(None, description="Cover art URL")
    estimated_playtime_hours: Optional[int] = Field(None, ge=0, description="Estimated playtime in hours")
    howlongtobeat_main: Optional[int] = Field(None, ge=0, description="Main story hours from HowLongToBeat")
    howlongtobeat_extra: Optional[int] = Field(None, ge=0, description="Main + extras hours from HowLongToBeat")
    howlongtobeat_completionist: Optional[int] = Field(None, ge=0, description="Completionist hours from HowLongToBeat")
    igdb_id: Optional[str] = Field(None, max_length=50, description="IGDB identifier")
    metadata: Optional[Dict[str, Any]] = Field(default_factory=dict, description="Additional metadata")


class GameUpdateRequest(BaseModel):
    """Request schema for updating a game."""
    title: Optional[str] = Field(None, max_length=500, description="Game title")
    description: Optional[str] = Field(None, description="Game description")
    genre: Optional[str] = Field(None, max_length=200, description="Game genre")
    developer: Optional[str] = Field(None, max_length=200, description="Game developer")
    publisher: Optional[str] = Field(None, max_length=200, description="Game publisher")
    release_date: Optional[date] = Field(None, description="Game release date")
    cover_art_url: Optional[HttpUrl] = Field(None, description="Cover art URL")
    estimated_playtime_hours: Optional[int] = Field(None, ge=0, description="Estimated playtime in hours")
    howlongtobeat_main: Optional[int] = Field(None, ge=0, description="Main story hours from HowLongToBeat")
    howlongtobeat_extra: Optional[int] = Field(None, ge=0, description="Main + extras hours from HowLongToBeat")
    howlongtobeat_completionist: Optional[int] = Field(None, ge=0, description="Completionist hours from HowLongToBeat")
    igdb_id: Optional[str] = Field(None, max_length=50, description="IGDB identifier")
    metadata: Optional[Dict[str, Any]] = Field(None, description="Additional metadata")


class GameResponse(BaseModel, TimestampMixin):
    """Response schema for game data."""
    id: str
    title: str
    slug: str
    description: Optional[str]
    genre: Optional[str]
    developer: Optional[str]
    publisher: Optional[str]
    release_date: Optional[date]
    cover_art_url: Optional[str]
    rating_average: Optional[float]
    rating_count: int
    metadata: Dict[str, Any]
    estimated_playtime_hours: Optional[int]
    howlongtobeat_main: Optional[int]
    howlongtobeat_extra: Optional[int]
    howlongtobeat_completionist: Optional[int]
    igdb_id: Optional[str]
    is_verified: bool

    class Config:
        from_attributes = True


class GameSearchRequest(BaseModel):
    """Request schema for game search."""
    q: Optional[str] = Field(None, description="Search query")
    genre: Optional[str] = Field(None, description="Filter by genre")
    developer: Optional[str] = Field(None, description="Filter by developer")
    publisher: Optional[str] = Field(None, description="Filter by publisher")
    release_year: Optional[int] = Field(None, description="Filter by release year")
    is_verified: Optional[bool] = Field(None, description="Filter by verification status")


class GameListResponse(BaseModel):
    """Response schema for game list."""
    games: List[GameResponse]
    total: int
    page: int
    per_page: int
    pages: int


class GameAliasResponse(BaseModel):
    """Response schema for game aliases."""
    id: str
    alias_title: str
    source: Optional[str]
    created_at: datetime

    class Config:
        from_attributes = True


class IGDBSearchRequest(BaseModel):
    """Request schema for IGDB game search."""
    title: str = Field(..., min_length=1, description="Game title to search for")
    limit: Optional[int] = Field(default=10, ge=1, le=50, description="Maximum number of results")


class IGDBGameCandidate(BaseModel):
    """Schema for IGDB game search candidate."""
    igdb_id: str
    title: str
    release_date: Optional[date]
    cover_art_url: Optional[str]
    description: Optional[str]
    platforms: List[str]


class IGDBSearchResponse(BaseModel):
    """Response schema for IGDB search results."""
    candidates: List[IGDBGameCandidate]
    total: int


class GameMetadataAcceptRequest(BaseModel):
    """Request schema for accepting IGDB metadata."""
    igdb_id: str = Field(..., description="Selected IGDB game ID")
    accept_metadata: bool = Field(default=True, description="Whether to accept the metadata")
    custom_overrides: Optional[Dict[str, Any]] = Field(default_factory=dict, description="Custom field overrides")
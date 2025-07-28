"""
User game collection-related schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, HttpUrl, ConfigDict
from typing import Optional, List
from datetime import date, datetime
from enum import Enum
from .common import TimestampMixin
from .game import GameResponse
from .platform import PlatformResponse, StorefrontResponse


class OwnershipStatus(str, Enum):
    """Ownership status enumeration."""
    OWNED = "owned"
    BORROWED = "borrowed"
    RENTED = "rented"
    SUBSCRIPTION = "subscription"


class PlayStatus(str, Enum):
    """Play status enumeration with completion levels."""
    NOT_STARTED = "not_started"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    MASTERED = "mastered"
    DOMINATED = "dominated"
    SHELVED = "shelved"
    DROPPED = "dropped"
    REPLAY = "replay"


class UserGameCreateRequest(BaseModel):
    """Request schema for adding a game to user's collection."""
    game_id: str = Field(..., description="Game ID to add to collection")
    ownership_status: OwnershipStatus = Field(default=OwnershipStatus.OWNED, description="Ownership status")
    personal_rating: Optional[float] = Field(None, ge=1, le=5, description="Personal rating (1-5)")
    is_loved: bool = Field(default=False, description="Whether game is marked as loved")
    play_status: PlayStatus = Field(default=PlayStatus.NOT_STARTED, description="Current play status")
    hours_played: int = Field(default=0, ge=0, description="Hours played")
    personal_notes: Optional[str] = Field(None, description="Personal notes about the game")
    acquired_date: Optional[date] = Field(None, description="Date when game was acquired")
    platforms: Optional[List[str]] = Field(default_factory=list, description="Platform IDs where game is owned")


class UserGameUpdateRequest(BaseModel):
    """Request schema for updating user's game collection entry."""
    ownership_status: Optional[OwnershipStatus] = Field(None, description="Ownership status")
    personal_rating: Optional[float] = Field(None, ge=1, le=5, description="Personal rating (1-5)")
    is_loved: Optional[bool] = Field(None, description="Whether game is marked as loved")
    play_status: Optional[PlayStatus] = Field(None, description="Current play status")
    hours_played: Optional[int] = Field(None, ge=0, description="Hours played")
    personal_notes: Optional[str] = Field(None, description="Personal notes about the game")
    acquired_date: Optional[date] = Field(None, description="Date when game was acquired")


class ProgressUpdateRequest(BaseModel):
    """Request schema for updating game progress."""
    play_status: PlayStatus = Field(..., description="Current play status")
    hours_played: Optional[int] = Field(None, ge=0, description="Hours played")
    personal_notes: Optional[str] = Field(None, description="Personal notes about the game")


class UserGamePlatformCreateRequest(BaseModel):
    """Request schema for adding platform association to user game."""
    platform_id: str = Field(..., description="Platform ID")
    storefront_id: Optional[str] = Field(None, description="Storefront ID")
    store_game_id: Optional[str] = Field(None, max_length=200, description="Game ID in store")
    store_url: Optional[HttpUrl] = Field(None, description="Store URL for game")
    is_available: bool = Field(default=True, description="Whether the game is available on this platform")


class UserGamePlatformResponse(BaseModel, TimestampMixin):
    """Response schema for user game platform association."""
    id: str
    platform_id: str
    storefront_id: Optional[str]
    platform: PlatformResponse
    storefront: Optional[StorefrontResponse]
    store_game_id: Optional[str]
    store_url: Optional[str]
    is_available: bool

    model_config = ConfigDict(from_attributes=True)


class UserGameResponse(BaseModel, TimestampMixin):
    """Response schema for user's game collection entry."""
    id: str
    game: GameResponse
    ownership_status: OwnershipStatus
    personal_rating: Optional[float]
    is_loved: bool
    play_status: PlayStatus
    hours_played: int
    personal_notes: Optional[str]
    acquired_date: Optional[date]
    platforms: List[UserGamePlatformResponse]

    model_config = ConfigDict(from_attributes=True)


class UserGameListRequest(BaseModel):
    """Request schema for filtering user's game collection."""
    play_status: Optional[PlayStatus] = Field(None, description="Filter by play status")
    ownership_status: Optional[OwnershipStatus] = Field(None, description="Filter by ownership status")
    is_loved: Optional[bool] = Field(None, description="Filter by loved status")
    platform_id: Optional[str] = Field(None, description="Filter by platform")
    storefront_id: Optional[str] = Field(None, description="Filter by storefront")
    rating_min: Optional[float] = Field(None, ge=1, le=5, description="Minimum rating filter")
    rating_max: Optional[float] = Field(None, ge=1, le=5, description="Maximum rating filter")
    has_notes: Optional[bool] = Field(None, description="Filter by presence of notes")


class UserGameListResponse(BaseModel):
    """Response schema for user's game collection list."""
    user_games: List[UserGameResponse]
    total: int
    page: int
    per_page: int
    pages: int


class BulkUpdateData(BaseModel):
    """Schema for the updates object in bulk operations."""
    play_status: Optional[PlayStatus] = Field(None, description="New play status")
    personal_rating: Optional[float] = Field(None, ge=1, le=5, description="New rating")
    is_loved: Optional[bool] = Field(None, description="New loved status")


class BulkStatusUpdateRequest(BaseModel):
    """Request schema for bulk status updates."""
    user_game_ids: List[str] = Field(..., min_length=1, description="List of user game IDs to update")
    updates: BulkUpdateData = Field(..., description="Updates to apply to the user games")


class CollectionStatsResponse(BaseModel):
    """Response schema for collection statistics."""
    total_games: int
    completion_stats: dict[PlayStatus, int]
    ownership_stats: dict[OwnershipStatus, int]
    platform_stats: dict[str, int]
    genre_stats: dict[str, int]
    by_status: dict[PlayStatus, int]
    by_platform: dict[str, int]
    by_rating: dict[str, int]
    pile_of_shame: int  # not_started games
    completion_rate: float  # percentage of completed games
    average_rating: Optional[float]
    total_hours_played: int
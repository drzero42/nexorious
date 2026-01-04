"""
User game collection-related schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, HttpUrl, ConfigDict, model_validator
from typing import Optional, List, Self
from datetime import date
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
    NO_LONGER_OWNED = "no_longer_owned"


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


class UserGamePlatformCreateRequest(BaseModel):
    """Request schema for adding platform association to user game."""
    platform: str = Field(..., description="Platform slug")
    storefront: Optional[str] = Field(None, description="Storefront slug")
    store_game_id: Optional[str] = Field(None, max_length=200, description="Game ID in store")
    store_url: Optional[HttpUrl] = Field(None, description="Store URL for game")
    is_available: bool = Field(default=True, description="Whether the game is available on this platform")
    hours_played: int = Field(default=0, ge=0, description="Hours played on this storefront")


class UserGameCreateRequest(BaseModel):
    """Request schema for adding a game to user's collection."""
    game_id: int = Field(..., gt=0, description="Game ID to add to collection")
    ownership_status: OwnershipStatus = Field(default=OwnershipStatus.OWNED, description="Ownership status")
    personal_rating: Optional[float] = Field(None, ge=1, le=5, description="Personal rating (1-5)")
    is_loved: bool = Field(default=False, description="Whether game is marked as loved")
    play_status: PlayStatus = Field(default=PlayStatus.NOT_STARTED, description="Current play status")
    hours_played: int = Field(default=0, ge=0, description="Hours played")
    personal_notes: Optional[str] = Field(None, description="Personal notes about the game")
    acquired_date: Optional[date] = Field(None, description="Date when game was acquired")
    platforms: Optional[List[UserGamePlatformCreateRequest]] = Field(default_factory=list, description="Platform associations with complete details")


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


class UserGamePlatformResponse(BaseModel, TimestampMixin):
    """Response schema for user game platform association."""
    id: str
    platform: Optional[str]
    storefront: Optional[str]
    platform_details: Optional[PlatformResponse] = Field(default=None, validation_alias="platform_rel")
    storefront_details: Optional[StorefrontResponse] = Field(default=None, validation_alias="storefront_rel")
    store_game_id: Optional[str]
    store_url: Optional[str]
    is_available: bool
    hours_played: int
    original_platform_name: Optional[str] = None
    original_storefront_name: Optional[str] = None

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

    @model_validator(mode='after')
    def compute_hours_played(self) -> Self:
        """Compute aggregate hours_played from platforms with legacy fallback."""
        platform_hours = sum(p.hours_played for p in self.platforms)
        # If platforms have playtime, use that; otherwise keep legacy value
        if platform_hours > 0:
            self.hours_played = platform_hours
        # else: keep the original hours_played value (legacy fallback)
        return self


class UserGameListRequest(BaseModel):
    """Request schema for filtering user's game collection."""
    play_status: Optional[PlayStatus] = Field(None, description="Filter by play status")
    ownership_status: Optional[OwnershipStatus] = Field(None, description="Filter by ownership status")
    is_loved: Optional[bool] = Field(None, description="Filter by loved status")
    platform: Optional[str] = Field(None, description="Filter by platform slug")
    storefront: Optional[str] = Field(None, description="Filter by storefront slug")
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


class BulkStatusUpdateRequest(BaseModel):
    """Request schema for bulk status updates."""
    user_game_ids: List[str] = Field(..., min_length=1, description="List of user game IDs to update")
    play_status: Optional[PlayStatus] = Field(None, description="New play status")
    personal_rating: Optional[float] = Field(None, ge=1, le=5, description="New rating")
    is_loved: Optional[bool] = Field(None, description="New loved status")
    ownership_status: Optional[OwnershipStatus] = Field(None, description="New ownership status")


class BulkDeleteRequest(BaseModel):
    """Request schema for bulk deletion operations."""
    user_game_ids: List[str] = Field(..., min_length=1, description="List of user game IDs to delete")


class BulkAddPlatformRequest(BaseModel):
    """Request schema for bulk platform addition operations."""
    user_game_ids: List[str] = Field(..., min_length=1, description="List of user game IDs to add platforms to")
    platform_associations: List[UserGamePlatformCreateRequest] = Field(..., min_length=1, description="Platform associations to add")


class BulkRemovePlatformRequest(BaseModel):
    """Request schema for bulk platform removal operations."""
    user_game_ids: List[str] = Field(..., min_length=1, description="List of user game IDs to remove platforms from")
    platform_association_ids: List[str] = Field(..., min_length=1, description="Platform association IDs to remove")


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


class UserGameIdsResponse(BaseModel):
    """Response schema for user game IDs list."""
    ids: List[str] = Field(..., description="List of user game IDs")


class UserGameGenresResponse(BaseModel):
    """Response schema for unique genres in user's collection."""
    genres: List[str] = Field(..., description="List of unique genres sorted alphabetically")


class FilterOptionsResponse(BaseModel):
    """Response schema for filter dropdown options."""
    genres: List[str] = Field(default_factory=list)
    game_modes: List[str] = Field(default_factory=list)
    themes: List[str] = Field(default_factory=list)
    player_perspectives: List[str] = Field(default_factory=list)
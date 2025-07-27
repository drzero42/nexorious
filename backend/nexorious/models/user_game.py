"""
User game collection models for ownership and progress tracking.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, List
from datetime import datetime, date, timezone
from decimal import Decimal
from enum import Enum
import uuid


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


class UserGame(SQLModel, table=True):
    """User game model linking users to games with ownership and progress data."""
    
    __tablename__ = "user_games"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    game_id: str = Field(foreign_key="games.id", index=True)
    ownership_status: OwnershipStatus = Field(default=OwnershipStatus.OWNED)
    is_physical: bool = Field(default=False)
    physical_location: Optional[str] = Field(default=None, max_length=200)
    personal_rating: Optional[Decimal] = Field(default=None, max_digits=2, decimal_places=1)
    is_loved: bool = Field(default=False, index=True)
    play_status: PlayStatus = Field(default=PlayStatus.NOT_STARTED, index=True)
    hours_played: int = Field(default=0)
    personal_notes: Optional[str] = Field(default=None)
    acquired_date: Optional[date] = Field(default=None)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user: "User" = Relationship(back_populates="user_games")
    game: "Game" = Relationship(back_populates="user_games")
    platforms: List["UserGamePlatform"] = Relationship(back_populates="user_game")
    tags: List["UserGameTag"] = Relationship(back_populates="user_game")
    
    # Unique constraint
    __table_args__ = (
        {"extend_existing": True},
    )


class UserGamePlatform(SQLModel, table=True):
    """User game platform model for platform-specific ownership data."""
    
    __tablename__ = "user_game_platforms"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_game_id: str = Field(foreign_key="user_games.id", index=True)
    platform_id: str = Field(foreign_key="platforms.id", index=True)
    storefront_id: Optional[str] = Field(default=None, foreign_key="storefronts.id")
    store_game_id: Optional[str] = Field(default=None, max_length=200)
    store_url: Optional[str] = Field(default=None, max_length=500)
    is_available: bool = Field(default=True)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user_game: UserGame = Relationship(back_populates="platforms")
    platform: "Platform" = Relationship(back_populates="user_game_platforms")
    storefront: Optional["Storefront"] = Relationship(back_populates="user_game_platforms")
    
    # Unique constraint
    __table_args__ = (
        {"extend_existing": True},
    )


# Import forward references
from .user import User
from .game import Game
from .platform import Platform, Storefront
from .tag import UserGameTag
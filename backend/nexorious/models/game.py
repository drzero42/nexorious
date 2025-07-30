"""
Game metadata models with IGDB integration support.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, List
from datetime import datetime, date, timezone
from decimal import Decimal
import uuid


class Game(SQLModel, table=True):
    """Game model with comprehensive metadata and IGDB integration."""
    
    __tablename__ = "games"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    title: str = Field(index=True, max_length=500)
    description: Optional[str] = Field(default=None)
    genre: Optional[str] = Field(default=None, max_length=200)
    developer: Optional[str] = Field(default=None, max_length=200)
    publisher: Optional[str] = Field(default=None, max_length=200)
    release_date: Optional[date] = Field(default=None)
    cover_art_url: Optional[str] = Field(default=None, max_length=500)
    rating_average: Optional[Decimal] = Field(default=None, max_digits=3, decimal_places=2)
    rating_count: int = Field(default=0)
    game_metadata: str = Field(default="{}")  # JSON string for extensible metadata
    estimated_playtime_hours: Optional[int] = Field(default=None)
    
    # How Long to Beat integration (stored in hours, converted from IGDB's seconds)
    howlongtobeat_main: Optional[int] = Field(default=None)
    howlongtobeat_extra: Optional[int] = Field(default=None)
    howlongtobeat_completionist: Optional[int] = Field(default=None)
    
    # IGDB integration
    igdb_id: Optional[str] = Field(default=None, index=True, max_length=50)
    igdb_slug: Optional[str] = Field(default=None, index=True, max_length=200)
    igdb_platform_ids: Optional[str] = Field(default=None, description="JSON array of IGDB platform IDs")
    igdb_platform_names: Optional[str] = Field(default=None, description="JSON array of IGDB platform names for frontend filtering")
    is_verified: bool = Field(default=False)
    
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    aliases: List["GameAlias"] = Relationship(back_populates="game")
    user_games: List["UserGame"] = Relationship(back_populates="game")
    wishlists: List["Wishlist"] = Relationship(back_populates="game")


class GameAlias(SQLModel, table=True):
    """Game alias model for alternative titles and search optimization."""
    
    __tablename__ = "game_aliases"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    game_id: str = Field(foreign_key="games.id", index=True)
    alias_title: str = Field(index=True, max_length=500)
    source: Optional[str] = Field(default=None, max_length=100)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    game: Game = Relationship(back_populates="aliases")


# Import forward references
from .user_game import UserGame
from .wishlist import Wishlist
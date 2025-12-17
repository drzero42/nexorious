"""
Game metadata models with IGDB integration support.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import Column, Numeric
from pydantic import field_validator
from typing import Optional, List
from datetime import datetime, date, timezone
from decimal import Decimal

# Import forward references
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .user_game import UserGame
    from .wishlist import Wishlist


class Game(SQLModel, table=True):
    """Game model with comprehensive metadata and IGDB integration."""

    __tablename__ = "games"  # type: ignore[assignment]

    id: int = Field(primary_key=True, description="IGDB ID as primary key")
    title: str = Field(index=True, max_length=500)
    description: Optional[str] = Field(default=None)
    genre: Optional[str] = Field(default=None, max_length=200)
    developer: Optional[str] = Field(default=None, max_length=200)
    publisher: Optional[str] = Field(default=None, max_length=200)
    release_date: Optional[date] = Field(default=None)
    cover_art_url: Optional[str] = Field(default=None, max_length=500)
    rating_average: Optional[Decimal] = Field(
        default=None, sa_column=Column(Numeric(precision=5, scale=2), nullable=True)
    )
    rating_count: int = Field(default=0)
    game_metadata: str = Field(default="{}")  # JSON string for extensible metadata
    estimated_playtime_hours: Optional[int] = Field(default=None)

    # How Long to Beat integration (stored in hours, converted from IGDB's seconds)
    howlongtobeat_main: Optional[int] = Field(default=None)
    howlongtobeat_extra: Optional[int] = Field(default=None)
    howlongtobeat_completionist: Optional[int] = Field(default=None)
    igdb_slug: Optional[str] = Field(default=None, index=True, max_length=200)
    igdb_platform_ids: Optional[str] = Field(
        default=None, description="JSON array of IGDB platform IDs"
    )
    igdb_platform_names: Optional[str] = Field(
        default=None,
        description="JSON array of IGDB platform names for frontend filtering",
    )

    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    last_updated: Optional[datetime] = Field(
        default=None, description="Timestamp when IGDB metadata was last refreshed"
    )

    # Relationships
    user_games: List["UserGame"] = Relationship(back_populates="game")
    wishlists: List["Wishlist"] = Relationship(back_populates="game")

    @field_validator("id")
    @classmethod
    def validate_igdb_id(cls, v: int) -> int:
        if v <= 0:
            raise ValueError("IGDB ID must be a positive integer")
        return v

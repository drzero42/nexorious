"""
Game metadata models with IGDB integration support.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy.orm import Mapped
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

    __tablename__ = "games"

    id: Mapped[int] = Field(primary_key=True, description="IGDB ID as primary key")
    title: Mapped[str] = Field(index=True, max_length=500)
    description: Mapped[Optional[str]] = Field(default=None)
    genre: Mapped[Optional[str]] = Field(default=None, max_length=200)
    developer: Mapped[Optional[str]] = Field(default=None, max_length=200)
    publisher: Mapped[Optional[str]] = Field(default=None, max_length=200)
    release_date: Mapped[Optional[date]] = Field(default=None)
    cover_art_url: Mapped[Optional[str]] = Field(default=None, max_length=500)
    rating_average: Mapped[Optional[Decimal]] = Field(
        default=None, max_digits=3, decimal_places=2
    )
    rating_count: Mapped[int] = Field(default=0)
    game_metadata: Mapped[str] = Field(default="{}")  # JSON string for extensible metadata
    estimated_playtime_hours: Mapped[Optional[int]] = Field(default=None)

    # How Long to Beat integration (stored in hours, converted from IGDB's seconds)
    howlongtobeat_main: Mapped[Optional[int]] = Field(default=None)
    howlongtobeat_extra: Mapped[Optional[int]] = Field(default=None)
    howlongtobeat_completionist: Mapped[Optional[int]] = Field(default=None)
    igdb_slug: Mapped[Optional[str]] = Field(default=None, index=True, max_length=200)
    igdb_platform_ids: Mapped[Optional[str]] = Field(
        default=None, description="JSON array of IGDB platform IDs"
    )
    igdb_platform_names: Mapped[Optional[str]] = Field(
        default=None,
        description="JSON array of IGDB platform names for frontend filtering",
    )

    created_at: Mapped[datetime] = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: Mapped[datetime] = Field(default_factory=lambda: datetime.now(timezone.utc))
    last_updated: Mapped[Optional[datetime]] = Field(
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

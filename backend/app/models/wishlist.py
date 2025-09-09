"""
Wishlist model for tracking desired games.
"""

from sqlmodel import SQLModel, Field, Relationship
from datetime import datetime, timezone
import uuid

# Import forward references
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .user import User
    from .game import Game


class Wishlist(SQLModel, table=True):
    """Wishlist model for tracking games users want to purchase."""
    
    __tablename__ = "wishlists"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    game_id: int = Field(foreign_key="games.id", index=True)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user: "User" = Relationship(back_populates="wishlists")
    game: "Game" = Relationship(back_populates="wishlists")
    
    # Unique constraint (user_id, game_id)
    __table_args__ = (
        {"extend_existing": True},
    )



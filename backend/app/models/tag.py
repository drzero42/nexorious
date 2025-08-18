"""
Tagging system models for game organization and categorization.
"""

from sqlmodel import SQLModel, Field, Relationship, UniqueConstraint
from pydantic import PrivateAttr, ConfigDict
from typing import Optional, List
from datetime import datetime, timezone
import uuid


class Tag(SQLModel, table=True):
    """Tag model for user-defined game categorization."""
    
    __tablename__ = "tags"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    name: str = Field(max_length=100)
    color: str = Field(default="#6B7280", max_length=7)  # Hex color code
    description: Optional[str] = Field(default=None)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Computed field using PrivateAttr - completely excluded from SQL
    _game_count: Optional[int] = PrivateAttr(default=None)
    
    @property
    def game_count(self) -> Optional[int]:
        """Computed field that returns the game count."""
        return self._game_count
    
    # Model configuration
    model_config = ConfigDict(from_attributes=True)
    
    # Relationships
    user: "User" = Relationship(back_populates="tags")
    user_game_tags: List["UserGameTag"] = Relationship(back_populates="tag")
    
    # Unique constraint (user_id, name)
    __table_args__ = (
        UniqueConstraint("user_id", "name", name="uq_tag_user_name"),
        {"extend_existing": True},
    )


class UserGameTag(SQLModel, table=True):
    """Many-to-many relationship between user games and tags."""
    
    __tablename__ = "user_game_tags"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_game_id: str = Field(foreign_key="user_games.id", index=True)
    tag_id: str = Field(foreign_key="tags.id", index=True)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user_game: "UserGame" = Relationship(back_populates="tags")
    tag: Tag = Relationship(back_populates="user_game_tags")
    
    # Unique constraint (user_game_id, tag_id)
    __table_args__ = (
        UniqueConstraint("user_game_id", "tag_id", name="uq_user_game_tag"),
        {"extend_existing": True},
    )


# Import forward references
from .user import User
from .user_game import UserGame
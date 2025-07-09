"""
Platform and storefront models for gaming platforms and digital stores.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, List
from datetime import datetime, timezone
import uuid


class Platform(SQLModel, table=True):
    """Platform model for gaming platforms (Windows, PlayStation, Xbox, Nintendo Switch, etc.)."""
    
    __tablename__ = "platforms"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    name: str = Field(unique=True, index=True, max_length=100)
    display_name: str = Field(max_length=100)
    icon_url: Optional[str] = Field(default=None, max_length=500)
    is_active: bool = Field(default=True)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user_game_platforms: List["UserGamePlatform"] = Relationship(back_populates="platform")


class Storefront(SQLModel, table=True):
    """Storefront model for digital game stores."""
    
    __tablename__ = "storefronts"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    name: str = Field(unique=True, index=True, max_length=100)
    display_name: str = Field(max_length=100)
    icon_url: Optional[str] = Field(default=None, max_length=500)
    base_url: Optional[str] = Field(default=None, max_length=500)
    is_active: bool = Field(default=True)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user_game_platforms: List["UserGamePlatform"] = Relationship(back_populates="storefront")


# Import forward references
from .user_game import UserGamePlatform
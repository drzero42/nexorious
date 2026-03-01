"""
User management models for authentication and user data.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, List
from datetime import datetime, timezone
from pydantic import computed_field
import uuid
import json

# Import forward references
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .user_game import UserGame
    from .tag import Tag
    from .wishlist import Wishlist
    from .job import Job
    from .user_sync_config import UserSyncConfig
    from .ignored_external_game import IgnoredExternalGame
    from .external_game import ExternalGame


class User(SQLModel, table=True):
    """User model for authentication and profile data."""
    
    __tablename__ = "users"  # type: ignore[assignment]
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    username: str = Field(unique=True, index=True, min_length=3, max_length=100)
    password_hash: str = Field(min_length=1, max_length=255)
    is_active: bool = Field(default=True)
    is_admin: bool = Field(default=False)
    preferences_json: str = Field(default="{}", sa_column_kwargs={"name": "preferences"})  # JSON string for user preferences
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    @computed_field
    @property
    def preferences(self) -> dict:
        """Convert JSON string to dictionary for API responses."""
        try:
            if self.preferences_json is None or self.preferences_json == "":
                return {}
            return json.loads(self.preferences_json)
        except (json.JSONDecodeError, TypeError):
            return {}
    
    # Relationships
    sessions: List["UserSession"] = Relationship(back_populates="user")
    user_games: List["UserGame"] = Relationship(back_populates="user")
    tags: List["Tag"] = Relationship(back_populates="user")
    wishlists: List["Wishlist"] = Relationship(back_populates="user")
    jobs: List["Job"] = Relationship(back_populates="user")
    sync_configs: List["UserSyncConfig"] = Relationship(back_populates="user")
    ignored_external_games: List["IgnoredExternalGame"] = Relationship(back_populates="user")
    external_games: list["ExternalGame"] = Relationship(back_populates="user")


class UserSession(SQLModel, table=True):
    """User session model for JWT token management."""
    
    __tablename__ = "user_sessions"  # type: ignore[assignment]
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    token_hash: str = Field(index=True, max_length=255)
    refresh_token_hash: str = Field(max_length=255)
    expires_at: datetime
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    user_agent: Optional[str] = Field(default=None)
    ip_address: Optional[str] = Field(default=None, max_length=45)
    
    # Relationships
    user: User = Relationship(back_populates="sessions")



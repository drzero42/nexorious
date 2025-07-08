"""
User management models for authentication and user data.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, List
from datetime import datetime
from pydantic import EmailStr
import uuid


class User(SQLModel, table=True):
    """User model for authentication and profile data."""
    
    __tablename__ = "users"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    email: EmailStr = Field(unique=True, index=True)
    username: str = Field(unique=True, index=True, min_length=3, max_length=100)
    password_hash: str = Field(min_length=1, max_length=255)
    first_name: Optional[str] = Field(default=None, max_length=100)
    last_name: Optional[str] = Field(default=None, max_length=100)
    is_active: bool = Field(default=True)
    is_admin: bool = Field(default=False)
    preferences: str = Field(default="{}")  # JSON string for user preferences
    created_at: datetime = Field(default_factory=datetime.utcnow)
    updated_at: datetime = Field(default_factory=datetime.utcnow)
    
    # Relationships
    sessions: List["UserSession"] = Relationship(back_populates="user")
    user_games: List["UserGame"] = Relationship(back_populates="user")
    tags: List["Tag"] = Relationship(back_populates="user")
    wishlists: List["Wishlist"] = Relationship(back_populates="user")
    import_jobs: List["ImportJob"] = Relationship(back_populates="user")


class UserSession(SQLModel, table=True):
    """User session model for JWT token management."""
    
    __tablename__ = "user_sessions"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    token_hash: str = Field(index=True, max_length=255)
    refresh_token_hash: str = Field(max_length=255)
    expires_at: datetime
    created_at: datetime = Field(default_factory=datetime.utcnow)
    user_agent: Optional[str] = Field(default=None)
    ip_address: Optional[str] = Field(default=None, max_length=45)
    
    # Relationships
    user: User = Relationship(back_populates="sessions")


# Import forward references
from .user_game import UserGame
from .tag import Tag
from .wishlist import Wishlist
from .import_job import ImportJob
"""
Model for tracking games users have explicitly ignored from external sources.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import UniqueConstraint
from typing import TYPE_CHECKING
from datetime import datetime, timezone
import uuid

from .job import BackgroundJobSource

if TYPE_CHECKING:
    from .user import User


class IgnoredExternalGame(SQLModel, table=True):
    """
    Tracks games a user has explicitly ignored from external sync sources.

    When a user ignores a game during sync review, it's recorded here
    so it won't appear in future syncs.
    """

    __tablename__ = "ignored_external_games"  # pyrefly: ignore[bad-override]
    __table_args__ = (
        UniqueConstraint(
            "user_id", "source", "external_id",
            name="uq_ignored_external_games_user_source_external"
        ),
    )

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    source: BackgroundJobSource = Field(index=True, description="Source platform (STEAM, EPIC, GOG)")
    external_id: str = Field(max_length=100, index=True, description="Platform-specific ID (Steam AppID, etc.)")
    title: str = Field(max_length=500, description="Game title for display purposes")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user: "User" = Relationship(back_populates="ignored_external_games")

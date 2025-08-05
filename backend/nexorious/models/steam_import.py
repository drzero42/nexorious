"""
Steam import job models for tracking background Steam library import operations.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, List
from datetime import datetime, timezone
from enum import Enum
import uuid


class SteamImportJobStatus(str, Enum):
    """Steam import job status enumeration."""
    PENDING = "pending"
    PROCESSING = "processing"
    AWAITING_REVIEW = "awaiting_review"
    FINALIZING = "finalizing"
    COMPLETED = "completed"
    FAILED = "failed"


class SteamImportGameStatus(str, Enum):
    """Steam import game status enumeration."""
    MATCHED = "matched"
    AWAITING_USER = "awaiting_user"
    SKIPPED = "skipped"
    IMPORTED = "imported"
    PLATFORM_ADDED = "platform_added"
    ALREADY_OWNED = "already_owned"
    IMPORT_FAILED = "import_failed"


class SteamImportJob(SQLModel, table=True):
    """Steam import job model for tracking background Steam library import operations."""
    
    __tablename__ = "steam_import_jobs"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    status: SteamImportJobStatus = Field(default=SteamImportJobStatus.PENDING, index=True)
    
    # Game counting and progress tracking
    total_games: int = Field(default=0, description="Total number of games in Steam library")
    processed_games: int = Field(default=0, description="Number of games processed so far")
    matched_games: int = Field(default=0, description="Number of games automatically matched")
    awaiting_review_games: int = Field(default=0, description="Number of games awaiting user review")
    skipped_games: int = Field(default=0, description="Number of games skipped by user")
    imported_games: int = Field(default=0, description="Number of new games imported")
    platform_added_games: int = Field(default=0, description="Number of games where Steam platform was added")
    
    # Error handling and job metadata
    error_message: Optional[str] = Field(default=None, description="Error message if job failed")
    steam_library_data: str = Field(default="[]", description="JSON string containing Steam library data")
    
    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    completed_at: Optional[datetime] = Field(default=None)
    
    # Relationships
    user: "User" = Relationship(back_populates="steam_import_jobs")
    games: List["SteamImportGame"] = Relationship(back_populates="import_job", cascade_delete=True)


class SteamImportGame(SQLModel, table=True):
    """Steam import game model for tracking individual game status within import jobs."""
    
    __tablename__ = "steam_import_games"
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    import_job_id: str = Field(foreign_key="steam_import_jobs.id", index=True)
    
    # Steam game information
    steam_appid: int = Field(index=True, description="Steam AppID")
    steam_name: str = Field(description="Game name from Steam")
    
    # Status and matching information
    status: SteamImportGameStatus = Field(default=SteamImportGameStatus.AWAITING_USER, index=True)
    matched_game_id: Optional[str] = Field(default=None, foreign_key="games.id", description="ID of matched game in database")
    user_decision: Optional[str] = Field(default=None, description="JSON string containing user's matching decision")
    error_message: Optional[str] = Field(default=None, description="Error message if import failed")
    
    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    import_job: SteamImportJob = Relationship(back_populates="games")
    matched_game: Optional["Game"] = Relationship()


# Import forward references
from .user import User
from .game import Game
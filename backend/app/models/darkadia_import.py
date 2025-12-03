"""
Darkadia CSV import models for preserving original data and extended metadata.
"""

from sqlmodel import SQLModel, Field, Relationship, Column
from sqlalchemy import UniqueConstraint, JSON
from typing import Optional, Dict, Any
from datetime import datetime, timezone
import uuid
import json

# Import forward references
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .user import User
    from .user_game import UserGame, UserGamePlatform
    from .platform import Platform, Storefront


class DarkadiaImport(SQLModel, table=True):
    """Darkadia import model for storing CSV import data with extended metadata."""
    
    __tablename__ = "darkadia_imports"  # type: ignore[assignment]
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    user_game_id: Optional[str] = Field(default=None, foreign_key="user_games.id", index=True)
    user_game_platform_id: Optional[str] = Field(default=None, foreign_key="user_game_platforms.id", index=True)
    
    # Copy identification and consolidation
    csv_row_number: int = Field(description="Original CSV row number for tracking")
    game_name: str = Field(max_length=500, index=True, description="Game name for consolidation grouping")
    copy_identifier: Optional[str] = Field(default=None, max_length=200, description="Unique identifier for this copy")
    
    # Import batch tracking
    batch_id: str = Field(index=True, description="Unique identifier for the import batch")
    csv_file_hash: str = Field(max_length=64, description="SHA-256 hash of the source CSV file")
    import_timestamp: datetime = Field(default_factory=lambda: datetime.now(timezone.utc), index=True)
    
    # Original CSV data preservation (JSONB)
    original_csv_data_json: str = Field(
        default="{}",
        sa_column=Column("original_csv_data", JSON),
        description="Original CSV row data as JSON"
    )
    
    # Darkadia boolean flags
    played: bool = Field(default=False, description="Game has been played")
    playing: bool = Field(default=False, description="Currently playing the game")
    finished: bool = Field(default=False, description="Game has been finished")
    mastered: bool = Field(default=False, description="Game has been mastered")
    dominated: bool = Field(default=False, description="Game has been dominated")
    shelved: bool = Field(default=False, description="Game has been shelved")
    
    # Physical copy metadata (JSONB)
    physical_copy_data_json: str = Field(
        default=None,
        sa_column=Column("physical_copy_data", JSON),
        description="Physical copy metadata as JSON"
    )
    
    # Platform resolution tracking
    original_platform_name: Optional[str] = Field(default=None, max_length=200)
    original_storefront_name: Optional[str] = Field(default=None, max_length=200)
    fallback_platform_name: Optional[str] = Field(default=None, max_length=200, description="From generic Platforms field when no copy data")
    platform_resolved: bool = Field(default=False, index=True)
    storefront_resolved: bool = Field(default=False, index=True)
    resolved_platform_id: Optional[str] = Field(default=None, foreign_key="platforms.id", description="Resolved platform ID")
    resolved_storefront_id: Optional[str] = Field(default=None, foreign_key="storefronts.id", description="Resolved storefront ID")
    requires_storefront_resolution: bool = Field(default=False, description="Copy has platform but no storefront")
    platform_resolution_data_json: str = Field(
        default="{}",
        sa_column=Column("platform_resolution_data", JSON),
        description="Platform resolution tracking data as JSON"
    )
    
    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user: "User" = Relationship(back_populates="darkadia_imports")
    user_game: "UserGame" = Relationship(back_populates="darkadia_imports")
    user_game_platform: "UserGamePlatform" = Relationship()
    resolved_platform: Optional["Platform"] = Relationship()
    resolved_storefront: Optional["Storefront"] = Relationship()
    
    # Unique constraint per CSV row and copy (allows multiple copies per CSV row)
    __table_args__ = (
        UniqueConstraint(
            "user_id", "csv_row_number", "copy_identifier", "batch_id",
            name="uq_darkadia_imports_user_row_copy_batch"
        ),
        {"extend_existing": True},
    )
    
    # JSON helper methods (following ImportJob pattern)
    def get_original_csv_data(self) -> Dict[str, Any]:
        """Get original CSV data as a dictionary."""
        try:
            return json.loads(self.original_csv_data_json or "{}")
        except (json.JSONDecodeError, TypeError):
            return {}
    
    def set_original_csv_data(self, value: Dict[str, Any]) -> None:
        """Set original CSV data from a dictionary."""
        self.original_csv_data_json = json.dumps(value)
    
    def get_physical_copy_data(self) -> Optional[Dict[str, Any]]:
        """Get physical copy data as a dictionary."""
        if not self.physical_copy_data_json:
            return None
        try:
            return json.loads(self.physical_copy_data_json)
        except (json.JSONDecodeError, TypeError):
            return None
    
    def set_physical_copy_data(self, value: Optional[Dict[str, Any]]) -> None:
        """Set physical copy data from a dictionary."""
        self.physical_copy_data_json = json.dumps(value) if value else None
    
    def get_platform_resolution_data(self) -> Dict[str, Any]:
        """Get platform resolution data as a dictionary."""
        try:
            return json.loads(self.platform_resolution_data_json or "{}")
        except (json.JSONDecodeError, TypeError):
            return {}
    
    def set_platform_resolution_data(self, value: Dict[str, Any]) -> None:
        """Set platform resolution data from a dictionary."""
        self.platform_resolution_data_json = json.dumps(value)



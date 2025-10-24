"""
Darkadia games models for Darkadia CSV import staging and management.
"""

from sqlmodel import SQLModel, Field, Relationship, Column
from sqlalchemy import UniqueConstraint, JSON
from typing import Optional, Dict, Any
from datetime import datetime, timezone
import uuid
import json
from ..utils.json_serialization import safe_json_dumps

# Import forward references
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .user import User
    from .game import Game


class DarkadiaGame(SQLModel, table=True):
    """Darkadia game model for staging CSV import data before sync to collection."""
    
    __tablename__ = "darkadia_games"
    __table_args__ = (
        UniqueConstraint("user_id", "external_id", name="uq_darkadia_games_user_external"),
    )
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    external_id: str = Field(max_length=50, index=True, description="Row number or unique identifier from CSV")
    game_name: str = Field(max_length=500, description="Game name from CSV")
    igdb_id: Optional[int] = Field(default=None, index=True, description="IGDB API ID from IGDB service (e.g., 1942)")
    igdb_title: Optional[str] = Field(default=None, max_length=500, description="Game title from IGDB when matched")
    game_id: Optional[int] = Field(default=None, foreign_key="games.id", index=True, description="Game ID when synced to user collection")
    ignored: bool = Field(default=False, description="Whether user has marked this game as ignored")
    
    # Store all CSV data as JSON for flexibility and audit trail
    csv_data_json: str = Field(
        default="{}",
        sa_column=Column("csv_data", JSON),
        description="All CSV row data as JSON"
    )
    
    # Store transformation metadata (platform mappings, validation results, etc.)
    transformation_data_json: str = Field(
        default="{}",
        sa_column=Column("transformation_data", JSON),
        description="Transformation metadata as JSON"
    )
    
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user: "User" = Relationship(back_populates="darkadia_games")
    synced_game: Optional["Game"] = Relationship(
        sa_relationship_kwargs={"foreign_keys": "DarkadiaGame.game_id", "post_update": True}
    )
    
    # JSON helper methods for CSV data
    def get_csv_data(self) -> Dict[str, Any]:
        """Get CSV data as a dictionary."""
        try:
            return json.loads(self.csv_data_json or "{}")
        except (json.JSONDecodeError, TypeError):
            return {}
    
    def set_csv_data(self, value: Dict[str, Any]) -> None:
        """Set CSV data from a dictionary."""
        self.csv_data_json = safe_json_dumps(value)
    
    def get_csv_field(self, field_name: str, default: Any = None) -> Any:
        """Get a specific field from CSV data."""
        csv_data = self.get_csv_data()
        return csv_data.get(field_name, default)
    
    def get_transformation_data(self) -> Dict[str, Any]:
        """Get transformation data as a dictionary."""
        try:
            return json.loads(self.transformation_data_json or "{}")
        except (json.JSONDecodeError, TypeError):
            return {}
    
    def set_transformation_data(self, value: Dict[str, Any]) -> None:
        """Set transformation data from a dictionary."""
        self.transformation_data_json = safe_json_dumps(value)
    
    def get_transformation_field(self, field_name: str, default: Any = None) -> Any:
        """Get a specific field from transformation data."""
        transform_data = self.get_transformation_data()
        return transform_data.get(field_name, default)
    
    @property
    def platforms(self) -> str:
        """Get platforms from CSV data."""
        return self.get_csv_field("Platforms", "")
    
    @property
    def rating(self) -> Optional[float]:
        """Get rating from CSV data."""
        rating_str = self.get_csv_field("Rating", "")
        try:
            return float(rating_str) if rating_str else None
        except (ValueError, TypeError):
            return None
    
    @property
    def notes(self) -> str:
        """Get notes from CSV data."""
        return self.get_csv_field("Notes", "")
    
    @property
    def played_flags(self) -> Dict[str, bool]:
        """Get all played status flags from CSV data."""
        csv_data = self.get_csv_data()
        return {
            "played": csv_data.get("Played", False),
            "playing": csv_data.get("Playing", False),
            "finished": csv_data.get("Finished", False),
            "mastered": csv_data.get("Mastered", False),
            "dominated": csv_data.get("Dominated", False),
            "shelved": csv_data.get("Shelved", False)
        }



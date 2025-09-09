"""
Steam games models for Steam library management and sync.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import UniqueConstraint
from typing import Optional
from datetime import datetime, timezone
import uuid

# Import forward references
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .user import User
    from .game import Game


class SteamGame(SQLModel, table=True):
    """Steam game model for managing user's Steam library games."""
    
    __tablename__ = "steam_games"
    __table_args__ = (
        UniqueConstraint("user_id", "steam_appid", name="uq_steam_games_user_appid"),
    )
    
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    steam_appid: int = Field(index=True, description="Steam AppID from Steam Web API")
    game_name: str = Field(max_length=500, description="Game name from Steam Web API")
    igdb_title: Optional[str] = Field(default=None, max_length=500, description="Game title from IGDB when matched to IGDB game")
    game_id: Optional[int] = Field(default=None, foreign_key="games.id", index=True, description="Game ID when synced to user collection")
    ignored: bool = Field(default=False, description="Whether user has marked this game as ignored (won't be imported)")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    # Relationships
    user: "User" = Relationship(back_populates="steam_games")
    synced_game: Optional["Game"] = Relationship(
        sa_relationship_kwargs={"foreign_keys": "SteamGame.game_id", "post_update": True}
    )



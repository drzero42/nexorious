"""Persistent model for tracking games from external sync sources."""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import UniqueConstraint
from typing import Optional, TYPE_CHECKING
from datetime import datetime, timezone
import uuid

from .user_game import OwnershipStatus

if TYPE_CHECKING:
    from .user import User
    from .user_game import UserGamePlatform


def build_store_url(storefront: str, external_id: str) -> Optional[str]:
    """Compute store URL from storefront identifier and external ID."""
    if storefront == "steam":
        return f"https://store.steampowered.com/app/{external_id}"
    return None


class ExternalGame(SQLModel, table=True):
    """
    Persistent record of a game from an external sync source (Steam, PSN, etc.).

    Created once per (user, storefront, external_id) and updated on every sync.
    Stores IGDB resolution so matching is never re-computed after first run.
    """

    __tablename__ = "external_games"  # type: ignore[assignment]

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    storefront: str = Field(foreign_key="storefronts.name", index=True)
    external_id: str = Field(max_length=200)
    title: str = Field(max_length=500)

    # IGDB resolution state
    resolved_igdb_id: Optional[int] = Field(default=None, foreign_key="games.id")
    is_skipped: bool = Field(default=False)

    # Source state — always reflects what the platform last reported
    is_available: bool = Field(default=True)
    is_subscription: bool = Field(default=False)
    playtime_hours: int = Field(default=0)
    ownership_status: Optional[OwnershipStatus] = Field(default=None)
    platform: Optional[str] = Field(default=None, foreign_key="platforms.name")

    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user: "User" = Relationship(back_populates="external_games")
    user_game_platforms: list["UserGamePlatform"] = Relationship(back_populates="external_game")

    __table_args__ = (
        UniqueConstraint("user_id", "storefront", "external_id", name="uq_external_games_user_storefront_external"),
        {"extend_existing": True},
    )

    @property
    def store_url(self) -> Optional[str]:
        """Compute store URL from storefront + external_id."""
        return build_store_url(self.storefront, self.external_id)

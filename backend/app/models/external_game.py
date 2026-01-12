"""
ExternalGame model for persistent sync source tracking.

This model stores the state of games from external sync sources (Steam, Epic, PSN, etc.)
including IGDB resolution, subscription status, and availability.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import UniqueConstraint
from typing import Optional, TYPE_CHECKING
from datetime import datetime, timezone
from pydantic import computed_field
import uuid

from .user_game import OwnershipStatus

if TYPE_CHECKING:
    from .user import User


def build_store_url(storefront: str, external_id: str) -> Optional[str]:
    """Build store URL from storefront and external_id.

    Args:
        storefront: The storefront identifier (steam, epic, psn, gog)
        external_id: The platform-specific game ID

    Returns:
        The store URL or None if storefront is not supported
    """
    url_patterns = {
        "steam": f"https://store.steampowered.com/app/{external_id}",
        "epic": f"https://store.epicgames.com/p/{external_id}",
        "psn": f"https://store.playstation.com/product/{external_id}",
        "gog": f"https://www.gog.com/game/{external_id}",
    }
    return url_patterns.get(storefront.lower())


class ExternalGame(SQLModel, table=True):
    """
    Persistent model for games from external sync sources.

    This model tracks:
    - Source state (what the platform reports)
    - Resolution state (IGDB ID, skip status)
    - Link to UserGamePlatform (when imported to collection)
    """

    __tablename__ = "external_games"  # pyrefly: ignore[bad-override]

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    storefront: str = Field(foreign_key="storefronts.name", index=True)
    external_id: str = Field(max_length=200, index=True)
    title: str = Field(max_length=500)

    # Resolution state
    resolved_igdb_id: Optional[int] = Field(default=None, foreign_key="games.id", index=True)
    is_skipped: bool = Field(default=False, index=True)

    # Source state (always reflects what platform reports)
    is_available: bool = Field(default=True, index=True)
    is_subscription: bool = Field(default=False)
    playtime_hours: int = Field(default=0, ge=0)
    ownership_status: Optional[OwnershipStatus] = Field(default=None)

    # Platform info
    platform: Optional[str] = Field(default=None, foreign_key="platforms.name")

    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user: "User" = Relationship(back_populates="external_games")

    __table_args__ = (
        UniqueConstraint("user_id", "storefront", "external_id", name="uq_external_games_user_storefront_external"),
        {"extend_existing": True},
    )

    @computed_field
    @property
    def store_url(self) -> Optional[str]:
        """Compute store URL from storefront and external_id."""
        return build_store_url(self.storefront, self.external_id)

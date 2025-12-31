"""
Platform and storefront models for gaming platforms and digital stores.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, List
from datetime import datetime, timezone

# Import forward references
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .user_game import UserGamePlatform


class Storefront(SQLModel, table=True):
    """Storefront model for digital game stores."""

    __tablename__ = "storefronts"  # type: ignore[assignment]

    name: str = Field(primary_key=True, max_length=100)
    display_name: str = Field(max_length=100)
    icon_url: Optional[str] = Field(default=None, max_length=500)
    base_url: Optional[str] = Field(default=None, max_length=500)
    is_active: bool = Field(default=True)
    source: str = Field(default="custom", max_length=20, description="Source of the storefront: 'official' or 'custom'")
    version_added: Optional[str] = Field(default=None, max_length=10, description="Version when this official storefront was added")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user_game_platforms: List["UserGamePlatform"] = Relationship(back_populates="storefront_rel")
    default_for_platforms: List["Platform"] = Relationship(
        back_populates="default_storefront_rel",
        sa_relationship_kwargs={"foreign_keys": "[Platform.default_storefront]"}
    )


class Platform(SQLModel, table=True):
    """Platform model for gaming platforms (Windows, PlayStation, Xbox, Nintendo Switch, etc.)."""

    __tablename__ = "platforms"  # type: ignore[assignment]

    name: str = Field(primary_key=True, max_length=100)
    display_name: str = Field(max_length=100)
    icon_url: Optional[str] = Field(default=None, max_length=500)
    default_storefront: Optional[str] = Field(default=None, foreign_key="storefronts.name", description="Default storefront for this platform")
    is_active: bool = Field(default=True)
    source: str = Field(default="custom", max_length=20, description="Source of the platform: 'official' or 'custom'")
    version_added: Optional[str] = Field(default=None, max_length=10, description="Version when this official platform was added")
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user_game_platforms: List["UserGamePlatform"] = Relationship(back_populates="platform_rel")
    default_storefront_rel: Optional["Storefront"] = Relationship(
        back_populates="default_for_platforms",
        sa_relationship_kwargs={"foreign_keys": "[Platform.default_storefront]"}
    )


class PlatformStorefront(SQLModel, table=True):
    """Junction table for many-to-many platform-storefront associations."""

    __tablename__ = "platform_storefronts"  # type: ignore[assignment]

    platform: str = Field(foreign_key="platforms.name", primary_key=True)
    storefront: str = Field(foreign_key="storefronts.name", primary_key=True)
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))



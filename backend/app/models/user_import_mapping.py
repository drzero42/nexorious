"""
User import mapping models for persisting platform/storefront string mappings.

These mappings allow users to define how import source strings (e.g., "PC", "Steam")
should be mapped to actual Platform and Storefront entities. Mappings are reused
across future imports from the same source.
"""

from sqlmodel import SQLModel, Field, Relationship
from sqlalchemy import UniqueConstraint
from typing import Optional
from datetime import datetime, timezone
from enum import Enum
import uuid

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .user import User


class ImportMappingType(str, Enum):
    """Type of import mapping."""

    PLATFORM = "platform"
    STOREFRONT = "storefront"


class UserImportMapping(SQLModel, table=True):
    """
    User-defined mapping from import source strings to platform/storefront IDs.

    This allows users to define how strings like "PC", "PS4", "Steam" from
    import sources (e.g., Darkadia CSV) should be mapped to actual Platform
    or Storefront entities. These mappings are persisted and reused across
    future imports.

    Attributes:
        id: Unique identifier for the mapping
        user_id: User who created this mapping
        import_source: Source of the import (e.g., "darkadia", "steam")
        mapping_type: Whether this maps to a platform or storefront
        source_value: The original string from the import (e.g., "PC", "Steam")
        target_id: The ID of the Platform or Storefront to map to
        created_at: When the mapping was created
        updated_at: When the mapping was last updated
    """

    __tablename__ = "user_import_mappings"  # type: ignore[assignment]

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    user_id: str = Field(foreign_key="users.id", index=True)
    import_source: str = Field(
        max_length=50, index=True, description="Import source (e.g., 'darkadia')"
    )
    mapping_type: ImportMappingType = Field(
        index=True, description="Whether this is a platform or storefront mapping"
    )
    source_value: str = Field(
        max_length=255, description="Original string from import source"
    )
    target_id: str = Field(
        max_length=100, description="ID of the Platform or Storefront to map to"
    )
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user: Optional["User"] = Relationship(back_populates="import_mappings")

    # Ensure unique mapping per user/source/type/value combination
    __table_args__ = (
        UniqueConstraint(
            "user_id",
            "import_source",
            "mapping_type",
            "source_value",
            name="uq_user_import_mapping",
        ),
        {"extend_existing": True},
    )

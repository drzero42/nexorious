"""
User sync configuration model for managing platform sync settings.

This model stores per-user, per-platform synchronization preferences
including frequency, auto-add behavior, and last sync timestamp.
"""

from sqlmodel import SQLModel, Field, Relationship
from typing import Optional, TYPE_CHECKING
from datetime import datetime, timezone
from enum import Enum
import uuid

if TYPE_CHECKING:
    from .user import User


class SyncFrequency(str, Enum):
    """Frequency options for automatic platform syncing."""

    MANUAL = "manual"
    HOURLY = "hourly"
    DAILY = "daily"
    WEEKLY = "weekly"


class UserSyncConfig(SQLModel, table=True):
    """
    User sync configuration for external platform integration.

    Stores per-user, per-platform settings for automatic library syncing.
    Used by the sync scheduler to determine which users need syncing
    and how matched games should be handled.
    """

    __tablename__ = "user_sync_configs"  # type: ignore[assignment]

    # Primary key
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)

    # User association
    user_id: str = Field(foreign_key="users.id", index=True)

    # Platform identifier (steam, epic, gog, etc.)
    platform: str = Field(max_length=50, index=True)

    # Sync settings
    frequency: SyncFrequency = Field(default=SyncFrequency.MANUAL)
    auto_add: bool = Field(
        default=False,
        description="If True, matched games are added automatically. If False, queued for review.",
    )

    # Tracking
    last_synced_at: Optional[datetime] = Field(
        default=None, description="Timestamp of last successful sync"
    )

    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

    # Relationships
    user: "User" = Relationship(back_populates="sync_configs")

    # Note: Unique constraint on (user_id, platform) is enforced via Alembic migration

    @property
    def needs_sync(self) -> bool:
        """
        Check if this config needs syncing based on frequency and last sync time.

        Returns True if:
        - frequency is not MANUAL AND
        - (last_synced_at is None OR enough time has passed based on frequency)
        """
        if self.frequency == SyncFrequency.MANUAL:
            return False

        if self.last_synced_at is None:
            return True

        now = datetime.now(timezone.utc)
        elapsed = now - self.last_synced_at

        if self.frequency == SyncFrequency.HOURLY:
            return elapsed.total_seconds() >= 3600  # 1 hour
        elif self.frequency == SyncFrequency.DAILY:
            return elapsed.total_seconds() >= 86400  # 24 hours
        elif self.frequency == SyncFrequency.WEEKLY:
            return elapsed.total_seconds() >= 604800  # 7 days

        return False

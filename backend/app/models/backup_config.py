"""
BackupConfig model for storing backup schedule and retention settings.

This is a singleton table - only one row should exist.
"""

from sqlmodel import SQLModel, Field
from enum import Enum
from datetime import datetime, timezone


class BackupSchedule(str, Enum):
    """Backup schedule options."""

    MANUAL = "manual"
    DAILY = "daily"
    WEEKLY = "weekly"


class RetentionMode(str, Enum):
    """Retention policy mode."""

    DAYS = "days"
    COUNT = "count"


class BackupConfig(SQLModel, table=True):
    """
    Backup configuration singleton.

    Only one row should exist in this table, enforced by application logic.
    """

    __tablename__ = "backup_config"  # type: ignore[assignment]

    id: int = Field(default=1, primary_key=True)

    # Schedule settings
    schedule: BackupSchedule = Field(default=BackupSchedule.MANUAL)
    schedule_time: str = Field(default="02:00", max_length=5)  # HH:MM format
    schedule_day: int | None = Field(default=None, ge=0, le=6)  # 0=Monday, 6=Sunday

    # Retention settings
    retention_mode: RetentionMode = Field(default=RetentionMode.COUNT)
    retention_value: int = Field(default=10, ge=1)  # Days or count depending on mode

    # Timestamps
    updated_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))

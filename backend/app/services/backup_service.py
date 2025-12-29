"""
Backup service for creating and managing system backups.

Handles:
- Creating backups (database dump + static files)
- Listing available backups
- Deleting backups
- Retention policy enforcement
"""

import logging
import tarfile
import json
from pathlib import Path
from datetime import datetime, timezone
from dataclasses import dataclass
from enum import Enum
from typing import Optional

from app.core.config import settings

logger = logging.getLogger(__name__)


class BackupType(str, Enum):
    """Type of backup."""
    SCHEDULED = "scheduled"
    MANUAL = "manual"
    PRE_RESTORE = "pre_restore"


@dataclass
class BackupManifest:
    """Manifest data for a backup archive."""
    version: int
    created_at: datetime
    app_version: str
    alembic_revision: str
    backup_type: BackupType

    # Database info
    database_file: str
    database_size_bytes: int
    database_checksum: str

    # Cover art info
    cover_art_count: int
    cover_art_size_bytes: int
    cover_art_checksum: str

    # Logos info
    logos_count: int
    logos_size_bytes: int
    logos_checksum: str

    # Stats
    stats_users: int
    stats_games: int
    stats_tags: int

    def to_dict(self) -> dict:
        """Convert manifest to dictionary for JSON serialization."""
        return {
            "version": self.version,
            "created_at": self.created_at.isoformat(),
            "app_version": self.app_version,
            "alembic_revision": self.alembic_revision,
            "backup_type": self.backup_type.value,
            "database": {
                "file": self.database_file,
                "size_bytes": self.database_size_bytes,
                "checksum": self.database_checksum,
            },
            "files": {
                "cover_art": {
                    "count": self.cover_art_count,
                    "total_size_bytes": self.cover_art_size_bytes,
                    "checksum": self.cover_art_checksum,
                },
                "logos": {
                    "count": self.logos_count,
                    "total_size_bytes": self.logos_size_bytes,
                    "checksum": self.logos_checksum,
                },
            },
            "stats": {
                "users": self.stats_users,
                "games": self.stats_games,
                "tags": self.stats_tags,
            },
        }

    @classmethod
    def from_dict(cls, data: dict) -> "BackupManifest":
        """Create manifest from dictionary."""
        return cls(
            version=data["version"],
            created_at=datetime.fromisoformat(data["created_at"]),
            app_version=data["app_version"],
            alembic_revision=data["alembic_revision"],
            backup_type=BackupType(data["backup_type"]),
            database_file=data["database"]["file"],
            database_size_bytes=data["database"]["size_bytes"],
            database_checksum=data["database"]["checksum"],
            cover_art_count=data["files"]["cover_art"]["count"],
            cover_art_size_bytes=data["files"]["cover_art"]["total_size_bytes"],
            cover_art_checksum=data["files"]["cover_art"]["checksum"],
            logos_count=data["files"]["logos"]["count"],
            logos_size_bytes=data["files"]["logos"]["total_size_bytes"],
            logos_checksum=data["files"]["logos"]["checksum"],
            stats_users=data["stats"]["users"],
            stats_games=data["stats"]["games"],
            stats_tags=data["stats"]["tags"],
        )


@dataclass
class BackupInfo:
    """Information about a backup file."""
    id: str
    created_at: datetime
    backup_type: BackupType
    size_bytes: int
    stats_users: int
    stats_games: int
    stats_tags: int


class BackupService:
    """Service for managing backups."""

    def __init__(self, backup_path: Optional[str] = None):
        """Initialize backup service.

        Args:
            backup_path: Path to backup directory. Defaults to settings.backup_path.
        """
        self._backup_path = Path(backup_path) if backup_path else Path(settings.backup_path)

    def get_backup_dir(self) -> Path:
        """Get and ensure backup directory exists."""
        self._backup_path.mkdir(parents=True, exist_ok=True)
        return self._backup_path

    def generate_backup_id(self) -> str:
        """Generate a unique backup ID based on current timestamp."""
        now = datetime.now(timezone.utc)
        return f"backup-{now.strftime('%Y-%m-%dT%H%M%S')}Z"

    def get_backup_path(self, backup_id: str) -> Path:
        """Get the full path to a backup file."""
        return self.get_backup_dir() / f"{backup_id}.tar.gz"

    def list_backups(self) -> list[BackupInfo]:
        """List all available backups.

        Returns:
            List of BackupInfo objects sorted by creation date (newest first).
        """
        backup_dir = self.get_backup_dir()
        backups = []

        for archive_path in backup_dir.glob("backup-*.tar.gz"):
            try:
                info = self._read_backup_info(archive_path)
                if info:
                    backups.append(info)
            except Exception as e:
                logger.warning(f"Failed to read backup {archive_path}: {e}")

        # Sort by creation date, newest first
        backups.sort(key=lambda b: b.created_at, reverse=True)
        return backups

    def _read_backup_info(self, archive_path: Path) -> Optional[BackupInfo]:
        """Read backup info from archive manifest.

        Args:
            archive_path: Path to the backup archive.

        Returns:
            BackupInfo if manifest could be read, None otherwise.
        """
        try:
            with tarfile.open(archive_path, "r:gz") as tar:
                # Find manifest file
                manifest_member = None
                for member in tar.getmembers():
                    if member.name.endswith("manifest.json"):
                        manifest_member = member
                        break

                if not manifest_member:
                    logger.warning(f"No manifest found in {archive_path}")
                    return None

                # Read manifest
                manifest_file = tar.extractfile(manifest_member)
                if not manifest_file:
                    return None

                manifest_data = json.load(manifest_file)
                manifest = BackupManifest.from_dict(manifest_data)

                # Get backup ID from filename
                backup_id = archive_path.stem  # Remove .tar.gz

                return BackupInfo(
                    id=backup_id,
                    created_at=manifest.created_at,
                    backup_type=manifest.backup_type,
                    size_bytes=archive_path.stat().st_size,
                    stats_users=manifest.stats_users,
                    stats_games=manifest.stats_games,
                    stats_tags=manifest.stats_tags,
                )
        except Exception as e:
            logger.error(f"Error reading backup {archive_path}: {e}")
            return None

    def delete_backup(self, backup_id: str) -> bool:
        """Delete a backup file.

        Args:
            backup_id: ID of the backup to delete.

        Returns:
            True if deleted, False if not found.
        """
        backup_path = self.get_backup_path(backup_id)
        if backup_path.exists():
            backup_path.unlink()
            logger.info(f"Deleted backup: {backup_id}")
            return True
        return False

    def backup_exists(self, backup_id: str) -> bool:
        """Check if a backup exists."""
        return self.get_backup_path(backup_id).exists()


# Global instance
backup_service = BackupService()

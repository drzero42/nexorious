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
import hashlib
import shutil
import subprocess
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

    def _get_backups_to_delete_by_count(
        self,
        backups: list[BackupInfo],
        keep_count: int
    ) -> list[str]:
        """Get backup IDs that exceed the count limit.

        Only considers SCHEDULED backups. Manual and pre-restore are excluded.
        """
        # Filter to only scheduled backups
        scheduled = [b for b in backups if b.backup_type == BackupType.SCHEDULED]

        # Sort by creation date (newest first)
        scheduled.sort(key=lambda b: b.created_at, reverse=True)

        # Return IDs of backups beyond the keep count
        return [b.id for b in scheduled[keep_count:]]

    def _get_backups_to_delete_by_days(
        self,
        backups: list[BackupInfo],
        keep_days: int
    ) -> list[str]:
        """Get backup IDs that are older than the retention period.

        Only considers SCHEDULED backups. Manual and pre-restore are excluded.
        """
        from datetime import timedelta

        cutoff = datetime.now(timezone.utc) - timedelta(days=keep_days)

        # Filter to only scheduled backups older than cutoff
        to_delete = [
            b.id for b in backups
            if b.backup_type == BackupType.SCHEDULED and b.created_at < cutoff
        ]

        return to_delete

    def _get_prerestore_backups_to_delete(
        self,
        backups: list[BackupInfo]
    ) -> list[str]:
        """Get pre-restore backup IDs older than 7 days."""
        from datetime import timedelta

        cutoff = datetime.now(timezone.utc) - timedelta(days=7)

        return [
            b.id for b in backups
            if b.backup_type == BackupType.PRE_RESTORE and b.created_at < cutoff
        ]

    def run_retention_cleanup(
        self,
        retention_mode: str,
        retention_value: int
    ) -> list[str]:
        """Run retention cleanup based on configuration."""
        backups = self.list_backups()
        deleted = []

        # Get scheduled backups to delete based on retention policy
        if retention_mode == "days":
            to_delete = self._get_backups_to_delete_by_days(backups, retention_value)
        else:  # count
            to_delete = self._get_backups_to_delete_by_count(backups, retention_value)

        # Also clean up old pre-restore backups (7 day retention)
        prerestore_to_delete = self._get_prerestore_backups_to_delete(backups)
        to_delete.extend(prerestore_to_delete)

        # Delete the backups
        for backup_id in to_delete:
            if self.delete_backup(backup_id):
                deleted.append(backup_id)

        if deleted:
            logger.info(f"Retention cleanup deleted {len(deleted)} backups: {deleted}")

        return deleted

    def _calculate_file_checksum(self, file_path: Path) -> str:
        """Calculate SHA256 checksum of a file."""
        sha256 = hashlib.sha256()
        with open(file_path, "rb") as f:
            for chunk in iter(lambda: f.read(8192), b""):
                sha256.update(chunk)
        return f"sha256:{sha256.hexdigest()}"

    def _calculate_directory_stats(self, dir_path: Path) -> tuple[str, int, int]:
        """Calculate combined checksum, file count, and total size for a directory."""
        if not dir_path.exists():
            return "sha256:empty", 0, 0

        sha256 = hashlib.sha256()
        file_count = 0
        total_size = 0

        for file_path in sorted(dir_path.rglob("*")):
            if file_path.is_file():
                file_count += 1
                total_size += file_path.stat().st_size
                sha256.update(str(file_path.relative_to(dir_path)).encode())
                with open(file_path, "rb") as f:
                    for chunk in iter(lambda: f.read(8192), b""):
                        sha256.update(chunk)

        return f"sha256:{sha256.hexdigest()}", file_count, total_size

    def _get_alembic_revision(self) -> str:
        """Get current Alembic revision from database."""
        try:
            from sqlalchemy import text
            from app.core.database import get_engine

            engine = get_engine()
            with engine.connect() as conn:
                result = conn.execute(text("SELECT version_num FROM alembic_version"))
                row = result.fetchone()
                if row:
                    return row[0]
        except Exception as e:
            logger.warning(f"Failed to get Alembic revision: {e}")
        return "unknown"

    def _get_database_stats(self) -> tuple[int, int, int]:
        """Get counts of users, games, and tags from database."""
        try:
            from sqlmodel import Session, select, func
            from app.core.database import get_engine
            from app.models.user import User
            from app.models.game import Game
            from app.models.tag import Tag

            engine = get_engine()
            with Session(engine) as session:
                users = session.exec(select(func.count()).select_from(User)).one()
                games = session.exec(select(func.count()).select_from(Game)).one()
                tags = session.exec(select(func.count()).select_from(Tag)).one()
                return users, games, tags
        except Exception as e:
            logger.warning(f"Failed to get database stats: {e}")
            return 0, 0, 0

    def _run_pg_dump(self, output_path: Path, timeout: int = 300) -> None:
        """Run pg_dump to create database dump."""
        from urllib.parse import urlparse

        parsed = urlparse(settings.database_url)

        env = {
            "PGPASSWORD": parsed.password or "",
        }

        cmd = [
            "pg_dump",
            "--format=plain",
            "--no-owner",
            "--no-acl",
            f"--host={parsed.hostname}",
            f"--port={parsed.port or 5432}",
            f"--username={parsed.username}",
            f"--dbname={parsed.path.lstrip('/')}",
            f"--file={output_path}",
        ]

        try:
            import os
            result = subprocess.run(
                cmd,
                env={**os.environ, **env},
                capture_output=True,
                text=True,
                timeout=timeout,
            )

            if result.returncode != 0:
                raise RuntimeError(f"pg_dump failed: {result.stderr}")

            logger.info(f"Database dump created: {output_path}")
        except subprocess.TimeoutExpired:
            raise RuntimeError(f"pg_dump timed out after {timeout} seconds")

    def create_backup(
        self,
        backup_type: BackupType = BackupType.MANUAL,
        backup_id: Optional[str] = None,
    ) -> str:
        """Create a full system backup."""
        import tempfile

        backup_id = backup_id or self.generate_backup_id()
        logger.info(f"Creating backup: {backup_id} (type: {backup_type.value})")

        with tempfile.TemporaryDirectory() as tmpdir:
            staging_dir = Path(tmpdir) / backup_id
            staging_dir.mkdir()

            try:
                # 1. Database dump
                db_path = staging_dir / "database.sql"
                self._run_pg_dump(db_path)
                db_checksum = self._calculate_file_checksum(db_path)
                db_size = db_path.stat().st_size

                # 2. Copy cover art
                storage_path = settings.storage_path or "storage"
                cover_art_src = Path(storage_path) / "cover_art"
                cover_art_dst = staging_dir / "cover_art"
                if cover_art_src.exists():
                    shutil.copytree(cover_art_src, cover_art_dst)
                else:
                    cover_art_dst.mkdir()
                cover_art_checksum, cover_art_count, cover_art_size = \
                    self._calculate_directory_stats(cover_art_dst)

                # 3. Copy logos
                logos_src = Path("static/logos")
                logos_dst = staging_dir / "logos"
                if logos_src.exists():
                    shutil.copytree(logos_src, logos_dst)
                else:
                    logos_dst.mkdir()
                logos_checksum, logos_count, logos_size = \
                    self._calculate_directory_stats(logos_dst)

                # 4. Get stats
                users, games, tags = self._get_database_stats()

                # 5. Create manifest
                manifest = BackupManifest(
                    version=1,
                    created_at=datetime.now(timezone.utc),
                    app_version=settings.app_version,
                    alembic_revision=self._get_alembic_revision(),
                    backup_type=backup_type,
                    database_file="database.sql",
                    database_size_bytes=db_size,
                    database_checksum=db_checksum,
                    cover_art_count=cover_art_count,
                    cover_art_size_bytes=cover_art_size,
                    cover_art_checksum=cover_art_checksum,
                    logos_count=logos_count,
                    logos_size_bytes=logos_size,
                    logos_checksum=logos_checksum,
                    stats_users=users,
                    stats_games=games,
                    stats_tags=tags,
                )

                manifest_path = staging_dir / "manifest.json"
                manifest_path.write_text(json.dumps(manifest.to_dict(), indent=2))

                # 6. Create tar.gz archive
                archive_path = self.get_backup_path(backup_id)
                with tarfile.open(archive_path, "w:gz") as tar:
                    tar.add(staging_dir, arcname=backup_id)

                logger.info(f"Backup created successfully: {archive_path}")
                return backup_id

            except Exception as e:
                logger.error(f"Backup creation failed: {e}")
                archive_path = self.get_backup_path(backup_id)
                if archive_path.exists():
                    archive_path.unlink()
                raise RuntimeError(f"Backup creation failed: {e}")


# Global instance
backup_service = BackupService()

"""Tests for backup service."""

import pytest
import tempfile
from datetime import datetime, timezone, timedelta
from pathlib import Path

from app.services.backup_service import (
    BackupService,
    BackupManifest,
    BackupInfo,
    BackupType,
)


class TestBackupManifest:
    """Tests for BackupManifest dataclass."""

    def test_manifest_to_dict(self):
        """Test manifest serialization."""
        manifest = BackupManifest(
            version=1,
            created_at=datetime(2025, 1, 15, 2, 0, 0, tzinfo=timezone.utc),
            app_version="0.1.0",
            alembic_revision="abc123",
            backup_type=BackupType.MANUAL,
            database_file="database.sql",
            database_size_bytes=1000,
            database_checksum="sha256:abc",
            cover_art_count=10,
            cover_art_size_bytes=5000,
            cover_art_checksum="sha256:def",
            logos_count=5,
            logos_size_bytes=2000,
            logos_checksum="sha256:ghi",
            stats_users=1,
            stats_games=50,
            stats_tags=5,
        )

        data = manifest.to_dict()

        assert data["version"] == 1
        assert data["backup_type"] == "manual"
        assert data["database"]["file"] == "database.sql"
        assert data["stats"]["users"] == 1


class TestBackupService:
    """Tests for BackupService."""

    def test_get_backup_dir_creates_directory(self):
        """Test that backup directory is created if it doesn't exist."""
        with tempfile.TemporaryDirectory() as tmpdir:
            service = BackupService(backup_path=tmpdir)
            backup_dir = service.get_backup_dir()
            assert backup_dir.exists()
            assert backup_dir.is_dir()

    def test_generate_backup_id(self):
        """Test backup ID generation format."""
        service = BackupService(backup_path="/tmp")
        backup_id = service.generate_backup_id()

        # Should be in format: backup-YYYY-MM-DDTHHMMSSZ
        assert backup_id.startswith("backup-")
        # Should be parseable as a date
        date_part = backup_id.replace("backup-", "").replace("Z", "")
        datetime.strptime(date_part, "%Y-%m-%dT%H%M%S")

    def test_list_backups_empty(self):
        """Test listing backups when directory is empty."""
        with tempfile.TemporaryDirectory() as tmpdir:
            service = BackupService(backup_path=tmpdir)
            backups = service.list_backups()
            assert backups == []


class TestBackupCreation:
    """Tests for backup creation functionality."""

    @pytest.fixture
    def mock_db_url(self):
        """Mock database URL for testing."""
        return "postgresql://test:test@localhost:5432/testdb"

    def test_calculate_checksum(self):
        """Test checksum calculation for a file."""
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"test content")
            f.flush()

            service = BackupService(backup_path="/tmp")
            checksum = service._calculate_file_checksum(Path(f.name))

            assert checksum.startswith("sha256:")
            # Verify it's deterministic
            checksum2 = service._calculate_file_checksum(Path(f.name))
            assert checksum == checksum2

            Path(f.name).unlink()

    def test_calculate_directory_checksum(self):
        """Test checksum calculation for a directory."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create some test files
            (Path(tmpdir) / "file1.txt").write_text("content1")
            (Path(tmpdir) / "file2.txt").write_text("content2")

            service = BackupService(backup_path="/tmp")
            checksum, count, size = service._calculate_directory_stats(Path(tmpdir))

            assert checksum.startswith("sha256:")
            assert count == 2
            assert size > 0


class TestRetentionLogic:
    """Tests for backup retention logic."""

    def test_get_backups_to_delete_by_count(self):
        """Test retention by count."""
        service = BackupService(backup_path="/tmp")

        # Create mock backups
        now = datetime.now(timezone.utc)
        backups = [
            BackupInfo(
                id=f"backup-{i}",
                created_at=now - timedelta(days=i),
                backup_type=BackupType.SCHEDULED,
                size_bytes=1000,
                stats_users=1,
                stats_games=10,
                stats_tags=5,
            )
            for i in range(5)
        ]

        # Keep 3, should delete 2 oldest
        to_delete = service._get_backups_to_delete_by_count(backups, 3)

        assert len(to_delete) == 2
        # Should delete oldest (backup-3 and backup-4)
        assert "backup-3" in to_delete
        assert "backup-4" in to_delete

    def test_get_backups_to_delete_by_days(self):
        """Test retention by days."""
        service = BackupService(backup_path="/tmp")

        now = datetime.now(timezone.utc)
        backups = [
            BackupInfo(
                id="backup-new",
                created_at=now - timedelta(days=1),
                backup_type=BackupType.SCHEDULED,
                size_bytes=1000,
                stats_users=1,
                stats_games=10,
                stats_tags=5,
            ),
            BackupInfo(
                id="backup-old",
                created_at=now - timedelta(days=10),
                backup_type=BackupType.SCHEDULED,
                size_bytes=1000,
                stats_users=1,
                stats_games=10,
                stats_tags=5,
            ),
        ]

        # Keep backups from last 7 days
        to_delete = service._get_backups_to_delete_by_days(backups, 7)

        assert len(to_delete) == 1
        assert "backup-old" in to_delete

    def test_retention_excludes_manual_and_prerestore(self):
        """Test that manual and pre-restore backups are excluded from regular retention."""
        service = BackupService(backup_path="/tmp")

        now = datetime.now(timezone.utc)
        backups = [
            BackupInfo(
                id="backup-scheduled",
                created_at=now - timedelta(days=10),
                backup_type=BackupType.SCHEDULED,
                size_bytes=1000,
                stats_users=1,
                stats_games=10,
                stats_tags=5,
            ),
            BackupInfo(
                id="backup-manual",
                created_at=now - timedelta(days=10),
                backup_type=BackupType.MANUAL,
                size_bytes=1000,
                stats_users=1,
                stats_games=10,
                stats_tags=5,
            ),
            BackupInfo(
                id="backup-prerestore",
                created_at=now - timedelta(days=10),
                backup_type=BackupType.PRE_RESTORE,
                size_bytes=1000,
                stats_users=1,
                stats_games=10,
                stats_tags=5,
            ),
        ]

        # Only scheduled backups should be considered
        to_delete = service._get_backups_to_delete_by_days(backups, 7)

        assert len(to_delete) == 1
        assert "backup-scheduled" in to_delete

"""
Tests for the setup restore endpoint.
"""

import io
import tarfile
import json
from datetime import datetime, timezone
from pathlib import Path

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select

from ..models.user import User


def create_minimal_backup_archive() -> bytes:
    """Create a minimal valid backup archive for testing."""
    buffer = io.BytesIO()

    with tarfile.open(fileobj=buffer, mode="w:gz") as tar:
        # Create manifest
        manifest = {
            "version": 1,
            "created_at": datetime.now(timezone.utc).isoformat(),
            "app_version": "1.0.0",
            "alembic_revision": "test",
            "backup_type": "manual",
            "database": {
                "file": "database.sql",
                "size_bytes": 100,
                "checksum": "sha256:test",
            },
            "files": {
                "cover_art": {
                    "count": 0,
                    "total_size_bytes": 0,
                    "checksum": "sha256:empty",
                },
                "logos": {
                    "count": 0,
                    "total_size_bytes": 0,
                    "checksum": "sha256:empty",
                },
            },
            "stats": {
                "users": 1,
                "games": 0,
                "tags": 0,
            },
        }

        # Add manifest to archive
        manifest_data = json.dumps(manifest).encode()
        manifest_info = tarfile.TarInfo(name="backup-test/manifest.json")
        manifest_info.size = len(manifest_data)
        tar.addfile(manifest_info, io.BytesIO(manifest_data))

        # Add empty database.sql
        db_data = b"-- Empty database\n"
        db_info = tarfile.TarInfo(name="backup-test/database.sql")
        db_info.size = len(db_data)
        tar.addfile(db_info, io.BytesIO(db_data))

    buffer.seek(0)
    return buffer.read()


class TestSetupRestoreEndpoint:
    """Test POST /api/auth/setup/restore endpoint."""

    def test_setup_restore_rejects_when_users_exist(self, client: TestClient, session: Session):
        """Test that restore fails when users already exist."""
        # Create a user first
        from ..core.security import get_password_hash
        user = User(
            username="existinguser",
            password_hash=get_password_hash("password123"),
        )
        session.add(user)
        session.commit()

        # Try to restore
        backup_data = create_minimal_backup_archive()
        response = client.post(
            "/api/auth/setup/restore",
            files={"file": ("backup.tar.gz", backup_data, "application/gzip")},
        )

        assert response.status_code == 400
        response_data = response.json()
        # Handle both error formats (custom error handler uses "error", FastAPI uses "detail")
        error_message = response_data.get("error") or response_data.get("detail", "")
        assert "already been completed" in error_message or "Users already exist" in error_message

    def test_setup_restore_rejects_invalid_file_format(self, client: TestClient):
        """Test that restore fails with non-.tar.gz file."""
        response = client.post(
            "/api/auth/setup/restore",
            files={"file": ("backup.txt", b"not a backup", "text/plain")},
        )

        assert response.status_code == 400
        response_data = response.json()
        # Handle both error formats (custom error handler uses "error", FastAPI uses "detail")
        error_message = response_data.get("error") or response_data.get("detail", "")
        assert "Invalid file format" in error_message or ".tar.gz" in error_message

    def test_setup_restore_rejects_invalid_archive(self, client: TestClient):
        """Test that restore fails with invalid archive content."""
        response = client.post(
            "/api/auth/setup/restore",
            files={"file": ("backup.tar.gz", b"not a valid archive", "application/gzip")},
        )

        assert response.status_code == 400
        # Should get validation error from BackupService

    def test_setup_restore_endpoint_exists(self, client: TestClient):
        """Test that the endpoint exists and accepts file uploads."""
        # Send without a file to verify endpoint routing
        response = client.post("/api/auth/setup/restore")

        # Should get 422 (validation error for missing file), not 404
        assert response.status_code == 422

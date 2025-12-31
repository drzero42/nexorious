"""Integration tests for backup API endpoints."""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session
from unittest.mock import patch, MagicMock, AsyncMock
from datetime import datetime, timezone

from app.services.backup_service import BackupInfo, BackupType


class TestBackupEndpointsAuthentication:
    """Tests for backup endpoint authentication requirements."""

    def test_list_backups_requires_admin(self, client: TestClient):
        """Test that listing backups requires admin auth."""
        response = client.get("/api/admin/backups")
        assert response.status_code == 403

    def test_list_backups_regular_user_forbidden(
        self, client: TestClient, auth_headers: dict
    ):
        """Test that regular users cannot list backups."""
        response = client.get("/api/admin/backups", headers=auth_headers)
        assert response.status_code == 403

    def test_get_config_requires_admin(self, client: TestClient):
        """Test that getting config requires admin auth."""
        response = client.get("/api/admin/backups/config")
        assert response.status_code == 403

    def test_get_config_regular_user_forbidden(
        self, client: TestClient, auth_headers: dict
    ):
        """Test that regular users cannot get backup config."""
        response = client.get("/api/admin/backups/config", headers=auth_headers)
        assert response.status_code == 403

    def test_create_backup_requires_admin(self, client: TestClient):
        """Test that creating backup requires admin auth."""
        response = client.post("/api/admin/backups")
        assert response.status_code == 403

    def test_create_backup_regular_user_forbidden(
        self, client: TestClient, auth_headers: dict
    ):
        """Test that regular users cannot create backups."""
        response = client.post("/api/admin/backups", headers=auth_headers)
        assert response.status_code == 403

    def test_delete_backup_requires_admin(self, client: TestClient):
        """Test that deleting backup requires admin auth."""
        response = client.delete("/api/admin/backups/test-backup-id")
        assert response.status_code == 403

    def test_download_backup_requires_admin(self, client: TestClient):
        """Test that downloading backup requires admin auth."""
        response = client.get("/api/admin/backups/test-backup-id/download")
        assert response.status_code == 403

    def test_restore_backup_requires_admin(self, client: TestClient):
        """Test that restoring backup requires admin auth."""
        response = client.post(
            "/api/admin/backups/test-backup-id/restore",
            json={"confirm": True}
        )
        assert response.status_code == 403


class TestBackupConfigEndpoints:
    """Tests for backup configuration endpoints."""

    def test_get_config_returns_defaults(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test getting backup configuration returns defaults or existing config."""
        response = client.get("/api/admin/backups/config", headers=admin_headers)

        assert response.status_code == 200
        data = response.json()

        # Verify response contains expected fields
        assert "schedule" in data
        assert "schedule_time" in data
        assert "retention_mode" in data
        assert "retention_value" in data
        assert "updated_at" in data

        # Verify default values
        assert data["schedule"] in ["manual", "daily", "weekly"]
        assert data["retention_mode"] in ["days", "count"]
        assert isinstance(data["retention_value"], int)

    def test_update_config_schedule(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test updating backup schedule."""
        response = client.put(
            "/api/admin/backups/config",
            headers=admin_headers,
            json={"schedule": "daily", "schedule_time": "03:00"}
        )

        assert response.status_code == 200
        data = response.json()
        assert data["schedule"] == "daily"
        assert data["schedule_time"] == "03:00"

    def test_update_config_retention_count(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test updating retention policy to count mode."""
        response = client.put(
            "/api/admin/backups/config",
            headers=admin_headers,
            json={"retention_mode": "count", "retention_value": 5}
        )

        assert response.status_code == 200
        data = response.json()
        assert data["retention_mode"] == "count"
        assert data["retention_value"] == 5

    def test_update_config_retention_days(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test updating retention policy to days mode."""
        response = client.put(
            "/api/admin/backups/config",
            headers=admin_headers,
            json={"retention_mode": "days", "retention_value": 30}
        )

        assert response.status_code == 200
        data = response.json()
        assert data["retention_mode"] == "days"
        assert data["retention_value"] == 30

    def test_update_config_weekly_schedule_with_day(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test updating to weekly schedule with schedule_day."""
        response = client.put(
            "/api/admin/backups/config",
            headers=admin_headers,
            json={"schedule": "weekly", "schedule_time": "04:00", "schedule_day": 0}
        )

        assert response.status_code == 200
        data = response.json()
        assert data["schedule"] == "weekly"
        assert data["schedule_time"] == "04:00"
        assert data["schedule_day"] == 0

    def test_update_config_invalid_time_format(
        self, client: TestClient, admin_headers: dict
    ):
        """Test that invalid time format is rejected."""
        response = client.put(
            "/api/admin/backups/config",
            headers=admin_headers,
            json={"schedule_time": "invalid"}
        )

        assert response.status_code == 422

    def test_update_config_invalid_retention_value(
        self, client: TestClient, admin_headers: dict
    ):
        """Test that invalid retention value is rejected."""
        response = client.put(
            "/api/admin/backups/config",
            headers=admin_headers,
            json={"retention_value": 0}
        )

        assert response.status_code == 422


class TestBackupListEndpoint:
    """Tests for backup listing endpoint."""

    def test_list_backups_empty(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test listing backups when none exist."""
        with patch("app.api.backup_endpoints.backup_service") as mock_service:
            mock_service.list_backups.return_value = []

            response = client.get("/api/admin/backups", headers=admin_headers)

            assert response.status_code == 200
            data = response.json()
            assert data["backups"] == []
            assert data["total"] == 0

    def test_list_backups_with_results(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test listing backups returns backup info."""
        mock_backup = BackupInfo(
            id="backup-2024-01-15T120000Z",
            created_at=datetime(2024, 1, 15, 12, 0, 0, tzinfo=timezone.utc),
            backup_type=BackupType.MANUAL,
            size_bytes=1024000,
            stats_users=5,
            stats_games=100,
            stats_tags=25,
        )

        with patch("app.api.backup_endpoints.backup_service") as mock_service:
            mock_service.list_backups.return_value = [mock_backup]

            response = client.get("/api/admin/backups", headers=admin_headers)

            assert response.status_code == 200
            data = response.json()
            assert data["total"] == 1
            assert len(data["backups"]) == 1

            backup = data["backups"][0]
            assert backup["id"] == "backup-2024-01-15T120000Z"
            assert backup["backup_type"] == "manual"
            assert backup["size_bytes"] == 1024000
            assert backup["stats"]["users"] == 5
            assert backup["stats"]["games"] == 100
            assert backup["stats"]["tags"] == 25


class TestBackupCreateEndpoint:
    """Tests for backup creation endpoint."""

    def test_create_backup_success(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test creating a backup successfully dispatches task."""
        mock_result = MagicMock()
        mock_result.task_id = "test-backup-task-id"

        with patch(
            "app.api.backup_endpoints.create_backup_task.kiq",
            new_callable=AsyncMock,
            return_value=mock_result,
        ):
            response = client.post("/api/admin/backups", headers=admin_headers)

            assert response.status_code == 202
            data = response.json()
            assert data["job_id"] == "pending"
            assert "message" in data
            assert "dispatched" in data["message"].lower()

    def test_create_backup_dispatches_manual_type(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test that backup task is dispatched with manual type."""
        mock_result = MagicMock()
        mock_result.task_id = "test-backup-task-id"

        with patch(
            "app.api.backup_endpoints.create_backup_task.kiq",
            new_callable=AsyncMock,
            return_value=mock_result,
        ) as mock_kiq:
            response = client.post("/api/admin/backups", headers=admin_headers)

            assert response.status_code == 202
            # Verify kiq was called with manual backup type
            mock_kiq.assert_called_once_with(backup_type="manual")


class TestBackupDeleteEndpoint:
    """Tests for backup deletion endpoint."""

    def test_delete_backup_success(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test deleting a backup successfully."""
        with patch("app.api.backup_endpoints.backup_service") as mock_service:
            mock_service.delete_backup.return_value = True

            response = client.delete(
                "/api/admin/backups/backup-2024-01-15T120000Z",
                headers=admin_headers
            )

            assert response.status_code == 200
            data = response.json()
            assert data["success"] is True
            assert "deleted" in data["message"].lower()

    def test_delete_backup_not_found(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test deleting a backup that doesn't exist."""
        with patch("app.api.backup_endpoints.backup_service") as mock_service:
            mock_service.delete_backup.return_value = False

            response = client.delete(
                "/api/admin/backups/nonexistent-backup",
                headers=admin_headers
            )

            assert response.status_code == 404


class TestBackupDownloadEndpoint:
    """Tests for backup download endpoint."""

    def test_download_backup_not_found(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test downloading a backup that doesn't exist."""
        with patch("app.api.backup_endpoints.backup_service") as mock_service:
            mock_path = MagicMock()
            mock_path.exists.return_value = False
            mock_service.get_backup_path.return_value = mock_path

            response = client.get(
                "/api/admin/backups/nonexistent-backup/download",
                headers=admin_headers
            )

            assert response.status_code == 404


class TestBackupRestoreEndpoint:
    """Tests for backup restore endpoint."""

    def test_restore_requires_confirmation(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test that restore requires confirm=true."""
        response = client.post(
            "/api/admin/backups/backup-2024-01-15T120000Z/restore",
            headers=admin_headers,
            json={"confirm": False}
        )

        # Verify that the restore is rejected without confirmation
        assert response.status_code == 400

    def test_restore_backup_not_found(
        self, client: TestClient, admin_headers: dict, session: Session
    ):
        """Test restoring a backup that doesn't exist."""
        with patch("app.api.backup_endpoints.backup_service") as mock_service:
            mock_service.restore_backup.side_effect = ValueError("Backup not found")

            response = client.post(
                "/api/admin/backups/nonexistent-backup/restore",
                headers=admin_headers,
                json={"confirm": True}
            )

            assert response.status_code == 400


class TestRestoreFromUploadEndpoint:
    """Tests for restore from upload endpoint."""

    def test_upload_restore_requires_admin(self, client: TestClient):
        """Test that upload restore requires admin auth."""
        import io

        file_content = b"fake tar content"

        response = client.post(
            "/api/admin/backups/restore/upload",
            files={"file": ("backup.tar.gz", io.BytesIO(file_content), "application/gzip")}
        )

        assert response.status_code == 403

    def test_upload_restore_requires_file(
        self, client: TestClient, admin_headers: dict
    ):
        """Test that upload restore requires a file."""
        response = client.post(
            "/api/admin/backups/restore/upload",
            headers=admin_headers,
        )

        # Should reject due to missing file - 422 for validation error
        assert response.status_code == 422

"""Tests for sync configuration API endpoints."""

from datetime import datetime, timezone

from fastapi.testclient import TestClient
from sqlmodel import Session

from app.models.user_sync_config import UserSyncConfig, SyncFrequency
from app.models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus


class TestSyncConfigEndpoints:
    """Tests for sync configuration endpoints."""

    def test_get_sync_configs_returns_defaults_for_new_user(
        self, client: TestClient, auth_headers: dict
    ):
        """GET /sync/config returns default configs for all platforms."""
        response = client.get(
            "/api/sync/config",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert "configs" in data
        assert "total" in data
        assert data["total"] == 4  # steam, epic, gog, psn

        # Check each platform has default values
        platforms = {c["platform"] for c in data["configs"]}
        assert platforms == {"steam", "epic", "gog", "psn"}

        for config in data["configs"]:
            assert config["frequency"] == "manual"
            assert config["auto_add"] is False

    def test_get_sync_configs_returns_existing_configs(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """GET /sync/config returns existing configs from database."""
        # Create a config for steam
        config = UserSyncConfig(
            user_id=test_user.id,
            platform="steam",
            frequency=SyncFrequency.DAILY,
            auto_add=True,
        )
        session.add(config)
        session.commit()

        response = client.get(
            "/api/sync/config",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        steam_config = next(c for c in data["configs"] if c["platform"] == "steam")
        assert steam_config["frequency"] == "daily"
        assert steam_config["auto_add"] is True

    def test_get_sync_config_single_platform(
        self, client: TestClient, auth_headers: dict
    ):
        """GET /sync/config/{platform} returns config for specific platform."""
        response = client.get(
            "/api/sync/config/steam",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert data["platform"] == "steam"
        assert data["frequency"] == "manual"  # Default
        assert data["auto_add"] is False

    def test_get_sync_config_invalid_platform(
        self, client: TestClient, auth_headers: dict
    ):
        """GET /sync/config/{platform} returns 422 for invalid platform."""
        response = client.get(
            "/api/sync/config/invalid_platform",
            headers=auth_headers,
        )

        assert response.status_code == 422

    def test_update_sync_config_creates_new(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """PUT /sync/config/{platform} creates config if it doesn't exist."""
        response = client.put(
            "/api/sync/config/steam",
            json={"frequency": "daily", "auto_add": True},
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert data["platform"] == "steam"
        assert data["frequency"] == "daily"
        assert data["auto_add"] is True

        # Verify in database
        from sqlmodel import select

        stmt = select(UserSyncConfig).where(
            UserSyncConfig.user_id == test_user.id,
            UserSyncConfig.platform == "steam",
        )
        config = session.exec(stmt).first()
        assert config is not None
        assert config.frequency == SyncFrequency.DAILY
        assert config.auto_add is True

    def test_update_sync_config_updates_existing(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """PUT /sync/config/{platform} updates existing config."""
        # Create initial config
        config = UserSyncConfig(
            user_id=test_user.id,
            platform="steam",
            frequency=SyncFrequency.MANUAL,
            auto_add=False,
        )
        session.add(config)
        session.commit()

        response = client.put(
            "/api/sync/config/steam",
            json={"frequency": "hourly", "auto_add": True},
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert data["frequency"] == "hourly"
        assert data["auto_add"] is True

    def test_update_sync_config_partial_update(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """PUT /sync/config/{platform} only updates provided fields."""
        # Create initial config
        config = UserSyncConfig(
            user_id=test_user.id,
            platform="steam",
            frequency=SyncFrequency.DAILY,
            auto_add=True,
        )
        session.add(config)
        session.commit()

        # Only update frequency
        response = client.put(
            "/api/sync/config/steam",
            json={"frequency": "weekly"},
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert data["frequency"] == "weekly"
        assert data["auto_add"] is True  # Unchanged


class TestManualSyncTrigger:
    """Tests for manual sync trigger endpoint."""

    def test_trigger_manual_sync_creates_job(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """POST /sync/{platform} creates a new sync job."""
        from unittest.mock import patch, AsyncMock

        with patch("app.api.sync.enqueue_task", new_callable=AsyncMock):
            response = client.post(
                "/api/sync/steam",
                headers=auth_headers,
            )

        assert response.status_code == 200
        data = response.json()

        assert data["platform"] == "steam"
        assert data["status"] == "queued"
        assert "job_id" in data

        # Verify job in database
        from sqlmodel import select

        stmt = select(Job).where(Job.id == data["job_id"])
        job = session.exec(stmt).first()
        assert job is not None
        assert job.job_type == BackgroundJobType.SYNC
        assert job.source == BackgroundJobSource.STEAM
        assert job.status == BackgroundJobStatus.PENDING

    def test_trigger_manual_sync_conflict_when_active(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """POST /sync/{platform} returns 409 if sync already in progress."""
        # Create an active sync job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            priority="high",
        )
        session.add(job)
        session.commit()

        response = client.post(
            "/api/sync/steam",
            headers=auth_headers,
        )

        assert response.status_code == 409
        error_data = response.json()
        # The error could be in "detail" or "error" depending on exception handling
        error_message = error_data.get("detail", error_data.get("error", ""))
        assert "already in progress" in error_message


class TestSyncStatus:
    """Tests for sync status endpoint."""

    def test_get_sync_status_not_syncing(
        self, client: TestClient, auth_headers: dict
    ):
        """GET /sync/{platform}/status returns is_syncing=false when no active job."""
        response = client.get(
            "/api/sync/steam/status",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert data["platform"] == "steam"
        assert data["is_syncing"] is False
        assert data["active_job_id"] is None

    def test_get_sync_status_syncing(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """GET /sync/{platform}/status returns is_syncing=true when job is active."""
        # Create an active sync job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            priority="high",
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.get(
            "/api/sync/steam/status",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert data["platform"] == "steam"
        assert data["is_syncing"] is True
        assert data["active_job_id"] == job.id

    def test_get_sync_status_includes_last_synced_at(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """GET /sync/{platform}/status includes last_synced_at from config."""
        # Create config with last_synced_at
        sync_time = datetime.now(timezone.utc)
        config = UserSyncConfig(
            user_id=test_user.id,
            platform="steam",
            frequency=SyncFrequency.DAILY,
            last_synced_at=sync_time,
        )
        session.add(config)
        session.commit()

        response = client.get(
            "/api/sync/steam/status",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert data["last_synced_at"] is not None


class TestSyncSchemas:
    """Tests for sync schemas."""

    def test_sync_frequency_enum_values(self):
        """SyncFrequency enum has expected values."""
        from app.schemas.sync import SyncFrequency

        assert SyncFrequency.MANUAL.value == "manual"
        assert SyncFrequency.HOURLY.value == "hourly"
        assert SyncFrequency.DAILY.value == "daily"
        assert SyncFrequency.WEEKLY.value == "weekly"

    def test_sync_platform_enum_values(self):
        """SyncPlatform enum has expected values."""
        from app.schemas.sync import SyncPlatform

        assert SyncPlatform.STEAM.value == "steam"
        assert SyncPlatform.EPIC.value == "epic"
        assert SyncPlatform.GOG.value == "gog"

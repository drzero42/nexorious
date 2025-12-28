"""Tests for job item API endpoints."""

import pytest
from datetime import datetime, timezone
from unittest.mock import AsyncMock, patch

from sqlmodel import Session

from app.models.user import User
from app.models.job import (
    Job,
    JobItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    JobItemStatus,
)


class TestRetryJobItem:
    """Tests for POST /api/job-items/{item_id}/retry endpoint."""

    @pytest.fixture(autouse=True)
    def mock_task_queue(self):
        """Mock the task queue to prevent actual task enqueuing during tests."""
        with patch(
            "app.api.job_items.enqueue_import_task",
            new_callable=AsyncMock,
        ):
            yield

    def test_retry_job_item_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test successful retry of a single failed item."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()

        failed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game_1",
            source_title="Test Game",
            status=JobItemStatus.FAILED,
            error_message="IGDB timeout",
            processed_at=datetime.now(timezone.utc),
        )
        session.add(failed_item)
        session.commit()

        response = client.post(
            f"/api/job-items/{failed_item.id}/retry", headers=auth_headers
        )
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "pending"
        assert data["error_message"] is None

    def test_retry_job_item_not_failed(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that retry fails if item is not in FAILED status."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()

        completed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game_1",
            source_title="Test Game",
            status=JobItemStatus.COMPLETED,
        )
        session.add(completed_item)
        session.commit()

        response = client.post(
            f"/api/job-items/{completed_item.id}/retry", headers=auth_headers
        )
        assert response.status_code == 400
        data = response.json()
        # Check for detail in the response (can be nested or flat)
        detail = data.get("detail", str(data))
        assert "not in failed status" in detail.lower()

    def test_retry_job_item_job_not_terminal(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that retry fails if parent job is not in terminal state."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()

        failed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game_1",
            source_title="Test Game",
            status=JobItemStatus.FAILED,
            error_message="Error",
        )
        session.add(failed_item)
        session.commit()

        response = client.post(
            f"/api/job-items/{failed_item.id}/retry", headers=auth_headers
        )
        assert response.status_code == 400
        data = response.json()
        # Check for detail in the response (can be nested or flat)
        detail = data.get("detail", str(data))
        assert "must be completed" in detail.lower()

    def test_retry_job_item_not_found(self, client, auth_headers):
        """Test retry returns 404 for non-existent item."""
        response = client.post(
            "/api/job-items/nonexistent-id/retry", headers=auth_headers
        )
        assert response.status_code == 404

    def test_retry_job_item_other_user(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users cannot retry other users' items."""
        job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()

        failed_item = JobItem(
            job_id=job.id,
            user_id=admin_user.id,
            item_key="game_1",
            source_title="Test Game",
            status=JobItemStatus.FAILED,
        )
        session.add(failed_item)
        session.commit()

        response = client.post(
            f"/api/job-items/{failed_item.id}/retry", headers=auth_headers
        )
        assert response.status_code == 404


class TestResolveJobItem:
    """Tests for POST /api/job-items/{item_id}/resolve endpoint."""

    @pytest.fixture(autouse=True)
    def mock_task_queue(self):
        """Mock the task queue to prevent actual task enqueuing during tests."""
        with patch(
            "app.api.job_items.enqueue_task",
            new_callable=AsyncMock,
        ) as mock_enqueue:
            self.mock_enqueue = mock_enqueue
            yield

    def test_resolve_job_item_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test successful resolve re-queues the worker task."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()

        pending_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="steam_12345",
            source_title="Test Game",
            status=JobItemStatus.PENDING_REVIEW,
            source_metadata_json='{"external_id": "12345", "storefront_id": "steam"}',
        )
        session.add(pending_item)
        session.commit()

        response = client.post(
            f"/api/job-items/{pending_item.id}/resolve",
            headers=auth_headers,
            json={"igdb_id": 999},
        )
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "pending"  # Reset to pending for worker
        assert data["resolved_igdb_id"] == 999

        # Verify task was enqueued
        self.mock_enqueue.assert_called_once()

    def test_resolve_job_item_not_pending_review(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that resolve fails if item is not in PENDING_REVIEW status."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()

        completed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="steam_12345",
            source_title="Test Game",
            status=JobItemStatus.COMPLETED,
        )
        session.add(completed_item)
        session.commit()

        response = client.post(
            f"/api/job-items/{completed_item.id}/resolve",
            headers=auth_headers,
            json={"igdb_id": 999},
        )
        assert response.status_code == 400

    def test_resolve_job_item_not_found(self, client, auth_headers):
        """Test resolve returns 404 for non-existent item."""
        response = client.post(
            "/api/job-items/nonexistent-id/resolve",
            headers=auth_headers,
            json={"igdb_id": 999},
        )
        assert response.status_code == 404


class TestSkipJobItem:
    """Tests for POST /api/job-items/{item_id}/skip endpoint."""

    def test_skip_job_item_triggers_job_completion(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that skipping the last PENDING_REVIEW item completes the job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()

        # One completed item, one pending review
        completed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="steam_11111",
            source_title="Already Done",
            status=JobItemStatus.COMPLETED,
        )
        pending_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="steam_12345",
            source_title="Test Game",
            status=JobItemStatus.PENDING_REVIEW,
        )
        session.add(completed_item)
        session.add(pending_item)
        session.commit()

        response = client.post(
            f"/api/job-items/{pending_item.id}/skip",
            headers=auth_headers,
            json={},
        )
        assert response.status_code == 200

        data = response.json()
        assert data["status"] == "skipped"

        # Refresh job from database and check it's completed
        session.refresh(job)
        assert job.status == BackgroundJobStatus.COMPLETED
        assert job.completed_at is not None

    def test_skip_job_item_not_pending_review(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that skip fails if item is not in PENDING_REVIEW status."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()

        completed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="steam_12345",
            source_title="Test Game",
            status=JobItemStatus.COMPLETED,
        )
        session.add(completed_item)
        session.commit()

        response = client.post(
            f"/api/job-items/{completed_item.id}/skip",
            headers=auth_headers,
            json={},
        )
        assert response.status_code == 400

    def test_skip_job_item_not_found(self, client, auth_headers):
        """Test skip returns 404 for non-existent item."""
        response = client.post(
            "/api/job-items/nonexistent-id/skip",
            headers=auth_headers,
            json={},
        )
        assert response.status_code == 404

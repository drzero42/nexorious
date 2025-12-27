"""Tests for job item API endpoints."""

from datetime import datetime, timezone

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

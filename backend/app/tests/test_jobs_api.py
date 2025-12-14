"""
Integration tests for Job management API endpoints.

Tests the following endpoints:
- GET /api/jobs - List user's jobs with filtering/pagination
- GET /api/jobs/{job_id} - Get job details
- POST /api/jobs/{job_id}/cancel - Cancel an in-progress job
- DELETE /api/jobs/{job_id} - Delete a job record
"""

import pytest
from sqlmodel import Session, select
from datetime import datetime, timezone

from ..models.user import User
from ..models.job import (
    Job,
    ReviewItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    ReviewItemStatus,
)


class TestListJobs:
    """Tests for GET /api/jobs endpoint."""

    def test_list_jobs_empty(self, client, auth_headers, test_user: User):
        """Test listing jobs when user has no jobs."""
        response = client.get("/api/jobs/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 0
        assert data["jobs"] == []
        assert data["page"] == 1
        assert data["per_page"] == 20
        assert data["pages"] == 1

    def test_list_jobs_single_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test listing jobs with a single job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PENDING,
        )
        session.add(job)
        session.commit()

        response = client.get("/api/jobs/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert len(data["jobs"]) == 1
        assert data["jobs"][0]["job_type"] == "import"
        assert data["jobs"][0]["source"] == "steam"
        assert data["jobs"][0]["status"] == "pending"

    def test_list_jobs_multiple_jobs(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test listing multiple jobs."""
        # Create multiple jobs
        for i, job_type in enumerate(BackgroundJobType):
            job = Job(
                user_id=test_user.id,
                job_type=job_type,
                source=BackgroundJobSource.STEAM,
                status=BackgroundJobStatus.COMPLETED,
            )
            session.add(job)
        session.commit()

        response = client.get("/api/jobs/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 3  # sync, import, export

    def test_list_jobs_filter_by_type(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test filtering jobs by type."""
        # Create jobs of different types
        for job_type in BackgroundJobType:
            job = Job(
                user_id=test_user.id,
                job_type=job_type,
                source=BackgroundJobSource.STEAM,
            )
            session.add(job)
        session.commit()

        response = client.get("/api/jobs/?job_type=import", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["jobs"][0]["job_type"] == "import"

    def test_list_jobs_filter_by_source(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test filtering jobs by source."""
        # Create jobs with different sources
        for source in [BackgroundJobSource.STEAM, BackgroundJobSource.DARKADIA]:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=source,
            )
            session.add(job)
        session.commit()

        response = client.get("/api/jobs/?source=steam", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["jobs"][0]["source"] == "steam"

    def test_list_jobs_filter_by_status(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test filtering jobs by status."""
        # Create jobs with different statuses
        for status in [
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
            BackgroundJobStatus.COMPLETED,
        ]:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.SYNC,
                source=BackgroundJobSource.STEAM,
                status=status,
            )
            session.add(job)
        session.commit()

        response = client.get("/api/jobs/?status=processing", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["jobs"][0]["status"] == "processing"

    def test_list_jobs_pagination(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test pagination of job list."""
        # Create 25 jobs
        for i in range(25):
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=BackgroundJobSource.STEAM,
            )
            session.add(job)
        session.commit()

        # First page
        response = client.get("/api/jobs/?page=1&per_page=10", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 25
        assert len(data["jobs"]) == 10
        assert data["page"] == 1
        assert data["pages"] == 3

        # Second page
        response = client.get("/api/jobs/?page=2&per_page=10", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert len(data["jobs"]) == 10
        assert data["page"] == 2

        # Third page (partial)
        response = client.get("/api/jobs/?page=3&per_page=10", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert len(data["jobs"]) == 5
        assert data["page"] == 3

    def test_list_jobs_sorting_desc(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test sorting jobs by created_at descending."""
        # Create jobs
        for i in range(3):
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=BackgroundJobSource.STEAM,
            )
            session.add(job)
            session.commit()  # Commit each to get different timestamps

        response = client.get(
            "/api/jobs/?sort_by=created_at&sort_order=desc", headers=auth_headers
        )
        assert response.status_code == 200

        data = response.json()
        assert len(data["jobs"]) == 3
        # Verify descending order (newest first)
        timestamps = [job["created_at"] for job in data["jobs"]]
        assert timestamps == sorted(timestamps, reverse=True)

    def test_list_jobs_only_own_jobs(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users only see their own jobs."""
        # Create job for test_user
        user_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(user_job)

        # Create job for admin_user
        admin_job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
        )
        session.add(admin_job)
        session.commit()

        response = client.get("/api/jobs/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["jobs"][0]["user_id"] == test_user.id

    def test_list_jobs_unauthenticated(self, client):
        """Test listing jobs without authentication."""
        response = client.get("/api/jobs/")
        assert response.status_code == 403  # No token = 403 Forbidden


class TestGetJob:
    """Tests for GET /api/jobs/{job_id} endpoint."""

    def test_get_job_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test getting a specific job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            progress_current=50,
            progress_total=100,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.get(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["id"] == job.id
        assert data["job_type"] == "import"
        assert data["source"] == "steam"
        assert data["status"] == "processing"
        assert data["progress_current"] == 50
        assert data["progress_total"] == 100
        assert data["progress_percent"] == 50
        assert data["is_terminal"] is False

    def test_get_job_with_review_items(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test getting a job with review items."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create review items
        for i in range(5):
            status = ReviewItemStatus.PENDING if i < 3 else ReviewItemStatus.MATCHED
            review_item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Game {i}",
                status=status,
            )
            session.add(review_item)
        session.commit()

        response = client.get(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["review_item_count"] == 5
        assert data["pending_review_count"] == 3

    def test_get_job_not_found(self, client, auth_headers):
        """Test getting a non-existent job."""
        response = client.get("/api/jobs/nonexistent-id", headers=auth_headers)
        assert response.status_code == 404
        assert "not found" in response.json()["error"].lower()

    def test_get_job_other_user(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users cannot view other users' jobs."""
        # Create job for admin_user
        admin_job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
        )
        session.add(admin_job)
        session.commit()
        session.refresh(admin_job)

        # Try to access as test_user
        response = client.get(f"/api/jobs/{admin_job.id}", headers=auth_headers)
        assert response.status_code == 404
        assert "not found" in response.json()["error"].lower()

    def test_get_job_computed_fields(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that computed fields are returned correctly."""
        now = datetime.now(timezone.utc)
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.COMPLETED,
            started_at=now,
            completed_at=now,
        )
        job.set_result_summary({"games_synced": 42})
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.get(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["is_terminal"] is True
        assert data["result_summary"]["games_synced"] == 42
        assert data["duration_seconds"] is not None


class TestCancelJob:
    """Tests for POST /api/jobs/{job_id}/cancel endpoint."""

    def test_cancel_pending_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cancelling a pending job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PENDING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/cancel", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert "pending" in data["message"]
        assert data["job"]["status"] == "cancelled"

        # Verify database state
        session.refresh(job)
        assert job.status == BackgroundJobStatus.CANCELLED
        assert job.completed_at is not None

    def test_cancel_processing_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cancelling a processing job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            progress_current=25,
            progress_total=100,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/cancel", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert data["job"]["status"] == "cancelled"

    def test_cancel_awaiting_review_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cancelling a job awaiting review."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/cancel", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True

    def test_cancel_already_completed_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that completed jobs cannot be cancelled."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/cancel", headers=auth_headers)
        assert response.status_code == 400
        assert "terminal state" in response.json()["error"].lower()

    def test_cancel_already_failed_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that failed jobs cannot be cancelled."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.FAILED,
            error_message="Network error",
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/cancel", headers=auth_headers)
        assert response.status_code == 400
        assert "terminal state" in response.json()["error"].lower()

    def test_cancel_already_cancelled_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that cancelled jobs cannot be cancelled again."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.CANCELLED,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/cancel", headers=auth_headers)
        assert response.status_code == 400

    def test_cancel_job_not_found(self, client, auth_headers):
        """Test cancelling a non-existent job."""
        response = client.post("/api/jobs/nonexistent-id/cancel", headers=auth_headers)
        assert response.status_code == 404

    def test_cancel_other_user_job(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users cannot cancel other users' jobs."""
        admin_job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(admin_job)
        session.commit()
        session.refresh(admin_job)

        response = client.post(f"/api/jobs/{admin_job.id}/cancel", headers=auth_headers)
        assert response.status_code == 404


class TestDeleteJob:
    """Tests for DELETE /api/jobs/{job_id} endpoint."""

    def test_delete_completed_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test deleting a completed job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()
        job_id = job.id

        response = client.delete(f"/api/jobs/{job_id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert data["deleted_job_id"] == job_id

        # Verify deletion
        deleted_job = session.get(Job, job_id)
        assert deleted_job is None

    def test_delete_failed_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test deleting a failed job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.FAILED,
            error_message="Import failed",
        )
        session.add(job)
        session.commit()
        job_id = job.id

        response = client.delete(f"/api/jobs/{job_id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True

    def test_delete_cancelled_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test deleting a cancelled job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.CANCELLED,
        )
        session.add(job)
        session.commit()
        job_id = job.id

        response = client.delete(f"/api/jobs/{job_id}", headers=auth_headers)
        assert response.status_code == 200

    def test_delete_pending_job_fails(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that pending jobs cannot be deleted."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PENDING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.delete(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 400
        assert "terminal state" in response.json()["error"].lower()

    def test_delete_processing_job_fails(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that processing jobs cannot be deleted."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.delete(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 400

    def test_delete_job_cascades_review_items(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that deleting a job also deletes associated review items."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()
        job_id = job.id

        # Create review items
        review_item_ids = []
        for i in range(3):
            review_item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Game {i}",
            )
            session.add(review_item)
        session.commit()

        # Store review item IDs before deletion
        review_items = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == job_id)
        ).all()
        review_item_ids = [item.id for item in review_items]
        assert len(review_item_ids) == 3

        # Delete job
        response = client.delete(f"/api/jobs/{job_id}", headers=auth_headers)
        assert response.status_code == 200

        # Verify review items are also deleted
        for review_id in review_item_ids:
            item = session.get(ReviewItem, review_id)
            assert item is None

    def test_delete_job_not_found(self, client, auth_headers):
        """Test deleting a non-existent job."""
        response = client.delete("/api/jobs/nonexistent-id", headers=auth_headers)
        assert response.status_code == 404

    def test_delete_other_user_job(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users cannot delete other users' jobs."""
        admin_job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(admin_job)
        session.commit()
        session.refresh(admin_job)

        response = client.delete(f"/api/jobs/{admin_job.id}", headers=auth_headers)
        assert response.status_code == 404


class TestJobsApiAuthentication:
    """Tests for authentication requirements on all job endpoints."""

    def test_list_jobs_no_auth(self, client):
        """Test that list jobs requires authentication."""
        response = client.get("/api/jobs/")
        assert response.status_code == 403  # No token = 403 Forbidden

    def test_get_job_no_auth(self, client):
        """Test that get job requires authentication."""
        response = client.get("/api/jobs/some-id")
        assert response.status_code == 403  # No token = 403 Forbidden

    def test_cancel_job_no_auth(self, client):
        """Test that cancel job requires authentication."""
        response = client.post("/api/jobs/some-id/cancel")
        assert response.status_code == 403  # No token = 403 Forbidden

    def test_delete_job_no_auth(self, client):
        """Test that delete job requires authentication."""
        response = client.delete("/api/jobs/some-id")
        assert response.status_code == 403  # No token = 403 Forbidden


class TestJobResponseFields:
    """Tests for job response field correctness."""

    def test_job_response_all_fields(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that all expected fields are in the response."""
        now = datetime.now(timezone.utc)
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
            priority=BackgroundJobPriority.HIGH,
            progress_current=100,
            progress_total=100,
            error_message=None,
            file_path="/exports/test.json",
            taskiq_task_id="task-123",
            started_at=now,
            completed_at=now,
        )
        job.set_result_summary({"exported": 50})
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.get(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()

        # Verify all expected fields
        assert "id" in data
        assert "user_id" in data
        assert "job_type" in data
        assert "source" in data
        assert "status" in data
        assert "priority" in data
        assert "progress_current" in data
        assert "progress_total" in data
        assert "progress_percent" in data
        assert "result_summary" in data
        assert "error_message" in data
        assert "file_path" in data
        assert "taskiq_task_id" in data
        assert "created_at" in data
        assert "started_at" in data
        assert "completed_at" in data
        assert "is_terminal" in data
        assert "duration_seconds" in data
        assert "review_item_count" in data
        assert "pending_review_count" in data

        # Verify specific values
        assert data["file_path"] == "/exports/test.json"
        assert data["taskiq_task_id"] == "task-123"
        assert data["result_summary"]["exported"] == 50
        assert data["progress_percent"] == 100
        assert data["is_terminal"] is True

    def test_job_enum_values(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that enum values are serialized correctly."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.GOG,
            status=BackgroundJobStatus.AWAITING_REVIEW,
            priority=BackgroundJobPriority.LOW,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.get(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["job_type"] == "sync"
        assert data["source"] == "gog"
        assert data["status"] == "awaiting_review"
        assert data["priority"] == "low"


class TestJobsApiEdgeCases:
    """Tests for edge cases and boundary conditions."""

    def test_list_jobs_max_per_page(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test maximum items per page limit."""
        response = client.get("/api/jobs/?per_page=101", headers=auth_headers)
        assert response.status_code == 422  # Validation error

    def test_list_jobs_negative_page(self, client, auth_headers):
        """Test negative page number."""
        response = client.get("/api/jobs/?page=0", headers=auth_headers)
        assert response.status_code == 422  # Validation error

    def test_list_jobs_invalid_sort_order(self, client, auth_headers):
        """Test invalid sort order."""
        response = client.get("/api/jobs/?sort_order=invalid", headers=auth_headers)
        assert response.status_code == 422

    def test_list_jobs_combined_filters(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test combining multiple filters."""
        # Create various jobs
        job1 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.COMPLETED,
        )
        job2 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.COMPLETED,
        )
        job3 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add_all([job1, job2, job3])
        session.commit()

        response = client.get(
            "/api/jobs/?job_type=import&source=steam&status=completed",
            headers=auth_headers,
        )
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["jobs"][0]["job_type"] == "import"
        assert data["jobs"][0]["source"] == "steam"
        assert data["jobs"][0]["status"] == "completed"

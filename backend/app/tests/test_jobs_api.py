"""
Integration tests for Job management API endpoints.

Tests the following endpoints:
- GET /api/jobs - List user's jobs with filtering/pagination
- GET /api/jobs/{job_id} - Get job details
- POST /api/jobs/{job_id}/cancel - Cancel an in-progress job
- DELETE /api/jobs/{job_id} - Delete a job record
"""

from sqlmodel import Session, select
from datetime import datetime, timezone

from ..models.user import User
from ..models.job import (
    Job,
    JobItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    JobItemStatus,
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
        for source in [BackgroundJobSource.STEAM, BackgroundJobSource.GOG]:
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
        """Test filtering jobs by terminal status (filtering works on DB status).

        Note: PENDING/PROCESSING are derived from JobItems, but FAILED/CANCELLED
        are stored explicitly. This test verifies filtering by FAILED status.
        """
        # Create job with FAILED status (explicit, not derived)
        job_failed = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.FAILED,
            error_message="Test failure",
        )
        session.add(job_failed)

        # Create job with PENDING status (default)
        job_pending = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job_pending)
        session.commit()

        # Filter by failed status
        response = client.get("/api/jobs/?status=failed", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["jobs"][0]["status"] == "failed"

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
        """Test getting a specific job (status is derived from JobItems)."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        # Add a PROCESSING item to derive PROCESSING status
        session.add(JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game-1",
            source_title="Test Game",
            status=JobItemStatus.PROCESSING,
        ))
        session.commit()
        session.refresh(job)

        response = client.get(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["id"] == job.id
        assert data["job_type"] == "import"
        assert data["source"] == "steam"
        assert data["status"] == "processing"
        assert data["is_terminal"] is False

    def test_get_job_with_review_items(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test getting a job with review items."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create job items
        for i in range(5):
            status = JobItemStatus.PENDING if i < 3 else JobItemStatus.COMPLETED
            job_item = JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key=f"game-{i}",
                source_title=f"Game {i}",
                status=status,
            )
            session.add(job_item)
        session.commit()

        response = client.get(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()

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
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.get(f"/api/jobs/{job.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["is_terminal"] is True
        assert data["duration_seconds"] is not None


class TestCancelJob:
    """Tests for POST /api/jobs/{job_id}/cancel endpoint."""

    def test_cancel_pending_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cancelling a pending job - job is immediately deleted."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PENDING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)
        job_id = job.id

        response = client.post(f"/api/jobs/{job_id}/cancel", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert data["message"] == "Job cancelled and removed"
        assert data["job"] is None

        # Verify job is deleted from database
        deleted_job = session.get(Job, job_id)
        assert deleted_job is None

    def test_cancel_processing_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cancelling a processing job - job is immediately deleted."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)
        job_id = job.id

        response = client.post(f"/api/jobs/{job_id}/cancel", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert data["message"] == "Job cancelled and removed"
        assert data["job"] is None

        # Verify job is deleted from database
        deleted_job = session.get(Job, job_id)
        assert deleted_job is None

    def test_cancel_awaiting_review_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test cancelling a job awaiting review."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
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
            source=BackgroundJobSource.STEAM,
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

        # Create job items
        job_item_ids = []
        for i in range(3):
            job_item = JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key=f"game-{i}",
                source_title=f"Game {i}",
            )
            session.add(job_item)
        session.commit()

        # Store job item IDs before deletion
        job_items = session.exec(
            select(JobItem).where(JobItem.job_id == job_id)
        ).all()
        job_item_ids = [item.id for item in job_items]
        assert len(job_item_ids) == 3

        # Delete job
        response = client.delete(f"/api/jobs/{job_id}", headers=auth_headers)
        assert response.status_code == 200

        # Verify job items are also deleted
        for item_id in job_item_ids:
            item = session.get(JobItem, item_id)
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
            error_message=None,
            file_path="/exports/test.json",
            started_at=now,
            completed_at=now,
        )
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
        assert "error_message" in data
        assert "file_path" in data
        assert "created_at" in data
        assert "started_at" in data
        assert "completed_at" in data
        assert "is_terminal" in data
        assert "duration_seconds" in data

        # Verify specific values
        assert data["file_path"] == "/exports/test.json"
        assert data["is_terminal"] is True

    def test_job_enum_values(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that enum values are serialized correctly."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.GOG,
            status=BackgroundJobStatus.PROCESSING,
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
        # Status is derived from JobItems - no items means PENDING
        assert data["status"] == "pending"
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
        """Test combining multiple filters.

        Note: Status filtering uses the explicit DB status, not derived status.
        FAILED and CANCELLED are terminal/explicit statuses stored in DB.
        """
        # Create various jobs with FAILED status (explicit/terminal)
        job1 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.FAILED,
        )
        job2 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.GOG,
            status=BackgroundJobStatus.FAILED,
        )
        job3 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.FAILED,
        )
        session.add_all([job1, job2, job3])
        session.commit()

        response = client.get(
            "/api/jobs/?job_type=import&source=steam&status=failed",
            headers=auth_headers,
        )
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["jobs"][0]["job_type"] == "import"
        assert data["jobs"][0]["source"] == "steam"
        assert data["jobs"][0]["status"] == "failed"



class TestGetActiveJob:
    """Tests for GET /api/jobs/active/{job_type} endpoint."""

    def test_get_active_job_none(self, client, auth_headers, test_user: User):
        """Test getting active job when no jobs exist."""
        response = client.get("/api/jobs/active/import", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() is None

    def test_get_active_job_pending(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test getting active job that is pending."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PENDING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.get("/api/jobs/active/import", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data is not None
        assert data["id"] == job.id
        assert data["job_type"] == "import"

    def test_get_active_job_processing(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test getting active job that is processing."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.get("/api/jobs/active/sync", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data is not None
        assert data["id"] == job.id
        assert data["job_type"] == "sync"

    def test_get_active_job_excludes_completed(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that completed jobs are not returned as active."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()

        response = client.get("/api/jobs/active/export", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() is None

    def test_get_active_job_excludes_failed(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that failed jobs are not returned as active."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.FAILED,
            error_message="Test error",
        )
        session.add(job)
        session.commit()

        response = client.get("/api/jobs/active/import", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() is None

    def test_get_active_job_excludes_cancelled(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that cancelled jobs are not returned as active."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.CANCELLED,
        )
        session.add(job)
        session.commit()

        response = client.get("/api/jobs/active/sync", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() is None

    def test_get_active_job_filters_by_type(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that active job filters correctly by job type."""
        # Create jobs of different types
        import_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        sync_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add_all([import_job, sync_job])
        session.commit()
        session.refresh(import_job)
        session.refresh(sync_job)

        # Check import type returns import job
        response = client.get("/api/jobs/active/import", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert data["id"] == import_job.id
        assert data["job_type"] == "import"

        # Check sync type returns sync job
        response = client.get("/api/jobs/active/sync", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert data["id"] == sync_job.id
        assert data["job_type"] == "sync"

    def test_get_active_job_returns_most_recent(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that most recent active job is returned when multiple exist."""
        # Create older job
        older_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PENDING,
        )
        session.add(older_job)
        session.commit()

        # Create newer job
        newer_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.GOG,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(newer_job)
        session.commit()
        session.refresh(newer_job)

        response = client.get("/api/jobs/active/import", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["id"] == newer_job.id

    def test_get_active_job_only_own_jobs(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that only user's own active jobs are returned."""
        # Create admin's job
        admin_job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(admin_job)
        session.commit()

        # User should not see admin's job
        response = client.get("/api/jobs/active/import", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() is None

    def test_get_active_job_no_auth(self, client):
        """Test that getting active job requires authentication."""
        response = client.get("/api/jobs/active/import")
        assert response.status_code == 403

    def test_get_active_job_invalid_type(self, client, auth_headers):
        """Test that invalid job type returns validation error."""
        response = client.get("/api/jobs/active/invalid", headers=auth_headers)
        assert response.status_code == 422


class TestPendingReviewCount:
    """Tests for GET /api/jobs/pending-review-count endpoint."""

    def test_pending_review_count_empty(self, client, auth_headers, test_user: User):
        """Test pending review count when user has no items."""
        response = client.get("/api/jobs/pending-review-count", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() == {"pending_review_count": 0}

    def test_pending_review_count_with_items(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test pending review count with items needing review."""
        # Create a job with items
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()

        # Add items with different statuses
        for i, status in enumerate([
            JobItemStatus.PENDING_REVIEW,
            JobItemStatus.PENDING_REVIEW,
            JobItemStatus.COMPLETED,
            JobItemStatus.FAILED,
        ]):
            item = JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key=f"game_{i}",
                source_title=f"Game {i}",
                status=status,
            )
            session.add(item)
        session.commit()

        response = client.get("/api/jobs/pending-review-count", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() == {"pending_review_count": 2}

    def test_pending_review_count_excludes_other_users(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that pending review count only includes current user's items."""
        # Create another user
        other_user = User(
            username="other_user",
            password_hash="$2b$12$test_hash",
        )
        session.add(other_user)
        session.commit()

        # Create job for other user
        other_job = Job(
            user_id=other_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(other_job)
        session.commit()

        # Add pending review item for other user
        other_item = JobItem(
            job_id=other_job.id,
            user_id=other_user.id,
            item_key="other_game",
            source_title="Other Game",
            status=JobItemStatus.PENDING_REVIEW,
        )
        session.add(other_item)
        session.commit()

        # Current user should see 0
        response = client.get("/api/jobs/pending-review-count", headers=auth_headers)
        assert response.status_code == 200
        assert response.json() == {"pending_review_count": 0}


# Note: TestDiscardImport class removed - /discard endpoint does not exist.
# Delete functionality is provided by DELETE /api/jobs/{job_id} endpoint instead.



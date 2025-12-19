"""Tests for batch job service."""

from unittest.mock import MagicMock
from sqlmodel import Session

from app.services.batch_job_service import BatchJobService
from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ImportJobSubtype,
)
from app.models.batch_constants import BatchOperationType


class TestBatchJobService:
    """Tests for BatchJobService."""

    def test_create_batch_job_auto_match(self):
        """Should create a job for auto-match batch operation."""
        mock_session = MagicMock(spec=Session)
        service = BatchJobService(mock_session)

        job = service.create_batch_job(
            user_id="user-123",
            operation_type=BatchOperationType.AUTO_MATCH,
            source=BackgroundJobSource.DARKADIA,
            total_items=50,
        )

        assert job.user_id == "user-123"
        assert job.job_type == BackgroundJobType.IMPORT
        assert job.source == BackgroundJobSource.DARKADIA
        assert job.import_subtype == ImportJobSubtype.AUTO_MATCH
        assert job.progress_total == 50
        assert job.status == BackgroundJobStatus.PROCESSING
        mock_session.add.assert_called_once_with(job)
        mock_session.commit.assert_called_once()

    def test_create_batch_job_sync(self):
        """Should create a job for sync batch operation."""
        mock_session = MagicMock(spec=Session)
        service = BatchJobService(mock_session)

        job = service.create_batch_job(
            user_id="user-123",
            operation_type=BatchOperationType.SYNC,
            source=BackgroundJobSource.DARKADIA,
            total_items=25,
        )

        assert job.import_subtype == ImportJobSubtype.BULK_SYNC

    def test_get_batch_job(self):
        """Should retrieve a batch job by ID."""
        mock_session = MagicMock(spec=Session)
        mock_job = Job(
            id="job-123",
            user_id="user-123",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )
        mock_session.get.return_value = mock_job

        service = BatchJobService(mock_session)
        job = service.get_batch_job("job-123")

        assert job == mock_job
        mock_session.get.assert_called_once_with(Job, "job-123")

    def test_update_batch_progress(self):
        """Should update batch job progress."""
        mock_session = MagicMock(spec=Session)
        job = Job(
            id="job-123",
            user_id="user-123",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            progress_total=100,
            progress_current=0,
        )
        mock_session.get.return_value = job

        service = BatchJobService(mock_session)
        updated_job = service.update_batch_progress(
            job_id="job-123",
            processed_count=5,
            successful_count=4,
            failed_count=1,
            processed_ids=["g1", "g2", "g3", "g4", "g5"],
            failed_ids=["g5"],
            errors=["g5 failed: no match"],
        )

        assert updated_job is not None
        assert updated_job.progress_current == 5
        assert updated_job.successful_items == 4
        assert updated_job.failed_items == 1
        assert updated_job.get_processed_item_ids() == ["g1", "g2", "g3", "g4", "g5"]
        assert updated_job.get_failed_item_ids() == ["g5"]

    def test_cancel_batch_job(self):
        """Should cancel a batch job."""
        mock_session = MagicMock(spec=Session)
        job = Job(
            id="job-123",
            user_id="user-123",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
        )
        mock_session.get.return_value = job

        service = BatchJobService(mock_session)
        cancelled = service.cancel_batch_job("job-123", "user-123")

        assert cancelled is not None
        assert cancelled.status == BackgroundJobStatus.CANCELLED

    def test_cancel_batch_job_wrong_user(self):
        """Should return None when cancelling job for wrong user."""
        mock_session = MagicMock(spec=Session)
        job = Job(
            id="job-123",
            user_id="user-123",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
        )
        mock_session.get.return_value = job

        service = BatchJobService(mock_session)
        result = service.cancel_batch_job("job-123", "wrong-user")

        assert result is None

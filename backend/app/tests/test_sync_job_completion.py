"""Tests for sync job completion logic."""

from sqlmodel import Session

from app.worker.tasks.sync.process_item import _check_and_update_job_completion
from app.models.user import User
from app.models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobStatus,
    BackgroundJobType,
    BackgroundJobSource,
)


class TestJobCompletionBlocking:
    """Test that PENDING_REVIEW items block job completion."""

    def test_job_not_completed_when_pending_review_items_exist(
        self, session: Session, test_user: User
    ):
        """Job should stay PROCESSING when PENDING_REVIEW items exist."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            total_items=3,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create items: 1 completed, 1 pending_review, 1 skipped
        items = [
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game1",
                source_title="Game 1",
                status=JobItemStatus.COMPLETED,
            ),
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game2",
                source_title="Game 2",
                status=JobItemStatus.PENDING_REVIEW,
            ),
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game3",
                source_title="Game 3",
                status=JobItemStatus.SKIPPED,
            ),
        ]
        for item in items:
            session.add(item)
        session.commit()

        # Check completion - should NOT complete
        result = _check_and_update_job_completion(session, job.id)

        assert result is False
        session.refresh(job)
        assert job.status == BackgroundJobStatus.PROCESSING

    def test_job_completed_when_no_pending_review_items(
        self, session: Session, test_user: User
    ):
        """Job should complete when all items are terminal (no PENDING_REVIEW)."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            total_items=3,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create items: all terminal (completed, skipped, failed)
        items = [
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game1",
                source_title="Game 1",
                status=JobItemStatus.COMPLETED,
            ),
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game2",
                source_title="Game 2",
                status=JobItemStatus.SKIPPED,
            ),
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game3",
                source_title="Game 3",
                status=JobItemStatus.FAILED,
            ),
        ]
        for item in items:
            session.add(item)
        session.commit()

        # Check completion - should complete
        result = _check_and_update_job_completion(session, job.id)

        assert result is True
        session.refresh(job)
        assert job.status == BackgroundJobStatus.COMPLETED

    def test_job_not_completed_when_pending_items_exist(
        self, session: Session, test_user: User
    ):
        """Job should stay PROCESSING when PENDING items exist."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            total_items=2,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create items: 1 completed, 1 pending
        items = [
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game1",
                source_title="Game 1",
                status=JobItemStatus.COMPLETED,
            ),
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game2",
                source_title="Game 2",
                status=JobItemStatus.PENDING,
            ),
        ]
        for item in items:
            session.add(item)
        session.commit()

        # Check completion - should NOT complete
        result = _check_and_update_job_completion(session, job.id)

        assert result is False
        session.refresh(job)
        assert job.status == BackgroundJobStatus.PROCESSING

    def test_job_not_completed_when_processing_items_exist(
        self, session: Session, test_user: User
    ):
        """Job should stay PROCESSING when PROCESSING items exist."""
        # Create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            total_items=2,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create items: 1 completed, 1 processing
        items = [
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game1",
                source_title="Game 1",
                status=JobItemStatus.COMPLETED,
            ),
            JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key="game2",
                source_title="Game 2",
                status=JobItemStatus.PROCESSING,
            ),
        ]
        for item in items:
            session.add(item)
        session.commit()

        # Check completion - should NOT complete
        result = _check_and_update_job_completion(session, job.id)

        assert result is False
        session.refresh(job)
        assert job.status == BackgroundJobStatus.PROCESSING

    def test_job_not_updated_when_already_terminal(
        self, session: Session, test_user: User
    ):
        """Job should not be updated if already in terminal state."""
        # Create a job that's already completed
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.COMPLETED,
            total_items=1,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create a completed item
        item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game1",
            source_title="Game 1",
            status=JobItemStatus.COMPLETED,
        )
        session.add(item)
        session.commit()

        # Check completion - should return False (not updated)
        result = _check_and_update_job_completion(session, job.id)

        assert result is False
        session.refresh(job)
        assert job.status == BackgroundJobStatus.COMPLETED

    def test_returns_false_for_nonexistent_job(self, session: Session):
        """Should return False if job doesn't exist."""
        result = _check_and_update_job_completion(session, "nonexistent-job-id")

        assert result is False

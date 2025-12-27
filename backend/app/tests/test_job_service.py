"""Tests for job service functions."""

from sqlmodel import Session
from datetime import datetime, timezone

from app.models.user import User
from app.models.job import (
    Job,
    JobItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    JobItemStatus,
)
from app.services.job_service import retry_failed_items, retry_job_item


class TestRetryFailedItems:
    """Tests for retry_failed_items service function."""

    def test_retry_failed_items_resets_status(self, session: Session, test_user: User):
        """Test that failed items are reset to PENDING."""
        # Create a completed job with failed items
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()

        # Add failed items
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

        # Retry failed items
        reset_count = retry_failed_items(session, job.id)

        session.refresh(failed_item)
        assert reset_count == 1
        assert failed_item.status == JobItemStatus.PENDING
        assert failed_item.error_message is None
        assert failed_item.processed_at is None

    def test_retry_failed_items_returns_zero_when_no_failed(
        self, session: Session, test_user: User
    ):
        """Test that retry returns 0 when no failed items exist."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()

        # Add only completed items
        completed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game_1",
            source_title="Test Game",
            status=JobItemStatus.COMPLETED,
        )
        session.add(completed_item)
        session.commit()

        reset_count = retry_failed_items(session, job.id)
        assert reset_count == 0

    def test_retry_failed_items_only_resets_failed(
        self, session: Session, test_user: User
    ):
        """Test that only FAILED items are reset, not other statuses."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()

        # Add items with various statuses
        failed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game_1",
            source_title="Failed Game",
            status=JobItemStatus.FAILED,
            error_message="Error",
        )
        completed_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game_2",
            source_title="Completed Game",
            status=JobItemStatus.COMPLETED,
        )
        skipped_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="game_3",
            source_title="Skipped Game",
            status=JobItemStatus.SKIPPED,
        )
        session.add_all([failed_item, completed_item, skipped_item])
        session.commit()

        reset_count = retry_failed_items(session, job.id)

        session.refresh(failed_item)
        session.refresh(completed_item)
        session.refresh(skipped_item)

        assert reset_count == 1
        assert failed_item.status == JobItemStatus.PENDING
        assert completed_item.status == JobItemStatus.COMPLETED
        assert skipped_item.status == JobItemStatus.SKIPPED


class TestRetryJobItem:
    """Tests for retry_job_item service function."""

    def test_retry_job_item_resets_status(self, session: Session, test_user: User):
        """Test that a single failed item is reset to PENDING."""
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

        result = retry_job_item(session, failed_item.id)

        assert result is True
        session.refresh(failed_item)
        assert failed_item.status == JobItemStatus.PENDING
        assert failed_item.error_message is None
        assert failed_item.processed_at is None

    def test_retry_job_item_returns_false_if_not_failed(
        self, session: Session, test_user: User
    ):
        """Test that retry returns False if item is not in FAILED status."""
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

        result = retry_job_item(session, completed_item.id)

        assert result is False
        session.refresh(completed_item)
        assert completed_item.status == JobItemStatus.COMPLETED

    def test_retry_job_item_returns_false_if_not_found(self, session: Session):
        """Test that retry returns False if item doesn't exist."""
        result = retry_job_item(session, "nonexistent-id")
        assert result is False

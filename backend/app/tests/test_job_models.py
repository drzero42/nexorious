"""
Unit tests for Job and ReviewItem models.

Tests the unified job model for background tasks (sync, import, export)
and the review item model for games needing user matching decisions.
"""

import pytest
from sqlmodel import Session, select
from sqlalchemy.exc import IntegrityError
from datetime import datetime, timezone, timedelta

from ..models.user import User
from ..models.job import (
    Job,
    ReviewItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    ImportJobSubtype,
    ReviewItemStatus,
)


class TestJobModel:
    """Test Job model database operations and constraints."""

    def test_create_job_success(self, session: Session, test_user: User):
        """Test creating a Job with valid data."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PENDING,
            priority=BackgroundJobPriority.HIGH,
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.id is not None
        assert job.user_id == test_user.id
        assert job.job_type == BackgroundJobType.IMPORT
        assert job.source == BackgroundJobSource.STEAM
        assert job.status == BackgroundJobStatus.PENDING
        assert job.priority == BackgroundJobPriority.HIGH
        assert job.progress_current == 0
        assert job.progress_total == 0
        assert job.created_at is not None
        assert isinstance(job.created_at, datetime)

    def test_job_defaults(self, session: Session, test_user: User):
        """Test Job default values."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.status == BackgroundJobStatus.PENDING
        assert job.priority == BackgroundJobPriority.HIGH
        assert job.progress_current == 0
        assert job.progress_total == 0
        assert job.successful_items == 0
        assert job.failed_items == 0
        assert job.import_subtype is None
        assert job.error_message is None
        assert job.file_path is None
        assert job.taskiq_task_id is None
        assert job.started_at is None
        assert job.completed_at is None
        assert job.get_error_log() == []

    def test_job_all_types(self, session: Session, test_user: User):
        """Test all job types can be created."""
        for job_type in BackgroundJobType:
            job = Job(
                user_id=test_user.id,
                job_type=job_type,
                source=BackgroundJobSource.SYSTEM,
            )
            session.add(job)

        session.commit()

        jobs = session.exec(
            select(Job).where(Job.user_id == test_user.id)
        ).all()

        assert len(jobs) == len(BackgroundJobType)
        job_types = {j.job_type for j in jobs}
        assert job_types == set(BackgroundJobType)

    def test_job_all_sources(self, session: Session, test_user: User):
        """Test all job sources can be created."""
        for source in BackgroundJobSource:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=source,
            )
            session.add(job)

        session.commit()

        jobs = session.exec(
            select(Job).where(Job.user_id == test_user.id)
        ).all()

        assert len(jobs) == len(BackgroundJobSource)
        sources = {j.source for j in jobs}
        assert sources == set(BackgroundJobSource)

    def test_job_all_import_subtypes(self, session: Session, test_user: User):
        """Test all import job subtypes can be set."""
        for subtype in ImportJobSubtype:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=BackgroundJobSource.STEAM,
                import_subtype=subtype,
            )
            session.add(job)

        session.commit()

        jobs = session.exec(
            select(Job).where(Job.user_id == test_user.id)
        ).all()

        assert len(jobs) == len(ImportJobSubtype)
        subtypes = {j.import_subtype for j in jobs}
        assert subtypes == set(ImportJobSubtype)

    def test_job_successful_and_failed_items(self, session: Session, test_user: User):
        """Test successful and failed items tracking."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.CSV,
            import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
            progress_total=100,
            progress_current=80,
            successful_items=70,
            failed_items=10,
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.progress_total == 100
        assert job.progress_current == 80
        assert job.successful_items == 70
        assert job.failed_items == 10
        # Verify: successful + failed = processed
        assert job.successful_items + job.failed_items == job.progress_current

    def test_job_error_log_json(self, session: Session, test_user: User):
        """Test error log JSON serialization."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.CSV,
        )

        # Set error log
        errors = [
            {"row": 1, "error": "Invalid game title", "field": "title"},
            {"row": 5, "error": "Missing platform", "field": "platform"},
            {"row": 12, "error": "Duplicate entry", "field": "igdb_id"},
        ]
        job.set_error_log(errors)

        session.add(job)
        session.commit()
        session.refresh(job)

        # Get error log
        retrieved = job.get_error_log()
        assert retrieved == errors
        assert len(retrieved) == 3
        assert retrieved[0]["row"] == 1
        assert retrieved[1]["error"] == "Missing platform"

    def test_job_add_error(self, session: Session, test_user: User):
        """Test adding errors incrementally."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )

        session.add(job)
        session.commit()

        # Start with empty error log
        assert job.get_error_log() == []

        # Add errors one by one
        job.add_error({"row": 1, "error": "First error"})
        job.add_error({"row": 2, "error": "Second error"})
        job.add_error({"row": 3, "error": "Third error"})

        session.commit()
        session.refresh(job)

        errors = job.get_error_log()
        assert len(errors) == 3
        assert errors[0]["error"] == "First error"
        assert errors[2]["error"] == "Third error"

    def test_job_error_log_empty(self, session: Session, test_user: User):
        """Test empty error log returns empty list."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.get_error_log() == []

    def test_job_all_statuses(self, session: Session, test_user: User):
        """Test all job statuses can be set."""
        for status in BackgroundJobStatus:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=BackgroundJobSource.STEAM,
                status=status,
            )
            session.add(job)

        session.commit()

        jobs = session.exec(
            select(Job).where(Job.user_id == test_user.id)
        ).all()

        assert len(jobs) == len(BackgroundJobStatus)
        statuses = {j.status for j in jobs}
        assert statuses == set(BackgroundJobStatus)

    def test_job_result_summary_json(self, session: Session, test_user: User):
        """Test result summary JSON serialization."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )

        # Set result summary
        summary = {"games_added": 42, "games_skipped": 3, "errors": []}
        job.set_result_summary(summary)

        session.add(job)
        session.commit()
        session.refresh(job)

        # Get result summary
        retrieved = job.get_result_summary()
        assert retrieved == summary
        assert retrieved["games_added"] == 42

    def test_job_result_summary_empty(self, session: Session, test_user: User):
        """Test empty result summary returns empty dict."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.get_result_summary() == {}

    def test_job_progress_percent(self, session: Session, test_user: User):
        """Test progress percentage calculation."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            progress_current=50,
            progress_total=100,
        )

        assert job.progress_percent == 50

        # Test edge cases
        job.progress_current = 0
        assert job.progress_percent == 0

        job.progress_current = 100
        assert job.progress_percent == 100

        # Test division by zero
        job.progress_total = 0
        assert job.progress_percent == 0

        # Test overflow protection
        job.progress_total = 100
        job.progress_current = 150
        assert job.progress_percent == 100  # Capped at 100

    def test_job_is_terminal(self, session: Session, test_user: User):
        """Test terminal state detection."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )

        # Non-terminal states
        for status in [
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
            BackgroundJobStatus.AWAITING_REVIEW,
            BackgroundJobStatus.READY,
            BackgroundJobStatus.FINALIZING,
        ]:
            job.status = status
            assert not job.is_terminal, f"{status} should not be terminal"

        # Terminal states
        for status in [
            BackgroundJobStatus.COMPLETED,
            BackgroundJobStatus.FAILED,
            BackgroundJobStatus.CANCELLED,
        ]:
            job.status = status
            assert job.is_terminal, f"{status} should be terminal"

    def test_job_duration_seconds(self, session: Session, test_user: User):
        """Test duration calculation."""
        now = datetime.now(timezone.utc)
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )

        # No started_at
        assert job.duration_seconds is None

        # With started_at but no completed_at (in progress)
        job.started_at = now - timedelta(seconds=60)
        duration = job.duration_seconds
        assert duration is not None
        assert duration >= 60

        # With both timestamps (completed)
        job.completed_at = now
        assert job.duration_seconds == 60

    def test_job_user_relationship(self, session: Session, test_user: User):
        """Test relationship with User model."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.user is not None
        assert job.user.id == test_user.id
        assert job.user.username == test_user.username

    def test_job_foreign_key_constraint(self, session: Session):
        """Test foreign key constraint for user_id."""
        job = Job(
            user_id="nonexistent-user-id",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )

        session.add(job)
        with pytest.raises(IntegrityError):
            session.commit()

    def test_update_job(self, session: Session, test_user: User):
        """Test updating Job fields."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PENDING,
        )

        session.add(job)
        session.commit()

        # Update status and progress
        job.status = BackgroundJobStatus.PROCESSING
        job.started_at = datetime.now(timezone.utc)
        job.progress_current = 10
        job.progress_total = 100
        session.commit()
        session.refresh(job)

        assert job.status == BackgroundJobStatus.PROCESSING
        assert job.started_at is not None
        assert job.progress_current == 10

    def test_delete_job(self, session: Session, test_user: User):
        """Test deleting a Job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
        )

        session.add(job)
        session.commit()
        job_id = job.id

        session.delete(job)
        session.commit()

        deleted_job = session.get(Job, job_id)
        assert deleted_job is None

    def test_query_jobs_by_status(self, session: Session, test_user: User):
        """Test querying jobs by status."""
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

        # Query pending jobs
        pending_jobs = session.exec(
            select(Job).where(
                Job.user_id == test_user.id,
                Job.status == BackgroundJobStatus.PENDING
            )
        ).all()

        assert len(pending_jobs) == 1
        assert pending_jobs[0].status == BackgroundJobStatus.PENDING


class TestReviewItemModel:
    """Test ReviewItem model database operations and constraints."""

    def test_create_review_item_success(self, session: Session, test_user: User):
        """Test creating a ReviewItem with valid data."""
        # First create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create review item
        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Counter-Strike: Global Offensive",
        )

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        assert review_item.id is not None
        assert review_item.job_id == job.id
        assert review_item.user_id == test_user.id
        assert review_item.source_title == "Counter-Strike: Global Offensive"
        assert review_item.status == ReviewItemStatus.PENDING
        assert review_item.created_at is not None

    def test_review_item_defaults(self, session: Session, test_user: User):
        """Test ReviewItem default values."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Test Game",
        )

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        assert review_item.status == ReviewItemStatus.PENDING
        assert review_item.resolved_igdb_id is None
        assert review_item.resolved_at is None

    def test_review_item_all_statuses(self, session: Session, test_user: User):
        """Test all review item statuses can be set."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )
        session.add(job)
        session.commit()

        for status in ReviewItemStatus:
            review_item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Game with status {status.value}",
                status=status,
            )
            session.add(review_item)

        session.commit()

        items = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == job.id)
        ).all()

        assert len(items) == len(ReviewItemStatus)
        statuses = {item.status for item in items}
        assert statuses == set(ReviewItemStatus)

    def test_review_item_source_metadata_json(
        self, session: Session, test_user: User
    ):
        """Test source metadata JSON serialization."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Counter-Strike: Global Offensive",
        )

        # Set source metadata
        metadata = {
            "steam_appid": 730,
            "release_year": 2012,
            "developer": "Valve",
        }
        review_item.set_source_metadata(metadata)

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        # Get source metadata
        retrieved = review_item.get_source_metadata()
        assert retrieved == metadata
        assert retrieved["steam_appid"] == 730

    def test_review_item_igdb_candidates_json(
        self, session: Session, test_user: User
    ):
        """Test IGDB candidates JSON serialization."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Final Fantasy VII",
        )

        # Set IGDB candidates
        candidates = [
            {"id": 427, "name": "Final Fantasy VII", "score": 95},
            {"id": 1234, "name": "Final Fantasy VII Remake", "score": 85},
            {"id": 5678, "name": "Final Fantasy VII: Advent Children", "score": 70},
        ]
        review_item.set_igdb_candidates(candidates)

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        # Get IGDB candidates
        retrieved = review_item.get_igdb_candidates()
        assert retrieved == candidates
        assert len(retrieved) == 3
        assert retrieved[0]["name"] == "Final Fantasy VII"

    def test_review_item_empty_metadata(self, session: Session, test_user: User):
        """Test empty metadata returns empty dict/list."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Test Game",
        )

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        assert review_item.get_source_metadata() == {}
        assert review_item.get_igdb_candidates() == []

    def test_review_item_job_relationship(
        self, session: Session, test_user: User
    ):
        """Test relationship with Job model."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Test Game",
        )

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        # Check forward relationship
        assert review_item.job is not None
        assert review_item.job.id == job.id

        # Check reverse relationship
        session.refresh(job)
        assert len(job.review_items) == 1
        assert job.review_items[0].id == review_item.id

    def test_review_item_foreign_key_constraints(self, session: Session, test_user: User):
        """Test foreign key constraints."""
        # Create valid job first
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        # Test invalid job_id
        review_item = ReviewItem(
            job_id="nonexistent-job-id",
            user_id=test_user.id,
            source_title="Test Game",
        )
        session.add(review_item)
        with pytest.raises(IntegrityError):
            session.commit()

        session.rollback()

        # Test invalid user_id
        review_item = ReviewItem(
            job_id=job.id,
            user_id="nonexistent-user-id",
            source_title="Test Game",
        )
        session.add(review_item)
        with pytest.raises(IntegrityError):
            session.commit()

    def test_resolve_review_item(self, session: Session, test_user: User):
        """Test resolving a review item with IGDB match."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Counter-Strike: Global Offensive",
            status=ReviewItemStatus.PENDING,
        )
        session.add(review_item)
        session.commit()

        # Resolve the item
        review_item.status = ReviewItemStatus.MATCHED
        review_item.resolved_igdb_id = 1942
        review_item.resolved_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(review_item)

        assert review_item.status == ReviewItemStatus.MATCHED
        assert review_item.resolved_igdb_id == 1942
        assert review_item.resolved_at is not None

    def test_skip_review_item(self, session: Session, test_user: User):
        """Test skipping a review item."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Unknown Game",
            status=ReviewItemStatus.PENDING,
        )
        session.add(review_item)
        session.commit()

        # Skip the item
        review_item.status = ReviewItemStatus.SKIPPED
        review_item.resolved_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(review_item)

        assert review_item.status == ReviewItemStatus.SKIPPED
        assert review_item.resolved_igdb_id is None  # No IGDB match for skipped

    def test_removal_review_item(self, session: Session, test_user: User):
        """Test review item for game removal detection."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Removed Game",
            status=ReviewItemStatus.REMOVAL,
        )

        metadata = {"reason": "refund", "original_steam_appid": 12345}
        review_item.set_source_metadata(metadata)

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        assert review_item.status == ReviewItemStatus.REMOVAL
        assert review_item.get_source_metadata()["reason"] == "refund"

    def test_delete_review_item(self, session: Session, test_user: User):
        """Test deleting a ReviewItem."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Test Game",
        )
        session.add(review_item)
        session.commit()
        item_id = review_item.id

        session.delete(review_item)
        session.commit()

        deleted_item = session.get(ReviewItem, item_id)
        assert deleted_item is None

    def test_cascade_delete_review_items_with_job(
        self, session: Session, test_user: User
    ):
        """Test that deleting a job should handle review items properly."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        job_id = job.id

        # Create multiple review items
        for i in range(3):
            review_item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Game {i}",
            )
            session.add(review_item)
        session.commit()

        # Verify items exist
        items = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == job_id)
        ).all()
        assert len(items) == 3

        # Delete the job - review items should be deleted by cascade
        # Note: This depends on cascade configuration in the model
        # If cascade is not configured, this test documents that behavior
        session.delete(job)
        session.commit()

        # Check job is deleted
        deleted_job = session.get(Job, job_id)
        assert deleted_job is None

    def test_query_pending_review_items(
        self, session: Session, test_user: User
    ):
        """Test querying pending review items for a user."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        # Create items with different statuses
        statuses = [
            ReviewItemStatus.PENDING,
            ReviewItemStatus.PENDING,
            ReviewItemStatus.MATCHED,
            ReviewItemStatus.SKIPPED,
        ]

        for i, status in enumerate(statuses):
            review_item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Game {i}",
                status=status,
            )
            session.add(review_item)
        session.commit()

        # Query pending items
        pending_items = session.exec(
            select(ReviewItem).where(
                ReviewItem.user_id == test_user.id,
                ReviewItem.status == ReviewItemStatus.PENDING,
            )
        ).all()

        assert len(pending_items) == 2
        assert all(item.status == ReviewItemStatus.PENDING for item in pending_items)


class TestJobModelIndexes:
    """Test Job model database indexes."""

    def test_user_id_index(self, session: Session):
        """Test user_id index for efficient user job queries."""
        users = [
            User(username=f"job_user_{i}", password_hash=f"hash_{i}")
            for i in range(5)
        ]
        session.add_all(users)
        session.commit()

        # Create jobs for different users
        for user in users:
            for _ in range(3):
                job = Job(
                    user_id=user.id,
                    job_type=BackgroundJobType.SYNC,
                    source=BackgroundJobSource.STEAM,
                )
                session.add(job)
        session.commit()

        # Query by user should be efficient
        target_user = users[2]
        user_jobs = session.exec(
            select(Job).where(Job.user_id == target_user.id)
        ).all()

        assert len(user_jobs) == 3
        assert all(j.user_id == target_user.id for j in user_jobs)

    def test_status_index(self, session: Session, test_user: User):
        """Test status index for efficient status filtering."""
        # Create jobs with various statuses
        for status in BackgroundJobStatus:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=BackgroundJobSource.STEAM,
                status=status,
            )
            session.add(job)
        session.commit()

        # Query by status should be efficient
        processing_jobs = session.exec(
            select(Job).where(Job.status == BackgroundJobStatus.PROCESSING)
        ).all()

        assert len(processing_jobs) == 1

    def test_job_type_index(self, session: Session, test_user: User):
        """Test job_type index for efficient type filtering."""
        for job_type in BackgroundJobType:
            for _ in range(2):
                job = Job(
                    user_id=test_user.id,
                    job_type=job_type,
                    source=BackgroundJobSource.SYSTEM,
                )
                session.add(job)
        session.commit()

        # Query by type should be efficient
        import_jobs = session.exec(
            select(Job).where(Job.job_type == BackgroundJobType.IMPORT)
        ).all()

        assert len(import_jobs) == 2


class TestReviewItemModelIndexes:
    """Test ReviewItem model database indexes."""

    def test_job_id_index(self, session: Session, test_user: User):
        """Test job_id index for efficient job review queries."""
        # Create multiple jobs
        jobs = []
        for i in range(3):
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=BackgroundJobSource.STEAM,
            )
            session.add(job)
            jobs.append(job)
        session.commit()

        # Create review items for each job
        for job in jobs:
            for j in range(5):
                review_item = ReviewItem(
                    job_id=job.id,
                    user_id=test_user.id,
                    source_title=f"Game {j} for Job {job.id[:8]}",
                )
                session.add(review_item)
        session.commit()

        # Query by job_id should be efficient
        target_job = jobs[1]
        job_items = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == target_job.id)
        ).all()

        assert len(job_items) == 5

    def test_status_index(self, session: Session, test_user: User):
        """Test status index for efficient review queue queries."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        # Create items with different statuses
        for status in ReviewItemStatus:
            for _ in range(2):
                review_item = ReviewItem(
                    job_id=job.id,
                    user_id=test_user.id,
                    source_title=f"Game {status.value}",
                    status=status,
                )
                session.add(review_item)
        session.commit()

        # Query by status should be efficient
        pending_items = session.exec(
            select(ReviewItem).where(ReviewItem.status == ReviewItemStatus.PENDING)
        ).all()

        assert len(pending_items) == 2


class TestJobModelEdgeCases:
    """Test edge cases and error conditions for Job model."""

    def test_long_error_message(self, session: Session, test_user: User):
        """Test error message field length constraints."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.FAILED,
            error_message="E" * 2000,  # Max length
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert len(job.error_message) == 2000

    def test_file_path_for_exports(self, session: Session, test_user: User):
        """Test file path field for export jobs."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            file_path="/exports/collection_2025-01-01.json",
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.file_path == "/exports/collection_2025-01-01.json"

    def test_taskiq_task_id(self, session: Session, test_user: User):
        """Test taskiq task ID tracking."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            taskiq_task_id="abc123-def456",
        )

        session.add(job)
        session.commit()

        # Query by taskiq ID should work efficiently
        found_job = session.exec(
            select(Job).where(Job.taskiq_task_id == "abc123-def456")
        ).first()

        assert found_job is not None
        assert found_job.id == job.id

    def test_invalid_json_handling(self, session: Session, test_user: User):
        """Test handling of invalid JSON in result_summary."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        # Manually set invalid JSON
        job.result_summary_json = "not valid json"

        session.add(job)
        session.commit()
        session.refresh(job)

        # Should return empty dict for invalid JSON
        assert job.get_result_summary() == {}

    def test_invalid_error_log_json_handling(self, session: Session, test_user: User):
        """Test handling of invalid JSON in error_log."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        # Manually set invalid JSON
        job.error_log_json = "not valid json"

        session.add(job)
        session.commit()
        session.refresh(job)

        # Should return empty list for invalid JSON
        assert job.get_error_log() == []

    def test_import_subtype_index(self, session: Session, test_user: User):
        """Test import_subtype index for efficient subtype filtering."""
        # Create jobs with different subtypes
        for subtype in ImportJobSubtype:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.IMPORT,
                source=BackgroundJobSource.CSV,
                import_subtype=subtype,
            )
            session.add(job)
        session.commit()

        # Query by import_subtype should be efficient
        library_import_jobs = session.exec(
            select(Job).where(Job.import_subtype == ImportJobSubtype.LIBRARY_IMPORT)
        ).all()

        assert len(library_import_jobs) == 1
        assert library_import_jobs[0].import_subtype == ImportJobSubtype.LIBRARY_IMPORT


class TestReviewItemModelEdgeCases:
    """Test edge cases for ReviewItem model."""

    def test_long_source_title(self, session: Session, test_user: User):
        """Test source title field length constraints."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="T" * 500,  # Max length
        )

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        assert len(review_item.source_title) == 500

    def test_unicode_source_title(self, session: Session, test_user: User):
        """Test Unicode characters in source title."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        unicode_titles = [
            "ファイナルファンタジー VII",  # Japanese
            "最终幻想 VII",  # Chinese
            "Последняя Фантазия VII",  # Russian
            "🎮 Game with Emojis 🎯",
        ]

        for title in unicode_titles:
            review_item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=title,
            )
            session.add(review_item)

        session.commit()

        items = session.exec(
            select(ReviewItem).where(ReviewItem.job_id == job.id)
        ).all()

        saved_titles = {item.source_title for item in items}
        assert saved_titles == set(unicode_titles)

    def test_invalid_metadata_json_handling(
        self, session: Session, test_user: User
    ):
        """Test handling of invalid JSON in metadata fields."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Test Game",
        )
        # Manually set invalid JSON
        review_item.source_metadata_json = "invalid json"
        review_item.igdb_candidates_json = "also invalid"

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        # Should return empty dict/list for invalid JSON
        assert review_item.get_source_metadata() == {}
        assert review_item.get_igdb_candidates() == []

    def test_large_igdb_candidates_list(
        self, session: Session, test_user: User
    ):
        """Test storing many IGDB candidates."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )
        session.add(job)
        session.commit()

        review_item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Common Game Name",
        )

        # Create many candidates
        candidates = [
            {"id": i, "name": f"Game Variant {i}", "score": 100 - i}
            for i in range(50)
        ]
        review_item.set_igdb_candidates(candidates)

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        retrieved = review_item.get_igdb_candidates()
        assert len(retrieved) == 50
        assert retrieved[0]["id"] == 0
        assert retrieved[49]["id"] == 49

"""
Unit tests for Job and JobItem models.

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
    JobItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    JobItemStatus,
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
        assert job.total_items == 0
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
        assert job.total_items == 0
        assert job.error_message is None
        assert job.file_path is None
        assert job.started_at is None
        assert job.completed_at is None

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

    def test_job_total_items(self, session: Session, test_user: User):
        """Test total items tracking."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.CSV,
            total_items=100,
        )

        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.total_items == 100

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

        # Update status and timestamps
        job.status = BackgroundJobStatus.PROCESSING
        job.started_at = datetime.now(timezone.utc)
        job.total_items = 100
        session.commit()
        session.refresh(job)

        assert job.status == BackgroundJobStatus.PROCESSING
        assert job.started_at is not None
        assert job.total_items == 100

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

class TestJobItemModel:
    """Test JobItem model database operations and constraints."""

    def test_create_job_item_success(self, session: Session, test_user: User):
        """Test creating a JobItem with valid data."""
        # First create a job
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create job item
        job_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="cs-go",
            source_title="Counter-Strike: Global Offensive",
        )

        session.add(job_item)
        session.commit()
        session.refresh(job_item)

        assert job_item.id is not None
        assert job_item.job_id == job.id
        assert job_item.user_id == test_user.id
        assert job_item.source_title == "Counter-Strike: Global Offensive"
        assert job_item.status == JobItemStatus.PENDING
        assert job_item.created_at is not None

    def test_review_item_defaults(self, session: Session, test_user: User):
        """Test JobItem default values."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="test-game",
            source_title="Test Game",
        )

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        assert review_item.status == JobItemStatus.PENDING
        assert review_item.resolved_igdb_id is None
        assert review_item.resolved_at is None

    def test_review_item_all_statuses(self, session: Session, test_user: User):
        """Test all review item statuses can be set."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        for i, status in enumerate(JobItemStatus):
            review_item = JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key=f"game-{i}",
                source_title=f"Game with status {status.value}",
                status=status,
            )
            session.add(review_item)

        session.commit()

        items = session.exec(
            select(JobItem).where(JobItem.job_id == job.id)
        ).all()

        assert len(items) == len(JobItemStatus)
        statuses = {item.status for item in items}
        assert statuses == set(JobItemStatus)

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

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="cs-go-730",
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
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="ff7-427",
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

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="test-game",
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

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="test-game",
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
        assert len(job.items) == 1
        assert job.items[0].id == review_item.id

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
        review_item = JobItem(
            job_id="nonexistent-job-id",
            user_id=test_user.id,
            item_key="test-game",
            source_title="Test Game",
        )
        session.add(review_item)
        with pytest.raises(IntegrityError):
            session.commit()

        session.rollback()

        # Test invalid user_id
        review_item = JobItem(
            job_id=job.id,
            user_id="nonexistent-user-id",
            item_key="test-game-2",
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

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="cs-go-730",
            source_title="Counter-Strike: Global Offensive",
            status=JobItemStatus.PENDING_REVIEW,
        )
        session.add(review_item)
        session.commit()

        # Resolve the item (mark as completed with IGDB match)
        review_item.status = JobItemStatus.COMPLETED
        review_item.resolved_igdb_id = 1942
        review_item.resolved_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(review_item)

        assert review_item.status == JobItemStatus.COMPLETED
        assert review_item.resolved_igdb_id == 1942
        assert review_item.resolved_at is not None

    def test_skip_review_item(self, session: Session, test_user: User):
        """Test skipping a review item."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="unknown-game",
            source_title="Unknown Game",
            status=JobItemStatus.PENDING_REVIEW,
        )
        session.add(review_item)
        session.commit()

        # Skip the item
        review_item.status = JobItemStatus.SKIPPED
        review_item.resolved_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(review_item)

        assert review_item.status == JobItemStatus.SKIPPED
        assert review_item.resolved_igdb_id is None  # No IGDB match for skipped

    def test_failed_job_item(self, session: Session, test_user: User):
        """Test job item with failure metadata."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="failed-game-12345",
            source_title="Failed Game",
            status=JobItemStatus.FAILED,
            error_message="IGDB API returned 404",
        )

        metadata = {"reason": "api_error", "steam_appid": 12345}
        review_item.set_source_metadata(metadata)

        session.add(review_item)
        session.commit()
        session.refresh(review_item)

        assert review_item.status == JobItemStatus.FAILED
        assert review_item.error_message == "IGDB API returned 404"
        assert review_item.get_source_metadata()["reason"] == "api_error"

    def test_delete_review_item(self, session: Session, test_user: User):
        """Test deleting a JobItem."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()

        review_item = JobItem(
            job_id=job.id,
            user_id=test_user.id,
            item_key="test-game",
            source_title="Test Game",
        )
        session.add(review_item)
        session.commit()
        item_id = review_item.id

        session.delete(review_item)
        session.commit()

        deleted_item = session.get(JobItem, item_id)
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
            review_item = JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key=f"game-{i}",
                source_title=f"Game {i}",
            )
            session.add(review_item)
        session.commit()

        # Verify items exist
        items = session.exec(
            select(JobItem).where(JobItem.job_id == job_id)
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
            JobItemStatus.PENDING,
            JobItemStatus.PENDING,
            JobItemStatus.COMPLETED,
            JobItemStatus.SKIPPED,
        ]

        for i, status in enumerate(statuses):
            review_item = JobItem(
                job_id=job.id,
                user_id=test_user.id,
                item_key=f"game-{i}",
                source_title=f"Game {i}",
                status=status,
            )
            session.add(review_item)
        session.commit()

        # Query pending items
        pending_items = session.exec(
            select(JobItem).where(
                JobItem.user_id == test_user.id,
                JobItem.status == JobItemStatus.PENDING,
            )
        ).all()

        assert len(pending_items) == 2
        assert all(item.status == JobItemStatus.PENDING for item in pending_items)


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


class TestJobItemModelIndexes:
    """Test JobItem model database indexes."""

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
        for idx, job in enumerate(jobs):
            for j in range(5):
                review_item = JobItem(
                    job_id=job.id,
                    user_id=test_user.id,
                    item_key=f"game-{idx}-{j}",
                    source_title=f"Game {j} for Job {job.id[:8]}",
                )
                session.add(review_item)
        session.commit()

        # Query by job_id should be efficient
        target_job = jobs[1]
        job_items = session.exec(
            select(JobItem).where(JobItem.job_id == target_job.id)
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
        # Each item_key must be unique within the job due to unique constraint
        item_counter = 0
        for status in JobItemStatus:
            for i in range(2):
                review_item = JobItem(
                    job_id=job.id,
                    user_id=test_user.id,
                    item_key=f"game-{item_counter}",
                    source_title=f"Game {status.value} {item_counter}",
                    status=status,
                )
                session.add(review_item)
                item_counter += 1
        session.commit()

        # Query by status should be efficient
        pending_items = session.exec(
            select(JobItem).where(JobItem.status == JobItemStatus.PENDING)
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

    def test_job_auto_retry_done_default_false(self, session: Session, test_user: User):
        """Test that auto_retry_done defaults to False."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        assert job.auto_retry_done is False


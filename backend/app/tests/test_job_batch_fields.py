"""Tests for Job model batch session fields."""

from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ImportJobSubtype,
)


def test_job_has_batch_session_fields():
    """Job model should have fields for batch session tracking."""
    job = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        import_subtype=ImportJobSubtype.AUTO_MATCH,
    )

    # New batch session fields should exist
    assert hasattr(job, "processed_item_ids_json")
    assert hasattr(job, "failed_item_ids_json")

    # Helper methods should work
    assert job.get_processed_item_ids() == []
    assert job.get_failed_item_ids() == []

    job.add_processed_item_id("game-1")
    job.add_processed_item_id("game-2")
    assert job.get_processed_item_ids() == ["game-1", "game-2"]

    job.add_failed_item_id("game-3")
    assert job.get_failed_item_ids() == ["game-3"]


def test_job_progress_percentage():
    """Job should calculate progress percentage correctly."""
    job = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        progress_total=100,
        progress_current=50,
    )

    assert job.progress_percent == 50


def test_job_remaining_items():
    """Job should calculate remaining items."""
    job = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        progress_total=100,
        progress_current=30,
    )

    assert job.remaining_items == 70


def test_job_is_active():
    """Job should report active status correctly."""
    job = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        status=BackgroundJobStatus.PROCESSING,
    )

    assert job.is_active is True

    job.status = BackgroundJobStatus.COMPLETED
    assert job.is_active is False

"""Tests for maintenance tasks."""

import pytest


@pytest.mark.asyncio
async def test_cleanup_stale_batch_jobs_exists():
    """Should be able to import cleanup_stale_batch_jobs task."""
    from app.worker.tasks.maintenance import cleanup_stale_batch_jobs

    # Verify the task exists and is callable
    assert cleanup_stale_batch_jobs is not None
    assert callable(cleanup_stale_batch_jobs)

"""Tests for worker advisory lock utilities."""

import pytest
from sqlmodel import Session, text

from app.worker.locking import job_id_to_lock_key, acquire_job_lock, release_job_lock


class TestJobIdToLockKey:
    """Test lock key generation from job IDs."""

    def test_returns_positive_integer(self):
        """Lock key is always a positive integer."""
        key = job_id_to_lock_key("test-job-123")
        assert isinstance(key, int)
        assert key >= 0

    def test_same_job_id_same_key(self):
        """Same job ID produces same lock key."""
        key1 = job_id_to_lock_key("job-abc-123")
        key2 = job_id_to_lock_key("job-abc-123")
        assert key1 == key2

    def test_different_job_ids_different_keys(self):
        """Different job IDs produce different lock keys."""
        key1 = job_id_to_lock_key("job-abc-123")
        key2 = job_id_to_lock_key("job-xyz-456")
        assert key1 != key2

    def test_fits_in_bigint(self):
        """Lock key fits in PostgreSQL bigint range."""
        key = job_id_to_lock_key("test-job-with-long-uuid-identifier")
        # PostgreSQL bigint max is 2^63-1
        assert key <= 0x7FFFFFFFFFFFFFFF


class TestAcquireJobLock:
    """Test advisory lock acquisition."""

    def test_acquire_lock_succeeds_when_available(self, session: Session):
        """Lock acquisition returns True when lock is available."""
        result = acquire_job_lock(session, "test-job-001")
        assert result is True
        # Clean up
        release_job_lock(session, "test-job-001")

    def test_acquire_lock_fails_when_held(self, session: Session):
        """Lock acquisition returns False when another session holds the lock."""
        from app.core.database import get_engine

        # First session acquires the lock
        result1 = acquire_job_lock(session, "test-job-002")
        assert result1 is True

        # Second session tries to acquire same lock
        with Session(get_engine()) as session2:
            result2 = acquire_job_lock(session2, "test-job-002")
            assert result2 is False

        # Clean up
        release_job_lock(session, "test-job-002")

    def test_different_jobs_can_lock_simultaneously(self, session: Session):
        """Different jobs can be locked by different sessions."""
        from app.core.database import get_engine

        # First session locks job A
        result1 = acquire_job_lock(session, "job-A")
        assert result1 is True

        # Second session locks job B (should succeed)
        with Session(get_engine()) as session2:
            result2 = acquire_job_lock(session2, "job-B")
            assert result2 is True
            release_job_lock(session2, "job-B")

        # Clean up
        release_job_lock(session, "job-A")


class TestReleaseJobLock:
    """Test advisory lock release."""

    def test_release_allows_reacquisition(self, session: Session):
        """After release, another session can acquire the lock."""
        from app.core.database import get_engine

        # Acquire and release
        acquire_job_lock(session, "test-job-003")
        release_job_lock(session, "test-job-003")

        # Another session can now acquire
        with Session(get_engine()) as session2:
            result = acquire_job_lock(session2, "test-job-003")
            assert result is True
            release_job_lock(session2, "test-job-003")

    def test_release_nonexistent_lock_is_safe(self, session: Session):
        """Releasing a lock that wasn't acquired doesn't raise."""
        # Should not raise
        release_job_lock(session, "never-acquired-job")

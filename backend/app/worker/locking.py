"""Advisory lock utilities for preventing duplicate task execution.

PostgreSQL advisory locks are used to ensure only one worker processes
a given job, even when the taskiq-pg broker broadcasts to all workers.
"""

import logging
from sqlmodel import Session, text

logger = logging.getLogger(__name__)


def job_id_to_lock_key(job_id: str) -> int:
    """Convert a job ID string to a PostgreSQL advisory lock key.

    Advisory locks use bigint keys. We use a deterministic hash (MD5)
    to ensure the same job_id produces the same lock key across all
    worker processes, regardless of Python's hash randomization.

    Args:
        job_id: The job ID string (typically a UUID)

    Returns:
        A positive integer suitable for pg_advisory_lock
    """
    import hashlib
    # Use MD5 for deterministic hashing (not for security, just consistency)
    # Take first 8 bytes and convert to int, mask to positive bigint range
    hash_bytes = hashlib.md5(job_id.encode()).digest()[:8]
    return int.from_bytes(hash_bytes, byteorder='big') & 0x7FFFFFFFFFFFFFFF


def acquire_job_lock(session: Session, job_id: str) -> bool:
    """Attempt to acquire an advisory lock for a job.

    Uses pg_try_advisory_lock which returns immediately (non-blocking).
    The lock is held until explicitly released or the session ends.

    Args:
        session: Database session
        job_id: The job ID to lock

    Returns:
        True if lock acquired, False if another session holds it
    """
    lock_key = job_id_to_lock_key(job_id)
    result = session.execute(text(f"SELECT pg_try_advisory_lock({lock_key})"))  # pyrefly: ignore[deprecated]
    acquired = result.scalar() is True
    logger.info(f"Advisory lock attempt for job {job_id} (key={lock_key}): {'acquired' if acquired else 'BLOCKED'}")
    return acquired


def release_job_lock(session: Session, job_id: str) -> None:
    """Release an advisory lock for a job.

    Safe to call even if the lock wasn't acquired (returns False but no error).

    Args:
        session: Database session
        job_id: The job ID to unlock
    """
    lock_key = job_id_to_lock_key(job_id)
    session.execute(text(f"SELECT pg_advisory_unlock({lock_key})"))  # pyrefly: ignore[deprecated]

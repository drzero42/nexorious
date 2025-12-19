"""Advisory lock utilities for preventing duplicate task execution.

PostgreSQL advisory locks are used to ensure only one worker processes
a given job, even when the taskiq-pg broker broadcasts to all workers.
"""

from sqlmodel import Session, text


def job_id_to_lock_key(job_id: str) -> int:
    """Convert a job ID string to a PostgreSQL advisory lock key.

    Advisory locks use bigint keys. We hash the job_id and mask to ensure
    it fits in the positive bigint range (0 to 2^63-1).

    Args:
        job_id: The job ID string (typically a UUID)

    Returns:
        A positive integer suitable for pg_advisory_lock
    """
    return hash(job_id) & 0x7FFFFFFFFFFFFFFF


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
    result = session.exec(text(f"SELECT pg_try_advisory_lock({lock_key})"))
    return result.scalar() is True


def release_job_lock(session: Session, job_id: str) -> None:
    """Release an advisory lock for a job.

    Safe to call even if the lock wasn't acquired (returns False but no error).

    Args:
        session: Database session
        job_id: The job ID to unlock
    """
    lock_key = job_id_to_lock_key(job_id)
    session.exec(text(f"SELECT pg_advisory_unlock({lock_key})"))

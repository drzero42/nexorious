"""Service for job operations with derived status."""

from sqlmodel import Session, select, func
from sqlalchemy import update as sa_update

from app.models.job import Job, JobItem, JobItemStatus, BackgroundJobStatus
from app.schemas.job import JobProgress


def get_job_progress(session: Session, job_id: str) -> JobProgress:
    """Get progress counts for a job from its items."""
    counts = session.exec(
        select(JobItem.status, func.count())
        .where(JobItem.job_id == job_id)
        .group_by(JobItem.status)
    ).all()

    status_counts = {status: count for status, count in counts}

    return JobProgress(
        pending=status_counts.get(JobItemStatus.PENDING, 0),
        processing=status_counts.get(JobItemStatus.PROCESSING, 0),
        completed=status_counts.get(JobItemStatus.COMPLETED, 0),
        pending_review=status_counts.get(JobItemStatus.PENDING_REVIEW, 0),
        skipped=status_counts.get(JobItemStatus.SKIPPED, 0),
        failed=status_counts.get(JobItemStatus.FAILED, 0),
    )


def get_derived_job_status(session: Session, job: Job) -> BackgroundJobStatus:
    """Derive job status from its items.

    For jobs that don't use items (like exports), falls back to the job's
    stored status.
    """
    # Explicit terminal statuses take precedence
    if job.status in (
        BackgroundJobStatus.FAILED,
        BackgroundJobStatus.CANCELLED,
        BackgroundJobStatus.COMPLETED,
    ):
        return job.status

    progress = get_job_progress(session, job.id)

    # Jobs without items (e.g., exports) use their stored status
    if progress.total == 0:
        return job.status

    if progress.processing > 0 or progress.pending > 0:
        return BackgroundJobStatus.PROCESSING

    return BackgroundJobStatus.COMPLETED


def retry_failed_items(session: Session, job_id: str) -> int:
    """Reset all failed items in a job to pending status.

    Args:
        session: Database session
        job_id: The job ID to retry failed items for

    Returns:
        Number of items reset
    """
    result = session.execute(
        sa_update(JobItem)
        .where(JobItem.job_id == job_id)
        .where(JobItem.status == JobItemStatus.FAILED)
        .values(
            status=JobItemStatus.PENDING,
            error_message=None,
            processed_at=None,
        )
    )
    session.commit()
    return result.rowcount


def retry_job_item(session: Session, item_id: str) -> bool:
    """Reset a single failed item to pending status.

    Args:
        session: Database session
        item_id: The job item ID to retry

    Returns:
        True if item was reset, False if not found or not in FAILED status
    """
    item = session.get(JobItem, item_id)
    if not item or item.status != JobItemStatus.FAILED:
        return False

    item.status = JobItemStatus.PENDING
    item.error_message = None
    item.processed_at = None
    session.add(item)
    session.commit()
    return True

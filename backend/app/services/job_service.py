"""Service for job operations with derived status."""

from sqlmodel import Session, select, func

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
    """Derive job status from its items."""
    # Explicit statuses take precedence
    if job.status in (BackgroundJobStatus.FAILED, BackgroundJobStatus.CANCELLED):
        return job.status

    progress = get_job_progress(session, job.id)

    if progress.total == 0:
        return BackgroundJobStatus.PENDING

    if progress.processing > 0 or progress.pending > 0:
        return BackgroundJobStatus.PROCESSING

    return BackgroundJobStatus.COMPLETED

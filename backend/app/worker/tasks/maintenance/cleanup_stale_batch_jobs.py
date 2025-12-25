"""Task to clean up stale batch jobs.

Runs every 30 minutes to mark batch jobs that have been stuck in PROCESSING
state for too long as FAILED. This replaces the in-memory cleanup loop that
was previously part of BatchSessionManager.
"""

import logging
from datetime import datetime, timezone, timedelta
from typing import Dict, Any

from sqlmodel import select

from app.worker.broker import broker
from app.worker.queues import SUBJECT_LOW_MAINTENANCE
from app.core.database import get_session_context
from app.models.job import Job, BackgroundJobStatus, BackgroundJobType, ImportJobSubtype

logger = logging.getLogger(__name__)


@broker.task(
    task_name=SUBJECT_LOW_MAINTENANCE,
    schedule=[{"cron": "*/30 * * * *"}],  # Every 30 minutes
)
async def cleanup_stale_batch_jobs(timeout_minutes: int = 30) -> Dict[str, Any]:
    """
    Mark stale batch jobs as failed.

    Batch jobs (auto_match or bulk_sync) that have been in PROCESSING status
    for longer than timeout_minutes without updates are considered stale
    and marked as FAILED.

    Args:
        timeout_minutes: Number of minutes after which a processing job is
            considered stale. Defaults to 30 minutes.

    Returns:
        Dictionary with cleanup statistics.
    """
    logger.info(f"Starting cleanup of stale batch jobs (timeout: {timeout_minutes} minutes)")

    cutoff_time = datetime.now(timezone.utc) - timedelta(minutes=timeout_minutes)
    cleaned_up = 0

    try:
        async with get_session_context() as session:
            # Find stale batch jobs (auto_match or bulk_sync that are still processing)
            stale_jobs = session.exec(
                select(Job).where(
                    Job.job_type == BackgroundJobType.IMPORT,
                    Job.import_subtype.in_([  # pyrefly: ignore[missing-attribute]
                        ImportJobSubtype.AUTO_MATCH,
                        ImportJobSubtype.BULK_SYNC,
                    ]),
                    Job.status == BackgroundJobStatus.PROCESSING,
                    Job.started_at < cutoff_time,  # pyrefly: ignore[unsupported-operation]
                )
            ).all()

            for job in stale_jobs:
                job.status = BackgroundJobStatus.FAILED
                job.error_message = f"Job timed out after {timeout_minutes} minutes of inactivity"
                job.completed_at = datetime.now(timezone.utc)
                logger.warning(
                    f"Marked stale batch job {job.id} as failed "
                    f"(user: {job.user_id}, subtype: {job.import_subtype})"
                )
                cleaned_up += 1

            session.commit()

        if cleaned_up > 0:
            logger.info(f"Cleaned up {cleaned_up} stale batch jobs")
        else:
            logger.debug("No stale batch jobs to cleanup")

        return {
            "status": "success",
            "cleaned_up": cleaned_up,
            "timeout_minutes": timeout_minutes,
            "cutoff_time": cutoff_time.isoformat(),
        }

    except Exception as e:
        logger.error(f"Failed to cleanup stale batch jobs: {e}")
        return {
            "status": "error",
            "error": str(e),
            "cleaned_up": cleaned_up,
        }

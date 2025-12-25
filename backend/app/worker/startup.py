"""Worker startup handlers.

Handles recovery of orphaned jobs when workers restart.
"""

import asyncio
import logging

from sqlmodel import select
from taskiq import TaskiqEvents, TaskiqState

from app.worker.broker import broker
from app.core.database import get_session_context
from app.models.job import Job, BackgroundJobStatus, BackgroundJobSource

logger = logging.getLogger(__name__)

# Track if recovery has been scheduled to avoid duplicates
_recovery_scheduled = False


async def _recover_orphaned_jobs() -> None:
    """Find and re-enqueue orphaned child jobs.

    This runs as a deferred task after broker startup is complete,
    allowing time for the broker to be fully initialized.
    """
    # Wait a bit for the broker to be fully ready
    await asyncio.sleep(2)

    logger.info("Checking for orphaned jobs to recover...")

    async with get_session_context() as session:
        # Find child jobs that are stuck in non-terminal states
        # These are jobs with a parent_job_id that are PENDING or PROCESSING
        orphaned_jobs = session.exec(
            select(Job).where(
                Job.parent_job_id.isnot(None),  # type: ignore[union-attr]
                Job.source == BackgroundJobSource.NEXORIOUS,
                Job.status.in_([  # type: ignore[union-attr]
                    BackgroundJobStatus.PENDING,
                    BackgroundJobStatus.PROCESSING,
                ]),
            )
        ).all()

        if not orphaned_jobs:
            logger.info("No orphaned jobs found")
            return

        logger.info(f"Found {len(orphaned_jobs)} orphaned jobs to recover")

        # Import task here to avoid circular imports
        from app.worker.tasks.import_export.import_nexorious_item import (
            import_nexorious_item,
        )

        recovered_count = 0
        for job in orphaned_jobs:
            # Reset PROCESSING jobs back to PENDING before re-enqueue
            if job.status == BackgroundJobStatus.PROCESSING:
                job.status = BackgroundJobStatus.PENDING
                job.started_at = None
                session.add(job)

            try:
                # Re-enqueue the task
                await import_nexorious_item.kiq(job.id)
                recovered_count += 1
                logger.debug(f"Re-enqueued orphaned job {job.id}")
            except Exception as e:
                logger.error(f"Failed to re-enqueue job {job.id}: {e}")

        # Commit status changes
        session.commit()

        logger.info(f"Successfully recovered {recovered_count} orphaned jobs")


@broker.on_event(TaskiqEvents.WORKER_STARTUP)
async def schedule_orphan_recovery(state: TaskiqState) -> None:
    """Schedule orphaned job recovery after broker startup.

    Uses asyncio.create_task to defer the actual recovery work until after
    the broker is fully initialized and can accept task submissions.
    """
    global _recovery_scheduled
    if _recovery_scheduled:
        return
    _recovery_scheduled = True

    logger.info("Scheduling orphaned job recovery...")
    asyncio.create_task(_recover_orphaned_jobs())

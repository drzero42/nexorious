"""Nexorious JSON import coordinator task.

Parses the JSON export and fans out individual game/wishlist imports
to child tasks for parallel processing.
"""

import logging
from datetime import datetime, timezone
from typing import Dict, Any

from sqlmodel import Session

from app.worker.locking import acquire_job_lock, release_job_lock
from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH
from app.core.database import get_session_context
from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    ImportJobSubtype,
)

logger = logging.getLogger(__name__)


@broker.task(
    task_name="import.nexorious_coordinator",
    queue=QUEUE_HIGH,
)
async def import_nexorious_coordinator(job_id: str) -> Dict[str, Any]:
    """
    Coordinate Nexorious JSON import by creating child jobs.

    This task:
    1. Parses the JSON data from the parent job
    2. Creates a child Job for each game and wishlist item
    3. Enqueues the item processing task for each child
    4. Clears the raw import data from the parent
    5. Exits immediately (does not wait for children)
    """
    logger.info(f"Starting Nexorious import coordinator (job: {job_id})")

    async with get_session_context() as session:
        # Try to acquire advisory lock - prevents duplicate execution
        if not acquire_job_lock(session, job_id):
            logger.info(f"Job {job_id} already being processed by another worker")
            return {"status": "skipped", "reason": "duplicate_execution"}

        # Get job first (outside try so exception handler can access it)
        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            release_job_lock(session, job_id)
            return {"status": "error", "error": "Job not found"}

        try:
            # Update job status
            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            # Get import data
            result_summary = job.get_result_summary()
            import_data = result_summary.get("_import_data", {})

            if not import_data:
                raise ValueError("No import data found in job")

            games = import_data.get("games", [])
            wishlist = import_data.get("wishlist", [])

            children_created = 0

            # Import the child task here to avoid circular import
            from app.worker.tasks.import_export.import_nexorious_item import (
                import_nexorious_item,
            )

            # Create child jobs for games
            for game_data in games:
                child = _create_child_job(
                    session=session,
                    parent_job=job,
                    item_data=game_data,
                    subtype=ImportJobSubtype.LIBRARY_IMPORT,
                )
                children_created += 1

                # Enqueue child task
                await import_nexorious_item.kiq(child.id)

            # Create child jobs for wishlist
            for wishlist_data in wishlist:
                child = _create_child_job(
                    session=session,
                    parent_job=job,
                    item_data=wishlist_data,
                    subtype=ImportJobSubtype.WISHLIST_IMPORT,
                )
                children_created += 1

                # Enqueue child task
                await import_nexorious_item.kiq(child.id)

            # Clear import data and update parent
            result_summary.pop("_import_data", None)
            result_summary["children_created"] = children_created
            result_summary["games_count"] = len(games)
            result_summary["wishlist_count"] = len(wishlist)
            job.set_result_summary(result_summary)

            session.add(job)
            session.commit()

            logger.info(
                f"Coordinator completed for job {job_id}: "
                f"created {children_created} child jobs"
            )

            return {
                "status": "success",
                "children_created": children_created,
                "games_count": len(games),
                "wishlist_count": len(wishlist),
            }

        except Exception as e:
            logger.error(f"Coordinator failed for job {job_id}: {e}")
            session.rollback()
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e)}
        finally:
            release_job_lock(session, job_id)


def _create_child_job(
    session: Session,
    parent_job: Job,
    item_data: Dict[str, Any],
    subtype: ImportJobSubtype,
) -> Job:
    """Create a child job for a single game or wishlist item."""
    child = Job(
        user_id=parent_job.user_id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        import_subtype=subtype,
        status=BackgroundJobStatus.PENDING,
        priority=BackgroundJobPriority.HIGH,
        parent_job_id=parent_job.id,
    )

    # Store item data for processing
    child.set_result_summary({
        "_item_data": item_data,
        "title": item_data.get("title"),
        "igdb_id": item_data.get("igdb_id"),
    })

    session.add(child)
    session.commit()
    session.refresh(child)

    return child

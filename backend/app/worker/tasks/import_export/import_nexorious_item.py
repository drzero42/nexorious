"""Nexorious item import task.

Processes a single game or wishlist item from a Nexorious export.
Called by the coordinator as part of fan-out processing.
"""

import logging
from datetime import datetime, timezone
from typing import Dict, Any

from sqlmodel import Session, select

from app.worker.locking import acquire_job_lock, release_job_lock
from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH
from app.core.database import get_session_context
from app.models.job import Job, BackgroundJobStatus, ImportJobSubtype
from app.services.igdb import IGDBService
from app.services.game_service import GameService
from app.worker.tasks.import_export.import_nexorious_helpers import (
    _process_nexorious_game,
    _process_wishlist_item,
)

logger = logging.getLogger(__name__)


@broker.task(
    task_name="import.nexorious_item",
    queue=QUEUE_HIGH,
)
async def import_nexorious_item(job_id: str) -> Dict[str, Any]:
    """
    Process a single Nexorious import item (game or wishlist).

    This task is called by the coordinator for each item in the import.
    It processes the item data and updates the child job status.
    When all siblings are complete, it finalizes the parent job.

    Args:
        job_id: The child Job ID for this item

    Returns:
        Dictionary with processing result.
    """
    logger.info(f"Starting Nexorious item import (job: {job_id})")

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

            # Get item data from job
            result_summary = job.get_result_summary()
            item_data = result_summary.get("_item_data")

            if not item_data:
                raise ValueError("No item data found in job")

            # Create services
            igdb_service = IGDBService()
            game_service = GameService(session, igdb_service)

            # Process based on import_subtype
            result = None
            if job.import_subtype == ImportJobSubtype.LIBRARY_IMPORT:
                result = await _process_nexorious_game(
                    session=session,
                    game_service=game_service,
                    user_id=job.user_id,
                    game_data=item_data,
                )
            elif job.import_subtype == ImportJobSubtype.WISHLIST_IMPORT:
                result = await _process_wishlist_item(
                    session=session,
                    game_service=game_service,
                    user_id=job.user_id,
                    wishlist_data=item_data,
                )
            else:
                raise ValueError(f"Unknown import_subtype: {job.import_subtype}")

            # Clear item data from result summary to save space
            result_summary.pop("_item_data", None)
            job.set_result_summary(result_summary)

            # Mark job as completed
            job.status = BackgroundJobStatus.COMPLETED
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            logger.info(f"Nexorious item import completed for job {job_id}: {result}")

            # Check if all siblings are complete and finalize parent
            if job.parent_job_id:
                await _check_and_finalize_parent(session, job.parent_job_id)

            return {"status": "success", "result": result}

        except Exception as e:
            logger.error(f"Nexorious item import failed for job {job_id}: {e}", exc_info=True)
            # Rollback any pending transaction before updating job status
            session.rollback()
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e)}
        finally:
            release_job_lock(session, job_id)


async def _check_and_finalize_parent(session: Session, parent_job_id: str) -> None:
    """
    Check if all child jobs are complete and finalize the parent if they are.

    This function is called after each child job completes to check if it's
    the last one. If so, it marks the parent as COMPLETED.

    Args:
        session: Database session
        parent_job_id: ID of the parent job to check
    """
    # Get all children of the parent
    children = session.exec(
        select(Job).where(Job.parent_job_id == parent_job_id)
    ).all()

    # Check if all children are in terminal states
    terminal_states = {
        BackgroundJobStatus.COMPLETED,
        BackgroundJobStatus.FAILED,
        BackgroundJobStatus.CANCELLED,
    }

    all_complete = all(child.status in terminal_states for child in children)

    if all_complete:
        logger.info(f"All children complete, finalizing parent job {parent_job_id}")
        parent = session.get(Job, parent_job_id)
        if parent:
            parent.status = BackgroundJobStatus.COMPLETED
            parent.completed_at = datetime.now(timezone.utc)
            session.add(parent)
            session.commit()
            logger.info(f"Parent job {parent_job_id} finalized")
    else:
        logger.debug(
            f"Not finalizing parent {parent_job_id} - some children still in progress"
        )

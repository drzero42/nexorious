"""Generic import item processor for Nexorious JSON imports.

Processes individual import items from fan-out imports using the new Job + JobItem pattern.
Supports high and low priority task variants using NATS subject-based routing.
"""

import json
import logging
from datetime import datetime, timezone

from sqlmodel import Session, select, func, col

from app.worker.broker import broker
from app.worker.queues import SUBJECT_HIGH_IMPORT, SUBJECT_LOW_IMPORT
from app.core.database import get_sync_session
from app.models.job import Job, JobItem, JobItemStatus, BackgroundJobStatus, BackgroundJobPriority
from app.services.igdb.service import IGDBService
from app.services.game_service import GameService
from app.worker.tasks.import_export.import_nexorious_helpers import (
    _process_nexorious_game,
    _process_wishlist_item,
)

logger = logging.getLogger(__name__)


@broker.task(task_name=SUBJECT_HIGH_IMPORT)
async def process_import_item_high(job_item_id: str) -> dict:
    """Process high-priority import item.

    Args:
        job_item_id: The JobItem ID to process

    Returns:
        Dictionary with processing result details
    """
    return await _process_import_item(job_item_id)


@broker.task(task_name=SUBJECT_LOW_IMPORT)
async def process_import_item_low(job_item_id: str) -> dict:
    """Process low-priority import item.

    Args:
        job_item_id: The JobItem ID to process

    Returns:
        Dictionary with processing result details
    """
    return await _process_import_item(job_item_id)


async def _update_job_item_error(job_item_id: str, error_message: str) -> dict:
    """Update a JobItem with error status using a fresh session.

    Args:
        job_item_id: The JobItem ID to update
        error_message: Error message to set

    Returns:
        Dictionary with error details
    """
    session = get_sync_session()
    try:
        job_item = session.get(JobItem, job_item_id)
        if job_item:
            job_id = job_item.job_id
            job_item.status = JobItemStatus.FAILED
            job_item.error_message = error_message
            job_item.processed_at = datetime.now(timezone.utc)
            session.add(job_item)
            session.commit()

            # Check if all items are processed and update job status
            _check_and_update_job_completion(session, job_id)
    except Exception as update_error:
        logger.error(f"Failed to update JobItem {job_item_id} with error: {update_error}")
    finally:
        session.close()

    return {"status": "error", "error": error_message}


def _check_and_update_job_completion(session: Session, job_id: str) -> bool:
    """Check if all job items are processed and update job status if complete.

    Args:
        session: Database session
        job_id: The Job ID to check

    Returns:
        True if job was marked as complete, False otherwise
    """
    # Count items that are still pending or processing
    pending_count = session.exec(
        select(func.count())
        .select_from(JobItem)
        .where(
            JobItem.job_id == job_id,
            col(JobItem.status).in_([JobItemStatus.PENDING, JobItemStatus.PROCESSING])
        )
    ).one()

    if pending_count > 0:
        return False

    # All items are processed - update job status
    job = session.get(Job, job_id)
    if not job:
        logger.error(f"Job {job_id} not found when checking completion")
        return False

    # Only update if job is not already in a terminal state
    if job.status in (BackgroundJobStatus.COMPLETED, BackgroundJobStatus.FAILED, BackgroundJobStatus.CANCELLED):
        return False

    # Check if any items failed to determine final status
    failed_count = session.exec(
        select(func.count())
        .select_from(JobItem)
        .where(JobItem.job_id == job_id, JobItem.status == JobItemStatus.FAILED)
    ).one()

    if failed_count > 0:
        # Some items failed but job completed processing
        job.status = BackgroundJobStatus.COMPLETED
    else:
        job.status = BackgroundJobStatus.COMPLETED

    job.completed_at = datetime.now(timezone.utc)
    session.add(job)
    session.commit()

    logger.info(f"Job {job_id} marked as {job.status.value}")
    return True


async def _process_import_item(job_item_id: str) -> dict:
    """Process a single import item.

    Implementation details:
    - Uses short-lived DB sessions to avoid holding connections during rate-limited API calls
    - Checks idempotency (skip if not PENDING or PROCESSING)
    - Sets status to PROCESSING before processing
    - Parses source_metadata_json to determine item type
    - Calls appropriate helper function (_process_nexorious_game or _process_wishlist_item)
    - Maps result string to JobItemStatus
    - Updates JobItem with status, result_json, and processed_at
    - Handles errors by setting FAILED status with error_message

    Args:
        job_item_id: The JobItem ID to process

    Returns:
        Dictionary with processing result details
    """
    # Phase 1: Fetch job item and validate (short-lived session)
    session = get_sync_session()
    try:
        job_item = session.get(JobItem, job_item_id)
        if not job_item:
            logger.error(f"JobItem {job_item_id} not found")
            return {"status": "error", "error": "JobItem not found"}

        # Idempotency check - skip if already processed
        if job_item.status not in (JobItemStatus.PENDING, JobItemStatus.PROCESSING):
            logger.info(
                f"JobItem {job_item_id} already processed with status {job_item.status}"
            )
            return {
                "status": "skipped",
                "reason": "already_processed",
                "current_status": job_item.status.value,
            }

        # Set status to PROCESSING
        job_item.status = JobItemStatus.PROCESSING
        session.add(job_item)
        session.commit()

        # Extract data we need before closing session
        user_id = job_item.user_id
        job_id = job_item.job_id
        source_metadata_json = job_item.source_metadata_json
    finally:
        session.close()

    # Phase 2: Parse metadata (no DB needed)
    try:
        source_metadata = json.loads(source_metadata_json)
    except json.JSONDecodeError as e:
        logger.error(f"Failed to parse source_metadata_json for JobItem {job_item_id}: {e}")
        return await _update_job_item_error(
            job_item_id, f"Invalid JSON in source_metadata: {str(e)}"
        )

    item_type = source_metadata.get("item_type")
    if not item_type:
        logger.error(f"No item_type found in source_metadata for JobItem {job_item_id}")
        return await _update_job_item_error(
            job_item_id, "No item_type in source_metadata"
        )

    # Phase 3: Process with fresh session (may involve rate-limited IGDB calls)
    session = get_sync_session()
    try:
        # Initialize services
        igdb_service = IGDBService()
        game_service = GameService(session, igdb_service)

        # Process based on item type
        result_status: str
        if item_type == "game":
            game_data = source_metadata.get("data", {})
            result_status = await _process_nexorious_game(
                session, game_service, user_id, game_data
            )
        elif item_type == "wishlist":
            wishlist_data = source_metadata.get("data", {})
            result_status = await _process_wishlist_item(
                session, game_service, user_id, wishlist_data
            )
        else:
            logger.error(f"Unknown item_type '{item_type}' for JobItem {job_item_id}")
            session.close()
            return await _update_job_item_error(
                job_item_id, f"Unknown item_type: {item_type}"
            )

        # Map result string to JobItemStatus
        status_mapping = {
            "imported": JobItemStatus.COMPLETED,
            "already_in_collection": JobItemStatus.COMPLETED,
            "already_exists": JobItemStatus.COMPLETED,
            "skipped_no_igdb_id": JobItemStatus.SKIPPED,
            "skipped_invalid": JobItemStatus.SKIPPED,
            "error": JobItemStatus.FAILED,
        }

        job_item_status = status_mapping.get(result_status, JobItemStatus.FAILED)

        # Re-fetch job_item in this session and update with result
        job_item = session.get(JobItem, job_item_id)
        if job_item:
            job_item.status = job_item_status
            job_item.result_json = json.dumps({
                "result": result_status,
                "item_type": item_type,
                "title": source_metadata.get("data", {}).get("title"),
            })
            job_item.processed_at = datetime.now(timezone.utc)

            # Set error message if status is FAILED
            if job_item_status == JobItemStatus.FAILED:
                job_item.error_message = f"Processing returned error status: {result_status}"

            session.add(job_item)
            session.commit()

            # Check if all items are processed and update job status
            _check_and_update_job_completion(session, job_id)

        logger.info(
            f"Processed JobItem {job_item_id}: {result_status} -> {job_item_status.value}"
        )

        return {
            "status": "success",
            "result": result_status,
            "job_item_status": job_item_status.value,
            "item_type": item_type,
        }

    except Exception as e:
        logger.error(f"Failed to process JobItem {job_item_id}: {e}", exc_info=True)
        session.close()
        return await _update_job_item_error(job_item_id, str(e)[:500])

    finally:
        session.close()


async def enqueue_import_task(job_item_id: str, priority: BackgroundJobPriority):
    """Enqueue import task with appropriate priority.

    Routes the task to the appropriate priority queue based on the priority parameter.

    Args:
        job_item_id: The JobItem ID to process
        priority: Priority level (HIGH or LOW)
    """
    if priority == BackgroundJobPriority.HIGH:
        await process_import_item_high.kiq(job_item_id)
    else:
        await process_import_item_low.kiq(job_item_id)

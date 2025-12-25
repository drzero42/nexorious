"""Generic import item processor for Nexorious JSON imports.

Processes individual import items from fan-out imports using the new Job + JobItem pattern.
Supports high and low priority task variants using NATS subject-based routing.
"""

import json
import logging
from datetime import datetime, timezone

from sqlmodel import Session

from app.worker.broker import broker
from app.worker.queues import SUBJECT_HIGH_IMPORT, SUBJECT_LOW_IMPORT
from app.core.database import get_sync_session
from app.models.job import JobItem, JobItemStatus, BackgroundJobPriority
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


async def _process_import_item(job_item_id: str) -> dict:
    """Process a single import item.

    Implementation details:
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
    session: Session = get_sync_session()

    try:
        # Fetch the JobItem
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

        # Parse source_metadata_json to get item type and data
        try:
            source_metadata = json.loads(job_item.source_metadata_json)
        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse source_metadata_json for JobItem {job_item_id}: {e}")
            job_item.status = JobItemStatus.FAILED
            job_item.error_message = f"Invalid JSON in source_metadata: {str(e)}"
            job_item.processed_at = datetime.now(timezone.utc)
            session.add(job_item)
            session.commit()
            return {"status": "error", "error": "Invalid JSON in source_metadata"}

        item_type = source_metadata.get("item_type")
        if not item_type:
            logger.error(f"No item_type found in source_metadata for JobItem {job_item_id}")
            job_item.status = JobItemStatus.FAILED
            job_item.error_message = "No item_type in source_metadata"
            job_item.processed_at = datetime.now(timezone.utc)
            session.add(job_item)
            session.commit()
            return {"status": "error", "error": "No item_type in source_metadata"}

        # Initialize services
        igdb_service = IGDBService()
        game_service = GameService(session, igdb_service)

        # Process based on item type
        result_status: str
        if item_type == "game":
            game_data = source_metadata.get("data", {})
            result_status = await _process_nexorious_game(
                session, game_service, job_item.user_id, game_data
            )
        elif item_type == "wishlist":
            wishlist_data = source_metadata.get("data", {})
            result_status = await _process_wishlist_item(
                session, game_service, job_item.user_id, wishlist_data
            )
        else:
            logger.error(f"Unknown item_type '{item_type}' for JobItem {job_item_id}")
            job_item.status = JobItemStatus.FAILED
            job_item.error_message = f"Unknown item_type: {item_type}"
            job_item.processed_at = datetime.now(timezone.utc)
            session.add(job_item)
            session.commit()
            return {"status": "error", "error": f"Unknown item_type: {item_type}"}

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

        # Update JobItem with result
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

        # Try to update JobItem with error
        try:
            job_item = session.get(JobItem, job_item_id)
            if job_item:
                job_item.status = JobItemStatus.FAILED
                job_item.error_message = str(e)[:500]  # Limit error message length
                job_item.processed_at = datetime.now(timezone.utc)
                session.add(job_item)
                session.commit()
        except Exception as update_error:
            logger.error(
                f"Failed to update JobItem {job_item_id} with error: {update_error}"
            )

        return {"status": "error", "error": str(e)}

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

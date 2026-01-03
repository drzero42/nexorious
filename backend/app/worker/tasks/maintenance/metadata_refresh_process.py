"""Metadata refresh item processor for individual game processing.

Processes individual JobItems created by the dispatch task.
Fetches fresh metadata from IGDB and updates game records.
"""

import json
import logging
from datetime import datetime, timezone
from typing import Dict, Any

from sqlmodel import select, func, col

from app.worker.broker import broker
from app.core.database import get_sync_session
from app.models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobStatus,
    BackgroundJobType,
)
from app.models.game import Game
from app.services.igdb.service import IGDBService
from app.services.game_service import GameService

logger = logging.getLogger(__name__)


@broker.task(task_name="maintenance.metadata_refresh_process")
async def process_metadata_refresh(job_item_id: str) -> Dict[str, Any]:
    """
    Process a single metadata refresh item.

    Fetches fresh metadata from IGDB and updates the game record.

    Args:
        job_item_id: The JobItem ID to process

    Returns:
        Dictionary with processing result details
    """
    # Phase 1: Fetch job item and validate
    session = get_sync_session()
    try:
        job_item = session.get(JobItem, job_item_id)
        if not job_item:
            logger.error(f"JobItem {job_item_id} not found")
            return {"status": "error", "error": "JobItem not found"}

        # Idempotency check
        if job_item.status not in (JobItemStatus.PENDING, JobItemStatus.PROCESSING):
            logger.info(f"JobItem {job_item_id} already processed: {job_item.status}")
            return {"status": "skipped", "reason": "already_processed"}

        # Set status to PROCESSING
        job_item.status = JobItemStatus.PROCESSING
        session.add(job_item)
        session.commit()

        # Extract data
        job_id = job_item.job_id
        source_metadata_json = job_item.source_metadata_json
        source_title = job_item.source_title
    finally:
        session.close()

    # Phase 2: Parse metadata
    try:
        metadata = json.loads(source_metadata_json)
    except json.JSONDecodeError as e:
        logger.error(f"Invalid JSON in JobItem {job_item_id}: {e}")
        return await _update_job_item_error(job_item_id, f"Invalid metadata: {e}")

    game_id = metadata.get("game_id")
    if not game_id:
        return await _update_job_item_error(job_item_id, "Missing game_id in metadata")

    # Phase 3: Process with fresh session
    session = get_sync_session()
    try:
        # Get the game
        game = session.get(Game, game_id)
        if not game:
            return await _update_job_item_error(job_item_id, f"Game {game_id} not found")

        logger.info(f"Refreshing metadata for game {game.title} (ID: {game_id})")

        # Fetch fresh metadata from IGDB
        igdb_service = IGDBService()
        game_service = GameService(session, igdb_service)

        # Record what changed for reporting
        old_values = {
            "game_modes": game.game_modes,
            "themes": game.themes,
            "player_perspectives": game.player_perspectives,
        }

        # Update game with fresh metadata from IGDB
        await game_service.create_or_update_game_from_igdb(game_id)

        # Refresh to get updated values
        session.refresh(game)

        new_values = {
            "game_modes": game.game_modes,
            "themes": game.themes,
            "player_perspectives": game.player_perspectives,
        }

        # Determine what changed
        updated_fields = []
        for field, old_val in old_values.items():
            new_val = new_values[field]
            if old_val != new_val:
                updated_fields.append(field)

        result_data = {
            "game_id": game_id,
            "game_title": game.title,
            "updated_fields": updated_fields,
            "old_values": old_values,
            "new_values": new_values,
        }

        # Update JobItem with result
        job_item = session.get(JobItem, job_item_id)
        if job_item:
            job_item.resolved_igdb_id = game_id
            job_item.result_json = json.dumps(result_data)
            session.add(job_item)

        result = "updated" if updated_fields else "unchanged"
        logger.info(f"Metadata refresh for {game.title}: {result} (fields: {updated_fields})")

        return await _complete_job_item(
            session, job_item_id, job_id,
            JobItemStatus.COMPLETED, result
        )

    except Exception as e:
        logger.error(f"Error processing metadata refresh for JobItem {job_item_id}: {e}", exc_info=True)
        return await _update_job_item_error(job_item_id, str(e)[:500])
    finally:
        session.close()


async def _complete_job_item(
    session,
    job_item_id: str,
    job_id: str,
    status: JobItemStatus,
    result: str,
) -> Dict[str, Any]:
    """Mark JobItem as complete and check job completion."""
    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.status = status
        job_item.processed_at = datetime.now(timezone.utc)
        session.add(job_item)
        session.commit()

    _check_and_update_job_completion(session, job_id)

    return {"status": "success", "result": result, "job_item_status": status.value}


async def _update_job_item_error(job_item_id: str, error_message: str) -> Dict[str, Any]:
    """Update JobItem with error status."""
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

            _check_and_update_job_completion(session, job_id)
    finally:
        session.close()

    return {"status": "error", "error": error_message}


def _check_and_update_job_completion(session, job_id: str) -> bool:
    """Check if all job items are processed and update job status.

    A job is considered complete when ALL items are in terminal states:
    - COMPLETED
    - SKIPPED
    - FAILED

    Returns:
        True if job was marked complete, False otherwise
    """
    # Count items that are NOT in terminal state (still need work)
    non_terminal_count = session.exec(
        select(func.count())
        .select_from(JobItem)
        .where(
            JobItem.job_id == job_id,
            col(JobItem.status).in_([
                JobItemStatus.PENDING,
                JobItemStatus.PROCESSING,
            ])
        )
    ).one()

    if non_terminal_count > 0:
        return False

    # All items processed - update job
    job = session.get(Job, job_id)
    if not job:
        logger.error(f"Job {job_id} not found when checking completion")
        return False

    # Only update if not already terminal
    if job.status in (BackgroundJobStatus.COMPLETED, BackgroundJobStatus.FAILED, BackgroundJobStatus.CANCELLED):
        return False

    # Mark job complete
    job.status = BackgroundJobStatus.COMPLETED
    job.completed_at = datetime.now(timezone.utc)
    session.add(job)
    session.commit()
    logger.info(f"Metadata refresh job {job_id} marked as COMPLETED")

    return True

"""Sync dispatch task for fan-out processing.

Creates JobItems for each game from an external source and dispatches
individual processing tasks for parallel execution.
"""

import json
import logging
from datetime import datetime, timezone
from typing import Dict, Any

from sqlmodel import Session

from app.worker.broker import broker
from app.worker.queues import enqueue_task
from app.core.database import get_session_context
from app.models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobStatus,
    BackgroundJobPriority,
)
from app.models.user import User
from app.worker.tasks.sync.adapters import ExternalGame, get_sync_adapter

logger = logging.getLogger(__name__)


@broker.task(task_name="sync.dispatch")
async def dispatch_sync_items(
    job_id: str,
    user_id: str,
    source: str,
) -> Dict[str, Any]:
    """
    Fan-out task that creates JobItems and dispatches worker tasks.

    This task:
    1. Fetches the user's game library via the source adapter
    2. Creates a JobItem for each game (streaming insert)
    3. Dispatches a process_sync_item task for each JobItem
    4. Returns quickly - actual processing happens in parallel workers

    Args:
        job_id: The Job ID for tracking progress
        user_id: The user to sync
        source: Source identifier ("steam", "epic", "gog")

    Returns:
        Dictionary with dispatch statistics.
    """
    logger.info(f"Starting sync dispatch for user {user_id}, source {source} (job: {job_id})")

    stats: Dict[str, int] = {
        "total_games": 0,
        "dispatched": 0,
        "errors": 0,
    }

    async with get_session_context() as session:
        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            return {"status": "error", "error": "Job not found"}

        try:
            # Update job status to PROCESSING
            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            # Get user
            user = session.get(User, user_id)
            if not user:
                raise ValueError(f"User {user_id} not found")

            # Get adapter and fetch games
            adapter = get_sync_adapter(source)
            games = await adapter.fetch_games(user)
            stats["total_games"] = len(games)

            # Update job total_items
            job.total_items = len(games)
            session.add(job)
            session.commit()

            logger.info(f"Fetched {len(games)} games from {source} for user {user_id}")

            # Determine priority for item tasks
            priority = job.priority

            # Stream create JobItems and dispatch tasks
            for game in games:
                try:
                    job_item = _create_job_item(
                        session=session,
                        job=job,
                        user_id=user_id,
                        game=game,
                    )

                    # Dispatch worker task
                    await _dispatch_process_task(job_item.id, priority)
                    stats["dispatched"] += 1

                except Exception as e:
                    logger.error(f"Error creating/dispatching item for {game.title}: {e}")
                    stats["errors"] += 1

            logger.info(
                f"Sync dispatch completed for job {job_id}: "
                f"{stats['dispatched']} dispatched, {stats['errors']} errors"
            )

            # Note: Job stays in PROCESSING state
            # It will be marked COMPLETED by the last worker task

            return {"status": "dispatched", **stats}

        except Exception as e:
            logger.error(f"Sync dispatch failed for job {job_id}: {e}")
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e), **stats}


def _create_job_item(
    session: Session,
    job: Job,
    user_id: str,
    game: ExternalGame,
) -> JobItem:
    """Create a JobItem for a game.

    Args:
        session: Database session
        job: The parent Job
        user_id: User ID
        game: ExternalGame from adapter

    Returns:
        Created JobItem
    """
    source_metadata = {
        "external_id": game.external_id,
        "platform": game.platform,
        "storefront": game.storefront,
        "metadata": game.metadata,
        "playtime_hours": game.playtime_hours,
    }

    job_item = JobItem(
        job_id=job.id,
        user_id=user_id,
        item_key=f"{game.storefront}_{game.external_id}",
        source_title=game.title,
        source_metadata_json=json.dumps(source_metadata),
        status=JobItemStatus.PENDING,
    )

    session.add(job_item)
    session.commit()
    session.refresh(job_item)

    logger.debug(f"Created JobItem {job_item.id} for {game.title}")

    return job_item


async def _dispatch_process_task(job_item_id: str, priority: BackgroundJobPriority) -> None:
    """Dispatch a process_sync_item task for a JobItem.

    Args:
        job_item_id: The JobItem ID to process
        priority: Task priority (HIGH or LOW)
    """
    from app.worker.tasks.sync.process_item import process_sync_item

    await enqueue_task(process_sync_item, job_item_id, priority=priority)

"""Metadata refresh dispatch task for fan-out processing.

Creates JobItems for each game in the database and dispatches
individual processing tasks for parallel execution.
"""

import json
import logging
from datetime import datetime, timezone
from typing import Dict, Any, List, Optional

from sqlmodel import Session, select

from app.worker.broker import broker
from app.worker.queues import enqueue_task
from app.core.database import get_session_context
from app.models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobStatus,
    BackgroundJobPriority,
    BackgroundJobType,
    BackgroundJobSource,
)
from app.models.game import Game
from app.models.user import User

logger = logging.getLogger(__name__)


@broker.task(task_name="maintenance.metadata_refresh_dispatch")
async def dispatch_metadata_refresh(
    job_id: str,
    user_id: str,
    game_ids: Optional[List[int]] = None,
) -> Dict[str, Any]:
    """
    Fan-out task that creates JobItems and dispatches worker tasks for metadata refresh.

    This task:
    1. Fetches games from the database (all games or specific game_ids)
    2. Creates a JobItem for each game (streaming insert)
    3. Dispatches a process_metadata_refresh task for each JobItem
    4. Returns quickly - actual processing happens in parallel workers

    Args:
        job_id: The Job ID for tracking progress
        user_id: The user who initiated the refresh
        game_ids: Optional list of specific game IDs to refresh (None = all games)

    Returns:
        Dictionary with dispatch statistics.
    """
    logger.info(f"Starting metadata refresh dispatch (job: {job_id})")

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

            # Get games to refresh
            if game_ids:
                games = session.exec(
                    select(Game).where(Game.id.in_(game_ids))  # type: ignore[attr-defined]
                ).all()
            else:
                games = session.exec(select(Game)).all()

            stats["total_games"] = len(games)

            # Update job total_items
            job.total_items = len(games)
            session.add(job)
            session.commit()

            logger.info(f"Found {len(games)} games to refresh metadata")

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
                    logger.error(f"Error creating/dispatching item for game {game.id} ({game.title}): {e}")
                    stats["errors"] += 1

            logger.info(
                f"Metadata refresh dispatch completed for job {job_id}: "
                f"{stats['dispatched']} dispatched, {stats['errors']} errors"
            )

            # Note: Job stays in PROCESSING state
            # It will be marked COMPLETED by the last worker task

            return {"status": "dispatched", **stats}

        except Exception as e:
            logger.error(f"Metadata refresh dispatch failed for job {job_id}: {e}")
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
    game: Game,
) -> JobItem:
    """Create a JobItem for a game metadata refresh.

    Args:
        session: Database session
        job: The parent Job
        user_id: User ID who initiated the refresh
        game: Game to refresh

    Returns:
        Created JobItem
    """
    source_metadata = {
        "game_id": game.id,
        "current_title": game.title,
        "current_game_modes": game.game_modes,
        "current_themes": game.themes,
        "current_player_perspectives": game.player_perspectives,
    }

    job_item = JobItem(
        job_id=job.id,
        user_id=user_id,
        item_key=f"game_{game.id}",
        source_title=game.title,
        source_metadata_json=json.dumps(source_metadata),
        status=JobItemStatus.PENDING,
    )

    session.add(job_item)
    session.commit()
    session.refresh(job_item)

    logger.debug(f"Created JobItem {job_item.id} for game {game.title} (ID: {game.id})")

    return job_item


async def _dispatch_process_task(job_item_id: str, priority: BackgroundJobPriority) -> None:
    """Dispatch a process_metadata_refresh task for a JobItem.

    Args:
        job_item_id: The JobItem ID to process
        priority: Task priority (HIGH or LOW)
    """
    from app.worker.tasks.maintenance.metadata_refresh_process import process_metadata_refresh

    await enqueue_task(process_metadata_refresh, job_item_id, priority=priority)

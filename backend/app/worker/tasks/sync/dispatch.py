"""Sync dispatch task: fetch library, upsert ExternalGame records, mark removed games.

Phases 1-3 of the sync flow. After completing, dispatches process_sync_item
for each ExternalGame that still needs IGDB resolution or collection sync.
"""

import logging
from datetime import datetime, timezone
from typing import Dict, Any, Set

from sqlmodel import Session, select

from app.worker.broker import broker
from app.worker.queues import enqueue_task
from app.core.database import get_session_context
from app.models.job import (
    Job, JobItem, JobItemStatus, BackgroundJobStatus, BackgroundJobPriority,
)
from app.models.user import User
from app.models.external_game import ExternalGame
from app.models.user_game import OwnershipStatus
from app.worker.tasks.sync.adapters import ExternalLibraryEntry, get_sync_adapter

logger = logging.getLogger(__name__)


@broker.task(task_name="sync.dispatch")
async def dispatch_sync_items(
    job_id: str,
    user_id: str,
    source: str,
) -> Dict[str, Any]:
    """
    Phases 1-3: fetch library, upsert ExternalGames, mark removed.
    Then dispatches process_sync_item for each actionable ExternalGame.
    """
    logger.info(f"Starting sync dispatch for user {user_id}, source {source} (job: {job_id})")
    stats: Dict[str, int] = {"total_games": 0, "dispatched": 0, "errors": 0}

    async with get_session_context() as session:
        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            return {"status": "error", "error": "Job not found"}

        try:
            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            user = session.get(User, user_id)
            if not user:
                raise ValueError(f"User {user_id} not found")

            # Phase 1: Fetch from source
            adapter = get_sync_adapter(source)
            entries = await adapter.fetch_games(user, session)
            stats["total_games"] = len(entries)

            # Phase 2: Upsert ExternalGame records
            seen_external_ids: Set[str] = set()
            external_games: list[ExternalGame] = []
            for entry in entries:
                try:
                    eg = _upsert_external_game(session, user_id, entry)
                    external_games.append(eg)
                    seen_external_ids.add(entry.external_id)
                except Exception as e:
                    logger.error(f"Error upserting ExternalGame for {entry.title}: {e}")
                    stats["errors"] += 1
                    session.rollback()

            # Phase 3: Mark removed games + handle subscription lapses
            _mark_removed_games(session, user_id, source, seen_external_ids)

            # Dispatch process_sync_item for each available, non-skipped game
            actionable = [eg for eg in external_games if eg.is_available and not eg.is_skipped]
            job.total_items = len(actionable)
            session.add(job)
            session.commit()

            for eg in actionable:
                try:
                    job_item = _create_job_item(session, job, eg)
                    if job_item:
                        await _dispatch_process_task(job_item.id, job.priority)
                        stats["dispatched"] += 1
                except Exception as e:
                    logger.error(f"Error dispatching item for {eg.title}: {e}")
                    stats["errors"] += 1
                    session.rollback()

            logger.info(
                f"Sync dispatch completed for job {job_id}: "
                f"{stats['dispatched']} dispatched, {stats['errors']} errors"
            )
            return {"status": "dispatched", **stats}

        except Exception as e:
            logger.error(f"Sync dispatch failed for job {job_id}: {e}")
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e), **stats}


def _upsert_external_game(
    session: Session,
    user_id: str,
    entry: ExternalLibraryEntry,
) -> ExternalGame:
    """Phase 2: Find or create ExternalGame, always updating source fields."""
    existing = session.exec(
        select(ExternalGame).where(
            ExternalGame.user_id == user_id,
            ExternalGame.storefront == entry.storefront,
            ExternalGame.external_id == entry.external_id,
        )
    ).first()

    is_subscription = entry.ownership_status == OwnershipStatus.SUBSCRIPTION

    if existing:
        existing.title = entry.title
        existing.playtime_hours = entry.playtime_hours
        existing.is_subscription = is_subscription
        existing.ownership_status = entry.ownership_status
        existing.platform = entry.platform
        existing.is_available = True
        existing.updated_at = datetime.now(timezone.utc)
        session.add(existing)
        session.commit()
        session.refresh(existing)
        return existing
    else:
        eg = ExternalGame(
            user_id=user_id,
            storefront=entry.storefront,
            external_id=entry.external_id,
            title=entry.title,
            playtime_hours=entry.playtime_hours,
            is_subscription=is_subscription,
            ownership_status=entry.ownership_status,
            platform=entry.platform,
            is_available=True,
        )
        session.add(eg)
        session.commit()
        session.refresh(eg)
        return eg


def _mark_removed_games(
    session: Session,
    user_id: str,
    storefront: str,
    present_external_ids: Set[str],
) -> None:
    """Phase 3: Mark absent games unavailable. Auto-downgrade lapsed subscriptions."""
    all_available = session.exec(
        select(ExternalGame).where(
            ExternalGame.user_id == user_id,
            ExternalGame.storefront == storefront,
            ExternalGame.is_available == True,
        )
    ).all()

    for eg in all_available:
        if eg.external_id not in present_external_ids:
            eg.is_available = False
            eg.updated_at = datetime.now(timezone.utc)
            session.add(eg)

            if eg.is_subscription:
                for ugp in eg.user_game_platforms:
                    if ugp.ownership_status == OwnershipStatus.SUBSCRIPTION:
                        ugp.ownership_status = OwnershipStatus.NO_LONGER_OWNED
                        ugp.updated_at = datetime.now(timezone.utc)
                        session.add(ugp)

    session.commit()


def _create_job_item(session: Session, job: Job, eg: ExternalGame) -> JobItem | None:
    """Create a JobItem for an ExternalGame (idempotent by job + external game key)."""
    item_key = f"{eg.storefront}_{eg.external_id}"
    existing = session.exec(
        select(JobItem).where(JobItem.job_id == job.id, JobItem.item_key == item_key)
    ).first()
    if existing:
        return None

    job_item = JobItem(
        job_id=job.id,
        user_id=eg.user_id,
        item_key=item_key,
        source_title=eg.title,
        source_metadata_json=f'{{"external_game_id": "{eg.id}"}}',
        status=JobItemStatus.PENDING,
    )
    session.add(job_item)
    session.commit()
    session.refresh(job_item)
    return job_item


async def _dispatch_process_task(job_item_id: str, priority: BackgroundJobPriority) -> None:
    from app.worker.tasks.sync.process_item import process_sync_item
    await enqueue_task(process_sync_item, job_item_id, priority=priority)

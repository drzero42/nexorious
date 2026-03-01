"""Sync item processor: Phases 4 (IGDB resolution) and 5 (collection sync).

Each JobItem carries an ExternalGame ID in its source_metadata_json.
Phase 4: If unresolved, runs IGDB matching. High confidence → sets resolved_igdb_id.
         Low confidence → PENDING_REVIEW (user resolves, next sync picks it up).
Phase 5: If resolved, syncs playtime + ownership_status to UserGamePlatform.
         Auto-links manually added entries on first match by (user+igdb+platform+storefront).
"""

import json
import logging
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

from sqlmodel import Session, select, func, col

from app.worker.broker import broker
from app.core.database import get_sync_session
from app.models.job import Job, JobItem, JobItemStatus, BackgroundJobStatus
from app.models.game import Game
from app.models.user_game import UserGame, UserGamePlatform, OwnershipStatus
from app.models.external_game import ExternalGame
from app.services.igdb.service import IGDBService
from app.services.matching.service import MatchingService
from app.services.matching.models import MatchRequest, MatchStatus
from app.services.game_service import GameService

logger = logging.getLogger(__name__)

AUTO_MATCH_CONFIDENCE_THRESHOLD = 0.85

OWNERSHIP_PRECEDENCE: dict[OwnershipStatus, int] = {
    OwnershipStatus.OWNED: 4,
    OwnershipStatus.BORROWED: 3,
    OwnershipStatus.RENTED: 3,
    OwnershipStatus.SUBSCRIPTION: 2,
    OwnershipStatus.NO_LONGER_OWNED: 1,
}


def _should_update_ownership(current: OwnershipStatus, incoming: OwnershipStatus) -> bool:
    """Return True only if incoming ownership rank >= current rank (never downgrade)."""
    return OWNERSHIP_PRECEDENCE.get(incoming, 0) >= OWNERSHIP_PRECEDENCE.get(current, 0)


@broker.task(task_name="sync.process_item")
async def process_sync_item(job_item_id: str) -> Dict[str, Any]:
    """Process a single sync item via its linked ExternalGame."""
    session = get_sync_session()
    try:
        job_item = session.get(JobItem, job_item_id)
        if not job_item:
            return {"status": "error", "error": "JobItem not found"}
        if job_item.status not in (JobItemStatus.PENDING, JobItemStatus.PROCESSING):
            return {"status": "skipped", "reason": "already_processed"}

        job_item.status = JobItemStatus.PROCESSING
        session.add(job_item)
        session.commit()

        metadata = json.loads(job_item.source_metadata_json)
        external_game_id = metadata.get("external_game_id")
        job_id = job_item.job_id
    finally:
        session.close()

    if not external_game_id:
        return await _update_job_item_error(job_item_id, "Missing external_game_id in metadata")

    session = get_sync_session()
    try:
        eg = session.get(ExternalGame, external_game_id)
        if not eg:
            return await _update_job_item_error(job_item_id, f"ExternalGame {external_game_id} not found")

        if eg.is_skipped:
            return await _complete_job_item(session, job_item_id, job_id, JobItemStatus.SKIPPED, "skipped")

        # Phase 4: Resolve IGDB if not yet resolved
        if not eg.resolved_igdb_id:
            resolved = await _resolve_igdb(session, job_item_id, job_id, eg)
            if not resolved:
                return {"status": "success", "result": "pending_review"}

        # Phase 5: Sync resolved game to collection
        _sync_external_game_to_collection(session, eg)

        game = session.get(Game, eg.resolved_igdb_id)
        job_item = session.get(JobItem, job_item_id)
        if job_item and game:
            job_item.resolved_igdb_id = eg.resolved_igdb_id
            job_item.result_json = json.dumps({
                "igdb_title": game.title,
                "igdb_id": game.id,
                "result_type": "synced",
            })
            session.add(job_item)

        return await _complete_job_item(session, job_item_id, job_id, JobItemStatus.COMPLETED, "synced")

    except Exception as e:
        logger.error(f"Error processing JobItem {job_item_id}: {e}", exc_info=True)
        return await _update_job_item_error(job_item_id, str(e)[:500])
    finally:
        session.close()


async def _resolve_igdb(
    session: Session,
    job_item_id: str,
    job_id: str,
    eg: ExternalGame,
) -> bool:
    """Phase 4: IGDB matching. Returns True if resolved, False if sent to review."""
    igdb_service = IGDBService()
    matching_service = MatchingService(session, igdb_service)

    match_result = await matching_service.match_game(MatchRequest(
        source_title=eg.title,
        source_platform=eg.storefront,
        platform_id=eg.external_id,
        source_metadata={},
    ))

    if match_result.status == MatchStatus.MATCHED and match_result.igdb_id:
        confidence = match_result.confidence_score or 0.0
        if confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
            game_service = GameService(session, igdb_service)
            await game_service.create_or_update_game_from_igdb(match_result.igdb_id)
            eg.resolved_igdb_id = match_result.igdb_id
            eg.updated_at = datetime.now(timezone.utc)
            session.add(eg)
            session.commit()
            return True

        await _set_pending_review(
            session, job_item_id, job_id,
            candidates=match_result.candidates or [],
            confidence=confidence,
            igdb_id=match_result.igdb_id,
            igdb_title=match_result.igdb_title,
        )
        return False

    await _set_pending_review(
        session, job_item_id, job_id,
        candidates=getattr(match_result, "candidates", []) or [],
        confidence=0.0,
    )
    return False


def _sync_external_game_to_collection(session: Session, eg: ExternalGame) -> None:
    """Phase 5: Create or update UserGamePlatform, respecting ownership precedence."""
    if not eg.resolved_igdb_id:
        return

    # Find already-linked UserGamePlatform
    ugp = session.exec(
        select(UserGamePlatform)
        .join(UserGame, UserGamePlatform.user_game_id == UserGame.id)
        .where(
            UserGamePlatform.external_game_id == eg.id,
            UserGame.user_id == eg.user_id,
        )
    ).first()

    if ugp is None:
        # Check for a matching entry by game identity; also catches UGPs already claimed by
        # another ExternalGame (e.g. same game appearing under two Steam app IDs in a bundle)
        ugp = session.exec(
            select(UserGamePlatform)
            .join(UserGame, UserGamePlatform.user_game_id == UserGame.id)
            .where(
                UserGame.user_id == eg.user_id,
                UserGame.game_id == eg.resolved_igdb_id,
                UserGamePlatform.platform == eg.platform,
                UserGamePlatform.storefront == eg.storefront,
            )
        ).first()

        if ugp is None:
            ugp = _create_user_game_platform(session, eg)
        elif ugp.external_game_id is None:
            ugp.external_game_id = eg.id

    if ugp is None:
        return

    if not ugp.sync_from_source:
        return

    # Always update playtime
    ugp.hours_played = eg.playtime_hours

    # Update ownership only if incoming rank >= current rank
    incoming = eg.ownership_status or OwnershipStatus.OWNED
    if _should_update_ownership(ugp.ownership_status, incoming):
        ugp.ownership_status = incoming

    ugp.updated_at = datetime.now(timezone.utc)
    session.add(ugp)
    session.commit()


def _create_user_game_platform(session: Session, eg: ExternalGame) -> Optional[UserGamePlatform]:
    """Create UserGame (if needed) and a new linked UserGamePlatform."""
    from sqlalchemy import text

    insert_sql = text("""
        INSERT INTO user_games (id, user_id, game_id, personal_rating, is_loved, play_status,
                                hours_played, personal_notes, created_at, updated_at)
        VALUES (gen_random_uuid(), :user_id, :game_id, NULL, false, 'NOT_STARTED', 0, NULL, NOW(), NOW())
        ON CONFLICT (user_id, game_id) DO NOTHING
        RETURNING id
    """)
    result = session.execute(insert_sql, {"user_id": eg.user_id, "game_id": eg.resolved_igdb_id})
    row = result.first()

    if row:
        user_game_id = row[0]
    else:
        existing_ug = session.exec(
            select(UserGame).where(
                UserGame.user_id == eg.user_id,
                UserGame.game_id == eg.resolved_igdb_id,
            )
        ).first()
        if not existing_ug:
            logger.error(f"Could not find or create UserGame for user {eg.user_id}, game {eg.resolved_igdb_id}")
            return None
        user_game_id = existing_ug.id

    session.commit()

    ugp = UserGamePlatform(
        user_game_id=user_game_id,
        platform=eg.platform,
        storefront=eg.storefront,
        external_game_id=eg.id,
        is_available=True,
        hours_played=eg.playtime_hours,
        ownership_status=eg.ownership_status or OwnershipStatus.OWNED,
        sync_from_source=True,
    )
    session.add(ugp)
    session.commit()
    session.refresh(ugp)
    return ugp


async def _set_pending_review(
    session: Session,
    job_item_id: str,
    job_id: str,
    candidates: List[Any],
    confidence: float,
    igdb_id: Optional[int] = None,
    igdb_title: Optional[str] = None,
) -> None:
    serializable = []
    for c in candidates:
        if hasattr(c, "to_dict"):
            serializable.append(c.to_dict())
        elif isinstance(c, dict):
            serializable.append(c)
        else:
            try:
                serializable.append(c.__dict__)
            except AttributeError:
                pass

    if igdb_id and igdb_title:
        existing_ids = {c.get("igdb_id") for c in serializable}
        if igdb_id not in existing_ids:
            serializable.insert(0, {"igdb_id": igdb_id, "name": igdb_title, "similarity_score": confidence})

    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.status = JobItemStatus.PENDING_REVIEW
        job_item.igdb_candidates_json = json.dumps(serializable)
        job_item.match_confidence = confidence if confidence > 0 else None
        job_item.processed_at = datetime.now(timezone.utc)
        session.add(job_item)
        session.commit()

    _check_and_update_job_completion(session, job_id)


async def _complete_job_item(
    session: Session,
    job_item_id: str,
    job_id: str,
    status: JobItemStatus,
    result: str,
) -> Dict[str, Any]:
    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.status = status
        job_item.processed_at = datetime.now(timezone.utc)
        session.add(job_item)
        session.commit()
    _check_and_update_job_completion(session, job_id)
    return {"status": "success", "result": result, "job_item_status": status.value}


async def _update_job_item_error(job_item_id: str, error_msg: str) -> Dict[str, Any]:
    session = get_sync_session()
    try:
        job_item = session.get(JobItem, job_item_id)
        if job_item:
            job_item.status = JobItemStatus.FAILED
            job_item.error_message = error_msg
            job_item.processed_at = datetime.now(timezone.utc)
            session.add(job_item)
            session.commit()
            _check_and_update_job_completion(session, job_item.job_id)
    finally:
        session.close()
    return {"status": "error", "error": error_msg}


def _check_and_update_job_completion(session: Session, job_id: str) -> bool:
    """Check if all job items are processed and update job status.

    PENDING_REVIEW items block completion — user must resolve them first.
    Returns True if job was marked complete, False otherwise.
    """
    if not job_id:
        return False

    non_terminal_count = session.exec(
        select(func.count()).select_from(JobItem).where(
            JobItem.job_id == job_id,
            col(JobItem.status).in_([
                JobItemStatus.PENDING,
                JobItemStatus.PROCESSING,
                JobItemStatus.PENDING_REVIEW,
            ]),
        )
    ).first() or 0

    if non_terminal_count > 0:
        return False

    job = session.get(Job, job_id)
    if not job:
        return False

    if job.status in (BackgroundJobStatus.COMPLETED, BackgroundJobStatus.FAILED, BackgroundJobStatus.CANCELLED):
        return False

    job.status = BackgroundJobStatus.COMPLETED
    job.completed_at = datetime.now(timezone.utc)
    session.add(job)
    session.commit()
    logger.info(f"Job {job_id} marked as COMPLETED")
    return True

"""Sync item processor for individual game processing.

Processes individual JobItems created by the dispatch task.
Handles matching, linking, and review workflow.
"""

import json
import logging
from datetime import datetime, timezone
from typing import Dict, Any, Optional, List

from sqlmodel import Session, select, func, col

from app.worker.broker import broker
from app.core.database import get_sync_session
from app.models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobStatus,
    BackgroundJobType,
    BackgroundJobSource,
)
from app.models.game import Game
from app.models.user_game import UserGame, UserGamePlatform
from app.models.user_sync_config import UserSyncConfig
from app.models.ignored_external_game import IgnoredExternalGame
from app.services.igdb.service import IGDBService
from app.services.matching.service import MatchingService
from app.services.matching.models import MatchRequest, MatchStatus, MatchResult
from app.services.game_service import GameService

logger = logging.getLogger(__name__)

# Auto-match confidence threshold (85%)
AUTO_MATCH_CONFIDENCE_THRESHOLD = 0.85


@broker.task(task_name="sync.process_item")
async def process_sync_item(job_item_id: str) -> Dict[str, Any]:
    """
    Process a single sync item.

    Implements the processing flows:
    - Flow A (new item): Check synced -> check ignored -> IGDB match -> link/review
    - Flow B (reviewed item): Check synced -> use resolved_igdb_id -> link

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
        user_id = job_item.user_id
        job_id = job_item.job_id
        resolved_igdb_id = job_item.resolved_igdb_id
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

    external_id = metadata.get("external_id")
    platform = metadata.get("platform")
    storefront = metadata.get("storefront")
    playtime_hours = metadata.get("playtime_hours", 0)

    if not external_id or not storefront:
        return await _update_job_item_error(job_item_id, "Missing external_id or storefront")

    # Phase 3: Process with fresh session
    session = get_sync_session()
    try:
        # Step 1: Check if already synced
        existing_sync = _get_existing_sync(session, user_id, storefront, external_id)
        if existing_sync:
            user_game_id, game_id, game_title = existing_sync
            # Update JobItem with result data for recent activity display
            job_item = session.get(JobItem, job_item_id)
            if job_item:
                job_item.resolved_igdb_id = game_id
                job_item.result_json = json.dumps({
                    "igdb_title": game_title,
                    "igdb_id": game_id,
                    "user_game_id": user_game_id,
                    "result_type": "already_synced",
                })
                session.add(job_item)
            return await _complete_job_item(
                session, job_item_id, job_id,
                JobItemStatus.COMPLETED, "already_synced"
            )

        # Step 2: Check if ignored
        if _is_ignored(session, user_id, storefront, external_id):
            return await _complete_job_item(
                session, job_item_id, job_id,
                JobItemStatus.SKIPPED, "ignored"
            )

        # Step 3: Check if user provided resolved_igdb_id (Flow B)
        if resolved_igdb_id:
            return await _process_with_resolved_id(
                session, job_item_id, job_id, user_id,
                resolved_igdb_id, platform, storefront, external_id, source_title,
                playtime_hours
            )

        # Step 4: Flow A - Match via IGDB
        return await _process_with_matching(
            session, job_item_id, job_id, user_id,
            source_title, platform, storefront, external_id, metadata,
            playtime_hours
        )

    except Exception as e:
        logger.error(f"Error processing JobItem {job_item_id}: {e}", exc_info=True)
        return await _update_job_item_error(job_item_id, str(e)[:500])
    finally:
        session.close()


def _get_existing_sync(
    session: Session,
    user_id: str,
    storefront: str,
    external_id: str
) -> tuple[str, int, str] | None:
    """Check if game is already synced and return (user_game_id, game_id, game_title) if so."""
    result = session.exec(
        select(UserGamePlatform, UserGame, Game)
        .join(UserGame, UserGamePlatform.user_game_id == UserGame.id)
        .join(Game, UserGame.game_id == Game.id)
        .where(
            UserGame.user_id == user_id,
            UserGamePlatform.storefront == storefront,
            UserGamePlatform.store_game_id == external_id,
        )
    ).first()
    if result:
        _, user_game, game = result
        return (user_game.id, game.id, game.title)
    return None


def _is_ignored(
    session: Session,
    user_id: str,
    storefront: str,
    external_id: str
) -> bool:
    """Check if game is in the ignored list."""
    # Map storefront to BackgroundJobSource
    source_map = {
        "steam": BackgroundJobSource.STEAM,
        "epic": BackgroundJobSource.EPIC,
        "gog": BackgroundJobSource.GOG,
    }
    source = source_map.get(storefront)
    if not source:
        return False

    result = session.exec(
        select(IgnoredExternalGame).where(
            IgnoredExternalGame.user_id == user_id,
            IgnoredExternalGame.source == source,
            IgnoredExternalGame.external_id == external_id,
        )
    ).first()
    return result is not None


async def _process_with_resolved_id(
    session: Session,
    job_item_id: str,
    job_id: str,
    user_id: str,
    igdb_id: int,
    platform: str,
    storefront: str,
    external_id: str,
    source_title: str,
    playtime_hours: int = 0,
) -> Dict[str, Any]:
    """Process item with user-provided IGDB ID (Flow B)."""
    logger.info(f"Processing {source_title} with resolved IGDB ID {igdb_id}")

    # Check if game exists in user's collection
    existing_user_game = session.exec(
        select(UserGame).where(
            UserGame.user_id == user_id,
            UserGame.game_id == igdb_id,
        )
    ).first()

    if existing_user_game:
        # Add platform association to existing game
        _add_platform_association(
            session, existing_user_game.id,
            platform, storefront, external_id, playtime_hours
        )
        result = "linked_existing"
        user_game_id = existing_user_game.id
    else:
        # Create new UserGame and add platform association
        igdb_service = IGDBService()
        game_service = GameService(session, igdb_service)

        # First ensure the Game record exists in the database
        await game_service.create_or_update_game_from_igdb(igdb_id)

        # Then create the UserGame association
        user_game = UserGame(
            user_id=user_id,
            game_id=igdb_id,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        _add_platform_association(
            session, user_game.id,
            platform, storefront, external_id, playtime_hours
        )
        result = "imported_new"
        user_game_id = user_game.id

    # Fetch the game title for result_json
    game = session.get(Game, igdb_id)
    igdb_title = game.title if game else source_title

    # Update JobItem with result data for recent activity display
    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.resolved_igdb_id = igdb_id
        job_item.result_json = json.dumps({
            "igdb_title": igdb_title,
            "igdb_id": igdb_id,
            "user_game_id": user_game_id,
            "result_type": result,
        })
        session.add(job_item)

    return await _complete_job_item(
        session, job_item_id, job_id,
        JobItemStatus.COMPLETED, result
    )


async def _process_with_matching(
    session: Session,
    job_item_id: str,
    job_id: str,
    user_id: str,
    source_title: str,
    platform: str,
    storefront: str,
    external_id: str,
    metadata: Dict[str, Any],
    playtime_hours: int = 0,
) -> Dict[str, Any]:
    """Process item with IGDB matching (Flow A)."""
    logger.debug(f"Matching {source_title} via IGDB")

    igdb_service = IGDBService()
    matching_service = MatchingService(session, igdb_service)

    match_request = MatchRequest(
        source_title=source_title,
        source_platform=storefront,
        platform_id=external_id,
        source_metadata=metadata.get("metadata", {}),
    )

    match_result = await matching_service.match_game(match_request)

    if match_result.status == MatchStatus.MATCHED:
        confidence = match_result.confidence_score or 0.0
        igdb_id = match_result.igdb_id

        if not igdb_id:
            logger.warning(f"Match result MATCHED but no IGDB ID for {source_title}")
            return await _set_pending_review(
                session, job_item_id, job_id,
                candidates=[], confidence=0.0
            )

        if confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
            # High confidence - auto-import
            return await _auto_import_game(
                session, job_item_id, job_id, user_id,
                igdb_id, platform, storefront, external_id,
                match_result, confidence, playtime_hours
            )
        else:
            # Low confidence - needs review
            return await _set_pending_review(
                session, job_item_id, job_id,
                candidates=match_result.candidates or [],
                confidence=confidence,
                igdb_id=igdb_id,
                igdb_title=match_result.igdb_title,
            )

    elif match_result.status == MatchStatus.NEEDS_REVIEW:
        # Multiple candidates
        return await _set_pending_review(
            session, job_item_id, job_id,
            candidates=match_result.candidates or [],
            confidence=0.0,
        )

    else:
        # No match found
        return await _set_pending_review(
            session, job_item_id, job_id,
            candidates=[],
            confidence=0.0,
        )


async def _auto_import_game(
    session: Session,
    job_item_id: str,
    job_id: str,
    user_id: str,
    igdb_id: int,
    platform: str,
    storefront: str,
    external_id: str,
    match_result: MatchResult,
    confidence: float,
    playtime_hours: int = 0,
) -> Dict[str, Any]:
    """Auto-import a high-confidence match."""
    logger.info(f"Auto-importing {match_result.igdb_title} (confidence: {confidence:.2f})")

    # Check if game exists in user's collection
    existing_user_game = session.exec(
        select(UserGame).where(
            UserGame.user_id == user_id,
            UserGame.game_id == igdb_id,
        )
    ).first()

    if existing_user_game:
        # Add platform association to existing game
        _add_platform_association(
            session, existing_user_game.id,
            platform, storefront, external_id, playtime_hours
        )
        result = "auto_linked"
        user_game_id = existing_user_game.id
    else:
        # Create new UserGame
        igdb_service = IGDBService()
        game_service = GameService(session, igdb_service)

        # First ensure the Game record exists in the database
        await game_service.create_or_update_game_from_igdb(igdb_id)

        # Then create the UserGame association
        user_game = UserGame(
            user_id=user_id,
            game_id=igdb_id,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        _add_platform_association(
            session, user_game.id,
            platform, storefront, external_id, playtime_hours
        )
        result = "auto_imported"
        user_game_id = user_game.id

    # Update JobItem with match info AND result data for recent activity display
    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.resolved_igdb_id = igdb_id
        job_item.match_confidence = confidence
        job_item.result_json = json.dumps({
            "igdb_title": match_result.igdb_title,
            "igdb_id": igdb_id,
            "user_game_id": user_game_id,
            "result_type": result,
        })
        session.add(job_item)

    return await _complete_job_item(
        session, job_item_id, job_id,
        JobItemStatus.COMPLETED, result
    )


def _add_platform_association(
    session: Session,
    user_game_id: str,
    platform: str,
    storefront: str,
    external_id: str,
    playtime_hours: int = 0,
) -> None:
    """Add platform association to a UserGame."""
    # Check if association already exists
    # Note: unique constraint is on (user_game_id, platform, storefront)
    existing = session.exec(
        select(UserGamePlatform).where(
            UserGamePlatform.user_game_id == user_game_id,
            UserGamePlatform.platform == platform,
            UserGamePlatform.storefront == storefront,
        )
    ).first()

    if existing:
        # Update playtime if already exists (sync updates)
        existing.hours_played = playtime_hours
        session.add(existing)
        session.commit()
        logger.debug(f"Updated playtime for existing platform association on UserGame {user_game_id}")
    else:
        # Build store URL based on storefront
        store_url = None
        if storefront == "steam":
            store_url = f"https://store.steampowered.com/app/{external_id}"

        platform_assoc = UserGamePlatform(
            user_game_id=user_game_id,
            platform=platform,
            storefront=storefront,
            store_game_id=external_id,
            store_url=store_url,
            is_available=True,
            hours_played=playtime_hours,
        )
        session.add(platform_assoc)
        session.commit()
        logger.debug(f"Added platform association for UserGame {user_game_id} with {playtime_hours}h playtime")


async def _set_pending_review(
    session: Session,
    job_item_id: str,
    job_id: str,
    candidates: List[Any],
    confidence: float,
    igdb_id: Optional[int] = None,
    igdb_title: Optional[str] = None,
) -> Dict[str, Any]:
    """Set JobItem to PENDING_REVIEW status."""
    # Serialize candidates
    serializable_candidates = []
    for candidate in candidates:
        if hasattr(candidate, "to_dict"):
            serializable_candidates.append(candidate.to_dict())
        elif isinstance(candidate, dict):
            serializable_candidates.append(candidate)
        else:
            try:
                serializable_candidates.append(candidate.__dict__)
            except AttributeError:
                logger.warning(f"Failed to serialize candidate: {candidate}")

    # Add matched game to candidates if not present
    if igdb_id and igdb_title:
        candidate_ids = {c.get("igdb_id") for c in serializable_candidates}
        if igdb_id not in candidate_ids:
            serializable_candidates.insert(0, {
                "igdb_id": igdb_id,
                "name": igdb_title,
                "similarity_score": confidence,
            })

    job_item = session.get(JobItem, job_item_id)
    if job_item:
        job_item.status = JobItemStatus.PENDING_REVIEW
        job_item.igdb_candidates_json = json.dumps(serializable_candidates)
        job_item.match_confidence = confidence if confidence > 0 else None
        job_item.processed_at = datetime.now(timezone.utc)
        session.add(job_item)
        session.commit()

    # Check job completion (PENDING_REVIEW counts as "processed" for job status)
    _check_and_update_job_completion(session, job_id)

    return {"status": "success", "result": "pending_review", "candidates": len(serializable_candidates)}


async def _complete_job_item(
    session: Session,
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


def _check_and_update_job_completion(session: Session, job_id: str) -> bool:
    """Check if all job items are processed and update job status.

    A job is considered complete only when ALL items are in terminal states:
    - COMPLETED
    - SKIPPED
    - FAILED

    PENDING_REVIEW items block completion - user must resolve them first.

    Also updates last_synced_at for SYNC jobs when complete.

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
                JobItemStatus.PENDING_REVIEW,  # PENDING_REVIEW blocks completion
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

    # Update last_synced_at for SYNC jobs
    if job.job_type == BackgroundJobType.SYNC:
        _update_sync_config_timestamp(session, job.user_id, job.source)

    session.commit()
    logger.info(f"Job {job_id} marked as COMPLETED")

    return True


def _update_sync_config_timestamp(session: Session, user_id: str, source: BackgroundJobSource):
    """Update last_synced_at for the user's sync config."""
    platform_map = {
        BackgroundJobSource.STEAM: "steam",
        BackgroundJobSource.EPIC: "epic",
        BackgroundJobSource.GOG: "gog",
    }
    platform = platform_map.get(source)
    if not platform:
        return

    sync_config = session.exec(
        select(UserSyncConfig).where(
            UserSyncConfig.user_id == user_id,
            UserSyncConfig.platform == platform,
        )
    ).first()

    if sync_config:
        sync_config.last_synced_at = datetime.now(timezone.utc)
        session.add(sync_config)
        logger.info(f"Updated last_synced_at for user {user_id}, platform {platform}")

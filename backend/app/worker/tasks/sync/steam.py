"""Steam library sync task.

Fetches user's Steam library and syncs games to the collection.
Uses the matching service to match games to IGDB and creates
review items for games that need manual matching.
"""

import logging
import json
from datetime import datetime, timezone
from typing import Dict, Any, List, Optional

from sqlmodel import Session, select

from app.worker.broker import broker
from app.worker.queues import SUBJECT_HIGH_SYNC
from app.core.database import get_session_context
from app.models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobStatus,
    BackgroundJobSource,
)
from app.models.user import User
from app.models.user_sync_config import UserSyncConfig
from app.models.ignored_external_game import IgnoredExternalGame
from app.models.user_game import UserGame, UserGamePlatform
from app.services.steam import SteamService
from app.services.matching.service import MatchingService
from app.services.matching.models import MatchRequest, MatchStatus
from app.services.igdb.service import IGDBService

logger = logging.getLogger(__name__)

# Constants
STEAM_STOREFRONT_ID = "steam"
PC_WINDOWS_PLATFORM_ID = "pc-windows"
AUTO_MATCH_CONFIDENCE_THRESHOLD = 0.85


@broker.task(task_name=SUBJECT_HIGH_SYNC)
async def sync_steam_library(
    job_id: str,
    user_id: str,
    is_scheduled: bool = False,
) -> Dict[str, Any]:
    """
    Sync a user's Steam library.

    This task:
    1. Fetches the user's Steam library via Steam API
    2. Filters out already synced games (check UserGamePlatform)
    3. Filters out ignored games (check IgnoredExternalGame)
    4. For remaining games, matches to IGDB using MatchingService
    5. Auto-links high confidence matches to existing collection games
    6. Creates ReviewItems for games that need manual review
    7. Updates last_synced_at on completion

    Args:
        job_id: The Job ID for tracking progress
        user_id: The user to sync
        is_scheduled: Whether this is a scheduled sync (uses low priority)

    Returns:
        Dictionary with sync statistics.
    """
    logger.info(f"Starting Steam sync for user {user_id} (job: {job_id}, scheduled: {is_scheduled})")

    stats = {
        "total_games": 0,
        "already_synced": 0,
        "ignored": 0,
        "auto_linked": 0,
        "auto_matched": 0,
        "needs_review": 0,
        "no_match": 0,
        "errors": 0,
    }

    async with get_session_context() as session:
        # Get job first (outside try so exception handler can access it)
        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            return {"status": "error", "error": "Job not found"}

        try:
            # Update job status
            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            # Get user and sync config
            user = session.get(User, user_id)
            if not user:
                raise ValueError(f"User {user_id} not found")

            sync_config = _get_steam_sync_config(session, user_id)
            if not sync_config:
                raise ValueError("Steam sync not configured for this user")

            # Get Steam credentials from user preferences
            steam_credentials = _get_steam_credentials(user)
            if not steam_credentials:
                raise ValueError("Steam credentials not configured")

            # Create Steam service
            steam_service = SteamService(api_key=steam_credentials["api_key"])

            # Fetch Steam library
            logger.info(f"Fetching Steam library for user {user_id}")
            steam_games_list = await steam_service.get_owned_games(
                steam_credentials["steam_id"]
            )
            stats["total_games"] = len(steam_games_list)

            # Update job total_items
            job.total_items = len(steam_games_list)
            session.add(job)
            session.commit()

            # Get already synced Steam AppIDs
            synced_appids = _get_synced_steam_appids(session, user_id)
            logger.info(f"Found {len(synced_appids)} already synced Steam games")

            # Get ignored Steam AppIDs
            ignored_appids = _get_ignored_steam_appids(session, user_id)
            logger.info(f"Found {len(ignored_appids)} ignored Steam games")

            # Create IGDB service and matching service
            igdb_service = IGDBService()
            matching_service = MatchingService(session, igdb_service)

            # Process each game
            for i, steam_game in enumerate(steam_games_list):
                try:
                    appid = str(steam_game.appid)
                    game_name = steam_game.name

                    # Skip already synced games
                    if appid in synced_appids:
                        logger.debug(f"Skipping already synced game: {game_name} (AppID: {appid})")
                        stats["already_synced"] += 1
                        continue

                    # Skip ignored games
                    if appid in ignored_appids:
                        logger.debug(f"Skipping ignored game: {game_name} (AppID: {appid})")
                        stats["ignored"] += 1
                        continue

                    # Match to IGDB
                    logger.debug(f"Matching game to IGDB: {game_name} (AppID: {appid})")
                    match_request = MatchRequest(
                        source_title=game_name,
                        source_platform="steam",
                        platform_id=appid,
                        source_metadata={"steam_appid": appid},
                    )
                    match_result = await matching_service.match_game(match_request)

                    # Process match result
                    if match_result.status == MatchStatus.MATCHED:
                        confidence = match_result.confidence_score or 0.0
                        igdb_id = match_result.igdb_id

                        if not igdb_id:
                            logger.warning(f"Match result has MATCHED status but no IGDB ID for {game_name}")
                            stats["errors"] += 1
                            continue

                        # Check if game already in user's collection
                        existing_user_game = session.exec(
                            select(UserGame).where(
                                UserGame.user_id == user_id,
                                UserGame.game_id == igdb_id,
                            )
                        ).first()

                        if existing_user_game and confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
                            # High confidence + already in collection = auto-link platform association
                            logger.info(
                                f"Auto-linking Steam platform for existing game: {game_name} -> "
                                f"{match_result.igdb_title} (confidence: {confidence:.2f})"
                            )
                            _add_steam_platform(session, existing_user_game.id, appid)
                            stats["auto_linked"] += 1
                        else:
                            # Create JobItem (either new game or low confidence)
                            if confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
                                logger.info(
                                    f"Creating job item for auto-matched new game: {game_name} -> "
                                    f"{match_result.igdb_title} (confidence: {confidence:.2f})"
                                )
                                stats["auto_matched"] += 1
                            else:
                                logger.info(
                                    f"Creating job item for low confidence match: {game_name} -> "
                                    f"{match_result.igdb_title} (confidence: {confidence:.2f})"
                                )
                                stats["needs_review"] += 1

                            _create_job_item(
                                session=session,
                                job=job,
                                user_id=user_id,
                                source_title=game_name,
                                steam_appid=appid,
                                igdb_id=igdb_id,
                                igdb_title=match_result.igdb_title,
                                confidence=confidence,
                                candidates=match_result.candidates or [],
                            )

                    elif match_result.status == MatchStatus.NEEDS_REVIEW:
                        # Multiple candidates - needs manual review
                        logger.info(
                            f"Creating job item for multiple candidates: {game_name} "
                            f"({len(match_result.candidates or [])} candidates)"
                        )
                        _create_job_item(
                            session=session,
                            job=job,
                            user_id=user_id,
                            source_title=game_name,
                            steam_appid=appid,
                            igdb_id=None,
                            igdb_title=None,
                            confidence=0.0,
                            candidates=match_result.candidates or [],
                        )
                        stats["needs_review"] += 1

                    else:
                        # No match found
                        logger.info(f"No match found for game: {game_name} (AppID: {appid})")
                        _create_job_item(
                            session=session,
                            job=job,
                            user_id=user_id,
                            source_title=game_name,
                            steam_appid=appid,
                            igdb_id=None,
                            igdb_title=None,
                            confidence=0.0,
                            candidates=[],
                        )
                        stats["no_match"] += 1

                except Exception as e:
                    logger.error(f"Error processing Steam game {steam_game.appid} ({steam_game.name}): {e}")
                    stats["errors"] += 1

            # Update sync config last_synced_at
            sync_config.last_synced_at = datetime.now(timezone.utc)
            session.add(sync_config)

            # Set final job status
            job.status = BackgroundJobStatus.COMPLETED
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            logger.info(
                f"Steam sync completed for user {user_id}: "
                f"{stats['total_games']} total, "
                f"{stats['already_synced']} already synced, "
                f"{stats['ignored']} ignored, "
                f"{stats['auto_linked']} auto-linked, "
                f"{stats['auto_matched']} auto-matched, "
                f"{stats['needs_review']} need review, "
                f"{stats['no_match']} no match, "
                f"{stats['errors']} errors"
            )

            return {"status": "success", **stats}

        except Exception as e:
            logger.error(f"Steam sync failed for user {user_id}: {e}")
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e), **stats}


def _get_steam_sync_config(session: Session, user_id: str) -> Optional[UserSyncConfig]:
    """Get the user's Steam sync configuration."""
    stmt = select(UserSyncConfig).where(
        UserSyncConfig.user_id == user_id,
        UserSyncConfig.platform == "steam",
        UserSyncConfig.enabled,  # noqa: E712 - SQLAlchemy boolean column
    )
    return session.exec(stmt).first()


def _get_steam_credentials(user: User) -> Optional[Dict[str, str]]:
    """Extract Steam credentials from user preferences."""
    preferences = user.preferences or {}
    steam_config = preferences.get("steam", {})

    api_key = steam_config.get("web_api_key")
    steam_id = steam_config.get("steam_id")
    is_verified = steam_config.get("is_verified", False)

    if not api_key or not steam_id or not is_verified:
        return None

    return {"api_key": api_key, "steam_id": steam_id}


def _get_synced_steam_appids(session: Session, user_id: str) -> set[str]:
    """
    Get all Steam AppIDs already synced for this user.

    Checks UserGamePlatform for entries with:
    - storefront_id = 'steam'
    - store_game_id = Steam AppID
    """
    results = session.exec(
        select(UserGamePlatform.store_game_id)
        .join(UserGame)
        .where(
            UserGame.user_id == user_id,
            UserGamePlatform.storefront_id == STEAM_STOREFRONT_ID,
            UserGamePlatform.store_game_id.isnot(None),  # type: ignore
        )
    ).all()
    return {appid for appid in results if appid}


def _get_ignored_steam_appids(session: Session, user_id: str) -> set[str]:
    """
    Get all ignored Steam AppIDs for this user.

    Checks IgnoredExternalGame for entries with source = STEAM.
    """
    results = session.exec(
        select(IgnoredExternalGame.external_id).where(
            IgnoredExternalGame.user_id == user_id,
            IgnoredExternalGame.source == BackgroundJobSource.STEAM,
        )
    ).all()
    return set(results)


def _add_steam_platform(session: Session, user_game_id: str, steam_appid: str) -> None:
    """
    Add Steam platform association to an existing UserGame.

    Creates a UserGamePlatform entry linking the game to Steam storefront
    with the Steam AppID stored in store_game_id.
    """
    # Check if association already exists
    existing = session.exec(
        select(UserGamePlatform).where(
            UserGamePlatform.user_game_id == user_game_id,
            UserGamePlatform.storefront_id == STEAM_STOREFRONT_ID,
            UserGamePlatform.store_game_id == steam_appid,
        )
    ).first()

    if not existing:
        platform = UserGamePlatform(
            user_game_id=user_game_id,
            platform_id=PC_WINDOWS_PLATFORM_ID,
            storefront_id=STEAM_STOREFRONT_ID,
            store_game_id=steam_appid,
            store_url=f"https://store.steampowered.com/app/{steam_appid}",
            is_available=True,
        )
        session.add(platform)
        session.commit()
        logger.debug(f"Added Steam platform association for UserGame {user_game_id}, AppID {steam_appid}")


def _create_job_item(
    session: Session,
    job: Job,
    user_id: str,
    source_title: str,
    steam_appid: str,
    igdb_id: Optional[int],
    igdb_title: Optional[str],
    confidence: float,
    candidates: List,
) -> JobItem:
    """
    Create a JobItem for this sync item.

    Args:
        session: Database session
        job: The parent job
        user_id: User ID
        source_title: Original Steam game title
        steam_appid: Steam AppID
        igdb_id: Matched IGDB ID (if any)
        igdb_title: Matched IGDB title (if any)
        confidence: Match confidence score (0.0-1.0)
        candidates: List of IGDB candidate matches
    """
    # Convert candidates to serializable format
    serializable_candidates = []
    for candidate in candidates:
        if hasattr(candidate, "to_dict"):
            serializable_candidates.append(candidate.to_dict())
        elif isinstance(candidate, dict):
            serializable_candidates.append(candidate)
        else:
            # Convert dataclass or other object to dict
            try:
                serializable_candidates.append(candidate.__dict__)
            except AttributeError:
                logger.warning(f"Cannot serialize candidate: {candidate}")

    # If we have a matched IGDB ID but it's not in candidates, add it
    if igdb_id and igdb_title:
        candidate_ids = {c.get("igdb_id") for c in serializable_candidates}
        if igdb_id not in candidate_ids:
            serializable_candidates.insert(
                0,
                {
                    "igdb_id": igdb_id,
                    "name": igdb_title,
                    "similarity_score": confidence,
                },
            )

    source_metadata = {
        "steam_appid": steam_appid,
        "source": "steam",
        "data": {"title": source_title},
    }

    # Determine status based on match result
    if igdb_id and confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
        status = JobItemStatus.COMPLETED
    elif candidates:
        status = JobItemStatus.PENDING_REVIEW
    else:
        status = JobItemStatus.PENDING_REVIEW

    job_item = JobItem(
        job_id=job.id,
        user_id=user_id,
        item_key=f"steam_{steam_appid}",
        source_title=source_title,
        source_metadata_json=json.dumps(source_metadata),
        status=status,
        igdb_candidates_json=json.dumps(serializable_candidates),
        resolved_igdb_id=igdb_id if status == JobItemStatus.COMPLETED else None,
        match_confidence=confidence if confidence > 0 else None,
    )

    session.add(job_item)
    session.commit()

    logger.debug(
        f"Created JobItem for '{source_title}' "
        f"(AppID: {steam_appid}, status: {status}, confidence: {confidence:.2f}, "
        f"candidates: {len(serializable_candidates)})"
    )

    return job_item

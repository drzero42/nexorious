"""Steam library sync task.

Fetches user's Steam library and syncs games to the collection.
Uses the matching service to match games to IGDB and creates
review items for games that need manual matching.
"""

import logging
from datetime import datetime, timezone
from typing import Dict, Any, List, Optional

from sqlmodel import Session, select

from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH
from app.core.database import get_session_context
from app.models.job import (
    Job,
    BackgroundJobStatus,
    ReviewItem,
    ReviewItemStatus,
)
from app.models.user import User
from app.models.user_sync_config import UserSyncConfig
from app.models.steam_game import SteamGame
from app.models.user_game import UserGame
from app.models.game import Game
from app.services.steam import create_steam_service
from app.services.matching import MatchingService, MatchRequest, MatchStatus
from app.services.igdb import IGDBService

logger = logging.getLogger(__name__)


@broker.task(
    task_name="sync.steam_library",
    queue=QUEUE_HIGH,  # Default to high priority (can be overridden via labels)
)
async def sync_steam_library(
    job_id: str,
    user_id: str,
    is_scheduled: bool = False,
) -> Dict[str, Any]:
    """
    Sync a user's Steam library.

    This task:
    1. Fetches the user's Steam library via Steam API
    2. For each game, runs through the matching service
    3. Matched games are added automatically if auto_add is enabled
    4. Unmatched games are queued for review
    5. Detects and flags removals (games no longer in Steam library)
    6. Updates last_synced_at on completion

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
        "new_games": 0,
        "matched": 0,
        "needs_review": 0,
        "already_in_collection": 0,
        "removals_detected": 0,
        "errors": 0,
    }

    async with get_session_context() as session:
        # Get job and update status
        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            return {"status": "error", "error": "Job not found"}

        job.status = BackgroundJobStatus.PROCESSING
        job.started_at = datetime.now(timezone.utc)
        session.add(job)
        session.commit()

        try:
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
            steam_service = create_steam_service(steam_credentials["api_key"])

            # Fetch Steam library
            logger.info(f"Fetching Steam library for user {user_id}")
            steam_games_list = await steam_service.get_owned_games(
                steam_credentials["steam_id"]
            )
            stats["total_games"] = len(steam_games_list)

            # Update job progress
            job.progress_total = len(steam_games_list)
            session.add(job)
            session.commit()

            # Get existing Steam games in our DB
            existing_steam_games = _get_existing_steam_games(session, user_id)
            existing_appids = {g.steam_appid for g in existing_steam_games}
            current_appids = {g.appid for g in steam_games_list}

            # Create IGDB service for matching
            igdb_service = IGDBService()
            matching_service = MatchingService(session, igdb_service)

            # Process each game
            for i, steam_game in enumerate(steam_games_list):
                try:
                    result = await _process_steam_game(
                        session=session,
                        job=job,
                        user_id=user_id,
                        steam_game=steam_game,
                        existing_appids=existing_appids,
                        matching_service=matching_service,
                        auto_add=sync_config.auto_add,
                    )

                    # Update stats based on result
                    if result == "new":
                        stats["new_games"] += 1
                    elif result == "matched":
                        stats["matched"] += 1
                    elif result == "needs_review":
                        stats["needs_review"] += 1
                    elif result == "already_in_collection":
                        stats["already_in_collection"] += 1
                    elif result == "error":
                        stats["errors"] += 1

                except Exception as e:
                    logger.error(f"Error processing Steam game {steam_game.appid}: {e}")
                    stats["errors"] += 1

                # Update progress
                job.progress_current = i + 1
                session.add(job)
                session.commit()

            # Detect removals (games in DB but not in current Steam library)
            removed_appids = existing_appids - current_appids
            if removed_appids:
                stats["removals_detected"] = len(removed_appids)
                await _handle_removals(session, job, user_id, removed_appids)

            # Update sync config last_synced_at
            sync_config.last_synced_at = datetime.now(timezone.utc)
            session.add(sync_config)

            # Determine final job status
            if stats["needs_review"] > 0:
                job.status = BackgroundJobStatus.AWAITING_REVIEW
            else:
                job.status = BackgroundJobStatus.COMPLETED

            job.completed_at = datetime.now(timezone.utc)
            job.set_result_summary(stats)
            session.add(job)
            session.commit()

            logger.info(
                f"Steam sync completed for user {user_id}: "
                f"{stats['total_games']} total, {stats['matched']} matched, "
                f"{stats['needs_review']} need review, {stats['errors']} errors"
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


def _get_existing_steam_games(session: Session, user_id: str) -> List[SteamGame]:
    """Get all existing Steam games for a user."""
    stmt = select(SteamGame).where(SteamGame.user_id == user_id)
    return list(session.exec(stmt).all())


async def _process_steam_game(
    session: Session,
    job: Job,
    user_id: str,
    steam_game,
    existing_appids: set,
    matching_service: MatchingService,
    auto_add: bool,
) -> str:
    """
    Process a single Steam game during sync.

    Returns:
        Status string: "new", "matched", "needs_review", "already_in_collection", "error"
    """
    appid = steam_game.appid
    game_name = steam_game.name

    # Check if already in Steam games table
    if appid in existing_appids:
        # Game already imported, check if already in collection
        stmt = select(SteamGame).where(
            SteamGame.user_id == user_id,
            SteamGame.steam_appid == appid,
        )
        existing = session.exec(stmt).first()

        if existing and existing.igdb_id:
            # Check if already in user's collection
            user_game_stmt = select(UserGame).where(
                UserGame.user_id == user_id,
                UserGame.game_id == existing.igdb_id,
            )
            if session.exec(user_game_stmt).first():
                return "already_in_collection"

        return "already_in_collection"

    # New game - create SteamGame record
    new_steam_game = SteamGame(
        user_id=user_id,
        steam_appid=appid,
        game_name=game_name,
    )
    session.add(new_steam_game)
    session.commit()
    session.refresh(new_steam_game)

    # Attempt to match via matching service
    match_request = MatchRequest(
        source_title=game_name,
        source_platform="steam",
        platform_id=str(appid),
    )

    match_result = await matching_service.match_game(match_request)

    if match_result.status == MatchStatus.MATCHED and match_result.igdb_id is not None:
        # High confidence match
        new_steam_game.igdb_id = match_result.igdb_id
        new_steam_game.igdb_title = match_result.igdb_title
        session.add(new_steam_game)

        if auto_add:
            # Add to collection automatically
            await _add_to_collection(
                session, user_id, match_result.igdb_id, new_steam_game
            )
            return "matched"
        else:
            # Queue for review even though matched (user wants to review all)
            _create_review_item(
                session,
                job,
                user_id,
                game_name,
                appid,
                match_result,
            )
            return "needs_review"

    elif match_result.status == MatchStatus.NEEDS_REVIEW:
        # Low confidence or multiple candidates
        _create_review_item(
            session,
            job,
            user_id,
            game_name,
            appid,
            match_result,
        )
        return "needs_review"

    else:
        # No match found
        _create_review_item(
            session,
            job,
            user_id,
            game_name,
            appid,
            match_result,
        )
        return "needs_review"


async def _add_to_collection(
    session: Session,
    user_id: str,
    igdb_id: int,
    steam_game: SteamGame,
) -> None:
    """Add a matched game to the user's collection."""
    # Check if game exists in our games table
    game = session.get(Game, igdb_id)
    if not game:
        # Game not in our DB yet - skip adding to collection
        # The user will need to import this game separately
        logger.warning(f"IGDB game {igdb_id} not in database, skipping collection add")
        return

    # Check if already in collection
    stmt = select(UserGame).where(
        UserGame.user_id == user_id,
        UserGame.game_id == igdb_id,
    )
    if session.exec(stmt).first():
        return  # Already in collection

    # Add to collection
    user_game = UserGame(
        user_id=user_id,
        game_id=igdb_id,
    )
    session.add(user_game)
    session.commit()


def _create_review_item(
    session: Session,
    job: Job,
    user_id: str,
    game_name: str,
    appid: int,
    match_result,
) -> None:
    """Create a review item for a game that needs user decision."""
    source_metadata = {
        "steam_appid": appid,
        "platform": "steam",
    }
    if match_result.source_metadata:
        source_metadata.update(match_result.source_metadata)

    # Convert candidates to serializable format
    candidates = []
    if match_result.candidates:
        for c in match_result.candidates:
            candidates.append(c.to_dict() if hasattr(c, "to_dict") else dict(c))

    review_item = ReviewItem(
        job_id=job.id,
        user_id=user_id,
        source_title=game_name,
        status=ReviewItemStatus.PENDING,
    )
    review_item.set_source_metadata(source_metadata)
    review_item.set_igdb_candidates(candidates)

    session.add(review_item)
    session.commit()


async def _handle_removals(
    session: Session,
    job: Job,
    user_id: str,
    removed_appids: set,
) -> None:
    """Handle games that were removed from Steam library."""
    logger.info(f"Detected {len(removed_appids)} game removals for user {user_id}")

    for appid in removed_appids:
        # Get the Steam game record
        stmt = select(SteamGame).where(
            SteamGame.user_id == user_id,
            SteamGame.steam_appid == appid,
        )
        steam_game = session.exec(stmt).first()

        if steam_game:
            # Create a review item for the removal
            review_item = ReviewItem(
                job_id=job.id,
                user_id=user_id,
                source_title=steam_game.game_name,
                status=ReviewItemStatus.REMOVAL,
            )
            review_item.set_source_metadata({
                "steam_appid": appid,
                "platform": "steam",
                "igdb_id": steam_game.igdb_id,
                "action": "removal",
            })

            session.add(review_item)

    session.commit()

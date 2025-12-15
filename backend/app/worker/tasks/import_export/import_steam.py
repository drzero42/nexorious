"""Steam initial library import task.

Interactive initial import from Steam library.
Uses Steam AppID DB lookup first, then title-based IGDB search.
Matched games ready for import, unmatched queued for review.
Separate from sync - this is one-time initial library import.
"""

import logging
from datetime import datetime, timezone
from typing import Dict, Any, Optional

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
from app.models.game import Game
from app.models.user import User
from app.models.steam_game import SteamGame
from app.models.user_game import UserGame
from app.services.steam import create_steam_service
from app.services.igdb import IGDBService
from app.services.matching import MatchingService, MatchRequest, MatchStatus
from app.services.game_service import GameService

logger = logging.getLogger(__name__)


@broker.task(
    task_name="import.steam_library",
    queue=QUEUE_HIGH,
)
async def import_steam_library(job_id: str) -> Dict[str, Any]:
    """
    Import games from Steam library.

    This is an interactive import for initial library import that:
    1. Fetches the user's Steam library via Steam API
    2. For each game, looks up Steam AppID in DB first
    3. Falls back to title-based IGDB search
    4. High confidence matches are marked ready
    5. Low confidence matches are queued for user review

    This is distinct from Steam sync:
    - Import: One-time initial library import, all games need review
    - Sync: Ongoing synchronization, auto-adds matched games

    Args:
        job_id: The Job ID for tracking progress

    Returns:
        Dictionary with import statistics.
    """
    logger.info(f"Starting Steam library import (job: {job_id})")

    stats = {
        "total_games": 0,
        "matched": 0,
        "needs_review": 0,
        "no_match": 0,
        "already_in_collection": 0,
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
            # Get Steam ID from job result summary
            result_summary = job.get_result_summary()
            steam_id = result_summary.get("steam_id")

            if not steam_id:
                raise ValueError("No Steam ID found in job")

            # Get user and Steam credentials
            user = session.get(User, job.user_id)
            if not user:
                raise ValueError(f"User {job.user_id} not found")

            steam_credentials = _get_steam_credentials(user)
            if not steam_credentials:
                raise ValueError("Steam credentials not configured for this user")

            # Create Steam service
            steam_service = create_steam_service(steam_credentials["api_key"])

            # Fetch Steam library
            logger.info(f"Fetching Steam library for Steam ID: {steam_id}")
            steam_games_list = await steam_service.get_owned_games(steam_id)
            stats["total_games"] = len(steam_games_list)

            # Update job progress
            job.progress_total = len(steam_games_list)
            result_summary["total_games"] = len(steam_games_list)
            job.set_result_summary(result_summary)
            session.add(job)
            session.commit()

            if not steam_games_list:
                logger.warning(f"No games found in Steam library for {steam_id}")
                job.status = BackgroundJobStatus.COMPLETED
                job.completed_at = datetime.now(timezone.utc)
                job.set_result_summary({**result_summary, **stats})
                session.add(job)
                session.commit()
                return {"status": "success", **stats}

            # Create services
            igdb_service = IGDBService()
            matching_service = MatchingService(session, igdb_service)
            game_service = GameService(session, igdb_service)

            # Process each game
            for i, steam_game in enumerate(steam_games_list):
                try:
                    result = await _process_steam_game(
                        session=session,
                        job=job,
                        user_id=job.user_id,
                        steam_game=steam_game,
                        matching_service=matching_service,
                        game_service=game_service,
                    )

                    if result == "matched":
                        stats["matched"] += 1
                    elif result == "needs_review":
                        stats["needs_review"] += 1
                    elif result == "no_match":
                        stats["no_match"] += 1
                    elif result == "already_in_collection":
                        stats["already_in_collection"] += 1
                    else:
                        stats["errors"] += 1

                except Exception as e:
                    logger.error(
                        f"Error processing Steam game {steam_game.appid}: {e}",
                        exc_info=True,
                    )
                    stats["errors"] += 1

                # Update progress
                job.progress_current = i + 1
                session.add(job)
                session.commit()

            # Update result summary
            result_summary.update(stats)
            job.set_result_summary(result_summary)

            # Determine final status - Steam import always requires review
            if stats["matched"] > 0 or stats["needs_review"] > 0 or stats["no_match"] > 0:
                job.status = BackgroundJobStatus.AWAITING_REVIEW
            else:
                # Only already_in_collection - no review needed
                job.status = BackgroundJobStatus.COMPLETED

            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            logger.info(
                f"Steam import completed for job {job_id}: "
                f"{stats['matched']} matched, "
                f"{stats['needs_review']} need review, "
                f"{stats['no_match']} no match, "
                f"{stats['already_in_collection']} already in collection, "
                f"{stats['errors']} errors"
            )

            return {"status": "success", **stats}

        except Exception as e:
            logger.error(f"Steam import failed for job {job_id}: {e}")
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e), **stats}


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


async def _process_steam_game(
    session: Session,
    job: Job,
    user_id: str,
    steam_game: Any,
    matching_service: MatchingService,
    game_service: GameService,
) -> str:
    """
    Process a single Steam game during import.

    Returns:
        Status string: "matched", "needs_review", "no_match",
                      "already_in_collection", "error"
    """
    appid = steam_game.appid
    game_name = steam_game.name

    # Check if already in user's collection via existing SteamGame mapping
    existing_steam_game = session.exec(
        select(SteamGame).where(
            SteamGame.user_id == user_id,
            SteamGame.steam_appid == appid,
        )
    ).first()

    if existing_steam_game and existing_steam_game.igdb_id:
        # Check if already in collection
        existing_user_game = session.exec(
            select(UserGame).where(
                UserGame.user_id == user_id,
                UserGame.game_id == existing_steam_game.igdb_id,
            )
        ).first()

        if existing_user_game:
            return "already_in_collection"

    # Build source metadata
    source_metadata = {
        "steam_appid": appid,
        "platform": "steam",
        "playtime_forever": getattr(steam_game, "playtime_forever", 0),
        "playtime_2weeks": getattr(steam_game, "playtime_2weeks", 0),
    }

    # Create match request
    match_request = MatchRequest(
        source_title=game_name,
        source_platform="steam",
        platform_id=str(appid),
        source_metadata=source_metadata,
    )

    # Run through matching service
    match_result = await matching_service.match_game(match_request)

    # Record Steam game if not already present
    if not existing_steam_game:
        new_steam_game = SteamGame(
            user_id=user_id,
            steam_appid=appid,
            game_name=game_name,
        )
        session.add(new_steam_game)
        session.commit()
        session.refresh(new_steam_game)
        existing_steam_game = new_steam_game

    if match_result.status == MatchStatus.MATCHED and match_result.igdb_id is not None:
        # High confidence match
        existing_steam_game.igdb_id = match_result.igdb_id
        existing_steam_game.igdb_title = match_result.igdb_title
        session.add(existing_steam_game)
        session.commit()

        # Create review item with matched status
        _create_review_item(
            session=session,
            job=job,
            user_id=user_id,
            game_name=game_name,
            match_result=match_result,
            source_metadata=source_metadata,
            status=ReviewItemStatus.MATCHED,
        )
        return "matched"

    elif match_result.status == MatchStatus.NEEDS_REVIEW:
        # Low confidence or multiple candidates
        _create_review_item(
            session=session,
            job=job,
            user_id=user_id,
            game_name=game_name,
            match_result=match_result,
            source_metadata=source_metadata,
            status=ReviewItemStatus.PENDING,
        )
        return "needs_review"

    else:
        # No match found
        _create_review_item(
            session=session,
            job=job,
            user_id=user_id,
            game_name=game_name,
            match_result=match_result,
            source_metadata=source_metadata,
            status=ReviewItemStatus.PENDING,
        )
        return "no_match"


def _create_review_item(
    session: Session,
    job: Job,
    user_id: str,
    game_name: str,
    match_result: Any,
    source_metadata: Dict[str, Any],
    status: ReviewItemStatus,
) -> None:
    """Create a review item for a Steam game."""
    # Convert candidates to serializable format
    candidates = []
    if match_result.candidates:
        for c in match_result.candidates:
            candidates.append(c.to_dict() if hasattr(c, "to_dict") else dict(c))

    # Add match result info to metadata
    if match_result.igdb_id:
        source_metadata["matched_igdb_id"] = match_result.igdb_id
        source_metadata["matched_igdb_title"] = match_result.igdb_title
        source_metadata["match_confidence"] = match_result.confidence_score

    review_item = ReviewItem(
        job_id=job.id,
        user_id=user_id,
        source_title=game_name,
        status=status,
        resolved_igdb_id=match_result.igdb_id if status == ReviewItemStatus.MATCHED else None,
    )
    review_item.set_source_metadata(source_metadata)
    review_item.set_igdb_candidates(candidates)

    session.add(review_item)
    session.commit()

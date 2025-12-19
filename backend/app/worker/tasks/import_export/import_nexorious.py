"""Nexorious JSON import task.

Non-interactive import of trusted Nexorious JSON exports.
Validates schema and export_version, looks up IGDB IDs,
fetches metadata if not cached, and restores user data.
"""

import logging
from datetime import datetime, timezone, date
from typing import Dict, Any, Optional, List
from decimal import Decimal

from sqlmodel import Session, select
from sqlalchemy.exc import IntegrityError

from app.worker.locking import acquire_job_lock, release_job_lock
from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH
from app.core.database import get_session_context
from app.models.job import Job, BackgroundJobStatus
from app.models.game import Game
from app.models.user_game import UserGame, UserGamePlatform, PlayStatus, OwnershipStatus
from app.models.platform import Platform, Storefront
from app.models.tag import Tag, UserGameTag
from app.services.igdb import IGDBService
from app.services.game_service import GameService

logger = logging.getLogger(__name__)

# Supported export versions
SUPPORTED_EXPORT_VERSIONS = ["1.0", "1.1"]


@broker.task(
    task_name="import.nexorious_json",
    queue=QUEUE_HIGH,
)
async def import_nexorious_json(job_id: str) -> Dict[str, Any]:
    """
    Import games from a Nexorious JSON export.

    This is a non-interactive import that trusts IGDB IDs in the export.
    The task:
    1. Validates the JSON schema and export version
    2. Looks up IGDB IDs and fetches metadata if not cached
    3. Restores user data (play status, rating, notes, tags, platforms)

    No review is required for this import type.

    Args:
        job_id: The Job ID for tracking progress

    Returns:
        Dictionary with import statistics.
    """
    logger.info(f"Starting Nexorious JSON import (job: {job_id})")

    stats = {
        "total_games": 0,
        "imported": 0,
        "already_in_collection": 0,
        "skipped_invalid": 0,
        "skipped_no_igdb_id": 0,
        "errors": 0,
    }

    async with get_session_context() as session:
        # Try to acquire advisory lock - prevents duplicate execution
        if not acquire_job_lock(session, job_id):
            logger.info(f"Job {job_id} already being processed by another worker")
            return {"status": "skipped", "reason": "duplicate_execution"}

        # Get job first (outside try so exception handler can access it)
        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            release_job_lock(session, job_id)
            return {"status": "error", "error": "Job not found"}

        try:
            # Update job status
            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            # Get import data from job
            result_summary = job.get_result_summary()
            import_data = result_summary.get("_import_data", {})

            if not import_data:
                raise ValueError("No import data found in job")

            # Validate export version
            export_version = import_data.get("export_version", "1.0")
            if export_version not in SUPPORTED_EXPORT_VERSIONS:
                logger.warning(
                    f"Unknown export version {export_version}, "
                    f"proceeding with best-effort import"
                )

            games = import_data.get("games", [])
            stats["total_games"] = len(games)
            job.progress_total = len(games)
            session.add(job)
            session.commit()

            # Create services
            igdb_service = IGDBService()
            game_service = GameService(session, igdb_service)

            # Process each game
            for i, game_data in enumerate(games):
                try:
                    result = await _process_nexorious_game(
                        session=session,
                        game_service=game_service,
                        user_id=job.user_id,
                        game_data=game_data,
                    )

                    if result == "imported":
                        stats["imported"] += 1
                    elif result == "already_in_collection":
                        stats["already_in_collection"] += 1
                    elif result == "skipped_no_igdb_id":
                        stats["skipped_no_igdb_id"] += 1
                    elif result == "skipped_invalid":
                        stats["skipped_invalid"] += 1
                    else:
                        stats["errors"] += 1

                except Exception as e:
                    logger.error(
                        f"Error processing Nexorious game: {e}",
                        exc_info=True,
                    )
                    stats["errors"] += 1

                # Update progress
                job.progress_current = i + 1
                session.add(job)
                session.commit()

            # Clear import data from result summary to save space
            result_summary.pop("_import_data", None)
            result_summary.update(stats)
            job.set_result_summary(result_summary)

            # Nexorious imports don't need review - complete directly
            job.status = BackgroundJobStatus.COMPLETED
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            logger.info(
                f"Nexorious import completed for job {job_id}: "
                f"{stats['imported']} imported, "
                f"{stats['already_in_collection']} already in collection, "
                f"{stats['skipped_no_igdb_id']} skipped (no IGDB ID), "
                f"{stats['errors']} errors"
            )

            return {"status": "success", **stats}

        except Exception as e:
            logger.error(f"Nexorious import failed for job {job_id}: {e}")
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e), **stats}
        finally:
            release_job_lock(session, job_id)


async def _process_nexorious_game(
    session: Session,
    game_service: GameService,
    user_id: str,
    game_data: Dict[str, Any],
) -> str:
    """
    Process a single game from Nexorious export.

    Returns:
        Status string: "imported", "already_in_collection",
                      "skipped_no_igdb_id", "skipped_invalid", "error"
    """
    # Validate required fields
    title = game_data.get("title")
    if not title:
        logger.warning("Skipping game without title")
        return "skipped_invalid"

    igdb_id = game_data.get("igdb_id")
    if not igdb_id:
        logger.warning(f"Skipping game '{title}' without IGDB ID")
        return "skipped_no_igdb_id"

    try:
        igdb_id = int(igdb_id)
    except (ValueError, TypeError):
        logger.warning(f"Skipping game '{title}' with invalid IGDB ID: {igdb_id}")
        return "skipped_invalid"

    # Check if game already in user's collection
    existing_user_game = session.exec(
        select(UserGame).where(
            UserGame.user_id == user_id,
            UserGame.game_id == igdb_id,
        )
    ).first()

    if existing_user_game:
        logger.debug(f"Game '{title}' already in collection")
        return "already_in_collection"

    # Ensure game exists in our games table (fetch from IGDB if needed)
    game = session.get(Game, igdb_id)
    if not game:
        try:
            game = await game_service.create_or_update_game_from_igdb(igdb_id)
        except Exception as e:
            logger.error(f"Failed to fetch game '{title}' from IGDB: {e}")
            return "error"

    # Create UserGame with user data from export
    user_game = UserGame(
        user_id=user_id,
        game_id=igdb_id,
        play_status=_map_play_status(game_data.get("play_status")),
        ownership_status=_map_ownership_status(game_data.get("ownership_status")),
        personal_rating=_parse_rating(game_data.get("personal_rating")),
        is_loved=game_data.get("is_loved", False),
        hours_played=game_data.get("hours_played", 0),
        personal_notes=game_data.get("personal_notes"),
        acquired_date=_parse_date(game_data.get("acquired_date")),
    )
    session.add(user_game)
    try:
        session.commit()
    except IntegrityError:
        session.rollback()
        logger.info(f"Game '{title}' already in collection (caught by constraint)")
        return "already_in_collection"
    session.refresh(user_game)

    # Import platforms if present
    platforms_data = game_data.get("platforms", [])
    if platforms_data:
        await _import_platforms(session, user_game, platforms_data)

    # Import tags if present
    tags_data = game_data.get("tags", [])
    if tags_data:
        await _import_tags(session, user_game, user_id, tags_data)

    logger.debug(f"Imported game '{title}' (IGDB ID: {igdb_id})")
    return "imported"


async def _import_platforms(
    session: Session,
    user_game: UserGame,
    platforms_data: List[Dict[str, Any]],
) -> None:
    """Import platform associations for a user game."""
    for platform_data in platforms_data:
        platform_name = platform_data.get("platform_name") or platform_data.get("name")
        storefront_name = platform_data.get("storefront_name") or platform_data.get(
            "storefront"
        )

        # Try to resolve platform
        platform_id = None
        if platform_name:
            platform = session.exec(
                select(Platform).where(Platform.name == platform_name)
            ).first()
            if platform:
                platform_id = platform.id

        # Try to resolve storefront
        storefront_id = None
        if storefront_name:
            storefront = session.exec(
                select(Storefront).where(Storefront.name == storefront_name)
            ).first()
            if storefront:
                storefront_id = storefront.id

        # Create platform association
        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=platform_id,
            storefront_id=storefront_id,
            store_game_id=platform_data.get("store_game_id"),
            store_url=platform_data.get("store_url"),
            is_available=platform_data.get("is_available", True),
            original_platform_name=platform_name if not platform_id else None,
        )
        session.add(user_game_platform)

    session.commit()


async def _import_tags(
    session: Session,
    user_game: UserGame,
    user_id: str,
    tags_data: List[str],
) -> None:
    """Import tags for a user game."""
    for tag_name in tags_data:
        if not tag_name or not isinstance(tag_name, str):
            continue

        tag_name = tag_name.strip()
        if not tag_name:
            continue

        # Find or create tag for user
        tag = session.exec(
            select(Tag).where(Tag.user_id == user_id, Tag.name == tag_name)
        ).first()

        if not tag:
            tag = Tag(user_id=user_id, name=tag_name)
            session.add(tag)
            session.commit()
            session.refresh(tag)

        # Create tag association if it doesn't exist
        existing_assoc = session.exec(
            select(UserGameTag).where(
                UserGameTag.user_game_id == user_game.id,
                UserGameTag.tag_id == tag.id,
            )
        ).first()

        if not existing_assoc:
            user_game_tag = UserGameTag(
                user_game_id=user_game.id,
                tag_id=tag.id,
            )
            session.add(user_game_tag)

    session.commit()


def _map_play_status(status: Optional[str]) -> PlayStatus:
    """Map export play status to PlayStatus enum."""
    if not status:
        return PlayStatus.NOT_STARTED

    status_lower = status.lower().replace("-", "_").replace(" ", "_")
    status_mapping = {
        "not_started": PlayStatus.NOT_STARTED,
        "in_progress": PlayStatus.IN_PROGRESS,
        "completed": PlayStatus.COMPLETED,
        "mastered": PlayStatus.MASTERED,
        "dominated": PlayStatus.DOMINATED,
        "shelved": PlayStatus.SHELVED,
        "dropped": PlayStatus.DROPPED,
        "replay": PlayStatus.REPLAY,
        # Common aliases
        "playing": PlayStatus.IN_PROGRESS,
        "finished": PlayStatus.COMPLETED,
        "100%": PlayStatus.MASTERED,
        "abandoned": PlayStatus.DROPPED,
        "backlog": PlayStatus.NOT_STARTED,
    }
    return status_mapping.get(status_lower, PlayStatus.NOT_STARTED)


def _map_ownership_status(status: Optional[str]) -> OwnershipStatus:
    """Map export ownership status to OwnershipStatus enum."""
    if not status:
        return OwnershipStatus.OWNED

    status_lower = status.lower().replace("-", "_").replace(" ", "_")
    status_mapping = {
        "owned": OwnershipStatus.OWNED,
        "borrowed": OwnershipStatus.BORROWED,
        "rented": OwnershipStatus.RENTED,
        "subscription": OwnershipStatus.SUBSCRIPTION,
        "no_longer_owned": OwnershipStatus.NO_LONGER_OWNED,
        # Common aliases
        "gamepass": OwnershipStatus.SUBSCRIPTION,
        "game_pass": OwnershipStatus.SUBSCRIPTION,
        "ps_plus": OwnershipStatus.SUBSCRIPTION,
        "ps+": OwnershipStatus.SUBSCRIPTION,
        "sold": OwnershipStatus.NO_LONGER_OWNED,
    }
    return status_mapping.get(status_lower, OwnershipStatus.OWNED)


def _parse_rating(rating: Any) -> Optional[Decimal]:
    """Parse rating value to Decimal."""
    if rating is None:
        return None

    try:
        rating_decimal = Decimal(str(rating))
        # Clamp to valid range (0.0 - 10.0)
        if rating_decimal < 0:
            return Decimal("0.0")
        if rating_decimal > 10:
            return Decimal("10.0")
        return rating_decimal.quantize(Decimal("0.1"))
    except Exception:
        return None


def _parse_date(date_str: Any) -> Optional[date]:
    """Parse date string to date object."""
    if not date_str:
        return None

    if isinstance(date_str, date):
        return date_str

    try:
        # Handle ISO format (YYYY-MM-DD)
        if isinstance(date_str, str) and len(date_str) >= 10:
            return date.fromisoformat(date_str[:10])
    except (ValueError, TypeError):
        pass

    return None

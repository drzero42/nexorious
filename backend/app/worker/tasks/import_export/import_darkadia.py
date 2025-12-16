"""Darkadia CSV import task.

Interactive import requiring title-based matching.
Parses CSV, runs each game through matching service.
Matched games marked ready, unmatched queued for review.
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
from app.models.game import Game
from app.services.igdb import IGDBService
from app.services.matching import MatchingService, MatchRequest, MatchStatus
from app.services.game_service import GameService

logger = logging.getLogger(__name__)

# Column name mappings for Darkadia CSV
COLUMN_MAPPINGS = {
    "name": ["Name", "name", "Title", "title", "Game", "game"],
    "platform": ["Platform", "platform", "Console", "console", "System", "system"],
    "status": ["Status", "status", "Play Status", "play_status"],
    "rating": ["Rating", "rating", "Score", "score", "My Rating", "my_rating"],
    "notes": ["Notes", "notes", "Comments", "comments"],
    "hours": ["Hours", "hours", "Time Played", "time_played", "Playtime", "playtime"],
    "completion": [
        "Completion",
        "completion",
        "Progress",
        "progress",
        "Completed",
        "completed",
    ],
    "date_added": [
        "Date Added",
        "date_added",
        "Added",
        "added",
        "Acquired",
        "acquired",
    ],
    "release_year": [
        "Release Year",
        "release_year",
        "Year",
        "year",
        "Release",
        "release",
    ],
}


def parse_darkadia_platform(platform_str: str) -> Dict[str, Optional[str]]:
    """
    Parse Darkadia platform string into components.

    Darkadia format: "Platform|Storefront|MediaType"
    Examples:
        "PC|Steam|Digital" -> {"platform": "PC", "storefront": "Steam", "media_type": "Digital"}
        "PlayStation 4" -> {"platform": "PlayStation 4", "storefront": None, "media_type": None}

    Args:
        platform_str: Raw platform string from CSV

    Returns:
        Dict with platform, storefront, and media_type keys
    """
    if not platform_str or not platform_str.strip():
        return {"platform": None, "storefront": None, "media_type": None}

    parts = [p.strip() for p in platform_str.split("|")]

    return {
        "platform": parts[0] if len(parts) > 0 and parts[0] else None,
        "storefront": parts[1] if len(parts) > 1 and parts[1] else None,
        "media_type": parts[2] if len(parts) > 2 and parts[2] else None,
    }


@broker.task(
    task_name="import.darkadia_csv",
    queue=QUEUE_HIGH,
)
async def import_darkadia_csv(job_id: str) -> Dict[str, Any]:
    """
    Import games from a Darkadia CSV export.

    This is an interactive import that:
    1. Parses the CSV file
    2. Runs each game through the matching service
    3. High confidence matches are marked ready
    4. Low confidence matches are queued for user review

    If any games need review, job status changes to AWAITING_REVIEW.

    Args:
        job_id: The Job ID for tracking progress

    Returns:
        Dictionary with import statistics.
    """
    logger.info(f"Starting Darkadia CSV import (job: {job_id})")

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
            # Get import data from job
            result_summary = job.get_result_summary()
            import_rows = result_summary.get("_import_rows", [])
            columns = result_summary.get("columns", [])

            if not import_rows:
                raise ValueError("No import data found in job")

            stats["total_games"] = len(import_rows)
            job.progress_total = len(import_rows)
            session.add(job)
            session.commit()

            # Create column mapping
            column_map = _create_column_map(columns)

            # Create services
            igdb_service = IGDBService()
            matching_service = MatchingService(session, igdb_service)
            game_service = GameService(session, igdb_service)

            # Process each row
            for i, row in enumerate(import_rows):
                try:
                    result = await _process_darkadia_row(
                        session=session,
                        job=job,
                        user_id=job.user_id,
                        row=row,
                        column_map=column_map,
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
                        f"Error processing Darkadia row: {e}",
                        exc_info=True,
                    )
                    stats["errors"] += 1

                # Update progress
                job.progress_current = i + 1
                session.add(job)
                session.commit()

            # Clear import data from result summary to save space
            result_summary.pop("_import_rows", None)
            result_summary.update(stats)
            job.set_result_summary(result_summary)

            # Determine final status
            if stats["needs_review"] > 0 or stats["no_match"] > 0:
                job.status = BackgroundJobStatus.AWAITING_REVIEW
            else:
                job.status = BackgroundJobStatus.COMPLETED

            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            logger.info(
                f"Darkadia import completed for job {job_id}: "
                f"{stats['matched']} matched, "
                f"{stats['needs_review']} need review, "
                f"{stats['no_match']} no match, "
                f"{stats['errors']} errors"
            )

            return {"status": "success", **stats}

        except Exception as e:
            logger.error(f"Darkadia import failed for job {job_id}: {e}")
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e), **stats}


def _create_column_map(columns: List[str]) -> Dict[str, Optional[str]]:
    """Create a mapping from standard field names to actual column names."""
    column_map: Dict[str, Optional[str]] = {}

    for field_name, possible_names in COLUMN_MAPPINGS.items():
        column_map[field_name] = None
        for possible_name in possible_names:
            if possible_name in columns:
                column_map[field_name] = possible_name
                break

    return column_map


def _get_row_value(
    row: Dict[str, Any],
    column_map: Dict[str, Optional[str]],
    field_name: str,
) -> Optional[str]:
    """Get a value from a row using the column map."""
    column_name = column_map.get(field_name)
    if not column_name:
        return None

    value = row.get(column_name)
    if value is None:
        return None

    # Convert to string and strip whitespace
    value_str = str(value).strip()
    return value_str if value_str else None


async def _process_darkadia_row(
    session: Session,
    job: Job,
    user_id: str,
    row: Dict[str, Any],
    column_map: Dict[str, Optional[str]],
    matching_service: MatchingService,
    game_service: GameService,
) -> str:
    """
    Process a single row from Darkadia CSV.

    Returns:
        Status string: "matched", "needs_review", "no_match",
                      "already_in_collection", "error"
    """
    # Get game name
    game_name = _get_row_value(row, column_map, "name")
    if not game_name:
        logger.warning("Skipping row without game name")
        return "error"

    # Get optional fields for matching hints
    platform_raw = _get_row_value(row, column_map, "platform")
    release_year = _get_row_value(row, column_map, "release_year")

    # Parse platform string into components
    platforms: List[str] = []
    storefronts: List[str] = []
    if platform_raw:
        parsed = parse_darkadia_platform(platform_raw)
        if parsed["platform"]:
            platforms.append(parsed["platform"])
        if parsed["storefront"]:
            storefronts.append(parsed["storefront"])

    # Build source metadata
    source_metadata = {
        "source": "darkadia",
        "platforms": platforms,
        "storefronts": storefronts,
        "platform_raw": platform_raw,  # Keep original for reference
        "release_year": release_year,
        "status": _get_row_value(row, column_map, "status"),
        "rating": _get_row_value(row, column_map, "rating"),
        "notes": _get_row_value(row, column_map, "notes"),
        "hours": _get_row_value(row, column_map, "hours"),
        "completion": _get_row_value(row, column_map, "completion"),
        "date_added": _get_row_value(row, column_map, "date_added"),
    }
    # Remove None values
    source_metadata = {k: v for k, v in source_metadata.items() if v is not None}

    # Create match request
    match_request = MatchRequest(
        source_title=game_name,
        source_platform="darkadia",
        release_year=int(release_year) if release_year and release_year.isdigit() else None,
        source_metadata=source_metadata,
    )

    # Run through matching service
    match_result = await matching_service.match_game(match_request)

    if match_result.status == MatchStatus.MATCHED and match_result.igdb_id is not None:
        # High confidence match - check if already in collection
        existing = session.exec(
            select(Game).where(Game.id == match_result.igdb_id)
        ).first()

        if existing:
            # Check if already in user's collection
            from app.models.user_game import UserGame

            existing_user_game = session.exec(
                select(UserGame).where(
                    UserGame.user_id == user_id,
                    UserGame.game_id == match_result.igdb_id,
                )
            ).first()

            if existing_user_game:
                return "already_in_collection"

        # Create review item with matched status (ready for import)
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
        # Low confidence or multiple candidates - needs review
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
        # No match found - queue for review with no candidates
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
    """Create a review item for a game."""
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

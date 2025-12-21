"""Export task for collection and wishlist exports.

Exports user game collections to JSON or CSV format using the unified Job model.
Files are stored temporarily and can be downloaded via the export API endpoints.
"""

import csv
import logging
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, Any, List

from sqlmodel import Session, select

from app.worker.locking import acquire_job_lock, release_job_lock
from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH
from app.core.database import get_session_context
from app.core.config import settings
from app.models.job import Job, BackgroundJobStatus
from app.models.user_game import UserGame
from app.models.wishlist import Wishlist
from app.schemas.export import (
    ExportFormat,
    ExportGameData,
    ExportPlatformData,
    ExportTagData,
    ExportWishlistItem,
    NexoriousExportData,
    CsvExportRow,
)

logger = logging.getLogger(__name__)

# Export format version
EXPORT_VERSION = "1.2"


def _get_exports_dir() -> Path:
    """Get exports directory from settings, creating it if needed."""
    exports_dir = Path(getattr(settings, "storage_path", "storage")) / "exports"
    exports_dir.mkdir(parents=True, exist_ok=True)
    return exports_dir


def _build_user_games_query(
    session: Session,
    user_id: str,
) -> List[UserGame]:
    """Build and execute query for all user games."""
    query = (
        select(UserGame)
        .where(UserGame.user_id == user_id)
        .order_by(UserGame.created_at)  # pyrefly: ignore[bad-argument-type]
    )

    return list(session.exec(query).all())


def _build_wishlist_query(
    session: Session,
    user_id: str,
) -> List[Wishlist]:
    """Build and execute query for all user wishlist items."""
    query = (
        select(Wishlist)
        .where(Wishlist.user_id == user_id)
        .order_by(Wishlist.created_at)  # pyrefly: ignore[bad-argument-type]
    )

    return list(session.exec(query).all())


def _wishlist_to_export_data(
    wishlist_item: Wishlist,
) -> ExportWishlistItem:
    """Convert a Wishlist item to export format."""
    release_year = None
    if wishlist_item.game.release_date:
        release_year = wishlist_item.game.release_date.year

    return ExportWishlistItem(
        igdb_id=wishlist_item.game.id,
        title=wishlist_item.game.title,
        release_year=release_year,
        added_at=wishlist_item.created_at,
    )


def _user_game_to_export_data(
    session: Session,
    user_game: UserGame,
) -> ExportGameData:
    """Convert a UserGame to export format."""
    # Get platforms
    platforms_data: List[ExportPlatformData] = []
    for ugp in user_game.platforms:
        platform_data = ExportPlatformData(
            platform_id=ugp.platform_id,
            platform_name=ugp.platform.name if ugp.platform else ugp.original_platform_name,
            storefront_id=ugp.storefront_id,
            storefront_name=ugp.storefront.name if ugp.storefront else None,
            store_game_id=ugp.store_game_id,
            store_url=ugp.store_url,
            is_available=ugp.is_available,
        )
        platforms_data.append(platform_data)

    # Get tags
    tags_data: List[ExportTagData] = []
    for ugt in user_game.tags:
        if ugt.tag:
            tag_data = ExportTagData(
                name=ugt.tag.name,
                color=ugt.tag.color,
            )
            tags_data.append(tag_data)

    # Build export data
    release_year = None
    if user_game.game.release_date:
        release_year = user_game.game.release_date.year

    return ExportGameData(
        igdb_id=user_game.game.id,
        title=user_game.game.title,
        release_year=release_year,
        ownership_status=user_game.ownership_status.value,
        play_status=user_game.play_status.value,
        personal_rating=float(user_game.personal_rating) if user_game.personal_rating else None,
        is_loved=user_game.is_loved,
        hours_played=user_game.hours_played,
        personal_notes=user_game.personal_notes,
        acquired_date=user_game.acquired_date,
        platforms=platforms_data,
        tags=tags_data,
        created_at=user_game.created_at,
        updated_at=user_game.updated_at,
    )


def _user_game_to_csv_row(
    session: Session,
    user_game: UserGame,
) -> CsvExportRow:
    """Convert a UserGame to CSV row format."""
    # Collect platform and storefront names
    platform_names: List[str] = []
    storefront_names: List[str] = []
    for ugp in user_game.platforms:
        if ugp.platform:
            platform_names.append(ugp.platform.name)
        elif ugp.original_platform_name:
            platform_names.append(ugp.original_platform_name)
        if ugp.storefront:
            storefront_names.append(ugp.storefront.name)

    # Collect tag names
    tag_names: List[str] = []
    for ugt in user_game.tags:
        if ugt.tag:
            tag_names.append(ugt.tag.name)

    release_year = None
    if user_game.game.release_date:
        release_year = user_game.game.release_date.year

    return CsvExportRow(
        igdb_id=user_game.game.id,
        title=user_game.game.title,
        release_year=release_year,
        ownership_status=user_game.ownership_status.value,
        play_status=user_game.play_status.value,
        personal_rating=float(user_game.personal_rating) if user_game.personal_rating else None,
        is_loved=user_game.is_loved,
        hours_played=user_game.hours_played,
        personal_notes=user_game.personal_notes,
        acquired_date=user_game.acquired_date.isoformat() if user_game.acquired_date else None,
        platforms=", ".join(sorted(set(platform_names))),
        storefronts=", ".join(sorted(set(storefront_names))),
        tags=", ".join(sorted(tag_names)),
        created_at=user_game.created_at.isoformat(),
        updated_at=user_game.updated_at.isoformat(),
    )


def _write_json_export(
    export_data: NexoriousExportData,
    file_path: Path,
) -> int:
    """Write JSON export to file. Returns file size in bytes."""
    json_content = export_data.model_dump_json(indent=2)
    file_path.write_text(json_content, encoding="utf-8")
    return file_path.stat().st_size


def _write_csv_export(
    rows: List[CsvExportRow],
    file_path: Path,
) -> int:
    """Write CSV export to file. Returns file size in bytes."""
    if not rows:
        file_path.write_text("", encoding="utf-8")
        return 0

    fieldnames = list(CsvExportRow.model_fields.keys())

    with open(file_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow(row.model_dump())

    return file_path.stat().st_size


@broker.task(
    task_name="export.collection",
    queue=QUEUE_HIGH,
)
async def export_collection(
    job_id: str,
    export_format: str,
) -> Dict[str, Any]:
    """
    Export user's game collection.

    Creates a JSON or CSV file containing the user's games with all
    associated metadata. The file is stored in the exports directory
    and the job's file_path is updated to point to it.

    Args:
        job_id: The Job ID for tracking progress
        export_format: "json" or "csv"

    Returns:
        Dictionary with export statistics.
    """
    logger.info(
        f"Starting export (job: {job_id}, format: {export_format})"
    )

    stats: Dict[str, Any] = {
        "exported_games": 0,
        "exported_wishlist": 0,
        "file_size_bytes": 0,
        "format": export_format,
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
            # Query user games
            user_games = _build_user_games_query(session, job.user_id)
            total_games = len(user_games)

            job.progress_total = total_games
            session.add(job)
            session.commit()

            # Generate filename
            exports_dir = _get_exports_dir()
            timestamp = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%S")
            extension = "csv" if export_format == ExportFormat.CSV.value else "json"
            filename = f"{job.user_id}_{timestamp}.{extension}"
            file_path = exports_dir / filename

            if export_format == ExportFormat.CSV.value:
                # CSV export
                csv_rows: List[CsvExportRow] = []
                for i, user_game in enumerate(user_games):
                    csv_row = _user_game_to_csv_row(session, user_game)
                    csv_rows.append(csv_row)

                    # Update progress
                    job.progress_current = i + 1
                    session.add(job)
                    if i % 50 == 0:  # Commit every 50 items
                        session.commit()

                session.commit()
                file_size = _write_csv_export(csv_rows, file_path)

            else:
                # JSON export
                games_data: List[ExportGameData] = []
                for i, user_game in enumerate(user_games):
                    game_data = _user_game_to_export_data(session, user_game)
                    games_data.append(game_data)

                    # Update progress
                    job.progress_current = i + 1
                    session.add(job)
                    if i % 50 == 0:  # Commit every 50 items
                        session.commit()

                session.commit()

                # Query and process wishlist items
                wishlist_items = _build_wishlist_query(session, job.user_id)
                wishlist_data: List[ExportWishlistItem] = []
                for wishlist_item in wishlist_items:
                    wishlist_export = _wishlist_to_export_data(wishlist_item)
                    wishlist_data.append(wishlist_export)

                stats["exported_wishlist"] = len(wishlist_data)

                # Calculate stats for export
                export_stats = _calculate_export_stats(games_data)

                # Build full export data
                export_data = NexoriousExportData(
                    export_version=EXPORT_VERSION,
                    export_date=datetime.now(timezone.utc),
                    user_id=job.user_id,
                    total_games=total_games,
                    total_wishlist=len(wishlist_data),
                    export_stats=export_stats,
                    games=games_data,
                    wishlist=wishlist_data,
                )

                file_size = _write_json_export(export_data, file_path)

            # Update job with results
            stats["exported_games"] = total_games
            stats["file_size_bytes"] = file_size

            result_summary = job.get_result_summary()
            result_summary.update(stats)
            job.set_result_summary(result_summary)

            job.file_path = str(file_path)
            job.status = BackgroundJobStatus.COMPLETED
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            logger.info(
                f"Export completed for job {job_id}: "
                f"{total_games} games, {file_size} bytes"
            )

            return stats

        except Exception as e:
            logger.error(f"Export failed for job {job_id}: {e}", exc_info=True)

            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            return {
                "status": "error",
                "error": str(e),
                **stats,
            }
        finally:
            release_job_lock(session, job_id)


def _calculate_export_stats(games_data: List[ExportGameData]) -> Dict[str, Any]:
    """Calculate statistics about the exported games."""
    stats: Dict[str, Any] = {
        "total_games": len(games_data),
        "by_play_status": {},
        "by_ownership_status": {},
        "games_with_ratings": 0,
        "games_with_notes": 0,
        "games_with_tags": 0,
        "loved_games": 0,
        "total_hours_played": 0,
    }

    for game in games_data:
        # Count by play status
        status = game.play_status
        stats["by_play_status"][status] = stats["by_play_status"].get(status, 0) + 1

        # Count by ownership status
        ownership = game.ownership_status
        stats["by_ownership_status"][ownership] = (
            stats["by_ownership_status"].get(ownership, 0) + 1
        )

        # Count games with ratings
        if game.personal_rating is not None:
            stats["games_with_ratings"] += 1

        # Count games with notes
        if game.personal_notes:
            stats["games_with_notes"] += 1

        # Count games with tags
        if game.tags:
            stats["games_with_tags"] += 1

        # Count loved games
        if game.is_loved:
            stats["loved_games"] += 1

        # Sum hours played
        stats["total_hours_played"] += game.hours_played

    return stats

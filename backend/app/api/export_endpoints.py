"""
Export API endpoints for collection data export.

Provides endpoints for exporting user collections to JSON and CSV formats.
Exports are tracked using the unified Job model and files are stored
temporarily for download.
"""

from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.responses import FileResponse
from sqlmodel import Session, select, func
from typing import Annotated
from datetime import datetime, timezone, timedelta
from pathlib import Path
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..core.config import settings
from ..models.user import User
from ..models.user_game import UserGame, OwnershipStatus
from ..models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus
from ..schemas.export import ExportFormat, ExportScope, ExportJobCreatedResponse
from ..worker.tasks.import_export.export import export_collection as export_collection_task

router = APIRouter(prefix="/export", tags=["Export"])
logger = logging.getLogger(__name__)

# Export file retention (24 hours)
EXPORT_RETENTION_HOURS = 24


def _get_exports_dir() -> Path:
    """Get exports directory from settings, creating it if needed."""
    exports_dir = Path(getattr(settings, "storage_path", "storage")) / "exports"
    exports_dir.mkdir(parents=True, exist_ok=True)
    return exports_dir


@router.post("/collection/json", response_model=ExportJobCreatedResponse)
async def export_collection_json(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ExportJobCreatedResponse:
    """
    Start a JSON export of the user's game collection.

    Creates a background job that exports all games in the collection
    to a JSON file with full metadata. The export includes:
    - IGDB IDs for reliable re-import
    - All user data (ratings, notes, play status, etc.)
    - Platform and storefront associations
    - Tags

    The exported file can be downloaded once the job completes.
    Files are retained for 24 hours before automatic deletion.
    """
    logger.info(f"User {current_user.id} requesting JSON export of collection")

    # Count games to estimate export size
    game_count = session.exec(
        select(func.count()).select_from(UserGame).where(UserGame.user_id == current_user.id)
    ).one()

    if game_count == 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="No games in collection to export.",
        )

    # Create export job
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.EXPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PENDING,
        progress_total=game_count,
    )
    job.set_result_summary({
        "format": ExportFormat.JSON.value,
        "scope": ExportScope.COLLECTION.value,
        "estimated_items": game_count,
    })

    session.add(job)
    session.commit()
    session.refresh(job)

    # Schedule the export task
    await export_collection_task.kiq(
        job_id=job.id,
        export_format=ExportFormat.JSON.value,
        export_scope=ExportScope.COLLECTION.value,
    )

    logger.info(f"Created export job {job.id} for user {current_user.id}")

    return ExportJobCreatedResponse(
        job_id=job.id,
        status=job.status.value,
        message="Export job created. Check job status for progress.",
        estimated_items=game_count,
    )


@router.post("/collection/csv", response_model=ExportJobCreatedResponse)
async def export_collection_csv(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ExportJobCreatedResponse:
    """
    Start a CSV export of the user's game collection.

    Creates a background job that exports all games in the collection
    to a CSV file. The CSV format is useful for spreadsheet applications
    but has some limitations:
    - Platform/storefront data is comma-separated in columns
    - Personal notes may be truncated if very long
    - Not recommended for re-import (use JSON instead)

    The exported file can be downloaded once the job completes.
    Files are retained for 24 hours before automatic deletion.
    """
    logger.info(f"User {current_user.id} requesting CSV export of collection")

    # Count games to estimate export size
    game_count = session.exec(
        select(func.count()).select_from(UserGame).where(UserGame.user_id == current_user.id)
    ).one()

    if game_count == 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="No games in collection to export.",
        )

    # Create export job
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.EXPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PENDING,
        progress_total=game_count,
    )
    job.set_result_summary({
        "format": ExportFormat.CSV.value,
        "scope": ExportScope.COLLECTION.value,
        "estimated_items": game_count,
    })

    session.add(job)
    session.commit()
    session.refresh(job)

    # Schedule the export task
    await export_collection_task.kiq(
        job_id=job.id,
        export_format=ExportFormat.CSV.value,
        export_scope=ExportScope.COLLECTION.value,
    )

    logger.info(f"Created CSV export job {job.id} for user {current_user.id}")

    return ExportJobCreatedResponse(
        job_id=job.id,
        status=job.status.value,
        message="Export job created. Check job status for progress.",
        estimated_items=game_count,
    )


@router.post("/wishlist/json", response_model=ExportJobCreatedResponse)
async def export_wishlist_json(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ExportJobCreatedResponse:
    """
    Start a JSON export of the user's wishlist.

    Creates a background job that exports all games marked as wishlist
    (ownership_status = 'no_longer_owned' without platforms) to JSON.
    This is primarily for backup purposes.

    The exported file can be downloaded once the job completes.
    """
    logger.info(f"User {current_user.id} requesting JSON export of wishlist")

    # For wishlist, we'd filter by some criteria (depends on implementation)
    # For now, treating wishlist as a separate export scope
    game_count = session.exec(
        select(func.count())
        .select_from(UserGame)
        .where(
            UserGame.user_id == current_user.id,
            UserGame.ownership_status == OwnershipStatus.NO_LONGER_OWNED,
        )
    ).one()

    if game_count == 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="No wishlist games to export.",
        )

    # Create export job
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.EXPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PENDING,
        progress_total=game_count,
    )
    job.set_result_summary({
        "format": ExportFormat.JSON.value,
        "scope": ExportScope.WISHLIST.value,
        "estimated_items": game_count,
    })

    session.add(job)
    session.commit()
    session.refresh(job)

    # Schedule the export task
    await export_collection_task.kiq(
        job_id=job.id,
        export_format=ExportFormat.JSON.value,
        export_scope=ExportScope.WISHLIST.value,
    )

    logger.info(f"Created wishlist export job {job.id} for user {current_user.id}")

    return ExportJobCreatedResponse(
        job_id=job.id,
        status=job.status.value,
        message="Wishlist export job created. Check job status for progress.",
        estimated_items=game_count,
    )


@router.post("/wishlist/csv", response_model=ExportJobCreatedResponse)
async def export_wishlist_csv(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ExportJobCreatedResponse:
    """
    Start a CSV export of the user's wishlist.

    Similar to JSON wishlist export but in CSV format for spreadsheet use.
    """
    logger.info(f"User {current_user.id} requesting CSV export of wishlist")

    game_count = session.exec(
        select(func.count())
        .select_from(UserGame)
        .where(
            UserGame.user_id == current_user.id,
            UserGame.ownership_status == OwnershipStatus.NO_LONGER_OWNED,
        )
    ).one()

    if game_count == 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="No wishlist games to export.",
        )

    # Create export job
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.EXPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PENDING,
        progress_total=game_count,
    )
    job.set_result_summary({
        "format": ExportFormat.CSV.value,
        "scope": ExportScope.WISHLIST.value,
        "estimated_items": game_count,
    })

    session.add(job)
    session.commit()
    session.refresh(job)

    # Schedule the export task
    await export_collection_task.kiq(
        job_id=job.id,
        export_format=ExportFormat.CSV.value,
        export_scope=ExportScope.WISHLIST.value,
    )

    logger.info(f"Created CSV wishlist export job {job.id} for user {current_user.id}")

    return ExportJobCreatedResponse(
        job_id=job.id,
        status=job.status.value,
        message="Wishlist export job created. Check job status for progress.",
        estimated_items=game_count,
    )


@router.get("/{job_id}/download")
async def download_export(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> FileResponse:
    """
    Download a completed export file.

    Returns the exported file for download. Only available for completed
    export jobs that belong to the current user. Files are available
    for 24 hours after creation.
    """
    logger.debug(f"User {current_user.id} downloading export {job_id}")

    job = session.get(Job, job_id)

    if not job:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Export job not found.",
        )

    # Authorization check
    if job.user_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Export job not found.",
        )

    # Check job type
    if job.job_type != BackgroundJobType.EXPORT:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Job is not an export job.",
        )

    # Check job status
    if job.status != BackgroundJobStatus.COMPLETED:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Export not ready. Current status: {job.status.value}",
        )

    # Check file path
    if not job.file_path:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Export file path not set.",
        )

    file_path = Path(job.file_path)
    if not file_path.exists():
        raise HTTPException(
            status_code=status.HTTP_410_GONE,
            detail="Export file has expired or been deleted.",
        )

    # Check expiration (24 hours from completion)
    if job.completed_at:
        # Ensure completed_at is timezone-aware for comparison
        completed_at = job.completed_at
        if completed_at.tzinfo is None:
            completed_at = completed_at.replace(tzinfo=timezone.utc)
        expiration = completed_at + timedelta(hours=EXPORT_RETENTION_HOURS)
        if datetime.now(timezone.utc) > expiration:
            # Delete the file
            file_path.unlink(missing_ok=True)
            raise HTTPException(
                status_code=status.HTTP_410_GONE,
                detail="Export file has expired.",
            )

    # Determine content type and filename
    result_summary = job.get_result_summary()
    export_format = result_summary.get("format", "json")
    export_scope = result_summary.get("scope", "collection")

    if export_format == "csv":
        media_type = "text/csv"
        filename = f"nexorious_{export_scope}_{job.created_at.strftime('%Y%m%d_%H%M%S')}.csv"
    else:
        media_type = "application/json"
        filename = f"nexorious_{export_scope}_{job.created_at.strftime('%Y%m%d_%H%M%S')}.json"

    logger.info(f"User {current_user.id} downloading export {job_id}: {filename}")

    return FileResponse(
        path=str(file_path),
        media_type=media_type,
        filename=filename,
    )

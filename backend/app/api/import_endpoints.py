"""
Import endpoints for triggering background import jobs.

Provides endpoints for:
- POST /import/nexorious - Upload Nexorious JSON export (non-interactive)

All endpoints create unified Job records and return job_id for tracking.
Uses high priority queue for user-initiated imports.
"""

from fastapi import APIRouter, Depends, HTTPException, status, UploadFile, File
from pydantic import BaseModel, Field
from sqlmodel import Session, select
from typing import Annotated, Optional
import logging
import json

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
)
from ..worker.tasks.import_export import (
    import_nexorious_json as import_nexorious_task,
)

router = APIRouter(prefix="/import", tags=["Import Jobs"])
logger = logging.getLogger(__name__)

# Maximum file sizes
MAX_JSON_SIZE = 50 * 1024 * 1024  # 50MB for Nexorious JSON


class ImportJobCreatedResponse:
    """Response for import job creation."""

    def __init__(
        self,
        job_id: str,
        source: str,
        status: str,
        message: str,
        total_items: Optional[int] = None
    ):
        self.job_id = job_id
        self.source = source
        self.status = status
        self.message = message
        self.total_items = total_items


class ImportJobCreatedResponseModel(BaseModel):
    """Response model for import job creation."""

    job_id: str = Field(..., description="ID of the created import job")
    source: str = Field(..., description="Import source (nexorious)")
    status: str = Field(..., description="Initial job status")
    message: str = Field(..., description="Success message")
    total_items: Optional[int] = Field(None, description="Total items to import (if known)")


def _check_active_import(
    session: Session,
    user_id: str,
    source: BackgroundJobSource
) -> Optional[Job]:
    """Check if user has an active import job for this source."""
    stmt = select(Job).where(
        Job.user_id == user_id,
        Job.job_type == BackgroundJobType.IMPORT,
        Job.source == source,
        Job.status.in_([  # pyrefly: ignore[missing-attribute]
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
            BackgroundJobStatus.AWAITING_REVIEW,
        ])
    )
    return session.exec(stmt).first()


@router.post("/nexorious", response_model=ImportJobCreatedResponseModel)
async def import_nexorious_json(
    file: Annotated[UploadFile, File(description="Nexorious JSON export file")],
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> ImportJobCreatedResponseModel:
    """
    Import games from a Nexorious JSON export file.

    This is a non-interactive import that trusts the IGDB IDs in the export.
    Creates a background job that will:
    1. Validate the JSON schema and export version
    2. Look up IGDB IDs and fetch metadata if not cached
    3. Restore user data (play status, rating, notes, tags, platforms)

    No review is required for this import type.

    Returns the job_id for tracking import progress via the jobs API.
    """
    # Check for existing active import
    existing_job = _check_active_import(
        session, current_user.id, BackgroundJobSource.NEXORIOUS
    )
    if existing_job:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"Import already in progress. Job ID: {existing_job.id}"
        )

    # Validate file type
    if file.content_type and file.content_type not in [
        "application/json",
        "text/json",
        "application/octet-stream"
    ]:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Invalid file type: {file.content_type}. Expected JSON file."
        )

    # Read and validate file size
    content = await file.read()
    if len(content) > MAX_JSON_SIZE:
        raise HTTPException(
            status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE,
            detail=f"File too large. Maximum size is {MAX_JSON_SIZE // (1024*1024)}MB"
        )

    # Parse JSON to validate and count items
    try:
        data = json.loads(content.decode("utf-8"))
    except json.JSONDecodeError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Invalid JSON file: {str(e)}"
        )
    except UnicodeDecodeError:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="File encoding error. Expected UTF-8 encoded JSON."
        )

    # Validate required structure
    if not isinstance(data, dict):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid Nexorious export format. Expected JSON object."
        )

    # Check for required fields
    if "games" not in data:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid Nexorious export. Missing 'games' field."
        )

    games = data.get("games", [])
    if not isinstance(games, list):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid Nexorious export. 'games' must be an array."
        )

    total_items = len(games)

    if total_items == 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="No games found in export file."
        )

    logger.info(
        f"User {current_user.id} uploading Nexorious export with {total_items} games"
    )

    # Create job record
    job = Job(
        user_id=current_user.id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PENDING,
        priority=BackgroundJobPriority.HIGH,
        progress_total=total_items,
    )

    # Store the JSON data in result_summary for the worker to process
    job.set_result_summary({
        "file_name": file.filename,
        "file_size": len(content),
        "total_games": total_items,
        "export_version": data.get("export_version"),
        "export_date": data.get("export_date"),
        # Store raw data for worker (will be cleared after processing)
        "_import_data": data,
    })

    session.add(job)
    session.commit()
    session.refresh(job)

    # Enqueue the import task
    task_result = await import_nexorious_task.kiq(job.id)
    job.taskiq_task_id = task_result.task_id
    session.add(job)
    session.commit()

    logger.info(f"Created Nexorious import job {job.id} for user {current_user.id}")

    return ImportJobCreatedResponseModel(
        job_id=job.id,
        source="nexorious",
        status=job.status.value,
        message=f"Import job created. Processing {total_items} games.",
        total_items=total_items,
    )



"""
Core import management endpoints for cross-source operations.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, desc, col
from typing import Annotated, Optional
from datetime import datetime, timezone
import logging

from ...core.database import get_session
from ...core.security import get_current_user
from ...models.user import User
from ...models.job import Job, BackgroundJobType, BackgroundJobStatus
from ...schemas.import_schemas import (
    ImportSourceInfo,
    ImportSourcesResponse,
    ImportJobsListResponse,
    ImportJobResponse,
    ImportJobCancelResponse,
    ImportHistoryResponse
)

router = APIRouter()
logger = logging.getLogger(__name__)


@router.get("/sources", response_model=ImportSourcesResponse)
async def list_import_sources(
    current_user: Annotated[User, Depends(get_current_user)]
) -> ImportSourcesResponse:
    """List all available import sources and their status."""
    
    # TODO: This will be dynamically populated as we add more sources
    # For now, we'll start with Steam as the first implementation
    source_data = [
        {
            "name": "steam",
            "display_name": "Steam",
            "description": "Import games from your Steam library",
            "icon": "steam",
            "available": True,
            "configured": False,  # TODO: Check user's Steam config
            "status": "available"
        }
        # Future sources:
        # {
        #     "name": "epic",
        #     "display_name": "Epic Games Store",
        #     "description": "Import games from Epic Games Store",
        #     "icon": "epic",
        #     "available": False,
        #     "configured": False,
        #     "status": "coming_soon"
        # }
    ]
    
    # Convert dictionaries to ImportSourceInfo instances
    sources = [ImportSourceInfo(**source) for source in source_data]
    
    return ImportSourcesResponse(sources=sources)


@router.get("/jobs", response_model=ImportJobsListResponse)
async def list_import_jobs(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    offset: int = Query(default=0, ge=0),
    limit: int = Query(default=50, ge=1, le=100),
    source: Optional[str] = Query(default=None),
    status: Optional[str] = Query(default=None)
) -> ImportJobsListResponse:
    """List import jobs across all sources with filtering."""

    # Build query - filter by job_type=IMPORT
    query = select(Job).where(
        Job.user_id == current_user.id,
        Job.job_type == BackgroundJobType.IMPORT
    )

    # Apply filters
    if source:
        query = query.where(Job.source == source)
    if status:
        query = query.where(Job.status == status)

    # Get total count
    count_query = select(Job).where(
        Job.user_id == current_user.id,
        Job.job_type == BackgroundJobType.IMPORT
    )
    if source:
        count_query = count_query.where(Job.source == source)
    if status:
        count_query = count_query.where(Job.status == status)

    total = len(session.exec(count_query).all())

    # Apply pagination and ordering
    query = query.order_by(desc(Job.created_at)).offset(offset).limit(limit)
    jobs = session.exec(query).all()

    return ImportJobsListResponse(
        jobs=[ImportJobResponse.from_job(job) for job in jobs],
        total=total,
        offset=offset,
        limit=limit
    )


@router.get("/jobs/{job_id}", response_model=ImportJobResponse)
async def get_import_job(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> ImportJobResponse:
    """Get specific import job details."""

    job = session.exec(
        select(Job).where(
            Job.id == job_id,
            Job.user_id == current_user.id,
            Job.job_type == BackgroundJobType.IMPORT
        )
    ).first()

    if not job:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Import job not found"
        )

    return ImportJobResponse.from_job(job)


@router.post("/jobs/{job_id}/cancel", response_model=ImportJobCancelResponse)
async def cancel_import_job(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> ImportJobCancelResponse:
    """Cancel a running import job."""

    job = session.exec(
        select(Job).where(
            Job.id == job_id,
            Job.user_id == current_user.id,
            Job.job_type == BackgroundJobType.IMPORT
        )
    ).first()

    if not job:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Import job not found"
        )

    # Only allow cancelling jobs that are in cancellable states
    cancellable_statuses = [
        BackgroundJobStatus.PENDING,
        BackgroundJobStatus.PROCESSING,
        BackgroundJobStatus.AWAITING_REVIEW,
        BackgroundJobStatus.READY,
        BackgroundJobStatus.FINALIZING,
    ]
    if job.status not in cancellable_statuses:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot cancel job with status: {job.status.value}"
        )

    # Update job status
    job.status = BackgroundJobStatus.CANCELLED
    job.completed_at = datetime.now(timezone.utc)

    session.add(job)
    session.commit()
    session.refresh(job)

    logger.info(f"Import job {job_id} cancelled by user {current_user.id}")

    return ImportJobCancelResponse(
        message="Import job cancelled successfully",
        job_id=job_id,
        cancelled=True
    )


@router.get("/history", response_model=ImportHistoryResponse)
async def get_import_history(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    offset: int = Query(default=0, ge=0),
    limit: int = Query(default=50, ge=1, le=100),
    source: Optional[str] = Query(default=None)
) -> ImportHistoryResponse:
    """Get paginated import history."""

    # Terminal statuses for history
    terminal_statuses = [
        BackgroundJobStatus.COMPLETED,
        BackgroundJobStatus.FAILED,
        BackgroundJobStatus.CANCELLED
    ]

    # Build query for completed jobs only
    query = select(Job).where(
        Job.user_id == current_user.id,
        Job.job_type == BackgroundJobType.IMPORT,
        col(Job.status).in_(terminal_statuses)
    )

    if source:
        query = query.where(Job.source == source)

    # Get total count
    count_query = select(Job).where(
        Job.user_id == current_user.id,
        Job.job_type == BackgroundJobType.IMPORT,
        col(Job.status).in_(terminal_statuses)
    )
    if source:
        count_query = count_query.where(Job.source == source)

    total = len(session.exec(count_query).all())

    # Apply pagination and ordering
    query = query.order_by(desc(Job.completed_at)).offset(offset).limit(limit)
    jobs = session.exec(query).all()

    return ImportHistoryResponse(
        history=[ImportJobResponse.from_job(job) for job in jobs],
        total=total,
        offset=offset,
        limit=limit
    )
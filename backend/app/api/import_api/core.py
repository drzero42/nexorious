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
from ...models.import_job import ImportJob, ImportStatus
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
    
    # Build query
    query = select(ImportJob).where(ImportJob.user_id == current_user.id)
    
    # Apply filters
    if source:
        query = query.where(ImportJob.source == source)
    if status:
        query = query.where(ImportJob.status == status)
    
    # Get total count
    count_query = select(ImportJob).where(ImportJob.user_id == current_user.id)
    if source:
        count_query = count_query.where(ImportJob.source == source) 
    if status:
        count_query = count_query.where(ImportJob.status == status)
    
    total = len(session.exec(count_query).all())
    
    # Apply pagination and ordering
    query = query.order_by(desc(ImportJob.created_at)).offset(offset).limit(limit)
    jobs = session.exec(query).all()
    
    return ImportJobsListResponse(
        jobs=[
            ImportJobResponse(
                id=job.id,
                source=job.source or job.import_type.value,
                job_type=job.job_type.value if job.job_type else "legacy",
                status=job.status.value,
                progress=job.progress,
                total_items=job.total_items or job.total_records,
                processed_items=job.processed_items or job.processed_records,
                successful_items=job.successful_items,
                failed_items=job.failed_items or job.failed_records,
                created_at=job.created_at,
                started_at=job.started_at,
                completed_at=job.completed_at,
                error_message=job.error_message,
                metadata=job.get_metadata()
            ) for job in jobs
        ],
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
        select(ImportJob).where(
            ImportJob.id == job_id,
            ImportJob.user_id == current_user.id
        )
    ).first()
    
    if not job:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Import job not found"
        )
    
    return ImportJobResponse(
        id=job.id,
        source=job.source or job.import_type.value,
        job_type=job.job_type.value if job.job_type else "legacy",
        status=job.status.value,
        progress=job.progress,
        total_items=job.total_items or job.total_records,
        processed_items=job.processed_items or job.processed_records,
        successful_items=job.successful_items,
        failed_items=job.failed_items or job.failed_records,
        created_at=job.created_at,
        started_at=job.started_at,
        completed_at=job.completed_at,
        error_message=job.error_message,
        metadata=job.get_metadata()
    )


@router.post("/jobs/{job_id}/cancel", response_model=ImportJobCancelResponse)
async def cancel_import_job(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> ImportJobCancelResponse:
    """Cancel a running import job."""
    
    job = session.exec(
        select(ImportJob).where(
            ImportJob.id == job_id,
            ImportJob.user_id == current_user.id
        )
    ).first()
    
    if not job:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Import job not found"
        )
    
    if job.status not in [ImportStatus.PENDING, ImportStatus.PROCESSING, ImportStatus.RUNNING]:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot cancel job with status: {job.status.value}"
        )
    
    # Update job status
    job.status = ImportStatus.CANCELLED
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
    
    # Build query for completed jobs only
    query = select(ImportJob).where(
        ImportJob.user_id == current_user.id,
        col(ImportJob.status).in_([ImportStatus.COMPLETED, ImportStatus.FAILED, ImportStatus.CANCELLED])
    )
    
    if source:
        query = query.where(ImportJob.source == source)
    
    # Get total count
    count_query = select(ImportJob).where(
        ImportJob.user_id == current_user.id,
        col(ImportJob.status).in_([ImportStatus.COMPLETED, ImportStatus.FAILED, ImportStatus.CANCELLED])
    )
    if source:
        count_query = count_query.where(ImportJob.source == source)
    
    total = len(session.exec(count_query).all())
    
    # Apply pagination and ordering
    query = query.order_by(desc(ImportJob.completed_at)).offset(offset).limit(limit)
    jobs = session.exec(query).all()
    
    return ImportHistoryResponse(
        history=[
            ImportJobResponse(
                id=job.id,
                source=job.source or job.import_type.value,
                job_type=job.job_type.value if job.job_type else "legacy",
                status=job.status.value,
                progress=job.progress,
                total_items=job.total_items or job.total_records,
                processed_items=job.processed_items or job.processed_records,
                successful_items=job.successful_items,
                failed_items=job.failed_items or job.failed_records,
                created_at=job.created_at,
                started_at=job.started_at,
                completed_at=job.completed_at,
                error_message=job.error_message,
                metadata=job.get_metadata()
            ) for job in jobs
        ],
        total=total,
        offset=offset,
        limit=limit
    )
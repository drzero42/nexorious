"""
Job management endpoints for background task tracking.

Provides endpoints for listing, viewing, cancelling, and deleting
background jobs (sync, import, export operations).
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, func, col
from sqlalchemy import update as sa_update
from typing import Annotated, Optional
from datetime import datetime, timezone
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.job import (
    Job,
    JobItem,
    JobItemStatus,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
)
from ..schemas.job import (
    JobResponse,
    JobListResponse,
    JobCancelResponse,
    JobDeleteResponse,
    JobType,
    JobSource,
    JobStatus,
)
from ..schemas.job_item import JobItemListResponse, JobItemResponse
from ..schemas.import_schemas import JobsSummaryResponse
from ..services.job_service import get_job_progress, get_derived_job_status
from ..utils.sqlalchemy_typed import desc

router = APIRouter(prefix="/jobs", tags=["Jobs"])
logger = logging.getLogger(__name__)


def _job_to_response(job: Job, session: Session) -> JobResponse:
    """Convert a Job model to JobResponse with computed fields."""
    # Get derived status and progress from job items
    derived_status = get_derived_job_status(session, job)
    progress = get_job_progress(session, job.id)

    return JobResponse(
        id=job.id,
        user_id=job.user_id,
        job_type=JobType(job.job_type.value),
        source=JobSource(job.source.value),
        status=JobStatus(derived_status.value),
        priority=job.priority,
        progress=progress,
        total_items=progress.total,
        error_message=job.error_message,
        file_path=job.file_path,
        created_at=job.created_at,
        started_at=job.started_at,
        completed_at=job.completed_at,
        is_terminal=job.is_terminal,
        duration_seconds=job.duration_seconds,
    )


@router.get("/", response_model=JobListResponse)
async def list_jobs(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page"),
    job_type: Optional[JobType] = Query(default=None, description="Filter by job type"),
    source: Optional[JobSource] = Query(default=None, description="Filter by source"),
    status: Optional[JobStatus] = Query(default=None, description="Filter by status"),
    sort_by: str = Query(default="created_at", description="Sort field"),
    sort_order: str = Query(
        default="desc", pattern="^(asc|desc)$", description="Sort order"
    ),
):
    """
    List jobs for the current user.

    Supports filtering by job type, source, and status.
    Results are paginated and sorted by creation date (newest first) by default.
    """
    logger.debug(
        f"Listing jobs for user {current_user.id}: type={job_type}, source={source}, status={status}"
    )

    # Build query - only show jobs for the current user
    query = select(Job).where(Job.user_id == current_user.id)

    # Apply filters
    if job_type:
        query = query.where(Job.job_type == BackgroundJobType(job_type.value))

    if source:
        query = query.where(Job.source == BackgroundJobSource(source.value))

    if status:
        query = query.where(Job.status == BackgroundJobStatus(status.value))

    # Get total count before pagination
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()

    # Apply sorting
    sort_field = getattr(Job, sort_by, Job.created_at)
    if sort_order == "desc":
        query = query.order_by(desc(col(sort_field)))
    else:
        query = query.order_by(col(sort_field))

    # Apply pagination
    offset = (page - 1) * per_page
    jobs = session.exec(query.offset(offset).limit(per_page)).all()

    # Calculate pages
    pages = (total + per_page - 1) // per_page if total > 0 else 1

    # Convert to response models
    job_responses = [_job_to_response(job, session) for job in jobs]

    logger.info(f"Returning {len(job_responses)} jobs for user {current_user.id}")

    return JobListResponse(
        jobs=job_responses,
        total=total,
        page=page,
        per_page=per_page,
        pages=pages,
    )


@router.get("/summary", response_model=JobsSummaryResponse)
async def get_jobs_summary(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> JobsSummaryResponse:
    """
    Get summary counts of running and failed jobs for the current user.

    Used by the frontend sidebar to display badges showing the number of
    active and failed jobs.
    """
    logger.debug(f"Getting jobs summary for user {current_user.id}")

    # Count running jobs (processing)
    running_result = session.exec(
        select(func.count()).select_from(Job).where(
            Job.user_id == current_user.id,
            Job.status == BackgroundJobStatus.PROCESSING
        )
    )
    running_count = running_result.one()

    # Count failed jobs
    failed_result = session.exec(
        select(func.count()).select_from(Job).where(
            Job.user_id == current_user.id,
            Job.status == BackgroundJobStatus.FAILED
        )
    )
    failed_count = failed_result.one()

    logger.debug(f"Jobs summary for user {current_user.id}: running={running_count}, failed={failed_count}")

    return JobsSummaryResponse(
        running_count=running_count,
        failed_count=failed_count
    )


@router.get("/{job_id}", response_model=JobResponse)
async def get_job(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Get details for a specific job.

    Returns the full job information including progress, results, and review item counts.
    Users can only view their own jobs.
    """
    logger.debug(f"Getting job {job_id} for user {current_user.id}")

    job = session.get(Job, job_id)

    if not job:
        logger.warning(f"Job {job_id} not found")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Authorization check - users can only view their own jobs
    if job.user_id != current_user.id:
        logger.warning(
            f"User {current_user.id} attempted to access job {job_id} owned by {job.user_id}"
        )
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    return _job_to_response(job, session)


@router.get("/{job_id}/items", response_model=JobItemListResponse)
async def list_job_items(
    job_id: str,
    status: JobItemStatus | None = None,
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """List items for a job with optional status filter."""
    # Verify job belongs to user
    job = session.get(Job, job_id)
    if not job or job.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job not found")

    query = select(JobItem).where(JobItem.job_id == job_id)
    if status:
        query = query.where(JobItem.status == status)

    # Count total
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()

    # Paginate
    query = query.offset((page - 1) * page_size).limit(page_size)
    items = session.exec(query).all()

    return JobItemListResponse(
        items=[JobItemResponse.model_validate(item) for item in items],
        total=total,
        page=page,
        page_size=page_size,
    )


@router.post("/{job_id}/cancel", response_model=JobCancelResponse)
async def cancel_job(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Cancel an in-progress job.

    Only jobs that are not in a terminal state (completed, failed, cancelled)
    can be cancelled. Users can only cancel their own jobs.
    """
    logger.info(f"User {current_user.id} requesting to cancel job {job_id}")

    job = session.get(Job, job_id)

    if not job:
        logger.warning(f"Job {job_id} not found for cancellation")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Authorization check
    if job.user_id != current_user.id:
        logger.warning(
            f"User {current_user.id} attempted to cancel job {job_id} owned by {job.user_id}"
        )
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Check if job can be cancelled
    if job.is_terminal:
        logger.warning(
            f"Cannot cancel job {job_id} - already in terminal state {job.status}"
        )
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot cancel job - already in terminal state: {job.status.value}",
        )

    # Update job status to cancelled
    previous_status = job.status
    job.status = BackgroundJobStatus.CANCELLED
    job.completed_at = datetime.now(timezone.utc)

    session.commit()
    session.refresh(job)

    logger.info(
        f"Job {job_id} cancelled by user {current_user.id} (was {previous_status.value})"
    )

    return JobCancelResponse(
        success=True,
        message=f"Job cancelled successfully (was {previous_status.value})",
        job=_job_to_response(job, session),
    )


@router.delete("/{job_id}", response_model=JobDeleteResponse)
async def delete_job(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Delete a job record.

    Jobs can only be deleted if they are in a terminal state (completed, failed, cancelled).
    Deleting a job also deletes all associated review items.
    Users can only delete their own jobs.
    """
    logger.info(f"User {current_user.id} requesting to delete job {job_id}")

    job = session.get(Job, job_id)

    if not job:
        logger.warning(f"Job {job_id} not found for deletion")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Authorization check
    if job.user_id != current_user.id:
        logger.warning(
            f"User {current_user.id} attempted to delete job {job_id} owned by {job.user_id}"
        )
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Only allow deleting terminal jobs
    if not job.is_terminal:
        logger.warning(
            f"Cannot delete job {job_id} - not in terminal state ({job.status})"
        )
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot delete job - must be in terminal state (completed, failed, or cancelled). Current status: {job.status.value}",
        )

    # Delete the job (cascade will delete job items)
    session.delete(job)
    session.commit()

    logger.info(f"Job {job_id} deleted by user {current_user.id}")

    return JobDeleteResponse(
        success=True,
        message="Job deleted successfully",
        deleted_job_id=job_id,
    )



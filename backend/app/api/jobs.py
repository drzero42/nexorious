"""
Job management endpoints for background task tracking.

Provides endpoints for listing, viewing, cancelling, and deleting
background jobs (sync, import, export operations).
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, func, col
from sqlalchemy import update as sa_update
from sqlmodel import select as sql_select
from typing import Annotated, Optional
from datetime import datetime, timezone, timedelta
import json
import logging

from ..core.database import get_session
from ..worker.tasks.import_export.process_import_item import enqueue_import_task
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
    RetryFailedResponse,
    JobType,
    JobSource,
    JobStatus,
    JobItemSummary,
    RecentJobDetail,
    RecentJobsResponse,
)
from ..schemas.job_item import JobItemListResponse, JobItemResponse
from ..schemas.import_schemas import JobsSummaryResponse
from ..services.job_service import get_job_progress, get_derived_job_status, retry_failed_items
from ..utils.sqlalchemy_typed import desc, not_in
from pydantic import BaseModel


class PendingReviewCountResponse(BaseModel):
    """Response for pending review count endpoint."""

    pending_review_count: int


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


@router.get("/pending-review-count", response_model=PendingReviewCountResponse)
async def get_pending_review_count(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
) -> PendingReviewCountResponse:
    """
    Get total count of items needing review across all jobs.

    Used by the frontend navigation to show a badge with pending items.
    """
    count = session.exec(
        select(func.count())
        .select_from(JobItem)
        .where(JobItem.user_id == current_user.id)
        .where(JobItem.status == JobItemStatus.PENDING_REVIEW)
    ).one()

    return PendingReviewCountResponse(pending_review_count=count)


@router.get("/active/{job_type}", response_model=JobResponse | None)
async def get_active_job(
    job_type: JobType,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Get the active or most recent job for a specific type.

    Returns:
    - An in-progress job if one exists (pending/processing)
    - Otherwise, the most recently completed job (within last hour)
    - None if no relevant job exists

    This allows the frontend to show download buttons for recently
    completed export jobs.
    """
    # First, check for any in-progress job
    in_progress_job = session.exec(
        select(Job)
        .where(Job.user_id == current_user.id)
        .where(Job.job_type == BackgroundJobType(job_type.value))
        .where(
            not_in(
                Job.status,
                [
                    BackgroundJobStatus.COMPLETED,
                    BackgroundJobStatus.FAILED,
                    BackgroundJobStatus.CANCELLED,
                ],
            )
        )
        .order_by(desc(Job.created_at))
    ).first()

    if in_progress_job:
        return _job_to_response(in_progress_job, session)

    # No in-progress job, check for recently completed job (within last hour)
    one_hour_ago = datetime.now(timezone.utc) - timedelta(hours=1)
    recent_job = session.exec(
        select(Job)
        .where(Job.user_id == current_user.id)
        .where(Job.job_type == BackgroundJobType(job_type.value))
        .where(Job.completed_at.is_not(None))  # type: ignore[union-attr]
        .where(Job.completed_at >= one_hour_ago)  # type: ignore[operator]
        .order_by(desc(Job.completed_at))
    ).first()

    if recent_job:
        return _job_to_response(recent_job, session)

    return None


@router.get("/recent/{source}", response_model=RecentJobsResponse)
async def get_recent_jobs(
    source: str,
    limit: int = Query(default=5, ge=1, le=20),
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """
    Get recent completed sync jobs for a platform with item details.

    Returns the most recent completed jobs with their items grouped by status.
    """
    # Map source string to enum
    try:
        job_source = BackgroundJobSource(source)
    except ValueError:
        raise HTTPException(status_code=400, detail=f"Invalid source: {source}")

    # Fetch recent completed jobs
    jobs = session.exec(
        select(Job)
        .where(
            Job.user_id == current_user.id,
            Job.source == job_source,
            Job.job_type == BackgroundJobType.SYNC,
            Job.status == BackgroundJobStatus.COMPLETED,
        )
        .order_by(col(Job.completed_at).desc())
        .limit(limit)
    ).all()

    result = []
    for job in jobs:
        # Fetch items grouped by status
        completed_items = session.exec(
            select(JobItem)
            .where(JobItem.job_id == job.id, JobItem.status == JobItemStatus.COMPLETED)
        ).all()

        skipped_items = session.exec(
            select(JobItem)
            .where(JobItem.job_id == job.id, JobItem.status == JobItemStatus.SKIPPED)
        ).all()

        failed_items = session.exec(
            select(JobItem)
            .where(JobItem.job_id == job.id, JobItem.status == JobItemStatus.FAILED)
        ).all()

        result.append(RecentJobDetail(
            id=job.id,
            created_at=job.created_at,
            completed_at=job.completed_at,
            total_items=job.total_items,
            completed_count=len(completed_items),
            skipped_count=len(skipped_items),
            failed_count=len(failed_items),
            completed_items=[
                JobItemSummary(
                    source_title=item.source_title,
                    result_game_title=_get_result_game_title(item),
                    result_igdb_id=item.resolved_igdb_id,
                    result_user_game_id=_get_result_user_game_id(item),
                    is_new_addition=_check_is_new_addition(item),
                )
                for item in completed_items
            ],
            skipped_items=[
                JobItemSummary(source_title=item.source_title)
                for item in skipped_items
            ],
            failed_items=[
                JobItemSummary(
                    source_title=item.source_title,
                    error_message=item.error_message,
                )
                for item in failed_items
            ],
        ))

    return RecentJobsResponse(jobs=result)


def _get_result_game_title(item: JobItem) -> Optional[str]:
    """Extract game title from result JSON."""
    try:
        result = json.loads(item.result_json) if item.result_json else {}
        return result.get("igdb_title") or result.get("game_title")
    except json.JSONDecodeError:
        return None


def _check_is_new_addition(item: JobItem) -> bool:
    """Check if the item was a new addition vs already in library."""
    try:
        result = json.loads(item.result_json) if item.result_json else {}
        # Check result type from processing
        result_type = result.get("result_type", "")
        return result_type in ("auto_imported", "imported_new", "linked_new")
    except json.JSONDecodeError:
        return False


def _get_result_user_game_id(item: JobItem) -> Optional[str]:
    """Extract user_game_id from result JSON."""
    try:
        result = json.loads(item.result_json) if item.result_json else {}
        return result.get("user_game_id")
    except json.JSONDecodeError:
        return None


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

    # Calculate total pages
    pages = (total + page_size - 1) // page_size if total > 0 else 0

    return JobItemListResponse(
        items=[JobItemResponse.model_validate(item) for item in items],
        total=total,
        page=page,
        page_size=page_size,
        pages=pages,
    )


@router.post("/{job_id}/cancel", response_model=JobCancelResponse)
async def cancel_job(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Cancel and delete an in-progress job.

    Only jobs that are not in a terminal state (completed, failed, cancelled)
    can be cancelled. Cancelling a job immediately deletes it and all associated
    items. Users can only cancel their own jobs.
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

    # Log the cancellation before deleting
    previous_status = job.status
    logger.info(
        f"Cancelling and deleting job {job_id} for user {current_user.id} (was {previous_status.value})"
    )

    # Delete the job (cascade will delete job items)
    session.delete(job)
    session.commit()

    logger.info(f"Job {job_id} cancelled and removed by user {current_user.id}")

    return JobCancelResponse(
        success=True,
        message="Job cancelled and removed",
        job=None,
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


@router.post("/{job_id}/retry-failed", response_model=RetryFailedResponse)
async def retry_failed_job_items(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Retry all failed items in a job.

    Resets failed items to PENDING status and re-enqueues them for processing.
    Only works on jobs in terminal state (completed, failed, cancelled).
    """
    logger.info(f"User {current_user.id} requesting retry of failed items for job {job_id}")

    job = session.get(Job, job_id)

    if not job:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    if job.user_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    if not job.is_terminal:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Job must be completed to retry items",
        )

    # Get failed items before reset (we need the IDs for re-enqueueing)
    failed_items = session.exec(
        sql_select(JobItem)
        .where(JobItem.job_id == job_id)
        .where(JobItem.status == JobItemStatus.FAILED)
    ).all()

    if len(failed_items) == 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="No failed items to retry",
        )

    # Reset failed items
    retried_count = retry_failed_items(session, job_id)

    # Set job back to processing
    job.status = BackgroundJobStatus.PROCESSING
    job.completed_at = None
    session.add(job)
    session.commit()

    logger.info(f"Retrying {retried_count} failed items for job {job_id}")

    # Re-enqueue items for processing
    for item in failed_items:
        await enqueue_import_task(str(item.id), job.priority)

    return RetryFailedResponse(
        success=True,
        message=f"Retrying {retried_count} failed items",
        retried_count=retried_count,
    )



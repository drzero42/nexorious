"""
Job management endpoints for background task tracking.

Provides endpoints for listing, viewing, cancelling, and deleting
background jobs (sync, import, export operations).
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, func, col
from typing import Annotated, Optional
from datetime import datetime, timezone
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.job import (
    Job,
    ReviewItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ReviewItemStatus,
)
from ..schemas.job import (
    JobResponse,
    JobListResponse,
    JobCancelResponse,
    JobDeleteResponse,
    JobConfirmResponse,
    JobDiscardResponse,
    JobType,
    JobSource,
    JobStatus,
)
from ..schemas.mapping_resolution import (
    UnresolvedMappingsResponse,
    UnresolvedMapping,
    ResolveMappingsRequest,
)
from ..utils.sqlalchemy_typed import desc

router = APIRouter(prefix="/jobs", tags=["Jobs"])
logger = logging.getLogger(__name__)


def _job_to_response(job: Job, session: Session) -> JobResponse:
    """Convert a Job model to JobResponse with computed fields."""
    # Count review items for this job
    review_count_stmt = select(func.count()).select_from(ReviewItem).where(
        ReviewItem.job_id == job.id
    )
    review_item_count = session.exec(review_count_stmt).one()

    # Count pending review items
    pending_count_stmt = select(func.count()).select_from(ReviewItem).where(
        ReviewItem.job_id == job.id,
        ReviewItem.status == ReviewItemStatus.PENDING,
    )
    pending_review_count = session.exec(pending_count_stmt).one()

    return JobResponse(
        id=job.id,
        user_id=job.user_id,
        job_type=JobType(job.job_type.value),
        source=JobSource(job.source.value),
        status=JobStatus(job.status.value),
        priority=job.priority,
        progress_current=job.progress_current,
        progress_total=job.progress_total,
        progress_percent=job.progress_percent,
        result_summary=job.get_result_summary(),
        error_message=job.error_message,
        file_path=job.file_path,
        taskiq_task_id=job.taskiq_task_id,
        created_at=job.created_at,
        started_at=job.started_at,
        completed_at=job.completed_at,
        is_terminal=job.is_terminal,
        duration_seconds=job.duration_seconds,
        review_item_count=review_item_count,
        pending_review_count=pending_review_count,
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
    job.set_result_summary({
        **job.get_result_summary(),
        "cancelled_from_status": previous_status.value,
        "cancelled_by": current_user.id,
    })

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

    # Delete the job (cascade will delete review items)
    session.delete(job)
    session.commit()

    logger.info(f"Job {job_id} deleted by user {current_user.id}")

    return JobDeleteResponse(
        success=True,
        message="Job deleted successfully",
        deleted_job_id=job_id,
    )


@router.post("/{job_id}/discard", response_model=JobDiscardResponse)
async def discard_import(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Discard an import job and all associated review items.

    Works for import jobs in PENDING, PROCESSING, or AWAITING_REVIEW status.
    Completely removes the job and all review items from the database.
    """
    logger.info(f"User {current_user.id} requesting to discard import job {job_id}")

    job = session.get(Job, job_id)

    if not job or job.user_id != current_user.id:
        logger.warning(f"Job {job_id} not found for discard")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    if job.job_type != BackgroundJobType.IMPORT:
        logger.warning(f"Cannot discard job {job_id} - not an import job")
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Only import jobs can be discarded",
        )

    # Allow discarding jobs in PENDING, PROCESSING, or AWAITING_REVIEW status
    allowed_statuses = {
        BackgroundJobStatus.PENDING,
        BackgroundJobStatus.PROCESSING,
        BackgroundJobStatus.AWAITING_REVIEW,
    }
    if job.status not in allowed_statuses:
        logger.warning(
            f"Cannot discard job {job_id} - wrong status (status: {job.status})"
        )
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"Cannot discard job - must be pending, processing, or awaiting review. Current status: {job.status.value}",
        )

    # Count review items before deletion
    review_item_count = session.exec(
        select(func.count()).select_from(ReviewItem).where(ReviewItem.job_id == job_id)
    ).one()

    # Delete job (cascade will delete review items)
    session.delete(job)
    session.commit()

    logger.info(
        f"Import job {job_id} discarded by user {current_user.id} "
        f"({review_item_count} review items deleted)"
    )

    return JobDiscardResponse(
        success=True,
        message="Import discarded successfully",
        deleted_job_id=job_id,
        deleted_review_items=review_item_count,
    )


@router.post("/{job_id}/confirm", response_model=JobConfirmResponse)
async def confirm_import(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Confirm an import job after review is complete.

    This endpoint is called when all review items have been resolved and
    the user is ready to finalize the import. Only jobs in 'ready' or
    'awaiting_review' status can be confirmed (awaiting_review allowed
    if all review items have been resolved).
    """
    logger.info(f"User {current_user.id} requesting to confirm job {job_id}")

    job = session.get(Job, job_id)

    if not job:
        logger.warning(f"Job {job_id} not found for confirmation")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Authorization check
    if job.user_id != current_user.id:
        logger.warning(
            f"User {current_user.id} attempted to confirm job {job_id} owned by {job.user_id}"
        )
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Only import jobs can be confirmed
    if job.job_type != BackgroundJobType.IMPORT:
        logger.warning(f"Cannot confirm job {job_id} - not an import job")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Only import jobs can be confirmed",
        )

    # Check if job is in a confirmable state
    confirmable_statuses = [
        BackgroundJobStatus.READY,
        BackgroundJobStatus.AWAITING_REVIEW,
    ]
    if job.status not in confirmable_statuses:
        logger.warning(
            f"Cannot confirm job {job_id} - status is {job.status.value}"
        )
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot confirm job - status must be 'ready' or 'awaiting_review'. Current status: {job.status.value}",
        )

    # Check for pending review items
    pending_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.job_id == job.id,
            ReviewItem.status == ReviewItemStatus.PENDING,
        )
    ).one()

    if pending_count > 0:
        logger.warning(
            f"Cannot confirm job {job_id} - {pending_count} pending review items"
        )
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot confirm job - {pending_count} review items still pending",
        )

    # Count resolved items by status for the response
    matched_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.job_id == job.id,
            ReviewItem.status == ReviewItemStatus.MATCHED,
        )
    ).one()

    skipped_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.job_id == job.id,
            ReviewItem.status == ReviewItemStatus.SKIPPED,
        )
    ).one()

    removal_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.job_id == job.id,
            ReviewItem.status == ReviewItemStatus.REMOVAL,
        )
    ).one()

    # Update job status to finalizing (the actual game addition
    # would be handled by a background task in the full implementation)
    job.status = BackgroundJobStatus.FINALIZING

    # For now, we'll mark it as completed since the actual game addition
    # logic will be implemented in the import tasks
    job.status = BackgroundJobStatus.COMPLETED
    job.completed_at = datetime.now(timezone.utc)

    # Update result summary with confirmation details
    result_summary = job.get_result_summary()
    result_summary["confirmed_by"] = current_user.id
    result_summary["confirmed_at"] = datetime.now(timezone.utc).isoformat()
    result_summary["games_matched"] = matched_count
    result_summary["games_skipped"] = skipped_count
    result_summary["games_removed"] = removal_count
    job.set_result_summary(result_summary)

    session.commit()
    session.refresh(job)

    logger.info(
        f"Job {job_id} confirmed by user {current_user.id}: "
        f"matched={matched_count}, skipped={skipped_count}, removed={removal_count}"
    )

    return JobConfirmResponse(
        success=True,
        message="Import confirmed successfully",
        job=_job_to_response(job, session),
        games_added=matched_count,
        games_skipped=skipped_count,
        games_removed=removal_count,
    )


@router.get("/{job_id}/unresolved-mappings", response_model=UnresolvedMappingsResponse)
async def get_unresolved_mappings(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Get unresolved platform/storefront mappings for a Darkadia import job.

    Returns a list of platforms and storefronts that could not be automatically
    mapped to existing system platforms/storefronts. The user must resolve these
    before the import can proceed.
    """
    logger.debug(f"Getting unresolved mappings for job {job_id}")

    job = session.get(Job, job_id)

    if not job:
        logger.warning(f"Job {job_id} not found")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Authorization check
    if job.user_id != current_user.id:
        logger.warning(
            f"User {current_user.id} attempted to access job {job_id} owned by {job.user_id}"
        )
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Only Darkadia imports have mapping resolution
    if job.source != BackgroundJobSource.DARKADIA:
        logger.warning(
            f"Cannot get unresolved mappings for job {job_id} - source is {job.source}"
        )
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Only Darkadia imports have mapping resolution",
        )

    result_summary = job.get_result_summary()
    unresolved = result_summary.get("unresolved_mappings", {"platforms": [], "storefronts": []})

    platforms = [
        UnresolvedMapping(**mapping) for mapping in unresolved.get("platforms", [])
    ]
    storefronts = [
        UnresolvedMapping(**mapping) for mapping in unresolved.get("storefronts", [])
    ]

    all_resolved = len(platforms) == 0 and len(storefronts) == 0

    logger.info(
        f"Job {job_id} has {len(platforms)} unresolved platforms and {len(storefronts)} unresolved storefronts"
    )

    return UnresolvedMappingsResponse(
        platforms=platforms,
        storefronts=storefronts,
        all_resolved=all_resolved,
    )


@router.put("/{job_id}/resolve-mappings")
async def resolve_mappings(
    job_id: str,
    request: ResolveMappingsRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Submit user's platform/storefront mapping resolutions.

    Accepts a list of mapping resolutions from the user and stores them in the
    job's result_summary for use during import finalization.
    """
    logger.info(
        f"User {current_user.id} resolving {len(request.resolutions)} mappings for job {job_id}"
    )

    job = session.get(Job, job_id)

    if not job:
        logger.warning(f"Job {job_id} not found")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Authorization check
    if job.user_id != current_user.id:
        logger.warning(
            f"User {current_user.id} attempted to access job {job_id} owned by {job.user_id}"
        )
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Only Darkadia imports have mapping resolution
    if job.source != BackgroundJobSource.DARKADIA:
        logger.warning(
            f"Cannot resolve mappings for job {job_id} - source is {job.source}"
        )
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Only Darkadia imports have mapping resolution",
        )

    result_summary = job.get_result_summary()
    resolved_mappings = result_summary.get("resolved_mappings", {"platforms": {}, "storefronts": {}})

    # Apply user's resolutions
    for resolution in request.resolutions:
        if resolution.type == "platform":
            resolved_mappings["platforms"][resolution.original_name] = resolution.resolved_id
        elif resolution.type == "storefront":
            resolved_mappings["storefronts"][resolution.original_name] = resolution.resolved_id

    # Update job with resolved mappings and clear unresolved
    result_summary["resolved_mappings"] = resolved_mappings
    result_summary["unresolved_mappings"] = {"platforms": [], "storefronts": []}
    job.set_result_summary(result_summary)

    session.add(job)
    session.commit()

    logger.info(
        f"Resolved {len(request.resolutions)} mappings for job {job_id}"
    )

    return {
        "message": "Mappings resolved",
        "resolved_count": len(request.resolutions),
    }

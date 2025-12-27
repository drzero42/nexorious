"""API endpoints for job items (replaces review API)."""

from datetime import datetime, timezone

from fastapi import APIRouter, Depends, HTTPException
from sqlmodel import Session

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.job import Job, JobItem, JobItemStatus, BackgroundJobStatus
from ..models.user import User
from ..schemas.job_item import (
    JobItemDetailResponse,
    ResolveJobItemRequest,
    SkipJobItemRequest,
)
from ..worker.tasks.import_export.process_import_item import enqueue_import_task

router = APIRouter(prefix="/job-items", tags=["job-items"])


@router.get("/{item_id}", response_model=JobItemDetailResponse)
async def get_job_item(
    item_id: str,
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """Get a job item by ID."""
    item = session.get(JobItem, item_id)
    if not item or item.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job item not found")
    return JobItemDetailResponse.model_validate(item)


@router.post("/{item_id}/resolve", response_model=JobItemDetailResponse)
async def resolve_job_item(
    item_id: str,
    request: ResolveJobItemRequest,
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """Resolve a job item to an IGDB game."""
    item = session.get(JobItem, item_id)
    if not item or item.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job item not found")

    if item.status != JobItemStatus.PENDING_REVIEW:
        raise HTTPException(status_code=400, detail="Item is not pending review")

    item.resolved_igdb_id = request.igdb_id
    item.status = JobItemStatus.COMPLETED
    item.resolved_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(item)

    return JobItemDetailResponse.model_validate(item)


@router.post("/{item_id}/skip", response_model=JobItemDetailResponse)
async def skip_job_item(
    item_id: str,
    request: SkipJobItemRequest,
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """Skip a job item."""
    item = session.get(JobItem, item_id)
    if not item or item.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job item not found")

    if item.status != JobItemStatus.PENDING_REVIEW:
        raise HTTPException(status_code=400, detail="Item is not pending review")

    item.status = JobItemStatus.SKIPPED
    item.resolved_at = datetime.now(timezone.utc)
    if request.reason:
        item.result_json = f'{{"skip_reason": "{request.reason}"}}'
    session.commit()
    session.refresh(item)

    return JobItemDetailResponse.model_validate(item)


@router.post("/{item_id}/retry", response_model=JobItemDetailResponse)
async def retry_job_item(
    item_id: str,
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """Retry a single failed job item."""
    item = session.get(JobItem, item_id)
    if not item or item.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job item not found")

    # Check parent job is terminal
    job = session.get(Job, item.job_id)
    if not job or not job.is_terminal:
        raise HTTPException(
            status_code=400,
            detail="Job must be completed to retry items",
        )

    if item.status != JobItemStatus.FAILED:
        raise HTTPException(status_code=400, detail="Item is not in failed status")

    # Reset the item
    item.status = JobItemStatus.PENDING
    item.error_message = None
    item.processed_at = None
    session.add(item)

    # Set job back to processing
    job.status = BackgroundJobStatus.PROCESSING
    job.completed_at = None
    session.add(job)

    session.commit()
    session.refresh(item)

    # Re-enqueue item for processing
    await enqueue_import_task(str(item.id), job.priority)

    return JobItemDetailResponse.model_validate(item)

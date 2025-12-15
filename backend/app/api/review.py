"""
Review queue API endpoints for managing items that need user matching decisions.

Provides endpoints for listing pending review items, viewing details with
IGDB candidates, and resolving items (match, skip, keep, remove).
"""

from fastapi import APIRouter, Depends, HTTPException, status as http_status, Query
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
    ReviewItemStatus as ModelReviewItemStatus,
)
from ..schemas.review import (
    ReviewItemResponse,
    ReviewItemDetailResponse,
    ReviewListResponse,
    MatchRequest,
    MatchResponse,
    ReviewSummary,
    ReviewCountsByType,
    ReviewItemStatus,
    IGDBCandidate,
)
from ..models.job import BackgroundJobType

router = APIRouter(prefix="/review", tags=["Review"])
logger = logging.getLogger(__name__)


def _review_item_to_response(item: ReviewItem, session: Session) -> ReviewItemResponse:
    """Convert a ReviewItem model to ReviewItemResponse with job context."""
    # Get job for context
    job = session.get(Job, item.job_id)
    job_type = job.job_type.value if job else None
    job_source = job.source.value if job else None

    return ReviewItemResponse(
        id=item.id,
        job_id=item.job_id,
        user_id=item.user_id,
        status=ReviewItemStatus(item.status.value),
        source_title=item.source_title,
        source_metadata=item.get_source_metadata(),
        igdb_candidates=item.get_igdb_candidates(),
        resolved_igdb_id=item.resolved_igdb_id,
        created_at=item.created_at,
        resolved_at=item.resolved_at,
        job_type=job_type,
        job_source=job_source,
    )


def _review_item_to_detail_response(
    item: ReviewItem, session: Session
) -> ReviewItemDetailResponse:
    """Convert a ReviewItem model to ReviewItemDetailResponse with typed IGDB candidates."""
    # Get job for context
    job = session.get(Job, item.job_id)
    job_type = job.job_type.value if job else None
    job_source = job.source.value if job else None

    # Convert raw candidates to typed objects
    raw_candidates = item.get_igdb_candidates()
    typed_candidates = []
    for c in raw_candidates:
        typed_candidates.append(
            IGDBCandidate(
                igdb_id=c.get("igdb_id", c.get("id", 0)),
                name=c.get("name", ""),
                first_release_date=c.get("first_release_date"),
                cover_url=c.get("cover_url"),
                summary=c.get("summary"),
                platforms=c.get("platforms"),
                similarity_score=c.get("similarity_score"),
            )
        )

    return ReviewItemDetailResponse(
        id=item.id,
        job_id=item.job_id,
        user_id=item.user_id,
        status=ReviewItemStatus(item.status.value),
        source_title=item.source_title,
        source_metadata=item.get_source_metadata(),
        igdb_candidates=typed_candidates,
        resolved_igdb_id=item.resolved_igdb_id,
        created_at=item.created_at,
        resolved_at=item.resolved_at,
        job_type=job_type,
        job_source=job_source,
    )


@router.get("/", response_model=ReviewListResponse)
async def list_review_items(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page"),
    status: Optional[ReviewItemStatus] = Query(
        default=None, description="Filter by status"
    ),
    job_id: Optional[str] = Query(default=None, description="Filter by job ID"),
):
    """
    List review items for the current user.

    By default, returns all pending review items. Can be filtered by status
    or job ID. Results are paginated and sorted by creation date (oldest first)
    to show items in the order they were created.
    """
    logger.debug(
        f"Listing review items for user {current_user.id}: status={status}, job_id={job_id}"
    )

    # Build query - only show items for the current user
    query = select(ReviewItem).where(ReviewItem.user_id == current_user.id)

    # Apply filters
    if status:
        query = query.where(ReviewItem.status == ModelReviewItemStatus(status.value))

    if job_id:
        # Verify user owns the job
        job = session.get(Job, job_id)
        if not job or job.user_id != current_user.id:
            raise HTTPException(
                status_code=http_status.HTTP_404_NOT_FOUND,
                detail="Job not found",
            )
        query = query.where(ReviewItem.job_id == job_id)

    # Get total count before pagination
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()

    # Sort by created_at ascending (oldest first, to process in order)
    query = query.order_by(col(ReviewItem.created_at))

    # Apply pagination
    offset = (page - 1) * per_page
    items = session.exec(query.offset(offset).limit(per_page)).all()

    # Calculate pages
    pages = (total + per_page - 1) // per_page if total > 0 else 1

    # Convert to response models
    item_responses = [_review_item_to_response(item, session) for item in items]

    logger.info(
        f"Returning {len(item_responses)} review items for user {current_user.id}"
    )

    return ReviewListResponse(
        items=item_responses,
        total=total,
        page=page,
        per_page=per_page,
        pages=pages,
    )


@router.get("/summary", response_model=ReviewSummary)
async def get_review_summary(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Get summary statistics for the current user's review items.

    Returns counts of items by status and number of jobs with pending items.
    """
    logger.debug(f"Getting review summary for user {current_user.id}")

    # Count items by status
    pending_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.user_id == current_user.id,
            ReviewItem.status == ModelReviewItemStatus.PENDING,
        )
    ).one()

    matched_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.user_id == current_user.id,
            ReviewItem.status == ModelReviewItemStatus.MATCHED,
        )
    ).one()

    skipped_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.user_id == current_user.id,
            ReviewItem.status == ModelReviewItemStatus.SKIPPED,
        )
    ).one()

    removal_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.user_id == current_user.id,
            ReviewItem.status == ModelReviewItemStatus.REMOVAL,
        )
    ).one()

    # Count jobs with pending items
    jobs_with_pending = session.exec(
        select(func.count(func.distinct(ReviewItem.job_id)))
        .select_from(ReviewItem)
        .where(
            ReviewItem.user_id == current_user.id,
            ReviewItem.status == ModelReviewItemStatus.PENDING,
        )
    ).one()

    return ReviewSummary(
        total_pending=pending_count,
        total_matched=matched_count,
        total_skipped=skipped_count,
        total_removal=removal_count,
        jobs_with_pending=jobs_with_pending,
    )


@router.get("/counts", response_model=ReviewCountsByType)
async def get_review_counts_by_type(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Get pending review counts grouped by job type.

    Returns separate counts for import and sync operations.
    Used by navigation badges to show how many items need review.
    """
    logger.debug(f"Getting review counts by type for user {current_user.id}")

    # Count pending reviews from import jobs
    import_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .join(Job, ReviewItem.job_id == Job.id)
        .where(
            ReviewItem.user_id == current_user.id,
            ReviewItem.status == ModelReviewItemStatus.PENDING,
            Job.job_type == BackgroundJobType.IMPORT,
        )
    ).one()

    # Count pending reviews from sync jobs
    sync_count = session.exec(
        select(func.count())
        .select_from(ReviewItem)
        .join(Job, ReviewItem.job_id == Job.id)
        .where(
            ReviewItem.user_id == current_user.id,
            ReviewItem.status == ModelReviewItemStatus.PENDING,
            Job.job_type == BackgroundJobType.SYNC,
        )
    ).one()

    return ReviewCountsByType(
        import_pending=import_count,
        sync_pending=sync_count,
    )


@router.get("/{item_id}", response_model=ReviewItemDetailResponse)
async def get_review_item(
    item_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Get details for a specific review item.

    Returns the full item information including all IGDB candidates
    for the user to choose from.
    """
    logger.debug(f"Getting review item {item_id} for user {current_user.id}")

    item = session.get(ReviewItem, item_id)

    if not item:
        logger.warning(f"Review item {item_id} not found")
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    # Authorization check
    if item.user_id != current_user.id:
        logger.warning(
            f"User {current_user.id} attempted to access review item {item_id} owned by {item.user_id}"
        )
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    return _review_item_to_detail_response(item, session)


@router.post("/{item_id}/match", response_model=MatchResponse)
async def match_review_item(
    item_id: str,
    request: MatchRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Match a review item to an IGDB ID.

    Sets the resolved_igdb_id and marks the item as matched.
    """
    logger.info(
        f"User {current_user.id} matching review item {item_id} to IGDB ID {request.igdb_id}"
    )

    item = session.get(ReviewItem, item_id)

    if not item:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    if item.user_id != current_user.id:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    if item.status != ModelReviewItemStatus.PENDING:
        raise HTTPException(
            status_code=http_status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot match item - already resolved with status: {item.status.value}",
        )

    # Update the item
    item.status = ModelReviewItemStatus.MATCHED
    item.resolved_igdb_id = request.igdb_id
    item.resolved_at = datetime.now(timezone.utc)

    session.commit()
    session.refresh(item)

    logger.info(f"Review item {item_id} matched to IGDB ID {request.igdb_id}")

    return MatchResponse(
        success=True,
        message=f"Item matched to IGDB ID {request.igdb_id}",
        item=_review_item_to_response(item, session),
    )


@router.post("/{item_id}/skip", response_model=MatchResponse)
async def skip_review_item(
    item_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Skip a review item without matching.

    The game will not be added to the collection.
    """
    logger.info(f"User {current_user.id} skipping review item {item_id}")

    item = session.get(ReviewItem, item_id)

    if not item:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    if item.user_id != current_user.id:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    if item.status != ModelReviewItemStatus.PENDING:
        raise HTTPException(
            status_code=http_status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot skip item - already resolved with status: {item.status.value}",
        )

    # Update the item
    item.status = ModelReviewItemStatus.SKIPPED
    item.resolved_at = datetime.now(timezone.utc)

    session.commit()
    session.refresh(item)

    logger.info(f"Review item {item_id} skipped")

    return MatchResponse(
        success=True,
        message="Item skipped",
        item=_review_item_to_response(item, session),
    )


@router.post("/{item_id}/keep", response_model=MatchResponse)
async def keep_review_item(
    item_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Keep a game that was flagged for removal.

    For sync operations that detect a game was removed from a platform library,
    this endpoint allows the user to keep the game in their collection.
    """
    logger.info(f"User {current_user.id} keeping review item {item_id}")

    item = session.get(ReviewItem, item_id)

    if not item:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    if item.user_id != current_user.id:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    if item.status != ModelReviewItemStatus.PENDING:
        raise HTTPException(
            status_code=http_status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot keep item - already resolved with status: {item.status.value}",
        )

    # Update the item - mark as matched but with no IGDB ID change
    # The item stays in collection as-is
    item.status = ModelReviewItemStatus.MATCHED
    item.resolved_at = datetime.now(timezone.utc)

    # Store metadata about the decision
    metadata = item.get_source_metadata()
    metadata["kept_in_collection"] = True
    item.set_source_metadata(metadata)

    session.commit()
    session.refresh(item)

    logger.info(f"Review item {item_id} kept in collection")

    return MatchResponse(
        success=True,
        message="Game kept in collection",
        item=_review_item_to_response(item, session),
    )


@router.post("/{item_id}/remove", response_model=MatchResponse)
async def remove_review_item(
    item_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
):
    """
    Remove a game that was flagged for removal.

    For sync operations that detect a game was removed from a platform library,
    this endpoint confirms the removal and marks the game to be removed
    from the user's collection.
    """
    logger.info(f"User {current_user.id} removing review item {item_id}")

    item = session.get(ReviewItem, item_id)

    if not item:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    if item.user_id != current_user.id:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Review item not found",
        )

    if item.status != ModelReviewItemStatus.PENDING:
        raise HTTPException(
            status_code=http_status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot remove item - already resolved with status: {item.status.value}",
        )

    # Update the item
    item.status = ModelReviewItemStatus.REMOVAL
    item.resolved_at = datetime.now(timezone.utc)

    session.commit()
    session.refresh(item)

    logger.info(f"Review item {item_id} marked for removal")

    return MatchResponse(
        success=True,
        message="Game marked for removal",
        item=_review_item_to_response(item, session),
    )

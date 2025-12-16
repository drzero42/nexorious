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
    BackgroundJobType,
    BackgroundJobSource,
)
from ..models.ignored_external_game import IgnoredExternalGame
from ..models.user_game import UserGame, UserGamePlatform
from ..models.game import Game
from ..schemas.review import (
    ReviewItemResponse,
    ReviewItemDetailResponse,
    ReviewListResponse,
    MatchRequest,
    MatchResponse,
    ReviewSummary,
    ReviewCountsByType,
    ReviewItemStatus,
    ReviewSource,
    IGDBCandidate,
    PlatformMappingSuggestion,
    PlatformSummaryResponse,
)
from ..services.game_service import GameService
from ..services.igdb import IGDBService

router = APIRouter(prefix="/review", tags=["Review"])
logger = logging.getLogger(__name__)

# Constants for platform associations
STEAM_STOREFRONT_ID = "steam"
PC_WINDOWS_PLATFORM_ID = "pc-windows"


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
    source: Optional[ReviewSource] = Query(
        default=None, description="Filter by source type (import or sync)"
    ),
):
    """
    List review items for the current user.

    By default, returns all pending review items. Can be filtered by status,
    job ID, or source type (import/sync). Results are paginated and sorted
    by creation date (oldest first) to show items in the order they were created.
    """
    logger.debug(
        f"Listing review items for user {current_user.id}: status={status}, job_id={job_id}, source={source}"
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

    if source:
        # Filter by job type (import or sync)
        job_type = BackgroundJobType.IMPORT if source == ReviewSource.IMPORT else BackgroundJobType.SYNC
        query = query.join(Job, ReviewItem.job_id == Job.id).where(Job.job_type == job_type)

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


@router.get("/platform-summary", response_model=PlatformSummaryResponse)
async def get_platform_summary(
    job_id: str = Query(..., description="Job ID to get platform summary for"),
    session: Annotated[Session, Depends(get_session)] = None,
    current_user: Annotated[User, Depends(get_current_user)] = None,
):
    """
    Get summary of platform/storefront strings needing mapping for a job.

    Extracts unique platform and storefront strings from all ReviewItems
    for the given job, counts occurrences, and suggests matches to
    existing Platform/Storefront entities.
    """
    from collections import Counter
    from ..models.platform import Platform, Storefront

    # Verify job exists and belongs to user
    job = session.get(Job, job_id)
    if not job:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )
    if job.user_id != current_user.id:
        raise HTTPException(
            status_code=http_status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Get all review items for this job
    items = session.exec(
        select(ReviewItem).where(ReviewItem.job_id == job_id)
    ).all()

    # Extract and count platform/storefront strings
    platform_counts: Counter = Counter()
    storefront_counts: Counter = Counter()

    for item in items:
        metadata = item.get_source_metadata()
        for p in metadata.get("platforms", []):
            if p:
                platform_counts[p] += 1
        for s in metadata.get("storefronts", []):
            if s:
                storefront_counts[s] += 1

    # Get all platforms and storefronts for matching
    all_platforms = session.exec(select(Platform).where(Platform.is_active == True)).all()
    all_storefronts = session.exec(select(Storefront).where(Storefront.is_active == True)).all()

    # Build platform suggestions
    platform_suggestions = []
    for original, count in platform_counts.items():
        suggested = _find_best_match(original, [(p.id, p.name, p.display_name) for p in all_platforms])
        platform_suggestions.append(PlatformMappingSuggestion(
            original=original,
            count=count,
            suggested_id=suggested[0] if suggested else None,
            suggested_name=suggested[1] if suggested else None,
        ))

    # Build storefront suggestions
    storefront_suggestions = []
    for original, count in storefront_counts.items():
        suggested = _find_best_match(original, [(s.id, s.name, s.display_name) for s in all_storefronts])
        storefront_suggestions.append(PlatformMappingSuggestion(
            original=original,
            count=count,
            suggested_id=suggested[0] if suggested else None,
            suggested_name=suggested[1] if suggested else None,
        ))

    # Sort by count descending
    platform_suggestions.sort(key=lambda x: x.count, reverse=True)
    storefront_suggestions.sort(key=lambda x: x.count, reverse=True)

    # Check if all resolved
    all_resolved = (
        all(p.suggested_id for p in platform_suggestions) and
        all(s.suggested_id for s in storefront_suggestions)
    )

    return PlatformSummaryResponse(
        platforms=platform_suggestions,
        storefronts=storefront_suggestions,
        all_resolved=all_resolved,
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
    For sync sources (Steam), also adds the game to collection and creates platform associations.
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

    # Get source metadata to determine if this is a sync operation
    source_metadata = item.get_source_metadata()
    source = source_metadata.get("source")

    # Update the item
    item.status = ModelReviewItemStatus.MATCHED
    item.resolved_igdb_id = request.igdb_id
    item.resolved_at = datetime.now(timezone.utc)

    session.commit()
    session.refresh(item)

    # Handle sync source finalization (e.g., Steam)
    if source == "steam":
        steam_appid = source_metadata.get("steam_appid")
        if not steam_appid:
            logger.error(f"Missing steam_appid in source metadata for review item {item_id}")
            raise HTTPException(
                status_code=http_status.HTTP_400_BAD_REQUEST,
                detail="Missing Steam AppID in review item metadata",
            )

        try:
            await _finalize_steam_match(
                session=session,
                user_id=current_user.id,
                igdb_id=request.igdb_id,
                steam_appid=str(steam_appid),
            )
            logger.info(
                f"Finalized Steam match for review item {item_id}: "
                f"IGDB ID {request.igdb_id}, AppID {steam_appid}"
            )
        except Exception as e:
            logger.error(
                f"Failed to finalize Steam match for review item {item_id}: {e}",
                exc_info=True,
            )
            raise HTTPException(
                status_code=http_status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=f"Failed to add game to collection: {str(e)}",
            )

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
    For sync sources (Steam, Epic, GOG), creates an IgnoredExternalGame record
    to prevent the item from appearing in future syncs.
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

    # Get source metadata to determine if this is a sync operation
    source_metadata = item.get_source_metadata()
    source = source_metadata.get("source")

    # Update the item
    item.status = ModelReviewItemStatus.SKIPPED
    item.resolved_at = datetime.now(timezone.utc)

    session.commit()
    session.refresh(item)

    # Create ignored game record for sync sources
    if source in ["steam", "epic", "gog"]:
        try:
            _create_ignored_external_game(
                session=session,
                user_id=current_user.id,
                source=source,
                external_id=_get_external_id_from_metadata(source_metadata, source),
                title=item.source_title,
            )
            logger.info(
                f"Created ignored external game record for {source} game: {item.source_title}"
            )
        except Exception as e:
            logger.error(
                f"Failed to create ignored external game record for review item {item_id}: {e}",
                exc_info=True,
            )
            # Don't fail the skip operation if ignored game creation fails
            # The item is still skipped, we just can't prevent it from appearing again

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


# Helper functions for sync finalization


async def _finalize_steam_match(
    session: Session,
    user_id: str,
    igdb_id: int,
    steam_appid: str,
) -> None:
    """
    Finalize a Steam sync match by adding the game to collection.

    Creates or updates the game from IGDB, creates a UserGame entry if needed,
    and adds the Steam platform association.

    Args:
        session: Database session
        user_id: User ID
        igdb_id: IGDB game ID
        steam_appid: Steam AppID
    """
    # Check if user already has this game
    existing_user_game = session.exec(
        select(UserGame).where(
            UserGame.user_id == user_id,
            UserGame.game_id == igdb_id,
        )
    ).first()

    if existing_user_game:
        # Game already in collection, just add platform association
        logger.debug(
            f"Game {igdb_id} already in collection for user {user_id}, "
            f"adding Steam platform association"
        )
        _add_steam_platform(session, existing_user_game.id, steam_appid)
        return

    # Ensure game exists in games table (fetch from IGDB if needed)
    game = session.get(Game, igdb_id)
    if not game:
        # Create game from IGDB
        igdb_service = IGDBService()
        game_service = GameService(session, igdb_service)
        game = await game_service.create_or_update_game_from_igdb(
            igdb_id, download_cover_art=True
        )
        logger.debug(f"Created game {igdb_id} from IGDB: {game.title}")

    # Create UserGame entry
    user_game = UserGame(
        user_id=user_id,
        game_id=igdb_id,
    )
    session.add(user_game)
    session.commit()
    session.refresh(user_game)

    logger.info(
        f"Added game {igdb_id} ({game.title}) to collection for user {user_id}"
    )

    # Add Steam platform association
    _add_steam_platform(session, user_game.id, steam_appid)


def _add_steam_platform(
    session: Session,
    user_game_id: str,
    steam_appid: str,
) -> None:
    """
    Add Steam platform association to a UserGame.

    Creates a UserGamePlatform entry linking the game to Steam storefront
    with the Steam AppID stored in store_game_id.

    Args:
        session: Database session
        user_game_id: UserGame ID
        steam_appid: Steam AppID
    """
    # Check if association already exists
    existing = session.exec(
        select(UserGamePlatform).where(
            UserGamePlatform.user_game_id == user_game_id,
            UserGamePlatform.storefront_id == STEAM_STOREFRONT_ID,
            UserGamePlatform.store_game_id == steam_appid,
        )
    ).first()

    if not existing:
        platform = UserGamePlatform(
            user_game_id=user_game_id,
            platform_id=PC_WINDOWS_PLATFORM_ID,
            storefront_id=STEAM_STOREFRONT_ID,
            store_game_id=steam_appid,
            store_url=f"https://store.steampowered.com/app/{steam_appid}",
            is_available=True,
        )
        session.add(platform)
        session.commit()
        logger.debug(
            f"Added Steam platform association for UserGame {user_game_id}, "
            f"AppID {steam_appid}"
        )
    else:
        logger.debug(
            f"Steam platform association already exists for UserGame {user_game_id}"
        )


def _create_ignored_external_game(
    session: Session,
    user_id: str,
    source: str,
    external_id: str,
    title: str,
) -> None:
    """
    Create an IgnoredExternalGame record to prevent future sync matches.

    Args:
        session: Database session
        user_id: User ID
        source: Source platform (steam, epic, gog)
        external_id: Platform-specific game ID
        title: Game title for display
    """
    # Convert source string to BackgroundJobSource enum
    source_enum_map = {
        "steam": BackgroundJobSource.STEAM,
        "epic": BackgroundJobSource.EPIC,
        "gog": BackgroundJobSource.GOG,
    }

    source_enum = source_enum_map.get(source)
    if not source_enum:
        logger.warning(f"Unknown source type for ignored game: {source}")
        return

    # Check if already ignored
    existing = session.exec(
        select(IgnoredExternalGame).where(
            IgnoredExternalGame.user_id == user_id,
            IgnoredExternalGame.source == source_enum,
            IgnoredExternalGame.external_id == external_id,
        )
    ).first()

    if existing:
        logger.debug(
            f"External game already ignored: {source} {external_id} for user {user_id}"
        )
        return

    # Create ignored game record
    ignored_game = IgnoredExternalGame(
        user_id=user_id,
        source=source_enum,
        external_id=external_id,
        title=title,
    )
    session.add(ignored_game)
    session.commit()
    logger.debug(
        f"Created ignored external game: {source} {external_id} ({title}) "
        f"for user {user_id}"
    )


def _get_external_id_from_metadata(
    source_metadata: dict,
    source: str,
) -> str:
    """
    Extract the external game ID from source metadata.

    Args:
        source_metadata: Source metadata dictionary
        source: Source platform (steam, epic, gog)

    Returns:
        External game ID (e.g., Steam AppID)
    """
    # Map source to metadata key
    id_key_map = {
        "steam": "steam_appid",
        "epic": "epic_id",
        "gog": "gog_id",
    }

    id_key = id_key_map.get(source)
    if not id_key:
        logger.warning(f"Unknown source type: {source}")
        return ""

    external_id = source_metadata.get(id_key, "")
    if not external_id:
        logger.warning(
            f"Missing external ID in metadata for source {source}: {source_metadata}"
        )

    return str(external_id)


def _find_best_match(
    original: str,
    candidates: list[tuple[str, str, str]],  # (id, name, display_name)
) -> Optional[tuple[str, str]]:
    """
    Find best matching platform/storefront for a string.

    Uses case-insensitive exact matching on name or display_name.
    Returns (id, display_name) or None.
    """
    original_lower = original.lower().strip()

    for id_, name, display_name in candidates:
        if name.lower() == original_lower or display_name.lower() == original_lower:
            return (id_, display_name)

    # Try partial matching for common abbreviations
    abbrev_map = {
        "ps4": "playstation 4",
        "ps5": "playstation 5",
        "ps3": "playstation 3",
        "xb1": "xbox one",
        "xsx": "xbox series x",
        "nsw": "nintendo switch",
        "psn": "playstation store",
        "xbl": "xbox live",
    }

    if original_lower in abbrev_map:
        expanded = abbrev_map[original_lower]
        for id_, name, display_name in candidates:
            if name.lower() == expanded or display_name.lower() == expanded:
                return (id_, display_name)

    return None

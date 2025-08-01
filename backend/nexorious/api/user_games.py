"""
User game collection management endpoints.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, and_, func, or_
from datetime import datetime, timezone
from typing import Annotated, Optional, List
import logging
from rapidfuzz import fuzz

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.game import Game
from ..models.platform import Platform, Storefront
from ..models.user_game import UserGame, UserGamePlatform, OwnershipStatus, PlayStatus
from ..api.schemas.user_game import (
    UserGameCreateRequest,
    UserGameUpdateRequest,
    ProgressUpdateRequest,
    UserGamePlatformCreateRequest,
    UserGameResponse,
    UserGameListRequest,
    UserGameListResponse,
    UserGamePlatformResponse,
    BulkStatusUpdateRequest,
    BulkDeleteRequest,
    CollectionStatsResponse
)
from ..api.schemas.common import SuccessResponse

router = APIRouter(prefix="/user-games", tags=["User Game Collection"])
logger = logging.getLogger(__name__)


def _rank_user_games_by_fuzzy_match(user_games: List[UserGame], query: str, threshold: float = 0.6) -> List[UserGame]:
    """Rank user games by fuzzy matching similarity to query."""
    if not user_games or not query.strip():
        return user_games
    
    query_lower = query.lower().strip()
    
    # Calculate similarity scores for each user game
    scored_games = []
    for user_game in user_games:
        # Calculate multiple similarity scores using game title
        title_lower = user_game.game.title.lower()
        
        # Different matching strategies
        exact_score = 1.0 if query_lower == title_lower else 0.0
        ratio_score = fuzz.ratio(query_lower, title_lower) / 100.0
        partial_score = fuzz.partial_ratio(query_lower, title_lower) / 100.0
        token_sort_score = fuzz.token_sort_ratio(query_lower, title_lower) / 100.0
        token_set_score = fuzz.token_set_ratio(query_lower, title_lower) / 100.0
        
        # Calculate weighted final score
        final_score = max(
            exact_score * 1.0,  # Exact match gets highest priority
            ratio_score * 0.9,  # Overall similarity
            partial_score * 0.8,  # Partial match
            token_sort_score * 0.7,  # Token order similarity
            token_set_score * 0.6  # Token set similarity
        )
        
        # Only include games above threshold
        if final_score >= threshold:
            scored_games.append((user_game, final_score))
    
    # Sort by score (descending) and return user games
    scored_games.sort(key=lambda x: x[1], reverse=True)
    
    logger.debug(f"Fuzzy matching results for '{query}': {[(ug.game.title, s) for ug, s in scored_games[:5]]}")
    
    return [user_game for user_game, score in scored_games]


def _update_ownership_status_after_platform_change(
    session: Session, 
    user_game: UserGame
) -> None:
    """
    Update ownership status automatically based on platform associations.
    
    Rules:
    - If last platform is removed from an OWNED game, change to NO_LONGER_OWNED
    - If platform is added to a NO_LONGER_OWNED game, change to OWNED
    - Only affects OWNED and NO_LONGER_OWNED statuses, leaves others unchanged
    """
    # Only manage OWNED and NO_LONGER_OWNED statuses automatically
    if user_game.ownership_status not in [OwnershipStatus.OWNED, OwnershipStatus.NO_LONGER_OWNED]:
        return
    
    # Count current platform associations
    platform_count = session.exec(
        select(func.count()).where(UserGamePlatform.user_game_id == user_game.id)
    ).one()
    
    old_status = user_game.ownership_status
    
    # Apply ownership status rules
    if platform_count == 0 and user_game.ownership_status == OwnershipStatus.OWNED:
        # Last platform removed from owned game -> no longer owned
        user_game.ownership_status = OwnershipStatus.NO_LONGER_OWNED
        user_game.updated_at = datetime.now(timezone.utc)
        logger.info(f"Automatically changed ownership status from {old_status} to {user_game.ownership_status} for user game {user_game.id} (no platforms remaining)")
    
    elif platform_count > 0 and user_game.ownership_status == OwnershipStatus.NO_LONGER_OWNED:
        # Platform added to no longer owned game -> owned
        user_game.ownership_status = OwnershipStatus.OWNED
        user_game.updated_at = datetime.now(timezone.utc)
        logger.info(f"Automatically changed ownership status from {old_status} to {user_game.ownership_status} for user game {user_game.id} (platforms available)")


@router.get("/", response_model=UserGameListResponse)
async def list_user_games(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page"),
    limit: Optional[int] = Query(default=None, ge=1, le=100, description="Items per page (alias for per_page)"),
    play_status: Optional[PlayStatus] = Query(default=None, description="Filter by play status"),
    ownership_status: Optional[OwnershipStatus] = Query(default=None, description="Filter by ownership status"),
    is_loved: Optional[bool] = Query(default=None, description="Filter by loved status"),
    platform_id: Optional[str] = Query(default=None, description="Filter by platform"),
    storefront_id: Optional[str] = Query(default=None, description="Filter by storefront"),
    rating_min: Optional[float] = Query(default=None, ge=1, le=5, description="Minimum rating filter"),
    rating_max: Optional[float] = Query(default=None, ge=1, le=5, description="Maximum rating filter"),
    has_notes: Optional[bool] = Query(default=None, description="Filter by presence of notes"),
    q: Optional[str] = Query(default=None, description="Search in game titles and notes"),
    fuzzy_threshold: Optional[float] = Query(default=None, ge=0.0, le=1.0, description="Fuzzy matching threshold for title search (0.0-1.0)"),
    sort_by: Optional[str] = Query(default="created_at", description="Sort field"),
    sort_order: Optional[str] = Query(default="desc", pattern="^(asc|desc)$", description="Sort order")
):
    """List user's game collection with filtering and sorting."""
    
    # Handle limit parameter as alias for per_page
    if limit is not None:
        per_page = limit
    
    # Build base query
    query = select(UserGame).where(UserGame.user_id == current_user.id)
    
    # Apply filters
    filters = []
    
    if play_status is not None:
        filters.append(UserGame.play_status == play_status)
    
    if ownership_status is not None:
        filters.append(UserGame.ownership_status == ownership_status)
    
    if is_loved is not None:
        filters.append(UserGame.is_loved == is_loved)
    
    if rating_min is not None:
        filters.append(UserGame.personal_rating >= rating_min)
    
    if rating_max is not None:
        filters.append(UserGame.personal_rating <= rating_max)
    
    if has_notes is not None:
        if has_notes:
            filters.append(UserGame.personal_notes.is_not(None))
            filters.append(UserGame.personal_notes != "")
        else:
            filters.append(or_(
                UserGame.personal_notes.is_(None),
                UserGame.personal_notes == ""
            ))
    
    if platform_id:
        # Join with UserGamePlatform to filter by platform
        query = query.join(UserGamePlatform).where(UserGamePlatform.platform_id == platform_id)
    
    if storefront_id:
        # Join with UserGamePlatform to filter by storefront
        query = query.join(UserGamePlatform).where(UserGamePlatform.storefront_id == storefront_id)
    
    # Apply filters
    if filters:
        query = query.where(and_(*filters))
    
    # Check if we're in fuzzy search mode
    fuzzy_search_mode = q and fuzzy_threshold is not None
    
    if q and not fuzzy_search_mode:
        # Regular search in game title and personal notes
        query = query.join(Game)
        query = query.where(or_(
            Game.title.icontains(q),
            and_(UserGame.personal_notes.is_not(None), UserGame.personal_notes.icontains(q))
        ))
    
    if fuzzy_search_mode:
        # For fuzzy search, we need to get all matching games first, then apply fuzzy matching
        # Always join with Game table for fuzzy search
        query = query.join(Game)
        
        # Get all user games matching non-text filters
        all_user_games = session.exec(query).all()
        
        # Apply fuzzy matching
        fuzzy_user_games = _rank_user_games_by_fuzzy_match(all_user_games, q, fuzzy_threshold)
        
        # Apply pagination to fuzzy results
        total = len(fuzzy_user_games)
        offset = (page - 1) * per_page
        user_games = fuzzy_user_games[offset:offset + per_page]
        
        # Calculate pages
        pages = (total + per_page - 1) // per_page
        
        return UserGameListResponse(
            user_games=user_games,
            total=total,
            page=page,
            per_page=per_page,
            pages=pages
        )
    
    # Apply sorting
    # Check if we need to join with Game table for sorting
    game_sort_fields = {'title', 'genre', 'developer', 'publisher', 'release_date'}
    need_game_join = sort_by in game_sort_fields
    
    # Track if Game table is already joined (for search query)
    already_joined_game = q is not None
    
    # Only add Game join if needed and not already joined
    if need_game_join and not already_joined_game:
        query = query.join(Game)
    
    # Determine the sort field
    if sort_by in game_sort_fields:
        # Sort by Game model fields
        sort_field = getattr(Game, sort_by, Game.title)
    else:
        # Sort by UserGame model fields (default behavior)
        sort_field = getattr(UserGame, sort_by, UserGame.created_at)
    
    # Apply sort order
    if sort_order == "desc":
        query = query.order_by(sort_field.desc())
    else:
        query = query.order_by(sort_field.asc())
    
    # Get total count
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()
    
    # Apply pagination
    offset = (page - 1) * per_page
    user_games = session.exec(query.offset(offset).limit(per_page)).all()
    
    # Calculate pages
    pages = (total + per_page - 1) // per_page
    
    return UserGameListResponse(
        user_games=user_games,
        total=total,
        page=page,
        per_page=per_page,
        pages=pages
    )


@router.get("/stats", response_model=CollectionStatsResponse)
async def get_collection_stats(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get user's collection statistics."""
    
    # Total games
    total_games = session.exec(
        select(func.count()).where(UserGame.user_id == current_user.id)
    ).one()
    
    # Games by status
    status_counts = {}
    for status in PlayStatus:
        count = session.exec(
            select(func.count()).where(
                and_(
                    UserGame.user_id == current_user.id,
                    UserGame.play_status == status
                )
            )
        ).one()
        status_counts[status] = count
    
    # Platform counts (only if there are games)
    if total_games > 0:
        platform_counts = session.exec(
            select(Platform.display_name, func.count()).
            join(UserGamePlatform).
            join(UserGame).
            where(UserGame.user_id == current_user.id).
            group_by(Platform.id, Platform.display_name)
        ).all()
    else:
        platform_counts = []
    
    # Rating distribution (only if there are games)
    if total_games > 0:
        rating_counts = session.exec(
            select(UserGame.personal_rating, func.count()).
            where(
                and_(
                    UserGame.user_id == current_user.id,
                    UserGame.personal_rating.is_not(None)
                )
            ).
            group_by(UserGame.personal_rating)
        ).all()
    else:
        rating_counts = []
    
    # Calculate metrics
    pile_of_shame = status_counts.get(PlayStatus.NOT_STARTED, 0)
    completed_games = (
        status_counts.get(PlayStatus.COMPLETED, 0) +
        status_counts.get(PlayStatus.MASTERED, 0) +
        status_counts.get(PlayStatus.DOMINATED, 0)
    )
    completion_rate = (completed_games / total_games * 100) if total_games > 0 else 0
    
    # Average rating (only if there are games)
    if total_games > 0:
        avg_rating_result = session.exec(
            select(func.avg(UserGame.personal_rating)).
            where(
                and_(
                    UserGame.user_id == current_user.id,
                    UserGame.personal_rating.is_not(None)
                )
            )
        ).one()
    else:
        avg_rating_result = None
    
    # Total hours played
    total_hours = session.exec(
        select(func.sum(UserGame.hours_played)).
        where(UserGame.user_id == current_user.id)
    ).one() or 0
    
    # Ownership stats
    ownership_stats = {}
    for ownership_status in OwnershipStatus:
        count = session.exec(
            select(func.count()).where(
                and_(
                    UserGame.user_id == current_user.id,
                    UserGame.ownership_status == ownership_status
                )
            )
        ).one()
        ownership_stats[ownership_status] = count
    
    # Genre stats (from game data)
    genre_stats = {}
    if total_games > 0:
        genre_data = session.exec(
            select(Game.genre, func.count()).
            join(UserGame).
            where(UserGame.user_id == current_user.id).
            group_by(Game.genre)
        ).all()
        genre_stats = {genre: count for genre, count in genre_data}
    
    return CollectionStatsResponse(
        total_games=total_games,
        completion_stats=status_counts,
        ownership_stats=ownership_stats,
        platform_stats={name: count for name, count in platform_counts},
        genre_stats=genre_stats,
        by_status=status_counts,
        by_platform={name: count for name, count in platform_counts},
        by_rating={str(rating): count for rating, count in rating_counts},
        pile_of_shame=pile_of_shame,
        completion_rate=round(completion_rate, 2),
        average_rating=float(avg_rating_result) if avg_rating_result else None,
        total_hours_played=total_hours
    )


@router.put("/bulk-update", response_model=SuccessResponse)
async def bulk_update_user_games(
    bulk_data: BulkStatusUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Bulk update multiple user games."""
    
    # Get user games to update
    user_games = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id.in_(bulk_data.user_game_ids),
                UserGame.user_id == current_user.id
            )
        )
    ).all()
    
    found_ids = {user_game.id for user_game in user_games}
    update_count = 0
    failed_count = 0
    
    # Apply updates
    for user_game in user_games:
        updated = False
        
        if bulk_data.play_status is not None:
            user_game.play_status = bulk_data.play_status
            updated = True
        
        if bulk_data.personal_rating is not None:
            user_game.personal_rating = bulk_data.personal_rating
            updated = True
        
        if bulk_data.is_loved is not None:
            user_game.is_loved = bulk_data.is_loved
            updated = True
        
        if updated:
            user_game.updated_at = datetime.now(timezone.utc)
            update_count += 1
    
    # Calculate failed count
    failed_count = len(bulk_data.user_game_ids) - len(found_ids)
    
    session.commit()
    
    return SuccessResponse(
        message="Bulk update completed successfully",
        updated_count=update_count,
        failed_count=failed_count
    )


@router.delete("/bulk-delete", response_model=SuccessResponse)
async def bulk_delete_user_games(
    bulk_data: BulkDeleteRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Bulk delete multiple user games."""
    
    # Get user games to delete
    user_games = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id.in_(bulk_data.user_game_ids),
                UserGame.user_id == current_user.id
            )
        )
    ).all()
    
    found_ids = {user_game.id for user_game in user_games}
    delete_count = len(user_games)
    failed_count = len(bulk_data.user_game_ids) - len(found_ids)
    
    # Delete the user games
    for user_game in user_games:
        session.delete(user_game)
    
    session.commit()
    
    return SuccessResponse(
        message="Bulk deletion completed successfully",
        deleted_count=delete_count,
        failed_count=failed_count
    )


@router.get("/{user_game_id}", response_model=UserGameResponse)
async def get_user_game(
    user_game_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get a specific user game by ID."""
    
    user_game = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not user_game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User game not found"
        )
    
    return user_game


@router.post("/", response_model=UserGameResponse, status_code=status.HTTP_201_CREATED)
async def add_game_to_collection(
    user_game_data: UserGameCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Add a game to user's collection."""
    
    logger.info(f"Adding game to collection for user {current_user.id}: {user_game_data.model_dump()}")
    
    # Check if game exists
    game = session.get(Game, user_game_data.game_id)
    if not game:
        logger.warning(f"Game not found: {user_game_data.game_id}")
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    # Check if user already has this game
    existing_user_game = session.exec(
        select(UserGame).where(
            and_(
                UserGame.user_id == current_user.id,
                UserGame.game_id == user_game_data.game_id
            )
        )
    ).first()
    
    if existing_user_game:
        logger.error(
            f"409 CONFLICT - User game addition failed due to existing collection entry. "
            f"User: {current_user.username} ({current_user.id}) | "
            f"Game ID: {user_game_data.game_id} | "
            f"Game title: '{game.title}' | "
            f"Existing user game ID: {existing_user_game.id} | "
            f"Existing entry created: {existing_user_game.created_at} | "
            f"Existing ownership status: {existing_user_game.ownership_status} | "
            f"Existing play status: {existing_user_game.play_status} | "
            f"Requested ownership status: {user_game_data.ownership_status} | "
            f"Requested play status: {user_game_data.play_status}"
        )
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Game already exists in your collection"
        )
    
    # Create user game
    new_user_game = UserGame(
        user_id=current_user.id,
        game_id=user_game_data.game_id,
        ownership_status=user_game_data.ownership_status,
        personal_rating=user_game_data.personal_rating,
        is_loved=user_game_data.is_loved,
        play_status=user_game_data.play_status,
        hours_played=user_game_data.hours_played,
        personal_notes=user_game_data.personal_notes,
        acquired_date=user_game_data.acquired_date
    )
    
    logger.info(f"Created UserGame object with ID: {new_user_game.id}")
    session.add(new_user_game)
    logger.info("Added UserGame to session")
    
    session.commit()
    logger.info("Committed UserGame to database")
    
    session.refresh(new_user_game)
    logger.info(f"Refreshed UserGame - final ID: {new_user_game.id}")
    
    # Add platform associations if provided
    if user_game_data.platforms:
        logger.info(f"Adding {len(user_game_data.platforms)} platforms to user game")
        for platform_data in user_game_data.platforms:
            # Validate platform exists
            platform = session.get(Platform, platform_data.platform_id)
            if not platform:
                logger.warning(f"Platform not found: {platform_data.platform_id}")
                continue
            
            # Validate storefront exists if provided
            if platform_data.storefront_id:
                storefront = session.get(Storefront, platform_data.storefront_id)
                if not storefront:
                    logger.warning(f"Storefront not found: {platform_data.storefront_id}")
                    continue
            
            # Create complete platform association
            platform_assoc = UserGamePlatform(
                user_game_id=new_user_game.id,
                platform_id=platform_data.platform_id,
                storefront_id=platform_data.storefront_id,
                store_game_id=platform_data.store_game_id,
                store_url=str(platform_data.store_url) if platform_data.store_url else None,
                is_available=platform_data.is_available
            )
            session.add(platform_assoc)
            logger.info(f"Added platform {platform_data.platform_id} with storefront {platform_data.storefront_id} to user game {new_user_game.id}")
        
        session.commit()
        logger.info("Committed platform associations")
        session.refresh(new_user_game)
    
    logger.info(f"Successfully created user game {new_user_game.id} for user {current_user.id}")
    return new_user_game


@router.put("/{user_game_id}", response_model=UserGameResponse)
async def update_user_game(
    user_game_id: str,
    user_game_data: UserGameUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Update user's game collection entry."""
    
    user_game = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not user_game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User game not found"
        )
    
    # Update fields
    update_data = user_game_data.model_dump(exclude_unset=True)
    
    for field, value in update_data.items():
        setattr(user_game, field, value)
    
    user_game.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(user_game)
    
    return user_game


@router.put("/{user_game_id}/progress", response_model=UserGameResponse)
async def update_game_progress(
    user_game_id: str,
    progress_data: ProgressUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Update game progress and play status."""
    
    user_game = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not user_game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User game not found"
        )
    
    # Update progress fields
    user_game.play_status = progress_data.play_status
    
    if progress_data.hours_played is not None:
        user_game.hours_played = progress_data.hours_played
    
    if progress_data.personal_notes is not None:
        user_game.personal_notes = progress_data.personal_notes
    
    user_game.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(user_game)
    
    return user_game


@router.delete("/{user_game_id}", response_model=SuccessResponse)
async def remove_game_from_collection(
    user_game_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Remove a game from user's collection."""
    
    user_game = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not user_game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User game not found"
        )
    
    session.delete(user_game)
    session.commit()
    
    return SuccessResponse(message="User game deleted successfully")


@router.get("/{user_game_id}/platforms", response_model=List[UserGamePlatformResponse])
async def get_user_game_platforms(
    user_game_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Get platform associations for a user game."""
    
    # Verify user owns this game
    user_game = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not user_game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User game not found"
        )
    
    # Get platforms with joined Platform and Storefront data
    platforms = session.exec(
        select(UserGamePlatform)
        .join(Platform)
        .outerjoin(Storefront)
        .where(UserGamePlatform.user_game_id == user_game_id)
    ).all()
    
    # Manually load relationships for proper serialization
    for platform in platforms:
        if platform.platform_id:
            platform.platform = session.get(Platform, platform.platform_id)
        if platform.storefront_id:
            platform.storefront = session.get(Storefront, platform.storefront_id)
    
    return platforms


@router.post("/{user_game_id}/platforms", response_model=UserGameResponse, status_code=status.HTTP_201_CREATED)
async def add_platform_to_user_game(
    user_game_id: str,
    platform_data: UserGamePlatformCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Add a platform association to a user game."""
    
    # Verify user owns this game
    user_game = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not user_game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User game not found"
        )
    
    # Check if platform exists
    platform = session.get(Platform, platform_data.platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Check if storefront exists (if provided)
    if platform_data.storefront_id:
        storefront = session.get(Storefront, platform_data.storefront_id)
        if not storefront:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Storefront not found"
            )
    
    # Check if association already exists (platform + storefront combination)
    existing_platform = session.exec(
        select(UserGamePlatform).where(
            and_(
                UserGamePlatform.user_game_id == user_game_id,
                UserGamePlatform.platform_id == platform_data.platform_id,
                UserGamePlatform.storefront_id == platform_data.storefront_id
            )
        )
    ).first()
    
    if existing_platform:
        storefront_name = storefront.name if platform_data.storefront_id and 'storefront' in locals() else "None"
        logger.error(
            f"409 CONFLICT - Platform association addition failed due to existing combination. "
            f"User: {current_user.username} ({current_user.id}) | "
            f"User game ID: {user_game_id} | "
            f"Game title: '{user_game.game.title}' | "
            f"Platform: {platform.name} (ID: {platform_data.platform_id}) | "
            f"Storefront: {storefront_name} (ID: {platform_data.storefront_id}) | "
            f"Existing association ID: {existing_platform.id} | "
            f"Existing association created: {existing_platform.created_at} | "
            f"Existing store game ID: {existing_platform.store_game_id} | "
            f"Existing store URL: {existing_platform.store_url} | "
            f"Requested store game ID: {platform_data.store_game_id} | "
            f"Requested store URL: {platform_data.store_url}"
        )
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Platform and storefront association already exists"
        )
    
    new_platform = UserGamePlatform(
        user_game_id=user_game_id,
        platform_id=platform_data.platform_id,
        storefront_id=platform_data.storefront_id,
        store_game_id=platform_data.store_game_id,
        store_url=str(platform_data.store_url) if platform_data.store_url else None,
        is_available=platform_data.is_available
    )
    
    session.add(new_platform)
    
    # Update ownership status automatically after platform addition
    _update_ownership_status_after_platform_change(session, user_game)
    
    session.commit()
    session.refresh(new_platform)
    
    # Return the updated user game with all relationships loaded
    updated_user_game = session.exec(
        select(UserGame).where(UserGame.id == user_game_id)
    ).first()
    
    if not updated_user_game:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve updated user game"
        )
    
    return updated_user_game


@router.put("/{user_game_id}/platforms/{platform_association_id}", response_model=UserGamePlatformResponse)
async def update_platform_association(
    user_game_id: str,
    platform_association_id: str,
    platform_data: UserGamePlatformCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Update a platform association for a user game."""
    
    # Verify user owns this game and platform association
    platform_assoc = session.exec(
        select(UserGamePlatform).join(UserGame).where(
            and_(
                UserGamePlatform.id == platform_association_id,
                UserGamePlatform.user_game_id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not platform_assoc:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform association not found"
        )
    
    # Check if platform exists
    platform = session.get(Platform, platform_data.platform_id)
    if not platform:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform not found"
        )
    
    # Check if storefront exists (if provided)
    if platform_data.storefront_id:
        storefront = session.get(Storefront, platform_data.storefront_id)
        if not storefront:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Storefront not found"
            )
    
    # Check if the updated combination would conflict with existing associations
    if (platform_assoc.platform_id != platform_data.platform_id or 
        platform_assoc.storefront_id != platform_data.storefront_id):
        existing_platform = session.exec(
            select(UserGamePlatform).where(
                and_(
                    UserGamePlatform.user_game_id == user_game_id,
                    UserGamePlatform.platform_id == platform_data.platform_id,
                    UserGamePlatform.storefront_id == platform_data.storefront_id,
                    UserGamePlatform.id != platform_association_id  # Exclude current record
                )
            )
        ).first()
        
        if existing_platform:
            storefront_name = storefront.name if platform_data.storefront_id and 'storefront' in locals() else "None" 
            old_platform = session.get(Platform, platform_assoc.platform_id)
            old_storefront = session.get(Storefront, platform_assoc.storefront_id) if platform_assoc.storefront_id else None
            logger.error(
                f"409 CONFLICT - Platform association update failed due to existing combination. "
                f"User: {current_user.username} ({current_user.id}) | "
                f"User game ID: {user_game_id} | "
                f"Platform association ID: {platform_association_id} | "
                f"Current platform: {old_platform.name if old_platform else 'Unknown'} (ID: {platform_assoc.platform_id}) | "
                f"Current storefront: {old_storefront.name if old_storefront else 'None'} (ID: {platform_assoc.storefront_id}) | "
                f"Requested platform: {platform.name} (ID: {platform_data.platform_id}) | "
                f"Requested storefront: {storefront_name} (ID: {platform_data.storefront_id}) | "
                f"Conflicting existing association ID: {existing_platform.id} | "
                f"Conflicting association created: {existing_platform.created_at} | "
                f"Update attempt vs existing combination"
            )
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail="Platform and storefront association already exists"
            )
    
    # Update the platform association
    platform_assoc.platform_id = platform_data.platform_id
    platform_assoc.storefront_id = platform_data.storefront_id
    platform_assoc.store_game_id = platform_data.store_game_id
    platform_assoc.store_url = str(platform_data.store_url) if platform_data.store_url else None
    platform_assoc.is_available = platform_data.is_available
    platform_assoc.updated_at = datetime.now(timezone.utc)
    
    session.commit()
    session.refresh(platform_assoc)
    
    # Load relationships for proper serialization
    platform_assoc.platform = session.get(Platform, platform_assoc.platform_id)
    if platform_assoc.storefront_id:
        platform_assoc.storefront = session.get(Storefront, platform_assoc.storefront_id)
    
    return platform_assoc


@router.delete("/{user_game_id}/platforms/{platform_association_id}", response_model=SuccessResponse)
async def remove_platform_from_user_game(
    user_game_id: str,
    platform_association_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Remove a platform association from a user game."""
    
    # Verify user owns this game and platform association
    platform_assoc = session.exec(
        select(UserGamePlatform).join(UserGame).where(
            and_(
                UserGamePlatform.id == platform_association_id,
                UserGamePlatform.user_game_id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not platform_assoc:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Platform association not found"
        )
    
    # Get the user game for ownership status management
    user_game = session.exec(
        select(UserGame).where(
            and_(
                UserGame.id == user_game_id,
                UserGame.user_id == current_user.id
            )
        )
    ).first()
    
    if not user_game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="User game not found"
        )
    
    session.delete(platform_assoc)
    
    # Flush pending delete operations so the count query sees the correct number of platforms
    session.flush()
    
    # Update ownership status automatically after platform removal
    _update_ownership_status_after_platform_change(session, user_game)
    
    session.commit()
    
    return SuccessResponse(message="Platform association deleted successfully")
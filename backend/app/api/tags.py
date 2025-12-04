"""
Tag management API endpoints.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session
from typing import Annotated
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..services.tag_service import TagService
from ..schemas.tag import (
    TagResponse,
    TagCreateRequest,
    TagUpdateRequest,
    TagListResponse,
    TagAssignRequest,
    TagRemoveRequest,
    BulkTagOperationRequest,
    TagUsageStatsResponse,
    TagCreateOrGetResponse
)

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/tags", tags=["Tags"])


@router.get("/", response_model=TagListResponse)
async def list_tags(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=50, ge=1, le=100, description="Number of tags per page"),
    include_game_count: bool = Query(default=True, description="Include game count in response")
):
    """
    List all tags for the current user with pagination.
    """
    logger.info(f"Listing tags for user {current_user.id}, page {page}, per_page {per_page}")
    
    try:
        tag_service = TagService(session)
        tags, total_count = tag_service.get_user_tags(
            user_id=current_user.id,
            page=page,
            per_page=per_page,
            include_game_count=include_game_count
        )
        
        # Convert to response models
        tag_responses = []
        for tag in tags:
            game_count = tag.game_count if include_game_count else None
            tag_responses.append(TagResponse(
                id=tag.id,
                user_id=tag.user_id,
                name=tag.name,
                color=tag.color,
                description=tag.description,
                created_at=tag.created_at,
                updated_at=tag.updated_at,
                game_count=game_count
            ))
        
        total_pages = (total_count + per_page - 1) // per_page
        
        return TagListResponse(
            tags=tag_responses,
            total=total_count,
            page=page,
            per_page=per_page,
            total_pages=total_pages
        )
        
    except Exception as e:
        logger.error(f"Failed to list tags for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve tags"
        )


@router.post("/", response_model=TagResponse, status_code=status.HTTP_201_CREATED)
async def create_tag(
    tag_data: TagCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Create a new tag for the current user.
    """
    logger.info(f"Creating tag '{tag_data.name}' for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        tag = tag_service.create_tag(tag_data, current_user.id)
        
        return TagResponse(
            id=tag.id,
            user_id=tag.user_id,
            name=tag.name,
            color=tag.color,
            description=tag.description,
            created_at=tag.created_at,
            updated_at=tag.updated_at,
            game_count=tag.game_count  # Use the property (set by service layer)
        )
        
    except ValueError as e:
        logger.warning(f"Validation error creating tag for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Failed to create tag for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to create tag"
        )


@router.post("/create-or-get", response_model=TagCreateOrGetResponse)
async def create_or_get_tag(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    name: str = Query(..., description="Tag name"),
    color: str = Query(default=None, description="Tag color (optional)")
):
    """
    Create a new tag or get existing one by name (for inline tag creation).
    """
    logger.info(f"Creating or getting tag '{name}' for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        tag, was_created = tag_service.create_or_get_tag(name, current_user.id, color)
        
        return TagCreateOrGetResponse(
            tag=TagResponse(
                id=tag.id,
                user_id=tag.user_id,
                name=tag.name,
                color=tag.color,
                description=tag.description,
                created_at=tag.created_at,
                updated_at=tag.updated_at,
                game_count=tag.game_count  # Use the property
            ),
            created=was_created
        )
        
    except ValueError as e:
        logger.warning(f"Validation error creating/getting tag for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Failed to create/get tag for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to create or get tag"
        )


@router.get("/{tag_id}", response_model=TagResponse)
async def get_tag(
    tag_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Get a specific tag by ID.
    """
    logger.info(f"Getting tag {tag_id} for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        tag = tag_service.get_tag_by_id(tag_id, current_user.id)
        
        if not tag:
            logger.warning(f"Tag {tag_id} not found for user {current_user.id}")
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Tag not found"
            )
        
        return TagResponse(
            id=tag.id,
            user_id=tag.user_id,
            name=tag.name,
            color=tag.color,
            description=tag.description,
            created_at=tag.created_at,
            updated_at=tag.updated_at,
            game_count=tag.game_count  # Use the property
        )
        
    except HTTPException:
        # Re-raise HTTP exceptions
        raise
    except Exception as e:
        logger.error(f"Failed to get tag {tag_id} for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve tag"
        )


@router.put("/{tag_id}", response_model=TagResponse)
async def update_tag(
    tag_id: str,
    tag_data: TagUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Update an existing tag.
    """
    logger.info(f"Updating tag {tag_id} for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        tag = tag_service.update_tag(tag_id, tag_data, current_user.id)
        
        return TagResponse(
            id=tag.id,
            user_id=tag.user_id,
            name=tag.name,
            color=tag.color,
            description=tag.description,
            created_at=tag.created_at,
            updated_at=tag.updated_at,
            game_count=tag.game_count  # Use the property
        )
        
    except ValueError as e:
        logger.warning(f"Validation error updating tag {tag_id} for user {current_user.id}: {e}")
        if "not found" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=str(e)
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(e)
            )
    except Exception as e:
        logger.error(f"Failed to update tag {tag_id} for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to update tag"
        )


@router.delete("/bulk-remove", status_code=status.HTTP_200_OK)
async def bulk_remove_tags(
    request: BulkTagOperationRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Bulk remove tags from multiple games.
    """
    logger.info(f"Bulk removing {len(request.tag_ids)} tags from {len(request.user_game_ids)} games for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        total_removed = 0
        
        for user_game_id in request.user_game_ids:
            removed_count = tag_service.remove_tags_from_game(
                user_game_id=user_game_id,
                tag_ids=request.tag_ids,
                user_id=current_user.id
            )
            total_removed += removed_count
        
        return {
            "message": "Successfully completed bulk tag removal",
            "total_removed_associations": total_removed,
            "games_processed": len(request.user_game_ids),
            "tags_per_game": len(request.tag_ids)
        }
        
    except ValueError as e:
        logger.warning(f"Validation error in bulk tag removal for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Failed to bulk remove tags for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to bulk remove tags"
        )


@router.delete("/{tag_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_tag(
    tag_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Delete a tag and all its associations.
    """
    logger.info(f"Deleting tag {tag_id} for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        deleted = tag_service.delete_tag(tag_id, current_user.id)
        
        if not deleted:
            logger.warning(f"Tag {tag_id} not found for user {current_user.id}")
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Tag not found"
            )
        
        return None
        
    except HTTPException:
        # Re-raise HTTP exceptions
        raise
    except Exception as e:
        logger.error(f"Failed to delete tag {tag_id} for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to delete tag"
        )


@router.get("/usage/stats", response_model=TagUsageStatsResponse)
async def get_tag_usage_stats(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Get comprehensive tag usage statistics for the current user.
    """
    logger.info(f"Getting tag usage stats for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        stats = tag_service.get_tag_usage_stats(current_user.id)
        
        # Convert tag objects to response models
        popular_tags = [
            TagResponse(
                id=tag.id,
                user_id=tag.user_id,
                name=tag.name,
                color=tag.color,
                description=tag.description,
                created_at=tag.created_at,
                updated_at=tag.updated_at,
                game_count=tag.game_count  # Use the property (set by service layer)
            )
            for tag in stats["popular_tags"]
        ]
        
        unused_tags = [
            TagResponse(
                id=tag.id,
                user_id=tag.user_id,
                name=tag.name,
                color=tag.color,
                description=tag.description,
                created_at=tag.created_at,
                updated_at=tag.updated_at,
                game_count=tag.game_count  # Use the property (will be 0 for unused tags)
            )
            for tag in stats["unused_tags"]
        ]
        
        return TagUsageStatsResponse(
            total_tags=stats["total_tags"],
            total_tagged_games=stats["total_tagged_games"],
            average_tags_per_game=stats["average_tags_per_game"],
            tag_usage=stats["tag_usage"],
            popular_tags=popular_tags,
            unused_tags=unused_tags
        )
        
    except Exception as e:
        logger.error(f"Failed to get tag usage stats for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve tag usage statistics"
        )


# Game tag assignment endpoints (these would typically be in user_games.py but are here for cohesion)

@router.post("/assign/{user_game_id}", status_code=status.HTTP_200_OK)
async def assign_tags_to_game(
    user_game_id: str,
    request: TagAssignRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Assign tags to a user game.
    """
    logger.info(f"Assigning {len(request.tag_ids)} tags to game {user_game_id} for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        associations = tag_service.assign_tags_to_game(
            user_game_id=user_game_id,
            tag_ids=request.tag_ids,
            user_id=current_user.id
        )
        
        return {
            "message": f"Successfully assigned {len(associations)} tags to game",
            "new_associations": len(associations),
            "total_requested": len(request.tag_ids)
        }
        
    except ValueError as e:
        logger.warning(f"Validation error assigning tags to game {user_game_id} for user {current_user.id}: {e}")
        if "not found" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=str(e)
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(e)
            )
    except Exception as e:
        logger.error(f"Failed to assign tags to game {user_game_id} for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to assign tags to game"
        )


@router.delete("/remove/{user_game_id}", status_code=status.HTTP_200_OK)
async def remove_tags_from_game(
    user_game_id: str,
    request: TagRemoveRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Remove tags from a user game.
    """
    logger.info(f"Removing {len(request.tag_ids)} tags from game {user_game_id} for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        removed_count = tag_service.remove_tags_from_game(
            user_game_id=user_game_id,
            tag_ids=request.tag_ids,
            user_id=current_user.id
        )
        
        return {
            "message": f"Successfully removed {removed_count} tags from game",
            "removed_associations": removed_count,
            "total_requested": len(request.tag_ids)
        }
        
    except ValueError as e:
        logger.warning(f"Validation error removing tags from game {user_game_id} for user {current_user.id}: {e}")
        if "not found" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=str(e)
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(e)
            )
    except Exception as e:
        logger.error(f"Failed to remove tags from game {user_game_id} for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to remove tags from game"
        )


@router.post("/bulk-assign", status_code=status.HTTP_200_OK)
async def bulk_assign_tags(
    request: BulkTagOperationRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """
    Bulk assign tags to multiple games.
    """
    logger.info(f"Bulk assigning {len(request.tag_ids)} tags to {len(request.user_game_ids)} games for user {current_user.id}")
    
    try:
        tag_service = TagService(session)
        total_created = 0
        
        for user_game_id in request.user_game_ids:
            associations = tag_service.assign_tags_to_game(
                user_game_id=user_game_id,
                tag_ids=request.tag_ids,
                user_id=current_user.id
            )
            total_created += len(associations)
        
        return {
            "message": "Successfully completed bulk tag assignment",
            "total_new_associations": total_created,
            "games_processed": len(request.user_game_ids),
            "tags_per_game": len(request.tag_ids)
        }
        
    except ValueError as e:
        logger.warning(f"Validation error in bulk tag assignment for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Failed to bulk assign tags for user {current_user.id}: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to bulk assign tags"
        )
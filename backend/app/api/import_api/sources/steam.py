"""
Steam import endpoints using the new import framework.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session
from typing import Annotated, Optional, Dict, Any
import logging

from ....core.database import get_session
from ....core.security import get_current_user
from ....models.user import User
from ....services.import_sources.steam import create_steam_import_service
from ...dependencies import verify_steam_games_enabled
from ...schemas.import_schemas import (
    SourceConfigResponse,
    VerificationRequest,
    VerificationResponse,
    LibraryPreviewResponse,
    ImportGamesList,
    ImportStartResponse,
    GameMatchRequest,
    GameMatchResponse,
    GameSyncResponse,
    GameIgnoreResponse,
    BulkOperationResponse
)
from ...schemas.steam import (
    SteamConfigRequest,
    SteamConfigResponse,
    SteamVerificationRequest,
    SteamVerificationResponse,
    SteamVanityResolveRequest,
    SteamVanityResolveResponse
)

router = APIRouter()
logger = logging.getLogger(__name__)


def get_steam_service(session: Annotated[Session, Depends(get_session)]):
    """Dependency to get Steam import service."""
    return create_steam_import_service(session)


@router.get("/availability")
async def get_steam_availability(
    current_user: Annotated[User, Depends(get_current_user)],
    session: Annotated[Session, Depends(get_session)]
) -> Dict[str, Any]:
    """
    Check if Steam import feature is available for the current user.
    
    Returns simple boolean response indicating availability.
    If not available, includes reason for debugging.
    
    This endpoint uses the same logic as the verify_steam_games_enabled dependency
    but returns a JSON response instead of raising HTTP exceptions.
    """
    try:
        logger.info(f"Checking Steam availability for user {current_user.id}")
        
        # Use existing dependency logic - if this doesn't raise an exception, Steam is available
        verify_steam_games_enabled(current_user, session)
        
        logger.info(f"Steam is available for user {current_user.id}")
        return {
            "available": True,
            "reason": None
        }
    except HTTPException as e:
        logger.info(f"Steam not available for user {current_user.id}: {e.detail}")
        return {
            "available": False,
            "reason": e.detail
        }
    except Exception as e:
        logger.error(f"Unexpected error checking Steam availability for user {current_user.id}: {str(e)}")
        return {
            "available": False,
            "reason": "Internal error checking Steam availability"
        }


@router.get("/status")
async def get_steam_status(
    current_user: Annotated[User, Depends(get_current_user)],
    steam_service = Depends(get_steam_service)
) -> Dict[str, Any]:
    """Get Steam import source status."""
    try:
        config = await steam_service.get_config(current_user.id)
        return {
            "available": True,
            "configured": config.is_configured,
            "verified": config.is_verified,
            "last_configured": config.configured_at,
            "last_import": config.last_import
        }
    except Exception as e:
        logger.error(f"Error getting Steam status for user {current_user.id}: {str(e)}")
        return {
            "available": True,
            "configured": False,
            "verified": False,
            "last_configured": None,
            "last_import": None
        }


# Configuration endpoints
@router.get("/config", response_model=SteamConfigResponse)
async def get_steam_config(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> SteamConfigResponse:
    """Get current Steam configuration."""
    try:
        config = await steam_service.get_config(current_user.id)
        
        return SteamConfigResponse(
            has_api_key=config.config_data.get("has_api_key", False),
            api_key_masked=config.config_data.get("api_key_masked"),
            steam_id=config.config_data.get("steam_id"),
            is_verified=config.is_verified,
            configured_at=config.configured_at
        )
    except Exception as e:
        logger.error(f"Error getting Steam config for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve Steam configuration"
        )


@router.put("/config", response_model=SteamConfigResponse)
async def update_steam_config(
    request: SteamConfigRequest,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> SteamConfigResponse:
    """Update Steam configuration."""
    try:
        config = await steam_service.set_config(current_user.id, {
            "web_api_key": request.web_api_key,
            "steam_id": request.steam_id
        })
        
        return SteamConfigResponse(
            has_api_key=config.config_data.get("has_api_key", False),
            api_key_masked=config.config_data.get("api_key_masked", ""),
            steam_id=config.config_data.get("steam_id"),
            is_verified=config.is_verified,
            configured_at=config.configured_at
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error updating Steam config for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to update Steam configuration"
        )


@router.delete("/config")
async def delete_steam_config(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> Dict[str, str]:
    """Delete Steam configuration."""
    try:
        success = await steam_service.delete_config(current_user.id)
        if success:
            return {"message": "Steam configuration deleted successfully"}
        else:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to delete Steam configuration"
            )
    except Exception as e:
        logger.error(f"Error deleting Steam config for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to delete Steam configuration"
        )


@router.post("/verify", response_model=SteamVerificationResponse)
async def verify_steam_config(
    request: SteamVerificationRequest,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> SteamVerificationResponse:
    """Verify Steam configuration without saving."""
    try:
        is_valid, error_message, verification_data = await steam_service.verify_config({
            "web_api_key": request.web_api_key,
            "steam_id": request.steam_id
        })
        
        return SteamVerificationResponse(
            is_valid=is_valid,
            error_message=error_message,
            steam_user_info=verification_data.get("steam_user_info") if verification_data else None
        )
    except Exception as e:
        logger.error(f"Error verifying Steam config for user {current_user.id}: {str(e)}")
        return SteamVerificationResponse(
            is_valid=False,
            error_message="Verification failed due to an unexpected error"
        )


@router.post("/resolve-vanity", response_model=SteamVanityResolveResponse)
async def resolve_vanity_url(
    request: SteamVanityResolveRequest,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> SteamVanityResolveResponse:
    """Resolve Steam vanity URL to Steam ID."""
    try:
        success, steam_id, error_message = await steam_service.resolve_vanity_url(
            current_user.id, 
            request.vanity_url
        )
        
        return SteamVanityResolveResponse(
            success=success,
            steam_id=steam_id,
            error_message=error_message
        )
    except Exception as e:
        logger.error(f"Error resolving vanity URL for user {current_user.id}: {str(e)}")
        return SteamVanityResolveResponse(
            success=False,
            error_message="Failed to resolve vanity URL"
        )


# Library operations
@router.get("/library", response_model=LibraryPreviewResponse)
async def get_steam_library_preview(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> LibraryPreviewResponse:
    """Get preview of Steam library."""
    try:
        preview = await steam_service.get_library_preview(current_user.id)
        
        return LibraryPreviewResponse(
            total_games=preview["total_games"],
            preview_games=preview["preview_games"],
            source_info=preview.get("steam_user_info")
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error getting Steam library preview for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve Steam library preview"
        )


# Games management
@router.get("/games", response_model=ImportGamesList)
async def list_steam_games(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service),
    offset: int = Query(default=0, ge=0),
    limit: int = Query(default=100, ge=1, le=1000),
    status_filter: Optional[str] = Query(default=None, pattern="^(unmatched|matched|ignored|synced)$", description="Filter by status: unmatched, matched, ignored, synced"),
    search: Optional[str] = Query(default=None, description="Search game names")
) -> ImportGamesList:
    """List imported Steam games with filtering."""
    try:
        games, total = await steam_service.list_games(
            user_id=current_user.id,
            offset=offset,
            limit=limit,
            status_filter=status_filter,
            search=search
        )
        
        return ImportGamesList(
            games=[
                {
                    "id": game.id,
                    "external_id": game.external_id,
                    "name": game.name,
                    "igdb_id": game.igdb_id,
                    "igdb_title": game.igdb_title,
                    "game_id": game.game_id,
                    "user_game_id": game.user_game_id,
                    "ignored": game.ignored,
                    "created_at": game.created_at,
                    "updated_at": game.updated_at
                } for game in games
            ],
            total=total,
            offset=offset,
            limit=limit
        )
    except Exception as e:
        logger.error(f"Error listing Steam games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve Steam games"
        )


@router.post("/games/import", response_model=ImportStartResponse)
async def import_steam_library(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> ImportStartResponse:
    """Start Steam library import."""
    try:
        result = await steam_service.import_library(current_user.id)
        
        return ImportStartResponse(
            message=f"Steam library import completed: {result.imported_count} games imported, {result.auto_matched_count} auto-matched",
            job_id="",  # TODO: Return actual job ID when background jobs are implemented
            started=True
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error importing Steam library for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to import Steam library"
        )


# Individual game operations
@router.put("/games/{game_id}/match", response_model=GameMatchResponse)
async def match_steam_game(
    game_id: str,
    request: GameMatchRequest,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> GameMatchResponse:
    """Match Steam game to IGDB entry."""
    try:
        game = await steam_service.match_game(current_user.id, game_id, request.igdb_id)
        
        return GameMatchResponse(
            message="Game matched successfully" if request.igdb_id else "Game match cleared",
            game={
                "id": game.id,
                "external_id": game.external_id,
                "name": game.name,
                "igdb_id": game.igdb_id,
                "igdb_title": game.igdb_title,
                "game_id": game.game_id,
                "user_game_id": game.user_game_id,
                "ignored": game.ignored,
                "created_at": game.created_at,
                "updated_at": game.updated_at
            }
        )
    except FileNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e)
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error matching Steam game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to match game"
        )


@router.post("/games/{game_id}/auto-match", response_model=GameMatchResponse)
async def auto_match_steam_game(
    game_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> GameMatchResponse:
    """Automatically match Steam game to IGDB."""
    try:
        result = await steam_service.auto_match_game(current_user.id, game_id)
        
        if result.matched:
            # Get updated game info
            games, _ = await steam_service.list_games(
                user_id=current_user.id,
                offset=0,
                limit=1,
                status_filter=None,
                search=None
            )
            game = next((g for g in games if g.id == game_id), None)
            
            if game:
                return GameMatchResponse(
                    message=f"Game auto-matched successfully with confidence {result.confidence_score:.2f}",
                    game={
                        "id": game.id,
                        "external_id": game.external_id,
                        "name": game.name,
                        "igdb_id": game.igdb_id,
                        "igdb_title": game.igdb_title,
                        "game_id": game.game_id,
                        "user_game_id": game.user_game_id,
                        "ignored": game.ignored,
                        "created_at": game.created_at,
                        "updated_at": game.updated_at
                    }
                )
        
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=result.error_message or "Auto-matching failed"
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error auto-matching Steam game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to auto-match game"
        )


@router.post("/games/{game_id}/sync", response_model=GameSyncResponse)
async def sync_steam_game(
    game_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> GameSyncResponse:
    """Sync Steam game to main collection."""
    try:
        result = await steam_service.sync_game(current_user.id, game_id)
        
        if result.action == "failed":
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=result.error_message or "Failed to sync game"
            )
        
        # Get updated game info
        games, _ = await steam_service.list_games(
            user_id=current_user.id,
            offset=0,
            limit=1,
            status_filter=None,
            search=None
        )
        game = next((g for g in games if g.id == game_id), None)
        
        if not game:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Game not found after sync"
            )
        
        return GameSyncResponse(
            message="Game synced to collection successfully",
            game={
                "id": game.id,
                "external_id": game.external_id,
                "name": game.name,
                "igdb_id": game.igdb_id,
                "igdb_title": game.igdb_title,
                "game_id": game.game_id,
                "user_game_id": game.user_game_id,
                "ignored": game.ignored,
                "created_at": game.created_at,
                "updated_at": game.updated_at
            },
            user_game_id=result.user_game_id,
            action=result.action
        )
        
    except FileNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e)
        )
    except PermissionError as e:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail=str(e)
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error syncing Steam game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to sync game"
        )


@router.post("/games/{game_id}/unsync", response_model=GameSyncResponse)
async def unsync_steam_game(
    game_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> GameSyncResponse:
    """Remove Steam game from main collection."""
    try:
        game = await steam_service.unsync_game(current_user.id, game_id)
        
        return GameSyncResponse(
            message="Game removed from collection successfully",
            game={
                "id": game.id,
                "external_id": game.external_id,
                "name": game.name,
                "igdb_id": game.igdb_id,
                "igdb_title": game.igdb_title,
                "game_id": game.game_id,
                "user_game_id": game.user_game_id,
                "ignored": game.ignored,
                "created_at": game.created_at,
                "updated_at": game.updated_at
            },
            user_game_id=None,
            action="removed"
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error unsyncing Steam game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to unsync game"
        )


@router.put("/games/{game_id}/ignore", response_model=GameIgnoreResponse)
async def toggle_ignore_steam_game(
    game_id: str,
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> GameIgnoreResponse:
    """Toggle ignore status of Steam game."""
    try:
        game = await steam_service.ignore_game(current_user.id, game_id)
        
        return GameIgnoreResponse(
            message=f"Game {'ignored' if game.ignored else 'unignored'} successfully",
            game={
                "id": game.id,
                "external_id": game.external_id,
                "name": game.name,
                "igdb_id": game.igdb_id,
                "igdb_title": game.igdb_title,
                "game_id": game.game_id,
                "user_game_id": game.user_game_id,
                "ignored": game.ignored,
                "created_at": game.created_at,
                "updated_at": game.updated_at
            },
            ignored=game.ignored
        )
        
    except FileNotFoundError as e:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=str(e)
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error toggling ignore for Steam game {game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to toggle ignore status"
        )


# Bulk operations
@router.post("/games/auto-match", response_model=BulkOperationResponse)
async def auto_match_all_steam_games(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> BulkOperationResponse:
    """Auto-match all unmatched Steam games."""
    try:
        result = await steam_service.auto_match_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Auto-matching completed: {result.successful_operations} games matched, {result.failed_operations} failed",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            skipped_items=result.skipped_items,
            errors=result.errors
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error auto-matching all Steam games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to auto-match games"
        )


@router.post("/games/sync", response_model=BulkOperationResponse)
async def sync_all_steam_games(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> BulkOperationResponse:
    """Sync all matched Steam games to collection."""
    try:
        result = await steam_service.sync_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Bulk sync completed: {result.successful_operations} games synced, {result.failed_operations} failed",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            skipped_items=result.skipped_items,
            errors=result.errors
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error syncing all Steam games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to sync games"
        )


@router.post("/games/unsync", response_model=BulkOperationResponse)
async def unsync_all_steam_games(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> BulkOperationResponse:
    """Remove all synced Steam games from collection."""
    try:
        result = await steam_service.unsync_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Bulk unsync completed: {result.successful_operations} games removed from collection",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            errors=result.errors
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error unsyncing all Steam games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to unsync games"
        )


@router.put("/games/unignore-all", response_model=BulkOperationResponse)
async def unignore_all_steam_games(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> BulkOperationResponse:
    """Unignore all ignored Steam games."""
    try:
        result = await steam_service.unignore_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Bulk unignore completed: {result.successful_operations} games restored",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            errors=result.errors
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error unignoring all Steam games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to unignore games"
        )


@router.put("/games/unmatch-all", response_model=BulkOperationResponse)
async def unmatch_all_steam_games(
    current_user: Annotated[User, Depends(verify_steam_games_enabled)],
    steam_service = Depends(get_steam_service)
) -> BulkOperationResponse:
    """Remove IGDB matches from all Steam games."""
    try:
        result = await steam_service.unmatch_all_games(current_user.id)
        
        return BulkOperationResponse(
            message=f"Bulk unmatch completed: {result.successful_operations} games unmatched",
            total_processed=result.total_processed,
            successful_operations=result.successful_operations,
            failed_operations=result.failed_operations,
            errors=result.errors
        )
        
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error unmatching all Steam games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to unmatch games"
        )
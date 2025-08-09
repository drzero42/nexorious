"""
Steam Games management endpoints.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query, BackgroundTasks
from sqlmodel import Session, select, and_, func
from typing import Annotated, Optional
import logging
import json

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.steam_game import SteamGame
from ..services.steam import SteamAuthenticationError, SteamAPIError
from ..services.steam_games import create_steam_games_service, SteamGamesServiceError
from ..api.schemas.steam import (
    SteamGameResponse,
    SteamGamesListResponse,
    SteamGamesImportStartedResponse,
    SteamGameMatchRequest,
    SteamGameMatchResponse,
    SteamGameSyncRequest,
    SteamGameSyncResponse,
    SteamGameIgnoreResponse,
    SteamGamesBulkSyncResponse,
    SteamGamesBulkUnignoreResponse,
    SteamGamesAutoMatchResponse,
    SteamGameAutoMatchSingleResponse
)

router = APIRouter(prefix="/steam-games", tags=["Steam Games"])
logger = logging.getLogger(__name__)


def _get_user_steam_config(user: User) -> dict:
    """Get user's Steam configuration from preferences."""
    try:
        preferences = user.preferences
        steam_config = preferences.get("steam", {})
        logger.debug(f"Retrieved Steam config for user {user.id}: has_api_key={bool(steam_config.get('web_api_key'))}, has_steam_id={bool(steam_config.get('steam_id'))}, is_verified={steam_config.get('is_verified', False)}")
        return steam_config
    except (json.JSONDecodeError, TypeError) as e:
        logger.error(f"Error parsing preferences for user {user.id}: {str(e)}")
        return {}


async def import_steam_library_task(user_id: str, steam_config: dict):
    """Background task to import Steam library for a user using SteamGamesService."""
    logger.info(f"Starting Steam library import for user {user_id}")
    
    # Create new database session for background task
    from ..core.database import get_session
    session_gen = get_session()
    session = next(session_gen)
    
    try:
        # Create Steam games service
        steam_games_service = create_steam_games_service(session)
        
        # Use service to import Steam library with automatic IGDB matching
        result = await steam_games_service.import_steam_library(
            user_id=user_id,
            steam_config=steam_config,
            enable_auto_matching=True  # Enable the new automatic matching feature
        )
        
        logger.info(
            f"Steam library import completed for user {user_id}: "
            f"{result.imported_count} imported, {result.skipped_count} skipped, "
            f"{result.auto_matched_count} auto-matched. "
            f"Errors: {len(result.errors)}"
        )
        
        # Log any errors for debugging
        for error in result.errors:
            logger.warning(f"Import error: {error}")
        
    except SteamAuthenticationError as e:
        logger.error(f"Steam authentication error during import for user {user_id}: {str(e)}")
    except SteamAPIError as e:
        logger.error(f"Steam API error during import for user {user_id}: {str(e)}")
    except SteamGamesServiceError as e:
        logger.error(f"Steam games service error during import for user {user_id}: {str(e)}")
    except Exception as e:
        logger.error(f"Unexpected error during Steam library import for user {user_id}: {str(e)}")
    finally:
        session.close()


@router.get("", response_model=SteamGamesListResponse, status_code=status.HTTP_200_OK)
async def list_steam_games(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    offset: int = Query(default=0, ge=0, description="Number of items to skip"),
    limit: int = Query(default=100, ge=1, le=1000, description="Maximum number of items to return"),
    status_filter: Optional[str] = Query(default=None, pattern="^(unmatched|matched|ignored|synced)$", description="Filter by Steam game status"),
    search: Optional[str] = Query(default=None, min_length=1, description="Search by game name")
) -> SteamGamesListResponse:
    """
    List user's Steam games with filtering and pagination.
    
    Status filter options:
    - unmatched: Games without IGDB ID that need matching
    - matched: Games with IGDB ID ready for import
    - ignored: Games marked as ignored (won't be imported)
    - synced: Games successfully imported to main collection
    """
    try:
        # Build base query for user's Steam games
        query = select(SteamGame).where(SteamGame.user_id == current_user.id)
        
        # Apply status filter
        if status_filter:
            if status_filter == "unmatched":
                query = query.where(and_(SteamGame.igdb_id.is_(None), SteamGame.ignored == False))
            elif status_filter == "matched":
                query = query.where(and_(SteamGame.igdb_id.isnot(None), SteamGame.game_id.is_(None), SteamGame.ignored == False))
            elif status_filter == "ignored":
                query = query.where(SteamGame.ignored == True)
            elif status_filter == "synced":
                query = query.where(SteamGame.game_id.isnot(None))
        
        # Apply search filter
        if search:
            search_term = f"%{search.strip().lower()}%"
            query = query.where(func.lower(SteamGame.game_name).contains(search_term))
        
        # Get total count by creating a count query from the same filters
        count_query = select(func.count(SteamGame.id)).where(SteamGame.user_id == current_user.id)
        
        # Apply same filters for count
        if status_filter:
            if status_filter == "unmatched":
                count_query = count_query.where(and_(SteamGame.igdb_id.is_(None), SteamGame.ignored == False))
            elif status_filter == "matched":
                count_query = count_query.where(and_(SteamGame.igdb_id.isnot(None), SteamGame.game_id.is_(None), SteamGame.ignored == False))
            elif status_filter == "ignored":
                count_query = count_query.where(SteamGame.ignored == True)
            elif status_filter == "synced":
                count_query = count_query.where(SteamGame.game_id.isnot(None))
        
        if search:
            search_term = f"%{search.strip().lower()}%"
            count_query = count_query.where(func.lower(SteamGame.game_name).contains(search_term))
        
        total = session.exec(count_query).first() or 0
        
        # Apply pagination and ordering
        query = query.order_by(SteamGame.game_name.asc()).offset(offset).limit(limit)
        
        # Execute query
        steam_games = session.exec(query).all()
        
        # Convert to response format
        games = []
        for steam_game in steam_games:
            games.append(SteamGameResponse(
                id=steam_game.id,
                steam_appid=steam_game.steam_appid,
                game_name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                game_id=steam_game.game_id,
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            ))
        
        logger.info(f"Retrieved {len(games)} Steam games for user {current_user.id} (total: {total})")
        
        return SteamGamesListResponse(
            total=total,
            games=games
        )
        
    except Exception as e:
        logger.error(f"Error listing Steam games for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retrieve Steam games"
        )


@router.post("/import", response_model=SteamGamesImportStartedResponse, status_code=status.HTTP_202_ACCEPTED)
async def import_steam_library(
    background_tasks: BackgroundTasks,
    current_user: Annotated[User, Depends(get_current_user)]
) -> SteamGamesImportStartedResponse:
    """
    Start background import of user's Steam library.
    
    Requires user to have configured Steam Web API key and Steam ID.
    Returns immediately while import runs in background.
    Use GET /api/steam-games to check import progress and results.
    """
    try:
        # Get user's Steam configuration
        steam_config = _get_user_steam_config(current_user)
        
        # Validate Steam configuration
        if not steam_config.get("web_api_key"):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Steam Web API key not configured. Please configure your Steam settings first."
            )
        
        if not steam_config.get("steam_id"):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Steam ID not configured. Please configure your Steam settings first."
            )
        
        if not steam_config.get("is_verified", False):
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Steam configuration not verified. Please verify your Steam settings first."
            )
        
        # Start background import task
        background_tasks.add_task(import_steam_library_task, current_user.id, steam_config)
        
        logger.info(f"Started Steam library import background task for user {current_user.id}")
        
        return SteamGamesImportStartedResponse(
            message="Steam library import started successfully. Check back later for results.",
            started=True
        )
        
    except HTTPException:
        # Re-raise HTTPExceptions (like 400 Bad Request) without modification
        raise
    except Exception as e:
        logger.error(f"Error starting Steam library import for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to start Steam library import"
        )


@router.put("/{steam_game_id}/match", response_model=SteamGameMatchResponse, status_code=status.HTTP_200_OK)
async def match_steam_game_to_igdb(
    steam_game_id: str,
    match_request: SteamGameMatchRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> SteamGameMatchResponse:
    """
    Match a Steam game to an IGDB game by setting the igdb_id field.
    
    This endpoint allows users to manually match their Steam games to IGDB games,
    which prepares them for import into the main game collection.
    
    - Set igdb_id to match a Steam game to an IGDB game
    - Set igdb_id to null to clear an existing match
    - Only the Steam game owner can perform this operation
    """
    try:
        # Create Steam games service
        steam_games_service = create_steam_games_service(session)
        
        # Use service to handle IGDB matching
        steam_game, message = await steam_games_service.match_steam_game_to_igdb(
            steam_game_id=steam_game_id,
            igdb_id=match_request.igdb_id,
            user_id=current_user.id
        )
        
        return SteamGameMatchResponse(
            message=message,
            steam_game=SteamGameResponse(
                id=steam_game.id,
                steam_appid=steam_game.steam_appid,
                game_name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                game_id=steam_game.game_id,
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            )
        )
        
    except SteamGamesServiceError as e:
        # Convert service errors to appropriate HTTP errors
        if "not found or access denied" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=str(e)
            )
        elif "invalid igdb id" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=str(e)
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )
    except Exception as e:
        logger.error(f"Error matching Steam game {steam_game_id} to IGDB for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to match Steam game to IGDB"
        )


@router.post("/{steam_game_id}/sync", response_model=SteamGameSyncResponse, status_code=status.HTTP_200_OK)
async def sync_steam_game_to_collection(
    steam_game_id: str,
    sync_request: SteamGameSyncRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> SteamGameSyncResponse:
    """
    Sync a matched Steam game to the user's main collection.
    
    This endpoint performs a complete sync flow:
    1. Validates Steam game exists and is matched to IGDB
    2. Ensures the Game record exists in database (creates from IGDB if needed)
    3. Creates or updates UserGame relationship with Steam platform/storefront
    4. Updates Steam game sync tracking (sets game_id on first sync)
    
    This operation is idempotent - can be run multiple times safely.
    """
    logger.info(f"🎮 [Steam Sync API] Starting sync for steam_game_id: {steam_game_id}, user_id: {current_user.id}")
    logger.debug(f"🎮 [Steam Sync API] Sync request body: {sync_request}")
    logger.debug(f"🎮 [Steam Sync API] Current user: {current_user.username} (admin: {current_user.is_admin})")
    
    try:
        # Create Steam games service
        steam_games_service = create_steam_games_service(session)
        logger.debug(f"🎮 [Steam Sync API] Steam games service created successfully")
        
        # Use service to handle sync operation
        logger.debug(f"🎮 [Steam Sync API] Calling service sync method...")
        result = await steam_games_service.sync_steam_game_to_collection(
            steam_game_id=steam_game_id,
            user_id=current_user.id
        )
        logger.info(f"🎮 [Steam Sync API] Service returned result: action={result.action}")
        logger.debug(f"🎮 [Steam Sync API] Full service result: {result}")
        
        if result.action == "failed":
            logger.error(f"🎮 [Steam Sync API] Service reported failure: {result.error_message}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=result.error_message or "Failed to sync Steam game to collection"
            )
        
        # Create success message
        if result.action == "created_new":
            message = f"Successfully added Steam game '{result.steam_game_name}' to your collection"
        else:
            message = f"Updated Steam game '{result.steam_game_name}' in your collection (ensured Steam platform association)"
        
        logger.info(f"🎮 [Steam Sync API] Success message: {message}")
        
        # Get updated Steam game for response
        steam_game = session.get(SteamGame, steam_game_id)
        if not steam_game:
            logger.error(f"🎮 [Steam Sync API] Steam game not found in DB after sync: {steam_game_id}")
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam game not found after sync"
            )
        
        logger.debug(f"🎮 [Steam Sync API] Retrieved updated Steam game from DB: {steam_game.game_name} (game_id: {steam_game.game_id})")
        
        response = SteamGameSyncResponse(
            message=message,
            steam_game=SteamGameResponse(
                id=steam_game.id,
                steam_appid=steam_game.steam_appid,
                game_name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                game_id=steam_game.game_id,
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            ),
            user_game_id=result.user_game_id,
            action=result.action
        )
        
        logger.info(f"🎮 [Steam Sync API] Sync completed successfully")
        return response
        
    except SteamGamesServiceError as e:
        logger.error(f"🎮 [Steam Sync API] SteamGamesServiceError: {str(e)}")
        # Convert service errors to appropriate HTTP errors
        if "not found or access denied" in str(e).lower():
            logger.warning(f"🎮 [Steam Sync API] Steam game not found or access denied: {steam_game_id}")
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=str(e)
            )
        elif "must be matched to igdb" in str(e).lower():
            logger.warning(f"🎮 [Steam Sync API] Steam game not matched to IGDB: {steam_game_id}")
            raise HTTPException(
                status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
                detail=str(e)
            )
        else:
            logger.error(f"🎮 [Steam Sync API] Unexpected service error: {str(e)}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )
    except HTTPException as e:
        logger.warning(f"🎮 [Steam Sync API] HTTPException raised: {e.status_code} - {e.detail}")
        # Re-raise HTTPExceptions without modification
        raise
    except Exception as e:
        logger.error(f"🎮 [Steam Sync API] Unexpected error syncing Steam game {steam_game_id} for user {current_user.id}: {str(e)}")
        logger.exception("🎮 [Steam Sync API] Exception details:")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to sync Steam game to collection"
        )


@router.post("/sync", response_model=SteamGamesBulkSyncResponse, status_code=status.HTTP_200_OK)
async def sync_all_matched_steam_games(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> SteamGamesBulkSyncResponse:
    """
    Sync all matched Steam games to the user's main collection.
    
    This endpoint performs a comprehensive sync operation for all Steam games that:
    1. Have an IGDB ID (are matched to IGDB games)
    2. Are not ignored
    3. Have not been synced to the main collection yet (game_id is null)
    
    For each qualifying Steam game, this operation:
    1. Ensures the Game record exists in database (creates from IGDB if needed)
    2. Creates or updates UserGame relationship with Steam platform/storefront
    3. Updates Steam game sync tracking (sets game_id)
    
    This operation is idempotent and can be run multiple times safely.
    Returns detailed statistics about the sync operation.
    """
    try:
        # Create Steam games service
        steam_games_service = create_steam_games_service(session)
        
        # Use service to handle bulk sync operation
        results = await steam_games_service.sync_all_matched_games(current_user.id)
        
        # Create success message  
        if results.total_processed == 0:
            message = "No matched Steam games found that need syncing"
        elif results.successful_syncs == results.total_processed:
            message = f"Successfully synced all {results.successful_syncs} matched Steam games to your collection"
        elif results.successful_syncs > 0:
            message = f"Synced {results.successful_syncs} of {results.total_processed} Steam games to your collection ({results.failed_syncs} failed)"
        else:
            message = f"Failed to sync any of the {results.total_processed} matched Steam games"
        
        logger.info(f"Bulk Steam game sync completed for user {current_user.id}: {results.successful_syncs} successful, {results.failed_syncs} failed")
        
        return SteamGamesBulkSyncResponse(
            message=message,
            total_processed=results.total_processed,
            successful_syncs=results.successful_syncs,
            failed_syncs=results.failed_syncs,
            skipped_games=results.skipped_games,
            errors=results.errors
        )
        
    except SteamGamesServiceError as e:
        # Convert service errors to appropriate HTTP errors
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error during bulk Steam game sync for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to sync Steam games to collection"
        )


@router.put("/{steam_game_id}/ignore", response_model=SteamGameIgnoreResponse, status_code=status.HTTP_200_OK)
async def toggle_steam_game_ignored_status(
    steam_game_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> SteamGameIgnoreResponse:
    """
    Toggle the ignored status of a Steam game.
    
    This endpoint allows users to mark Steam games as ignored (won't be imported)
    or un-ignore them to make them available for import again.
    
    - Toggles the ignored field (True ↔ False)
    - Only the Steam game owner can perform this operation
    - Updates the updated_at timestamp
    """
    try:
        # Create Steam games service
        steam_games_service = create_steam_games_service(session)
        
        # Use service to handle ignore toggle
        steam_game, message, ignored_status = steam_games_service.toggle_steam_game_ignored(
            steam_game_id=steam_game_id,
            user_id=current_user.id
        )
        
        return SteamGameIgnoreResponse(
            message=message,
            steam_game=SteamGameResponse(
                id=steam_game.id,
                steam_appid=steam_game.steam_appid,
                game_name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                game_id=steam_game.game_id,
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            ),
            ignored=ignored_status
        )
        
    except SteamGamesServiceError as e:
        # Convert service errors to appropriate HTTP errors
        if "not found or access denied" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=str(e)
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )
    except Exception as e:
        logger.error(f"Error toggling ignored status for Steam game {steam_game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to toggle Steam game ignored status"
        )


@router.put("/unignore-all", response_model=SteamGamesBulkUnignoreResponse, status_code=status.HTTP_200_OK)
async def unignore_all_steam_games(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> SteamGamesBulkUnignoreResponse:
    """
    Unignore all ignored Steam games for the current user.
    
    This endpoint restores all ignored Steam games back to an active state,
    making them available for matching and importing to the main collection.
    
    Returns:
        SteamGamesBulkUnignoreResponse: Statistics about the bulk unignore operation
        
    Raises:
        HTTPException: If the operation fails
    """
    try:
        steam_games_service = create_steam_games_service(session)
        
        logger.info(f"Starting bulk unignore operation for user {current_user.id}")
        
        # Perform bulk unignore operation
        results = await steam_games_service.unignore_all_steam_games(current_user.id)
        
        # Create success message based on results
        if results.total_processed == 0:
            message = "No ignored Steam games found to unignore."
        elif results.failed_unignores == 0:
            message = f"Successfully unignored {results.successful_unignores} Steam game(s). These games are now available for matching and import."
        else:
            message = f"Unignore operation completed with mixed results: {results.successful_unignores} successful, {results.failed_unignores} failed."
        
        logger.info(f"Bulk Steam game unignore completed for user {current_user.id}: {results.successful_unignores} successful, {results.failed_unignores} failed")
        
        return SteamGamesBulkUnignoreResponse(
            message=message,
            total_processed=results.total_processed,
            successful_unignores=results.successful_unignores,
            failed_unignores=results.failed_unignores,
            errors=results.errors
        )
        
    except SteamGamesServiceError as e:
        logger.error(f"Steam games service error during bulk unignore for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e)
        )
    except Exception as e:
        logger.error(f"Error during bulk unignore for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to unignore Steam games"
        )


@router.post("/auto-match", response_model=SteamGamesAutoMatchResponse, status_code=status.HTTP_200_OK)
async def retry_auto_matching_for_unmatched_games(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> SteamGamesAutoMatchResponse:
    """
    Manually retry auto-matching for all unmatched Steam games.
    
    This endpoint allows users to manually trigger the auto-matching process
    for all Steam games that haven't been matched to IGDB games yet.
    
    The process:
    1. Finds all unmatched Steam games (no IGDB ID and not ignored)
    2. Searches IGDB for potential matches using fuzzy string matching
    3. Automatically matches games with confidence >= 80%
    4. Creates Game records in the database for new matches
    5. Updates Steam games with IGDB IDs
    
    This is useful after:
    - Initial Steam library import when some games weren't matched
    - Adding new games to Steam library that need matching
    - When IGDB database has been updated with new games
    """
    try:
        # Create Steam games service
        steam_games_service = create_steam_games_service(session)
        
        # Use service to retry auto-matching
        results = await steam_games_service.retry_auto_matching_for_unmatched_games(current_user.id)
        
        # Create success message  
        if results.total_processed == 0:
            message = "No unmatched Steam games found to process"
        elif results.successful_matches == results.total_processed:
            message = f"Successfully auto-matched all {results.successful_matches} unmatched Steam games"
        elif results.successful_matches > 0:
            message = f"Auto-matched {results.successful_matches} of {results.total_processed} unmatched Steam games ({results.failed_matches} failed, {results.skipped_games} skipped due to low confidence)"
        else:
            message = f"Unable to auto-match any of the {results.total_processed} unmatched Steam games (all had low confidence or failed)"
        
        logger.info(f"Manual auto-matching completed for user {current_user.id}: {results.successful_matches} successful, {results.failed_matches} failed, {results.skipped_games} skipped")
        
        return SteamGamesAutoMatchResponse(
            message=message,
            total_processed=results.total_processed,
            successful_matches=results.successful_matches,
            failed_matches=results.failed_matches,
            skipped_games=results.skipped_games,
            errors=results.errors
        )
        
    except SteamGamesServiceError as e:
        # Convert service errors to appropriate HTTP errors
        # Rollback session to prevent PendingRollbackError
        try:
            session.rollback()
        except Exception:
            pass
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e)
        )
    except Exception as e:
        # Rollback session to prevent PendingRollbackError
        try:
            session.rollback()
        except Exception:
            pass
        # Use a string literal instead of accessing current_user.id to avoid session issues
        logger.error(f"Error during manual auto-matching: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to retry auto-matching for Steam games"
        )


@router.post("/{steam_game_id}/auto-match", response_model=SteamGameAutoMatchSingleResponse, status_code=status.HTTP_200_OK)
async def auto_match_single_steam_game(
    steam_game_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
) -> SteamGameAutoMatchSingleResponse:
    """
    Automatically match a single Steam game to IGDB.
    
    This endpoint attempts to automatically find and match a Steam game to an IGDB entry
    using fuzzy string matching with confidence scoring. The game will only be matched
    if the confidence score meets or exceeds the configured threshold (typically 80%).
    
    - Only unmatched Steam games can be auto-matched
    - The game must belong to the current user
    - Uses the same matching algorithm as the bulk auto-match operation
    - Returns detailed matching results including confidence score
    """
    try:
        # Create Steam games service
        steam_games_service = create_steam_games_service(session)
        
        # Use service to handle single auto-match operation
        result = await steam_games_service.auto_match_single_steam_game(
            steam_game_id=steam_game_id,
            user_id=current_user.id
        )
        
        # Create success/failure message
        if result.matched:
            message = f"Successfully auto-matched '{result.steam_game_name}' to IGDB with {result.confidence_score:.1%} confidence"
        elif result.error_message:
            message = f"Failed to auto-match '{result.steam_game_name}': {result.error_message}"
        else:
            message = f"No suitable IGDB match found for '{result.steam_game_name}' (confidence too low)"
        
        logger.info(f"Single auto-match completed for Steam game {steam_game_id} for user {current_user.id}: matched={result.matched}")
        
        # Get updated Steam game for response
        steam_game = session.get(SteamGame, steam_game_id)
        if not steam_game:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam game not found after auto-match operation"
            )
        
        return SteamGameAutoMatchSingleResponse(
            message=message,
            steam_game=SteamGameResponse(
                id=steam_game.id,
                steam_appid=steam_game.steam_appid,
                game_name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                game_id=steam_game.game_id,
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            ),
            matched=result.matched,
            confidence=result.confidence_score
        )
        
    except SteamGamesServiceError as e:
        # Convert service errors to appropriate HTTP errors
        if "not found or access denied" in str(e).lower():
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=str(e)
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=str(e)
            )
    except Exception as e:
        logger.error(f"Error during single auto-match for Steam game {steam_game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to auto-match Steam game"
        )
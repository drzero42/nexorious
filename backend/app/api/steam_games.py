"""
Steam Games management endpoints.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query, BackgroundTasks
from sqlmodel import Session, select, and_, func, or_, col
from datetime import datetime, timezone
from typing import Annotated, Optional, List
import logging
import json

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.game import Game
from ..models.user_game import UserGame, UserGamePlatform, OwnershipStatus, PlayStatus
from ..models.platform import Platform, Storefront
from ..services.steam import create_steam_service, SteamAuthenticationError, SteamAPIError
from ..services.igdb import IGDBService
from ..api.schemas.steam import (
    SteamGameResponse,
    SteamGamesListResponse,
    SteamGamesImportStartedResponse,
    SteamGameMatchRequest,
    SteamGameMatchResponse,
    SteamGameSyncRequest,
    SteamGameSyncResponse,
    SteamGameIgnoreResponse,
    SteamGamesBulkSyncResponse
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
    """Background task to import Steam library for a user."""
    logger.info(f"Starting Steam library import for user {user_id}")
    
    # Create new database session for background task
    from ..core.database import get_session
    session_gen = get_session()
    session = next(session_gen)
    
    try:
        # Get user from database
        user = session.get(User, user_id)
        if not user:
            logger.error(f"User {user_id} not found during Steam library import")
            return
        
        # Create Steam service
        steam_service = create_steam_service(steam_config["web_api_key"])
        
        # Get Steam library
        steam_games = await steam_service.get_owned_games(steam_config["steam_id"])
        logger.info(f"Retrieved {len(steam_games)} games from Steam for user {user_id}")
        
        imported_count = 0
        skipped_count = 0
        
        # Import games with deduplication
        for steam_game in steam_games:
            try:
                # Check if game already exists for this user
                existing_query = select(SteamGame).where(
                    and_(
                        SteamGame.user_id == user_id,
                        SteamGame.steam_appid == steam_game.appid
                    )
                )
                existing_game = session.exec(existing_query).first()
                
                if existing_game:
                    # Update existing game name in case it changed
                    if existing_game.game_name != steam_game.name:
                        existing_game.game_name = steam_game.name
                        existing_game.updated_at = datetime.now(timezone.utc)
                        session.add(existing_game)
                    skipped_count += 1
                    logger.debug(f"Skipped existing Steam game {steam_game.appid} for user {user_id}")
                else:
                    # Create new Steam game record
                    new_steam_game = SteamGame(
                        user_id=user_id,
                        steam_appid=steam_game.appid,
                        game_name=steam_game.name,
                        created_at=datetime.now(timezone.utc),
                        updated_at=datetime.now(timezone.utc)
                    )
                    session.add(new_steam_game)
                    imported_count += 1
                    logger.debug(f"Added new Steam game {steam_game.appid} ({steam_game.name}) for user {user_id}")
                    
            except Exception as e:
                logger.error(f"Error processing Steam game {steam_game.appid} for user {user_id}: {str(e)}")
                continue
        
        # Commit all changes
        session.commit()
        logger.info(f"Steam library import completed for user {user_id}: {imported_count} imported, {skipped_count} skipped")
        
    except SteamAuthenticationError as e:
        logger.error(f"Steam authentication error during import for user {user_id}: {str(e)}")
    except SteamAPIError as e:
        logger.error(f"Steam API error during import for user {user_id}: {str(e)}")
    except Exception as e:
        logger.error(f"Unexpected error during Steam library import for user {user_id}: {str(e)}")
        session.rollback()
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
        # Find the Steam game and verify ownership
        steam_game_query = select(SteamGame).where(
            and_(
                SteamGame.id == steam_game_id,
                SteamGame.user_id == current_user.id
            )
        )
        steam_game = session.exec(steam_game_query).first()
        
        if not steam_game:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam game not found or access denied"
            )
        
        # If igdb_id is provided, validate it exists in games table
        if match_request.igdb_id is not None:
            game_query = select(Game).where(Game.igdb_id == match_request.igdb_id)
            existing_game = session.exec(game_query).first()
            
            if not existing_game:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Invalid IGDB ID. Game must exist in the main games collection first."
                )
        
        # Update the Steam game's IGDB ID
        old_igdb_id = steam_game.igdb_id
        steam_game.igdb_id = match_request.igdb_id
        steam_game.updated_at = datetime.now(timezone.utc)
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        # Create response message
        if match_request.igdb_id is None:
            if old_igdb_id:
                message = f"Cleared IGDB match for Steam game '{steam_game.game_name}'"
            else:
                message = f"No IGDB match to clear for Steam game '{steam_game.game_name}'"
        else:
            if old_igdb_id:
                message = f"Updated IGDB match for Steam game '{steam_game.game_name}'"
            else:
                message = f"Successfully matched Steam game '{steam_game.game_name}' to IGDB"
        
        logger.info(f"Steam game {steam_game_id} IGDB match updated: {old_igdb_id} -> {match_request.igdb_id} by user {current_user.id}")
        
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
        
    except HTTPException:
        # Re-raise HTTPExceptions (like 404, 400) without modification
        raise
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
    try:
        # Step 1: Find and validate Steam game
        steam_game_query = select(SteamGame).where(
            and_(
                SteamGame.id == steam_game_id,
                SteamGame.user_id == current_user.id
            )
        )
        steam_game = session.exec(steam_game_query).first()
        
        if not steam_game:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam game not found or access denied"
            )
        
        # Step 2: Validate Steam game has IGDB match
        if not steam_game.igdb_id:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Steam game must be matched to IGDB before syncing to collection"
            )
        
        # Step 3: Check if Game record exists, create if needed
        game_query = select(Game).where(Game.igdb_id == steam_game.igdb_id)
        game = session.exec(game_query).first()
        
        if not game:
            # Create Game record from IGDB
            logger.info(f"Creating Game record from IGDB for igdb_id {steam_game.igdb_id}")
            igdb_service = IGDBService()
            try:
                game_metadata = await igdb_service.get_game_by_id(steam_game.igdb_id)
                if not game_metadata:
                    raise HTTPException(
                        status_code=status.HTTP_400_BAD_REQUEST,
                        detail=f"Could not fetch game data from IGDB for ID {steam_game.igdb_id}"
                    )
                
                # Create new Game record from GameMetadata
                game = Game(
                    igdb_id=steam_game.igdb_id,
                    title=game_metadata.title,
                    summary=game_metadata.description,
                    release_date=game_metadata.release_date,
                    cover_art_url=game_metadata.cover_art_url,
                    igdb_rating=game_metadata.rating_average,
                    igdb_rating_count=game_metadata.rating_count,
                    time_to_beat_hastily=game_metadata.hastily,
                    time_to_beat_normally=game_metadata.normally,
                    time_to_beat_completely=game_metadata.completely,
                    igdb_platforms=game_metadata.igdb_platform_ids,
                    igdb_platform_names=game_metadata.platform_names,
                    created_at=datetime.now(timezone.utc),
                    updated_at=datetime.now(timezone.utc)
                )
                session.add(game)
                session.flush()  # Get the game ID
                logger.info(f"Created Game record {game.id} from IGDB ID {steam_game.igdb_id}")
                
            except Exception as e:
                logger.error(f"Error creating Game from IGDB ID {steam_game.igdb_id}: {str(e)}")
                raise HTTPException(
                    status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                    detail="Failed to create game from IGDB data"
                )
        
        # Step 4: Check if UserGame relationship exists
        user_game_query = select(UserGame).where(
            and_(
                UserGame.user_id == current_user.id,
                UserGame.game_id == game.id
            )
        )
        user_game = session.exec(user_game_query).first()
        
        action = "updated_existing"
        if not user_game:
            # Create new UserGame
            user_game = UserGame(
                user_id=current_user.id,
                game_id=game.id,
                ownership_status=OwnershipStatus.OWNED,
                play_status=PlayStatus.NOT_STARTED,
                is_loved=False,
                hours_played=0,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            session.add(user_game)
            session.flush()  # Get the user_game ID
            action = "created_new"
            logger.info(f"Created new UserGame {user_game.id} for game {game.id}")
        
        # Step 5: Ensure Steam platform/storefront association exists
        # First get platform and storefront IDs
        pc_platform_query = select(Platform).where(Platform.name == "pc-windows")
        pc_platform = session.exec(pc_platform_query).first()
        
        steam_storefront_query = select(Storefront).where(Storefront.name == "steam")
        steam_storefront = session.exec(steam_storefront_query).first()
        
        if not pc_platform or not steam_storefront:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="PC (Windows) platform or Steam storefront not found in database"
            )
        
        # Check if platform association already exists
        platform_association_query = select(UserGamePlatform).where(
            and_(
                UserGamePlatform.user_game_id == user_game.id,
                UserGamePlatform.platform_id == pc_platform.id,
                UserGamePlatform.storefront_id == steam_storefront.id
            )
        )
        existing_association = session.exec(platform_association_query).first()
        
        if not existing_association:
            # Create Steam platform association
            platform_association = UserGamePlatform(
                user_game_id=user_game.id,
                platform_id=pc_platform.id,
                storefront_id=steam_storefront.id,
                is_available=True,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            session.add(platform_association)
            logger.info(f"Added Steam platform association for UserGame {user_game.id}")
        
        # Step 6: Update Steam game sync tracking (only if not already set)
        if steam_game.game_id is None:
            steam_game.game_id = game.id
            steam_game.updated_at = datetime.now(timezone.utc)
            session.add(steam_game)
            logger.info(f"Set SteamGame {steam_game_id} game_id to {game.id}")
        
        # Commit all changes
        session.commit()
        session.refresh(steam_game)
        
        # Create success message
        if action == "created_new":
            message = f"Successfully added Steam game '{steam_game.game_name}' to your collection"
        else:
            message = f"Updated Steam game '{steam_game.game_name}' in your collection (ensured Steam platform association)"
        
        logger.info(f"Steam game sync completed: {steam_game_id} -> UserGame {user_game.id} ({action}) for user {current_user.id}")
        
        return SteamGameSyncResponse(
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
            user_game_id=user_game.id,
            action=action
        )
        
    except HTTPException:
        # Re-raise HTTPExceptions (like 404, 400) without modification
        raise
    except Exception as e:
        logger.error(f"Error syncing Steam game {steam_game_id} to collection for user {current_user.id}: {str(e)}")
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
        # Find all matched Steam games that haven't been synced yet
        matched_steam_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == current_user.id,
                SteamGame.igdb_id.isnot(None),  # Has IGDB match
                SteamGame.game_id.is_(None),     # Not yet synced
                SteamGame.ignored == False       # Not ignored
            )
        )
        matched_steam_games = session.exec(matched_steam_games_query).all()
        
        total_processed = len(matched_steam_games)
        successful_syncs = 0
        failed_syncs = 0
        skipped_games = 0
        errors = []
        
        logger.info(f"Starting bulk sync for {total_processed} matched Steam games for user {current_user.id}")
        
        if total_processed == 0:
            return SteamGamesBulkSyncResponse(
                message="No matched Steam games found that need syncing",
                total_processed=0,
                successful_syncs=0,
                failed_syncs=0,
                skipped_games=0,
                errors=[]
            )
        
        # Get platform and storefront objects once for efficiency
        pc_platform_query = select(Platform).where(Platform.name == "pc-windows")
        pc_platform = session.exec(pc_platform_query).first()
        
        steam_storefront_query = select(Storefront).where(Storefront.name == "steam")
        steam_storefront = session.exec(steam_storefront_query).first()
        
        if not pc_platform or not steam_storefront:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="PC (Windows) platform or Steam storefront not found in database"
            )
        
        # Process each Steam game
        for steam_game in matched_steam_games:
            try:
                # Check if Game record exists, create if needed
                game_query = select(Game).where(Game.igdb_id == steam_game.igdb_id)
                game = session.exec(game_query).first()
                
                if not game:
                    # Create Game record from IGDB
                    logger.debug(f"Creating Game record from IGDB for igdb_id {steam_game.igdb_id}")
                    igdb_service = IGDBService()
                    try:
                        game_metadata = await igdb_service.get_game_by_id(steam_game.igdb_id)
                        if not game_metadata:
                            errors.append(f"Could not fetch game data from IGDB for '{steam_game.game_name}' (IGDB ID: {steam_game.igdb_id})")
                            failed_syncs += 1
                            continue
                        
                        # Create new Game record from GameMetadata
                        game = Game(
                            igdb_id=steam_game.igdb_id,
                            title=game_metadata.title,
                            summary=game_metadata.description,
                            release_date=game_metadata.release_date,
                            cover_art_url=game_metadata.cover_art_url,
                            igdb_rating=game_metadata.rating_average,
                            igdb_rating_count=game_metadata.rating_count,
                            time_to_beat_hastily=game_metadata.hastily,
                            time_to_beat_normally=game_metadata.normally,
                            time_to_beat_completely=game_metadata.completely,
                            igdb_platforms=game_metadata.igdb_platform_ids,
                            igdb_platform_names=game_metadata.platform_names,
                            created_at=datetime.now(timezone.utc),
                            updated_at=datetime.now(timezone.utc)
                        )
                        session.add(game)
                        session.flush()  # Get the game ID
                        logger.debug(f"Created Game record {game.id} from IGDB ID {steam_game.igdb_id}")
                        
                    except Exception as e:
                        logger.error(f"Error creating Game from IGDB ID {steam_game.igdb_id}: {str(e)}")
                        errors.append(f"Failed to create game record for '{steam_game.game_name}': {str(e)}")
                        failed_syncs += 1
                        continue
                
                # Check if UserGame relationship exists
                user_game_query = select(UserGame).where(
                    and_(
                        UserGame.user_id == current_user.id,
                        UserGame.game_id == game.id
                    )
                )
                user_game = session.exec(user_game_query).first()
                
                if not user_game:
                    # Create new UserGame
                    user_game = UserGame(
                        user_id=current_user.id,
                        game_id=game.id,
                        ownership_status=OwnershipStatus.OWNED,
                        play_status=PlayStatus.NOT_STARTED,
                        is_loved=False,
                        hours_played=0,
                        created_at=datetime.now(timezone.utc),
                        updated_at=datetime.now(timezone.utc)
                    )
                    session.add(user_game)
                    session.flush()  # Get the user_game ID
                    logger.debug(f"Created new UserGame {user_game.id} for game {game.id}")
                
                # Ensure Steam platform/storefront association exists
                platform_association_query = select(UserGamePlatform).where(
                    and_(
                        UserGamePlatform.user_game_id == user_game.id,
                        UserGamePlatform.platform_id == pc_platform.id,
                        UserGamePlatform.storefront_id == steam_storefront.id
                    )
                )
                existing_association = session.exec(platform_association_query).first()
                
                if not existing_association:
                    # Create Steam platform association
                    platform_association = UserGamePlatform(
                        user_game_id=user_game.id,
                        platform_id=pc_platform.id,
                        storefront_id=steam_storefront.id,
                        is_available=True,
                        created_at=datetime.now(timezone.utc),
                        updated_at=datetime.now(timezone.utc)
                    )
                    session.add(platform_association)
                    logger.debug(f"Added Steam platform association for UserGame {user_game.id}")
                
                # Update Steam game sync tracking
                steam_game.game_id = game.id
                steam_game.updated_at = datetime.now(timezone.utc)
                session.add(steam_game)
                
                successful_syncs += 1
                logger.debug(f"Successfully synced Steam game '{steam_game.game_name}' to collection")
                
            except Exception as e:
                logger.error(f"Error syncing Steam game '{steam_game.game_name}' (ID: {steam_game.id}): {str(e)}")
                errors.append(f"Failed to sync '{steam_game.game_name}': {str(e)}")
                failed_syncs += 1
                continue
        
        # Commit all changes
        session.commit()
        
        # Create success message
        if successful_syncs == total_processed:
            message = f"Successfully synced all {successful_syncs} matched Steam games to your collection"
        elif successful_syncs > 0:
            message = f"Synced {successful_syncs} of {total_processed} Steam games to your collection ({failed_syncs} failed)"
        else:
            message = f"Failed to sync any of the {total_processed} matched Steam games"
        
        logger.info(f"Bulk Steam game sync completed for user {current_user.id}: {successful_syncs} successful, {failed_syncs} failed")
        
        return SteamGamesBulkSyncResponse(
            message=message,
            total_processed=total_processed,
            successful_syncs=successful_syncs,
            failed_syncs=failed_syncs,
            skipped_games=skipped_games,  # Always 0 in this implementation as we pre-filter
            errors=errors
        )
        
    except HTTPException:
        # Re-raise HTTPExceptions (like 500 for missing platform/storefront)
        raise
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
        # Find the Steam game and verify ownership
        steam_game_query = select(SteamGame).where(
            and_(
                SteamGame.id == steam_game_id,
                SteamGame.user_id == current_user.id
            )
        )
        steam_game = session.exec(steam_game_query).first()
        
        if not steam_game:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam game not found or access denied"
            )
        
        # Toggle the ignored status
        old_ignored = steam_game.ignored
        steam_game.ignored = not old_ignored
        steam_game.updated_at = datetime.now(timezone.utc)
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        # Create appropriate success message
        if steam_game.ignored:
            message = f"Steam game '{steam_game.game_name}' is now ignored and won't be imported"
        else:
            message = f"Steam game '{steam_game.game_name}' is no longer ignored and can be imported"
        
        logger.info(f"Steam game {steam_game_id} ignored status toggled: {old_ignored} -> {steam_game.ignored} by user {current_user.id}")
        
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
            ignored=steam_game.ignored
        )
        
    except HTTPException:
        # Re-raise HTTPExceptions (like 404) without modification
        raise
    except Exception as e:
        logger.error(f"Error toggling ignored status for Steam game {steam_game_id} for user {current_user.id}: {str(e)}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Failed to toggle Steam game ignored status"
        )
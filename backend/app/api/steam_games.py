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
from ..services.steam import create_steam_service, SteamAuthenticationError, SteamAPIError
from ..api.schemas.steam import (
    SteamGameResponse,
    SteamGamesListResponse,
    SteamGamesImportStartedResponse
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
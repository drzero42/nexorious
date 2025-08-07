"""
Steam Games management endpoints.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, and_, func, or_, col
from datetime import datetime, timezone
from typing import Annotated, Optional, List
import logging

from ..core.database import get_session
from ..core.security import get_current_user
from ..models.user import User
from ..models.steam_game import SteamGame
from ..api.schemas.steam import (
    SteamGameResponse,
    SteamGamesListResponse
)

router = APIRouter(prefix="/steam-games", tags=["Steam Games"])
logger = logging.getLogger(__name__)


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
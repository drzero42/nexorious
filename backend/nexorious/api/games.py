"""
Game management endpoints for CRUD operations and metadata handling.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, or_, and_, func
from datetime import datetime, timezone, date
import json
import re
import logging
from typing import Annotated, Optional, List

from ..core.database import get_session
from ..core.security import get_current_user, get_current_admin_user
from ..models.user import User
from ..models.game import Game, GameAlias
from ..services.igdb import IGDBService, IGDBError, TwitchAuthError
from ..api.dependencies import get_igdb_service_dependency
from ..api.schemas.game import (
    GameCreateRequest,
    GameUpdateRequest,
    GameResponse,
    GameSearchRequest,
    GameListResponse,
    GameAliasCreateRequest,
    GameAliasResponse,
    IGDBSearchRequest,
    IGDBGameCandidate,
    IGDBSearchResponse,
    GameMetadataAcceptRequest,
    MetadataStatusResponse,
    MetadataRefreshRequest,
    MetadataRefreshResponse,
    MetadataPopulateRequest,
    MetadataPopulateResponse,
    MetadataComparisonResponse,
    BulkMetadataRequest,
    BulkMetadataResponse,
    BulkCoverArtDownloadRequest
)
from ..api.schemas.common import SuccessResponse, PaginationParams

router = APIRouter(prefix="/games", tags=["Games"])
logger = logging.getLogger(__name__)


def parse_date_string(date_string: Optional[str]) -> Optional[date]:
    """Parse a date string into a Python date object."""
    if not date_string:
        return None
    
    try:
        # Handle YYYY-MM-DD format
        if len(date_string) == 10 and date_string.count('-') == 2:
            year, month, day = date_string.split('-')
            return date(int(year), int(month), int(day))
        # Handle YYYY format
        elif len(date_string) == 4:
            return date(int(date_string), 1, 1)
        else:
            return None
    except (ValueError, TypeError):
        return None


@router.get("/", response_model=GameListResponse)
async def list_games(
    session: Annotated[Session, Depends(get_session)],
    page: int = Query(default=1, ge=1, description="Page number"),
    per_page: int = Query(default=20, ge=1, le=100, description="Items per page"),
    q: Optional[str] = Query(default=None, description="Search query"),
    genre: Optional[str] = Query(default=None, description="Filter by genre"),
    developer: Optional[str] = Query(default=None, description="Filter by developer"),
    publisher: Optional[str] = Query(default=None, description="Filter by publisher"),
    release_year: Optional[int] = Query(default=None, description="Filter by release year"),
    is_verified: Optional[bool] = Query(default=None, description="Filter by verification status"),
    sort_by: Optional[str] = Query(default="title", description="Sort field"),
    sort_order: Optional[str] = Query(default="asc", pattern="^(asc|desc)$", description="Sort order")
):
    """List games with optional search and filtering."""
    
    # Build query
    query = select(Game)
    
    # Apply filters
    filters = []
    
    if q:
        # Search in title, description, and aliases
        search_filter = or_(
            Game.title.icontains(q),
            Game.description.icontains(q) if Game.description.is_not(None) else False
        )
        filters.append(search_filter)
    
    if genre:
        filters.append(Game.genre.icontains(genre))
    
    if developer:
        filters.append(Game.developer.icontains(developer))
    
    if publisher:
        filters.append(Game.publisher.icontains(publisher))
    
    if release_year:
        filters.append(func.extract('year', Game.release_date) == release_year)
    
    if is_verified is not None:
        filters.append(Game.is_verified == is_verified)
    
    if filters:
        query = query.where(and_(*filters))
    
    # Apply sorting
    sort_field = getattr(Game, sort_by, Game.title)
    if sort_order == "desc":
        query = query.order_by(sort_field.desc())
    else:
        query = query.order_by(sort_field.asc())
    
    # Get total count
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()
    
    # Apply pagination
    offset = (page - 1) * per_page
    games = session.exec(query.offset(offset).limit(per_page)).all()
    
    # Calculate pages
    pages = (total + per_page - 1) // per_page
    
    return GameListResponse(
        games=games,
        total=total,
        page=page,
        per_page=per_page,
        pages=pages
    )


@router.get("/{game_id}", response_model=GameResponse)
async def get_game(
    game_id: str,
    session: Annotated[Session, Depends(get_session)]
):
    """Get a specific game by ID."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    # Get aliases for this game
    aliases = session.exec(
        select(GameAlias).where(GameAlias.game_id == game_id)
    ).all()
    
    # Convert to response format
    game_response = GameResponse.model_validate(game)
    game_response.aliases = aliases
    
    return game_response


@router.post("/", response_model=GameResponse, status_code=status.HTTP_201_CREATED)
async def create_game(
    game_data: GameCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Create a new game."""
    
    # Check for duplicate title
    existing_game = session.exec(select(Game).where(Game.title == game_data.title)).first()
    if existing_game:
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail=f"Game with title '{game_data.title}' already exists"
        )
    
    # Create game
    new_game = Game(
        title=game_data.title,
        description=game_data.description,
        genre=game_data.genre,
        developer=game_data.developer,
        publisher=game_data.publisher,
        release_date=game_data.release_date,
        cover_art_url=str(game_data.cover_art_url) if game_data.cover_art_url else None,
        estimated_playtime_hours=game_data.estimated_playtime_hours,
        howlongtobeat_main=game_data.howlongtobeat_main,
        howlongtobeat_extra=game_data.howlongtobeat_extra,
        howlongtobeat_completionist=game_data.howlongtobeat_completionist,
        igdb_id=game_data.igdb_id,
        game_metadata=json.dumps(game_data.metadata),
        is_verified=current_user.is_admin  # Auto-verify if created by admin
    )
    
    session.add(new_game)
    session.commit()
    session.refresh(new_game)
    
    return new_game


@router.put("/{game_id}", response_model=GameResponse)
async def update_game(
    game_id: str,
    game_data: GameUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Update an existing game."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    # Check permissions - only admin or unverified games can be edited
    if not current_user.is_admin and game.is_verified:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Cannot edit verified games. Only administrators can modify verified games."
        )
    
    # Update fields
    update_data = game_data.model_dump(exclude_unset=True)
    
    for field, value in update_data.items():
        if field == "metadata":
            setattr(game, "game_metadata", json.dumps(value))
        elif field == "cover_art_url" and value:
            setattr(game, field, str(value))
        else:
            setattr(game, field, value)
    
    game.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(game)
    
    return game


@router.delete("/{game_id}", response_model=SuccessResponse)
async def delete_game(
    game_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Delete a game (admin only)."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    # Check if game is in use
    from ..models.user_game import UserGame
    from ..models.wishlist import Wishlist
    
    user_games_count = session.exec(
        select(func.count()).where(UserGame.game_id == game_id)
    ).one()
    
    wishlist_count = session.exec(
        select(func.count()).where(Wishlist.game_id == game_id)
    ).one()
    
    if user_games_count > 0 or wishlist_count > 0:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot delete game. It is referenced by {user_games_count} user collections and {wishlist_count} wishlists."
        )
    
    session.delete(game)
    session.commit()
    
    return SuccessResponse(message="Game deleted successfully")


@router.get("/{game_id}/aliases", response_model=List[GameAliasResponse])
async def get_game_aliases(
    game_id: str,
    session: Annotated[Session, Depends(get_session)]
):
    """Get all aliases for a specific game."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    aliases = session.exec(
        select(GameAlias).where(GameAlias.game_id == game_id)
    ).all()
    
    return aliases


@router.post("/{game_id}/aliases", response_model=GameAliasResponse, status_code=status.HTTP_201_CREATED)
async def create_game_alias(
    game_id: str,
    alias_data: GameAliasCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Create an alias for a game."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    # Check if alias already exists
    existing_alias = session.exec(
        select(GameAlias).where(
            and_(
                GameAlias.game_id == game_id,
                GameAlias.alias_title == alias_data.alias_title
            )
        )
    ).first()
    
    if existing_alias:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Alias already exists for this game"
        )
    
    new_alias = GameAlias(
        game_id=game_id,
        alias_title=alias_data.alias_title,
        source=alias_data.source
    )
    
    session.add(new_alias)
    session.commit()
    session.refresh(new_alias)
    
    return new_alias


@router.delete("/{game_id}/aliases/{alias_id}", response_model=SuccessResponse)
async def delete_game_alias(
    game_id: str,
    alias_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Delete a game alias."""
    
    alias = session.get(GameAlias, alias_id)
    if not alias or alias.game_id != game_id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Alias not found"
        )
    
    session.delete(alias)
    session.commit()
    
    return SuccessResponse(message="Alias deleted successfully")


@router.post("/search/igdb", response_model=IGDBSearchResponse)
async def search_igdb(
    search_data: IGDBSearchRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """Search for games in IGDB database with fuzzy matching."""
    
    try:
        # Search games using IGDB service with fuzzy matching
        game_metadata_list = await igdb_service.search_games(
            query=search_data.query,
            limit=search_data.limit or 10,
            fuzzy_threshold=0.6
        )
        
        # Convert GameMetadata objects to IGDBGameCandidate objects
        candidates = []
        for metadata in game_metadata_list:
            # Use IGDB platform data if available, otherwise empty list
            platforms = metadata.platform_names if metadata.platform_names else []
            
            candidate = IGDBGameCandidate(
                igdb_id=metadata.igdb_id,
                title=metadata.title,
                release_date=metadata.release_date,
                cover_art_url=metadata.cover_art_url,
                description=metadata.description,
                platforms=platforms,
                howlongtobeat_main=metadata.hastily,
                howlongtobeat_extra=metadata.normally,
                howlongtobeat_completionist=metadata.completely
            )
            candidates.append(candidate)
        
        return IGDBSearchResponse(
            games=candidates,
            total=len(candidates)
        )
        
    except HTTPException:
        # Re-raise HTTPException as-is
        raise
    except TwitchAuthError as e:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"IGDB authentication failed: {str(e)}"
        )
    except IGDBError as e:
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail=f"IGDB API error: {str(e)}"
        )
    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Internal server error: {str(e)}"
        )


@router.post("/igdb-import", response_model=GameResponse, status_code=status.HTTP_201_CREATED)
async def import_from_igdb(
    import_data: GameMetadataAcceptRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency),
    download_cover_art: bool = Query(default=True, description="Automatically download cover art during import")
):
    """Import a game from IGDB with accepted metadata."""
    
    try:
        # Retrieve full game metadata from IGDB
        game_metadata = await igdb_service.get_game_by_id(import_data.igdb_id)
        
        if not game_metadata:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Game not found in IGDB"
            )
        
        # Check if game already exists in our database
        existing_game = session.exec(
            select(Game).where(Game.igdb_id == import_data.igdb_id)
        ).first()
        
        if existing_game:
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail="Game already exists in database"
            )
        
        # Create game from IGDB metadata
        game_data = {
            "title": game_metadata.title,
            "description": game_metadata.description,
            "genre": game_metadata.genre,
            "developer": game_metadata.developer,
            "publisher": game_metadata.publisher,
            "release_date": parse_date_string(game_metadata.release_date),
            "cover_art_url": game_metadata.cover_art_url,
            "rating_average": game_metadata.rating_average,
            "rating_count": game_metadata.rating_count or 0,
            "estimated_playtime_hours": game_metadata.estimated_playtime_hours,
            "howlongtobeat_main": game_metadata.hastily,
            "howlongtobeat_extra": game_metadata.normally,
            "howlongtobeat_completionist": game_metadata.completely,
            "igdb_id": game_metadata.igdb_id,
            "igdb_slug": game_metadata.igdb_slug,
            "igdb_platform_ids": json.dumps(game_metadata.igdb_platform_ids) if game_metadata.igdb_platform_ids else None,
            "game_metadata": "{}",
            "is_verified": True  # Games imported from IGDB are considered verified
        }
        
        # Apply any custom overrides from the user
        if import_data.custom_overrides:
            for key, value in import_data.custom_overrides.items():
                if key in game_data and value is not None:
                    game_data[key] = value
        
        # Create the game
        new_game = Game(**game_data)
        session.add(new_game)
        session.commit()
        session.refresh(new_game)
        
        # Download cover art if requested and available
        if download_cover_art and game_metadata.cover_art_url:
            try:
                local_url = await igdb_service.download_and_store_cover_art(
                    game_metadata.igdb_id, 
                    game_metadata.cover_art_url
                )
                if local_url:
                    new_game.cover_art_url = local_url
                    session.commit()
                    session.refresh(new_game)
            except Exception as e:
                # Log the error but don't fail the import
                logger.error(f"Failed to download cover art for game {new_game.id}: {e}")
        
        return GameResponse.model_validate(new_game)
        
    except HTTPException:
        # Re-raise HTTPException as-is (e.g., 404 Game not found)
        raise
    except TwitchAuthError as e:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"IGDB authentication failed: {str(e)}"
        )
    except IGDBError as e:
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail=f"IGDB API error: {str(e)}"
        )
    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Internal server error: {str(e)}"
        )


@router.put("/{game_id}/verify", response_model=GameResponse)
async def verify_game(
    game_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)]
):
    """Verify a game (admin only)."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    game.is_verified = True
    game.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(game)
    
    return game


@router.get("/{game_id}/metadata/status", response_model=MetadataStatusResponse)
async def get_metadata_status(
    game_id: str,
    session: Annotated[Session, Depends(get_session)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """Get metadata completeness status for a game."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    # Create GameMetadata object from game data
    from ..services.igdb import GameMetadata
    
    game_metadata = GameMetadata(
        igdb_id=game.igdb_id or "",
        igdb_slug=game.igdb_slug,
        title=game.title,
        description=game.description,
        genre=game.genre,
        developer=game.developer,
        publisher=game.publisher,
        release_date=game.release_date.isoformat() if game.release_date else None,
        cover_art_url=game.cover_art_url,
        rating_average=float(game.rating_average) if game.rating_average else None,
        rating_count=game.rating_count,
        estimated_playtime_hours=game.estimated_playtime_hours,
        hastily=game.howlongtobeat_main,
        normally=game.howlongtobeat_extra,
        completely=game.howlongtobeat_completionist
    )
    
    try:
        status_data = await igdb_service.get_metadata_completeness(game_metadata)
        return MetadataStatusResponse(**status_data)
    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to analyze metadata status: {str(e)}"
        )


@router.post("/{game_id}/metadata/refresh", response_model=MetadataRefreshResponse)
async def refresh_game_metadata(
    game_id: str,
    refresh_request: MetadataRefreshRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """Refresh game metadata from IGDB."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    if not game.igdb_id:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Game does not have IGDB ID"
        )
    
    # Check permissions - only admin or unverified games can be refreshed
    if not current_user.is_admin and game.is_verified:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Cannot refresh metadata for verified games. Only administrators can refresh verified games."
        )
    
    try:
        fresh_metadata = await igdb_service.refresh_game_metadata(game.igdb_id)
        if not fresh_metadata:
            return MetadataRefreshResponse(
                success=False,
                updated_fields=[],
                errors=["Failed to retrieve fresh metadata from IGDB"],
                game=GameResponse.model_validate(game)
            )
        
        # Update game with fresh metadata
        updated_fields = []
        errors = []
        
        fields_to_refresh = refresh_request.fields or [
            'title', 'description', 'genre', 'developer', 'publisher',
            'release_date', 'cover_art_url', 'rating_average', 'rating_count',
            'igdb_slug', 'howlongtobeat_main', 'howlongtobeat_extra', 'howlongtobeat_completionist'
        ]
        
        for field in fields_to_refresh:
            # Handle time-to-beat field mapping
            if field == 'howlongtobeat_main':
                fresh_value = getattr(fresh_metadata, 'hastily', None)
                current_value = getattr(game, field, None)
            elif field == 'howlongtobeat_extra':
                fresh_value = getattr(fresh_metadata, 'normally', None)
                current_value = getattr(game, field, None)
            elif field == 'howlongtobeat_completionist':
                fresh_value = getattr(fresh_metadata, 'completely', None)
                current_value = getattr(game, field, None)
            elif hasattr(fresh_metadata, field):
                fresh_value = getattr(fresh_metadata, field)
                current_value = getattr(game, field, None)
            else:
                continue
            
            if fresh_value and (refresh_request.force or not current_value):
                if field == 'release_date':
                    setattr(game, field, parse_date_string(fresh_value))
                elif field == 'rating_average':
                    setattr(game, field, fresh_value)
                else:
                    setattr(game, field, fresh_value)
                updated_fields.append(field)
        
        game.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(game)
        
        return MetadataRefreshResponse(
            success=True,
            updated_fields=updated_fields,
            errors=errors,
            game=GameResponse.model_validate(game)
        )
        
    except Exception as e:
        return MetadataRefreshResponse(
            success=False,
            updated_fields=[],
            errors=[f"Failed to refresh metadata: {str(e)}"],
            game=GameResponse.model_validate(game)
        )


@router.post("/{game_id}/metadata/populate", response_model=MetadataPopulateResponse)
async def populate_game_metadata(
    game_id: str,
    populate_request: MetadataPopulateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """Populate missing metadata fields for a game."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    if not game.igdb_id:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Game does not have IGDB ID"
        )
    
    # Check permissions - only admin or unverified games can be populated
    if not current_user.is_admin and game.is_verified:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Cannot populate metadata for verified games. Only administrators can modify verified games."
        )
    
    try:
        # Create current metadata object
        from ..services.igdb import GameMetadata
        
        current_metadata = GameMetadata(
            igdb_id=game.igdb_id,
            igdb_slug=game.igdb_slug,
            title=game.title,
            description=game.description,
            genre=game.genre,
            developer=game.developer,
            publisher=game.publisher,
            release_date=game.release_date.isoformat() if game.release_date else None,
            cover_art_url=game.cover_art_url,
            rating_average=float(game.rating_average) if game.rating_average else None,
            rating_count=game.rating_count,
            estimated_playtime_hours=game.estimated_playtime_hours,
            hastily=game.howlongtobeat_main,
            normally=game.howlongtobeat_extra,
            completely=game.howlongtobeat_completionist
        )
        
        updated_metadata = await igdb_service.populate_missing_metadata(current_metadata, game.igdb_id)
        if not updated_metadata:
            return MetadataPopulateResponse(
                success=False,
                populated_fields=[],
                errors=["Failed to populate metadata from IGDB"],
                game=GameResponse.model_validate(game)
            )
        
        # Update game with populated metadata
        populated_fields = []
        errors = []
        
        fields_to_populate = populate_request.fields or [
            'title', 'description', 'genre', 'developer', 'publisher',
            'release_date', 'cover_art_url', 'rating_average', 'rating_count',
            'igdb_slug', 'howlongtobeat_main', 'howlongtobeat_extra', 'howlongtobeat_completionist'
        ]
        
        for field in fields_to_populate:
            # Handle time-to-beat field mapping
            if field == 'howlongtobeat_main':
                updated_value = getattr(updated_metadata, 'hastily', None)
                current_value = getattr(game, field, None)
            elif field == 'howlongtobeat_extra':
                updated_value = getattr(updated_metadata, 'normally', None)
                current_value = getattr(game, field, None)
            elif field == 'howlongtobeat_completionist':
                updated_value = getattr(updated_metadata, 'completely', None)
                current_value = getattr(game, field, None)
            elif hasattr(updated_metadata, field):
                updated_value = getattr(updated_metadata, field)
                current_value = getattr(game, field, None)
            else:
                continue
            
            # Only populate if missing (or if not populate_missing_only)
            if updated_value and (not populate_request.populate_missing_only or not current_value):
                if field == 'release_date':
                    setattr(game, field, parse_date_string(updated_value))
                elif field == 'rating_average':
                    setattr(game, field, updated_value)
                else:
                    setattr(game, field, updated_value)
                populated_fields.append(field)
        
        game.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(game)
        
        return MetadataPopulateResponse(
            success=True,
            populated_fields=populated_fields,
            errors=errors,
            game=GameResponse.model_validate(game)
        )
        
    except Exception as e:
        return MetadataPopulateResponse(
            success=False,
            populated_fields=[],
            errors=[f"Failed to populate metadata: {str(e)}"],
            game=GameResponse.model_validate(game)
        )


@router.post("/metadata/bulk", response_model=BulkMetadataResponse)
async def bulk_metadata_operation(
    bulk_request: BulkMetadataRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """Perform bulk metadata operations on multiple games."""
    
    # Only admin can perform bulk operations
    if not current_user.is_admin:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only administrators can perform bulk metadata operations"
        )
    
    results = []
    errors = []
    successful_operations = 0
    
    for game_id in bulk_request.game_ids:
        try:
            game = session.get(Game, game_id)
            if not game:
                errors.append(f"Game {game_id} not found")
                continue
            
            if not game.igdb_id:
                errors.append(f"Game {game_id} does not have IGDB ID")
                continue
            
            result = {"game_id": game_id, "success": False, "fields": []}
            
            if bulk_request.operation == "refresh":
                fresh_metadata = await igdb_service.refresh_game_metadata(game.igdb_id)
                if fresh_metadata:
                    # Update all fields
                    updated_fields = []
                    for field in ['title', 'description', 'genre', 'developer', 'publisher', 'cover_art_url', 'igdb_slug']:
                        fresh_value = getattr(fresh_metadata, field)
                        if fresh_value:
                            setattr(game, field, fresh_value)
                            updated_fields.append(field)
                    
                    if fresh_metadata.release_date:
                        game.release_date = parse_date_string(fresh_metadata.release_date)
                        updated_fields.append('release_date')
                    
                    if fresh_metadata.rating_average:
                        game.rating_average = fresh_metadata.rating_average
                        updated_fields.append('rating_average')
                    
                    game.rating_count = fresh_metadata.rating_count or 0
                    updated_fields.append('rating_count')
                    
                    # Update time-to-beat fields
                    if fresh_metadata.hastily:
                        game.howlongtobeat_main = fresh_metadata.hastily
                        updated_fields.append('howlongtobeat_main')
                    if fresh_metadata.normally:
                        game.howlongtobeat_extra = fresh_metadata.normally
                        updated_fields.append('howlongtobeat_extra')
                    if fresh_metadata.completely:
                        game.howlongtobeat_completionist = fresh_metadata.completely
                        updated_fields.append('howlongtobeat_completionist')
                    
                    result["success"] = True
                    result["fields"] = updated_fields
                    successful_operations += 1
                    
            elif bulk_request.operation == "populate":
                # Create current metadata object
                from ..services.igdb import GameMetadata
                
                current_metadata = GameMetadata(
                    igdb_id=game.igdb_id,
                    igdb_slug=game.igdb_slug,
                    title=game.title,
                    description=game.description,
                    genre=game.genre,
                    developer=game.developer,
                    publisher=game.publisher,
                    release_date=game.release_date.isoformat() if game.release_date else None,
                    cover_art_url=game.cover_art_url,
                    rating_average=float(game.rating_average) if game.rating_average else None,
                    rating_count=game.rating_count,
                    estimated_playtime_hours=game.estimated_playtime_hours,
                    hastily=game.howlongtobeat_main,
                    normally=game.howlongtobeat_extra,
                    completely=game.howlongtobeat_completionist
                )
                
                updated_metadata = await igdb_service.populate_missing_metadata(current_metadata, game.igdb_id)
                if updated_metadata:
                    populated_fields = []
                    for field in ['title', 'description', 'genre', 'developer', 'publisher', 'cover_art_url', 'igdb_slug']:
                        current_value = getattr(game, field, None)
                        updated_value = getattr(updated_metadata, field)
                        if updated_value and not current_value:
                            setattr(game, field, updated_value)
                            populated_fields.append(field)
                    
                    if updated_metadata.release_date and not game.release_date:
                        game.release_date = parse_date_string(updated_metadata.release_date)
                        populated_fields.append('release_date')
                    
                    if updated_metadata.rating_average and not game.rating_average:
                        game.rating_average = updated_metadata.rating_average
                        populated_fields.append('rating_average')
                    
                    # Populate time-to-beat fields if missing
                    if updated_metadata.hastily and not game.howlongtobeat_main:
                        game.howlongtobeat_main = updated_metadata.hastily
                        populated_fields.append('howlongtobeat_main')
                    if updated_metadata.normally and not game.howlongtobeat_extra:
                        game.howlongtobeat_extra = updated_metadata.normally
                        populated_fields.append('howlongtobeat_extra')
                    if updated_metadata.completely and not game.howlongtobeat_completionist:
                        game.howlongtobeat_completionist = updated_metadata.completely
                        populated_fields.append('howlongtobeat_completionist')
                    
                    result["success"] = True
                    result["fields"] = populated_fields
                    successful_operations += 1
            
            game.updated_at = datetime.now(timezone.utc)
            results.append(result)
            
        except Exception as e:
            errors.append(f"Failed to process game {game_id}: {str(e)}")
    
    session.commit()
    
    return BulkMetadataResponse(
        total_games=len(bulk_request.game_ids),
        processed_games=len(results),
        successful_operations=successful_operations,
        failed_operations=len(bulk_request.game_ids) - successful_operations,
        results=results,
        errors=errors
    )


@router.post("/{game_id}/cover-art/download", response_model=SuccessResponse)
async def download_game_cover_art(
    game_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """Download and store cover art locally for a game."""
    
    game = session.get(Game, game_id)
    if not game:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Game not found"
        )
    
    if not game.igdb_id:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Game does not have IGDB ID"
        )
        
    if not game.cover_art_url:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Game does not have cover art URL"
        )
    
    # Check permissions - only admin or unverified games can have cover art downloaded
    if not current_user.is_admin and game.is_verified:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Cannot download cover art for verified games. Only administrators can modify verified games."
        )
    
    try:
        local_url = await igdb_service.download_and_store_cover_art(game.igdb_id, game.cover_art_url)
        
        if local_url:
            # Update game with local cover art URL
            game.cover_art_url = local_url
            game.updated_at = datetime.now(timezone.utc)
            session.commit()
            
            return SuccessResponse(
                message=f"Cover art downloaded and stored successfully. Local URL: {local_url}"
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to download and store cover art"
            )
            
    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Error downloading cover art: {str(e)}"
        )


@router.post("/cover-art/bulk-download", response_model=BulkMetadataResponse)
async def bulk_download_cover_art(
    request: BulkCoverArtDownloadRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
):
    """Download and store cover art for multiple games."""
    
    # Only admin can perform bulk operations
    if not current_user.is_admin:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Only administrators can perform bulk cover art downloads"
        )
    
    results = []
    errors = []
    successful_operations = 0
    
    for game_id in request.game_ids:
        try:
            game = session.get(Game, game_id)
            if not game:
                errors.append(f"Game {game_id} not found")
                continue
            
            if not game.igdb_id:
                errors.append(f"Game {game_id} does not have IGDB ID")
                continue
            
            if not game.cover_art_url:
                errors.append(f"Game {game_id} does not have cover art URL")
                continue
            
            # Skip if already has local cover art and skip_existing is True
            if request.skip_existing and game.cover_art_url and game.cover_art_url.startswith("/static/"):
                results.append({
                    "game_id": game_id,
                    "success": True,
                    "fields": ["cover_art_url"],
                    "message": "Already has local cover art"
                })
                successful_operations += 1
                continue
            
            result = {"game_id": game_id, "success": False, "fields": []}
            
            # Download cover art
            local_url = await igdb_service.download_and_store_cover_art(game.igdb_id, game.cover_art_url)
            
            if local_url:
                # Update game with local cover art URL
                game.cover_art_url = local_url
                game.updated_at = datetime.now(timezone.utc)
                
                result["success"] = True
                result["fields"] = ["cover_art_url"]
                result["message"] = f"Cover art downloaded successfully"
                successful_operations += 1
            else:
                result["message"] = "Failed to download cover art"
                
            results.append(result)
            
        except Exception as e:
            errors.append(f"Failed to download cover art for game {game_id}: {str(e)}")
    
    session.commit()
    
    return BulkMetadataResponse(
        total_games=len(request.game_ids),
        processed_games=len(results),
        successful_operations=successful_operations,
        failed_operations=len(request.game_ids) - successful_operations,
        results=results,
        errors=errors
    )
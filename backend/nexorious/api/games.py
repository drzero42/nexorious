"""
Game management endpoints for CRUD operations and metadata handling.
"""

from fastapi import APIRouter, Depends, HTTPException, status, Query
from sqlmodel import Session, select, or_, and_, func
from datetime import datetime, timezone, date
import json
import re
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
    GameAliasResponse,
    IGDBSearchRequest,
    IGDBGameCandidate,
    IGDBSearchResponse,
    GameMetadataAcceptRequest
)
from ..api.schemas.common import SuccessResponse, PaginationParams

router = APIRouter(prefix="/games", tags=["Games"])


def create_slug(title: str) -> str:
    """Create a URL-friendly slug from game title."""
    slug = re.sub(r'[^\w\s-]', '', title.lower())
    slug = re.sub(r'[-\s]+', '-', slug)
    return slug.strip('-')


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


def ensure_unique_slug(session: Session, base_slug: str, game_id: Optional[str] = None) -> str:
    """Ensure slug is unique by appending counter if needed."""
    slug = base_slug
    counter = 1
    
    while True:
        query = select(Game).where(Game.slug == slug)
        if game_id:
            query = query.where(Game.id != game_id)
        
        existing_game = session.exec(query).first()
        if not existing_game:
            return slug
        
        slug = f"{base_slug}-{counter}"
        counter += 1


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
    
    return game


@router.post("/", response_model=GameResponse, status_code=status.HTTP_201_CREATED)
async def create_game(
    game_data: GameCreateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)]
):
    """Create a new game."""
    
    # Create slug from title
    base_slug = create_slug(game_data.title)
    unique_slug = ensure_unique_slug(session, base_slug)
    
    # Create game
    new_game = Game(
        title=game_data.title,
        slug=unique_slug,
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
    
    # Update slug if title changed
    if "title" in update_data:
        base_slug = create_slug(game_data.title)
        unique_slug = ensure_unique_slug(session, base_slug, game_id)
        game.slug = unique_slug
    
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
    alias_title: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    source: Optional[str] = None
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
                GameAlias.alias_title == alias_title
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
        alias_title=alias_title,
        source=source
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
            query=search_data.title,
            limit=search_data.limit or 10,
            fuzzy_threshold=0.6
        )
        
        # Convert GameMetadata objects to IGDBGameCandidate objects
        candidates = []
        for metadata in game_metadata_list:
            # Extract platform info from metadata if available
            platforms = []
            if metadata.description:
                # Basic platform extraction - these are actual platforms, not storefronts
                platform_keywords = ["PC", "PlayStation", "Xbox", "Nintendo", "Switch", "Steam Deck", "Mac", "Linux"]
                for keyword in platform_keywords:
                    if keyword.lower() in metadata.description.lower():
                        platforms.append(keyword)
            
            if not platforms:
                platforms = ["PC"]  # Default platform
            
            candidate = IGDBGameCandidate(
                igdb_id=metadata.igdb_id,
                title=metadata.title,
                release_date=metadata.release_date,
                cover_art_url=metadata.cover_art_url,
                description=metadata.description,
                platforms=platforms
            )
            candidates.append(candidate)
        
        return IGDBSearchResponse(
            candidates=candidates,
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
    igdb_service: IGDBService = Depends(get_igdb_service_dependency)
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
            "slug": game_metadata.slug,
            "description": game_metadata.description,
            "genre": game_metadata.genre,
            "developer": game_metadata.developer,
            "publisher": game_metadata.publisher,
            "release_date": parse_date_string(game_metadata.release_date),
            "cover_art_url": game_metadata.cover_art_url,
            "rating_average": game_metadata.rating_average,
            "rating_count": game_metadata.rating_count or 0,
            "estimated_playtime_hours": game_metadata.estimated_playtime_hours,
            "igdb_id": game_metadata.igdb_id,
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
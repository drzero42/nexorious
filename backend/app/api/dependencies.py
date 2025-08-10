"""
API dependencies for dependency injection.
"""

from typing import Generator
from functools import lru_cache

from fastapi import HTTPException, status, Depends
from app.services.igdb import IGDBService
from app.core.security import get_current_user
from app.models.user import User


@lru_cache()
def get_igdb_service() -> IGDBService:
    """Get IGDB service instance."""
    return IGDBService()


def get_igdb_service_dependency() -> Generator[IGDBService, None, None]:
    """FastAPI dependency for IGDB service."""
    service = get_igdb_service()
    try:
        yield service
    finally:
        # Clean up if needed
        pass


def verify_steam_games_enabled(
    current_user: User = Depends(get_current_user)
) -> User:
    """
    FastAPI dependency to verify Steam Games feature is enabled for the user.
    
    Returns the current user if Steam Games is enabled (default: True).
    Raises HTTP 404 if the feature is disabled for security reasons.
    """
    try:
        preferences = current_user.preferences or {}
        ui_preferences = preferences.get("ui", {})
        steam_games_visible = ui_preferences.get("steam_games_visible", True)  # Default: enabled
        
        if not steam_games_visible:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam Games feature is disabled"
            )
        
        return current_user
        
    except HTTPException:
        raise
    except Exception:
        # If preferences parsing fails, default to enabled for backwards compatibility
        return current_user
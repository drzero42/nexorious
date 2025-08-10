"""
API dependencies for dependency injection.
"""

from typing import Generator
from functools import lru_cache

from fastapi import HTTPException, status, Depends
from sqlmodel import Session, select
from app.services.igdb import IGDBService
from app.core.security import get_current_user
from app.core.database import get_session
from app.models.user import User
from app.models.platform import Platform, Storefront


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
    current_user: User = Depends(get_current_user),
    session: Session = Depends(get_session)
) -> User:
    """
    FastAPI dependency to verify Steam Games feature is enabled for the user.
    
    Validates that:
    1. User has Steam Games UI feature enabled (default: True)
    2. PC-Windows platform exists and is active
    3. Steam storefront exists and is active
    
    Returns the current user if all validations pass.
    Raises HTTP 404 if any validation fails for security reasons.
    """
    try:
        # Check user UI preference
        preferences = current_user.preferences or {}
        ui_preferences = preferences.get("ui", {})
        steam_games_visible = ui_preferences.get("steam_games_visible", True)  # Default: enabled
        
        if not steam_games_visible:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam Games feature is disabled"
            )
        
        # Check PC-Windows platform exists and is active
        pc_windows_platform = session.exec(
            select(Platform).where(Platform.name == "pc-windows")
        ).first()
        
        if not pc_windows_platform:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam Games feature is disabled - PC-Windows platform not found"
            )
        
        if not pc_windows_platform.is_active:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam Games feature is disabled - PC-Windows platform is inactive"
            )
        
        # Check Steam storefront exists and is active
        steam_storefront = session.exec(
            select(Storefront).where(Storefront.name == "steam")
        ).first()
        
        if not steam_storefront:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam Games feature is disabled - Steam storefront not found"
            )
        
        if not steam_storefront.is_active:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail="Steam Games feature is disabled - Steam storefront is inactive"
            )
        
        return current_user
        
    except HTTPException:
        raise
    except Exception:
        # If preferences parsing or database access fails, default to enabled for backwards compatibility
        return current_user
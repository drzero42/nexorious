"""
Utility functions for determining game sync status across platforms and storefronts.

This module provides generic sync checking functions that work with the user_game_platforms
table to determine if games are synced for specific platform/storefront combinations.
"""

from sqlmodel import Session, select
from sqlalchemy import and_
from typing import Optional
import logging

from ..models.user_game import UserGame, UserGamePlatform
from ..models.platform import Platform, Storefront

logger = logging.getLogger(__name__)


def is_game_synced(
    session: Session,
    user_id: str,
    igdb_id: int,
    platform: str,
    storefront: str
) -> bool:
    """
    Generic function to check if a game is synced for a specific platform/storefront combination.

    This function checks if there's a UserGame with the given igdb_id and a corresponding
    UserGamePlatform entry for the specified platform and storefront.

    Args:
        session: Database session
        user_id: User's unique identifier
        igdb_id: Game's IGDB ID (primary key in games table)
        platform: Platform identifier (e.g., 'pc-windows', 'playstation-5')
        storefront: Storefront identifier (e.g., 'steam', 'epic', 'gog')

    Returns:
        True if user_game_platforms association exists, False otherwise
    """
    # Input validation
    if session is None:
        logger.error("Database session is None")
        return False

    if not user_id:
        logger.error("User ID is empty or None")
        return False

    if igdb_id is None:
        logger.error("IGDB ID is None")
        return False

    if not platform:
        logger.error("Platform is empty or None")
        return False

    if not storefront:
        logger.error("Storefront is empty or None")
        return False

    try:
        # Optimized query using EXISTS for better performance
        # We only need to check if the association exists, not return the data
        from sqlalchemy import exists

        query = select(
            exists().where(
                and_(
                    UserGame.user_id == user_id,  # type: ignore[arg-type]
                    UserGame.game_id == igdb_id,  # type: ignore[arg-type]
                    UserGamePlatform.user_game_id == UserGame.id,  # type: ignore[arg-type]
                    UserGamePlatform.platform == platform,  # type: ignore[arg-type]
                    UserGamePlatform.storefront == storefront  # type: ignore[arg-type]
                )
            )
        )

        is_synced = session.scalar(query)

        logger.debug(
            f"Sync check for user {user_id}, IGDB ID {igdb_id}, "
            f"platform {platform}, storefront {storefront}: {is_synced}"
        )

        return bool(is_synced)

    except Exception as e:
        logger.error(
            f"Database error checking sync status for user {user_id}, IGDB ID {igdb_id}, "
            f"platform {platform}, storefront {storefront}: {e}",
            exc_info=True
        )
        return False


def is_steam_game_synced(session: Session, user_id: str, igdb_id: int) -> bool:
    """
    Check if a Steam game is synced using the generic sync function.

    This is a convenience wrapper around is_game_synced() for Steam games.

    Args:
        session: Database session
        user_id: User's unique identifier
        igdb_id: Game's IGDB ID

    Returns:
        True if the Steam game is synced, False otherwise
    """
    # Use slugs directly - FK now references name columns
    return is_game_synced(session, user_id, igdb_id, "pc-windows", "steam")


def get_platform(platform_name: str, session: Session) -> Optional[str]:
    """
    Get platform slug by name.

    Args:
        platform_name: Platform name/slug (e.g., 'pc-windows')
        session: Database session

    Returns:
        Platform name/slug if found, None otherwise
    """
    if session is None:
        logger.error("Database session is None")
        return None

    if not platform_name:
        logger.error("Platform name is empty or None")
        return None

    try:
        platform = session.exec(
            select(Platform).where(Platform.name == platform_name)
        ).first()

        if platform:
            logger.debug(f"Found platform '{platform_name}'")
            return platform.name
        else:
            logger.warning(f"Platform '{platform_name}' not found")
            return None

    except Exception as e:
        logger.error(f"Database error getting platform for '{platform_name}': {e}", exc_info=True)
        return None


def get_storefront(storefront_name: str, session: Session) -> Optional[str]:
    """
    Get storefront slug by name.

    Args:
        storefront_name: Storefront name/slug (e.g., 'steam')
        session: Database session

    Returns:
        Storefront name/slug if found, None otherwise
    """
    if session is None:
        logger.error("Database session is None")
        return None

    if not storefront_name:
        logger.error("Storefront name is empty or None")
        return None

    try:
        storefront = session.exec(
            select(Storefront).where(Storefront.name == storefront_name)
        ).first()

        if storefront:
            logger.debug(f"Found storefront '{storefront_name}'")
            return storefront.name
        else:
            logger.warning(f"Storefront '{storefront_name}' not found")
            return None

    except Exception as e:
        logger.error(f"Database error getting storefront for '{storefront_name}': {e}", exc_info=True)
        return None
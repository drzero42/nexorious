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
    platform_id: str,
    storefront_id: str
) -> bool:
    """
    Generic function to check if a game is synced for a specific platform/storefront combination.

    This function checks if there's a UserGame with the given igdb_id and a corresponding
    UserGamePlatform entry for the specified platform and storefront.

    Args:
        session: Database session
        user_id: User's unique identifier
        igdb_id: Game's IGDB ID (primary key in games table)
        platform_id: Platform identifier (e.g., 'pc-windows', 'playstation-5')
        storefront_id: Storefront identifier (e.g., 'steam', 'epic', 'gog')

    Returns:
        True if user_game_platforms association exists, False otherwise
    """
    try:
        # Query for UserGame + UserGamePlatform association
        query = (
            select(UserGame)
            .join(UserGamePlatform, UserGamePlatform.user_game_id == UserGame.id)
            .where(
                and_(
                    UserGame.user_id == user_id,
                    UserGame.game_id == igdb_id,  # UserGame.game_id references games.id which is the IGDB ID
                    UserGamePlatform.platform_id == platform_id,
                    UserGamePlatform.storefront_id == storefront_id
                )
            )
        )

        result = session.exec(query).first()
        is_synced = result is not None

        logger.debug(
            f"Sync check for user {user_id}, IGDB ID {igdb_id}, "
            f"platform {platform_id}, storefront {storefront_id}: {is_synced}"
        )

        return is_synced

    except Exception as e:
        logger.error(
            f"Error checking sync status for user {user_id}, IGDB ID {igdb_id}, "
            f"platform {platform_id}, storefront {storefront_id}: {e}"
        )
        return False


def is_steam_game_synced(session: Session, user_id: str, igdb_id: int) -> bool:
    """
    Check if a Steam game is synced using the generic sync function.

    This is a convenience wrapper around is_game_synced() that uses the
    Steam platform and storefront IDs.

    Args:
        session: Database session
        user_id: User's unique identifier
        igdb_id: Game's IGDB ID

    Returns:
        True if the Steam game is synced, False otherwise
    """
    # These IDs are from the seeded platform/storefront data
    STEAM_PLATFORM_ID = "1cfb37e2-db56-41e6-8e42-011ab79548ff"  # pc-windows
    STEAM_STOREFRONT_ID = "39933eda-2868-4903-bff4-3764b3b457ea"  # steam

    return is_game_synced(session, user_id, igdb_id, STEAM_PLATFORM_ID, STEAM_STOREFRONT_ID)


def get_platform_id(platform_name: str, session: Session) -> Optional[str]:
    """
    Get platform ID by name.

    Args:
        platform_name: Platform name (e.g., 'pc-windows')
        session: Database session

    Returns:
        Platform ID if found, None otherwise
    """
    try:
        platform = session.exec(
            select(Platform).where(Platform.name == platform_name)
        ).first()
        return platform.id if platform else None
    except Exception as e:
        logger.error(f"Error getting platform ID for {platform_name}: {e}")
        return None


def get_storefront_id(storefront_name: str, session: Session) -> Optional[str]:
    """
    Get storefront ID by name.

    Args:
        storefront_name: Storefront name (e.g., 'steam')
        session: Database session

    Returns:
        Storefront ID if found, None otherwise
    """
    try:
        storefront = session.exec(
            select(Storefront).where(Storefront.name == storefront_name)
        ).first()
        return storefront.id if storefront else None
    except Exception as e:
        logger.error(f"Error getting storefront ID for {storefront_name}: {e}")
        return None
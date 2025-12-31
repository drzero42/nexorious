"""
Platform lookup module for ID-based game matching.

Handles lookups by platform-specific IDs (Steam AppID, Epic ID, etc.)
using the Nexorious database to find existing IGDB matches.
"""

import logging
from typing import Optional

from sqlmodel import Session, select

from app.models.game import Game
from app.models.user_game import UserGame, UserGamePlatform

from .models import MatchResult, MatchStatus, MatchSource

logger = logging.getLogger(__name__)


async def lookup_by_steam_appid(
    session: Session,
    steam_appid: int,
    user_id: Optional[str] = None,
) -> Optional[MatchResult]:
    """
    Look up a game by Steam AppID in the existing database.

    Checks UserGamePlatform records for this AppID where storefront='steam'
    and store_game_id matches the Steam AppID. First checks user-specific
    matches, then any user's matches.

    Args:
        session: Database session
        steam_appid: Steam AppID to look up
        user_id: Optional user ID to check user-specific matches first

    Returns:
        MatchResult if found, None if no existing match exists
    """
    logger.debug(f"Looking up Steam AppID {steam_appid} in database")

    # Strategy 1: Check user's UserGamePlatform records for this AppID
    if user_id:
        user_match = _lookup_user_platform_game(session, steam_appid, user_id)
        if user_match:
            return user_match

    # Strategy 2: Check any user's UserGamePlatform records for this AppID
    any_user_match = _lookup_any_user_platform_game(session, steam_appid)
    if any_user_match:
        return any_user_match

    # No existing match found
    logger.debug(f"No existing IGDB match found for Steam AppID {steam_appid}")
    return None


def _lookup_user_platform_game(
    session: Session, steam_appid: int, user_id: str
) -> Optional[MatchResult]:
    """Check if user has already matched this Steam game via UserGamePlatform."""
    # Convert Steam AppID to string for comparison with store_game_id
    steam_appid_str = str(steam_appid)

    # Query UserGamePlatform where storefront='steam' and store_game_id matches
    # Join with UserGame to get the game_id (IGDB ID)
    stmt = (
        select(UserGamePlatform, UserGame, Game)
        .join(UserGame, UserGamePlatform.user_game_id == UserGame.id)  # pyrefly: ignore[bad-argument-type]
        .join(Game, UserGame.game_id == Game.id)  # pyrefly: ignore[bad-argument-type]
        .where(
            UserGamePlatform.storefront == "steam",
            UserGamePlatform.store_game_id == steam_appid_str,
            UserGame.user_id == user_id,
        )
    )
    result = session.exec(stmt).first()

    if result:
        platform, user_game, game = result
        logger.info(
            f"Found user's existing match for Steam AppID {steam_appid}: "
            f"IGDB ID {game.id}"
        )
        return MatchResult(
            source_title=game.title,
            status=MatchStatus.MATCHED,
            match_source=MatchSource.PLATFORM_LOOKUP,
            igdb_id=game.id,
            igdb_title=game.title,
            confidence_score=1.0,  # Platform ID lookup is authoritative
            source_metadata={"steam_appid": steam_appid, "user_id": user_id},
        )
    return None


def _lookup_any_user_platform_game(
    session: Session, steam_appid: int
) -> Optional[MatchResult]:
    """Check if any user has matched this Steam game via UserGamePlatform."""
    # Convert Steam AppID to string for comparison with store_game_id
    steam_appid_str = str(steam_appid)

    # Query UserGamePlatform where storefront='steam' and store_game_id matches
    # Join with UserGame to get the game_id (IGDB ID)
    stmt = (
        select(UserGamePlatform, UserGame, Game)
        .join(UserGame, UserGamePlatform.user_game_id == UserGame.id)  # pyrefly: ignore[bad-argument-type]
        .join(Game, UserGame.game_id == Game.id)  # pyrefly: ignore[bad-argument-type]
        .where(
            UserGamePlatform.storefront == "steam",
            UserGamePlatform.store_game_id == steam_appid_str,
        )
    )
    result = session.exec(stmt).first()

    if result:
        platform, user_game, game = result
        logger.info(
            f"Found another user's match for Steam AppID {steam_appid}: "
            f"IGDB ID {game.id}"
        )
        return MatchResult(
            source_title=game.title,
            status=MatchStatus.MATCHED,
            match_source=MatchSource.PLATFORM_LOOKUP,
            igdb_id=game.id,
            igdb_title=game.title,
            confidence_score=1.0,
            source_metadata={"steam_appid": steam_appid},
        )
    return None


async def lookup_by_igdb_id(
    session: Session,
    igdb_id: int,
) -> Optional[MatchResult]:
    """
    Look up a game by IGDB ID in the existing database.

    Checks if we already have this game in the games table.

    Args:
        session: Database session
        igdb_id: IGDB ID to look up

    Returns:
        MatchResult if game exists in DB, None otherwise
    """
    logger.debug(f"Looking up IGDB ID {igdb_id} in database")

    game = session.get(Game, igdb_id)

    if game:
        logger.info(f"Found existing game for IGDB ID {igdb_id}: '{game.title}'")
        return MatchResult(
            source_title=game.title,
            status=MatchStatus.MATCHED,
            match_source=MatchSource.IGDB_ID_PROVIDED,
            igdb_id=game.id,
            igdb_title=game.title,
            confidence_score=1.0,
            source_metadata={"igdb_id": igdb_id},
        )

    logger.debug(f"IGDB ID {igdb_id} not found in local database")
    return None


def check_existing_user_game(
    session: Session,
    user_id: str,
    igdb_id: int,
) -> bool:
    """
    Check if user already has this game in their collection.

    Args:
        session: Database session
        user_id: User ID to check
        igdb_id: IGDB ID to check

    Returns:
        True if user already has this game, False otherwise
    """
    from app.models.user_game import UserGame

    stmt = select(UserGame).where(
        UserGame.user_id == user_id,
        UserGame.game_id == igdb_id,
    )
    existing = session.exec(stmt).first()

    return existing is not None

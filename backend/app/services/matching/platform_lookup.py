"""
Platform lookup module for ID-based game matching.

Handles lookups by platform-specific IDs (Steam AppID, Epic ID, etc.)
using the Nexorious database to find existing IGDB matches.
"""

import logging
from typing import Optional

from sqlmodel import Session, select

from app.models.steam_game import SteamGame
from app.models.game import Game

from .models import MatchResult, MatchStatus, MatchSource

logger = logging.getLogger(__name__)


async def lookup_by_steam_appid(
    session: Session,
    steam_appid: int,
    user_id: Optional[str] = None,
) -> Optional[MatchResult]:
    """
    Look up a game by Steam AppID in the existing database.

    First checks if the user has a matched SteamGame with this AppID.
    If not found but another user has matched it, uses that IGDB ID.
    Falls back to checking the games table for any game with this Steam ID
    in its metadata.

    Args:
        session: Database session
        steam_appid: Steam AppID to look up
        user_id: Optional user ID to check user-specific matches first

    Returns:
        MatchResult if found, None if no existing match exists
    """
    logger.debug(f"Looking up Steam AppID {steam_appid} in database")

    # Strategy 1: Check user's SteamGame records for this AppID
    if user_id:
        user_match = _lookup_user_steam_game(session, steam_appid, user_id)
        if user_match:
            return user_match

    # Strategy 2: Check any user's SteamGame records for this AppID
    any_user_match = _lookup_any_user_steam_game(session, steam_appid)
    if any_user_match:
        return any_user_match

    # No existing match found
    logger.debug(f"No existing IGDB match found for Steam AppID {steam_appid}")
    return None


def _lookup_user_steam_game(
    session: Session, steam_appid: int, user_id: str
) -> Optional[MatchResult]:
    """Check if user has already matched this Steam game."""
    stmt = select(SteamGame).where(
        SteamGame.steam_appid == steam_appid,
        SteamGame.user_id == user_id,
        SteamGame.igdb_id.isnot(None),  # type: ignore[union-attr]
    )
    steam_game = session.exec(stmt).first()

    if steam_game and steam_game.igdb_id:
        logger.info(
            f"Found user's existing match for Steam AppID {steam_appid}: "
            f"IGDB ID {steam_game.igdb_id}"
        )
        return MatchResult(
            source_title=steam_game.game_name,
            status=MatchStatus.MATCHED,
            match_source=MatchSource.PLATFORM_LOOKUP,
            igdb_id=steam_game.igdb_id,
            igdb_title=steam_game.igdb_title,
            confidence_score=1.0,  # Platform ID lookup is authoritative
            source_metadata={"steam_appid": steam_appid, "user_id": user_id},
        )
    return None


def _lookup_any_user_steam_game(
    session: Session, steam_appid: int
) -> Optional[MatchResult]:
    """Check if any user has matched this Steam game."""
    stmt = select(SteamGame).where(
        SteamGame.steam_appid == steam_appid,
        SteamGame.igdb_id.isnot(None),  # type: ignore[union-attr]
    )
    steam_game = session.exec(stmt).first()

    if steam_game and steam_game.igdb_id:
        logger.info(
            f"Found another user's match for Steam AppID {steam_appid}: "
            f"IGDB ID {steam_game.igdb_id}"
        )
        return MatchResult(
            source_title=steam_game.game_name,
            status=MatchStatus.MATCHED,
            match_source=MatchSource.PLATFORM_LOOKUP,
            igdb_id=steam_game.igdb_id,
            igdb_title=steam_game.igdb_title,
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

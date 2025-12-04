"""
Steam games import operations module.

Handles Steam library import functionality.
"""

import logging
from typing import Dict, Any, List
from datetime import datetime, timezone

from sqlmodel import Session, select, and_

from app.models.user import User
from app.models.steam_game import SteamGame
from app.services.steam import SteamService, SteamAuthenticationError, SteamAPIError
from app.services.igdb import IGDBService
from app.utils.sqlalchemy_typed import is_

from .models import ImportResult, SteamGamesServiceError


logger = logging.getLogger(__name__)


async def import_steam_library(
    session: Session,
    user_id: str,
    steam_config: Dict[str, Any],
    enable_auto_matching: bool,
    steam_service: SteamService,
    igdb_service: IGDBService,
    auto_match_func
) -> ImportResult:
    """
    Import user's Steam library with optional automatic IGDB matching.

    Args:
        session: Database session
        user_id: User ID to import library for
        steam_config: Steam configuration (web_api_key, steam_id)
        enable_auto_matching: Whether to attempt automatic IGDB matching
        steam_service: Steam service instance
        igdb_service: IGDB service instance
        auto_match_func: Function to call for auto-matching Steam games

    Returns:
        ImportResult with statistics and any errors

    Raises:
        SteamGamesServiceError: For service-level errors
        SteamAuthenticationError: For Steam API authentication errors
        SteamAPIError: For other Steam API errors
    """
    logger.info(f"Starting Steam library import for user {user_id} (auto_matching: {enable_auto_matching})")

    try:
        # Validate user exists
        user = session.get(User, user_id)
        if not user:
            raise SteamGamesServiceError(f"User {user_id} not found")

        # Get Steam library
        steam_games = await steam_service.get_owned_games(steam_config["steam_id"])
        logger.info(f"Retrieved {len(steam_games)} games from Steam for user {user_id}")

        imported_count = 0
        skipped_count = 0
        errors: List[str] = []

        # Import games with deduplication
        new_steam_games: List[SteamGame] = []
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
                    new_steam_games.append(new_steam_game)
                    imported_count += 1
                    logger.debug(f"Added new Steam game {steam_game.appid} ({steam_game.name}) for user {user_id}")

            except Exception as e:
                error_msg = f"Error processing Steam game {steam_game.appid} ({getattr(steam_game, 'name', 'Unknown')}): {str(e)}"
                logger.error(error_msg)
                errors.append(error_msg)
                continue

        # Commit imported games first
        session.commit()
        logger.info(f"Steam library import phase completed for user {user_id}: {imported_count} imported, {skipped_count} skipped")

        # Automatic IGDB matching for all unmatched games (both new and existing)
        auto_matched_count = 0
        if enable_auto_matching:
            logger.info("Starting automatic IGDB matching for unmatched Steam games")
            try:
                # Find all unmatched Steam games for this user (not just newly imported ones)
                unmatched_games_query = select(SteamGame).where(
                    and_(
                        SteamGame.user_id == user_id,
                        is_(SteamGame.igdb_id, None),  # No IGDB match yet
                        not SteamGame.ignored    # Not ignored by user
                    )
                )
                unmatched_games = session.exec(unmatched_games_query).all()

                if unmatched_games:
                    logger.info(f"Found {len(unmatched_games)} unmatched Steam games to process (includes both new and existing games)")

                    # Perform automatic matching on all unmatched games
                    match_results = await auto_match_func([sg.id for sg in unmatched_games])
                    auto_matched_count = match_results.successful_matches

                    # Add any matching errors to overall error list
                    errors.extend(match_results.errors)

                    logger.info(f"Automatic IGDB matching completed: {auto_matched_count} games matched out of {len(unmatched_games)} unmatched games")
                else:
                    logger.info("No unmatched Steam games found for auto-matching")

            except Exception as e:
                error_msg = f"Error during automatic IGDB matching: {str(e)}"
                logger.error(error_msg)
                errors.append(error_msg)

        logger.info(f"Steam library import completed for user {user_id}: {imported_count} imported, {skipped_count} skipped, {auto_matched_count} auto-matched")

        return ImportResult(
            imported_count=imported_count,
            skipped_count=skipped_count,
            auto_matched_count=auto_matched_count,
            total_games=len(steam_games),
            errors=errors
        )

    except (SteamAuthenticationError, SteamAPIError):
        # Re-raise Steam API errors without wrapping
        raise
    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"Unexpected error during Steam library import for user {user_id}: {str(e)}")
        session.rollback()
        raise SteamGamesServiceError(f"Failed to import Steam library: {str(e)}")

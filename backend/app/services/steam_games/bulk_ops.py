"""
Steam games bulk operations module.

Handles bulk operations like unignore_all, unmatch_all, and unsync_all.
"""

import logging
from typing import Tuple
from datetime import datetime, timezone

from sqlmodel import Session, select

from app.models.steam_game import SteamGame
from app.services.sync_utils import is_steam_game_synced
from app.utils.sqlalchemy_typed import is_not

from .models import (
    BulkUnignoreResults,
    BulkUnmatchResults,
    BulkUnsyncResults,
    SteamGamesServiceError
)
from .sync_ops import unsync_steam_game_from_collection_internal


logger = logging.getLogger(__name__)


def toggle_steam_game_ignored(
    session: Session,
    steam_game_id: str,
    user_id: str
) -> Tuple[SteamGame, str, bool]:
    """
    Toggle the ignored status of a Steam game.

    Args:
        session: Database session
        steam_game_id: Steam game ID to toggle
        user_id: User ID for authorization

    Returns:
        Tuple of (updated SteamGame, success message, new ignored status)

    Raises:
        SteamGamesServiceError: For service errors
    """
    from sqlmodel import and_

    logger.info(f"Toggling ignored status for Steam game {steam_game_id} for user {user_id}")

    try:
        # Find the Steam game and verify ownership
        steam_game_query = select(SteamGame).where(
            and_(
                SteamGame.id == steam_game_id,
                SteamGame.user_id == user_id
            )
        )
        steam_game = session.exec(steam_game_query).first()

        if not steam_game:
            raise SteamGamesServiceError("Steam game not found or access denied")

        # Toggle the ignored status
        old_ignored = steam_game.ignored
        steam_game.ignored = not old_ignored
        steam_game.updated_at = datetime.now(timezone.utc)

        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)

        # Create appropriate success message
        if steam_game.ignored:
            message = f"Steam game '{steam_game.game_name}' is now ignored and won't be imported"
        else:
            message = f"Steam game '{steam_game.game_name}' is no longer ignored and can be imported"

        logger.info(f"Steam game {steam_game_id} ignored status toggled: {old_ignored} -> {steam_game.ignored} by user {user_id}")

        return steam_game, message, steam_game.ignored

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"Error toggling ignored status for Steam game {steam_game_id} for user {user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to toggle Steam game ignored status: {str(e)}")


async def unignore_all_steam_games(session: Session, user_id: str) -> BulkUnignoreResults:
    """
    Unignore all ignored Steam games for a user.

    Args:
        session: Database session
        user_id: User ID for authorization

    Returns:
        BulkUnignoreResults with detailed unignore statistics

    Raises:
        SteamGamesServiceError: For service errors
    """
    try:
        # Find all ignored Steam games for the user
        ignored_games = session.exec(
            select(SteamGame)
            .where(SteamGame.user_id == user_id)
            .where(SteamGame.ignored)
        ).all()

        total_processed = len(ignored_games)
        logger.info(f"Found {total_processed} ignored Steam games to unignore for user {user_id}")

        if total_processed == 0:
            return BulkUnignoreResults(
                total_processed=0,
                successful_unignores=0,
                failed_unignores=0,
                errors=[]
            )

        successful_unignores = 0
        failed_unignores = 0
        errors = []

        # Process each ignored game
        for steam_game in ignored_games:
            try:
                steam_game.ignored = False
                steam_game.updated_at = datetime.now(timezone.utc)
                session.add(steam_game)
                successful_unignores += 1
                logger.debug(f"Unignored Steam game: {steam_game.game_name} ({steam_game.id})")

            except Exception as e:
                failed_unignores += 1
                error_msg = f"Failed to unignore '{steam_game.game_name}': {str(e)}"
                errors.append(error_msg)
                logger.error(f"Error unignoring Steam game {steam_game.id}: {str(e)}")

        # Commit all changes in a single transaction
        try:
            session.commit()
        except Exception as e:
            session.rollback()
            error_msg = f"Failed to save unignore changes: {str(e)}"
            errors.append(error_msg)
            logger.error(f"Error committing bulk unignore for user {user_id}: {str(e)}")
            # All games failed if commit failed
            failed_unignores = total_processed
            successful_unignores = 0

        logger.info(f"Bulk Steam game unignore completed for user {user_id}: {successful_unignores} successful, {failed_unignores} failed")

        return BulkUnignoreResults(
            total_processed=total_processed,
            successful_unignores=successful_unignores,
            failed_unignores=failed_unignores,
            errors=errors
        )

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"Error during bulk Steam game unignore for user {user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to unignore Steam games: {str(e)}")


async def unmatch_all_matched_games(session: Session, user_id: str) -> BulkUnmatchResults:
    """
    Unmatch all matched (but not synced) Steam games for a user.

    Finds games with igdb_id that are not synced to collection and clears their igdb_id.
    This only affects games that are matched but haven't been synced yet.

    Args:
        session: Database session
        user_id: User ID for authorization

    Returns:
        BulkUnmatchResults with detailed unmatch statistics

    Raises:
        SteamGamesServiceError: For service errors
    """
    try:
        # Find matched Steam games that are not synced
        candidate_games = session.exec(
            select(SteamGame)
            .where(SteamGame.user_id == user_id)
            .where(is_not(SteamGame.igdb_id, None))  # Has IGDB match
            .where(not SteamGame.ignored)     # Not ignored
        ).all()

        # Filter for games that are not synced by checking each one
        matched_games = []
        for steam_game in candidate_games:
            if steam_game.igdb_id is None:
                continue
            is_synced = is_steam_game_synced(session, user_id, steam_game.igdb_id)
            if not is_synced:
                matched_games.append(steam_game)

        total_processed = len(matched_games)
        logger.info(f"Found {total_processed} matched (non-synced) Steam games to unmatch for user {user_id}")

        if total_processed == 0:
            return BulkUnmatchResults(
                total_processed=0,
                successful_unmatches=0,
                failed_unmatches=0,
                unsynced_games=0,  # Always 0 since we don't target synced games
                errors=[]
            )

        successful_unmatches = 0
        failed_unmatches = 0
        errors = []

        # Process each matched (non-synced) game
        for steam_game in matched_games:
            try:
                logger.debug(f"Processing '{steam_game.game_name}' (igdb_id: {steam_game.igdb_id})")

                # Clear IGDB match only (game is not synced to collection)
                steam_game.igdb_id = None
                steam_game.updated_at = datetime.now(timezone.utc)

                # Add to session (will be committed later)
                session.add(steam_game)

                successful_unmatches += 1
                logger.debug(f"Successfully unmatched Steam game: {steam_game.game_name} ({steam_game.id})")

            except Exception as e:
                failed_unmatches += 1
                error_msg = f"Failed to unmatch '{steam_game.game_name}': {str(e)}"
                errors.append(error_msg)
                logger.error(f"Error unmatching Steam game {steam_game.id}: {str(e)}")

        # Commit all changes in a single transaction
        try:
            session.commit()
            logger.info(f"Successfully committed bulk unmatch transaction for user {user_id}")
        except Exception as e:
            session.rollback()
            error_msg = f"Failed to save unmatch changes: {str(e)}"
            errors.append(error_msg)
            logger.error(f"Error committing bulk unmatch for user {user_id}: {str(e)}")
            # All games failed if commit failed
            failed_unmatches = total_processed
            successful_unmatches = 0

        logger.info(f"Bulk Steam game unmatch completed for user {user_id}: {successful_unmatches} successful, {failed_unmatches} failed")

        return BulkUnmatchResults(
            total_processed=total_processed,
            successful_unmatches=successful_unmatches,
            failed_unmatches=failed_unmatches,
            unsynced_games=0,  # Always 0 since we don't target synced games
            errors=errors
        )

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"Error during bulk Steam game unmatch for user {user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to unmatch Steam games: {str(e)}")


async def unsync_all_synced_games(session: Session, user_id: str) -> BulkUnsyncResults:
    """
    Unsync all synced Steam games for a user.

    Finds games synced to collection and:
    1. Removes Steam platform/storefront from UserGame
    2. If no other platforms, removes the entire UserGame
    3. Keeps igdb_id in SteamGame (returns to matched state)

    Args:
        session: Database session
        user_id: User ID for authorization

    Returns:
        BulkUnsyncResults with detailed unsync statistics

    Raises:
        SteamGamesServiceError: For service errors
    """
    try:
        # Find Steam games that are synced to collection
        candidate_games = session.exec(
            select(SteamGame)
            .where(SteamGame.user_id == user_id)
            .where(is_not(SteamGame.igdb_id, None))  # Has IGDB match
            .where(not SteamGame.ignored)     # Not ignored
        ).all()

        # Filter for games that are actually synced by checking each one
        synced_games = []
        for steam_game in candidate_games:
            if steam_game.igdb_id is None:
                continue
            is_synced = is_steam_game_synced(session, user_id, steam_game.igdb_id)
            if is_synced:
                synced_games.append(steam_game)

        total_processed = len(synced_games)
        logger.info(f"Found {total_processed} synced Steam games to unsync for user {user_id}")

        if total_processed == 0:
            return BulkUnsyncResults(
                total_processed=0,
                successful_unsyncs=0,
                failed_unsyncs=0,
                errors=[]
            )

        successful_unsyncs = 0
        failed_unsyncs = 0
        errors = []

        # Process each synced game
        for steam_game in synced_games:
            try:
                # Check if actually synced using new function
                igdb_id = steam_game.igdb_id
                if not igdb_id:
                    logger.warning(f"Steam game '{steam_game.game_name}' has no IGDB ID, skipping")
                    continue

                is_synced = is_steam_game_synced(session, user_id, igdb_id)
                if not is_synced:
                    logger.debug(f"Steam game '{steam_game.game_name}' is not actually synced, skipping")
                    continue

                logger.debug(f"Processing '{steam_game.game_name}' (IGDB ID: {igdb_id})")

                # First unsync from collection (removes platform association)
                try:
                    unsync_result = await unsync_steam_game_from_collection_internal(session, igdb_id, user_id)
                    logger.debug(f"Unsync operation result: {unsync_result}")
                except Exception as unsync_e:
                    error_msg = f"Failed to unsync '{steam_game.game_name}' from collection: {str(unsync_e)}"
                    errors.append(error_msg)
                    logger.error(f"Unsync error for {steam_game.id}: {str(unsync_e)}")
                    failed_unsyncs += 1
                    continue  # Skip to next game

                # Update Steam game timestamp (no more game_id to clear)
                steam_game.updated_at = datetime.now(timezone.utc)

                # Add to session (will be committed later)
                session.add(steam_game)

                successful_unsyncs += 1
                logger.debug(f"Successfully unsynced Steam game: {steam_game.game_name} ({steam_game.id})")

            except Exception as e:
                failed_unsyncs += 1
                error_msg = f"Failed to unsync '{steam_game.game_name}': {str(e)}"
                errors.append(error_msg)
                logger.error(f"Error unsyncing Steam game {steam_game.id}: {str(e)}")

        # Commit all changes in a single transaction
        try:
            session.commit()
            logger.info(f"Successfully committed bulk unsync transaction for user {user_id}")
        except Exception as e:
            session.rollback()
            error_msg = f"Failed to save unsync changes: {str(e)}"
            errors.append(error_msg)
            logger.error(f"Error committing bulk unsync for user {user_id}: {str(e)}")
            # All games failed if commit failed
            failed_unsyncs = total_processed
            successful_unsyncs = 0

        logger.info(f"Bulk Steam game unsync completed for user {user_id}: {successful_unsyncs} successful, {failed_unsyncs} failed")

        return BulkUnsyncResults(
            total_processed=total_processed,
            successful_unsyncs=successful_unsyncs,
            failed_unsyncs=failed_unsyncs,
            errors=errors
        )

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"Error during bulk Steam game unsync for user {user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to unsync Steam games: {str(e)}")

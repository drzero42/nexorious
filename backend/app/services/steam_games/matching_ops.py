"""
Steam games matching operations module.

Handles automatic IGDB matching functionality for Steam games.
"""

import logging
import asyncio
from typing import List, Tuple, Optional
from datetime import datetime, timezone

from sqlmodel import Session, select, and_

from app.models.user import User
from app.models.steam_game import SteamGame
from app.services.igdb import IGDBService, IGDBError
from app.services.sync_utils import is_steam_game_synced
from app.utils.fuzzy_match import calculate_fuzzy_confidence
from app.utils.sqlalchemy_typed import is_

from .models import AutoMatchResult, AutoMatchResults, SteamGamesServiceError


logger = logging.getLogger(__name__)


async def auto_match_steam_games(
    session: Session,
    steam_game_ids: List[str],
    igdb_service: IGDBService,
    auto_match_confidence_threshold: float,
    auto_match_batch_size: int
) -> AutoMatchResults:
    """
    Automatically match Steam games to IGDB games using fuzzy matching.

    Args:
        session: Database session
        steam_game_ids: List of Steam game IDs to attempt matching
        igdb_service: IGDB service instance
        auto_match_confidence_threshold: Minimum confidence for auto-matching
        auto_match_batch_size: Batch size for processing

    Returns:
        AutoMatchResults with detailed matching results
    """
    logger.info(f"🎯 [Auto-Match] Starting automatic IGDB matching for {len(steam_game_ids)} Steam games (confidence threshold: {auto_match_confidence_threshold:.0%})")

    results = []
    successful_matches = 0
    failed_matches = 0
    skipped_games = 0
    errors = []

    # Process in batches to respect rate limits
    total_batches = (len(steam_game_ids) + auto_match_batch_size - 1) // auto_match_batch_size
    for i in range(0, len(steam_game_ids), auto_match_batch_size):
        batch = steam_game_ids[i:i + auto_match_batch_size]
        current_batch = i // auto_match_batch_size + 1
        logger.info(f"🎯 [Auto-Match] Processing batch {current_batch}/{total_batches}: {len(batch)} games")

        for j, steam_game_id in enumerate(batch):
            try:
                logger.debug(f"🎯 [Auto-Match] Processing game {i+j+1}/{len(steam_game_ids)} (ID: {steam_game_id})")
                result = await auto_match_single_steam_game(
                    session, steam_game_id, igdb_service, auto_match_confidence_threshold
                )
                results.append(result)

                if result.matched:
                    successful_matches += 1
                    logger.info(f"✅ [Auto-Match] MATCHED: '{result.steam_game_name}' -> '{result.igdb_game_title}' (confidence: {result.confidence_score:.1%}, IGDB ID: {result.igdb_id})")
                elif result.error_message:
                    failed_matches += 1
                    logger.warning(f"❌ [Auto-Match] FAILED: '{result.steam_game_name}' - {result.error_message}")
                    errors.append(result.error_message)
                else:
                    skipped_games += 1  # Low confidence, skip for manual matching
                    confidence_info = f" (confidence: {result.confidence_score:.1%})" if result.confidence_score else ""
                    logger.info(f"⏭️ [Auto-Match] SKIPPED: '{result.steam_game_name}' - confidence too low{confidence_info}")

            except Exception as e:
                error_msg = f"Error auto-matching Steam game {steam_game_id}: {str(e)}"
                logger.error(f"💥 [Auto-Match] EXCEPTION: {error_msg}")
                errors.append(error_msg)
                failed_matches += 1
                continue

        # Small delay between batches to be respectful of IGDB API
        if i + auto_match_batch_size < len(steam_game_ids):
            logger.debug("🎯 [Auto-Match] Waiting 0.5s between batches...")
            await asyncio.sleep(0.5)

    logger.info(f"🎯 [Auto-Match] COMPLETED: {successful_matches} matched ✅, {failed_matches} failed ❌, {skipped_games} skipped ⏭️ (out of {len(steam_game_ids)} games)")

    return AutoMatchResults(
        total_processed=len(steam_game_ids),
        successful_matches=successful_matches,
        failed_matches=failed_matches,
        skipped_games=skipped_games,
        results=results,
        errors=errors
    )


async def auto_match_single_steam_game(
    session: Session,
    steam_game_id: str,
    igdb_service: IGDBService,
    auto_match_confidence_threshold: float
) -> AutoMatchResult:
    """
    Attempt to automatically match a single Steam game to IGDB.

    Args:
        session: Database session
        steam_game_id: Steam game ID to match
        igdb_service: IGDB service instance
        auto_match_confidence_threshold: Minimum confidence for auto-matching

    Returns:
        AutoMatchResult with matching details
    """
    # Get Steam game
    logger.debug(f"🎯 [Single Match] Looking up Steam game with ID: {steam_game_id}")
    steam_game = session.get(SteamGame, steam_game_id)
    if not steam_game:
        logger.error(f"🎯 [Single Match] Steam game not found in database: {steam_game_id}")
        return AutoMatchResult(
            steam_game_id=steam_game_id,
            steam_game_name="Unknown",
            steam_appid=0,
            matched=False,
            error_message="Steam game not found in database"
        )

    logger.debug(f"🎯 [Single Match] Processing '{steam_game.game_name}' (Steam AppID: {steam_game.steam_appid})")

    # Skip if already matched
    if steam_game.igdb_id:
        logger.debug(f"🎯 [Single Match] Skipping '{steam_game.game_name}' - already has IGDB match: {steam_game.igdb_id}")
        return AutoMatchResult(
            steam_game_id=steam_game_id,
            steam_game_name=steam_game.game_name,
            steam_appid=steam_game.steam_appid,
            matched=False,
            error_message="Steam game already has IGDB match"
        )

    try:
        # Search IGDB for potential matches
        # Use same fuzzy_threshold as manual search (0.6) instead of auto_match_confidence_threshold (0.8)
        # This ensures consistent behavior with manual search and gets more candidates
        logger.debug(f"🎯 [Single Match] Searching IGDB for '{steam_game.game_name}'...")
        igdb_candidates = await igdb_service.search_games(
            query=steam_game.game_name,
            limit=5,  # Get top 5 candidates
            fuzzy_threshold=0.6  # Use same threshold as manual search (/games/search/igdb)
        )

        logger.debug(f"🎯 [Single Match] IGDB search returned {len(igdb_candidates)} candidates for '{steam_game.game_name}'")

        if not igdb_candidates:
            logger.debug(f"🎯 [Single Match] No IGDB candidates found for '{steam_game.game_name}'")
            return AutoMatchResult(
                steam_game_id=steam_game_id,
                steam_game_name=steam_game.game_name,
                steam_appid=steam_game.steam_appid,
                matched=False
            )

        # Log all candidates for debugging
        for i, candidate in enumerate(igdb_candidates):
            logger.debug(f"🎯 [Single Match] Candidate {i+1}: '{candidate.title}' (IGDB ID: {candidate.igdb_id})")

        # Get the best match (first result, as they're ranked by fuzzy matching)
        best_match = igdb_candidates[0]
        logger.debug(f"🎯 [Single Match] Best match candidate: '{best_match.title}' (IGDB ID: {best_match.igdb_id})")

        # Calculate confidence score using sophisticated multi-metric fuzzy matching
        confidence = calculate_fuzzy_confidence(steam_game.game_name, best_match.title)
        logger.debug(f"🎯 [Single Match] Confidence score: {confidence:.1%} (threshold: {auto_match_confidence_threshold:.1%})")

        # Only auto-match if confidence is above threshold
        if confidence >= auto_match_confidence_threshold:
            logger.info(f"✅ [Single Match] Confidence meets threshold ({confidence:.1%} >= {auto_match_confidence_threshold:.1%}), proceeding with match...")

            # Log game state before update
            logger.info(f"📋 [Single Match] BEFORE UPDATE - Game: '{steam_game.game_name}' | igdb_id: {steam_game.igdb_id} | ignored: {steam_game.ignored}")

            # Update Steam game with IGDB match (no Game record creation)
            logger.info(f"💾 [Single Match] Setting IGDB ID for SteamGame {steam_game_id}: '{steam_game.game_name}' -> IGDB ID {best_match.igdb_id}")
            old_igdb_id = steam_game.igdb_id
            steam_game.igdb_id = int(best_match.igdb_id)
            steam_game.igdb_title = best_match.title
            steam_game.updated_at = datetime.now(timezone.utc)

            logger.info(f"📋 [Single Match] AFTER ASSIGNMENT - Game: '{steam_game.game_name}' | old_igdb_id: {old_igdb_id} | new_igdb_id: {steam_game.igdb_id}")

            try:
                logger.info("💾 [Single Match] Adding to session and committing...")
                session.add(steam_game)
                session.commit()
                logger.info(f"✅ [Single Match] DATABASE COMMIT SUCCESSFUL - '{steam_game.game_name}' matched to '{best_match.title}' (IGDB ID: {best_match.igdb_id})")

                # Verify the update in database
                session.refresh(steam_game)
                logger.info(f"🔍 [Single Match] VERIFICATION - Game after refresh: '{steam_game.game_name}' | igdb_id: {steam_game.igdb_id} | ignored: {steam_game.ignored}")

            except Exception as e:
                logger.error(f"❌ [Single Match] DATABASE COMMIT FAILED for '{steam_game.game_name}': {str(e)}")
                logger.error("💥 [Single Match] Rolling back transaction...")
                session.rollback()  # Rollback the failed transaction

                # Log final state after rollback
                session.refresh(steam_game)
                logger.error(f"🔄 [Single Match] AFTER ROLLBACK - Game: '{steam_game.game_name}' | igdb_id: {steam_game.igdb_id}")
                raise  # Re-raise to trigger the outer catch block

            return AutoMatchResult(
                steam_game_id=steam_game_id,
                steam_game_name=steam_game.game_name,
                steam_appid=steam_game.steam_appid,
                matched=True,
                igdb_id=int(best_match.igdb_id),
                igdb_game_title=best_match.title,
                confidence_score=confidence
            )
        else:
            # Low confidence, leave for manual matching
            logger.debug(f"🎯 [Single Match] Confidence {confidence:.1%} below threshold {auto_match_confidence_threshold:.1%}, skipping '{steam_game.game_name}'")
            return AutoMatchResult(
                steam_game_id=steam_game_id,
                steam_game_name=steam_game.game_name,
                steam_appid=steam_game.steam_appid,
                matched=False,
                confidence_score=confidence
            )

    except IGDBError as e:
        error_msg = f"IGDB error matching '{steam_game.game_name}': {str(e)}"
        logger.error(f"🎯 [Single Match] IGDB ERROR for '{steam_game.game_name}': {str(e)}")
        # Ensure session is in clean state after IGDB errors
        try:
            session.rollback()
        except Exception:
            pass  # Ignore rollback errors
        return AutoMatchResult(
            steam_game_id=steam_game_id,
            steam_game_name=steam_game.game_name,
            steam_appid=steam_game.steam_appid,
            matched=False,
            error_message=error_msg
        )
    except Exception as e:
        error_msg = f"Unexpected error matching '{steam_game.game_name}': {str(e)}"
        logger.error(f"🎯 [Single Match] UNEXPECTED ERROR for '{steam_game.game_name}': {str(e)}")
        logger.exception(f"🎯 [Single Match] Exception details for '{steam_game.game_name}':")
        # Ensure session is in clean state after unexpected errors
        try:
            session.rollback()
        except Exception:
            pass  # Ignore rollback errors
        return AutoMatchResult(
            steam_game_id=steam_game_id,
            steam_game_name=steam_game.game_name,
            steam_appid=steam_game.steam_appid,
            matched=False,
            error_message=error_msg
        )


async def retry_auto_matching_for_unmatched_games(
    session: Session,
    user_id: str,
    igdb_service: IGDBService,
    auto_match_confidence_threshold: float,
    auto_match_batch_size: int
) -> AutoMatchResults:
    """
    Manually retry auto-matching for all unmatched Steam games.

    Args:
        session: Database session
        user_id: User ID for authorization
        igdb_service: IGDB service instance
        auto_match_confidence_threshold: Minimum confidence for auto-matching
        auto_match_batch_size: Batch size for processing

    Returns:
        AutoMatchResults with detailed matching results

    Raises:
        SteamGamesServiceError: For service errors
    """
    logger.info(f"🎯 [Manual Auto-Match] Starting manual auto-matching retry for user {user_id}")

    try:
        # Validate user exists
        user = session.get(User, user_id)
        if not user:
            raise SteamGamesServiceError(f"User {user_id} not found")

        # Find all unmatched Steam games for this user
        logger.info(f"🔍 [Manual Auto-Match] Searching for unmatched games for user {user_id}")
        unmatched_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == user_id,
                is_(SteamGame.igdb_id, None),  # No IGDB match yet
                not SteamGame.ignored    # Not ignored by user
            )
        )
        unmatched_games = session.exec(unmatched_games_query).all()

        # Debug: Log current state of all games for this user
        all_games_query = select(SteamGame).where(SteamGame.user_id == user_id)
        all_games = session.exec(all_games_query).all()
        logger.info(f"📊 [Manual Auto-Match] All games state for user {user_id}:")
        for game in all_games:
            # Check sync status for each game
            is_synced = is_steam_game_synced(session, user_id, game.igdb_id) if game.igdb_id else False
            logger.info(f"  - {game.game_name} | igdb_id: {game.igdb_id} | synced: {is_synced} | ignored: {game.ignored}")

        logger.info(f"📊 [Manual Auto-Match] Game counts for user {user_id}:")
        logger.info(f"  - Total games: {len(all_games)}")
        logger.info(f"  - Unmatched (no igdb_id, not ignored): {len([g for g in all_games if not g.igdb_id and not g.ignored])}")
        # For matched and synced counts, we need to check each game individually
        matched_unsynced = 0
        synced_count = 0
        for game in all_games:
            if game.igdb_id and not game.ignored:
                is_synced = is_steam_game_synced(session, user_id, game.igdb_id)
                if is_synced:
                    synced_count += 1
                else:
                    matched_unsynced += 1
        logger.info(f"  - Matched (has igdb_id, not synced, not ignored): {matched_unsynced}")
        logger.info(f"  - Synced: {synced_count}")
        logger.info(f"  - Ignored: {len([g for g in all_games if g.ignored])}")

        if not unmatched_games:
            logger.info(f"🎯 [Manual Auto-Match] No unmatched Steam games found for user {user_id}")
            return AutoMatchResults(
                total_processed=0,
                successful_matches=0,
                failed_matches=0,
                skipped_games=0,
                results=[],
                errors=[]
            )

        logger.info(f"🎯 [Manual Auto-Match] Found {len(unmatched_games)} unmatched Steam games for user {user_id}")
        logger.info("🎮 [Manual Auto-Match] Unmatched games:")
        for game in unmatched_games:
            logger.info(f"  - {game.game_name} (Steam ID: {game.steam_appid})")

        # Perform automatic matching on all unmatched games
        logger.info("🚀 [Manual Auto-Match] Starting automatic matching process...")
        match_results = await auto_match_steam_games(
            session,
            [sg.id for sg in unmatched_games],
            igdb_service,
            auto_match_confidence_threshold,
            auto_match_batch_size
        )

        logger.info(f"🎯 [Manual Auto-Match] Manual auto-matching completed for user {user_id}: "
                   f"{match_results.successful_matches} matched, {match_results.failed_matches} failed, "
                   f"{match_results.skipped_games} skipped")

        return match_results

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"Error during manual auto-matching for user {user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to retry auto-matching: {str(e)}")


async def match_steam_game_to_igdb(
    session: Session,
    steam_game_id: str,
    igdb_id: Optional[int],
    user_id: str,
    igdb_service: IGDBService,
    unsync_func
) -> Tuple[SteamGame, str]:
    """
    Manually match or unmatch a Steam game to/from an IGDB game.

    Args:
        session: Database session
        steam_game_id: Steam game ID to match
        igdb_id: IGDB ID to match to, or None to clear match
        user_id: User ID for authorization
        igdb_service: IGDB service instance
        unsync_func: Function to unsync Steam game from collection

    Returns:
        Tuple of (updated SteamGame, success message)

    Raises:
        SteamGamesServiceError: For service errors
    """
    logger.info(f"🔄 [Transaction] Starting Steam game match operation: {steam_game_id} -> IGDB ID {igdb_id} for user {user_id}")

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

        # Validate IGDB ID and fetch title if provided (skip validation for clearing matches)
        igdb_title = None
        if igdb_id is not None:
            try:
                # Fetch IGDB game data to get the title and validate the ID
                game_data = await igdb_service.get_game_by_id(igdb_id)
                if not game_data:
                    raise SteamGamesServiceError(f"Invalid IGDB ID: {igdb_id}")
                igdb_title = game_data.title
                logger.debug(f"🔄 [Transaction] Retrieved IGDB title: '{igdb_title}' for IGDB ID {igdb_id}")
            except Exception as e:
                # If IGDB service fails, treat it as invalid IGDB ID
                logger.warning(f"IGDB validation failed for ID {igdb_id}: {str(e)}")
                raise SteamGamesServiceError(f"Invalid IGDB ID: {igdb_id}")

        # Store current state for unsync operations
        old_igdb_id = steam_game.igdb_id
        was_synced = is_steam_game_synced(session, user_id, old_igdb_id) if old_igdb_id else False

        # Update the Steam game's IGDB ID and title
        steam_game.igdb_id = igdb_id
        steam_game.igdb_title = igdb_title  # Set to None when clearing match

        steam_game.updated_at = datetime.now(timezone.utc)

        # Add to session but DO NOT commit yet - need to do unsync operations first
        session.add(steam_game)
        logger.debug("🔄 [Transaction] Updated SteamGame in session (not committed yet)")

        # Handle collection unsync for unmatch operations
        unsync_result = None
        if igdb_id is None and was_synced and old_igdb_id is not None:
            logger.info(f"🔄 [Transaction] Steam game was synced (IGDB ID: {old_igdb_id}), performing unsync operation")
            unsync_result = await unsync_func(old_igdb_id, user_id)
            logger.debug(f"🔄 [Transaction] Unsync operation result: {unsync_result}")

        # Create response message
        if igdb_id is None:
            if old_igdb_id and was_synced:
                # Was matched and synced - unmatched and unsynced
                if unsync_result == "complete":
                    message = f"Unmatched Steam game '{steam_game.game_name}' and removed from collection"
                elif unsync_result == "platform_only":
                    message = f"Unmatched Steam game '{steam_game.game_name}' and removed Steam platform (other platforms retained)"
                else:
                    message = f"Unmatched Steam game '{steam_game.game_name}' and removed from collection"
            elif old_igdb_id:
                # Was matched but not synced - just unmatched
                message = f"Cleared IGDB match for Steam game '{steam_game.game_name}'"
            else:
                # Was neither matched nor synced
                message = f"No IGDB match to clear for Steam game '{steam_game.game_name}'"
        else:
            if old_igdb_id:
                message = f"Updated IGDB match for Steam game '{steam_game.game_name}'"
            else:
                message = f"Successfully matched Steam game '{steam_game.game_name}' to IGDB"

        # Commit ALL operations atomically (SteamGame changes + platform deletions)
        logger.debug("🔄 [Transaction] Committing all changes to database")
        session.commit()
        session.refresh(steam_game)
        logger.info(f"✅ [Transaction] Successfully committed Steam game {steam_game_id} IGDB match update: {old_igdb_id} -> {igdb_id} by user {user_id}")

        return steam_game, message

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping but rollback first
        logger.error("🔄 [Transaction] SteamGamesServiceError occurred, rolling back transaction")
        session.rollback()
        raise
    except Exception as e:
        # Rollback on any unexpected errors
        logger.error(f"🔄 [Transaction] Unexpected error occurred, rolling back transaction: {str(e)}")
        session.rollback()
        logger.error(f"Error matching Steam game {steam_game_id} to IGDB for user {user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to match Steam game to IGDB: {str(e)}")

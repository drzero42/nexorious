"""
Steam games sync operations module.

Handles syncing Steam games to the user's main collection.
"""

import logging
from typing import Tuple
from datetime import datetime, timezone

from sqlmodel import Session, select, and_

from app.models.steam_game import SteamGame
from app.models.game import Game
from app.models.user_game import UserGame, UserGamePlatform, OwnershipStatus, PlayStatus
from app.models.platform import Platform, Storefront
from app.services.igdb import IGDBService
from app.services.sync_utils import is_steam_game_synced
from app.services.game_service import GameService
from app.utils.sqlalchemy_typed import is_not

from .models import SyncResult, BulkSyncResults, SteamGamesServiceError


logger = logging.getLogger(__name__)


async def remove_steam_platform_association(session: Session, user_game_id: str) -> bool:
    """
    Remove Steam platform association from a UserGame.

    Args:
        session: Database session
        user_game_id: UserGame ID to remove Steam association from

    Returns:
        bool: True if Steam association was found and removed, False otherwise

    Raises:
        SteamGamesServiceError: For service errors
    """
    logger.debug(f"🔍 [Platform Delete] Starting Steam platform association removal for UserGame {user_game_id}")

    try:
        # Get Steam platform and storefront IDs
        steam_platform_query = select(Platform).where(Platform.name == "pc-windows")
        steam_platform = session.exec(steam_platform_query).first()
        logger.debug(f"🔍 [Platform Delete] Found Steam platform: {steam_platform.id if steam_platform else 'None'} (name: pc-windows)")

        steam_storefront_query = select(Storefront).where(Storefront.name == "steam")
        steam_storefront = session.exec(steam_storefront_query).first()
        logger.debug(f"🔍 [Platform Delete] Found Steam storefront: {steam_storefront.id if steam_storefront else 'None'} (name: steam)")

        if not steam_platform or not steam_storefront:
            logger.error(f"🔍 [Platform Delete] Steam configuration missing - platform: {steam_platform is not None}, storefront: {steam_storefront is not None}")
            raise SteamGamesServiceError("Steam platform configuration not found")

        # Debug: Query ALL platform associations for this UserGame first
        all_associations_query = select(UserGamePlatform).where(UserGamePlatform.user_game_id == user_game_id)
        all_associations = session.exec(all_associations_query).all()
        logger.debug(f"🔍 [Platform Delete] UserGame {user_game_id} has {len(all_associations)} total platform associations:")
        for assoc in all_associations:
            platform_name = assoc.platform.name if assoc.platform else "Unknown"
            storefront_name = assoc.storefront.name if assoc.storefront else "None"
            logger.debug(f"  - Platform: {platform_name} (ID: {assoc.platform_id}), Storefront: {storefront_name} (ID: {assoc.storefront_id})")

        # Find and remove Steam platform association
        steam_association_query = select(UserGamePlatform).where(
            and_(
                UserGamePlatform.user_game_id == user_game_id,
                UserGamePlatform.platform_id == steam_platform.id,
                UserGamePlatform.storefront_id == steam_storefront.id
            )
        )
        steam_association = session.exec(steam_association_query).first()

        if steam_association:
            logger.info(f"✅ [Platform Delete] Found Steam platform association for UserGame {user_game_id}, deleting...")
            logger.debug(f"🔍 [Platform Delete] Deleting association: UserGamePlatform ID {steam_association.id}")
            session.delete(steam_association)
            logger.info("✅ [Platform Delete] Steam platform association deleted from session (will commit with transaction)")
            return True
        else:
            logger.warning(f"❌ [Platform Delete] Steam platform association NOT FOUND for UserGame {user_game_id}")
            logger.debug(f"🔍 [Platform Delete] Query was looking for: user_game_id={user_game_id}, platform_id={steam_platform.id}, storefront_id={steam_storefront.id}")
            return False

    except Exception as e:
        logger.error(f"💥 [Platform Delete] Error removing Steam platform association for UserGame {user_game_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to remove Steam platform association: {str(e)}")


async def unsync_steam_game_from_collection_internal(
    session: Session,
    game_id: int,
    user_id: str
) -> str:
    """
    Remove Steam game from user's collection with multi-platform protection.

    Args:
        session: Database session
        game_id: Game ID that was synced
        user_id: User ID for authorization

    Returns:
        str: Type of removal performed ("complete", "platform_only", "not_found")

    Raises:
        SteamGamesServiceError: For service errors
    """
    logger.info(f"🔍 [Unsync] Starting Steam game unsync: game_id={game_id}, user_id={user_id}")

    try:
        # Find UserGame
        user_game_query = select(UserGame).where(
            and_(UserGame.user_id == user_id, UserGame.game_id == game_id)
        )
        user_game = session.exec(user_game_query).first()

        if not user_game:
            logger.warning(f"❌ [Unsync] UserGame not found for game_id={game_id}, user_id={user_id}")
            return "not_found"

        logger.debug(f"✅ [Unsync] Found UserGame {user_game.id} for game_id={game_id}")

        # Check current platform associations BEFORE removal
        before_platforms_query = select(UserGamePlatform).where(
            UserGamePlatform.user_game_id == user_game.id
        )
        before_platforms = session.exec(before_platforms_query).all()
        logger.debug(f"🔍 [Unsync] UserGame {user_game.id} has {len(before_platforms)} platform associations BEFORE Steam removal:")
        for assoc in before_platforms:
            platform_name = assoc.platform.name if assoc.platform else "Unknown"
            storefront_name = assoc.storefront.name if assoc.storefront else "None"
            logger.debug(f"  - Platform: {platform_name}, Storefront: {storefront_name}")

        # Remove Steam platform association
        logger.debug("🔍 [Unsync] Attempting to remove Steam platform association...")
        steam_removed = await remove_steam_platform_association(session, user_game.id)

        if not steam_removed:
            logger.warning(f"❌ [Unsync] Steam platform association not found for UserGame {user_game.id}")
        else:
            logger.info(f"✅ [Unsync] Steam platform association removed for UserGame {user_game.id}")

        # Check for remaining platform associations AFTER removal
        remaining_platforms_query = select(UserGamePlatform).where(
            UserGamePlatform.user_game_id == user_game.id
        )
        remaining_platforms = session.exec(remaining_platforms_query).all()
        logger.debug(f"🔍 [Unsync] UserGame {user_game.id} has {len(remaining_platforms)} platform associations AFTER Steam removal:")
        for assoc in remaining_platforms:
            platform_name = assoc.platform.name if assoc.platform else "Unknown"
            storefront_name = assoc.storefront.name if assoc.storefront else "None"
            logger.debug(f"  - Platform: {platform_name}, Storefront: {storefront_name}")

        if len(remaining_platforms) == 0:
            # No other platforms - safe to remove entire UserGame
            logger.info(f"✅ [Unsync] No other platforms remain, removing entire UserGame {user_game.id}")
            session.delete(user_game)  # Cascades to UserGameTag automatically
            return "complete"
        else:
            # Other platforms exist - only removed Steam association
            platform_names = [p.platform.name for p in remaining_platforms if p.platform]
            logger.info(f"🔍 [Unsync] Other platforms remain for UserGame {user_game.id}: {platform_names}")
            return "platform_only"

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"💥 [Unsync] Error unsyncing Steam game from collection: game_id={game_id}, user_id={user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to unsync Steam game from collection: {str(e)}")


async def sync_steam_game_to_collection(
    session: Session,
    steam_game_id: str,
    user_id: str,
    igdb_service: IGDBService
) -> SyncResult:
    """
    Sync a matched Steam game to the user's main collection.

    Args:
        session: Database session
        steam_game_id: Steam game ID to sync
        user_id: User ID for authorization
        igdb_service: IGDB service instance

    Returns:
        SyncResult with sync details

    Raises:
        SteamGamesServiceError: For service errors
    """
    logger.info(f"🎮 [Steam Service] Syncing Steam game {steam_game_id} to collection for user {user_id}")

    try:
        # Step 1: Find and validate Steam game
        logger.debug(f"🎮 [Steam Service] Step 1: Looking up Steam game with ID {steam_game_id} for user {user_id}")
        steam_game_query = select(SteamGame).where(
            and_(
                SteamGame.id == steam_game_id,
                SteamGame.user_id == user_id
            )
        )
        steam_game = session.exec(steam_game_query).first()

        if not steam_game:
            logger.error(f"🎮 [Steam Service] Steam game not found: {steam_game_id} for user {user_id}")
            raise SteamGamesServiceError("Steam game not found or access denied")

        logger.debug(f"🎮 [Steam Service] Found Steam game: {steam_game.game_name} (AppID: {steam_game.steam_appid})")
        logger.debug(f"🎮 [Steam Service] Steam game status: IGDB ID: {steam_game.igdb_id}, Ignored: {steam_game.ignored}")

        # Step 2: Validate Steam game has IGDB match
        if not steam_game.igdb_id:
            logger.error(f"🎮 [Steam Service] Steam game {steam_game_id} not matched to IGDB")
            raise SteamGamesServiceError("Steam game must be matched to IGDB before syncing to collection")

        logger.debug(f"🎮 [Steam Service] Step 2: Steam game is matched to IGDB ID: {steam_game.igdb_id}")

        # Step 3: Check if Game record exists, create if needed
        logger.debug(f"🎮 [Steam Service] Step 3: Looking for existing Game record with IGDB ID: {steam_game.igdb_id}")
        game_query = select(Game).where(Game.id == steam_game.igdb_id)
        game = session.exec(game_query).first()

        if not game:
            # Create Game record using GameService
            logger.info(f"🎮 [Steam Service] Step 3a: Creating Game record from IGDB for igdb_id {steam_game.igdb_id}")
            try:
                # Use GameService to create Game record with proper platform handling
                game_service = GameService(session, igdb_service)
                game = await game_service.create_or_update_game_from_igdb(
                    igdb_id=steam_game.igdb_id,
                    download_cover_art=True,
                )
                logger.info(f"🎮 [Steam Service] Created Game record {game.id} from IGDB ID {steam_game.igdb_id} via GameService")

            except Exception as e:
                logger.error(f"🎮 [Steam Service] Error creating Game from IGDB ID {steam_game.igdb_id}: {str(e)}")
                raise SteamGamesServiceError(f"Failed to create game from IGDB data: {str(e)}")
        else:
            logger.debug(f"🎮 [Steam Service] Step 3b: Found existing Game record: {game.title} (ID: {game.id})")

        # Step 4: Check if UserGame relationship exists
        logger.debug(f"🎮 [Steam Service] Step 4: Checking for UserGame relationship for user {user_id} and game {game.id}")
        user_game_query = select(UserGame).where(
            and_(
                UserGame.user_id == user_id,
                UserGame.game_id == game.id
            )
        )
        user_game = session.exec(user_game_query).first()

        action = "updated_existing"
        if not user_game:
            # Create new UserGame
            logger.info("🎮 [Steam Service] Step 4a: Creating new UserGame relationship")
            user_game = UserGame(
                user_id=user_id,
                game_id=game.id,
                ownership_status=OwnershipStatus.OWNED,
                play_status=PlayStatus.NOT_STARTED,
                is_loved=False,
                hours_played=0,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            session.add(user_game)
            session.flush()  # Get the user_game ID
            action = "created_new"
            logger.info(f"🎮 [Steam Service] Created new UserGame {user_game.id} for game {game.id}")
        else:
            logger.debug(f"🎮 [Steam Service] Step 4b: Found existing UserGame relationship: {user_game.id}")

        # Step 5: Ensure Steam platform/storefront association exists
        logger.debug("🎮 [Steam Service] Step 5: Looking up Steam platform and storefront")
        # Get platform and storefront objects
        pc_platform_query = select(Platform).where(Platform.name == "pc-windows")
        pc_platform = session.exec(pc_platform_query).first()

        steam_storefront_query = select(Storefront).where(Storefront.name == "steam")
        steam_storefront = session.exec(steam_storefront_query).first()

        if not pc_platform or not steam_storefront:
            logger.error(f"🎮 [Steam Service] Missing platform/storefront: PC={pc_platform}, Steam={steam_storefront}")
            raise SteamGamesServiceError("PC (Windows) platform or Steam storefront not found in database")

        logger.debug(f"🎮 [Steam Service] Found PC platform: {pc_platform.name} (ID: {pc_platform.id})")
        logger.debug(f"🎮 [Steam Service] Found Steam storefront: {steam_storefront.name} (ID: {steam_storefront.id})")

        # Check if platform association already exists
        logger.debug("🎮 [Steam Service] Step 5a: Checking for existing platform association")
        platform_association_query = select(UserGamePlatform).where(
            and_(
                UserGamePlatform.user_game_id == user_game.id,
                UserGamePlatform.platform_id == pc_platform.id,
                UserGamePlatform.storefront_id == steam_storefront.id
            )
        )
        existing_association = session.exec(platform_association_query).first()

        if not existing_association:
            # Create Steam platform association
            logger.info(f"🎮 [Steam Service] Step 5b: Creating Steam platform association for UserGame {user_game.id}")
            platform_association = UserGamePlatform(
                user_game_id=user_game.id,
                platform_id=pc_platform.id,
                storefront_id=steam_storefront.id,
                is_available=True,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            session.add(platform_association)
            logger.info(f"🎮 [Steam Service] Added Steam platform association for UserGame {user_game.id}")
        else:
            logger.debug("🎮 [Steam Service] Step 5c: Steam platform association already exists")

        # Commit all changes
        logger.debug("🎮 [Steam Service] Step 6: Committing all database changes")
        session.commit()
        session.refresh(steam_game)

        logger.info(f"🎮 [Steam Service] Steam game sync completed: {steam_game_id} -> UserGame {user_game.id} ({action}) for user {user_id}")

        result = SyncResult(
            steam_game_id=steam_game_id,
            steam_game_name=steam_game.game_name,
            user_game_id=user_game.id,
            action=action
        )
        logger.debug(f"🎮 [Steam Service] Returning result: {result}")
        return result

    except SteamGamesServiceError as e:
        logger.error(f"🎮 [Steam Service] SteamGamesServiceError in sync: {str(e)}")
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"🎮 [Steam Service] Unexpected error syncing Steam game {steam_game_id} for user {user_id}: {str(e)}")
        logger.exception("🎮 [Steam Service] Exception details:")
        # Try to get game name from locals if steam_game was assigned
        game_name = 'Unknown'
        if 'steam_game' in dir() and steam_game is not None:  # type: ignore[possibly-undefined]
            game_name = getattr(steam_game, 'game_name', 'Unknown')  # type: ignore[possibly-undefined]
        return SyncResult(
            steam_game_id=steam_game_id,
            steam_game_name=game_name,
            user_game_id=None,
            action="failed",
            error_message=str(e)
        )


async def sync_all_matched_games(
    session: Session,
    user_id: str,
    igdb_service: IGDBService
) -> BulkSyncResults:
    """
    Sync all matched Steam games to the user's main collection.

    Args:
        session: Database session
        user_id: User ID for authorization
        igdb_service: IGDB service instance

    Returns:
        BulkSyncResults with detailed sync statistics

    Raises:
        SteamGamesServiceError: For service errors
    """
    logger.info(f"Starting bulk sync for all matched Steam games for user {user_id}")

    try:
        # Find all matched Steam games that haven't been synced yet
        candidate_steam_games_query = select(SteamGame).where(
            and_(
                SteamGame.user_id == user_id,
                is_not(SteamGame.igdb_id, None),  # Has IGDB match
                not SteamGame.ignored       # Not ignored
            )
        )
        candidate_steam_games = session.exec(candidate_steam_games_query).all()

        # Filter for games that are not yet synced by checking each one
        matched_steam_games = []
        for steam_game in candidate_steam_games:
            if steam_game.igdb_id is None:
                continue
            is_synced = is_steam_game_synced(session, user_id, steam_game.igdb_id)
            if not is_synced:
                matched_steam_games.append(steam_game)

        total_processed = len(matched_steam_games)
        successful_syncs = 0
        failed_syncs = 0
        skipped_games = 0
        results = []
        errors = []

        logger.info(f"Found {total_processed} matched Steam games to sync for user {user_id}")

        if total_processed == 0:
            return BulkSyncResults(
                total_processed=0,
                successful_syncs=0,
                failed_syncs=0,
                skipped_games=0,
                results=[],
                errors=[]
            )

        # Get platform and storefront objects once for efficiency
        pc_platform_query = select(Platform).where(Platform.name == "pc-windows")
        pc_platform = session.exec(pc_platform_query).first()

        steam_storefront_query = select(Storefront).where(Storefront.name == "steam")
        steam_storefront = session.exec(steam_storefront_query).first()

        if not pc_platform or not steam_storefront:
            raise SteamGamesServiceError("PC (Windows) platform or Steam storefront not found in database")

        # Process each Steam game
        for steam_game in matched_steam_games:
            try:
                # Check if Game record exists, create if needed
                game_query = select(Game).where(Game.id == steam_game.igdb_id)
                game = session.exec(game_query).first()

                if not game:
                    # Create Game record using GameService
                    logger.debug(f"Creating Game record from IGDB for igdb_id {steam_game.igdb_id}")
                    try:
                        # Use GameService to create Game record with proper platform handling
                        game_service = GameService(session, igdb_service)
                        game = await game_service.create_or_update_game_from_igdb(
                            igdb_id=steam_game.igdb_id,
                            download_cover_art=True,
                        )
                        logger.debug(f"Created Game record {game.id} from IGDB ID {steam_game.igdb_id} via GameService")

                    except Exception as e:
                        logger.error(f"Error creating Game from IGDB ID {steam_game.igdb_id}: {str(e)}")
                        # Ensure session is rolled back after any error
                        try:
                            session.rollback()
                        except Exception:
                            pass  # Ignore rollback errors
                        error_msg = f"Failed to create game record for '{steam_game.game_name}': {str(e)}"
                        errors.append(error_msg)
                        results.append(SyncResult(
                            steam_game_id=steam_game.id,
                            steam_game_name=steam_game.game_name,
                            user_game_id=None,
                            action="failed",
                            error_message=error_msg
                        ))
                        failed_syncs += 1
                        continue

                # Check if UserGame relationship exists
                user_game_query = select(UserGame).where(
                    and_(
                        UserGame.user_id == user_id,
                        UserGame.game_id == game.id
                    )
                )
                user_game = session.exec(user_game_query).first()

                action = "updated_existing"
                if not user_game:
                    # Create new UserGame
                    user_game = UserGame(
                        user_id=user_id,
                        game_id=game.id,
                        ownership_status=OwnershipStatus.OWNED,
                        play_status=PlayStatus.NOT_STARTED,
                        is_loved=False,
                        hours_played=0,
                        created_at=datetime.now(timezone.utc),
                        updated_at=datetime.now(timezone.utc)
                    )
                    session.add(user_game)
                    session.flush()  # Get the user_game ID
                    action = "created_new"
                    logger.debug(f"Created new UserGame {user_game.id} for game {game.id}")

                # Ensure Steam platform/storefront association exists
                platform_association_query = select(UserGamePlatform).where(
                    and_(
                        UserGamePlatform.user_game_id == user_game.id,
                        UserGamePlatform.platform_id == pc_platform.id,
                        UserGamePlatform.storefront_id == steam_storefront.id
                    )
                )
                existing_association = session.exec(platform_association_query).first()

                if not existing_association:
                    # Create Steam platform association
                    platform_association = UserGamePlatform(
                        user_game_id=user_game.id,
                        platform_id=pc_platform.id,
                        storefront_id=steam_storefront.id,
                        is_available=True,
                        created_at=datetime.now(timezone.utc),
                        updated_at=datetime.now(timezone.utc)
                    )
                    session.add(platform_association)
                    logger.debug(f"Added Steam platform association for UserGame {user_game.id}")

                # Update Steam game timestamp
                steam_game.updated_at = datetime.now(timezone.utc)
                session.add(steam_game)

                # Add successful result
                results.append(SyncResult(
                    steam_game_id=steam_game.id,
                    steam_game_name=steam_game.game_name,
                    user_game_id=user_game.id,
                    action=action
                ))

                successful_syncs += 1
                logger.debug(f"Successfully synced Steam game '{steam_game.game_name}' to collection")

            except Exception as e:
                logger.error(f"Error syncing Steam game '{steam_game.game_name}' (ID: {steam_game.id}): {str(e)}")
                # Ensure session is rolled back after sync errors
                try:
                    session.rollback()
                except Exception:
                    pass  # Ignore rollback errors
                error_msg = f"Failed to sync '{steam_game.game_name}': {str(e)}"
                errors.append(error_msg)
                results.append(SyncResult(
                    steam_game_id=steam_game.id,
                    steam_game_name=steam_game.game_name,
                    user_game_id=None,
                    action="failed",
                    error_message=error_msg
                ))
                failed_syncs += 1
                continue

        # Commit all changes
        session.commit()

        logger.info(f"Bulk Steam game sync completed for user {user_id}: {successful_syncs} successful, {failed_syncs} failed")

        return BulkSyncResults(
            total_processed=total_processed,
            successful_syncs=successful_syncs,
            failed_syncs=failed_syncs,
            skipped_games=skipped_games,  # Always 0 as we pre-filter
            results=results,
            errors=errors
        )

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        raise
    except Exception as e:
        logger.error(f"Error during bulk Steam game sync for user {user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to sync Steam games to collection: {str(e)}")


async def unsync_steam_game_from_collection(
    session: Session,
    steam_game_id: str,
    user_id: str
) -> Tuple[SteamGame, str]:
    """
    Unsync a single Steam game from the user's collection.

    This removes the Steam platform/storefront from the UserGame.

    Args:
        session: Database session
        steam_game_id: Steam game ID to unsync
        user_id: User ID for authorization

    Returns:
        Tuple of (updated SteamGame, status message)

    Raises:
        SteamGamesServiceError: For service errors
    """
    try:
        # Find and validate Steam game
        steam_game = session.get(SteamGame, steam_game_id)
        if not steam_game or steam_game.user_id != user_id:
            raise SteamGamesServiceError(f"Steam game {steam_game_id} not found or access denied")

        # Check if game is actually synced using new sync function
        igdb_id = steam_game.igdb_id
        if not igdb_id:
            raise SteamGamesServiceError(f"Steam game '{steam_game.game_name}' has no IGDB match")

        is_synced = is_steam_game_synced(session, user_id, igdb_id)
        if not is_synced:
            raise SteamGamesServiceError(f"Steam game '{steam_game.game_name}' is not synced to collection")

        logger.info(f"Unsyncing Steam game '{steam_game.game_name}' (IGDB ID: {igdb_id}) for user {user_id}")

        # Perform unsync from collection using IGDB ID
        unsync_result = await unsync_steam_game_from_collection_internal(session, igdb_id, user_id)
        logger.debug(f"Unsync operation result: {unsync_result}")

        # Update timestamp
        steam_game.updated_at = datetime.now(timezone.utc)

        # Commit changes
        session.add(steam_game)
        session.commit()

        # Create response message
        if unsync_result == "complete":
            message = f"Removed Steam game '{steam_game.game_name}' from collection"
        elif unsync_result == "platform_only":
            message = f"Removed Steam platform from '{steam_game.game_name}' (other platforms retained)"
        else:
            message = f"Unsynced Steam game '{steam_game.game_name}' from collection"

        logger.info(f"Successfully unsynced Steam game '{steam_game.game_name}' for user {user_id}")

        return steam_game, message

    except SteamGamesServiceError:
        # Re-raise service errors without wrapping
        session.rollback()
        raise
    except Exception as e:
        session.rollback()
        logger.error(f"Error unsyncing Steam game {steam_game_id} for user {user_id}: {str(e)}")
        raise SteamGamesServiceError(f"Failed to unsync Steam game: {str(e)}")

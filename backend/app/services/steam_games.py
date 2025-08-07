"""
Steam Games service for Steam library management and sync operations.

This service centralizes all Steam games business logic including:
- Steam library import with automatic IGDB matching
- Manual IGDB matching workflows
- Collection sync operations (single and bulk)
- Ignore/un-ignore game management
- Comprehensive error handling and logging
"""

import logging
from typing import Optional, Dict, Any, List, Tuple
from datetime import datetime, timezone
from dataclasses import dataclass
from sqlmodel import Session, select, and_
import asyncio

from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.game import Game
from ..models.user_game import UserGame, UserGamePlatform, OwnershipStatus, PlayStatus
from ..models.platform import Platform, Storefront
from ..services.steam import SteamService, SteamAuthenticationError, SteamAPIError
from ..services.igdb import IGDBService, IGDBError
from ..core.database import get_session


logger = logging.getLogger(__name__)


@dataclass
class AutoMatchResult:
    """Result of automatic IGDB matching for a single Steam game."""
    steam_game_id: str
    steam_game_name: str
    steam_appid: int
    matched: bool
    igdb_id: Optional[str] = None
    igdb_game_title: Optional[str] = None
    confidence_score: Optional[float] = None
    error_message: Optional[str] = None


@dataclass
class AutoMatchResults:
    """Results of automatic IGDB matching for multiple Steam games."""
    total_processed: int
    successful_matches: int
    failed_matches: int
    skipped_games: int
    results: List[AutoMatchResult]
    errors: List[str]


@dataclass
class ImportResult:
    """Result of Steam library import operation."""
    imported_count: int
    skipped_count: int
    auto_matched_count: int
    total_games: int
    errors: List[str]


@dataclass
class SyncResult:
    """Result of Steam game collection sync operation."""
    steam_game_id: str
    steam_game_name: str
    user_game_id: Optional[str]
    action: str  # "created_new", "updated_existing", "failed"
    error_message: Optional[str] = None


@dataclass
class BulkSyncResults:
    """Results of bulk Steam game collection sync operation."""
    total_processed: int
    successful_syncs: int
    failed_syncs: int
    skipped_games: int
    results: List[SyncResult]
    errors: List[str]


class SteamGamesServiceError(Exception):
    """Base exception for Steam Games service errors."""
    pass


class SteamGamesService:
    """Service for Steam games import, matching, and sync operations."""
    
    def __init__(
        self, 
        session: Session,
        steam_service: Optional[SteamService] = None,
        igdb_service: Optional[IGDBService] = None
    ):
        """
        Initialize Steam Games service.
        
        Args:
            session: Database session for operations
            steam_service: Optional Steam service instance (for testing)
            igdb_service: Optional IGDB service instance (for testing)
        """
        self.session = session
        self._steam_service = steam_service
        self._igdb_service = igdb_service
        
        # Configuration for automatic matching
        self.auto_match_confidence_threshold = 0.90  # 90% similarity required
        self.auto_match_batch_size = 10  # Process in batches for rate limiting
        
        logger.debug("SteamGamesService initialized")
    
    @property
    def igdb_service(self) -> IGDBService:
        """Get IGDB service instance (lazy initialization)."""
        if self._igdb_service is None:
            self._igdb_service = IGDBService()
        return self._igdb_service
    
    def _create_steam_service(self, api_key: str) -> SteamService:
        """Create Steam service instance (allows injection for testing)."""
        if self._steam_service is not None:
            return self._steam_service
        
        from ..services.steam import create_steam_service
        return create_steam_service(api_key)
    
    async def import_steam_library(
        self, 
        user_id: str, 
        steam_config: Dict[str, Any],
        enable_auto_matching: bool = True
    ) -> ImportResult:
        """
        Import user's Steam library with optional automatic IGDB matching.
        
        Args:
            user_id: User ID to import library for
            steam_config: Steam configuration (web_api_key, steam_id)
            enable_auto_matching: Whether to attempt automatic IGDB matching
            
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
            user = self.session.get(User, user_id)
            if not user:
                raise SteamGamesServiceError(f"User {user_id} not found")
            
            # Create Steam service
            steam_service = self._create_steam_service(steam_config["web_api_key"])
            
            # Get Steam library
            steam_games = await steam_service.get_owned_games(steam_config["steam_id"])
            logger.info(f"Retrieved {len(steam_games)} games from Steam for user {user_id}")
            
            imported_count = 0
            skipped_count = 0
            errors = []
            
            # Import games with deduplication
            new_steam_games = []
            for steam_game in steam_games:
                try:
                    # Check if game already exists for this user
                    existing_query = select(SteamGame).where(
                        and_(
                            SteamGame.user_id == user_id,
                            SteamGame.steam_appid == steam_game.appid
                        )
                    )
                    existing_game = self.session.exec(existing_query).first()
                    
                    if existing_game:
                        # Update existing game name in case it changed
                        if existing_game.game_name != steam_game.name:
                            existing_game.game_name = steam_game.name
                            existing_game.updated_at = datetime.now(timezone.utc)
                            self.session.add(existing_game)
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
                        self.session.add(new_steam_game)
                        new_steam_games.append(new_steam_game)
                        imported_count += 1
                        logger.debug(f"Added new Steam game {steam_game.appid} ({steam_game.name}) for user {user_id}")
                        
                except Exception as e:
                    error_msg = f"Error processing Steam game {steam_game.appid} ({getattr(steam_game, 'name', 'Unknown')}): {str(e)}"
                    logger.error(error_msg)
                    errors.append(error_msg)
                    continue
            
            # Commit imported games first
            self.session.commit()
            logger.info(f"Steam library import phase completed for user {user_id}: {imported_count} imported, {skipped_count} skipped")
            
            # Automatic IGDB matching for newly imported games
            auto_matched_count = 0
            if enable_auto_matching and new_steam_games:
                logger.info(f"Starting automatic IGDB matching for {len(new_steam_games)} new Steam games")
                try:
                    # Refresh Steam game records to get database IDs
                    self.session.refresh_all(new_steam_games)
                    
                    # Perform automatic matching
                    match_results = await self._auto_match_steam_games([sg.id for sg in new_steam_games])
                    auto_matched_count = match_results.successful_matches
                    
                    # Add any matching errors to overall error list
                    errors.extend(match_results.errors)
                    
                    logger.info(f"Automatic IGDB matching completed: {auto_matched_count} games matched")
                    
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
            self.session.rollback()
            raise SteamGamesServiceError(f"Failed to import Steam library: {str(e)}")
    
    async def _auto_match_steam_games(self, steam_game_ids: List[str]) -> AutoMatchResults:
        """
        Automatically match Steam games to IGDB games using fuzzy matching.
        
        Args:
            steam_game_ids: List of Steam game IDs to attempt matching
            
        Returns:
            AutoMatchResults with detailed matching results
        """
        logger.info(f"Starting automatic IGDB matching for {len(steam_game_ids)} Steam games")
        
        results = []
        successful_matches = 0
        failed_matches = 0
        skipped_games = 0
        errors = []
        
        # Process in batches to respect rate limits
        for i in range(0, len(steam_game_ids), self.auto_match_batch_size):
            batch = steam_game_ids[i:i + self.auto_match_batch_size]
            logger.debug(f"Processing auto-match batch {i//self.auto_match_batch_size + 1}: {len(batch)} games")
            
            for steam_game_id in batch:
                try:
                    result = await self._auto_match_single_steam_game(steam_game_id)
                    results.append(result)
                    
                    if result.matched:
                        successful_matches += 1
                        logger.debug(f"Auto-matched {result.steam_game_name} -> {result.igdb_game_title} (confidence: {result.confidence_score:.2f})")
                    elif result.error_message:
                        failed_matches += 1
                        errors.append(result.error_message)
                    else:
                        skipped_games += 1  # Low confidence, skip for manual matching
                        
                except Exception as e:
                    error_msg = f"Error auto-matching Steam game {steam_game_id}: {str(e)}"
                    logger.error(error_msg)
                    errors.append(error_msg)
                    failed_matches += 1
                    continue
            
            # Small delay between batches to be respectful of IGDB API
            if i + self.auto_match_batch_size < len(steam_game_ids):
                await asyncio.sleep(0.5)
        
        logger.info(f"Automatic IGDB matching completed: {successful_matches} matched, {failed_matches} failed, {skipped_games} skipped")
        
        return AutoMatchResults(
            total_processed=len(steam_game_ids),
            successful_matches=successful_matches,
            failed_matches=failed_matches,
            skipped_games=skipped_games,
            results=results,
            errors=errors
        )
    
    async def _auto_match_single_steam_game(self, steam_game_id: str) -> AutoMatchResult:
        """
        Attempt to automatically match a single Steam game to IGDB.
        
        Args:
            steam_game_id: Steam game ID to match
            
        Returns:
            AutoMatchResult with matching details
        """
        # Get Steam game
        steam_game = self.session.get(SteamGame, steam_game_id)
        if not steam_game:
            return AutoMatchResult(
                steam_game_id=steam_game_id,
                steam_game_name="Unknown",
                steam_appid=0,
                matched=False,
                error_message="Steam game not found in database"
            )
        
        # Skip if already matched
        if steam_game.igdb_id:
            return AutoMatchResult(
                steam_game_id=steam_game_id,
                steam_game_name=steam_game.game_name,
                steam_appid=steam_game.steam_appid,
                matched=False,
                error_message="Steam game already has IGDB match"
            )
        
        try:
            # Search IGDB for potential matches
            igdb_candidates = await self.igdb_service.search_games(
                query=steam_game.game_name,
                limit=5,  # Get top 5 candidates
                fuzzy_threshold=self.auto_match_confidence_threshold
            )
            
            if not igdb_candidates:
                return AutoMatchResult(
                    steam_game_id=steam_game_id,
                    steam_game_name=steam_game.game_name,
                    steam_appid=steam_game.steam_appid,
                    matched=False
                )
            
            # Get the best match (first result, as they're ranked by fuzzy matching)
            best_match = igdb_candidates[0]
            
            # Calculate confidence score using fuzzy matching
            from rapidfuzz import fuzz
            confidence = fuzz.ratio(steam_game.game_name.lower(), best_match.title.lower()) / 100.0
            
            # Only auto-match if confidence is above threshold
            if confidence >= self.auto_match_confidence_threshold:
                # Check if Game record exists in our database, create if needed
                game_query = select(Game).where(Game.igdb_id == best_match.igdb_id)
                existing_game = self.session.exec(game_query).first()
                
                if not existing_game:
                    # Create Game record from IGDB metadata
                    game = Game(
                        igdb_id=best_match.igdb_id,
                        title=best_match.title,
                        summary=best_match.description,
                        release_date=best_match.release_date,
                        cover_art_url=best_match.cover_art_url,
                        igdb_rating=best_match.rating_average,
                        igdb_rating_count=best_match.rating_count,
                        time_to_beat_hastily=best_match.hastily,
                        time_to_beat_normally=best_match.normally,
                        time_to_beat_completely=best_match.completely,
                        igdb_platforms=best_match.igdb_platform_ids,
                        igdb_platform_names=best_match.platform_names,
                        created_at=datetime.now(timezone.utc),
                        updated_at=datetime.now(timezone.utc)
                    )
                    self.session.add(game)
                    self.session.flush()  # Get the ID
                
                # Update Steam game with IGDB match
                steam_game.igdb_id = best_match.igdb_id
                steam_game.updated_at = datetime.now(timezone.utc)
                self.session.add(steam_game)
                self.session.commit()
                
                return AutoMatchResult(
                    steam_game_id=steam_game_id,
                    steam_game_name=steam_game.game_name,
                    steam_appid=steam_game.steam_appid,
                    matched=True,
                    igdb_id=best_match.igdb_id,
                    igdb_game_title=best_match.title,
                    confidence_score=confidence
                )
            else:
                # Low confidence, leave for manual matching
                return AutoMatchResult(
                    steam_game_id=steam_game_id,
                    steam_game_name=steam_game.game_name,
                    steam_appid=steam_game.steam_appid,
                    matched=False,
                    confidence_score=confidence
                )
                
        except IGDBError as e:
            error_msg = f"IGDB error matching '{steam_game.game_name}': {str(e)}"
            logger.error(error_msg)
            return AutoMatchResult(
                steam_game_id=steam_game_id,
                steam_game_name=steam_game.game_name,
                steam_appid=steam_game.steam_appid,
                matched=False,
                error_message=error_msg
            )
        except Exception as e:
            error_msg = f"Unexpected error matching '{steam_game.game_name}': {str(e)}"
            logger.error(error_msg)
            return AutoMatchResult(
                steam_game_id=steam_game_id,
                steam_game_name=steam_game.game_name,
                steam_appid=steam_game.steam_appid,
                matched=False,
                error_message=error_msg
            )
    
    # Placeholder methods for remaining functionality
    # These will be implemented in subsequent steps
    
    async def match_steam_game_to_igdb(
        self, 
        steam_game_id: str, 
        igdb_id: Optional[str], 
        user_id: str
    ) -> Tuple[SteamGame, str]:
        """
        Manually match or unmatch a Steam game to/from an IGDB game.
        
        Args:
            steam_game_id: Steam game ID to match
            igdb_id: IGDB ID to match to, or None to clear match
            user_id: User ID for authorization
            
        Returns:
            Tuple of (updated SteamGame, success message)
            
        Raises:
            SteamGamesServiceError: For service errors
        """
        logger.info(f"Matching Steam game {steam_game_id} to IGDB ID {igdb_id} for user {user_id}")
        
        try:
            # Find the Steam game and verify ownership
            steam_game_query = select(SteamGame).where(
                and_(
                    SteamGame.id == steam_game_id,
                    SteamGame.user_id == user_id
                )
            )
            steam_game = self.session.exec(steam_game_query).first()
            
            if not steam_game:
                raise SteamGamesServiceError("Steam game not found or access denied")
            
            # If igdb_id is provided, validate it exists in games table
            if igdb_id is not None:
                game_query = select(Game).where(Game.igdb_id == igdb_id)
                existing_game = self.session.exec(game_query).first()
                
                if not existing_game:
                    raise SteamGamesServiceError("Invalid IGDB ID. Game must exist in the main games collection first.")
            
            # Update the Steam game's IGDB ID
            old_igdb_id = steam_game.igdb_id
            steam_game.igdb_id = igdb_id
            steam_game.updated_at = datetime.now(timezone.utc)
            
            self.session.add(steam_game)
            self.session.commit()
            self.session.refresh(steam_game)
            
            # Create response message
            if igdb_id is None:
                if old_igdb_id:
                    message = f"Cleared IGDB match for Steam game '{steam_game.game_name}'"
                else:
                    message = f"No IGDB match to clear for Steam game '{steam_game.game_name}'"
            else:
                if old_igdb_id:
                    message = f"Updated IGDB match for Steam game '{steam_game.game_name}'"
                else:
                    message = f"Successfully matched Steam game '{steam_game.game_name}' to IGDB"
            
            logger.info(f"Steam game {steam_game_id} IGDB match updated: {old_igdb_id} -> {igdb_id} by user {user_id}")
            
            return steam_game, message
            
        except SteamGamesServiceError:
            # Re-raise service errors without wrapping
            raise
        except Exception as e:
            logger.error(f"Error matching Steam game {steam_game_id} to IGDB for user {user_id}: {str(e)}")
            raise SteamGamesServiceError(f"Failed to match Steam game to IGDB: {str(e)}")
    
    async def sync_steam_game_to_collection(
        self, 
        steam_game_id: str, 
        user_id: str
    ) -> SyncResult:
        """
        Sync a matched Steam game to the user's main collection.
        
        Args:
            steam_game_id: Steam game ID to sync
            user_id: User ID for authorization
            
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
            steam_game = self.session.exec(steam_game_query).first()
            
            if not steam_game:
                logger.error(f"🎮 [Steam Service] Steam game not found: {steam_game_id} for user {user_id}")
                raise SteamGamesServiceError("Steam game not found or access denied")
            
            logger.debug(f"🎮 [Steam Service] Found Steam game: {steam_game.game_name} (AppID: {steam_game.steam_appid})")
            logger.debug(f"🎮 [Steam Service] Steam game status: IGDB ID: {steam_game.igdb_id}, Game ID: {steam_game.game_id}, Ignored: {steam_game.ignored}")
            
            # Step 2: Validate Steam game has IGDB match
            if not steam_game.igdb_id:
                logger.error(f"🎮 [Steam Service] Steam game {steam_game_id} not matched to IGDB")
                raise SteamGamesServiceError("Steam game must be matched to IGDB before syncing to collection")
            
            logger.debug(f"🎮 [Steam Service] Step 2: Steam game is matched to IGDB ID: {steam_game.igdb_id}")
            
            # Step 3: Check if Game record exists, create if needed
            logger.debug(f"🎮 [Steam Service] Step 3: Looking for existing Game record with IGDB ID: {steam_game.igdb_id}")
            game_query = select(Game).where(Game.igdb_id == steam_game.igdb_id)
            game = self.session.exec(game_query).first()
            
            if not game:
                # Create Game record from IGDB
                logger.info(f"🎮 [Steam Service] Step 3a: Creating Game record from IGDB for igdb_id {steam_game.igdb_id}")
                try:
                    logger.debug(f"🎮 [Steam Service] Fetching game metadata from IGDB...")
                    game_metadata = await self.igdb_service.get_game_by_id(steam_game.igdb_id)
                    if not game_metadata:
                        logger.error(f"🎮 [Steam Service] Could not fetch game data from IGDB for ID {steam_game.igdb_id}")
                        raise SteamGamesServiceError(f"Could not fetch game data from IGDB for ID {steam_game.igdb_id}")
                    
                    logger.debug(f"🎮 [Steam Service] Got IGDB metadata: {game_metadata.title}")
                    
                    # Create new Game record from GameMetadata
                    game = Game(
                        igdb_id=steam_game.igdb_id,
                        title=game_metadata.title,
                        summary=game_metadata.description,
                        release_date=game_metadata.release_date,
                        cover_art_url=game_metadata.cover_art_url,
                        igdb_rating=game_metadata.rating_average,
                        igdb_rating_count=game_metadata.rating_count,
                        time_to_beat_hastily=game_metadata.hastily,
                        time_to_beat_normally=game_metadata.normally,
                        time_to_beat_completely=game_metadata.completely,
                        igdb_platforms=game_metadata.igdb_platform_ids,
                        igdb_platform_names=game_metadata.platform_names,
                        created_at=datetime.now(timezone.utc),
                        updated_at=datetime.now(timezone.utc)
                    )
                    self.session.add(game)
                    self.session.flush()  # Get the game ID
                    logger.info(f"🎮 [Steam Service] Created Game record {game.id} from IGDB ID {steam_game.igdb_id}")
                    
                except Exception as e:
                    logger.error(f"🎮 [Steam Service] Error creating Game from IGDB ID {steam_game.igdb_id}: {str(e)}")
                    raise SteamGamesServiceError("Failed to create game from IGDB data")
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
            user_game = self.session.exec(user_game_query).first()
            
            action = "updated_existing"
            if not user_game:
                # Create new UserGame
                logger.info(f"🎮 [Steam Service] Step 4a: Creating new UserGame relationship")
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
                self.session.add(user_game)
                self.session.flush()  # Get the user_game ID
                action = "created_new"
                logger.info(f"🎮 [Steam Service] Created new UserGame {user_game.id} for game {game.id}")
            else:
                logger.debug(f"🎮 [Steam Service] Step 4b: Found existing UserGame relationship: {user_game.id}")
            
            # Step 5: Ensure Steam platform/storefront association exists
            logger.debug(f"🎮 [Steam Service] Step 5: Looking up Steam platform and storefront")
            # Get platform and storefront objects
            pc_platform_query = select(Platform).where(Platform.name == "pc-windows")
            pc_platform = self.session.exec(pc_platform_query).first()
            
            steam_storefront_query = select(Storefront).where(Storefront.name == "steam")
            steam_storefront = self.session.exec(steam_storefront_query).first()
            
            if not pc_platform or not steam_storefront:
                logger.error(f"🎮 [Steam Service] Missing platform/storefront: PC={pc_platform}, Steam={steam_storefront}")
                raise SteamGamesServiceError("PC (Windows) platform or Steam storefront not found in database")
            
            logger.debug(f"🎮 [Steam Service] Found PC platform: {pc_platform.name} (ID: {pc_platform.id})")
            logger.debug(f"🎮 [Steam Service] Found Steam storefront: {steam_storefront.name} (ID: {steam_storefront.id})")
            
            # Check if platform association already exists
            logger.debug(f"🎮 [Steam Service] Step 5a: Checking for existing platform association")
            platform_association_query = select(UserGamePlatform).where(
                and_(
                    UserGamePlatform.user_game_id == user_game.id,
                    UserGamePlatform.platform_id == pc_platform.id,
                    UserGamePlatform.storefront_id == steam_storefront.id
                )
            )
            existing_association = self.session.exec(platform_association_query).first()
            
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
                self.session.add(platform_association)
                logger.info(f"🎮 [Steam Service] Added Steam platform association for UserGame {user_game.id}")
            else:
                logger.debug(f"🎮 [Steam Service] Step 5c: Steam platform association already exists")
            
            # Step 6: Update Steam game sync tracking (only if not already set)
            logger.debug(f"🎮 [Steam Service] Step 6: Updating Steam game sync tracking")
            if steam_game.game_id is None:
                logger.info(f"🎮 [Steam Service] Setting Steam game game_id to {game.id}")
                steam_game.game_id = game.id
                steam_game.updated_at = datetime.now(timezone.utc)
                self.session.add(steam_game)
                logger.info(f"🎮 [Steam Service] Set SteamGame {steam_game_id} game_id to {game.id}")
            else:
                logger.debug(f"🎮 [Steam Service] Steam game already has game_id set: {steam_game.game_id}")
            
            # Commit all changes
            logger.debug(f"🎮 [Steam Service] Step 7: Committing all database changes")
            self.session.commit()
            self.session.refresh(steam_game)
            
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
            return SyncResult(
                steam_game_id=steam_game_id,
                steam_game_name=getattr(steam_game, 'game_name', 'Unknown') if 'steam_game' in locals() else 'Unknown',
                user_game_id=None,
                action="failed",
                error_message=str(e)
            )
    
    async def sync_all_matched_games(self, user_id: str) -> BulkSyncResults:
        """
        Sync all matched Steam games to the user's main collection.
        
        Args:
            user_id: User ID for authorization
            
        Returns:
            BulkSyncResults with detailed sync statistics
            
        Raises:
            SteamGamesServiceError: For service errors
        """
        logger.info(f"Starting bulk sync for all matched Steam games for user {user_id}")
        
        try:
            # Find all matched Steam games that haven't been synced yet
            matched_steam_games_query = select(SteamGame).where(
                and_(
                    SteamGame.user_id == user_id,
                    SteamGame.igdb_id.isnot(None),  # Has IGDB match
                    SteamGame.game_id.is_(None),     # Not yet synced
                    SteamGame.ignored == False       # Not ignored
                )
            )
            matched_steam_games = self.session.exec(matched_steam_games_query).all()
            
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
            pc_platform = self.session.exec(pc_platform_query).first()
            
            steam_storefront_query = select(Storefront).where(Storefront.name == "steam")
            steam_storefront = self.session.exec(steam_storefront_query).first()
            
            if not pc_platform or not steam_storefront:
                raise SteamGamesServiceError("PC (Windows) platform or Steam storefront not found in database")
            
            # Process each Steam game
            for steam_game in matched_steam_games:
                try:
                    # Check if Game record exists, create if needed
                    game_query = select(Game).where(Game.igdb_id == steam_game.igdb_id)
                    game = self.session.exec(game_query).first()
                    
                    if not game:
                        # Create Game record from IGDB
                        logger.debug(f"Creating Game record from IGDB for igdb_id {steam_game.igdb_id}")
                        try:
                            game_metadata = await self.igdb_service.get_game_by_id(steam_game.igdb_id)
                            if not game_metadata:
                                error_msg = f"Could not fetch game data from IGDB for '{steam_game.game_name}' (IGDB ID: {steam_game.igdb_id})"
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
                            
                            # Create new Game record from GameMetadata
                            game = Game(
                                igdb_id=steam_game.igdb_id,
                                title=game_metadata.title,
                                summary=game_metadata.description,
                                release_date=game_metadata.release_date,
                                cover_art_url=game_metadata.cover_art_url,
                                igdb_rating=game_metadata.rating_average,
                                igdb_rating_count=game_metadata.rating_count,
                                time_to_beat_hastily=game_metadata.hastily,
                                time_to_beat_normally=game_metadata.normally,
                                time_to_beat_completely=game_metadata.completely,
                                igdb_platforms=game_metadata.igdb_platform_ids,
                                igdb_platform_names=game_metadata.platform_names,
                                created_at=datetime.now(timezone.utc),
                                updated_at=datetime.now(timezone.utc)
                            )
                            self.session.add(game)
                            self.session.flush()  # Get the game ID
                            logger.debug(f"Created Game record {game.id} from IGDB ID {steam_game.igdb_id}")
                            
                        except Exception as e:
                            logger.error(f"Error creating Game from IGDB ID {steam_game.igdb_id}: {str(e)}")
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
                    user_game = self.session.exec(user_game_query).first()
                    
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
                        self.session.add(user_game)
                        self.session.flush()  # Get the user_game ID
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
                    existing_association = self.session.exec(platform_association_query).first()
                    
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
                        self.session.add(platform_association)
                        logger.debug(f"Added Steam platform association for UserGame {user_game.id}")
                    
                    # Update Steam game sync tracking
                    steam_game.game_id = game.id
                    steam_game.updated_at = datetime.now(timezone.utc)
                    self.session.add(steam_game)
                    
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
            self.session.commit()
            
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
    
    def toggle_steam_game_ignored(
        self, 
        steam_game_id: str, 
        user_id: str
    ) -> Tuple[SteamGame, str, bool]:
        """
        Toggle the ignored status of a Steam game.
        
        Args:
            steam_game_id: Steam game ID to toggle
            user_id: User ID for authorization
            
        Returns:
            Tuple of (updated SteamGame, success message, new ignored status)
            
        Raises:
            SteamGamesServiceError: For service errors
        """
        logger.info(f"Toggling ignored status for Steam game {steam_game_id} for user {user_id}")
        
        try:
            # Find the Steam game and verify ownership
            steam_game_query = select(SteamGame).where(
                and_(
                    SteamGame.id == steam_game_id,
                    SteamGame.user_id == user_id
                )
            )
            steam_game = self.session.exec(steam_game_query).first()
            
            if not steam_game:
                raise SteamGamesServiceError("Steam game not found or access denied")
            
            # Toggle the ignored status
            old_ignored = steam_game.ignored
            steam_game.ignored = not old_ignored
            steam_game.updated_at = datetime.now(timezone.utc)
            
            self.session.add(steam_game)
            self.session.commit()
            self.session.refresh(steam_game)
            
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


# Factory function for creating service instances
def create_steam_games_service(session: Session) -> SteamGamesService:
    """Factory function to create a SteamGamesService instance."""
    return SteamGamesService(session)
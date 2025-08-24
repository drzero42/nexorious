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
from ..api.games import import_from_igdb
from ..api.schemas.game import GameMetadataAcceptRequest


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


@dataclass
class BulkUnignoreResults:
    """Results of bulk Steam game unignore operation."""
    total_processed: int
    successful_unignores: int
    failed_unignores: int
    errors: List[str]


@dataclass  
class BulkUnmatchResults:
    """Results of bulk Steam game unmatch operation."""
    total_processed: int
    successful_unmatches: int
    failed_unmatches: int
    unsynced_games: int
    errors: List[str]


@dataclass
class BulkUnsyncResults:
    """Results of bulk Steam game unsync operation."""
    total_processed: int
    successful_unsyncs: int
    failed_unsyncs: int
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
        self.auto_match_confidence_threshold = 0.60  # 60% similarity required (same as manual search)
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
            
            # Automatic IGDB matching for all unmatched games (both new and existing)
            auto_matched_count = 0
            if enable_auto_matching:
                logger.info(f"Starting automatic IGDB matching for unmatched Steam games")
                try:
                    # Find all unmatched Steam games for this user (not just newly imported ones)
                    unmatched_games_query = select(SteamGame).where(
                        and_(
                            SteamGame.user_id == user_id,
                            SteamGame.igdb_id.is_(None),  # No IGDB match yet
                            SteamGame.ignored == False    # Not ignored by user
                        )
                    )
                    unmatched_games = self.session.exec(unmatched_games_query).all()
                    
                    if unmatched_games:
                        logger.info(f"Found {len(unmatched_games)} unmatched Steam games to process (includes both new and existing games)")
                        
                        # Perform automatic matching on all unmatched games
                        match_results = await self._auto_match_steam_games([sg.id for sg in unmatched_games])
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
        logger.info(f"🎯 [Auto-Match] Starting automatic IGDB matching for {len(steam_game_ids)} Steam games (confidence threshold: {self.auto_match_confidence_threshold:.0%})")
        
        results = []
        successful_matches = 0
        failed_matches = 0
        skipped_games = 0
        errors = []
        
        # Process in batches to respect rate limits
        total_batches = (len(steam_game_ids) + self.auto_match_batch_size - 1) // self.auto_match_batch_size
        for i in range(0, len(steam_game_ids), self.auto_match_batch_size):
            batch = steam_game_ids[i:i + self.auto_match_batch_size]
            current_batch = i // self.auto_match_batch_size + 1
            logger.info(f"🎯 [Auto-Match] Processing batch {current_batch}/{total_batches}: {len(batch)} games")
            
            for j, steam_game_id in enumerate(batch):
                try:
                    logger.debug(f"🎯 [Auto-Match] Processing game {i+j+1}/{len(steam_game_ids)} (ID: {steam_game_id})")
                    result = await self._auto_match_single_steam_game(steam_game_id)
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
            if i + self.auto_match_batch_size < len(steam_game_ids):
                logger.debug(f"🎯 [Auto-Match] Waiting 0.5s between batches...")
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
    
    async def _auto_match_single_steam_game(self, steam_game_id: str) -> AutoMatchResult:
        """
        Attempt to automatically match a single Steam game to IGDB.
        
        Args:
            steam_game_id: Steam game ID to match
            
        Returns:
            AutoMatchResult with matching details
        """
        # Get Steam game
        logger.debug(f"🎯 [Single Match] Looking up Steam game with ID: {steam_game_id}")
        steam_game = self.session.get(SteamGame, steam_game_id)
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
            igdb_candidates = await self.igdb_service.search_games(
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
            from app.utils.fuzzy_match import calculate_fuzzy_confidence
            confidence = calculate_fuzzy_confidence(steam_game.game_name, best_match.title)
            logger.debug(f"🎯 [Single Match] Confidence score: {confidence:.1%} (threshold: {self.auto_match_confidence_threshold:.1%})")
            
            # Only auto-match if confidence is above threshold
            if confidence >= self.auto_match_confidence_threshold:
                logger.info(f"✅ [Single Match] Confidence meets threshold ({confidence:.1%} >= {self.auto_match_confidence_threshold:.1%}), proceeding with match...")
                
                # Log game state before update
                logger.info(f"📋 [Single Match] BEFORE UPDATE - Game: '{steam_game.game_name}' | igdb_id: {steam_game.igdb_id} | game_id: {steam_game.game_id} | ignored: {steam_game.ignored}")
                
                # Update Steam game with IGDB match (no Game record creation)
                logger.info(f"💾 [Single Match] Setting IGDB ID for SteamGame {steam_game_id}: '{steam_game.game_name}' -> IGDB ID {best_match.igdb_id}")
                old_igdb_id = steam_game.igdb_id
                steam_game.igdb_id = best_match.igdb_id
                steam_game.igdb_title = best_match.title
                steam_game.updated_at = datetime.now(timezone.utc)
                
                logger.info(f"📋 [Single Match] AFTER ASSIGNMENT - Game: '{steam_game.game_name}' | old_igdb_id: {old_igdb_id} | new_igdb_id: {steam_game.igdb_id}")
                
                try:
                    logger.info(f"💾 [Single Match] Adding to session and committing...")
                    self.session.add(steam_game)
                    self.session.commit()
                    logger.info(f"✅ [Single Match] DATABASE COMMIT SUCCESSFUL - '{steam_game.game_name}' matched to '{best_match.title}' (IGDB ID: {best_match.igdb_id})")
                    
                    # Verify the update in database
                    self.session.refresh(steam_game)
                    logger.info(f"🔍 [Single Match] VERIFICATION - Game after refresh: '{steam_game.game_name}' | igdb_id: {steam_game.igdb_id} | game_id: {steam_game.game_id} | ignored: {steam_game.ignored}")
                    
                except Exception as e:
                    logger.error(f"❌ [Single Match] DATABASE COMMIT FAILED for '{steam_game.game_name}': {str(e)}")
                    logger.error(f"💥 [Single Match] Rolling back transaction...")
                    self.session.rollback()  # Rollback the failed transaction
                    
                    # Log final state after rollback
                    self.session.refresh(steam_game)
                    logger.error(f"🔄 [Single Match] AFTER ROLLBACK - Game: '{steam_game.game_name}' | igdb_id: {steam_game.igdb_id}")
                    raise  # Re-raise to trigger the outer catch block
                
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
                logger.debug(f"🎯 [Single Match] Confidence {confidence:.1%} below threshold {self.auto_match_confidence_threshold:.1%}, skipping '{steam_game.game_name}'")
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
                self.session.rollback()
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
                self.session.rollback()
            except Exception:
                pass  # Ignore rollback errors
            return AutoMatchResult(
                steam_game_id=steam_game_id,
                steam_game_name=steam_game.game_name,
                steam_appid=steam_game.steam_appid,
                matched=False,
                error_message=error_msg
            )
    
    async def retry_auto_matching_for_unmatched_games(self, user_id: str) -> AutoMatchResults:
        """
        Manually retry auto-matching for all unmatched Steam games.
        
        Args:
            user_id: User ID for authorization
            
        Returns:
            AutoMatchResults with detailed matching results
            
        Raises:
            SteamGamesServiceError: For service errors
        """
        logger.info(f"🎯 [Manual Auto-Match] Starting manual auto-matching retry for user {user_id}")
        
        try:
            # Validate user exists
            user = self.session.get(User, user_id)
            if not user:
                raise SteamGamesServiceError(f"User {user_id} not found")
            
            # Find all unmatched Steam games for this user
            logger.info(f"🔍 [Manual Auto-Match] Searching for unmatched games for user {user_id}")
            unmatched_games_query = select(SteamGame).where(
                and_(
                    SteamGame.user_id == user_id,
                    SteamGame.igdb_id.is_(None),  # No IGDB match yet
                    SteamGame.ignored == False    # Not ignored by user
                )
            )
            unmatched_games = self.session.exec(unmatched_games_query).all()
            
            # Debug: Log current state of all games for this user
            all_games_query = select(SteamGame).where(SteamGame.user_id == user_id)
            all_games = self.session.exec(all_games_query).all()
            logger.info(f"📊 [Manual Auto-Match] All games state for user {user_id}:")
            for game in all_games:
                logger.info(f"  - {game.game_name} | igdb_id: {game.igdb_id} | game_id: {game.game_id} | ignored: {game.ignored}")
            
            logger.info(f"📊 [Manual Auto-Match] Game counts for user {user_id}:")
            logger.info(f"  - Total games: {len(all_games)}")
            logger.info(f"  - Unmatched (no igdb_id, not ignored): {len([g for g in all_games if not g.igdb_id and not g.ignored])}")
            logger.info(f"  - Matched (has igdb_id, no game_id, not ignored): {len([g for g in all_games if g.igdb_id and not g.game_id and not g.ignored])}")
            logger.info(f"  - Synced (has game_id): {len([g for g in all_games if g.game_id])}")
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
            logger.info(f"🎮 [Manual Auto-Match] Unmatched games:")
            for game in unmatched_games:
                logger.info(f"  - {game.game_name} (Steam ID: {game.steam_appid})")
            
            # Perform automatic matching on all unmatched games
            logger.info(f"🚀 [Manual Auto-Match] Starting automatic matching process...")
            match_results = await self._auto_match_steam_games([sg.id for sg in unmatched_games])
            
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
    
    async def auto_match_single_steam_game(self, steam_game_id: str, user_id: str) -> AutoMatchResult:
        """
        Attempt to automatically match a single Steam game to IGDB.
        
        Args:
            steam_game_id: Steam game ID to auto-match
            user_id: User ID for authorization
            
        Returns:
            AutoMatchResult with matching details
            
        Raises:
            SteamGamesServiceError: For service errors or validation failures
        """
        logger.info(f"🎯 [Single Auto-Match] Starting auto-match for Steam game {steam_game_id} for user {user_id}")
        
        try:
            # Validate user exists
            user = self.session.get(User, user_id)
            if not user:
                raise SteamGamesServiceError(f"User {user_id} not found")
            
            # Validate Steam game exists and belongs to user
            steam_game_query = select(SteamGame).where(
                and_(
                    SteamGame.id == steam_game_id,
                    SteamGame.user_id == user_id
                )
            )
            steam_game = self.session.exec(steam_game_query).first()
            
            if not steam_game:
                logger.error(f"🎯 [Single Auto-Match] Steam game not found: {steam_game_id} for user {user_id}")
                raise SteamGamesServiceError("Steam game not found or access denied")
            
            # Perform single auto-match
            result = await self._auto_match_single_steam_game(steam_game_id)
            
            if result.matched:
                logger.info(f"🎯 [Single Auto-Match] Successfully matched '{result.steam_game_name}' to IGDB (confidence: {result.confidence_score:.1%})")
            elif result.error_message:
                logger.info(f"🎯 [Single Auto-Match] Failed to match '{result.steam_game_name}': {result.error_message}")
            else:
                logger.info(f"🎯 [Single Auto-Match] No suitable match found for '{result.steam_game_name}' (low confidence)")
            
            return result
            
        except SteamGamesServiceError:
            # Re-raise service errors without wrapping
            raise
        except Exception as e:
            logger.error(f"Error during single auto-match for Steam game {steam_game_id} for user {user_id}: {str(e)}")
            raise SteamGamesServiceError(f"Failed to auto-match Steam game: {str(e)}")
    
    async def _remove_steam_platform_association(self, user_game_id: str) -> bool:
        """
        Remove Steam platform association from a UserGame.
        
        Args:
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
            steam_platform = self.session.exec(steam_platform_query).first()
            logger.debug(f"🔍 [Platform Delete] Found Steam platform: {steam_platform.id if steam_platform else 'None'} (name: pc-windows)")
            
            steam_storefront_query = select(Storefront).where(Storefront.name == "steam")
            steam_storefront = self.session.exec(steam_storefront_query).first()
            logger.debug(f"🔍 [Platform Delete] Found Steam storefront: {steam_storefront.id if steam_storefront else 'None'} (name: steam)")
            
            if not steam_platform or not steam_storefront:
                logger.error(f"🔍 [Platform Delete] Steam configuration missing - platform: {steam_platform is not None}, storefront: {steam_storefront is not None}")
                raise SteamGamesServiceError("Steam platform configuration not found")
            
            # Debug: Query ALL platform associations for this UserGame first
            all_associations_query = select(UserGamePlatform).where(UserGamePlatform.user_game_id == user_game_id)
            all_associations = self.session.exec(all_associations_query).all()
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
            steam_association = self.session.exec(steam_association_query).first()
            
            if steam_association:
                logger.info(f"✅ [Platform Delete] Found Steam platform association for UserGame {user_game_id}, deleting...")
                logger.debug(f"🔍 [Platform Delete] Deleting association: UserGamePlatform ID {steam_association.id}")
                self.session.delete(steam_association)
                logger.info(f"✅ [Platform Delete] Steam platform association deleted from session (will commit with transaction)")
                return True
            else:
                logger.warning(f"❌ [Platform Delete] Steam platform association NOT FOUND for UserGame {user_game_id}")
                logger.debug(f"🔍 [Platform Delete] Query was looking for: user_game_id={user_game_id}, platform_id={steam_platform.id}, storefront_id={steam_storefront.id}")
                return False
                
        except Exception as e:
            logger.error(f"💥 [Platform Delete] Error removing Steam platform association for UserGame {user_game_id}: {str(e)}")
            raise SteamGamesServiceError(f"Failed to remove Steam platform association: {str(e)}")
    
    async def _unsync_steam_game_from_collection(self, game_id: str, user_id: str) -> str:
        """
        Remove Steam game from user's collection with multi-platform protection.
        
        Args:
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
            user_game = self.session.exec(user_game_query).first()
            
            if not user_game:
                logger.warning(f"❌ [Unsync] UserGame not found for game_id={game_id}, user_id={user_id}")
                return "not_found"
            
            logger.debug(f"✅ [Unsync] Found UserGame {user_game.id} for game_id={game_id}")
            
            # Check current platform associations BEFORE removal
            before_platforms_query = select(UserGamePlatform).where(
                UserGamePlatform.user_game_id == user_game.id
            )
            before_platforms = self.session.exec(before_platforms_query).all()
            logger.debug(f"🔍 [Unsync] UserGame {user_game.id} has {len(before_platforms)} platform associations BEFORE Steam removal:")
            for assoc in before_platforms:
                platform_name = assoc.platform.name if assoc.platform else "Unknown"
                storefront_name = assoc.storefront.name if assoc.storefront else "None"
                logger.debug(f"  - Platform: {platform_name}, Storefront: {storefront_name}")
            
            # Remove Steam platform association
            logger.debug(f"🔍 [Unsync] Attempting to remove Steam platform association...")
            steam_removed = await self._remove_steam_platform_association(user_game.id)
            
            if not steam_removed:
                logger.warning(f"❌ [Unsync] Steam platform association not found for UserGame {user_game.id}")
            else:
                logger.info(f"✅ [Unsync] Steam platform association removed for UserGame {user_game.id}")
            
            # Check for remaining platform associations AFTER removal
            remaining_platforms_query = select(UserGamePlatform).where(
                UserGamePlatform.user_game_id == user_game.id
            )
            remaining_platforms = self.session.exec(remaining_platforms_query).all()
            logger.debug(f"🔍 [Unsync] UserGame {user_game.id} has {len(remaining_platforms)} platform associations AFTER Steam removal:")
            for assoc in remaining_platforms:
                platform_name = assoc.platform.name if assoc.platform else "Unknown"
                storefront_name = assoc.storefront.name if assoc.storefront else "None"
                logger.debug(f"  - Platform: {platform_name}, Storefront: {storefront_name}")
            
            if len(remaining_platforms) == 0:
                # No other platforms - safe to remove entire UserGame
                logger.info(f"✅ [Unsync] No other platforms remain, removing entire UserGame {user_game.id}")
                self.session.delete(user_game)  # Cascades to UserGameTag automatically
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
        logger.info(f"🔄 [Transaction] Starting Steam game match operation: {steam_game_id} -> IGDB ID {igdb_id} for user {user_id}")
        
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
            
            # Validate IGDB ID and fetch title if provided (skip validation for clearing matches)
            igdb_title = None
            if igdb_id is not None:
                try:
                    # Fetch IGDB game data to get the title and validate the ID
                    game_data = await self.igdb_service.get_game_by_id(igdb_id)
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
            old_game_id = steam_game.game_id
            
            # Update the Steam game's IGDB ID and title
            steam_game.igdb_id = igdb_id
            steam_game.igdb_title = igdb_title  # Set to None when clearing match
            
            # If unmatching (igdb_id=None), also clear sync status
            if igdb_id is None:
                steam_game.game_id = None
            
            steam_game.updated_at = datetime.now(timezone.utc)
            
            # Add to session but DO NOT commit yet - need to do unsync operations first
            self.session.add(steam_game)
            logger.debug(f"🔄 [Transaction] Updated SteamGame in session (not committed yet)")
            
            # Handle collection unsync for unmatch operations
            unsync_result = None
            if igdb_id is None and old_game_id:
                logger.info(f"🔄 [Transaction] Steam game was synced (game_id: {old_game_id}), performing unsync operation")
                unsync_result = await self._unsync_steam_game_from_collection(old_game_id, user_id)
                logger.debug(f"🔄 [Transaction] Unsync operation result: {unsync_result}")
            
            # Create response message
            if igdb_id is None:
                if old_igdb_id and old_game_id:
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
            logger.debug(f"🔄 [Transaction] Committing all changes to database")
            self.session.commit()
            self.session.refresh(steam_game)
            logger.info(f"✅ [Transaction] Successfully committed Steam game {steam_game_id} IGDB match update: {old_igdb_id} -> {igdb_id} by user {user_id}")
            
            return steam_game, message
            
        except SteamGamesServiceError:
            # Re-raise service errors without wrapping but rollback first
            logger.error(f"🔄 [Transaction] SteamGamesServiceError occurred, rolling back transaction")
            self.session.rollback()
            raise
        except Exception as e:
            # Rollback on any unexpected errors
            logger.error(f"🔄 [Transaction] Unexpected error occurred, rolling back transaction: {str(e)}")
            self.session.rollback()
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
                # Create Game record using import_from_igdb
                logger.info(f"🎮 [Steam Service] Step 3a: Creating Game record from IGDB for igdb_id {steam_game.igdb_id}")
                try:
                    # Get user for import_from_igdb call
                    user = self.session.get(User, user_id)
                    if not user:
                        raise SteamGamesServiceError(f"User {user_id} not found")
                    
                    # Use import_from_igdb to create Game record with proper platform handling
                    import_request = GameMetadataAcceptRequest(igdb_id=steam_game.igdb_id)
                    game_response = await import_from_igdb(import_request, self.session, user, self.igdb_service)
                    
                    # Get the created Game record
                    game = self.session.get(Game, game_response.id)
                    logger.info(f"🎮 [Steam Service] Created Game record {game.id} from IGDB ID {steam_game.igdb_id} via import_from_igdb")
                    
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
                        # Create Game record using import_from_igdb
                        logger.debug(f"Creating Game record from IGDB for igdb_id {steam_game.igdb_id}")
                        try:
                            # Get user for import_from_igdb call
                            user = self.session.get(User, user_id)
                            if not user:
                                error_msg = f"User {user_id} not found during bulk sync"
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
                            
                            # Use import_from_igdb to create Game record with proper platform handling
                            import_request = GameMetadataAcceptRequest(igdb_id=steam_game.igdb_id)
                            game_response = await import_from_igdb(import_request, self.session, user, self.igdb_service)
                            
                            # Get the created Game record
                            game = self.session.get(Game, game_response.id)
                            logger.debug(f"Created Game record {game.id} from IGDB ID {steam_game.igdb_id} via import_from_igdb")
                            
                        except Exception as e:
                            logger.error(f"Error creating Game from IGDB ID {steam_game.igdb_id}: {str(e)}")
                            # Ensure session is rolled back after any error
                            try:
                                self.session.rollback()
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
                    # Ensure session is rolled back after sync errors
                    try:
                        self.session.rollback()
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

    async def unignore_all_steam_games(self, user_id: str) -> BulkUnignoreResults:
        """
        Unignore all ignored Steam games for a user.
        
        Args:
            user_id: User ID for authorization
            
        Returns:
            BulkUnignoreResults with detailed unignore statistics
            
        Raises:
            SteamGamesServiceError: For service errors
        """
        try:
            # Find all ignored Steam games for the user
            ignored_games = self.session.exec(
                select(SteamGame)
                .where(SteamGame.user_id == user_id)
                .where(SteamGame.ignored == True)
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
                    self.session.add(steam_game)
                    successful_unignores += 1
                    logger.debug(f"Unignored Steam game: {steam_game.game_name} ({steam_game.id})")
                    
                except Exception as e:
                    failed_unignores += 1
                    error_msg = f"Failed to unignore '{steam_game.game_name}': {str(e)}"
                    errors.append(error_msg)
                    logger.error(f"Error unignoring Steam game {steam_game.id}: {str(e)}")
            
            # Commit all changes in a single transaction
            try:
                self.session.commit()
            except Exception as e:
                self.session.rollback()
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

    async def unmatch_all_matched_games(self, user_id: str) -> BulkUnmatchResults:
        """
        Unmatch all matched (but not synced) Steam games for a user.
        
        Finds games with igdb_id but no game_id and clears their igdb_id.
        This only affects games that are matched but haven't been synced yet.
        
        Args:
            user_id: User ID for authorization
            
        Returns:
            BulkUnmatchResults with detailed unmatch statistics
            
        Raises:
            SteamGamesServiceError: For service errors
        """
        try:
            # Find matched but not synced Steam games for the user
            matched_games = self.session.exec(
                select(SteamGame)
                .where(SteamGame.user_id == user_id)
                .where(SteamGame.igdb_id.isnot(None))  # Has IGDB match
                .where(SteamGame.game_id.is_(None))    # NOT synced to collection
                .where(SteamGame.ignored == False)     # Not ignored
            ).all()
            
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
                    
                    # Clear IGDB match only (no game_id to clear since we filtered them out)
                    steam_game.igdb_id = None
                    steam_game.updated_at = datetime.now(timezone.utc)
                    
                    # Add to session (will be committed later)
                    self.session.add(steam_game)
                    
                    successful_unmatches += 1
                    logger.debug(f"Successfully unmatched Steam game: {steam_game.game_name} ({steam_game.id})")
                    
                except Exception as e:
                    failed_unmatches += 1
                    error_msg = f"Failed to unmatch '{steam_game.game_name}': {str(e)}"
                    errors.append(error_msg)
                    logger.error(f"Error unmatching Steam game {steam_game.id}: {str(e)}")
            
            # Commit all changes in a single transaction
            try:
                self.session.commit()
                logger.info(f"Successfully committed bulk unmatch transaction for user {user_id}")
            except Exception as e:
                self.session.rollback()
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

    async def unsync_all_synced_games(self, user_id: str) -> BulkUnsyncResults:
        """
        Unsync all synced Steam games for a user.
        
        Finds games with game_id (synced to collection) and:
        1. Removes Steam platform/storefront from UserGame
        2. If no other platforms, removes the entire UserGame
        3. Clears game_id from SteamGame (but keeps igdb_id)
        
        Args:
            user_id: User ID for authorization
            
        Returns:
            BulkUnsyncResults with detailed unsync statistics
            
        Raises:
            SteamGamesServiceError: For service errors
        """
        try:
            # Find synced Steam games for the user
            synced_games = self.session.exec(
                select(SteamGame)
                .where(SteamGame.user_id == user_id)
                .where(SteamGame.game_id.isnot(None))  # Synced to collection
                .where(SteamGame.ignored == False)     # Not ignored
            ).all()
            
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
                    logger.debug(f"Processing '{steam_game.game_name}' (game_id: {steam_game.game_id})")
                    
                    # Store original game_id for unsync operation
                    old_game_id = steam_game.game_id
                    
                    # First unsync from collection (removes platform association)
                    try:
                        unsync_result = await self._unsync_steam_game_from_collection(old_game_id, user_id)
                        logger.debug(f"Unsync operation result: {unsync_result}")
                    except Exception as unsync_e:
                        error_msg = f"Failed to unsync '{steam_game.game_name}' from collection: {str(unsync_e)}"
                        errors.append(error_msg)
                        logger.error(f"Unsync error for {steam_game.id}: {str(unsync_e)}")
                        failed_unsyncs += 1
                        continue  # Skip to next game
                    
                    # Clear game_id from SteamGame (keeps igdb_id - returns to matched state)
                    steam_game.game_id = None
                    steam_game.updated_at = datetime.now(timezone.utc)
                    
                    # Add to session (will be committed later)
                    self.session.add(steam_game)
                    
                    successful_unsyncs += 1
                    logger.debug(f"Successfully unsynced Steam game: {steam_game.game_name} ({steam_game.id})")
                    
                except Exception as e:
                    failed_unsyncs += 1
                    error_msg = f"Failed to unsync '{steam_game.game_name}': {str(e)}"
                    errors.append(error_msg)
                    logger.error(f"Error unsyncing Steam game {steam_game.id}: {str(e)}")
            
            # Commit all changes in a single transaction
            try:
                self.session.commit()
                logger.info(f"Successfully committed bulk unsync transaction for user {user_id}")
            except Exception as e:
                self.session.rollback()
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

    async def unsync_steam_game_from_collection(self, steam_game_id: str, user_id: str) -> Tuple[SteamGame, str]:
        """
        Unsync a single Steam game from the user's collection.
        
        This removes the Steam platform/storefront from the UserGame and
        clears the game_id from SteamGame (but keeps igdb_id intact).
        
        Args:
            steam_game_id: Steam game ID to unsync
            user_id: User ID for authorization
            
        Returns:
            Tuple of (updated SteamGame, status message)
            
        Raises:
            SteamGamesServiceError: For service errors
        """
        try:
            # Find and validate Steam game
            steam_game = self.session.get(SteamGame, steam_game_id)
            if not steam_game or steam_game.user_id != user_id:
                raise SteamGamesServiceError(f"Steam game {steam_game_id} not found or access denied")
            
            # Check if game is actually synced
            if not steam_game.game_id:
                raise SteamGamesServiceError(f"Steam game '{steam_game.game_name}' is not synced to collection")
            
            logger.info(f"Unsyncing Steam game '{steam_game.game_name}' (game_id: {steam_game.game_id}) for user {user_id}")
            
            # Store original game_id for unsync operation
            old_game_id = steam_game.game_id
            
            # Perform unsync from collection
            unsync_result = await self._unsync_steam_game_from_collection(old_game_id, user_id)
            logger.debug(f"Unsync operation result: {unsync_result}")
            
            # Clear game_id from SteamGame (keeps igdb_id - returns to matched state)
            steam_game.game_id = None
            steam_game.updated_at = datetime.now(timezone.utc)
            
            # Commit changes
            self.session.add(steam_game)
            self.session.commit()
            
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
            self.session.rollback()
            raise
        except Exception as e:
            self.session.rollback()
            logger.error(f"Error unsyncing Steam game {steam_game_id} for user {user_id}: {str(e)}")
            raise SteamGamesServiceError(f"Failed to unsync Steam game: {str(e)}")


# Factory function for creating service instances
def create_steam_games_service(session: Session, igdb_service: Optional[IGDBService] = None) -> SteamGamesService:
    """Factory function to create a SteamGamesService instance."""
    return SteamGamesService(session, igdb_service=igdb_service)
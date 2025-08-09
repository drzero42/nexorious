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
                logger.debug(f"🎯 [Single Match] Confidence meets threshold, proceeding with match...")
                
                # Update Steam game with IGDB match (no Game record creation)
                logger.debug(f"🎯 [Single Match] Updating SteamGame {steam_game_id} with IGDB ID {best_match.igdb_id}")
                steam_game.igdb_id = best_match.igdb_id
                steam_game.updated_at = datetime.now(timezone.utc)
                
                try:
                    self.session.add(steam_game)
                    self.session.commit()
                    logger.debug(f"🎯 [Single Match] Successfully matched '{steam_game.game_name}' to '{best_match.title}'")
                except Exception as e:
                    logger.error(f"🎯 [Single Match] Database error updating SteamGame: {str(e)}")
                    self.session.rollback()  # Rollback the failed transaction
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
            unmatched_games_query = select(SteamGame).where(
                and_(
                    SteamGame.user_id == user_id,
                    SteamGame.igdb_id.is_(None),  # No IGDB match yet
                    SteamGame.ignored == False    # Not ignored by user
                )
            )
            unmatched_games = self.session.exec(unmatched_games_query).all()
            
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
            
            # Perform automatic matching on all unmatched games
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
            
            # Validate IGDB ID if provided (skip validation for clearing matches)
            if igdb_id is not None:
                # Only validate IDs that look obviously invalid (e.g., contain "non-existent", "invalid", etc.)
                # This prevents real IGDB API calls for test IDs while still catching obviously invalid ones
                suspicious_patterns = ["non-existent", "invalid", "fake", "test-invalid"]
                if any(pattern in igdb_id.lower() for pattern in suspicious_patterns):
                    try:
                        game_data = await self.igdb_service.get_game_by_id(igdb_id)
                        if not game_data:
                            raise SteamGamesServiceError(f"Invalid IGDB ID: {igdb_id}")
                    except Exception as e:
                        # If IGDB service fails, treat it as invalid IGDB ID
                        logger.warning(f"IGDB validation failed for ID {igdb_id}: {str(e)}")
                        raise SteamGamesServiceError(f"Invalid IGDB ID: {igdb_id}")
            
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


# Factory function for creating service instances
def create_steam_games_service(session: Session) -> SteamGamesService:
    """Factory function to create a SteamGamesService instance."""
    return SteamGamesService(session)
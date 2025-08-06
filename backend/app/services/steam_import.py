"""
Steam import background processing service for handling Steam library imports.
"""

import asyncio
import json
import logging
from typing import List, Optional, Dict, Any, Tuple
from datetime import datetime, timezone
from sqlmodel import Session, select, and_
from rapidfuzz import fuzz

from ..core.database import get_session
from ..models.steam_import import SteamImportJob, SteamImportGame, SteamImportJobStatus, SteamImportGameStatus
from ..models.game import Game
from ..models.user import User
from ..services.steam import SteamService, SteamGame, SteamAPIError, SteamAuthenticationError
from ..services.igdb import IGDBService, GameMetadata, IGDBError
from ..services.websocket_manager import get_websocket_manager, WebSocketEventType
from ..api.schemas.game import GameMetadataAcceptRequest

logger = logging.getLogger(__name__)


class SteamImportProcessingError(Exception):
    """Exception for Steam import processing errors."""
    pass


class SteamImportService:
    """Service for handling background Steam library import processing."""
    
    def __init__(self, session: Session, igdb_service: IGDBService):
        """Initialize Steam import service."""
        self.session = session
        self.igdb_service = igdb_service
        self.ws_manager = get_websocket_manager()
        logger.info("Steam import service initialized")
    
    async def start_import_job(self, job_id: str, steam_api_key: str, steam_id: str) -> None:
        """
        Start background processing of a Steam import job.
        
        This is the main entry point for background Steam library processing.
        """
        logger.info(f"Starting Steam import job {job_id}")
        
        try:
            # Get the job from database
            job = self.session.get(SteamImportJob, job_id)
            if not job:
                raise SteamImportProcessingError(f"Import job {job_id} not found")
            
            # Update job status to processing
            await self._update_job_status(job, SteamImportJobStatus.PROCESSING)
            await self._emit_status_change(job)
            
            # Initialize Steam service
            steam_service = SteamService(steam_api_key)
            
            # Phase 1: Retrieve Steam library
            logger.info(f"Phase 1: Retrieving Steam library for user {steam_id}")
            logger.debug(f"Starting Steam library retrieval for job {job_id}")
            steam_games = await self._retrieve_steam_library(steam_service, steam_id)
            logger.debug(f"Steam library retrieval completed for job {job_id}: {len(steam_games) if steam_games else 0} games found")
            
            if not steam_games:
                await self._fail_job(job, "No games found in Steam library")
                return
            
            # Update job with total game count
            job.total_games = len(steam_games)
            job.steam_library_data = json.dumps([{
                "appid": game.appid,
                "name": game.name
            } for game in steam_games])
            await self._save_job_changes(job)
            
            # Emit progress update with total game count
            await self._emit_progress_update(job)
            
            # Phase 2: Two-phase matching process
            logger.info(f"Phase 2: Starting two-phase matching for {len(steam_games)} games")
            logger.debug(f"Beginning two-phase matching process for job {job_id}")
            await self._process_steam_games(job, steam_games)
            logger.debug(f"Two-phase matching completed for job {job_id}")
            
            # Phase 3: Determine next status based on results
            logger.debug(f"Determining next job status for job {job_id}")
            await self._determine_next_job_status(job)
            logger.debug(f"Job status determination completed for job {job_id}: {job.status}")
            
            logger.info(f"Steam import job {job_id} processing completed successfully")
            
        except SteamAPIError as e:
            logger.error(f"Steam API error during import job {job_id}: {str(e)}")
            await self._fail_job_by_id(job_id, f"Steam API error: {str(e)}")
        except SteamAuthenticationError as e:
            logger.error(f"Steam authentication error during import job {job_id}: {str(e)}")
            await self._fail_job_by_id(job_id, f"Steam authentication error: {str(e)}")
        except Exception as e:
            logger.error(f"Unexpected error during Steam import job {job_id}: {str(e)}", exc_info=True)
            await self._fail_job_by_id(job_id, f"Unexpected error: {str(e)}")
    
    async def _retrieve_steam_library(self, steam_service: SteamService, steam_id: str) -> List[SteamGame]:
        """Retrieve Steam library games using Steam Web API."""
        try:
            games = await steam_service.get_owned_games(
                steam_id=steam_id,
                include_appinfo=True,
                include_played_free_games=True
            )
            logger.info(f"Retrieved {len(games)} games from Steam library")
            return games
        except Exception as e:
            logger.error(f"Error retrieving Steam library: {str(e)}")
            raise
    
    async def _process_steam_games(self, job: SteamImportJob, steam_games: List[SteamGame]) -> None:
        """Process Steam games through two-phase matching system."""
        processed_count = 0
        
        for steam_game in steam_games:
            try:
                logger.debug(f"Processing Steam game: {steam_game.name} (AppID: {steam_game.appid})")
                
                # Create import game record
                import_game = SteamImportGame(
                    import_job_id=job.id,
                    steam_appid=steam_game.appid,
                    steam_name=steam_game.name,
                    status=SteamImportGameStatus.AWAITING_USER  # Default status
                )
                
                # Two-phase matching
                match_result = await self._perform_two_phase_matching(steam_game)
                
                if match_result:
                    matched_game_id, match_type = match_result
                    import_game.matched_game_id = matched_game_id
                    import_game.status = SteamImportGameStatus.MATCHED
                    import_game.user_decision = json.dumps({
                        "match_type": match_type,
                        "confidence": "high" if match_type == "database" else "medium"
                    })
                    job.matched_games += 1
                    
                    # Emit game matched event
                    await self._emit_game_matched(job, steam_game, matched_game_id, match_type)
                else:
                    # No match found - needs user review
                    import_game.status = SteamImportGameStatus.AWAITING_USER
                    job.awaiting_review_games += 1
                    
                    # Emit game needs review event
                    await self._emit_game_needs_review(job, steam_game)
                
                # Save the import game record
                self.session.add(import_game)
                
                # Update progress
                processed_count += 1
                job.processed_games = processed_count
                
                # Commit changes periodically (every 10 games)
                if processed_count % 10 == 0:
                    await self._save_job_changes(job)
                    await self._emit_progress_update(job)
                    logger.debug(f"Progress update: {processed_count}/{len(steam_games)} games processed")
                
            except Exception as e:
                logger.error(f"Error processing Steam game {steam_game.name}: {str(e)}")
                # Create failed import game record
                failed_import_game = SteamImportGame(
                    import_job_id=job.id,
                    steam_appid=steam_game.appid,
                    steam_name=steam_game.name,
                    status=SteamImportGameStatus.IMPORT_FAILED,
                    error_message=str(e)
                )
                self.session.add(failed_import_game)
                processed_count += 1
                job.processed_games = processed_count
        
        # Final save of all changes
        await self._save_job_changes(job)
        await self._emit_progress_update(job)
        logger.info(f"Completed processing {processed_count} Steam games")
    
    async def _perform_two_phase_matching(self, steam_game: SteamGame) -> Optional[Tuple[str, str]]:
        """
        Perform two-phase matching for a Steam game.
        
        Phase 1: Check existing games in database for matching Steam AppID
        Phase 2: If no database match, search IGDB for exact title matches
        
        Returns:
            Tuple of (game_id, match_type) if match found, None otherwise
        """
        
        # Phase 1: Database lookup by Steam AppID
        db_match = await self._phase1_database_lookup(steam_game.appid)
        if db_match:
            logger.debug(f"Phase 1 match found for {steam_game.name}: {db_match}")
            return db_match, "database"
        
        # Phase 2: IGDB search by title
        igdb_match = await self._phase2_igdb_search(steam_game.name)
        if igdb_match:
            logger.debug(f"Phase 2 match found for {steam_game.name}: {igdb_match}")
            return igdb_match, "igdb"
        
        logger.debug(f"No automatic match found for {steam_game.name}")
        return None
    
    async def _phase1_database_lookup(self, steam_appid: int) -> Optional[str]:
        """Phase 1: Check existing games in database for matching Steam AppID."""
        try:
            statement = select(Game).where(Game.steam_appid == steam_appid)
            result = self.session.exec(statement).first()
            
            if result:
                logger.debug(f"Database match found for Steam AppID {steam_appid}: {result.title}")
                return result.id
                
            return None
            
        except Exception as e:
            logger.error(f"Error in Phase 1 database lookup for AppID {steam_appid}: {str(e)}")
            return None
    
    async def _phase2_igdb_search(self, game_title: str) -> Optional[str]:
        """Phase 2: Search IGDB for exact title matches."""
        try:
            # Search IGDB for the game
            search_results = await self.igdb_service.search_games(
                query=game_title,
                limit=5,
                fuzzy_threshold=0.85  # High threshold for automatic matching
            )
            
            if not search_results:
                return None
            
            # Look for exact or very close matches
            for game_metadata in search_results:
                similarity = fuzz.ratio(game_title.lower(), game_metadata.title.lower())
                
                if similarity >= 85:  # 85% similarity threshold
                    # Check if this game already exists in our database by IGDB ID
                    existing_game = await self._find_existing_game_by_igdb_id(game_metadata.igdb_id)
                    
                    if existing_game:
                        logger.debug(f"Found existing game by IGDB ID {game_metadata.igdb_id}: {existing_game.title}")
                        return existing_game.id
                    else:
                        # This is a potential new game that could be imported automatically
                        # For now, we'll flag it for manual review to be safe
                        logger.debug(f"IGDB match found but not in database: {game_metadata.title}")
                        return None
            
            return None
            
        except IGDBError as e:
            logger.error(f"IGDB error during Phase 2 search for '{game_title}': {str(e)}")
            return None
        except Exception as e:
            logger.error(f"Error in Phase 2 IGDB search for '{game_title}': {str(e)}")
            return None
    
    async def _find_existing_game_by_igdb_id(self, igdb_id: str) -> Optional[Game]:
        """Find existing game in database by IGDB ID."""
        try:
            statement = select(Game).where(Game.igdb_id == igdb_id)
            result = self.session.exec(statement).first()
            return result
        except Exception as e:
            logger.error(f"Error finding game by IGDB ID {igdb_id}: {str(e)}")
            return None
    
    async def _determine_next_job_status(self, job: SteamImportJob) -> None:
        """Determine the next status for the job based on matching results."""
        if job.awaiting_review_games > 0:
            # Has games that need manual review
            await self._update_job_status(job, SteamImportJobStatus.AWAITING_REVIEW)
            await self._emit_status_change(job)
        elif job.matched_games > 0:
            # All games are matched, ready for final import
            await self._update_job_status(job, SteamImportJobStatus.FINALIZING)
            await self._emit_status_change(job)
        else:
            # No games could be processed
            await self._fail_job(job, "No games could be matched or processed")
    
    async def submit_user_decisions(self, job_id: str, decisions: Dict[str, Any]) -> None:
        """
        Submit user decisions for games awaiting manual review.
        
        Args:
            job_id: Import job ID
            decisions: Dictionary mapping steam_appid to user decision
        """
        logger.info(f"Submitting user decisions for job {job_id}")
        
        try:
            job = self.session.get(SteamImportJob, job_id)
            if not job:
                raise SteamImportProcessingError(f"Import job {job_id} not found")
            
            if job.status != SteamImportJobStatus.AWAITING_REVIEW:
                raise SteamImportProcessingError(f"Job {job_id} is not awaiting review")
            
            # Process user decisions
            awaiting_games = self.session.exec(
                select(SteamImportGame).where(
                    and_(
                        SteamImportGame.import_job_id == job_id,
                        SteamImportGame.status == SteamImportGameStatus.AWAITING_USER
                    )
                )
            ).all()
            
            for game in awaiting_games:
                steam_appid_str = str(game.steam_appid)
                
                if steam_appid_str in decisions:
                    decision = decisions[steam_appid_str]
                    game.user_decision = json.dumps(decision)
                    
                    if decision.get("action") == "import":
                        game.status = SteamImportGameStatus.MATCHED
                        if decision.get("igdb_id"):
                            # User selected a specific IGDB match
                            existing_game = await self._find_existing_game_by_igdb_id(decision["igdb_id"])
                            if existing_game:
                                game.matched_game_id = existing_game.id
                        job.matched_games += 1
                        job.awaiting_review_games -= 1
                    elif decision.get("action") == "skip":
                        game.status = SteamImportGameStatus.SKIPPED
                        job.skipped_games += 1
                        job.awaiting_review_games -= 1
                        
                        # Emit game skipped event
                        await self._emit_game_skipped(job, game)
            
            # Save changes and update job status
            await self._save_job_changes(job)
            
            # Move to finalizing if no more games await review
            if job.awaiting_review_games == 0:
                await self._update_job_status(job, SteamImportJobStatus.FINALIZING)
                await self._emit_status_change(job)
            
            logger.info(f"User decisions processed for job {job_id}")
            
        except Exception as e:
            logger.error(f"Error submitting user decisions for job {job_id}: {str(e)}")
            raise
    
    async def confirm_final_import(self, job_id: str) -> None:
        """
        Confirm and execute final import of matched games.
        
        This method handles the actual import of games and platform assignments.
        """
        logger.info(f"Confirming final import for job {job_id}")
        
        try:
            job = self.session.get(SteamImportJob, job_id)
            if not job:
                raise SteamImportProcessingError(f"Import job {job_id} not found")
            
            if job.status != SteamImportJobStatus.FINALIZING:
                raise SteamImportProcessingError(f"Job {job_id} is not ready for final import")
            
            # Get all matched games
            matched_games = self.session.exec(
                select(SteamImportGame).where(
                    and_(
                        SteamImportGame.import_job_id == job_id,
                        SteamImportGame.status == SteamImportGameStatus.MATCHED
                    )
                )
            ).all()
            
            for import_game in matched_games:
                try:
                    if import_game.matched_game_id:
                        # Game already exists - add Steam platform
                        await self._add_steam_platform_to_existing_game(import_game)
                        import_game.status = SteamImportGameStatus.PLATFORM_ADDED
                        job.platform_added_games += 1
                        
                        # Emit platform added event
                        await self._emit_platform_added(job, import_game)
                    else:
                        # Import new game from IGDB
                        success = await self._import_new_game_from_igdb(import_game)
                        if success:
                            import_game.status = SteamImportGameStatus.IMPORTED
                            job.imported_games += 1
                            
                            # Emit game imported event
                            await self._emit_game_imported(job, import_game)
                        else:
                            import_game.status = SteamImportGameStatus.IMPORT_FAILED
                            import_game.error_message = "Failed to import game from IGDB"
                    
                except Exception as e:
                    logger.error(f"Error importing game {import_game.steam_name}: {str(e)}")
                    import_game.status = SteamImportGameStatus.IMPORT_FAILED
                    import_game.error_message = str(e)
            
            # Complete the job
            await self._update_job_status(job, SteamImportJobStatus.COMPLETED)
            job.completed_at = datetime.now(timezone.utc)
            await self._save_job_changes(job)
            
            # Emit completion event
            await self._emit_import_complete(job)
            
            logger.info(f"Final import completed for job {job_id}")
            
        except Exception as e:
            logger.error(f"Error during final import for job {job_id}: {str(e)}")
            await self._fail_job_by_id(job_id, f"Final import error: {str(e)}")
    
    async def _add_steam_platform_to_existing_game(self, import_game: SteamImportGame) -> None:
        """Add Steam platform to an existing game."""
        # This would integrate with the existing platform management system
        # For now, we'll just update the game's Steam AppID if it's not set
        if import_game.matched_game_id:
            game = self.session.get(Game, import_game.matched_game_id)
            if game and not game.steam_appid:
                game.steam_appid = import_game.steam_appid
                game.updated_at = datetime.now(timezone.utc)
                self.session.add(game)
        
        logger.debug(f"Added Steam platform to existing game: {import_game.steam_name}")
    
    async def _import_new_game_from_igdb(self, import_game: SteamImportGame) -> bool:
        """Import a new game from IGDB based on user decision."""
        try:
            user_decision = json.loads(import_game.user_decision or "{}")
            igdb_id = user_decision.get("igdb_id")
            
            if not igdb_id:
                return False
            
            # Get complete game metadata from IGDB
            game_metadata = await self.igdb_service.get_game_by_id(igdb_id)
            if not game_metadata:
                return False
            
            # Use existing import logic (similar to import_from_igdb endpoint)
            game = await self._create_game_from_metadata(game_metadata, import_game.steam_appid)
            if game:
                import_game.matched_game_id = game.id
                return True
            
            return False
            
        except Exception as e:
            logger.error(f"Error importing new game from IGDB: {str(e)}")
            return False
    
    async def _create_game_from_metadata(self, metadata: GameMetadata, steam_appid: int) -> Optional[Game]:
        """Create a new game from IGDB metadata."""
        try:
            # This would use the existing game creation logic
            # For now, simplified implementation
            game = Game(
                title=metadata.title,
                description=metadata.description,
                genre=metadata.genre,
                developer=metadata.developer,
                publisher=metadata.publisher,
                igdb_id=metadata.igdb_id,
                igdb_slug=metadata.igdb_slug,
                steam_appid=steam_appid
            )
            
            self.session.add(game)
            self.session.commit()
            self.session.refresh(game)
            
            logger.debug(f"Created new game from IGDB: {game.title}")
            return game
            
        except Exception as e:
            logger.error(f"Error creating game from metadata: {str(e)}")
            return None
    
    async def cancel_import_job(self, job_id: str) -> None:
        """Cancel an import job."""
        logger.info(f"Cancelling import job {job_id}")
        
        try:
            job = self.session.get(SteamImportJob, job_id)
            if not job:
                raise SteamImportProcessingError(f"Import job {job_id} not found")
            
            if job.status in [SteamImportJobStatus.COMPLETED, SteamImportJobStatus.FAILED]:
                raise SteamImportProcessingError(f"Cannot cancel job {job_id} with status {job.status}")
            
            await self._update_job_status(job, SteamImportJobStatus.FAILED)
            job.error_message = "Job cancelled by user"
            await self._save_job_changes(job)
            
            logger.info(f"Import job {job_id} cancelled successfully")
            
        except Exception as e:
            logger.error(f"Error cancelling job {job_id}: {str(e)}")
            raise
    
    # Helper methods for job management
    
    async def _update_job_status(self, job: SteamImportJob, status: SteamImportJobStatus) -> None:
        """Update job status and timestamp."""
        job.status = status
        job.updated_at = datetime.now(timezone.utc)
        logger.debug(f"Updated job {job.id} status to {status}")
    
    async def _save_job_changes(self, job: SteamImportJob) -> None:
        """Save job changes to database."""
        try:
            self.session.add(job)
            self.session.commit()
            self.session.refresh(job)
        except Exception as e:
            logger.error(f"Error saving job changes: {str(e)}")
            self.session.rollback()
            raise
    
    async def _fail_job(self, job: SteamImportJob, error_message: str) -> None:
        """Mark job as failed with error message."""
        job.status = SteamImportJobStatus.FAILED
        job.error_message = error_message
        job.updated_at = datetime.now(timezone.utc)
        await self._save_job_changes(job)
        
        # Emit error event
        await self._emit_import_error(job, error_message)
        logger.error(f"Job {job.id} failed: {error_message}")
    
    async def _fail_job_by_id(self, job_id: str, error_message: str) -> None:
        """Mark job as failed by ID."""
        try:
            job = self.session.get(SteamImportJob, job_id)
            if job:
                await self._fail_job(job, error_message)
        except Exception as e:
            logger.error(f"Error failing job {job_id}: {str(e)}")
    
    # WebSocket event emission methods
    
    async def _emit_status_change(self, job: SteamImportJob) -> None:
        """Emit status change event via WebSocket."""
        logger.debug(f"Emitting status change event for job {job.id}: {job.status}")
        try:
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.IMPORT_STATUS_CHANGE,
                data={
                    "status": job.status.value,
                    "total_games": job.total_games,
                    "processed_games": job.processed_games,
                    "matched_games": job.matched_games,
                    "awaiting_review_games": job.awaiting_review_games,
                    "skipped_games": job.skipped_games,
                    "imported_games": job.imported_games,
                    "platform_added_games": job.platform_added_games,
                    "error_message": job.error_message
                }
            )
            logger.debug(f"Status change event sent successfully for job {job.id}")
        except Exception as e:
            logger.error(f"Error emitting status change event for job {job.id}: {str(e)}")
    
    async def _emit_progress_update(self, job: SteamImportJob) -> None:
        """Emit progress update event via WebSocket."""
        logger.debug(f"Emitting progress update for job {job.id}: {job.processed_games}/{job.total_games}")
        try:
            progress_percentage = 0
            if job.total_games > 0:
                progress_percentage = (job.processed_games / job.total_games) * 100
            
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.IMPORT_PROGRESS,
                data={
                    "total_games": job.total_games,
                    "processed_games": job.processed_games,
                    "matched_games": job.matched_games,
                    "awaiting_review_games": job.awaiting_review_games,
                    "skipped_games": job.skipped_games,
                    "imported_games": job.imported_games,
                    "platform_added_games": job.platform_added_games,
                    "progress_percentage": round(progress_percentage, 2)
                }
            )
        except Exception as e:
            logger.error(f"Error emitting progress update event for job {job.id}: {str(e)}")
    
    async def _emit_game_matched(self, job: SteamImportJob, steam_game: SteamGame, matched_game_id: str, match_type: str) -> None:
        """Emit game matched event via WebSocket."""
        try:
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.GAME_MATCHED,
                data={
                    "steam_appid": steam_game.appid,
                    "steam_name": steam_game.name,
                    "matched_game_id": matched_game_id,
                    "match_type": match_type
                }
            )
        except Exception as e:
            logger.error(f"Error emitting game matched event for job {job.id}: {str(e)}")
    
    async def _emit_game_needs_review(self, job: SteamImportJob, steam_game: SteamGame) -> None:
        """Emit game needs review event via WebSocket."""
        try:
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.GAME_NEEDS_REVIEW,
                data={
                    "steam_appid": steam_game.appid,
                    "steam_name": steam_game.name
                }
            )
        except Exception as e:
            logger.error(f"Error emitting game needs review event for job {job.id}: {str(e)}")
    
    async def _emit_game_imported(self, job: SteamImportJob, import_game: SteamImportGame) -> None:
        """Emit game imported event via WebSocket."""
        try:
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.GAME_IMPORTED,
                data={
                    "steam_appid": import_game.steam_appid,
                    "steam_name": import_game.steam_name,
                    "matched_game_id": import_game.matched_game_id
                }
            )
        except Exception as e:
            logger.error(f"Error emitting game imported event for job {job.id}: {str(e)}")
    
    async def _emit_platform_added(self, job: SteamImportJob, import_game: SteamImportGame) -> None:
        """Emit platform added event via WebSocket."""
        try:
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.PLATFORM_ADDED,
                data={
                    "steam_appid": import_game.steam_appid,
                    "steam_name": import_game.steam_name,
                    "matched_game_id": import_game.matched_game_id
                }
            )
        except Exception as e:
            logger.error(f"Error emitting platform added event for job {job.id}: {str(e)}")
    
    async def _emit_game_skipped(self, job: SteamImportJob, import_game: SteamImportGame) -> None:
        """Emit game skipped event via WebSocket."""
        try:
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.GAME_SKIPPED,
                data={
                    "steam_appid": import_game.steam_appid,
                    "steam_name": import_game.steam_name
                }
            )
        except Exception as e:
            logger.error(f"Error emitting game skipped event for job {job.id}: {str(e)}")
    
    async def _emit_import_complete(self, job: SteamImportJob) -> None:
        """Emit import complete event via WebSocket."""
        try:
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.IMPORT_COMPLETE,
                data={
                    "total_games": job.total_games,
                    "processed_games": job.processed_games,
                    "matched_games": job.matched_games,
                    "skipped_games": job.skipped_games,
                    "imported_games": job.imported_games,
                    "platform_added_games": job.platform_added_games,
                    "completed_at": job.completed_at.isoformat() if job.completed_at else None
                }
            )
        except Exception as e:
            logger.error(f"Error emitting import complete event for job {job.id}: {str(e)}")
    
    async def _emit_import_error(self, job: SteamImportJob, error_message: str) -> None:
        """Emit import error event via WebSocket."""
        try:
            await self.ws_manager.send_to_job(
                job_id=job.id,
                event_type=WebSocketEventType.IMPORT_ERROR,
                data={
                    "error_message": error_message,
                    "status": job.status.value,
                    "total_games": job.total_games,
                    "processed_games": job.processed_games
                }
            )
        except Exception as e:
            logger.error(f"Error emitting import error event for job {job.id}: {str(e)}")


def create_steam_import_service(session: Session, igdb_service: IGDBService) -> SteamImportService:
    """Factory function to create a Steam import service instance."""
    return SteamImportService(session, igdb_service)
"""
Darkadia CSV import service implementation using the new import framework.
"""

import logging
import json
import hashlib
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple, Callable
from datetime import datetime, timezone
from decimal import Decimal
from sqlmodel import Session, select, and_, func

from .base import (
    ImportSourceService,
    ImportSourceConfig,
    ImportGame,
    ImportResult,
    SyncResult,
    BulkOperationResult,
    MatchResult
)
from ...models.user import User
from ...models.darkadia_game import DarkadiaGame
from ...models.user_game import UserGame, PlayStatus, OwnershipStatus
from ...models.game import Game
from ...models.darkadia_import import DarkadiaImport
from ...security.file_upload_validator import SecureFileUploadValidator
from ...security.csv_sanitizer import CSVSanitizer
from ...services.igdb import IGDBService
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', '..'))
from scripts.darkadia.parser import DarkadiaCSVParser
from scripts.darkadia.mapper import DarkadiaDataMapper

logger = logging.getLogger(__name__)


class DarkadiaImportService(ImportSourceService):
    """Darkadia CSV import service implementation."""
    
    def __init__(self, session: Session):
        super().__init__("darkadia")
        self.session = session
        self.csv_sanitizer = CSVSanitizer()
        self.file_validator = SecureFileUploadValidator()
        self.csv_parser = DarkadiaCSVParser()
        self.data_mapper = DarkadiaDataMapper()
    
    def _get_user_darkadia_config(self, user: User) -> dict:
        """Get user's Darkadia configuration from preferences."""
        try:
            preferences = user.preferences or {}
            darkadia_config = preferences.get("darkadia", {})
            logger.debug(f"Retrieved Darkadia config for user {user.id}: has_csv_file={bool(darkadia_config.get('csv_file_path'))}")
            return darkadia_config
        except (TypeError, AttributeError) as e:
            logger.error(f"Error parsing preferences for user {user.id}: {str(e)}")
            return {}
    
    def _update_user_darkadia_config(self, user: User, darkadia_config: dict) -> None:
        """Update user's Darkadia configuration in preferences."""
        try:
            preferences = user.preferences or {}
            preferences["darkadia"] = darkadia_config
            user.preferences_json = json.dumps(preferences)
            user.updated_at = datetime.now(timezone.utc)
            
            self.session.add(user)
            self.session.commit()
            self.session.refresh(user)
            logger.debug(f"Darkadia config updated for user {user.id}")
        except Exception as e:
            logger.error(f"Error updating Darkadia config for user {user.id}: {str(e)}")
            self.session.rollback()
            raise
    
    async def get_config(self, user_id: str) -> ImportSourceConfig:
        """Get Darkadia configuration for user."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        darkadia_config = self._get_user_darkadia_config(user)
        csv_file_path = darkadia_config.get("csv_file_path")
        
        return ImportSourceConfig(
            source_name=self.source_name,
            is_configured=bool(csv_file_path),
            is_verified=bool(csv_file_path and Path(csv_file_path).exists()),
            configured_at=datetime.fromisoformat(darkadia_config["configured_at"]) if darkadia_config.get("configured_at") else None,
            last_import=None,  # TODO: Track last import from import jobs
            config_data={
                "has_csv_file": bool(csv_file_path),
                "csv_file_path": csv_file_path,
                "file_exists": bool(csv_file_path and Path(csv_file_path).exists()),
                "file_hash": darkadia_config.get("file_hash")
            }
        )
    
    async def set_config(self, user_id: str, config_data: Dict[str, Any]) -> ImportSourceConfig:
        """Set/update Darkadia configuration."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        csv_file_path = config_data.get("csv_file_path")
        
        if not csv_file_path:
            raise ValueError("CSV file path is required")
        
        # Verify the configuration
        is_valid, error_message, verification_data = await self.verify_config(config_data)
        if not is_valid:
            raise ValueError(error_message or "CSV file verification failed")
        
        # Calculate file hash for change detection
        file_hash = None
        if Path(csv_file_path).exists():
            with open(csv_file_path, 'rb') as f:
                file_hash = hashlib.sha256(f.read()).hexdigest()
        
        # Save configuration
        darkadia_config = {
            "csv_file_path": csv_file_path,
            "file_hash": file_hash,
            "is_verified": True,
            "configured_at": datetime.now(timezone.utc).isoformat()
        }
        
        self._update_user_darkadia_config(user, darkadia_config)
        
        return await self.get_config(user_id)
    
    async def delete_config(self, user_id: str) -> bool:
        """Delete Darkadia configuration."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        preferences = user.preferences or {}
        if "darkadia" in preferences:
            del preferences["darkadia"]
            user.preferences_json = json.dumps(preferences)
            user.updated_at = datetime.now(timezone.utc)
            self.session.add(user)
            self.session.commit()
        
        return True
    
    async def verify_config(self, config_data: Dict[str, Any]) -> Tuple[bool, Optional[str], Optional[Dict[str, Any]]]:
        """Verify CSV file configuration without saving."""
        csv_file_path = config_data.get("csv_file_path")
        
        if not csv_file_path:
            return False, "CSV file path is required", None
        
        file_path = Path(csv_file_path)
        
        # Check if file exists
        if not file_path.exists():
            return False, f"CSV file not found: {csv_file_path}", None
        
        # Check if file is readable
        if not file_path.is_file():
            return False, f"Path is not a file: {csv_file_path}", None
        
        try:
            # Basic file size check
            file_size = file_path.stat().st_size
            if file_size > 10 * 1024 * 1024:  # 10MB limit
                return False, "CSV file too large (max 10MB)", None
            
            # Try to parse a small sample
            import pandas as pd
            try:
                df_sample = pd.read_csv(file_path, nrows=5)
                row_count = len(df_sample)
                column_count = len(df_sample.columns)
            except Exception as e:
                return False, f"Invalid CSV format: {str(e)}", None
            
            verification_data = {
                "file_size": file_size,
                "row_count": row_count,
                "column_count": column_count,
                "encoding": "utf-8"
            }
            
            return True, None, verification_data
            
        except Exception as e:
            logger.error(f"Error verifying CSV file {csv_file_path}: {str(e)}")
            return False, f"CSV file validation failed: {str(e)}", None
    
    async def get_library_preview(self, user_id: str) -> Dict[str, Any]:
        """Get preview of CSV data."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        config = await self.get_config(user_id)
        if not config.is_configured:
            raise ValueError("Darkadia CSV file not configured")
        
        csv_file_path = config.config_data.get("csv_file_path")
        if not csv_file_path or not Path(csv_file_path).exists():
            raise ValueError("CSV file not found")
        
        try:
            # Parse full CSV file
            full_data = await self.csv_parser.parse_csv(Path(csv_file_path))
            
            # Use first 50 rows for platform analysis
            analysis_data = full_data[:50] if len(full_data) > 50 else full_data
            
            # Analyze platforms from the sample
            platform_analysis = self._analyze_platforms(analysis_data)
            
            # Get actual total count
            total_estimate = len(full_data)
            
            return {
                "total_games_estimate": total_estimate,
                "preview_games": [
                    {
                        "name": game_data.get("Name", ""),
                        "platforms": game_data.get("Platforms", ""),
                        "rating": game_data.get("Rating", ""),
                        "played": game_data.get("Played", False),
                        "finished": game_data.get("Finished", False)
                    }
                    for game_data in analysis_data[:5]
                ],
                "platform_analysis": platform_analysis,
                "file_info": {
                    "path": csv_file_path,
                    "size": Path(csv_file_path).stat().st_size,
                    "modified": datetime.fromtimestamp(Path(csv_file_path).stat().st_mtime).isoformat()
                }
            }
        except Exception as e:
            logger.error(f"Error getting library preview for user {user_id}: {str(e)}")
            raise ValueError(f"Failed to preview CSV file: {str(e)}")
    
    def _analyze_platforms(self, games_data: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Analyze platform data from CSV to detect unknown platforms and resolution status."""
        platform_stats = {}
        unknown_platforms = set()
        unknown_storefronts = set() 
        
        # Reset mapper's tracking sets
        self.data_mapper.unknown_platforms = set()
        self.data_mapper.unknown_storefronts = set()
        
        for game_data in games_data:
            # Extract platform information
            platforms_field = game_data.get("Platforms", "")
            if platforms_field and str(platforms_field).strip() and str(platforms_field) != "nan":
                # Split multiple platforms (assume comma-separated)
                platform_names = [p.strip() for p in str(platforms_field).split(",") if p.strip()]
                
                for platform_name in platform_names:
                    if platform_name not in platform_stats:
                        platform_stats[platform_name] = {
                            "name": platform_name,
                            "games_count": 0,
                            "is_known": platform_name in self.data_mapper.PLATFORM_MAPPINGS,
                            "mapped_name": self.data_mapper.PLATFORM_MAPPINGS.get(platform_name),
                            "suggested_mapping": self.data_mapper._map_platform_name(platform_name)
                        }
                    
                    platform_stats[platform_name]["games_count"] += 1
                    
                    # Track unknown platforms
                    if not platform_stats[platform_name]["is_known"] and not platform_stats[platform_name]["suggested_mapping"]:
                        unknown_platforms.add(platform_name)
            
            # Also check Copy platform field
            copy_platform = game_data.get("Copy platform", "")
            if copy_platform and str(copy_platform).strip() and str(copy_platform) != "nan":
                platform_name = str(copy_platform).strip()
                
                if platform_name not in platform_stats:
                    platform_stats[platform_name] = {
                        "name": platform_name,
                        "games_count": 0,
                        "is_known": platform_name in self.data_mapper.PLATFORM_MAPPINGS,
                        "mapped_name": self.data_mapper.PLATFORM_MAPPINGS.get(platform_name),
                        "suggested_mapping": self.data_mapper._map_platform_name(platform_name)
                    }
                
                platform_stats[platform_name]["games_count"] += 1
                
                if not platform_stats[platform_name]["is_known"] and not platform_stats[platform_name]["suggested_mapping"]:
                    unknown_platforms.add(platform_name)
        
        # Include platforms detected by the mapper
        unknown_platforms.update(self.data_mapper.unknown_platforms)
        unknown_storefronts.update(self.data_mapper.unknown_storefronts)
        
        return {
            "platform_stats": list(platform_stats.values()),
            "unknown_platforms": list(unknown_platforms),
            "unknown_storefronts": list(unknown_storefronts),
            "total_platforms": len(platform_stats),
            "unknown_platform_count": len(unknown_platforms),
            "known_platform_count": len(platform_stats) - len(unknown_platforms)
        }
    
    async def import_library(self, user_id: str, progress_callback: Optional[Callable[[int], None]] = None) -> ImportResult:
        """Import CSV data into staging table."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        config = await self.get_config(user_id)
        if not config.is_configured or not config.is_verified:
            raise ValueError("Darkadia CSV file not configured or verified")
        
        csv_file_path = config.config_data.get("csv_file_path")
        if not csv_file_path:
            raise ValueError("CSV file path not found in configuration")
        
        try:
            # Parse the CSV file
            games_data = await self.csv_parser.parse_csv(Path(csv_file_path))
            
            imported_count = 0
            skipped_count = 0
            errors = []
            
            # Import games in batches with progress tracking
            total_games = len(games_data)
            for i, game_data in enumerate(games_data):
                try:
                    # Update progress every 10 games
                    if progress_callback and i % 10 == 0:
                        progress_percent = int((i / total_games) * 100)
                        progress_callback(progress_percent)
                    
                    # Sanitize all CSV data
                    sanitized_data = {}
                    for key, value in game_data.items():
                        sanitized_data[key] = self.csv_sanitizer.sanitize_cell(value)
                    
                    # Check if game already exists (based on external_id)
                    external_id = str(i + 1)  # Use row number as external ID
                    existing_game = self.session.exec(
                        select(DarkadiaGame).where(
                            and_(
                                DarkadiaGame.user_id == user_id,
                                DarkadiaGame.external_id == external_id
                            )
                        )
                    ).first()
                    
                    if existing_game:
                        skipped_count += 1
                        continue
                    
                    # Create new staging game record
                    darkadia_game = DarkadiaGame(
                        user_id=user_id,
                        external_id=external_id,
                        game_name=sanitized_data.get("Name", "Unknown Game"),
                        ignored=False
                    )
                    
                    # Store all CSV data as JSON
                    darkadia_game.set_csv_data(sanitized_data)
                    
                    self.session.add(darkadia_game)
                    imported_count += 1
                    
                    # Commit in batches of 100
                    if imported_count % 100 == 0:
                        self.session.commit()
                        
                except Exception as e:
                    logger.error(f"Error importing game {i+1}: {str(e)}")
                    errors.append(f"Row {i+1}: {str(e)}")
                    continue
            
            # Final progress update
            if progress_callback:
                progress_callback(100)
            
            # Final commit
            self.session.commit()
            
            return ImportResult(
                imported_count=imported_count,
                skipped_count=skipped_count,
                auto_matched_count=0,  # Matching happens in separate step
                total_games=len(games_data),
                errors=errors
            )
            
        except Exception as e:
            logger.error(f"Error importing library for user {user_id}: {str(e)}")
            self.session.rollback()
            raise ValueError(f"Import failed: {str(e)}")
    
    async def list_games(self, 
                        user_id: str, 
                        offset: int = 0,
                        limit: int = 100,
                        status_filter: Optional[str] = None,
                        search: Optional[str] = None) -> Tuple[List[ImportGame], int]:
        """List imported games with filtering and pagination."""
        query = select(DarkadiaGame).where(DarkadiaGame.user_id == user_id)
        
        # Apply status filter
        if status_filter == "unmatched":
            query = query.where(DarkadiaGame.igdb_id == None)
        elif status_filter == "matched":
            query = query.where(DarkadiaGame.igdb_id != None)
        elif status_filter == "ignored":
            query = query.where(DarkadiaGame.ignored == True)
        elif status_filter == "synced":
            query = query.where(DarkadiaGame.game_id != None)
        
        # Apply search filter
        if search:
            search_term = f"%{search.lower()}%"
            query = query.where(func.lower(DarkadiaGame.game_name).like(search_term))
        
        # Get total count
        total_count = len(self.session.exec(query).all())
        
        # Apply pagination
        games = self.session.exec(query.offset(offset).limit(limit)).all()
        
        # Convert to ImportGame objects
        import_games = []
        for game in games:
            import_games.append(ImportGame(
                id=game.id,
                external_id=game.external_id,
                name=game.game_name,
                igdb_id=game.igdb_id,
                igdb_title=game.igdb_title,
                game_id=game.game_id,
                ignored=game.ignored,
                created_at=game.created_at,
                updated_at=game.updated_at
            ))
        
        return import_games, total_count
    
    async def match_game(self, user_id: str, game_id: str, igdb_id: Optional[str]) -> ImportGame:
        """Match game to IGDB entry."""
        game = self.session.get(DarkadiaGame, game_id)
        if not game or game.user_id != user_id:
            raise ValueError(f"Game {game_id} not found")
        
        if igdb_id:
            # TODO: Validate IGDB ID exists
            game.igdb_id = igdb_id
            game.igdb_title = f"IGDB Game {igdb_id}"  # This should come from IGDB service
        else:
            # Clear match
            game.igdb_id = None
            game.igdb_title = None
        
        game.updated_at = datetime.now(timezone.utc)
        self.session.add(game)
        self.session.commit()
        self.session.refresh(game)
        
        return ImportGame(
            id=game.id,
            external_id=game.external_id,
            name=game.game_name,
            igdb_id=game.igdb_id,
            igdb_title=game.igdb_title,
            game_id=game.game_id,
            ignored=game.ignored,
            created_at=game.created_at,
            updated_at=game.updated_at
        )
    
    async def auto_match_game(self, user_id: str, game_id: str) -> MatchResult:
        """Automatically match single game to IGDB."""
        game = self.session.get(DarkadiaGame, game_id)
        if not game or game.user_id != user_id:
            raise ValueError(f"Game {game_id} not found")
        
        try:
            # For now, implement a simple mock matching
            # TODO: Implement real IGDB integration
            if len(game.game_name) > 3:  # Simple heuristic for demo
                game.igdb_id = f"mock_{hash(game.game_name) % 10000}"
                game.igdb_title = f"{game.game_name} (IGDB)"
                game.updated_at = datetime.now(timezone.utc)
                self.session.add(game)
                self.session.commit()
                
                return MatchResult(
                    game_id=game.id,
                    game_name=game.game_name,
                    matched=True,
                    igdb_id=game.igdb_id,
                    igdb_title=game.igdb_title,
                    confidence_score=0.8
                )
            else:
                return MatchResult(
                    game_id=game.id,
                    game_name=game.game_name,
                    matched=False,
                    error_message="Game name too short for matching"
                )
                
        except Exception as e:
            logger.error(f"Error auto-matching game {game_id}: {str(e)}")
            return MatchResult(
                game_id=game.id,
                game_name=game.game_name,
                matched=False,
                error_message=str(e)
            )
    
    async def auto_match_all_games(self, user_id: str) -> BulkOperationResult:
        """Automatically match all unmatched games to IGDB."""
        # Get all unmatched games
        unmatched_games = self.session.exec(
            select(DarkadiaGame).where(
                and_(
                    DarkadiaGame.user_id == user_id,
                    DarkadiaGame.igdb_id == None,
                    DarkadiaGame.ignored == False
                )
            )
        ).all()
        
        total_processed = len(unmatched_games)
        successful_operations = 0
        failed_operations = 0
        errors = []
        results = []
        
        for game in unmatched_games:
            try:
                result = await self.auto_match_game(user_id, game.id)
                results.append(result)
                if result.matched:
                    successful_operations += 1
                else:
                    failed_operations += 1
                    if result.error_message:
                        errors.append(f"{game.game_name}: {result.error_message}")
            except Exception as e:
                failed_operations += 1
                errors.append(f"{game.game_name}: {str(e)}")
        
        return BulkOperationResult(
            total_processed=total_processed,
            successful_operations=successful_operations,
            failed_operations=failed_operations,
            errors=errors,
            results=results
        )
    
    async def sync_game(self, user_id: str, game_id: str) -> SyncResult:
        """Sync Darkadia game to main collection."""
        logger.info(f"🎮 [Darkadia Service] Syncing Darkadia game {game_id} to collection for user {user_id}")
        
        try:
            # Step 1: Find and validate Darkadia game
            darkadia_game = self.session.get(DarkadiaGame, game_id)
            
            if not darkadia_game or darkadia_game.user_id != user_id:
                logger.error(f"🎮 [Darkadia Service] Darkadia game not found: {game_id} for user {user_id}")
                return SyncResult(
                    steam_game_id=game_id,  # Using this field for darkadia game ID
                    steam_game_name="",
                    user_game_id=None,
                    action="failed",
                    error_message="Darkadia game not found or access denied"
                )
            
            logger.debug(f"🎮 [Darkadia Service] Found Darkadia game: {darkadia_game.game_name}")
            
            # Step 2: Validate game has IGDB match
            if not darkadia_game.igdb_id:
                logger.error(f"🎮 [Darkadia Service] Darkadia game {game_id} not matched to IGDB")
                return SyncResult(
                    steam_game_id=game_id,
                    steam_game_name=darkadia_game.game_name,
                    user_game_id=None,
                    action="failed",
                    error_message="Game must be matched to IGDB before syncing to collection"
                )
            
            logger.debug(f"🎮 [Darkadia Service] Game is matched to IGDB ID: {darkadia_game.igdb_id}")
            
            # Step 3: Check if Game record exists, create if needed
            game_query = select(Game).where(Game.igdb_id == darkadia_game.igdb_id)
            game = self.session.exec(game_query).first()
            
            if not game:
                # Create a basic Game record (for now, we'll create a minimal one)
                # TODO: Integrate with IGDB service to create proper Game record
                logger.info(f"🎮 [Darkadia Service] Creating Game record for IGDB ID {darkadia_game.igdb_id}")
                game = Game(
                    igdb_id=darkadia_game.igdb_id,
                    title=darkadia_game.igdb_title or darkadia_game.game_name,
                    description="",
                    genre="",
                    developer="",
                    publisher="",
                    release_date=None,
                    created_at=datetime.now(timezone.utc),
                    updated_at=datetime.now(timezone.utc)
                )
                self.session.add(game)
                self.session.flush()  # Get the game ID
                logger.info(f"🎮 [Darkadia Service] Created Game record {game.id}")
            else:
                logger.debug(f"🎮 [Darkadia Service] Found existing Game record: {game.title} (ID: {game.id})")
            
            # Step 4: Check if UserGame relationship exists
            user_game_query = select(UserGame).where(
                and_(
                    UserGame.user_id == user_id,
                    UserGame.game_id == game.id
                )
            )
            user_game = self.session.exec(user_game_query).first()
            
            action = "updated_existing"
            if not user_game:
                # Create new UserGame with Darkadia data
                csv_data = darkadia_game.get_csv_data()
                played_flags = darkadia_game.played_flags
                
                # Resolve play status from Darkadia flags
                play_status = self._resolve_play_status(played_flags)
                
                # Parse rating
                rating = None
                try:
                    rating_str = csv_data.get("Rating", "")
                    if rating_str and rating_str.strip():
                        rating = float(rating_str)
                        if rating < 0 or rating > 10:
                            rating = None
                except (ValueError, TypeError):
                    rating = None
                
                # Parse acquired date
                acquired_date = None
                try:
                    date_str = csv_data.get("Added", "")
                    if date_str and date_str.strip():
                        # Try common date formats
                        for fmt in ["%Y-%m-%d", "%m/%d/%Y", "%d/%m/%Y"]:
                            try:
                                acquired_date = datetime.strptime(date_str, fmt).date()
                                break
                            except ValueError:
                                continue
                except (ValueError, TypeError):
                    pass
                
                logger.info(f"🎮 [Darkadia Service] Creating new UserGame relationship")
                user_game = UserGame(
                    user_id=user_id,
                    game_id=game.id,
                    ownership_status=OwnershipStatus.OWNED,
                    play_status=play_status,
                    personal_rating=Decimal(str(rating)) if rating is not None else None,
                    is_loved=csv_data.get("Loved", False),
                    hours_played=0,  # Darkadia doesn't track hours
                    personal_notes=csv_data.get("Notes", ""),
                    acquired_date=acquired_date,
                    created_at=datetime.now(timezone.utc),
                    updated_at=datetime.now(timezone.utc)
                )
                self.session.add(user_game)
                self.session.flush()  # Get the user_game ID
                action = "created_new"
                logger.info(f"🎮 [Darkadia Service] Created new UserGame {user_game.id}")
                
                # Create DarkadiaImport record for extended metadata
                darkadia_import = DarkadiaImport(
                    user_id=user_id,
                    user_game_id=user_game.id,
                    batch_id=f"darkadia_sync_{datetime.now().strftime('%Y%m%d_%H%M%S')}",
                    csv_file_hash="manual_sync",
                    played=played_flags.get("played", False),
                    playing=played_flags.get("playing", False),
                    finished=played_flags.get("finished", False),
                    mastered=played_flags.get("mastered", False),
                    dominated=played_flags.get("dominated", False),
                    shelved=played_flags.get("shelved", False),
                    original_platform_name=csv_data.get("Platforms", ""),
                    platform_resolved=False
                )
                darkadia_import.set_original_csv_data(csv_data)
                
                # Extract physical copy metadata
                copy_metadata = {}
                for field in ["Copy label", "Copy Release", "Copy platform", "Copy media", 
                             "Copy source", "Copy box", "Copy manual", "Copy complete"]:
                    if field in csv_data and csv_data[field]:
                        copy_metadata[field] = csv_data[field]
                
                if copy_metadata:
                    darkadia_import.set_physical_copy_data(copy_metadata)
                
                self.session.add(darkadia_import)
                logger.info(f"🎮 [Darkadia Service] Created DarkadiaImport record for extended metadata")
            else:
                logger.debug(f"🎮 [Darkadia Service] Found existing UserGame relationship: {user_game.id}")
            
            # Step 5: Update the darkadia_game to reference the game
            darkadia_game.game_id = game.id
            darkadia_game.updated_at = datetime.now(timezone.utc)
            self.session.add(darkadia_game)
            
            # Final commit
            self.session.commit()
            
            return SyncResult(
                steam_game_id=game_id,
                steam_game_name=darkadia_game.game_name,
                user_game_id=user_game.id,
                action=action,
                error_message=None
            )
            
        except Exception as e:
            logger.error(f"🎮 [Darkadia Service] Error syncing game {game_id}: {str(e)}")
            self.session.rollback()
            return SyncResult(
                steam_game_id=game_id,
                steam_game_name="",
                user_game_id=None,
                action="failed",
                error_message=str(e)
            )
    
    def _resolve_play_status(self, played_flags: Dict[str, bool]) -> PlayStatus:
        """Resolve play status from Darkadia boolean flags using weighted decision matrix."""
        # Priority order (highest to lowest)
        if played_flags.get("dominated", False):
            return PlayStatus.DOMINATED
        elif played_flags.get("mastered", False):
            return PlayStatus.MASTERED
        elif played_flags.get("finished", False):
            return PlayStatus.COMPLETED
        elif played_flags.get("shelved", False):
            return PlayStatus.SHELVED
        elif played_flags.get("playing", False):
            return PlayStatus.IN_PROGRESS
        elif played_flags.get("played", False):
            return PlayStatus.COMPLETED
        else:
            return PlayStatus.NOT_STARTED
    
    async def sync_all_games(self, user_id: str) -> BulkOperationResult:
        """Sync all matched games to main collection."""
        # Get all matched but unsynced games
        matched_games = self.session.exec(
            select(DarkadiaGame).where(
                and_(
                    DarkadiaGame.user_id == user_id,
                    DarkadiaGame.igdb_id != None,
                    DarkadiaGame.game_id == None,  # Not yet synced
                    DarkadiaGame.ignored == False
                )
            )
        ).all()
        
        total_processed = len(matched_games)
        successful_operations = 0
        failed_operations = 0
        errors = []
        results = []
        
        for game in matched_games:
            try:
                result = await self.sync_game(user_id, game.id)
                results.append(result)
                if result.action in ["created_new", "updated_existing"]:
                    successful_operations += 1
                else:
                    failed_operations += 1
                    if result.error_message:
                        errors.append(f"{game.game_name}: {result.error_message}")
            except Exception as e:
                failed_operations += 1
                errors.append(f"{game.game_name}: {str(e)}")
        
        return BulkOperationResult(
            total_processed=total_processed,
            successful_operations=successful_operations,
            failed_operations=failed_operations,
            errors=errors,
            results=results
        )
    
    async def unsync_game(self, user_id: str, game_id: str) -> ImportGame:
        """Remove game from main collection but keep import record."""
        darkadia_game = self.session.get(DarkadiaGame, game_id)
        if not darkadia_game or darkadia_game.user_id != user_id:
            raise ValueError(f"Game {game_id} not found")
        
        if not darkadia_game.game_id:
            # Game not synced, nothing to do
            return self._convert_to_import_game(darkadia_game)
        
        try:
            # Find and remove UserGame record
            user_game = self.session.exec(
                select(UserGame).where(
                    and_(
                        UserGame.user_id == user_id,
                        UserGame.game_id == darkadia_game.game_id
                    )
                )
            ).first()
            
            if user_game:
                # Remove associated DarkadiaImport records
                darkadia_imports = self.session.exec(
                    select(DarkadiaImport).where(
                        DarkadiaImport.user_game_id == user_game.id
                    )
                ).all()
                
                for darkadia_import in darkadia_imports:
                    self.session.delete(darkadia_import)
                
                # Remove UserGame and its platform associations (cascade should handle this)
                self.session.delete(user_game)
                logger.info(f"🎮 [Darkadia Service] Removed UserGame {user_game.id} for game {darkadia_game.game_name}")
            
            # Clear game_id reference in staging record
            darkadia_game.game_id = None
            darkadia_game.updated_at = datetime.now(timezone.utc)
            self.session.add(darkadia_game)
            
            self.session.commit()
            
            return self._convert_to_import_game(darkadia_game)
            
        except Exception as e:
            logger.error(f"🎮 [Darkadia Service] Error unsyncing game {game_id}: {str(e)}")
            self.session.rollback()
            raise ValueError(f"Failed to unsync game: {str(e)}")
    
    async def unsync_all_games(self, user_id: str) -> BulkOperationResult:
        """Remove all synced games from main collection."""
        synced_games = self.session.exec(
            select(DarkadiaGame).where(
                and_(
                    DarkadiaGame.user_id == user_id,
                    DarkadiaGame.game_id != None
                )
            )
        ).all()
        
        total_processed = len(synced_games)
        successful_operations = 0
        failed_operations = 0
        errors = []
        results = []
        
        for game in synced_games:
            try:
                result = await self.unsync_game(user_id, game.id)
                results.append(result)
                successful_operations += 1
            except Exception as e:
                failed_operations += 1
                errors.append(f"{game.game_name}: {str(e)}")
        
        return BulkOperationResult(
            total_processed=total_processed,
            successful_operations=successful_operations,
            failed_operations=failed_operations,
            errors=errors,
            results=results
        )
    
    def _convert_to_import_game(self, darkadia_game: DarkadiaGame) -> ImportGame:
        """Convert DarkadiaGame to ImportGame."""
        return ImportGame(
            id=darkadia_game.id,
            external_id=darkadia_game.external_id,
            name=darkadia_game.game_name,
            igdb_id=darkadia_game.igdb_id,
            igdb_title=darkadia_game.igdb_title,
            game_id=darkadia_game.game_id,
            ignored=darkadia_game.ignored,
            created_at=darkadia_game.created_at,
            updated_at=darkadia_game.updated_at
        )
    
    async def ignore_game(self, user_id: str, game_id: str) -> ImportGame:
        """Toggle ignore status of game."""
        game = self.session.get(DarkadiaGame, game_id)
        if not game or game.user_id != user_id:
            raise ValueError(f"Game {game_id} not found")
        
        game.ignored = not game.ignored
        game.updated_at = datetime.now(timezone.utc)
        self.session.add(game)
        self.session.commit()
        self.session.refresh(game)
        
        return ImportGame(
            id=game.id,
            external_id=game.external_id,
            name=game.game_name,
            igdb_id=game.igdb_id,
            igdb_title=game.igdb_title,
            game_id=game.game_id,
            ignored=game.ignored,
            created_at=game.created_at,
            updated_at=game.updated_at
        )
    
    async def unignore_all_games(self, user_id: str) -> BulkOperationResult:
        """Unignore all ignored games."""
        ignored_games = self.session.exec(
            select(DarkadiaGame).where(
                and_(
                    DarkadiaGame.user_id == user_id,
                    DarkadiaGame.ignored == True
                )
            )
        ).all()
        
        successful_operations = 0
        for game in ignored_games:
            game.ignored = False
            game.updated_at = datetime.now(timezone.utc)
            self.session.add(game)
            successful_operations += 1
        
        self.session.commit()
        
        return BulkOperationResult(
            total_processed=len(ignored_games),
            successful_operations=successful_operations,
            failed_operations=0,
            errors=[]
        )
    
    async def unmatch_all_games(self, user_id: str) -> BulkOperationResult:
        """Remove IGDB matches from all matched games."""
        matched_games = self.session.exec(
            select(DarkadiaGame).where(
                and_(
                    DarkadiaGame.user_id == user_id,
                    DarkadiaGame.igdb_id != None
                )
            )
        ).all()
        
        successful_operations = 0
        for game in matched_games:
            game.igdb_id = None
            game.igdb_title = None
            game.updated_at = datetime.now(timezone.utc)
            self.session.add(game)
            successful_operations += 1
        
        self.session.commit()
        
        return BulkOperationResult(
            total_processed=len(matched_games),
            successful_operations=successful_operations,
            failed_operations=0,
            errors=[]
        )


def create_darkadia_import_service(session: Session) -> DarkadiaImportService:
    """Factory function to create Darkadia import service."""
    return DarkadiaImportService(session)
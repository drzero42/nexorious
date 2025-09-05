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
from ...models.user_game import UserGame, PlayStatus, OwnershipStatus, UserGamePlatform
from ...models.game import Game
from ...models.darkadia_import import DarkadiaImport
from ...models.platform import Platform, Storefront
from ...security.file_upload_validator import SecureFileUploadValidator
from ...security.csv_sanitizer import CSVSanitizer
from ...services.platform_resolution import create_platform_resolution_service
from .darkadia_transformer import DarkadiaTransformationPipeline
from .copy_consolidation import CopyConsolidationProcessor, ConsolidatedGame
from ...utils.json_serialization import safe_json_dumps, log_serialization_debug, enhanced_safe_json_dumps
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
        self.transformer = DarkadiaTransformationPipeline()
        self.consolidation_processor = CopyConsolidationProcessor()
        self.platform_resolution_service = create_platform_resolution_service(session)
    
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
    
    def _normalize_platform_name(self, platform_name: str) -> str:
        """Normalize platform name from CSV to database format."""
        # Platform mapping from CSV values to database platform names
        platform_mappings = {
            # Direct CSV platform names
            'PC': 'pc-windows',
            'Mac': 'mac',
            'Linux': 'pc-linux',
            'Android': 'android',
            'PlayStation 4': 'playstation-4',
            'PlayStation 5': 'playstation-5',
            'PlayStation 3': 'playstation-3',
            'PlayStation 2': 'playstation-2',
            'PlayStation Network (PS3)': 'playstation-3',  # Map PS3 network to PS3
            'PlayStation Network (Vita)': 'playstation-vita',  # May need to add this platform
            'Nintendo Switch': 'nintendo-switch',
            'Xbox 360': 'xbox-360',
            'Xbox 360 Games Store': 'xbox-360',  # Map Xbox 360 store to Xbox 360 platform
            'Xbox One': 'xbox-one',
            'Wii': 'nintendo-wii',
            'iOS': 'ios',
            
            # Also handle mapper output format if it still gets called
            'PC (Windows)': 'pc-windows',
            'PC (Linux)': 'pc-linux',
            'Xbox Series X/S': 'xbox-series',
            'Nintendo Wii': 'nintendo-wii',
        }
        return platform_mappings.get(platform_name, platform_name.lower().replace(' ', '-'))
    
    def _normalize_storefront_name(self, storefront_name: str) -> Optional[str]:
        """Normalize storefront name from CSV to database format."""
        # Storefront mapping from CSV values to database storefront names
        storefront_mappings = {
            # Direct CSV storefront names
            'Steam': 'steam',
            'Epic': 'epic-games-store',
            'Epic Game Store': 'epic-games-store',
            'Epic Games Store': 'epic-games-store',
            'Epic Gamestore': 'epic-games-store',
            'GOG': 'gog',
            'Sony Entertainment Network': 'playstation-store',
            'Nintendo eShop': 'nintendo-eshop',
            'Google Play': 'google-play-store',
            'Humble Bundle': 'humble-bundle',
            'Origin': 'origin-ea-app',
            'Uplay': 'uplay',
            'Ubisoft Club': 'uplay',
            'GameStop': 'physical',  # Physical retail
            'Gamestop': 'physical',
            
            # Also handle mapper output format if it still gets called
            'PlayStation Store': 'playstation-store',
            'Microsoft Store': 'microsoft-store',
            'Itch.io': 'itch-io',
            'Origin/EA App': 'origin-ea-app',
            'Apple App Store': 'apple-app-store',
            'Google Play Store': 'google-play-store',
            'Physical': 'physical',
            
            # Keep only truly physical retail storefronts
            # Many digital storefronts were incorrectly mapped to 'physical' - let users resolve them
        }
        # Return None for unknown storefronts - user must resolve them manually
        return storefront_mappings.get(storefront_name, None)
    
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
            platform_analysis = await self._analyze_platforms(analysis_data)
            
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
    
    async def _analyze_platforms(self, games_data: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Analyze platform and storefront data from CSV to detect unknown entries and provide resolution suggestions."""
        platform_stats = {}
        storefront_stats = {}
        unknown_platforms = set()
        unknown_storefronts = set() 
        platform_suggestions = {}
        storefront_suggestions = {}
        
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
                    await self._process_platform_name(platform_name, platform_stats, unknown_platforms, platform_suggestions)
            
            # Also check Copy platform field
            copy_platform = game_data.get("Copy platform", "")
            if copy_platform and str(copy_platform).strip() and str(copy_platform) != "nan":
                platform_name = str(copy_platform).strip()
                await self._process_platform_name(platform_name, platform_stats, unknown_platforms, platform_suggestions)
            
            # Extract storefront information from Copy source fields
            await self._process_storefront_data(game_data, storefront_stats, unknown_storefronts)
        
        # Include platforms/storefronts detected by the mapper
        unknown_platforms.update(self.data_mapper.unknown_platforms)
        unknown_storefronts.update(self.data_mapper.unknown_storefronts)
        
        # Generate suggestions for unknown platforms using platform resolution service
        for unknown_platform in unknown_platforms:
            if unknown_platform not in platform_suggestions:
                try:
                    suggestions_response = await self.platform_resolution_service.suggest_platform_matches(
                        unknown_platform_name=unknown_platform,
                        min_confidence=0.6,
                        max_suggestions=3
                    )
                    platform_suggestions[unknown_platform] = {
                        "platform_suggestions": [s.model_dump() for s in suggestions_response.platform_suggestions],
                        "storefront_suggestions": [s.model_dump() for s in suggestions_response.storefront_suggestions]
                    }
                except Exception as e:
                    logger.warning(f"Failed to generate suggestions for platform '{unknown_platform}': {str(e)}")
                    platform_suggestions[unknown_platform] = {
                        "platform_suggestions": [],
                        "storefront_suggestions": []
                    }
        
        # Generate suggestions for unknown storefronts
        for unknown_storefront in unknown_storefronts:
            if unknown_storefront not in storefront_suggestions:
                try:
                    # Try to find general storefront suggestions
                    suggestions_response = await self.platform_resolution_service.suggest_platform_matches(
                        unknown_platform_name="",  # Empty platform name
                        unknown_storefront_name=unknown_storefront,
                        min_confidence=0.6,
                        max_suggestions=3
                    )
                    storefront_suggestions[unknown_storefront] = {
                        "storefront_suggestions": [s.model_dump() for s in suggestions_response.storefront_suggestions]
                    }
                except Exception as e:
                    logger.warning(f"Failed to generate suggestions for storefront '{unknown_storefront}': {str(e)}")
                    storefront_suggestions[unknown_storefront] = {
                        "storefront_suggestions": []
                    }
        
        return {
            "platform_stats": list(platform_stats.values()),
            "storefront_stats": list(storefront_stats.values()),
            "unknown_platforms": list(unknown_platforms),
            "unknown_storefronts": list(unknown_storefronts),
            "platform_suggestions": platform_suggestions,
            "storefront_suggestions": storefront_suggestions,
            "total_platforms": len(platform_stats),
            "total_storefronts": len(storefront_stats),
            "unknown_platform_count": len(unknown_platforms),
            "unknown_storefront_count": len(unknown_storefronts),
            "known_platform_count": len(platform_stats) - len(unknown_platforms),
            "known_storefront_count": len(storefront_stats) - len(unknown_storefronts)
        }
    
    async def _process_platform_name(
        self, 
        platform_name: str, 
        platform_stats: Dict[str, Any], 
        unknown_platforms: set, 
        platform_suggestions: Dict[str, Any]
    ):
        """Process a single platform name and update tracking structures."""
        if platform_name not in platform_stats:
            # Check if platform exists in database using fuzzy matching
            is_known = platform_name in self.data_mapper.PLATFORM_MAPPINGS
            mapped_name = self.data_mapper.PLATFORM_MAPPINGS.get(platform_name)
            suggested_mapping = self.data_mapper._map_platform_name(platform_name)
            
            platform_stats[platform_name] = {
                "name": platform_name,
                "games_count": 0,
                "is_known": is_known,
                "mapped_name": mapped_name,
                "suggested_mapping": suggested_mapping,
                "resolution_status": "resolved" if is_known or suggested_mapping else "pending"
            }
        
        platform_stats[platform_name]["games_count"] += 1
        
        # Track unknown platforms for resolution
        if not platform_stats[platform_name]["is_known"] and not platform_stats[platform_name]["suggested_mapping"]:
            unknown_platforms.add(platform_name)

    async def _process_storefront_data(
        self,
        game_data: Dict[str, Any],
        storefront_stats: Dict[str, Any],
        unknown_storefronts: set
    ):
        """Process storefront data from CSV game entry."""
        # Extract storefront from Copy source field
        copy_source = game_data.get("Copy source", "")
        copy_source_other = game_data.get("Copy source other", "")
        
        # Determine the actual storefront name
        storefront_name = ""
        if copy_source and str(copy_source).strip() and str(copy_source) != "nan":
            copy_source = str(copy_source).strip()
            if copy_source.lower() == "other" and copy_source_other:
                # Use the "other" field when Copy source is "Other"
                storefront_name = str(copy_source_other).strip()
            else:
                storefront_name = copy_source
        
        if not storefront_name:
            return
        
        # Track storefront statistics
        if storefront_name not in storefront_stats:
            # Check if storefront exists in database
            is_known = storefront_name in self.data_mapper.STOREFRONT_MAPPINGS if hasattr(self.data_mapper, 'STOREFRONT_MAPPINGS') else False
            mapped_name = self.data_mapper.STOREFRONT_MAPPINGS.get(storefront_name) if hasattr(self.data_mapper, 'STOREFRONT_MAPPINGS') else None
            
            # Try to map using data mapper
            suggested_mapping = None
            if hasattr(self.data_mapper, '_map_storefront_name'):
                try:
                    suggested_mapping = self.data_mapper._map_storefront_name(storefront_name)
                except Exception:
                    pass
            
            storefront_stats[storefront_name] = {
                "name": storefront_name,
                "games_count": 0,
                "is_known": is_known,
                "mapped_name": mapped_name,
                "suggested_mapping": suggested_mapping,
                "resolution_status": "resolved" if is_known or suggested_mapping else "pending"
            }
        
        storefront_stats[storefront_name]["games_count"] += 1
        
        # Track unknown storefronts for resolution
        if not storefront_stats[storefront_name]["is_known"] and not storefront_stats[storefront_name]["suggested_mapping"]:
            unknown_storefronts.add(storefront_name)
    
    async def import_library(self, user_id: str, progress_callback: Optional[Callable[[int], None]] = None) -> ImportResult:
        """Import CSV data into staging table using enhanced transformation pipeline."""
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
            logger.info(f"Parsed {len(games_data)} rows from CSV file")
            
            # Consolidate copies for games with multiple rows
            consolidated_games = self.consolidation_processor.consolidate_games(games_data)
            logger.info(f"Copy consolidation completed: {len(consolidated_games)} unique games from {len(games_data)} rows")
            
            # Log consolidation stats
            stats = self.consolidation_processor.get_consolidation_stats()
            logger.info(f"Consolidation stats: {stats}")
            
            # Process consolidated games in batches
            imported_count = 0
            skipped_count = 0
            errors = []
            batch_size = 50  # Smaller batches since each game may have multiple copies
            total_games = len(consolidated_games)
            
            # Process games in batches
            for batch_start in range(0, total_games, batch_size):
                batch_end = min(batch_start + batch_size, total_games)
                batch = consolidated_games[batch_start:batch_end]
                
                # Update progress
                if progress_callback:
                    progress_percent = int((batch_start / total_games) * 100)
                    progress_callback(progress_percent)
                
                # Process batch
                batch_imported, batch_skipped, batch_errors = await self._process_consolidated_batch(
                    batch, user_id, batch_start
                )
                
                imported_count += batch_imported
                skipped_count += batch_skipped
                errors.extend(batch_errors)
                
                # Commit after each batch
                self.session.commit()
                logger.debug(f"Processed batch {batch_start}-{batch_end}: {batch_imported} imported, {batch_skipped} skipped")
            
            # Final progress update
            if progress_callback:
                progress_callback(100)
            
            logger.info(f"Import completed: {imported_count} imported, {skipped_count} skipped, {len(errors)} errors")
            
            return ImportResult(
                imported_count=imported_count,
                skipped_count=skipped_count,
                auto_matched_count=0,  # Matching happens in separate step
                total_games=total_games,
                errors=errors
            )
            
        except Exception as e:
            logger.error(f"Error importing library for user {user_id}: {str(e)}")
            self.session.rollback()
            raise ValueError(f"Import failed: {str(e)}")
    
    async def _process_batch(self, batch: List[Dict[str, Any]], user_id: str, 
                           batch_start_index: int) -> Tuple[int, int, List[str]]:
        """Process a batch of transformed games."""
        imported_count = 0
        skipped_count = 0
        errors = []
        
        for i, game_data in enumerate(batch):
            try:
                # Generate external_id (use actual row number from original CSV)
                external_id = str(batch_start_index + i + 1)
                
                # Check if game already exists
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
                    game_name=game_data.get("Name", "Unknown Game"),
                    ignored=False
                )
                
                # Store original and transformed CSV data
                original_data = {k: v for k, v in game_data.items() if not k.startswith('_')}
                darkadia_game.set_csv_data(original_data)
                
                # Store additional transformation metadata if present
                transformation_metadata = {}
                for key, value in game_data.items():
                    if key.startswith('_'):
                        transformation_metadata[key] = value
                
                if transformation_metadata:
                    darkadia_game.set_transformation_data(transformation_metadata)
                
                self.session.add(darkadia_game)
                self.session.flush()  # Get the DarkadiaGame ID
                
                # Create DarkadiaImport records during import phase for platform resolution
                await self._create_darkadia_import_records_for_import(
                    darkadia_game, game_data, batch_start_index + i
                )
                
                imported_count += 1
                
            except Exception as e:
                logger.error(f"Error processing game {batch_start_index + i + 1}: {str(e)}")
                errors.append(f"Row {batch_start_index + i + 1}: {str(e)}")
                continue
        
        return imported_count, skipped_count, errors
    
    async def _process_consolidated_batch(self, batch: List[ConsolidatedGame], user_id: str, 
                                        batch_start_index: int) -> Tuple[int, int, List[str]]:
        """Process a batch of consolidated games."""
        imported_count = 0
        skipped_count = 0
        errors = []
        
        for i, consolidated_game in enumerate(batch):
            try:
                # Generate external_id based on game name hash for consistency
                external_id = f"game_{abs(hash(consolidated_game.name)) % 1000000}"
                
                # Check if game already exists
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
                    game_name=consolidated_game.name,
                    ignored=False
                )
                
                # Store consolidated base data as CSV data (with enhanced debug logging)
                logger.debug(f"About to set CSV data for game '{consolidated_game.name}'")
                log_serialization_debug(consolidated_game.base_data, f"base_data for game '{consolidated_game.name}'")
                
                # Use enhanced serialization for setting CSV data to catch any remaining issues
                try:
                    darkadia_game.set_csv_data(consolidated_game.base_data)
                    logger.debug(f"Successfully set CSV data for game '{consolidated_game.name}'")
                except Exception as csv_data_error:
                    logger.error(f"Failed to set CSV data for game '{consolidated_game.name}': {csv_data_error}")
                    # Try with enhanced safe conversion first
                    from ...utils.json_serialization import make_json_serializable
                    safe_base_data = make_json_serializable(consolidated_game.base_data)
                    darkadia_game.set_csv_data(safe_base_data)
                    logger.warning(f"Used enhanced conversion for CSV data for game '{consolidated_game.name}'")
                
                self.session.add(darkadia_game)
                self.session.flush()  # Get the DarkadiaGame ID
                
                # Create DarkadiaImport records for each copy
                await self._create_darkadia_import_records_for_consolidated_game(
                    darkadia_game, consolidated_game
                )
                
                imported_count += 1
                
            except Exception as e:
                logger.error(f"Error processing consolidated game {consolidated_game.name}: {str(e)}")
                errors.append(f"Game '{consolidated_game.name}': {str(e)}")
                continue
        
        return imported_count, skipped_count, errors
    
    async def _create_darkadia_import_records_for_consolidated_game(
        self, 
        darkadia_game: DarkadiaGame, 
        consolidated_game: ConsolidatedGame
    ) -> None:
        """Create DarkadiaImport records for a consolidated game with multiple copies."""
        try:
            for copy_data in consolidated_game.copies:
                # Enhanced debug logging for copy data serialization issues
                copy_data_dict = {
                    'platform': copy_data.platform,
                    'storefront': copy_data.storefront,
                    'storefront_other': copy_data.storefront_other,
                    'media': copy_data.media,
                    'label': copy_data.label,
                    'release': copy_data.release,
                    'purchase_date': copy_data.purchase_date,
                    'box': copy_data.box,
                    'box_condition': copy_data.box_condition,
                    'box_notes': copy_data.box_notes,
                    'manual': copy_data.manual,
                    'manual_condition': copy_data.manual_condition,
                    'manual_notes': copy_data.manual_notes,
                    'complete': copy_data.complete,
                    'complete_notes': copy_data.complete_notes
                }
                
                logger.info(f"🔧 [IMPORT DEBUG] Processing copy '{copy_data.copy_identifier}' for game '{darkadia_game.game_name}'")
                logger.info(f"🔧 [IMPORT DEBUG] Copy platform: '{copy_data.platform}', storefront: '{copy_data.storefront or copy_data.storefront_other}'")
                log_serialization_debug(copy_data_dict, f"copy_data for game '{darkadia_game.game_name}' copy '{copy_data.copy_identifier}'")
                
                # Additional debug: check the specific fields that might contain Timestamps
                logger.debug(f"Copy purchase_date type: {type(copy_data.purchase_date)} = {copy_data.purchase_date}")
                logger.debug(f"Copy release type: {type(copy_data.release)} = {copy_data.release}")
                
                # Perform platform resolution for this copy
                platform_resolved = False
                storefront_resolved = False
                resolved_platform_id = None
                resolved_storefront_id = None
                
                # Resolve platform
                if copy_data.platform:
                    logger.info(f"🔍 [RESOLUTION DEBUG] Processing platform - Original: '{copy_data.platform}'")
                    
                    # Special handling for Mac - override mapper's decision to map Mac to PC
                    mapped_platform_name = copy_data.platform
                    if copy_data.platform and copy_data.platform.lower() == 'mac':
                        mapped_platform_name = 'Mac'  # Use direct Mac mapping
                        logger.info(f"🔍 [RESOLUTION DEBUG] Mac override applied: '{mapped_platform_name}'")
                    
                    # Normalize platform name to database format
                    normalized_platform_name = self._normalize_platform_name(mapped_platform_name)
                    logger.info(f"🔍 [RESOLUTION DEBUG] Platform normalization: '{mapped_platform_name}' -> '{normalized_platform_name}'")
                    
                    # Look up the normalized platform in the database
                    platform = self.session.exec(
                        select(Platform).where(Platform.name == normalized_platform_name)
                    ).first()
                    if platform:
                        resolved_platform_id = platform.id
                        platform_resolved = True
                        logger.info(f"🔍 [RESOLUTION DEBUG] ✅ Platform resolved: '{normalized_platform_name}' -> {platform.id} ({platform.display_name})")
                    else:
                        logger.info(f"🔍 [RESOLUTION DEBUG] ❌ Platform NOT FOUND: '{normalized_platform_name}'")
                        # Let's see what platforms are available for debugging
                        all_platforms = self.session.exec(select(Platform)).all()
                        available_names = [p.name for p in all_platforms]
                        logger.info(f"🔍 [RESOLUTION DEBUG] Available platforms: {available_names}")
                
                # Resolve storefront
                storefront_name = copy_data.storefront or copy_data.storefront_other
                if storefront_name:
                    logger.info(f"🔍 [RESOLUTION DEBUG] Processing storefront - Original: '{storefront_name}'")
                    
                    # Normalize storefront name to database format
                    normalized_storefront_name = self._normalize_storefront_name(storefront_name)
                    logger.info(f"🔍 [RESOLUTION DEBUG] Storefront normalization: '{storefront_name}' -> '{normalized_storefront_name}'")
                    
                    # Look up the normalized storefront in the database (if we got a mapped name)
                    if normalized_storefront_name:
                        storefront = self.session.exec(
                            select(Storefront).where(Storefront.name == normalized_storefront_name)
                        ).first()
                        if storefront:
                            resolved_storefront_id = storefront.id
                            storefront_resolved = True
                            logger.info(f"🔍 [RESOLUTION DEBUG] ✅ Storefront resolved: '{normalized_storefront_name}' -> {storefront.id} ({storefront.display_name})")
                        else:
                            logger.info(f"🔍 [RESOLUTION DEBUG] ❌ Storefront NOT FOUND: '{normalized_storefront_name}'")
                    else:
                        logger.info(f"🔍 [RESOLUTION DEBUG] ❓ Unknown storefront: '{storefront_name}' - requires user resolution")
                
                # Create a DarkadiaImport record for each copy
                darkadia_import = DarkadiaImport(
                    user_id=darkadia_game.user_id,
                    game_name=darkadia_game.game_name,
                    csv_row_number=copy_data.csv_row_number,
                    copy_identifier=copy_data.copy_identifier,
                    batch_id=darkadia_game.id,
                    csv_file_hash="",  # Will be set during actual import
                    import_timestamp=datetime.now(timezone.utc),
                    
                    # Original CSV data - store the base data merged with copy-specific data
                    # Use enhanced safe JSON dumps with better error handling and debugging
                    original_csv_data_json=enhanced_safe_json_dumps({
                        **consolidated_game.base_data,
                        'Copy platform': copy_data.platform or '',
                        'Copy source': copy_data.storefront or '',
                        'Copy source other': copy_data.storefront_other or '',
                        'Copy media': copy_data.media,
                        'Copy label': copy_data.label,
                        'Copy Release': copy_data.release,
                        'Copy purchase date': copy_data.purchase_date,
                        'Copy box': copy_data.box,
                        'Copy box condition': copy_data.box_condition,
                        'Copy box notes': copy_data.box_notes,
                        'Copy manual': copy_data.manual,
                        'Copy manual condition': copy_data.manual_condition,
                        'Copy manual notes': copy_data.manual_notes,
                        'Copy complete': copy_data.complete,
                        'Copy complete notes': copy_data.complete_notes
                    }, context=f"original_csv_data for game '{darkadia_game.game_name}' copy '{copy_data.copy_identifier}'"),
                    
                    # Boolean flags from consolidated base data
                    played=bool(consolidated_game.base_data.get('Played', False)),
                    playing=bool(consolidated_game.base_data.get('Playing', False)),
                    finished=bool(consolidated_game.base_data.get('Finished', False)),
                    mastered=bool(consolidated_game.base_data.get('Mastered', False)),
                    dominated=bool(consolidated_game.base_data.get('Dominated', False)),
                    shelved=bool(consolidated_game.base_data.get('Shelved', False)),
                    
                    # Platform/storefront resolution tracking
                    original_platform_name=copy_data.platform,
                    original_storefront_name=copy_data.storefront or copy_data.storefront_other,
                    fallback_platform_name=copy_data.platform if not copy_data.is_real_copy else None,
                    platform_resolved=platform_resolved,
                    storefront_resolved=storefront_resolved,
                    resolved_platform_id=resolved_platform_id,
                    resolved_storefront_id=resolved_storefront_id,
                    requires_storefront_resolution=copy_data.requires_storefront_resolution,
                    
                    # Copy metadata
                    physical_copy_data_json=safe_json_dumps({
                        'media': copy_data.media,
                        'label': copy_data.label,
                        'release': copy_data.release,
                        'purchase_date': copy_data.purchase_date,
                        'box': copy_data.box,
                        'box_condition': copy_data.box_condition,
                        'box_notes': copy_data.box_notes,
                        'manual': copy_data.manual,
                        'manual_condition': copy_data.manual_condition,
                        'manual_notes': copy_data.manual_notes,
                        'complete': copy_data.complete,
                        'complete_notes': copy_data.complete_notes
                    }) if any([copy_data.box, copy_data.manual, copy_data.complete]) else None,
                    
                    created_at=datetime.now(timezone.utc),
                    updated_at=datetime.now(timezone.utc)
                )
                
                # Set platform resolution data
                resolution_data = {
                    "status": "resolved" if platform_resolved else "pending",
                    "original_name": copy_data.platform or "",
                    "is_fallback": not copy_data.is_real_copy,
                    "requires_storefront_resolution": copy_data.requires_storefront_resolution,
                    "copy_identifier": copy_data.copy_identifier,
                    "suggestions": [],
                    "storefront_suggestions": [],
                    "resolved_platform_id": resolved_platform_id,
                    "resolved_storefront_id": resolved_storefront_id,
                    "resolution_timestamp": datetime.now(timezone.utc).isoformat() if platform_resolved else None,
                    "resolution_method": "auto" if platform_resolved else None,
                    "user_notes": None
                }
                darkadia_import.set_platform_resolution_data(resolution_data)
                
                self.session.add(darkadia_import)
                
            logger.debug(f"Created {len(consolidated_game.copies)} DarkadiaImport records for {darkadia_game.game_name}")
            
        except Exception as e:
            logger.error(f"Error creating DarkadiaImport records for {darkadia_game.game_name}: {str(e)}")
    
    async def _create_darkadia_import_records_for_import(
        self, 
        darkadia_game: DarkadiaGame, 
        game_data: Dict[str, Any],
        row_index: int
    ) -> None:
        """Create DarkadiaImport records during import phase for platform resolution."""
        try:
            logger.info(f"🔧 [IMPORT DEBUG] _create_darkadia_import_records_for_import called for game: {darkadia_game.game_name}")
            logger.info(f"🔧 [IMPORT DEBUG] game_data keys: {list(game_data.keys())}")
            
            # Get platform data from transformation metadata
            platforms_data = game_data.get('_platforms', [])
            logger.info(f"🔧 [IMPORT DEBUG] Processing {darkadia_game.game_name}: Found {len(platforms_data)} platform entries in _platforms metadata")
            logger.info(f"🔧 [IMPORT DEBUG] Platform data: {platforms_data}")
            
            if not platforms_data:
                # No platform data - check if we have fallback platform info
                csv_data = {k: v for k, v in game_data.items() if not k.startswith('_')}
                platforms_field = csv_data.get('Platforms', '').strip()
                logger.info(f"🔧 [IMPORT DEBUG] No _platforms data, checking fallback. Platforms field: '{platforms_field}'")
                
                if platforms_field:
                    # Create a single DarkadiaImport record for the fallback platform
                    await self._create_single_darkadia_import_for_import(
                        darkadia_game=darkadia_game,
                        original_platform_name=platforms_field.split(',')[0].strip(),
                        original_storefront_name=None,
                        copy_identifier='fallback',
                        csv_row_number=game_data.get('_csv_row_number', row_index + 1),
                        is_fallback=True,
                        platform_data=None
                    )
                return
            
            # Create DarkadiaImport records for each platform/copy
            for platform_data in platforms_data:
                # Use the original platform name for tracking, not the mapped name
                original_platform_name = platform_data.get('original_platform', '').strip()
                # Use the original storefront name for tracking, not the mapped name
                original_storefront_name = platform_data.get('original_storefront', '').strip()
                copy_identifier = platform_data.get('copy_identifier', 'unknown')
                
                logger.debug(f"Processing platform data for {darkadia_game.game_name} - Platform: {original_platform_name}, Storefront: {original_storefront_name}, Copy: {copy_identifier}")
                
                # Only create record if we have platform or storefront data
                if original_platform_name or original_storefront_name:
                    await self._create_single_darkadia_import_for_import(
                        darkadia_game=darkadia_game,
                        original_platform_name=original_platform_name,
                        original_storefront_name=original_storefront_name,
                        copy_identifier=copy_identifier,
                        csv_row_number=game_data.get('_csv_row_number', row_index + 1),
                        is_fallback=False,
                        platform_data=platform_data
                    )
                    
        except Exception as e:
            logger.error(f"Error creating DarkadiaImport records for {darkadia_game.game_name}: {str(e)}")
            # Don't fail the import for resolution record creation errors
    
    async def _create_single_darkadia_import_for_import(
        self,
        darkadia_game: DarkadiaGame,
        original_platform_name: Optional[str],
        original_storefront_name: Optional[str],
        copy_identifier: str,
        csv_row_number: int,
        is_fallback: bool,
        platform_data: Optional[Dict[str, Any]] = None
    ) -> None:
        """Create a single DarkadiaImport record during import phase."""
        try:
            # Check if platform/storefront are already resolved
            platform_resolved = False
            resolved_platform_id = None
            resolved_storefront_id = None
            
            # Get mapped platform name from platform_data or transformation data
            mapped_platform_name = None
            mapped_storefront_name = None
            
            if platform_data:
                # Use mapped names from the platform_data (preferred)
                mapped_platform_name = platform_data.get('platform')
                mapped_storefront_name = platform_data.get('storefront')
            else:
                # Fallback to transformation data (for fallback platforms)
                transform_data = darkadia_game.get_transformation_data()
                if transform_data:
                    mapped_platform_name = transform_data.get('_mapped_platform')
                    mapped_storefront_name = transform_data.get('_mapped_storefront')
            
            # Try to resolve platform
            if mapped_platform_name:
                logger.debug(f"🔍 [RESOLUTION DEBUG] Processing platform - Original: '{original_platform_name}' -> Mapped: '{mapped_platform_name}'")
                
                # Special handling for Mac - override mapper's decision to map Mac to PC
                if original_platform_name and original_platform_name.lower() == 'mac':
                    mapped_platform_name = 'Mac'  # Use direct Mac mapping
                    logger.debug(f"🔍 [RESOLUTION DEBUG] Mac override applied: '{mapped_platform_name}'")
                
                # Normalize platform name to database format
                normalized_platform_name = self._normalize_platform_name(mapped_platform_name)
                logger.debug(f"🔍 [RESOLUTION DEBUG] Platform normalization: '{mapped_platform_name}' -> '{normalized_platform_name}'")
                
                # Look up the normalized platform in the database
                platform = self.session.exec(
                    select(Platform).where(Platform.name == normalized_platform_name)
                ).first()
                if platform:
                    resolved_platform_id = platform.id
                    platform_resolved = True
                    logger.debug(f"🔍 [RESOLUTION DEBUG] ✅ Platform resolved: '{normalized_platform_name}' -> {platform.id} ({platform.display_name})")
                else:
                    logger.debug(f"🔍 [RESOLUTION DEBUG] ❌ Platform NOT FOUND: '{normalized_platform_name}'")
                    # Let's see what platforms are available for debugging
                    all_platforms = self.session.exec(select(Platform)).all()
                    available_names = [p.name for p in all_platforms]
                    logger.debug(f"🔍 [RESOLUTION DEBUG] Available platforms: {available_names}")
            
            # Try to resolve storefront
            if mapped_storefront_name:
                logger.debug(f"🔍 [RESOLUTION DEBUG] Processing storefront - Original: '{original_storefront_name}' -> Mapped: '{mapped_storefront_name}'")
                
                # Normalize storefront name to database format
                normalized_storefront_name = self._normalize_storefront_name(mapped_storefront_name)
                logger.debug(f"🔍 [RESOLUTION DEBUG] Storefront normalization: '{mapped_storefront_name}' -> '{normalized_storefront_name}'")
                
                # Look up the normalized storefront in the database (if we got a mapped name)
                if normalized_storefront_name:
                    storefront = self.session.exec(
                        select(Storefront).where(Storefront.name == normalized_storefront_name)
                    ).first()
                    if storefront:
                        resolved_storefront_id = storefront.id
                        logger.debug(f"🔍 [RESOLUTION DEBUG] ✅ Storefront resolved: '{normalized_storefront_name}' -> {storefront.id} ({storefront.display_name})")
                    else:
                        logger.debug(f"🔍 [RESOLUTION DEBUG] ❌ Storefront NOT FOUND: '{normalized_storefront_name}'")
                else:
                    logger.debug(f"🔍 [RESOLUTION DEBUG] ❓ Unknown storefront: '{mapped_storefront_name}' - requires user resolution")
            
            # Create the DarkadiaImport record
            darkadia_import = DarkadiaImport(
                user_id=darkadia_game.user_id,
                game_name=darkadia_game.game_name,
                csv_row_number=csv_row_number,
                copy_identifier=copy_identifier,
                batch_id=darkadia_game.id,  # Set batch_id to DarkadiaGame ID
                csv_file_hash="",  # Will be set during actual import
                import_timestamp=datetime.now(timezone.utc),
                original_platform_name=original_platform_name,
                original_storefront_name=original_storefront_name,
                platform_resolved=platform_resolved,
                resolved_platform_id=resolved_platform_id,
                resolved_storefront_id=resolved_storefront_id,
                user_game_platform_id=None,  # Will be set during sync phase
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            
            # Set resolution data
            resolution_data = {
                "status": "resolved" if platform_resolved else "pending",
                "original_name": original_platform_name or "",
                "is_fallback": is_fallback,
                "requires_storefront_resolution": bool(original_storefront_name and not resolved_storefront_id),
                "suggestions": [],
                "storefront_suggestions": [],
                "resolved_platform_id": resolved_platform_id,
                "resolved_storefront_id": resolved_storefront_id,
                "resolution_timestamp": None,
                "resolution_method": "auto" if platform_resolved else None,
                "user_notes": None
            }
            darkadia_import.set_platform_resolution_data(resolution_data)
            
            self.session.add(darkadia_import)
            
            logger.debug(f"Created DarkadiaImport record for {darkadia_game.game_name} - Platform: {original_platform_name}, Storefront: {original_storefront_name}, Copy: {copy_identifier}")
            
        except Exception as e:
            logger.error(f"Error creating single DarkadiaImport record: {str(e)}")
    
    # Note: _create_darkadia_import_record method removed - DarkadiaImport records
    # are now created during both import AND sync phases
    
    async def _create_platform_associations(self, user_game: UserGame, 
                                           darkadia_game: DarkadiaGame,
                                           csv_data: Dict[str, Any]) -> None:
        """Create UserGamePlatform associations from Darkadia copy data, one per copy."""
        try:
            # Get platform data from consolidated CSV data
            platforms_data = csv_data.get('_platforms', [])
            
            if not platforms_data:
                logger.warning(f"No platform data found for game {darkadia_game.game_name}")
                return
            
            logger.info(f"Creating {len(platforms_data)} platform associations for game {darkadia_game.game_name}")
            
            # Process each copy/platform combination
            for platform_data in platforms_data:
                await self._create_single_platform_association(
                    user_game=user_game,
                    darkadia_game=darkadia_game,
                    platform_data=platform_data,
                    csv_data=csv_data
                )
            
        except Exception as e:
            logger.error(f"Error creating platform associations for {darkadia_game.game_name}: {str(e)}")
            # Don't fail the sync for platform association errors
    
    async def _create_single_platform_association(self, user_game: UserGame,
                                                darkadia_game: DarkadiaGame,
                                                platform_data: Dict[str, Any],
                                                csv_data: Dict[str, Any]) -> None:
        """Create a single UserGamePlatform association and link DarkadiaImport record."""
        try:
            # Extract platform and storefront information
            original_platform_name = platform_data.get('platform', '').strip()
            original_storefront_name = platform_data.get('storefront', '').strip() or platform_data.get('storefront_other', '').strip()
            copy_identifier = platform_data.get('copy_identifier')
            is_real_copy = platform_data.get('is_real_copy', True)
            platform_data.get('requires_storefront_resolution', False)
            
            # Get transformed platform/storefront data if available
            transform_data = darkadia_game.get_transformation_data()
            
            # Use mapped values if available, otherwise fall back to original
            platform_name = original_platform_name
            storefront_name = original_storefront_name
            
            if transform_data:
                mapped_platform = transform_data.get('_mapped_platform')
                mapped_storefront = transform_data.get('_mapped_storefront') 
                
                if mapped_platform:
                    platform_name = mapped_platform
                if mapped_storefront:
                    storefront_name = mapped_storefront
            
            # Apply data mapper transformations if transformation data not available
            if not transform_data and original_platform_name:
                mapped_platform = self.data_mapper._map_platform_name(original_platform_name)
                if mapped_platform:
                    platform_name = mapped_platform
                    
                if original_storefront_name:
                    mapped_storefront = self.data_mapper._map_storefront_name(original_storefront_name, platform_name)
                    if mapped_storefront:
                        storefront_name = mapped_storefront
            
            # Get fallback platform name for tracking
            if not is_real_copy:
                pass
            
            # Look up platform by name (using mapped name)
            platform = None
            if platform_name:
                platform = self.session.exec(
                    select(Platform).where(Platform.name == platform_name)
                ).first()
                
                if not platform:
                    logger.info(f"Platform '{platform_name}' (mapped from '{original_platform_name}') pending resolution for copy {copy_identifier}")
            
            # Look up storefront by name (using mapped name)
            storefront = None
            if storefront_name:
                storefront = self.session.exec(
                    select(Storefront).where(Storefront.name == storefront_name)
                ).first()
                
                if not storefront:
                    logger.warning(f"Storefront '{storefront_name}' (mapped from '{original_storefront_name}') not found in database for copy {copy_identifier}")
            
            # Check if association already exists
            existing_association = self.session.exec(
                select(UserGamePlatform).where(
                    and_(
                        UserGamePlatform.user_game_id == user_game.id,
                        UserGamePlatform.platform_id == (platform.id if platform else None),
                        UserGamePlatform.storefront_id == (storefront.id if storefront else None)
                    )
                )
            ).first()
            
            if existing_association:
                platform_name_display = platform.display_name if platform else f"Unresolved ({platform_name})"
                logger.debug(f"Platform association already exists for {darkadia_game.game_name} on {platform_name_display} (copy: {copy_identifier})")
                
                # Still need to create/update DarkadiaImport record
                await self._create_or_update_darkadia_import(
                    user_game=user_game,
                    user_game_platform=existing_association,
                    darkadia_game=darkadia_game,
                    platform_data=platform_data,
                    csv_data=csv_data
                )
                return
            
            # Determine if this is a physical copy
            is_physical = platform_data.get('media', '').lower() == 'physical'
            
            # Create the association
            user_game_platform = UserGamePlatform(
                user_game_id=user_game.id,
                platform_id=platform.id if platform else None,
                storefront_id=storefront.id if storefront else None,
                original_platform_name=original_platform_name if not platform else None,
                is_available=True,
                is_physical=is_physical,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            
            self.session.add(user_game_platform)
            self.session.flush()  # Get the ID for the DarkadiaImport record
            
            # Create corresponding DarkadiaImport record
            await self._create_or_update_darkadia_import(
                user_game=user_game,
                user_game_platform=user_game_platform,
                darkadia_game=darkadia_game,
                platform_data=platform_data,
                csv_data=csv_data
            )
            
            if platform:
                logger.info(f"🎮 [Darkadia Service] Created platform association: {darkadia_game.game_name} on {platform.display_name} ({storefront.display_name if storefront else 'No storefront'}) [Copy: {copy_identifier}]")
            else:
                logger.info(f"🎮 [Darkadia Service] Created unresolved platform association: {darkadia_game.game_name} for '{platform_name}' ({storefront.display_name if storefront else 'No storefront'}) [Copy: {copy_identifier}]")
            
        except Exception as e:
            logger.error(f"Error creating single platform association for {darkadia_game.game_name} (copy: {copy_identifier}): {str(e)}")
    
    async def _create_or_update_darkadia_import(self, user_game: UserGame,
                                              user_game_platform: UserGamePlatform,
                                              darkadia_game: DarkadiaGame,
                                              platform_data: Dict[str, Any],
                                              csv_data: Dict[str, Any]) -> None:
        """Create or update DarkadiaImport record for this specific copy."""
        try:
            copy_identifier = platform_data.get('copy_identifier')
            # Ensure we have a valid csv_row_number
            csv_row_number = csv_data.get('_csv_row_number')
            if csv_row_number is None:
                # Fallback: use a hash of the game name and copy identifier for uniqueness
                fallback_id = f"{darkadia_game.game_name}_{copy_identifier or 'default'}"
                csv_row_number = abs(hash(fallback_id)) % 1000000
            
            # Create DarkadiaImport record for this specific copy
            darkadia_import = DarkadiaImport(
                user_id=user_game.user_id,
                user_game_id=user_game.id,
                user_game_platform_id=user_game_platform.id,
                csv_row_number=csv_row_number,
                game_name=darkadia_game.game_name,
                copy_identifier=copy_identifier,
                batch_id=darkadia_game.id,  # Link to the DarkadiaGame
                csv_file_hash="",  # Will be set during import
                import_timestamp=datetime.now(timezone.utc),
                
                # Copy metadata
                original_csv_data_json=json.dumps(csv_data),
                physical_copy_data_json=json.dumps(platform_data.get('metadata', {})) if platform_data.get('metadata') else None,
                
                # Boolean flags from CSV
                played=bool(csv_data.get('Played', False)),
                playing=bool(csv_data.get('Playing', False)),
                finished=bool(csv_data.get('Finished', False)),
                mastered=bool(csv_data.get('Mastered', False)),
                dominated=bool(csv_data.get('Dominated', False)),
                shelved=bool(csv_data.get('Shelved', False)),
                
                # Platform/storefront resolution tracking
                original_platform_name=platform_data.get('platform'),
                original_storefront_name=platform_data.get('storefront') or platform_data.get('storefront_other'),
                fallback_platform_name=platform_data.get('platform') if not platform_data.get('is_real_copy', True) else None,
                platform_resolved=bool(user_game_platform.platform_id),
                storefront_resolved=bool(user_game_platform.storefront_id),
                requires_storefront_resolution=platform_data.get('requires_storefront_resolution', False),
                platform_resolution_data_json=await self._generate_platform_resolution_data(
                    platform_data.get('platform'), bool(user_game_platform.platform_id)
                ),
                
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            
            self.session.add(darkadia_import)
            logger.debug(f"Created DarkadiaImport record for copy {copy_identifier}")
            
        except Exception as e:
            logger.error(f"Error creating DarkadiaImport record for copy {copy_identifier}: {str(e)}")
    
    async def _generate_platform_resolution_data(self, original_platform_name: str, platform_resolved: bool) -> str:
        """Generate platform resolution data for DarkadiaImport record."""
        resolution_data = {
            "status": "resolved" if platform_resolved else "pending",
            "original_name": original_platform_name,
            "suggestions": [],
            "storefront_suggestions": [],
            "resolved_platform_id": None,
            "resolved_storefront_id": None,
            "resolution_timestamp": None,
            "resolution_method": "auto" if platform_resolved else None,
            "user_notes": None
        }
        
        # If not resolved, generate suggestions
        if not platform_resolved and original_platform_name:
            try:
                suggestions_response = await self.platform_resolution_service.suggest_platform_matches(
                    unknown_platform_name=original_platform_name,
                    min_confidence=0.6,
                    max_suggestions=3
                )
                resolution_data["suggestions"] = [s.model_dump() for s in suggestions_response.platform_suggestions]
                resolution_data["storefront_suggestions"] = [s.model_dump() for s in suggestions_response.storefront_suggestions]
                
                if resolution_data["suggestions"]:
                    resolution_data["status"] = "suggested"
            except Exception as e:
                logger.warning(f"Failed to generate suggestions for platform '{original_platform_name}': {str(e)}")
        
        return json.dumps(resolution_data)
    
    async def list_games(self, 
                        user_id: str, 
                        offset: int = 0,
                        limit: int = 100,
                        status_filter: Optional[str] = None,
                        search: Optional[str] = None) -> Tuple[List[ImportGame], int]:
        """List imported games with filtering and pagination."""
        
        # Start with DarkadiaGame query
        query = select(DarkadiaGame).where(DarkadiaGame.user_id == user_id)
        
        # Apply status filter
        if status_filter == "unmatched":
            query = query.where(DarkadiaGame.igdb_id is None)
        elif status_filter == "matched":
            query = query.where(DarkadiaGame.igdb_id is not None)
        elif status_filter == "ignored":
            query = query.where(DarkadiaGame.ignored)
        elif status_filter == "synced":
            query = query.where(DarkadiaGame.game_id is not None)
        
        # Apply search filter
        if search:
            search_term = f"%{search.lower()}%"
            query = query.where(func.lower(DarkadiaGame.game_name).like(search_term))
        
        # Get total count and games
        games = self.session.exec(query.offset(offset).limit(limit)).all()
        # Use direct count query to avoid Cartesian product from subquery
        count_query = select(func.count(DarkadiaGame.id)).where(DarkadiaGame.user_id == user_id)
        
        # Apply the same filtering as main query
        if status_filter == "unmatched":
            count_query = count_query.where(DarkadiaGame.igdb_id is None)
        elif status_filter == "matched":
            count_query = count_query.where(DarkadiaGame.igdb_id is not None)
        elif status_filter == "ignored":
            count_query = count_query.where(DarkadiaGame.ignored)
        elif status_filter == "synced":
            count_query = count_query.where(DarkadiaGame.game_id is not None)
        
        if search:
            search_term = f"%{search.lower()}%"
            count_query = count_query.where(func.lower(DarkadiaGame.game_name).like(search_term))
        
        total_count = self.session.exec(count_query).first()
        
        logger.info(f"Successfully fetched {len(games)} games out of {total_count} total for user {user_id}")
        logger.debug(f"Service layer count query result for user {user_id}: {total_count} distinct games")
        
        # Convert to ImportGame objects
        import_games = []
        for game in games:
            # Get first DarkadiaImport for this game to avoid duplicates
            darkadia_import = self.session.exec(
                select(DarkadiaImport)
                .where(DarkadiaImport.user_id == user_id)
                .where(DarkadiaImport.game_name == game.game_name)
                .order_by(DarkadiaImport.created_at.asc())
                .limit(1)
            ).first()
            
            # Determine platform resolution status and names
            platform_resolved = None
            original_platform_name = None
            platform_resolution_status = None
            platform_name = None
            storefront_name = None
            original_storefront_name = None
            storefront_resolution_status = None
            
            if darkadia_import:
                platform_resolved = darkadia_import.platform_resolved
                original_platform_name = darkadia_import.original_platform_name
                original_storefront_name = darkadia_import.original_storefront_name
                
                # Get resolved platform and storefront names from relationships
                if darkadia_import.user_game_platform_id:
                    user_game_platform = self.session.get(UserGamePlatform, darkadia_import.user_game_platform_id)
                    if user_game_platform:
                        if user_game_platform.platform_id:
                            platform = self.session.get(Platform, user_game_platform.platform_id)
                            if platform:
                                platform_name = platform.name
                        if user_game_platform.storefront_id:
                            storefront = self.session.get(Storefront, user_game_platform.storefront_id)
                            if storefront:
                                storefront_name = storefront.name
                
                # Determine status based on resolution data
                if darkadia_import.platform_resolved:
                    platform_resolution_status = "resolved"
                elif darkadia_import.original_platform_name:
                    resolution_data = darkadia_import.get_platform_resolution_data()
                    status = resolution_data.get("status", "pending")
                    platform_resolution_status = status
                
                # Determine storefront resolution status
                if darkadia_import.storefront_resolved:
                    storefront_resolution_status = "resolved"
                elif original_storefront_name:
                    storefront_resolution_status = "mapped" if storefront_name else "pending"
            else:
                # No DarkadiaImport record - try to get data from DarkadiaGame
                transform_data = game.get_transformation_data()
                csv_data = game.get_csv_data()
                
                # Get mapped names from transformation data
                if transform_data:
                    platform_name = transform_data.get('_mapped_platform')
                    storefront_name = transform_data.get('_mapped_storefront')
                
                # Get original names from CSV data
                if csv_data:
                    original_platform_name = csv_data.get('Copy platform', '').strip() or csv_data.get('Platforms', '').strip()
                    original_storefront_name = csv_data.get('Copy storefront', '').strip() or csv_data.get('Storefronts', '').strip()
                    if original_platform_name:
                        platform_resolution_status = "mapped" if platform_name else "pending"
                    if original_storefront_name:
                        storefront_resolution_status = "mapped" if storefront_name else "pending"
            
            import_games.append(ImportGame(
                id=game.id,
                external_id=game.external_id,
                name=game.game_name,
                igdb_id=game.igdb_id,
                igdb_title=game.igdb_title,
                game_id=game.game_id,
                ignored=game.ignored,
                created_at=game.created_at,
                updated_at=game.updated_at,
                platform_resolved=platform_resolved,
                original_platform_name=original_platform_name,
                platform_resolution_status=platform_resolution_status,
                platform_name=platform_name,
                original_storefront_name=original_storefront_name,
                storefront_resolution_status=storefront_resolution_status,
                storefront_name=storefront_name
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
            updated_at=game.updated_at,
            # Fields specific to the main list_games method, not available here
            platform_resolved=None,
            original_platform_name=None,
            platform_resolution_status=None,
            platform_name=None,
            original_storefront_name=None,
            storefront_resolution_status=None,
            storefront_name=None
        )
    
    async def auto_match_game(self, user_id: str, game_id: str) -> MatchResult:
        """Automatically match single game to IGDB."""
        game = self.session.get(DarkadiaGame, game_id)
        if not game or game.user_id != user_id:
            raise ValueError(f"Game {game_id} not found")
        
        try:
            # For now, implement a simple mock matching
            # TODO: Implement real IGDB integration
            if len(game.game_name.strip()) > 0:  # Allow all non-empty game names
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
                    DarkadiaGame.igdb_id is None,
                    not DarkadiaGame.ignored
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
                
                logger.info("🎮 [Darkadia Service] Creating new UserGame relationship")
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
                
                # Create UserGamePlatform associations from copy data (now handles DarkadiaImport creation)
                await self._create_platform_associations(user_game, darkadia_game, csv_data)
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
        """Resolve play status from Darkadia boolean flags with correct mapping."""
        # Simple priority resolution based on Darkadia's design
        if played_flags.get("dominated", False):
            return PlayStatus.DOMINATED
        elif played_flags.get("mastered", False):
            return PlayStatus.MASTERED
        elif played_flags.get("finished", False):
            return PlayStatus.COMPLETED
        elif played_flags.get("shelved", False):
            return PlayStatus.DROPPED  # Shelved = permanently abandoned in Darkadia
        elif played_flags.get("playing", False):
            return PlayStatus.IN_PROGRESS
        elif played_flags.get("played", False):
            return PlayStatus.SHELVED  # Played but not finished = paused/backlog
        else:
            return PlayStatus.NOT_STARTED
    
    async def sync_all_games(self, user_id: str) -> BulkOperationResult:
        """Sync all matched games to main collection."""
        # Get all matched but unsynced games
        matched_games = self.session.exec(
            select(DarkadiaGame).where(
                and_(
                    DarkadiaGame.user_id == user_id,
                    DarkadiaGame.igdb_id is not None,
                    DarkadiaGame.game_id is None,  # Not yet synced
                    not DarkadiaGame.ignored
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
                    DarkadiaGame.game_id is not None
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
            updated_at=darkadia_game.updated_at,
            # Fields specific to the main list_games method, not available here
            platform_resolved=None,
            original_platform_name=None,
            platform_resolution_status=None,
            platform_name=None,
            original_storefront_name=None,
            storefront_resolution_status=None,
            storefront_name=None
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
            updated_at=game.updated_at,
            # Fields specific to the main list_games method, not available here
            platform_resolved=None,
            original_platform_name=None,
            platform_resolution_status=None,
            platform_name=None,
            original_storefront_name=None,
            storefront_resolution_status=None,
            storefront_name=None
        )
    
    async def unignore_all_games(self, user_id: str) -> BulkOperationResult:
        """Unignore all ignored games."""
        ignored_games = self.session.exec(
            select(DarkadiaGame).where(
                and_(
                    DarkadiaGame.user_id == user_id,
                    DarkadiaGame.ignored
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
                    DarkadiaGame.igdb_id is not None
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
    
    async def reset_import(self, user_id: str) -> Dict[str, Any]:
        """
        Complete reset of Darkadia import data.
        
        Unsyncs all synced games, deletes all import data, clears configuration,
        and removes uploaded CSV file. Returns the system to pre-import state.
        """
        try:
            logger.info(f"🔄 [Darkadia Reset] Starting complete reset for user {user_id}")
            
            # Statistics to return
            stats = {
                "deleted_games": 0,
                "unsynced_games": 0, 
                "deleted_imports": 0,
                "config_cleared": False,
                "file_deleted": False
            }
            
            # Step 1: Unsync all synced games (this removes UserGame records but preserves DarkadiaGame records)
            logger.info("🔄 [Darkadia Reset] Step 1: Unsyncing all games...")
            unsync_result = await self.unsync_all_games(user_id)
            stats["unsynced_games"] = unsync_result.successful_operations
            logger.info(f"🔄 [Darkadia Reset] Unsynced {stats['unsynced_games']} games")
            
            # Step 2: Delete all DarkadiaImport records for this user
            logger.info("🔄 [Darkadia Reset] Step 2: Deleting DarkadiaImport records...")
            darkadia_imports = self.session.exec(
                select(DarkadiaImport).where(DarkadiaImport.user_id == user_id)
            ).all()
            
            for darkadia_import in darkadia_imports:
                self.session.delete(darkadia_import)
            
            stats["deleted_imports"] = len(darkadia_imports)
            logger.info(f"🔄 [Darkadia Reset] Deleted {stats['deleted_imports']} import records")
            
            # Step 3: Delete all DarkadiaGame records for this user
            logger.info("🔄 [Darkadia Reset] Step 3: Deleting DarkadiaGame records...")
            darkadia_games = self.session.exec(
                select(DarkadiaGame).where(DarkadiaGame.user_id == user_id)
            ).all()
            
            for darkadia_game in darkadia_games:
                self.session.delete(darkadia_game)
            
            stats["deleted_games"] = len(darkadia_games)
            logger.info(f"🔄 [Darkadia Reset] Deleted {stats['deleted_games']} staging games")
            
            # Step 4: Clear user Darkadia configuration
            logger.info("🔄 [Darkadia Reset] Step 4: Clearing user configuration...")
            user = self.session.get(User, user_id)
            if user:
                preferences = user.preferences or {}
                darkadia_config = preferences.get("darkadia", {})
                csv_file_path = darkadia_config.get("csv_file_path")
                
                # Clear Darkadia configuration
                if "darkadia" in preferences:
                    del preferences["darkadia"]
                    # Always use "{}" as the default instead of None to respect the NOT NULL constraint
                    user.preferences_json = json.dumps(preferences) if preferences else "{}"
                    user.updated_at = datetime.now(timezone.utc)
                    self.session.add(user)
                    stats["config_cleared"] = True
                    logger.info("🔄 [Darkadia Reset] Cleared user configuration")
                
                # Step 5: Delete CSV file if it exists
                if csv_file_path and Path(csv_file_path).exists():
                    try:
                        Path(csv_file_path).unlink()
                        stats["file_deleted"] = True
                        logger.info(f"🔄 [Darkadia Reset] Deleted CSV file: {csv_file_path}")
                    except Exception as file_error:
                        logger.warning(f"🔄 [Darkadia Reset] Failed to delete CSV file {csv_file_path}: {file_error}")
            
            # Commit all changes
            self.session.commit()
            
            logger.info(f"🔄 [Darkadia Reset] Reset completed successfully: {stats}")
            
            return {
                "message": "Darkadia import reset completed successfully",
                **stats
            }
            
        except Exception as e:
            logger.error(f"🔄 [Darkadia Reset] Error during reset for user {user_id}: {str(e)}")
            self.session.rollback()
            raise ValueError(f"Failed to reset Darkadia import: {str(e)}")


def create_darkadia_import_service(session: Session) -> DarkadiaImportService:
    """Factory function to create Darkadia import service."""
    return DarkadiaImportService(session)
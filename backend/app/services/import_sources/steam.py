"""
Steam import service implementation using the new import framework.
"""

import logging
import json
from typing import Any, Dict, List, Optional, Tuple
from datetime import datetime, timezone
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
from ..sync_utils import is_steam_game_synced
from ...models.steam_game import SteamGame
from ...models.user_game import UserGame
from ...services.steam import create_steam_service, SteamAuthenticationError, SteamAPIError
from ...services.steam_games import create_steam_games_service, SteamGamesServiceError
from ...services.igdb import IGDBService

logger = logging.getLogger(__name__)


class SteamImportService(ImportSourceService):
    """Steam-specific import service implementation."""
    
    def __init__(self, session: Session, igdb_service: Optional[IGDBService] = None):
        super().__init__("steam")
        self.session = session
        self.igdb_service = igdb_service
    
    def _handle_steam_service_error(self, e: Exception) -> None:
        """Convert SteamGamesServiceError to appropriate exceptions for HTTP status codes."""
        error_msg = str(e)
        if "not found or access denied" in error_msg.lower():
            raise FileNotFoundError(error_msg)  # Will be converted to 404
        elif "must be matched to igdb" in error_msg.lower():
            raise PermissionError(error_msg)  # Will be converted to 422
        raise ValueError(error_msg)  # Will be converted to 400
    
    def _get_user_steam_config(self, user: User) -> dict:
        """Get user's Steam configuration from preferences."""
        try:
            preferences = user.preferences or {}
            steam_config = preferences.get("steam", {})
            logger.debug(f"Retrieved Steam config for user {user.id}: has_api_key={bool(steam_config.get('web_api_key'))}, has_steam_id={bool(steam_config.get('steam_id'))}, is_verified={steam_config.get('is_verified', False)}")
            return steam_config
        except (TypeError, AttributeError) as e:
            logger.error(f"Error parsing preferences for user {user.id}: {str(e)}")
            return {}
    
    def _update_user_steam_config(self, user: User, steam_config: dict) -> None:
        """Update user's Steam configuration in preferences."""
        try:
            preferences = user.preferences or {}
            preferences["steam"] = steam_config
            user.preferences_json = json.dumps(preferences)
            user.updated_at = datetime.now(timezone.utc)
            
            self.session.add(user)
            self.session.commit()
            self.session.refresh(user)
            logger.debug(f"Steam config updated for user {user.id}")
        except Exception as e:
            logger.error(f"Error updating Steam config for user {user.id}: {str(e)}")
            self.session.rollback()
            raise
    
    async def get_config(self, user_id: str) -> ImportSourceConfig:
        """Get Steam configuration for user."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        steam_config = self._get_user_steam_config(user)
        
        return ImportSourceConfig(
            source_name=self.source_name,
            is_configured=bool(steam_config.get("web_api_key")),
            is_verified=steam_config.get("is_verified", False),
            configured_at=datetime.fromisoformat(steam_config["configured_at"]) if steam_config.get("configured_at") else None,
            last_import=None,  # TODO: Track last import from import jobs
            config_data={
                "has_api_key": bool(steam_config.get("web_api_key")),
                "api_key_masked": self._mask_api_key(steam_config.get("web_api_key", "")),
                "steam_id": steam_config.get("steam_id"),
                "is_verified": steam_config.get("is_verified", False)
            }
        )
    
    def _mask_api_key(self, api_key: str) -> Optional[str]:
        """Mask Steam API key for safe display."""
        if not api_key:
            return None
        if len(api_key) < 12:
            return "****"
        return f"{api_key[:8]}****{api_key[-4:]}"
    
    async def set_config(self, user_id: str, config_data: Dict[str, Any]) -> ImportSourceConfig:
        """Set/update Steam configuration."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        web_api_key = config_data.get("web_api_key")
        steam_id = config_data.get("steam_id")
        
        if not web_api_key:
            raise ValueError("Steam Web API key is required")
        
        # Verify the configuration
        is_valid, error_message, verification_data = await self.verify_config(config_data)
        if not is_valid:
            raise ValueError(error_message or "Steam configuration verification failed")
        
        # Save configuration
        steam_config = {
            "web_api_key": web_api_key,
            "steam_id": steam_id,
            "is_verified": True,
            "configured_at": datetime.now(timezone.utc).isoformat()
        }
        
        self._update_user_steam_config(user, steam_config)
        
        return await self.get_config(user_id)
    
    async def delete_config(self, user_id: str) -> bool:
        """Delete Steam configuration."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        preferences = user.preferences or {}
        if "steam" in preferences:
            del preferences["steam"]
            user.preferences_json = json.dumps(preferences)
            user.updated_at = datetime.now(timezone.utc)
            self.session.add(user)
            self.session.commit()
        
        return True
    
    async def verify_config(self, config_data: Dict[str, Any]) -> Tuple[bool, Optional[str], Optional[Dict[str, Any]]]:
        """Verify Steam configuration without saving."""
        web_api_key = config_data.get("web_api_key")
        steam_id = config_data.get("steam_id")
        
        if not web_api_key:
            return False, "Steam Web API key is required", None
        
        try:
            steam_service = create_steam_service(web_api_key)
            
            # Verify API key
            is_valid = await steam_service.verify_api_key()
            if not is_valid:
                return False, "Invalid Steam Web API key", None
            
            verification_data = {}
            
            # If Steam ID is provided, verify it
            if steam_id:
                if not steam_service.validate_steam_id(steam_id):
                    return False, "Invalid Steam ID format", None
                
                user_info = await steam_service.get_user_info(steam_id)
                if not user_info:
                    return False, "Steam ID not found or profile is private", None
                
                verification_data["steam_user_info"] = {
                    "steam_id": user_info.steam_id,
                    "persona_name": user_info.persona_name,
                    "profile_url": user_info.profile_url,
                    "avatar": user_info.avatar,
                    "avatar_medium": user_info.avatar_medium,
                    "avatar_full": user_info.avatar_full
                }
            
            return True, None, verification_data
            
        except (SteamAuthenticationError, SteamAPIError) as e:
            return False, str(e), None
        except Exception as e:
            logger.error(f"Error verifying Steam config: {str(e)}")
            return False, "Verification failed due to an unexpected error", None
    
    async def resolve_vanity_url(self, user_id: str, vanity_url: str) -> Tuple[bool, Optional[str], Optional[str]]:
        """Resolve Steam vanity URL to Steam ID."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        steam_config = self._get_user_steam_config(user)
        web_api_key = steam_config.get("web_api_key")
        
        if not web_api_key:
            return False, None, "Please configure your Steam Web API key first"
        
        try:
            steam_service = create_steam_service(web_api_key)
            steam_id = await steam_service.resolve_vanity_url(vanity_url)
            
            if steam_id:
                return True, steam_id, None
            else:
                return False, None, "Vanity URL not found or user does not exist"
                
        except Exception as e:
            logger.error(f"Error resolving vanity URL for user {user_id}: {str(e)}")
            return False, None, "Failed to resolve vanity URL"
    
    async def get_library_preview(self, user_id: str) -> Dict[str, Any]:
        """Get preview of user's Steam library."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        steam_config = self._get_user_steam_config(user)
        web_api_key = steam_config.get("web_api_key")
        steam_id = steam_config.get("steam_id")
        
        if not web_api_key:
            raise ValueError("Please configure your Steam Web API key first")
        if not steam_id:
            raise ValueError("Please configure your Steam ID first")
        
        try:
            steam_service = create_steam_service(web_api_key)
            
            # Get user info
            user_info = await steam_service.get_user_info(steam_id)
            if not user_info:
                raise ValueError("Steam profile not found or is private")
            
            # Get owned games
            games = await steam_service.get_owned_games(steam_id)
            
            return {
                "total_games": len(games),
                "preview_games": [
                    {
                        "appid": game.appid,
                        "name": game.name,
                        "img_icon_url": game.img_icon_url
                    } for game in games[:10]  # Show first 10 games as preview
                ],
                "steam_user_info": {
                    "steam_id": user_info.steam_id,
                    "persona_name": user_info.persona_name,
                    "profile_url": user_info.profile_url,
                    "avatar": user_info.avatar,
                    "avatar_medium": user_info.avatar_medium,
                    "avatar_full": user_info.avatar_full
                }
            }
            
        except (SteamAuthenticationError, SteamAPIError) as e:
            raise ValueError(f"Steam API error: {str(e)}")
        except Exception as e:
            logger.error(f"Error getting Steam library preview for user {user_id}: {str(e)}")
            raise ValueError("Failed to retrieve Steam library")
    
    async def import_library(self, user_id: str) -> ImportResult:
        """Import user's Steam library."""
        user = self.session.get(User, user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        
        steam_config = self._get_user_steam_config(user)
        
        if not steam_config.get("web_api_key"):
            raise ValueError("Steam Web API key not configured")
        if not steam_config.get("steam_id"):
            raise ValueError("Steam ID not configured")
        if not steam_config.get("is_verified", False):
            raise ValueError("Steam configuration not verified")
        
        try:
            # Use existing Steam games service for import logic
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            result = await steam_games_service.import_steam_library(
                user_id=user_id,
                steam_config=steam_config,
                enable_auto_matching=True
            )
            
            return ImportResult(
                imported_count=result.imported_count,
                skipped_count=result.skipped_count,
                auto_matched_count=result.auto_matched_count,
                total_games=result.total_games,
                errors=result.errors
            )
            
        except SteamGamesServiceError as e:
            logger.error(f"Steam games service error during import for user {user_id}: {str(e)}")
            raise ValueError(str(e))
        except Exception as e:
            logger.error(f"Unexpected error during Steam library import for user {user_id}: {str(e)}")
            raise ValueError("Failed to import Steam library")
    
    async def list_games(self, 
                        user_id: str, 
                        offset: int = 0,
                        limit: int = 100,
                        status_filter: Optional[str] = None,
                        search: Optional[str] = None) -> Tuple[List[ImportGame], int]:
        """List imported Steam games with filtering and pagination."""
        
        # Build base query - no need for complex joins, we'll check sync status separately
        query = select(SteamGame).where(SteamGame.user_id == user_id)
        
        # Apply status filter (note: sync status will be checked post-query using new function)
        if status_filter:
            if status_filter == "unmatched":
                query = query.where(and_(SteamGame.igdb_id.is_(None), not SteamGame.ignored))
            elif status_filter == "matched":
                # Matched but not synced: has igdb_id but we'll filter for non-synced in Python
                query = query.where(and_(SteamGame.igdb_id.isnot(None), not SteamGame.ignored))
            elif status_filter == "ignored":
                query = query.where(SteamGame.ignored)
            elif status_filter == "synced":
                # Synced games: we'll filter these post-query using is_steam_game_synced
                query = query.where(and_(SteamGame.igdb_id.isnot(None), not SteamGame.ignored))
        
        # Apply search filter
        if search:
            search_term = f"%{search.strip().lower()}%"
            query = query.where(func.lower(SteamGame.game_name).contains(search_term))
        
        # Get total count with proper joins for synced games
        # For sync status, we'll need to count after filtering, so we build a base count query
        count_query = select(func.count(SteamGame.id)).where(SteamGame.user_id == user_id)
        if status_filter:
            if status_filter == "unmatched":
                count_query = count_query.where(and_(SteamGame.igdb_id.is_(None), not SteamGame.ignored))
            elif status_filter == "matched":
                count_query = count_query.where(and_(SteamGame.igdb_id.isnot(None), not SteamGame.ignored))
            elif status_filter == "ignored":
                count_query = count_query.where(SteamGame.ignored)
            elif status_filter == "synced":
                count_query = count_query.where(and_(SteamGame.igdb_id.isnot(None), not SteamGame.ignored))

        if search:
            search_term = f"%{search.strip().lower()}%"
            count_query = count_query.where(func.lower(SteamGame.game_name).contains(search_term))

        # Note: For synced status, we'll need to recount after filtering, but for performance
        # we'll do a rough count here and adjust if needed
        total = self.session.exec(count_query).first() or 0
        
        # Apply pagination and ordering
        query = query.order_by(SteamGame.game_name.asc()).offset(offset).limit(limit)
        results = self.session.exec(query).all()
        
        # Convert to ImportGame format with sync status filtering
        games = []
        filtered_count = 0

        for steam_game in results:
            # Check sync status using new function
            is_synced = is_steam_game_synced(self.session, user_id, steam_game.igdb_id) if steam_game.igdb_id else False

            # Apply post-query filtering for sync status
            if status_filter == "synced" and not is_synced:
                continue
            elif status_filter == "matched" and is_synced:
                continue  # "matched" means matched but NOT synced

            # Get user_game_id if synced
            user_game_id = None
            if is_synced and steam_game.igdb_id:
                user_game_query = select(UserGame.id).where(
                    and_(
                        UserGame.game_id == steam_game.igdb_id,
                        UserGame.user_id == user_id
                    )
                )
                user_game_result = self.session.exec(user_game_query).first()
                user_game_id = user_game_result if user_game_result else None

            games.append(ImportGame(
                id=steam_game.id,
                external_id=str(steam_game.steam_appid),
                name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                igdb_title=steam_game.igdb_title,
                user_game_id=user_game_id,
                is_synced=is_synced,
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            ))
            filtered_count += 1

        # Adjust total count for sync filtering
        if status_filter in ["synced", "matched"]:
            total = filtered_count
        
        return games, total
    
    async def match_game(self, user_id: str, game_id: str, igdb_id: Optional[int]) -> ImportGame:
        """Match Steam game to IGDB entry."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            steam_game, message = await steam_games_service.match_steam_game_to_igdb(
                steam_game_id=game_id,
                igdb_id=igdb_id,
                user_id=user_id
            )

            # Get user_game_id if synced using new sync function
            user_game_id = None
            if steam_game.igdb_id:
                is_synced = is_steam_game_synced(self.session, user_id, steam_game.igdb_id)
                if is_synced:
                    user_game_result = self.session.exec(
                        select(UserGame.id).where(
                            and_(
                                UserGame.game_id == steam_game.igdb_id,  # UserGame.game_id is the IGDB ID
                                UserGame.user_id == user_id
                            )
                        )
                    ).first()
                    user_game_id = user_game_result

            return ImportGame(
                id=steam_game.id,
                external_id=str(steam_game.steam_appid),
                name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                igdb_title=steam_game.igdb_title,
                user_game_id=user_game_id,
                is_synced=is_steam_game_synced(self.session, user_id, steam_game.igdb_id) if steam_game.igdb_id else False,
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            )
            
        except SteamGamesServiceError as e:
            self._handle_steam_service_error(e)
    
    async def auto_match_game(self, user_id: str, game_id: str) -> MatchResult:
        """Automatically match single Steam game to IGDB."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            result = await steam_games_service.auto_match_single_steam_game(
                steam_game_id=game_id,
                user_id=user_id
            )
            
            return MatchResult(
                game_id=game_id,
                game_name=result.steam_game_name,
                matched=result.matched,
                igdb_id=result.igdb_id if result.matched else None,
                confidence_score=result.confidence_score,
                error_message=result.error_message
            )
            
        except SteamGamesServiceError as e:
            raise ValueError(str(e))
    
    async def auto_match_all_games(self, user_id: str) -> BulkOperationResult:
        """Automatically match all unmatched Steam games to IGDB."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            results = await steam_games_service.retry_auto_matching_for_unmatched_games(user_id)
            
            return BulkOperationResult(
                total_processed=results.total_processed,
                successful_operations=results.successful_matches,
                failed_operations=results.failed_matches,
                skipped_items=results.skipped_games,
                errors=results.errors
            )
            
        except SteamGamesServiceError as e:
            raise ValueError(str(e))
    
    async def sync_game(self, user_id: str, game_id: str) -> SyncResult:
        """Sync Steam game to main collection."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            result = await steam_games_service.sync_steam_game_to_collection(
                steam_game_id=game_id,
                user_id=user_id
            )
            
            return SyncResult(
                steam_game_id=game_id,
                steam_game_name=result.steam_game_name,
                user_game_id=result.user_game_id,
                action=result.action,
                error_message=result.error_message if result.action == "failed" else None
            )
            
        except SteamGamesServiceError as e:
            self._handle_steam_service_error(e)
    
    async def sync_all_games(self, user_id: str) -> BulkOperationResult:
        """Sync all matched Steam games to main collection."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            results = await steam_games_service.sync_all_matched_games(user_id)
            
            return BulkOperationResult(
                total_processed=results.total_processed,
                successful_operations=results.successful_syncs,
                failed_operations=results.failed_syncs,
                skipped_items=results.skipped_games,
                errors=results.errors
            )
            
        except SteamGamesServiceError as e:
            raise ValueError(str(e))
    
    async def unsync_game(self, user_id: str, game_id: str) -> ImportGame:
        """Remove Steam game from main collection but keep import record."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            steam_game, message = await steam_games_service.unsync_steam_game_from_collection(
                steam_game_id=game_id,
                user_id=user_id
            )
            
            return ImportGame(
                id=steam_game.id,
                external_id=str(steam_game.steam_appid),
                name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                igdb_title=steam_game.igdb_title,
                user_game_id=None,  # Always None after unsync
                is_synced=False,  # Always False after unsync
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            )
            
        except SteamGamesServiceError as e:
            raise ValueError(str(e))
    
    async def unsync_all_games(self, user_id: str) -> BulkOperationResult:
        """Remove all synced Steam games from main collection."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            results = await steam_games_service.unsync_all_synced_games(user_id)
            
            return BulkOperationResult(
                total_processed=results.total_processed,
                successful_operations=results.successful_unsyncs,
                failed_operations=results.failed_unsyncs,
                errors=results.errors
            )
            
        except SteamGamesServiceError as e:
            raise ValueError(str(e))
    
    async def ignore_game(self, user_id: str, game_id: str) -> ImportGame:
        """Toggle ignore status of Steam game."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            steam_game, message, ignored_status = steam_games_service.toggle_steam_game_ignored(
                steam_game_id=game_id,
                user_id=user_id
            )
            
            # Get user_game_id if synced using new sync function
            user_game_id = None
            if steam_game.igdb_id:
                is_synced = is_steam_game_synced(self.session, user_id, steam_game.igdb_id)
                if is_synced:
                    user_game_result = self.session.exec(
                        select(UserGame.id).where(
                            and_(
                                UserGame.game_id == steam_game.igdb_id,  # UserGame.game_id is the IGDB ID
                                UserGame.user_id == user_id
                            )
                        )
                    ).first()
                    user_game_id = user_game_result
            
            return ImportGame(
                id=steam_game.id,
                external_id=str(steam_game.steam_appid),
                name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                igdb_title=steam_game.igdb_title,
                user_game_id=user_game_id,
                is_synced=is_steam_game_synced(self.session, user_id, steam_game.igdb_id) if steam_game.igdb_id else False,
                ignored=steam_game.ignored,
                created_at=steam_game.created_at,
                updated_at=steam_game.updated_at
            )
            
        except SteamGamesServiceError as e:
            self._handle_steam_service_error(e)
    
    async def unignore_all_games(self, user_id: str) -> BulkOperationResult:
        """Unignore all ignored Steam games."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            results = await steam_games_service.unignore_all_steam_games(user_id)
            
            return BulkOperationResult(
                total_processed=results.total_processed,
                successful_operations=results.successful_unignores,
                failed_operations=results.failed_unignores,
                errors=results.errors
            )
            
        except SteamGamesServiceError as e:
            raise ValueError(str(e))
    
    async def unmatch_all_games(self, user_id: str) -> BulkOperationResult:
        """Remove IGDB matches from all matched Steam games."""
        try:
            steam_games_service = create_steam_games_service(self.session, self.igdb_service)
            results = await steam_games_service.unmatch_all_matched_games(user_id)
            
            return BulkOperationResult(
                total_processed=results.total_processed,
                successful_operations=results.successful_unmatches,
                failed_operations=results.failed_unmatches,
                errors=results.errors
            )
            
        except SteamGamesServiceError as e:
            raise ValueError(str(e))


def create_steam_import_service(session: Session, igdb_service: Optional[IGDBService] = None) -> SteamImportService:
    """Factory function to create Steam import service."""
    return SteamImportService(session, igdb_service)
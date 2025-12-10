"""
Steam games service module.

Main facade for all Steam games operations.
"""

import logging
from typing import Dict, Any, List, Tuple, Optional

from sqlmodel import Session, select

from app.models.steam_game import SteamGame
from app.services.steam import SteamService
from app.services.igdb import IGDBService
from app.services.sync_utils import is_steam_game_synced
from app.core.config import settings

from .models import (
    ImportResult,
    AutoMatchResults,
    SyncResult,
    BulkSyncResults,
    BulkUnignoreResults,
    BulkUnmatchResults,
    BulkUnsyncResults,
    SteamGamesServiceError
)
from .import_ops import import_steam_library
from .matching_ops import (
    auto_match_steam_games,
    auto_match_single_steam_game,
    retry_auto_matching_for_unmatched_games,
    match_steam_game_to_igdb
)
from .sync_ops import (
    sync_steam_game_to_collection,
    sync_all_matched_games,
    unsync_steam_game_from_collection,
    unsync_steam_game_from_collection_internal
)
from .bulk_ops import (
    toggle_steam_game_ignored,
    unignore_all_steam_games,
    unmatch_all_matched_games,
    unsync_all_synced_games
)


logger = logging.getLogger(__name__)


class SteamGamesService:
    """
    Service for managing Steam games import, matching, and collection sync.

    Provides a high-level interface for:
    - Importing user's Steam library
    - Automatic and manual IGDB game matching
    - Syncing matched Steam games to user's main collection
    - Managing Steam game status (ignored, unsynced, etc.)
    """

    def __init__(
        self,
        session: Session,
        steam_service: Optional[SteamService] = None,
        igdb_service: Optional[IGDBService] = None,
        auto_match_confidence_threshold: float = 0.60,
        auto_match_batch_size: int = 10
    ):
        """
        Initialize SteamGamesService.

        Args:
            session: Database session
            steam_service: Steam API service instance (required for import operations)
            igdb_service: IGDB API service instance (lazy-initialized if not provided)
            auto_match_confidence_threshold: Minimum confidence score for auto-matching (default 0.60)
            auto_match_batch_size: Batch size for auto-matching operations (default 10)

        Note:
            steam_service can be None if only non-import operations are needed.
            Import operations will fail if steam_service is None.
            igdb_service is lazy-initialized on first access if not provided.
        """
        self.session = session
        self.steam_service = steam_service
        self._igdb_service_instance = igdb_service
        self.auto_match_confidence_threshold = auto_match_confidence_threshold
        self.auto_match_batch_size = auto_match_batch_size

        # Backwards compatibility aliases (underscore-prefixed)
        self._steam_service = steam_service

    @property
    def igdb_service(self) -> IGDBService:
        """Get or lazily initialize the IGDB service."""
        if self._igdb_service_instance is None:
            self._igdb_service_instance = IGDBService()
        return self._igdb_service_instance

    @property
    def _igdb_service(self) -> Optional[IGDBService]:
        """Backwards compatibility alias for _igdb_service."""
        return self._igdb_service_instance

    # ==================
    # Import Operations
    # ==================

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
        if self.steam_service is None:
            raise SteamGamesServiceError("Steam service not configured - cannot import Steam library")

        return await import_steam_library(
            self.session,
            user_id,
            steam_config,
            enable_auto_matching,
            self.steam_service,
            self.igdb_service,
            self._auto_match_steam_games
        )

    # ==================
    # Matching Operations
    # ==================

    async def _auto_match_steam_games(self, steam_game_ids: List[str]) -> AutoMatchResults:
        """
        Internal method for automatic IGDB matching of Steam games.

        Args:
            steam_game_ids: List of Steam game IDs to match

        Returns:
            AutoMatchResults with matching results
        """
        return await auto_match_steam_games(
            self.session,
            steam_game_ids,
            self.igdb_service,
            self.auto_match_confidence_threshold,
            self.auto_match_batch_size
        )

    async def auto_match_single_steam_game(self, steam_game_id: str, user_id: Optional[str] = None):
        """
        Attempt to automatically match a single Steam game to IGDB.

        Args:
            steam_game_id: Steam game ID to match
            user_id: Optional user ID (for backwards compatibility, not used)

        Returns:
            AutoMatchResult with matching details
        """
        return await auto_match_single_steam_game(
            self.session,
            steam_game_id,
            self.igdb_service,
            self.auto_match_confidence_threshold
        )

    # Backwards compatibility alias
    async def _auto_match_single_steam_game(self, steam_game_id: str, user_id: Optional[str] = None):
        """Backwards compatibility alias for auto_match_single_steam_game."""
        return await self.auto_match_single_steam_game(steam_game_id, user_id)

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
        return await retry_auto_matching_for_unmatched_games(
            self.session,
            user_id,
            self.igdb_service,
            self.auto_match_confidence_threshold,
            self.auto_match_batch_size
        )

    async def match_steam_game_to_igdb(
        self,
        steam_game_id: str,
        igdb_id: Optional[int],
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
        return await match_steam_game_to_igdb(
            self.session,
            steam_game_id,
            igdb_id,
            user_id,
            self.igdb_service,
            self._unsync_steam_game_from_collection_internal
        )

    # ==================
    # Sync Operations
    # ==================

    async def sync_steam_game_to_collection(self, steam_game_id: str, user_id: str) -> SyncResult:
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
        return await sync_steam_game_to_collection(
            self.session,
            steam_game_id,
            user_id,
            self.igdb_service
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
        return await sync_all_matched_games(self.session, user_id, self.igdb_service)

    async def unsync_steam_game_from_collection(self, steam_game_id: str, user_id: str) -> Tuple[SteamGame, str]:
        """
        Unsync a single Steam game from the user's collection.

        This removes the Steam platform/storefront from the UserGame.

        Args:
            steam_game_id: Steam game ID to unsync
            user_id: User ID for authorization

        Returns:
            Tuple of (updated SteamGame, status message)

        Raises:
            SteamGamesServiceError: For service errors
        """
        return await unsync_steam_game_from_collection(self.session, steam_game_id, user_id)

    async def _unsync_steam_game_from_collection_internal(self, game_id: int, user_id: str) -> str:
        """
        Internal method to unsync a Steam game from collection.

        Args:
            game_id: Game ID that was synced
            user_id: User ID for authorization

        Returns:
            str: Type of removal performed ("complete", "platform_only", "not_found")
        """
        return await unsync_steam_game_from_collection_internal(self.session, game_id, user_id)

    # ==================
    # Bulk Operations
    # ==================

    def toggle_steam_game_ignored(self, steam_game_id: str, user_id: str) -> Tuple[SteamGame, str, bool]:
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
        return toggle_steam_game_ignored(self.session, steam_game_id, user_id)

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
        return await unignore_all_steam_games(self.session, user_id)

    async def unmatch_all_matched_games(self, user_id: str) -> BulkUnmatchResults:
        """
        Unmatch all matched (but not synced) Steam games for a user.

        Finds games with igdb_id that are not synced to collection and clears their igdb_id.
        This only affects games that are matched but haven't been synced yet.

        Args:
            user_id: User ID for authorization

        Returns:
            BulkUnmatchResults with detailed unmatch statistics

        Raises:
            SteamGamesServiceError: For service errors
        """
        return await unmatch_all_matched_games(self.session, user_id)

    async def unsync_all_synced_games(self, user_id: str) -> BulkUnsyncResults:
        """
        Unsync all synced Steam games for a user.

        Finds games synced to collection and:
        1. Removes Steam platform/storefront from UserGame
        2. If no other platforms, removes the entire UserGame
        3. Keeps igdb_id in SteamGame (returns to matched state)

        Args:
            user_id: User ID for authorization

        Returns:
            BulkUnsyncResults with detailed unsync statistics

        Raises:
            SteamGamesServiceError: For service errors
        """
        return await unsync_all_synced_games(self.session, user_id)

    # ==================
    # Query Operations
    # ==================

    def get_steam_games_for_user(
        self,
        user_id: str,
        include_ignored: bool = False,
        include_synced: bool = True,
        include_matched: bool = True,
        include_unmatched: bool = True
    ) -> List[Dict[str, Any]]:
        """
        Get Steam games for a user with optional filters.

        Args:
            user_id: User ID to get games for
            include_ignored: Include ignored games
            include_synced: Include synced games (games in collection)
            include_matched: Include matched but not synced games
            include_unmatched: Include unmatched games

        Returns:
            List of Steam game dictionaries with computed status fields
        """
        # Build base query
        query = select(SteamGame).where(SteamGame.user_id == user_id)

        if not include_ignored:
            query = query.where(not SteamGame.ignored)

        steam_games = self.session.exec(query).all()

        # Process each game to add computed status fields
        result = []
        for steam_game in steam_games:
            # Determine sync status using new function
            is_synced = False
            if steam_game.igdb_id:
                is_synced = is_steam_game_synced(self.session, user_id, steam_game.igdb_id)

            # Apply filters based on computed status
            is_matched = steam_game.igdb_id is not None

            if is_synced and not include_synced:
                continue
            if is_matched and not is_synced and not include_matched:
                continue
            if not is_matched and not include_unmatched:
                continue

            # Build response dictionary
            game_dict = {
                "id": steam_game.id,
                "steam_appid": steam_game.steam_appid,
                "game_name": steam_game.game_name,
                "igdb_id": steam_game.igdb_id,
                "igdb_title": steam_game.igdb_title,
                "ignored": steam_game.ignored,
                "created_at": steam_game.created_at.isoformat() if steam_game.created_at else None,
                "updated_at": steam_game.updated_at.isoformat() if steam_game.updated_at else None,
                # Computed status fields
                "is_matched": is_matched,
                "is_synced": is_synced
            }
            result.append(game_dict)

        return result

    def get_steam_game_counts(self, user_id: str) -> Dict[str, int]:
        """
        Get counts of Steam games by status for a user.

        Args:
            user_id: User ID to get counts for

        Returns:
            Dictionary with counts: total, unmatched, matched, synced, ignored
        """
        # Get all Steam games for user
        all_games = self.session.exec(
            select(SteamGame).where(SteamGame.user_id == user_id)
        ).all()

        total = len(all_games)
        unmatched = 0
        matched = 0
        synced = 0
        ignored = 0

        for game in all_games:
            if game.ignored:
                ignored += 1
            elif not game.igdb_id:
                unmatched += 1
            else:
                # Has igdb_id - check if synced
                is_synced = is_steam_game_synced(self.session, user_id, game.igdb_id)
                if is_synced:
                    synced += 1
                else:
                    matched += 1

        return {
            "total": total,
            "unmatched": unmatched,
            "matched": matched,
            "synced": synced,
            "ignored": ignored
        }


def create_steam_games_service(
    session: Session,
    steam_service: Optional[SteamService] = None,
    igdb_service: Optional[IGDBService] = None
) -> SteamGamesService:
    """
    Factory function to create SteamGamesService with default dependencies.

    Args:
        session: Database session
        steam_service: Optional Steam service instance (required for import operations)
        igdb_service: Optional IGDB service instance (created if not provided)

    Returns:
        Configured SteamGamesService instance

    Note:
        steam_service must be provided for Steam library import operations,
        but is not required for other operations like matching and syncing.
    """
    # SteamService requires api_key, so we can't create a default one
    # steam_service can be None if only non-import operations are needed

    if igdb_service is None:
        igdb_service = IGDBService()

    return SteamGamesService(
        session=session,
        steam_service=steam_service,
        igdb_service=igdb_service,
        auto_match_confidence_threshold=getattr(settings, 'auto_match_confidence_threshold', 0.80),
        auto_match_batch_size=getattr(settings, 'auto_match_batch_size', 10)
    )

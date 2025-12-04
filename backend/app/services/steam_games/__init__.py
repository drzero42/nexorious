"""
Steam games service package.

This package provides Steam library import, IGDB matching,
and collection synchronization functionality.

Re-exports for backwards compatibility:
    - SteamGamesService: Main service class
    - create_steam_games_service: Factory function
    - Result dataclasses for import, matching, and sync operations
    - SteamGamesServiceError: Service exception class
"""

from .models import (
    AutoMatchResult,
    AutoMatchResults,
    ImportResult,
    SyncResult,
    BulkSyncResults,
    BulkUnignoreResults,
    BulkUnmatchResults,
    BulkUnsyncResults,
    SteamGamesServiceError,
)
from .service import SteamGamesService, create_steam_games_service

__all__ = [
    # Main service
    "SteamGamesService",
    # Factory function
    "create_steam_games_service",
    # Result models
    "AutoMatchResult",
    "AutoMatchResults",
    "ImportResult",
    "SyncResult",
    "BulkSyncResults",
    "BulkUnignoreResults",
    "BulkUnmatchResults",
    "BulkUnsyncResults",
    # Exceptions
    "SteamGamesServiceError",
]

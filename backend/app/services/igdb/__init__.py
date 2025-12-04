"""
IGDB service package.

This package provides the IGDB API integration for game metadata retrieval.

Re-exports for backwards compatibility:
    - IGDBService: Main service class for IGDB API interactions
    - GameMetadata: Structured game metadata dataclass
    - IGDBError: Exception for IGDB API errors
    - TwitchAuthError: Exception for Twitch authentication errors
    - IGDB_PLATFORM_MAPPING: Platform ID to internal name mapping
    - KEYWORD_EXPANSIONS: Search query keyword expansions
    - map_igdb_time_to_beat_to_db_fields: Utility function for time-to-beat mapping
"""

from .models import (
    GameMetadata,
    IGDBError,
    TwitchAuthError,
    IGDB_PLATFORM_MAPPING,
    KEYWORD_EXPANSIONS,
    map_igdb_time_to_beat_to_db_fields,
)
from .service import IGDBService

__all__ = [
    # Main service
    "IGDBService",
    # Models
    "GameMetadata",
    # Exceptions
    "IGDBError",
    "TwitchAuthError",
    # Constants
    "IGDB_PLATFORM_MAPPING",
    "KEYWORD_EXPANSIONS",
    # Utility functions
    "map_igdb_time_to_beat_to_db_fields",
]

"""
IGDB models and constants.

This module contains the GameMetadata dataclass, exception classes,
and platform/keyword mappings used by the IGDB service.
"""

from dataclasses import dataclass
from typing import Optional, List, Dict, Any


# IGDB Platform ID to internal platform name mapping
# Based on IGDB platform IDs from their API documentation
IGDB_PLATFORM_MAPPING = {
    6: "pc-windows",         # PC (Microsoft Windows)
    3: "pc-windows",         # Linux (map to PC Windows for simplicity)
    14: "pc-windows",        # Mac (map to PC Windows for simplicity)
    48: "playstation-4",     # PlayStation 4
    167: "playstation-5",    # PlayStation 5
    9: "playstation-3",      # PlayStation 3
    49: "xbox-one",          # Xbox One
    169: "xbox-series",      # Xbox Series X|S
    12: "xbox-360",          # Xbox 360
    130: "nintendo-switch",  # Nintendo Switch
    5: "nintendo-wii",       # Nintendo Wii
    39: "ios",               # iOS
    34: "android",           # Android
}

# Keyword expansions for search query enhancement
KEYWORD_EXPANSIONS = {
    "goty": "Game of the Year",
    "The Telltale Series": "",  # Remove this phrase from queries
    "®": "",  # Remove registered trademark symbol
    "(classic)": "",  # Remove (classic) from queries (case insensitive)
    ":": " ",  # Replace colon with space
    # Pattern-based keywords (special keys that trigger regex patterns)
    "_pattern_year_parentheses": "",  # Remove years in parentheses like (2023)
    "_pattern_standalone_one": "",   # Remove standalone number "1" (avoiding version numbers and episodes)
}


@dataclass
class GameMetadata:
    """Structured game metadata from IGDB."""

    igdb_id: int
    title: str
    igdb_slug: Optional[str] = None
    description: Optional[str] = None
    genre: Optional[str] = None
    developer: Optional[str] = None
    publisher: Optional[str] = None
    release_date: Optional[str] = None
    cover_art_url: Optional[str] = None
    rating_average: Optional[float] = None
    rating_count: Optional[int] = None
    estimated_playtime_hours: Optional[int] = None
    # How Long to Beat data from IGDB (hastily, normally, completely) - stored in hours
    hastily: Optional[int] = None
    normally: Optional[int] = None
    completely: Optional[int] = None
    # Platform data from IGDB
    igdb_platform_ids: Optional[List[int]] = None
    platform_names: Optional[List[str]] = None


class TwitchAuthError(Exception):
    """Exception for Twitch authentication errors."""
    pass


class IGDBError(Exception):
    """Exception for IGDB API errors."""
    pass


def map_igdb_time_to_beat_to_db_fields(igdb_time_data: Dict[str, Any]) -> Dict[str, Optional[int]]:
    """Map IGDB time-to-beat fields to our database fields.

    Note: This function expects igdb_time_data to already be converted to hours.
    The conversion from IGDB's seconds to hours should happen in _get_time_to_beat_data().
    """
    return {
        "howlongtobeat_main": igdb_time_data.get("hastily"),
        "howlongtobeat_extra": igdb_time_data.get("normally"),
        "howlongtobeat_completionist": igdb_time_data.get("completely")
    }

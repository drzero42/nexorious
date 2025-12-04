"""
Steam games models and dataclasses.

This module contains all dataclasses used by the Steam games service
for result types and the service exception class.
"""

from dataclasses import dataclass
from typing import Optional, List


@dataclass
class AutoMatchResult:
    """Result of automatic IGDB matching for a single Steam game."""
    steam_game_id: str
    steam_game_name: str
    steam_appid: int
    matched: bool
    igdb_id: Optional[int] = None
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

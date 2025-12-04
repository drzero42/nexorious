"""
Abstract base classes for import source services.
"""

from abc import ABC, abstractmethod
from typing import Any, Dict, List, Optional, Tuple
from dataclasses import dataclass
from datetime import datetime


@dataclass
class ImportSourceConfig:
    """Base configuration for import sources."""
    source_name: str
    is_configured: bool
    is_verified: bool
    configured_at: Optional[datetime] = None
    last_import: Optional[datetime] = None
    config_data: Optional[Dict[str, Any]] = None  # Source-specific config data


@dataclass
class ImportGame:
    """Base import game representation."""
    id: str
    external_id: str  # Steam AppID, Epic ID, GOG ID, etc.
    name: str
    igdb_id: Optional[int] = None
    igdb_title: Optional[str] = None
    user_game_id: Optional[str] = None  # ID in user_games table when synced
    game_id: Optional[int] = None  # ID of the game in the games table (IGDB ID)
    is_synced: bool = False  # Whether the game is synced to user collection
    ignored: bool = False
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
    
    # Platform resolution fields (primarily used by Darkadia import)
    platform_resolved: Optional[bool] = None
    original_platform_name: Optional[str] = None
    platform_resolution_status: Optional[str] = None
    platform_name: Optional[str] = None  # Resolved/mapped platform name
    
    # Storefront resolution fields (primarily used by Darkadia import)
    original_storefront_name: Optional[str] = None
    storefront_resolution_status: Optional[str] = None
    storefront_name: Optional[str] = None  # Resolved/mapped storefront name


@dataclass
class ImportResult:
    """Result of library import operation."""
    imported_count: int
    skipped_count: int
    auto_matched_count: int
    total_games: int
    errors: List[str]
    job_id: Optional[str] = None


@dataclass
class SyncResult:
    """Result of game sync operation."""
    steam_game_id: str
    steam_game_name: str
    user_game_id: Optional[str]
    action: str  # "created_new", "updated_existing", "failed"
    error_message: Optional[str] = None


@dataclass
class BulkOperationResult:
    """Result of bulk operations (sync, unmatch, etc.)."""
    total_processed: int
    successful_operations: int
    failed_operations: int
    skipped_items: int = 0
    errors: Optional[List[str]] = None
    results: Optional[List[Any]] = None  # Specific operation results

    def __post_init__(self):
        if self.errors is None:
            self.errors = []
        if self.results is None:
            self.results = []


@dataclass
class MatchResult:
    """Result of IGDB matching operation."""
    game_id: str
    game_name: str
    matched: bool
    igdb_id: Optional[int] = None
    igdb_title: Optional[str] = None
    confidence_score: Optional[float] = None
    error_message: Optional[str] = None


class ImportSourceService(ABC):
    """Abstract base class for import source services."""
    
    def __init__(self, source_name: str):
        self.source_name = source_name
    
    @abstractmethod
    async def get_config(self, user_id: str) -> ImportSourceConfig:
        """Get source configuration for user."""
        pass
    
    @abstractmethod
    async def set_config(self, user_id: str, config_data: Dict[str, Any]) -> ImportSourceConfig:
        """Set/update source configuration."""
        pass
    
    @abstractmethod
    async def delete_config(self, user_id: str) -> bool:
        """Delete source configuration."""
        pass
    
    @abstractmethod
    async def verify_config(self, config_data: Dict[str, Any]) -> Tuple[bool, Optional[str], Optional[Dict[str, Any]]]:
        """
        Verify configuration without saving.
        Returns: (is_valid, error_message, additional_data)
        """
        pass
    
    @abstractmethod
    async def get_library_preview(self, user_id: str) -> Dict[str, Any]:
        """Get preview of user's library from the source."""
        pass
    
    @abstractmethod
    async def import_library(self, user_id: str) -> ImportResult:
        """Import user's library from the source."""
        pass
    
    @abstractmethod
    async def list_games(self, 
                        user_id: str, 
                        offset: int = 0,
                        limit: int = 100,
                        status_filter: Optional[str] = None,
                        search: Optional[str] = None) -> Tuple[List[ImportGame], int]:
        """
        List imported games with filtering and pagination.
        Returns: (games, total_count)
        """
        pass
    
    @abstractmethod
    async def match_game(self, user_id: str, game_id: str, igdb_id: Optional[int]) -> ImportGame:
        """Match game to IGDB entry."""
        pass
    
    @abstractmethod
    async def auto_match_game(self, user_id: str, game_id: str) -> MatchResult:
        """Automatically match single game to IGDB."""
        pass
    
    @abstractmethod
    async def auto_match_all_games(self, user_id: str) -> BulkOperationResult:
        """Automatically match all unmatched games to IGDB."""
        pass
    
    @abstractmethod
    async def sync_game(self, user_id: str, game_id: str) -> SyncResult:
        """Sync game to main collection."""
        pass
    
    @abstractmethod
    async def sync_all_games(self, user_id: str) -> BulkOperationResult:
        """Sync all matched games to main collection."""
        pass
    
    @abstractmethod
    async def unsync_game(self, user_id: str, game_id: str) -> ImportGame:
        """Remove game from main collection but keep import record."""
        pass
    
    @abstractmethod
    async def unsync_all_games(self, user_id: str) -> BulkOperationResult:
        """Remove all synced games from main collection."""
        pass
    
    @abstractmethod
    async def ignore_game(self, user_id: str, game_id: str) -> ImportGame:
        """Toggle ignore status of game."""
        pass
    
    @abstractmethod
    async def unignore_all_games(self, user_id: str) -> BulkOperationResult:
        """Unignore all ignored games."""
        pass
    
    @abstractmethod
    async def unmatch_all_games(self, user_id: str) -> BulkOperationResult:
        """Remove IGDB matches from all matched games."""
        pass
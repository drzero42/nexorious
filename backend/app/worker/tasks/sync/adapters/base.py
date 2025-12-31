"""Base classes and protocols for sync source adapters.

This module provides the abstraction layer for fetching games from
external services (Steam, Epic, GOG, etc.) in a uniform format.
"""

from dataclasses import dataclass
from typing import Protocol, Optional, List, Dict, Any

from app.models.user import User
from app.models.job import BackgroundJobSource


@dataclass
class ExternalGame:
    """Standardized representation of a game from an external source.

    Attributes:
        external_id: Unique identifier from the source (e.g., Steam AppID)
        title: Game name from the source
        platform: Platform identifier (e.g., "pc-windows")
        storefront: Storefront identifier (e.g., "steam")
        metadata: Source-specific data (playtime, achievements, etc.)
    """
    external_id: str
    title: str
    platform: str
    storefront: str
    metadata: Dict[str, Any]


class SyncSourceAdapter(Protocol):
    """Protocol for sync source adapters.

    Each external service (Steam, Epic, GOG) implements this protocol
    to provide a uniform interface for fetching game libraries.
    """

    source: BackgroundJobSource

    async def fetch_games(self, user: User) -> List[ExternalGame]:
        """Fetch all games from external source for user.

        Args:
            user: The user whose games to fetch

        Returns:
            List of ExternalGame objects

        Raises:
            ValueError: If credentials are not configured
            Exception: On API errors
        """
        ...

    def get_credentials(self, user: User) -> Optional[Dict[str, str]]:
        """Extract credentials from user preferences.

        Args:
            user: The user whose credentials to extract

        Returns:
            Dictionary with credential keys, or None if not configured
        """
        ...

    def is_configured(self, user: User) -> bool:
        """Check if user has valid credentials for this source.

        Args:
            user: The user to check

        Returns:
            True if credentials are configured and verified
        """
        ...


def get_sync_adapter(source: str) -> SyncSourceAdapter:
    """Get the appropriate adapter for a sync source.

    Args:
        source: Source identifier ("steam", "epic", "gog")

    Returns:
        Adapter instance for the specified source

    Raises:
        ValueError: If source is not supported
    """
    from .steam import SteamSyncAdapter

    adapters = {
        "steam": SteamSyncAdapter,
    }

    adapter_class = adapters.get(source.lower())
    if not adapter_class:
        raise ValueError(f"Unsupported sync source: {source}")

    return adapter_class()

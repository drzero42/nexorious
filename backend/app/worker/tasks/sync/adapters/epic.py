"""Epic Games Store sync adapter for fetching user's Epic library.

Implements SyncSourceAdapter protocol to fetch games from Epic Games Store
and convert them to the standardized ExternalGame format.
"""

import logging
from typing import Optional, List, Dict, Any

from app.models.user import User
from app.models.job import BackgroundJobSource
from app.services.epic import EpicService
from .base import ExternalGame

logger = logging.getLogger(__name__)


class EpicSyncAdapter:
    """Adapter for syncing games from Epic Games Store.

    Fetches the user's Epic library and converts games to
    ExternalGame format for generic processing.
    """

    source = BackgroundJobSource.EPIC

    async def fetch_games(self, user: User) -> List[ExternalGame]:
        """Fetch all games from user's Epic library.

        Args:
            user: The user whose Epic library to fetch

        Returns:
            List of ExternalGame objects

        Raises:
            ValueError: If Epic credentials are not configured
            EpicAuthExpiredError: If Epic authentication has expired
        """
        credentials = self.get_credentials(user)
        if not credentials:
            raise ValueError("Epic credentials not configured for this user")

        epic_service = EpicService(user_id=user.id)
        epic_games = await epic_service.get_library()

        logger.info(f"Fetched {len(epic_games)} games from Epic for user {user.id}")

        return [
            ExternalGame(
                external_id=game.app_name,
                title=game.title,
                platform="pc-windows",
                storefront="epic",
                metadata={
                    "app_name": game.app_name,
                },
                playtime_hours=0,
            )
            for game in epic_games
        ]

    def get_credentials(self, user: User) -> Optional[Dict[str, Any]]:
        """Extract Epic credentials from user preferences.

        Args:
            user: The user whose credentials to extract

        Returns:
            Dictionary with Epic config, or None if not configured
        """
        preferences = user.preferences or {}
        epic_config = preferences.get("epic", {})

        is_verified = epic_config.get("is_verified", False)

        if not is_verified:
            return None

        return epic_config

    def is_configured(self, user: User) -> bool:
        """Check if user has verified Epic credentials.

        Args:
            user: The user to check

        Returns:
            True if Epic credentials are configured and verified
        """
        return self.get_credentials(user) is not None

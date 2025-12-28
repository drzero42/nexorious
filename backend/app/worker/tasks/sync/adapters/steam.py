"""Steam sync adapter for fetching user's Steam library.

Implements SyncSourceAdapter protocol to fetch games from Steam
and convert them to the standardized ExternalGame format.
"""

import logging
from typing import Optional, List, Dict

from app.models.user import User
from app.models.job import BackgroundJobSource
from app.services.steam import SteamService
from .base import ExternalGame

logger = logging.getLogger(__name__)


class SteamSyncAdapter:
    """Adapter for syncing games from Steam.

    Fetches the user's Steam library and converts games to
    ExternalGame format for generic processing.
    """

    source = BackgroundJobSource.STEAM

    async def fetch_games(self, user: User) -> List[ExternalGame]:
        """Fetch all games from user's Steam library.

        Args:
            user: The user whose Steam library to fetch

        Returns:
            List of ExternalGame objects

        Raises:
            ValueError: If Steam credentials are not configured
        """
        credentials = self.get_credentials(user)
        if not credentials:
            raise ValueError("Steam credentials not configured for this user")

        steam_service = SteamService(api_key=credentials["api_key"])
        steam_games = await steam_service.get_owned_games(credentials["steam_id"])

        logger.info(f"Fetched {len(steam_games)} games from Steam for user {user.id}")

        return [
            ExternalGame(
                external_id=str(game.appid),
                title=game.name,
                platform_id="pc-windows",
                storefront_id="steam",
                metadata={
                    "appid": game.appid,
                },
            )
            for game in steam_games
        ]

    def get_credentials(self, user: User) -> Optional[Dict[str, str]]:
        """Extract Steam credentials from user preferences.

        Args:
            user: The user whose credentials to extract

        Returns:
            Dictionary with api_key and steam_id, or None if not configured
        """
        preferences = user.preferences or {}
        steam_config = preferences.get("steam", {})

        api_key = steam_config.get("web_api_key")
        steam_id = steam_config.get("steam_id")
        is_verified = steam_config.get("is_verified", False)

        if not api_key or not steam_id or not is_verified:
            return None

        return {"api_key": api_key, "steam_id": steam_id}

    def is_configured(self, user: User) -> bool:
        """Check if user has verified Steam credentials.

        Args:
            user: The user to check

        Returns:
            True if Steam credentials are configured and verified
        """
        return self.get_credentials(user) is not None

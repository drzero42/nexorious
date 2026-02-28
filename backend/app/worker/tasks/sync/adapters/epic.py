"""Epic Games Store sync adapter for fetching user's Epic library.

Implements SyncSourceAdapter protocol to fetch games from Epic Games Store
and convert them to the standardized ExternalLibraryEntry format.
"""

import logging
from typing import Optional, List, Dict, Any

from sqlmodel import Session

from app.models.user import User
from app.models.job import BackgroundJobSource
from app.services.epic import EpicService
from .base import ExternalLibraryEntry

logger = logging.getLogger(__name__)


class EpicSyncAdapter:
    """Adapter for syncing games from Epic Games Store.

    Fetches the user's Epic library and converts games to
    ExternalLibraryEntry format for generic processing.
    """

    source = BackgroundJobSource.EPIC

    async def fetch_games(self, user: User, session: Session) -> List[ExternalLibraryEntry]:
        """Fetch all games from user's Epic library.

        Args:
            user: The user whose Epic library to fetch
            session: SQLModel database session

        Returns:
            List of ExternalLibraryEntry objects

        Raises:
            ValueError: If Epic credentials are not configured
            EpicAuthExpiredError: If Epic authentication has expired
        """
        credentials = self.get_credentials(user)
        if not credentials:
            raise ValueError("Epic credentials not configured for this user")

        epic_service = EpicService(user_id=user.id, session=session)
        epic_games = await epic_service.get_library()

        logger.info(f"Fetched {len(epic_games)} games from Epic for user {user.id}")

        return [
            ExternalLibraryEntry(
                external_id=game.app_name,
                title=game.title,
                platform="pc-windows",
                storefront="epic-games-store",
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

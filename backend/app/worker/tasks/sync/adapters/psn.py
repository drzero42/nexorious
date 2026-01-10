"""PSN sync adapter for fetching user's PlayStation Network library.

Implements SyncSourceAdapter protocol to fetch games from PSN
and convert them to the standardized ExternalGame format.
"""

import logging
from typing import Optional, List, Dict
from datetime import datetime, timezone

from sqlmodel import Session

from app.models.user import User
from app.models.job import BackgroundJobSource
from app.models.user_game import OwnershipStatus
from app.services.psn import PSNService, PSNTokenExpiredError
from .base import ExternalGame

logger = logging.getLogger(__name__)


class PSNSyncAdapter:
    """Adapter for syncing games from PlayStation Network.

    Fetches the user's PSN purchased library and converts games to
    ExternalGame format for generic processing.
    """

    source = BackgroundJobSource.PSN

    def get_credentials(self, user: User) -> Optional[Dict[str, str]]:
        """Extract PSN credentials from user preferences.

        Args:
            user: The user whose credentials to extract

        Returns:
            Dictionary with npsso_token, or None if not configured
        """
        preferences = user.preferences or {}
        psn_config = preferences.get("psn", {})

        npsso_token = psn_config.get("npsso_token")
        is_verified = psn_config.get("is_verified", False)

        if not npsso_token or not is_verified:
            return None

        return {"npsso_token": npsso_token}

    def is_configured(self, user: User) -> bool:
        """Check if user has verified PSN credentials.

        Args:
            user: The user to check

        Returns:
            True if PSN credentials are configured and verified
        """
        return self.get_credentials(user) is not None

    def _mark_token_expired(self, user: User, session: Session) -> None:
        """Mark PSN token as expired in user preferences.

        Args:
            user: The user whose token expired
            session: SQLModel database session
        """
        import json
        preferences = user.preferences or {}
        if "psn" in preferences:
            preferences["psn"]["is_verified"] = False
            preferences["psn"]["token_expired_at"] = datetime.now(timezone.utc).isoformat()
            user.preferences_json = json.dumps(preferences)
            user.updated_at = datetime.now(timezone.utc)
            session.add(user)
            # Don't commit here - let the caller handle transaction boundaries
            logger.warning(f"Marked PSN token as expired for user {user.id}")

    async def fetch_games(self, user: User, session: Session) -> List[ExternalGame]:
        """Fetch all purchased games from user's PSN library.

        Args:
            user: The user whose PSN library to fetch
            session: SQLModel database session

        Returns:
            List of ExternalGame objects

        Raises:
            ValueError: If PSN credentials are not configured
            PSNTokenExpiredError: If NPSSO token has expired
        """
        credentials = self.get_credentials(user)
        if not credentials:
            raise ValueError("PSN credentials not configured for this user")

        psn_service = PSNService(npsso_token=credentials["npsso_token"])

        try:
            psn_games = await psn_service.get_library()
        except PSNTokenExpiredError:
            # Mark token as expired in preferences
            self._mark_token_expired(user, session)
            raise

        logger.info(f"Fetched {len(psn_games)} games from PSN for user {user.id}")

        # Convert to ExternalGame objects
        # Create one ExternalGame per platform entitlement
        external_games = []
        for game in psn_games:
            # Use title_id from metadata as the unique identifier
            # product_id is the full SKU which can have variants, title_id is the game identifier
            title_id = game.metadata.get("title_id", game.product_id)

            # Determine ownership status based on subscription flag
            ownership_status = (
                OwnershipStatus.SUBSCRIPTION if game.is_subscription
                else OwnershipStatus.OWNED
            )

            for platform in game.platforms:
                external_games.append(
                    ExternalGame(
                        external_id=title_id,  # Use title_id, not product_id
                        title=game.name,
                        platform=platform,  # "playstation-4" or "playstation-5"
                        storefront="playstation-store",
                        metadata={
                            "product_id": game.product_id,
                            **game.metadata
                        },
                        playtime_hours=game.playtime_hours,
                        ownership_status=ownership_status,
                    )
                )

        logger.info(f"Created {len(external_games)} ExternalGame objects from {len(psn_games)} PSN games")
        return external_games

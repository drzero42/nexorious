"""
PSN service for interacting with PlayStation Network via PSNAWP library.
"""

import logging
from dataclasses import dataclass
from typing import List, Dict, Any

logger = logging.getLogger(__name__)


@dataclass
class PSNAccountInfo:
    """PSN account information."""
    online_id: str        # PSN username
    account_id: str       # Unique account identifier
    region: str           # Account region


@dataclass
class PSNGame:
    """PSN game information from purchased library."""
    product_id: str       # Unique game identifier
    name: str             # Game title
    platforms: List[str]  # ["playstation-4", "playstation-5"]
    metadata: Dict[str, Any]  # Additional game metadata


class PSNAPIError(Exception):
    """PSN API error."""
    pass


class PSNAuthenticationError(PSNAPIError):
    """PSN authentication failed or invalid NPSSO token."""
    pass


class PSNTokenExpiredError(PSNAPIError):
    """PSN NPSSO token expired (~2 months)."""
    pass


class PSNService:
    """Service for interacting with PlayStation Network via PSNAWP library.

    Args:
        npsso_token: User's 64-character NPSSO token from PlayStation.com
    """

    def __init__(self, npsso_token: str):
        """Initialize PSN service with user's NPSSO token."""
        from psnawp_api import PSNAWP

        self.npsso_token = npsso_token
        try:
            self.psnawp = PSNAWP(npsso_token)
        except Exception as e:
            logger.error(f"Failed to initialize PSNAWP: {e}")
            raise PSNAuthenticationError(f"Failed to initialize PSN service: {e}")

    async def verify_token(self) -> bool:
        """Verify that the NPSSO token is valid.

        Returns:
            True if token is valid, False otherwise
        """
        try:
            client = self.psnawp.me()
            # Try to access basic account info
            _ = client.online_id
            return True
        except Exception as e:
            logger.warning(f"Token verification failed: {e}")
            return False

    async def get_account_info(self) -> PSNAccountInfo:
        """Get PSN account information.

        Returns:
            PSN account information

        Raises:
            PSNAuthenticationError: If token is invalid
            PSNTokenExpiredError: If token has expired
        """
        try:
            client = self.psnawp.me()

            return PSNAccountInfo(
                online_id=client.online_id,
                account_id=client.account_id,
                region=client.get_region()
            )
        except Exception as e:
            # Check if error indicates expired token
            error_str = str(e).lower()
            if "expired" in error_str or "unauthorized" in error_str:
                raise PSNTokenExpiredError("NPSSO token has expired")
            raise PSNAuthenticationError(f"Failed to get account info: {e}")

    async def get_library(self) -> List[PSNGame]:
        """Get purchased games from PSN library (PS4/PS5 only).

        Returns:
            List of PSN games with platform entitlements

        Raises:
            PSNTokenExpiredError: If token has expired
            PSNAPIError: If library cannot be retrieved
        """
        try:
            client = self.psnawp.me()
            purchased_games = client.purchased_games()

            games = []
            for game in purchased_games:
                # Detect which platforms user has entitlement for
                platforms = []
                if hasattr(game, 'has_ps5_entitlement') and game.has_ps5_entitlement:
                    platforms.append("playstation-5")
                if hasattr(game, 'has_ps4_entitlement') and game.has_ps4_entitlement:
                    platforms.append("playstation-4")

                # Fallback to PS5 if no platform info
                if not platforms:
                    platforms = ["playstation-5"]

                psn_game = PSNGame(
                    product_id=game.product_id,
                    name=game.name,
                    platforms=platforms,
                    metadata={
                        "product_id": game.product_id,
                    }
                )
                games.append(psn_game)

            logger.info(f"Retrieved {len(games)} games from PSN library")
            return games

        except Exception as e:
            error_str = str(e).lower()
            if "expired" in error_str or "unauthorized" in error_str:
                raise PSNTokenExpiredError("NPSSO token has expired")
            raise PSNAPIError(f"Failed to retrieve PSN library: {e}")


def create_psn_service(npsso_token: str) -> PSNService:
    """Factory function to create a PSN service instance."""
    return PSNService(npsso_token)

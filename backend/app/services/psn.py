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


def create_psn_service(npsso_token: str) -> PSNService:
    """Factory function to create a PSN service instance."""
    return PSNService(npsso_token)

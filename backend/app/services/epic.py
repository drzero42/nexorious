"""
Epic Games Store service using legendary Python library.

Provides Epic authentication and library fetching via legendary.core.LegendaryCore.
All legendary configs are isolated per user via custom config directories.
"""

import logging
import os
from typing import Any, Dict, List

from legendary.core import LegendaryCore
from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)


class EpicAccountInfo(BaseModel):
    """Epic account information."""
    display_name: str
    account_id: str


class EpicGame(BaseModel):
    """Epic game information from library."""
    app_name: str
    title: str
    metadata: Dict[str, Any] = Field(default_factory=dict)


class EpicAuthenticationError(Exception):
    """Epic authentication failed or invalid."""
    pass


class EpicAuthExpiredError(Exception):
    """Epic authentication token expired."""
    pass


class EpicAPIError(Exception):
    """Epic API error or legendary operation failed."""
    pass


class EpicService:
    """Service for interacting with Epic Games Store via legendary library.

    Args:
        user_id: User's unique identifier for config isolation
    """

    def __init__(self, user_id: str):
        """Initialize Epic service with user-specific config path."""
        self.user_id = user_id
        self.config_path = f"/var/lib/nexorious/legendary-configs/{user_id}"

        # Set environment variable for legendary to use custom config directory
        # legendary respects XDG_CONFIG_HOME for storing its config
        os.environ['XDG_CONFIG_HOME'] = self.config_path

        # Initialize legendary core with custom config
        try:
            self.core = LegendaryCore()
            logger.debug(f"EpicService initialized for user {user_id} with config at {self.config_path}")
        except Exception as e:
            logger.error(f"Failed to initialize LegendaryCore: {e}")
            raise EpicAPIError(f"Failed to initialize Epic service: {e}")

    def get_auth_url(self) -> str:
        """Get Epic authentication URL for user to visit.

        Returns:
            Epic Games authentication URL
        """
        logger.info(f"Getting Epic auth URL for user {self.user_id}")
        try:
            # Use legendary's built-in method to generate the OAuth URL
            auth_url = self.core.egs.get_auth_url()
            logger.debug(f"Generated auth URL: {auth_url}")
            return auth_url
        except Exception as e:
            logger.error(f"Failed to generate auth URL: {e}")
            raise EpicAPIError(f"Failed to generate authentication URL: {e}")

    async def start_device_auth(self) -> str:
        """Start Epic device authentication flow.

        Returns:
            Device authorization URL for user to visit
        """
        logger.info(f"Starting Epic device auth for user {self.user_id}")
        return self.get_auth_url()

    async def complete_auth(self, code: str) -> bool:
        """Complete Epic authentication with authorization code.

        Args:
            code: Authorization code from Epic Games OAuth flow

        Returns:
            True if authentication succeeded

        Raises:
            EpicAuthenticationError: If authentication fails
        """
        logger.info(f"Completing Epic auth for user {self.user_id}")
        try:
            # Use legendary's auth_code method to complete authentication
            success = self.core.auth_code(code)

            if success:
                logger.info(f"Epic authentication successful for user {self.user_id}")
                return True
            else:
                logger.error("Epic authentication failed: auth_code returned False")
                raise EpicAuthenticationError("Epic authentication failed")

        except Exception as e:
            logger.error(f"Epic authentication failed: {e}")
            raise EpicAuthenticationError(f"Epic authentication failed: {e}")

    async def verify_auth(self) -> bool:
        """Verify Epic authentication status.

        Returns:
            True if authenticated, False otherwise
        """
        logger.debug(f"Verifying Epic auth for user {self.user_id}")
        try:
            # Try to login with existing credentials
            # This will return True if valid credentials exist and work
            is_authenticated = self.core.login()
            logger.debug(f"Auth status for user {self.user_id}: {is_authenticated}")
            return is_authenticated
        except Exception as e:
            logger.warning(f"Auth verification failed: {e}")
            return False

    async def get_account_info(self) -> EpicAccountInfo:
        """Get Epic account information.

        Returns:
            Epic account information

        Raises:
            EpicAuthExpiredError: If not authenticated
            EpicAPIError: If account info cannot be retrieved
        """
        logger.info(f"Fetching Epic account info for user {self.user_id}")

        # Verify authentication first
        if not await self.verify_auth():
            logger.error(f"User {self.user_id} not authenticated")
            raise EpicAuthExpiredError("Not authenticated with Epic Games")

        try:
            # Get user info from legendary
            user_data = self.core.egs.user
            if not user_data:
                raise EpicAPIError("No user data available")

            display_name = user_data.get('displayName', '')
            account_id = user_data.get('account_id', '')

            if not display_name or not account_id:
                logger.error("Missing account information in user data")
                raise EpicAPIError("Failed to retrieve account information")

            logger.debug(f"Retrieved account info for {display_name} ({account_id})")
            return EpicAccountInfo(
                display_name=display_name,
                account_id=account_id
            )

        except Exception as e:
            logger.error(f"Failed to get account info: {e}")
            raise EpicAPIError(f"Failed to retrieve account information: {e}")

    async def disconnect(self) -> None:
        """Disconnect Epic account by removing credentials.

        Raises:
            EpicAPIError: If disconnect fails
        """
        logger.info(f"Disconnecting Epic account for user {self.user_id}")
        try:
            # Invalidate the session and remove stored credentials
            self.core.lgd.invalidate_userdata()
            logger.info(f"Epic account disconnected for user {self.user_id}")
        except Exception as e:
            logger.error(f"Failed to disconnect: {e}")
            raise EpicAPIError(f"Failed to disconnect Epic account: {e}")

    async def get_library(self) -> List[EpicGame]:
        """Get Epic Games library.

        Returns:
            List of games owned by the authenticated user

        Raises:
            EpicAuthExpiredError: If not authenticated
            EpicAPIError: If library cannot be retrieved
        """
        logger.info(f"Fetching Epic library for user {self.user_id}")

        # Verify authentication first
        if not await self.verify_auth():
            logger.error(f"User {self.user_id} not authenticated")
            raise EpicAuthExpiredError("Not authenticated with Epic Games")

        try:
            # Get library items from legendary
            library_items = self.core.get_game_list(update_assets=True)

            games = []
            for game in library_items:
                epic_game = EpicGame(
                    app_name=game.app_name,
                    title=game.app_title,
                    metadata={
                        'namespace': game.namespace,
                        'catalog_item_id': game.catalog_item_id,
                    }
                )
                games.append(epic_game)

            logger.info(f"Retrieved {len(games)} games for user {self.user_id}")
            return games

        except Exception as e:
            logger.error(f"Failed to get library: {e}")
            raise EpicAPIError(f"Failed to retrieve Epic library: {e}")

"""
IGDB authentication module.

Handles Twitch OAuth authentication for IGDB API access.
"""

import logging
from datetime import datetime, timedelta, timezone
from typing import Optional

import httpx
from igdb.wrapper import IGDBWrapper

from app.core.config import settings
from .models import TwitchAuthError


logger = logging.getLogger(__name__)


class IGDBAuthManager:
    """Manages IGDB/Twitch authentication and wrapper lifecycle."""

    def __init__(self, http_client: httpx.AsyncClient):
        self.client_id = settings.igdb_client_id
        self.client_secret = settings.igdb_client_secret
        self._access_token = settings.igdb_access_token
        self._token_expires_at: Optional[datetime] = None
        self._wrapper: Optional[IGDBWrapper] = None
        self._http_client = http_client

    async def get_access_token(self) -> str:
        """Get or refresh Twitch access token using client credentials flow."""
        if not self.client_id or not self.client_secret:
            logger.error("IGDB client ID and secret not configured")
            raise TwitchAuthError("IGDB client ID and secret must be configured")

        # Check if current token is still valid
        if (self._access_token and
            self._token_expires_at and
            datetime.now(timezone.utc) < self._token_expires_at - timedelta(minutes=5)):
            logger.debug(f"Using existing access token (expires at {self._token_expires_at})")
            return self._access_token

        logger.info("Requesting new Twitch access token")
        logger.debug(f"Using client ID: {self.client_id[:8]}...")

        try:
            response = await self._http_client.post(
                "https://id.twitch.tv/oauth2/token",
                data={
                    "client_id": self.client_id,
                    "client_secret": self.client_secret,
                    "grant_type": "client_credentials"
                }
            )

            logger.debug(f"Twitch auth response status: {response.status_code}")
            response.raise_for_status()

            token_data = response.json()
            self._access_token = token_data["access_token"]
            expires_in = token_data.get("expires_in", 3600)  # Default to 1 hour
            self._token_expires_at = datetime.now(timezone.utc) + timedelta(seconds=expires_in)

            logger.info(f"Successfully obtained Twitch access token, expires at {self._token_expires_at}")
            logger.debug(f"Token preview: {self._access_token[:10]}...")
            return self._access_token

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting Twitch access token: {e}")
            logger.debug(f"Response body: {e.response.text}")
            raise TwitchAuthError(f"Failed to authenticate with Twitch: {e}")
        except httpx.HTTPError as e:
            logger.error(f"HTTP error getting Twitch access token: {e}")
            raise TwitchAuthError(f"Failed to authenticate with Twitch: {e}")
        except Exception as e:
            logger.error(f"Unexpected error getting access token: {e}", exc_info=True)
            raise TwitchAuthError(f"Failed to authenticate with Twitch: {e}")

    async def get_wrapper(self) -> IGDBWrapper:
        """Get initialized IGDB wrapper with valid access token."""
        if not self._wrapper:
            if not self.client_id:
                logger.error("IGDB client ID is required for wrapper initialization")
                raise TwitchAuthError("IGDB client ID is required")

            logger.debug("Initializing IGDB wrapper")
            access_token = await self.get_access_token()
            if not access_token:
                logger.error("Failed to obtain valid access token for IGDB wrapper")
                raise TwitchAuthError("Failed to obtain valid access token")

            logger.debug("Creating IGDBWrapper instance")
            self._wrapper = IGDBWrapper(self.client_id, access_token)

        return self._wrapper

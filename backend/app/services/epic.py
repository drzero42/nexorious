"""
Epic Games Store service using legendary CLI.

Provides Epic authentication and library fetching via legendary subprocess calls.
All legendary commands run with isolated XDG_CONFIG_HOME per user.
"""

import asyncio
import json
import logging
import os
import re
from typing import Any, Dict, List

from pydantic import BaseModel

logger = logging.getLogger(__name__)


class EpicAccountInfo(BaseModel):
    """Epic account information."""
    display_name: str
    account_id: str


class LegendaryNotFoundError(Exception):
    """legendary CLI not found on system."""
    pass


class EpicAuthenticationError(Exception):
    """Epic authentication failed or invalid."""
    pass


class EpicAuthExpiredError(Exception):
    """Epic authentication token expired."""
    pass


class EpicAPIError(Exception):
    """Epic API error or legendary command failed."""
    pass


class EpicService:
    """Service for interacting with Epic Games Store via legendary CLI.

    Args:
        user_id: User's unique identifier for config isolation
    """

    def __init__(self, user_id: str):
        """Initialize Epic service with user-specific config path."""
        self.user_id = user_id
        self.config_path = f"/var/lib/nexorious/legendary-configs/{user_id}"
        logger.debug(f"EpicService initialized for user {user_id}")

    async def _run_legendary_command(
        self, args: List[str], timeout: int = 60
    ) -> Dict[str, Any]:
        """Run legendary CLI command with isolated config.

        Args:
            args: Command arguments (e.g., ["status", "--json"])
            timeout: Command timeout in seconds

        Returns:
            Dict with stdout, stderr, and returncode

        Raises:
            LegendaryNotFoundError: legendary CLI not found
            EpicAuthExpiredError: Authentication expired
            EpicAPIError: Command failed
        """
        # Set isolated config path via XDG_CONFIG_HOME
        env = os.environ.copy()
        env['XDG_CONFIG_HOME'] = self.config_path

        try:
            process = await asyncio.create_subprocess_exec(
                'legendary',
                *args,
                env=env,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )

            stdout, stderr = await asyncio.wait_for(
                process.communicate(), timeout=timeout
            )

            stdout_str = stdout.decode('utf-8')
            stderr_str = stderr.decode('utf-8')

            # Check for auth expiration
            if process.returncode != 0:
                stderr_lower = stderr_str.lower()
                if any(phrase in stderr_lower for phrase in [
                    'not authenticated',
                    'login',
                    'expired',
                    'authentication required'
                ]):
                    logger.warning(f"Epic authentication expired for user {self.user_id}")
                    raise EpicAuthExpiredError("Epic authentication expired")

                # Generic command failure
                logger.error(f"legendary command failed: {stderr_str}")
                raise EpicAPIError(f"legendary command failed: {stderr_str}")

            return {
                "stdout": stdout_str,
                "stderr": stderr_str,
                "returncode": process.returncode
            }

        except FileNotFoundError:
            logger.error("legendary CLI not found on system")
            raise LegendaryNotFoundError(
                "legendary CLI not found. Install with: pip install legendary-gl"
            )
        except asyncio.TimeoutError:
            logger.error(f"legendary command timed out after {timeout}s")
            raise EpicAPIError(f"legendary command timed out after {timeout}s")

    async def start_device_auth(self) -> str:
        """Start Epic device authentication flow.

        Returns:
            Device authorization URL for user to visit

        Raises:
            EpicAPIError: If legendary command fails
        """
        logger.info(f"Starting Epic device auth for user {self.user_id}")
        result = await self._run_legendary_command(["auth", "--json"])

        # Extract URL from legendary output (format: "Please visit: <URL>")
        match = re.search(r'https://[^\s]+', result["stdout"])
        if not match:
            logger.error("Failed to extract auth URL from legendary output")
            raise EpicAPIError("Failed to extract authentication URL")

        url = match.group(0)
        logger.debug(f"Device auth URL: {url}")
        return url

    async def complete_auth(self, code: str) -> bool:
        """Complete Epic authentication with device code.

        Args:
            code: Device authorization code from Epic

        Returns:
            True if authentication succeeded

        Raises:
            EpicAuthenticationError: If authentication fails
        """
        logger.info(f"Completing Epic auth for user {self.user_id}")
        try:
            result = await self._run_legendary_command(
                ["auth", "--code", code, "--json"]
            )

            # Check if response indicates success
            stdout_lower = result["stdout"].lower()
            if "logged in" in stdout_lower:
                logger.info(f"Epic authentication successful for user {self.user_id}")
                return True

            logger.error("Epic authentication failed: unexpected response")
            raise EpicAuthenticationError("Epic authentication failed")

        except EpicAPIError as e:
            logger.error(f"Epic authentication failed: {e}")
            raise EpicAuthenticationError(f"Epic authentication failed: {e}")

    async def verify_auth(self) -> bool:
        """Verify Epic authentication status.

        Returns:
            True if authenticated, False otherwise
        """
        logger.debug(f"Verifying Epic auth for user {self.user_id}")
        try:
            result = await self._run_legendary_command(["status", "--json"])

            # Parse JSON response to check authentication
            try:
                data = json.loads(result["stdout"])
                is_authenticated = "account" in data
                logger.debug(f"Auth status for user {self.user_id}: {is_authenticated}")
                return is_authenticated
            except json.JSONDecodeError:
                logger.warning("Failed to parse status JSON, assuming not authenticated")
                return False

        except EpicAuthExpiredError:
            logger.debug(f"User {self.user_id} not authenticated")
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
        result = await self._run_legendary_command(["status", "--json"])

        try:
            data = json.loads(result["stdout"])
            account = data.get("account", {})

            display_name = account.get("displayName", "")
            account_id = account.get("id", "")

            if not display_name or not account_id:
                logger.error("Missing account information in legendary response")
                raise EpicAPIError("Failed to retrieve account information")

            logger.debug(f"Retrieved account info for {display_name} ({account_id})")
            return EpicAccountInfo(
                display_name=display_name,
                account_id=account_id
            )

        except json.JSONDecodeError as e:
            logger.error(f"Failed to parse account info JSON: {e}")
            raise EpicAPIError(f"Failed to parse account information: {e}")

    async def disconnect(self) -> None:
        """Disconnect Epic account by removing credentials.

        Raises:
            EpicAPIError: If disconnect fails
        """
        logger.info(f"Disconnecting Epic account for user {self.user_id}")
        await self._run_legendary_command(["auth", "--delete"])
        logger.info(f"Epic account disconnected for user {self.user_id}")

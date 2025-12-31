"""
Epic Games Store service using legendary CLI.

Provides Epic authentication and library fetching via legendary subprocess calls.
All legendary commands run with isolated XDG_CONFIG_HOME per user.
"""

import asyncio
import logging
import os
from typing import Any, Dict, List

logger = logging.getLogger(__name__)


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

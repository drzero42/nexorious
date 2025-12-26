"""Connection monitor for scheduler resilience.

Monitors NATS and PostgreSQL connectivity with exponential backoff.
Used by the scheduler to pause during outages and resume on recovery.
"""

import asyncio
import logging

import nats
from sqlalchemy import text

from app.core.config import settings
from app.core.database import get_engine

logger = logging.getLogger(__name__)


async def _quiet_error_callback(e: Exception) -> None:
    """Quiet error callback for NATS client during connection checks.

    Logs connection errors as warnings without full tracebacks.
    """
    logger.warning(f"NATS connection error: {e}")


class ConnectionMonitor:
    """Monitors NATS and PostgreSQL connections with exponential backoff reconnection.

    When connections are lost, wait_for_connections() blocks until both services
    are available again, using exponential backoff between retry attempts.
    """

    def __init__(
        self,
        initial_delay: float | None = None,
        max_delay: float | None = None,
        backoff_multiplier: float | None = None,
    ):
        """Initialize the connection monitor.

        Args:
            initial_delay: Initial delay in seconds before first retry (default from settings)
            max_delay: Maximum delay in seconds between retries (default from settings)
            backoff_multiplier: Multiplier for exponential backoff (default from settings)
        """
        self.initial_delay = (
            initial_delay
            if initial_delay is not None
            else settings.scheduler_reconnect_initial_delay
        )
        self.max_delay = (
            max_delay
            if max_delay is not None
            else settings.scheduler_reconnect_max_delay
        )
        self.backoff_multiplier = (
            backoff_multiplier
            if backoff_multiplier is not None
            else settings.scheduler_reconnect_backoff_multiplier
        )
        self._current_delay = self.initial_delay

    def reset_backoff(self) -> None:
        """Reset backoff delay to initial value after successful connection."""
        self._current_delay = self.initial_delay

    def _get_next_delay(self) -> float:
        """Get the next delay and increase for exponential backoff.

        Returns:
            The current delay value before increasing.
        """
        delay = self._current_delay
        self._current_delay = min(
            self._current_delay * self.backoff_multiplier,
            self.max_delay,
        )
        return delay

    async def check_nats(self) -> bool:
        """Check if NATS is reachable.

        Returns:
            True if NATS connection succeeds, False otherwise.
        """
        try:
            client = await nats.connect(
                settings.NATS_URL,
                error_cb=_quiet_error_callback,
            )
            is_connected = client.is_connected
            await client.close()
            return is_connected
        except Exception:
            return False

    async def check_postgres(self) -> bool:
        """Check if PostgreSQL is reachable.

        Returns:
            True if PostgreSQL connection succeeds, False otherwise.
        """
        try:
            engine = get_engine()
            with engine.connect() as conn:
                conn.execute(text("SELECT 1"))
            return True
        except Exception:
            return False

    async def wait_for_connections(self) -> None:
        """Block until both NATS and PostgreSQL are available.

        Uses exponential backoff between retry attempts. Logs warnings
        during reconnection attempts.
        """
        while True:
            nats_ok = await self.check_nats()
            postgres_ok = await self.check_postgres()

            if nats_ok and postgres_ok:
                self.reset_backoff()
                return

            # Build status message
            failed_services = []
            if not nats_ok:
                failed_services.append("NATS")
            if not postgres_ok:
                failed_services.append("PostgreSQL")

            delay = self._get_next_delay()
            logger.warning(
                f"Connection check failed for {', '.join(failed_services)}. "
                f"Retrying in {delay:.1f}s..."
            )
            await asyncio.sleep(delay)


# Global monitor instance for scheduler use
_monitor: ConnectionMonitor | None = None


def get_connection_monitor() -> ConnectionMonitor:
    """Get or create the global connection monitor instance."""
    global _monitor
    if _monitor is None:
        _monitor = ConnectionMonitor()
    return _monitor

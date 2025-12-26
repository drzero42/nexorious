"""
NATS client singleton for application-wide access.

This module provides a shared NATS client instance that can be used
across the application for distributed coordination (rate limiting, etc.)
"""

import logging
from typing import Optional

import nats
from nats.aio.client import Client as NATSClient

from app.core.config import settings

logger = logging.getLogger(__name__)

_nats_client: Optional[NATSClient] = None


async def get_nats_client() -> NATSClient:
    """
    Get the shared NATS client instance.

    Creates a new connection if one doesn't exist.

    Returns:
        Connected NATS client
    """
    global _nats_client

    if _nats_client is None or not _nats_client.is_connected:
        logger.info(f"Connecting to NATS at {settings.NATS_URL}")
        _nats_client = await nats.connect(settings.NATS_URL)
        logger.info("NATS connection established")

    return _nats_client


async def close_nats_client() -> None:
    """Close the NATS client connection."""
    global _nats_client

    if _nats_client is not None and _nats_client.is_connected:
        logger.info("Closing NATS connection")
        await _nats_client.close()
        _nats_client = None

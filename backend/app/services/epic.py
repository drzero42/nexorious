"""
Epic Games Store service using legendary CLI.

Provides Epic authentication and library fetching via legendary subprocess calls.
All legendary commands run with isolated XDG_CONFIG_HOME per user.
"""

import logging

logger = logging.getLogger(__name__)


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

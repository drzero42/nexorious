"""
Maintenance mode middleware.

During restore operations, blocks all non-admin API requests.
"""

import logging
from typing import Awaitable, Callable
from fastapi import Request, Response
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

logger = logging.getLogger(__name__)

# Global maintenance mode flag
_maintenance_mode = False
_maintenance_message = "System maintenance in progress"


def is_maintenance_mode() -> bool:
    """Check if maintenance mode is enabled."""
    return _maintenance_mode


def set_maintenance_mode(enabled: bool, message: str = "System maintenance in progress") -> None:
    """Set maintenance mode state.

    Args:
        enabled: Whether to enable maintenance mode.
        message: Message to display to users.
    """
    global _maintenance_mode, _maintenance_message
    _maintenance_mode = enabled
    _maintenance_message = message
    logger.info(f"Maintenance mode {'enabled' if enabled else 'disabled'}: {message}")


class MaintenanceModeMiddleware(BaseHTTPMiddleware):
    """Middleware that blocks requests during maintenance mode."""

    # Paths that are always allowed (health checks, admin backup endpoints)
    ALLOWED_PATHS = {
        "/health",
        "/docs",
        "/redoc",
        "/openapi.json",
    }

    # Admin paths that should be allowed during maintenance
    ADMIN_ALLOWED_PREFIXES = {
        "/api/admin/backups",
        "/api/auth/me",  # Allow checking current user
    }

    async def dispatch(self, request: Request, call_next: Callable[[Request], Awaitable[Response]]) -> Response:
        """Process request through maintenance mode check."""

        if not _maintenance_mode:
            return await call_next(request)

        path = request.url.path

        # Always allow certain paths
        if path in self.ALLOWED_PATHS:
            return await call_next(request)

        # Allow admin backup operations during maintenance
        for prefix in self.ADMIN_ALLOWED_PREFIXES:
            if path.startswith(prefix):
                return await call_next(request)

        # Block all other requests
        return JSONResponse(
            status_code=503,
            content={
                "error": "Service Unavailable",
                "detail": _maintenance_message,
                "maintenance_mode": True,
            },
        )

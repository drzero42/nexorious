"""
Application status endpoint.

Provides status information about external service configurations.
"""

from fastapi import APIRouter

from ..core.config import settings

router = APIRouter(tags=["status"])


@router.get("/status")
async def get_status():
    """
    Get application status including external service configurations.

    This endpoint is public and does not require authentication.

    Returns:
        dict: Status information including:
            - igdb_configured: Whether IGDB API credentials are configured
    """
    igdb_configured = bool(
        settings.igdb_client_id and settings.igdb_client_secret
    )

    return {"igdb_configured": igdb_configured}

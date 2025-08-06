"""
API dependencies for dependency injection.
"""

from typing import Generator
from functools import lru_cache

from app.services.igdb import IGDBService


@lru_cache()
def get_igdb_service() -> IGDBService:
    """Get IGDB service instance."""
    return IGDBService()


def get_igdb_service_dependency() -> Generator[IGDBService, None, None]:
    """FastAPI dependency for IGDB service."""
    service = get_igdb_service()
    try:
        yield service
    finally:
        # Clean up if needed
        pass
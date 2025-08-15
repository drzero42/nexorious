"""
Import API package for managing game library imports from various sources.
"""

from fastapi import APIRouter
from .core import router as core_router
from .sources import router as sources_router

router = APIRouter(prefix="/import", tags=["Import"])
router.include_router(core_router)
router.include_router(sources_router, prefix="/sources")
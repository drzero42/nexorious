"""
Import sources API package for managing source-specific import operations.
"""

from fastapi import APIRouter

# Future imports:
# from .epic import router as epic_router
# from .gog import router as gog_router

router = APIRouter(tags=["Import Sources"])

# Future source routers:
# router.include_router(epic_router, prefix="/epic", tags=["Import - Epic"])
# router.include_router(gog_router, prefix="/gog", tags=["Import - GOG"])

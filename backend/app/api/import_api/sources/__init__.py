"""
Import sources API package for managing source-specific import operations.
"""

from fastapi import APIRouter
from .darkadia import router as darkadia_router
from .darkadia_batch import router as darkadia_batch_router

# Future imports:
# from .epic import router as epic_router
# from .gog import router as gog_router

router = APIRouter(tags=["Import Sources"])
router.include_router(darkadia_router, prefix="/darkadia", tags=["Import - Darkadia"])
router.include_router(darkadia_batch_router, prefix="/darkadia", tags=["Import - Darkadia Batch"])

# Future source routers:
# router.include_router(epic_router, prefix="/epic", tags=["Import - Epic"])
# router.include_router(gog_router, prefix="/gog", tags=["Import - GOG"])
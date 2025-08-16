"""
Import sources API package for managing source-specific import operations.
"""

from fastapi import APIRouter
from .steam import router as steam_router
from .steam_batch import router as steam_batch_router

# Future imports:
# from .epic import router as epic_router
# from .gog import router as gog_router

router = APIRouter(tags=["Import Sources"])
router.include_router(steam_router, prefix="/steam", tags=["Import - Steam"])
router.include_router(steam_batch_router, prefix="/steam", tags=["Import - Steam Batch"])

# Future source routers:
# router.include_router(epic_router, prefix="/epic", tags=["Import - Epic"])
# router.include_router(gog_router, prefix="/gog", tags=["Import - GOG"])
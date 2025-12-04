"""
Darkadia batch processing endpoints for the import framework.

This module provides batch processing endpoints for Darkadia games,
using the generic batch processor for consistency with other import sources.
"""

from ..batch_processor import BatchSourceConfig, create_batch_router
from ....core.security import get_current_user
from ....models.darkadia_game import DarkadiaGame
from ....schemas.darkadia import DarkadiaGameResponse
from ....services.import_sources.darkadia import create_darkadia_import_service


def _darkadia_response_mapper(game: DarkadiaGame, results: dict) -> DarkadiaGameResponse:
    """Map DarkadiaGame model to DarkadiaGameResponse schema."""
    return DarkadiaGameResponse(
        id=game.id,
        external_id=game.external_id,
        name=game.game_name,
        igdb_id=game.igdb_id,
        igdb_title=game.igdb_title,
        game_id=game.game_id,
        user_game_id=results.get("user_game_id"),
        ignored=game.ignored,
        created_at=game.created_at,
        updated_at=game.updated_at,
    )


# Configure the Darkadia batch processor
darkadia_batch_config = BatchSourceConfig(
    source_name="Darkadia",
    router_prefix="/batch",
    router_tags=["Darkadia Batch Import"],
    game_model=DarkadiaGame,
    response_schema=DarkadiaGameResponse,
    service_factory=create_darkadia_import_service,
    auth_dependency=get_current_user,
    response_mapper=_darkadia_response_mapper,
)

# Create the router using the generic batch processor
router = create_batch_router(darkadia_batch_config)

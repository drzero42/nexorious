"""
Steam batch processing endpoints for the new import framework.

This module provides batch processing endpoints for Steam games under the new
import API structure, using the generic batch processor for consistency.
"""

from ..batch_processor import BatchSourceConfig, create_batch_router
from ....models.steam_game import SteamGame
from ....schemas.steam import SteamGameResponse
from ....services.import_sources.steam import create_steam_import_service
from ...dependencies import verify_steam_games_enabled


def _steam_response_mapper(game: SteamGame, results: dict) -> SteamGameResponse:
    """Map SteamGame model to SteamGameResponse schema."""
    return SteamGameResponse.model_validate({
        "id": game.id,
        "steam_appid": game.steam_appid,
        "game_name": game.game_name,
        "igdb_id": game.igdb_id,
        "igdb_title": game.igdb_title,
        "user_game_id": results.get("user_game_id"),
        "ignored": game.ignored,
        "created_at": game.created_at,
        "updated_at": game.updated_at,
    })


# Configure the Steam batch processor
steam_batch_config = BatchSourceConfig(
    source_name="Steam",
    router_prefix="/batch",
    router_tags=["Steam Batch Import"],
    game_model=SteamGame,
    response_schema=SteamGameResponse,
    service_factory=create_steam_import_service,
    auth_dependency=verify_steam_games_enabled,
    response_mapper=_steam_response_mapper,
)

# Create the router using the generic batch processor
router = create_batch_router(steam_batch_config)

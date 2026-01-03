"""
Game service for managing game import and IGDB integration.

This service handles the business logic for importing games from IGDB,
separating it from the API layer concerns.
"""

from datetime import date
from typing import Optional
import json
import logging

from sqlmodel import Session
from sqlalchemy.exc import IntegrityError

from ..models.game import Game
from ..services.igdb import IGDBService


logger = logging.getLogger(__name__)


class GameNotFoundError(Exception):
    """Raised when a game cannot be found in IGDB."""

    def __init__(self, igdb_id: int):
        self.igdb_id = igdb_id
        super().__init__(f"Game with IGDB ID {igdb_id} not found")


def parse_date_string(date_string: Optional[str]) -> Optional[date]:
    """Parse a date string into a Python date object."""
    if not date_string:
        return None

    try:
        # Handle YYYY-MM-DD format
        if len(date_string) == 10 and date_string.count("-") == 2:
            year, month, day = date_string.split("-")
            return date(int(year), int(month), int(day))
        # Handle YYYY format
        elif len(date_string) == 4:
            return date(int(date_string), 1, 1)
        else:
            return None
    except (ValueError, TypeError):
        return None


class GameService:
    """Service class for game management operations."""

    def __init__(self, session: Session, igdb_service: IGDBService):
        self.session = session
        self.igdb_service = igdb_service

    async def create_or_update_game_from_igdb(
        self,
        igdb_id: int,
        custom_overrides: dict | None = None,
        download_cover_art: bool = True,
    ) -> Game:
        """
        Create a new game from IGDB or update an existing one.

        Fetches full game metadata from IGDB and either creates a new game record
        or applies custom overrides to an existing game.

        Args:
            igdb_id: The IGDB ID of the game to import
            custom_overrides: Optional dictionary of field overrides to apply
            download_cover_art: Whether to download and store cover art locally

        Returns:
            Game model instance (either newly created or existing with overrides)

        Raises:
            GameNotFoundError: If the game is not found in IGDB
            TwitchAuthError: If IGDB authentication fails (propagated)
            IGDBError: If there's an IGDB API error (propagated)
        """
        # Retrieve full game metadata from IGDB
        game_metadata = await self.igdb_service.get_game_by_id(igdb_id)

        if not game_metadata:
            raise GameNotFoundError(igdb_id)

        # Check if game already exists in our database
        existing_game = self.session.get(Game, igdb_id)

        if existing_game:
            logger.info(
                f"IGDB game import found existing game - returning existing entry. "
                f"IGDB ID: {igdb_id} | "
                f"Existing game ID: {existing_game.id} | "
                f"Existing title: '{existing_game.title}' | "
                f"IGDB title: '{game_metadata.title}' | "
                f"Title match: {existing_game.title == game_metadata.title}"
            )
            # Apply any custom overrides to the existing game if requested
            if custom_overrides:
                for key, value in custom_overrides.items():
                    if hasattr(existing_game, key) and value is not None:
                        setattr(existing_game, key, value)
                self.session.commit()
                self.session.refresh(existing_game)

            return existing_game

        # Create game from IGDB metadata
        game_data = {
            "id": game_metadata.igdb_id,  # IGDB ID as primary key
            "title": game_metadata.title,
            "description": game_metadata.description,
            "genre": game_metadata.genre,
            "developer": game_metadata.developer,
            "publisher": game_metadata.publisher,
            "release_date": parse_date_string(game_metadata.release_date),
            "cover_art_url": game_metadata.cover_art_url,
            "rating_average": game_metadata.rating_average,
            "rating_count": game_metadata.rating_count or 0,
            "estimated_playtime_hours": game_metadata.estimated_playtime_hours,
            "howlongtobeat_main": game_metadata.hastily,
            "howlongtobeat_extra": game_metadata.normally,
            "howlongtobeat_completionist": game_metadata.completely,
            "igdb_slug": game_metadata.igdb_slug,
            "igdb_platform_ids": json.dumps(game_metadata.igdb_platform_ids)
            if game_metadata.igdb_platform_ids
            else None,
            "igdb_platform_names": json.dumps(game_metadata.platform_names)
            if game_metadata.platform_names
            else None,
            "game_modes": ", ".join(game_metadata.game_modes)
            if game_metadata.game_modes
            else None,
            "themes": ", ".join(game_metadata.themes)
            if game_metadata.themes
            else None,
            "player_perspectives": ", ".join(game_metadata.player_perspectives)
            if game_metadata.player_perspectives
            else None,
            "game_metadata": "{}",
        }

        # Apply any custom overrides from the user
        if custom_overrides:
            for key, value in custom_overrides.items():
                if key in game_data and value is not None:
                    game_data[key] = value

        # Create the game with race condition handling
        # Another process may have created the same game between our check and insert
        new_game = Game(**game_data)
        self.session.add(new_game)
        try:
            self.session.commit()
        except IntegrityError:
            # Race condition: another process created the game
            # Rollback and fetch the existing game
            self.session.rollback()
            existing_game = self.session.get(Game, igdb_id)
            if existing_game:
                logger.info(
                    f"Race condition handled: game {igdb_id} was created by another process, "
                    f"returning existing entry"
                )
                return existing_game
            # If still not found, re-raise the error
            raise
        self.session.refresh(new_game)

        # Download cover art if requested and available
        if download_cover_art and game_metadata.cover_art_url:
            try:
                local_url = await self.igdb_service.download_and_store_cover_art(
                    game_metadata.igdb_id, game_metadata.cover_art_url
                )
                if local_url:
                    new_game.cover_art_url = local_url
                    self.session.commit()
                    self.session.refresh(new_game)
            except Exception as e:
                # Log the error but don't fail the import
                logger.error(f"Failed to download cover art for game {new_game.id}: {e}")

        logger.info(
            f"Created new game from IGDB. "
            f"IGDB ID: {igdb_id} | "
            f"Title: '{new_game.title}' | "
            f"Developer: {new_game.developer}"
        )

        return new_game

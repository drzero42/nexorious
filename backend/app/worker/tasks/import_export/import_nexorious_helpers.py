"""Nexorious JSON import helpers.

Helper functions for processing Nexorious JSON exports.
Used by the import_nexorious_item task for fan-out processing.
"""

import logging
from datetime import date
from typing import Dict, Any, Optional, List
from decimal import Decimal

from sqlmodel import Session, select
from sqlalchemy.exc import IntegrityError

from app.models.game import Game
from app.models.user_game import UserGame, UserGamePlatform, PlayStatus, OwnershipStatus
from app.models.platform import Platform, Storefront
from app.models.tag import Tag, UserGameTag
from app.models.wishlist import Wishlist
from app.services.game_service import GameService

logger = logging.getLogger(__name__)

# Supported export versions
SUPPORTED_EXPORT_VERSIONS = ["1.0", "1.1", "1.2"]


async def _process_nexorious_game(
    session: Session,
    game_service: GameService,
    user_id: str,
    game_data: Dict[str, Any],
) -> str:
    """
    Process a single game from Nexorious export.

    Returns:
        Status string: "imported", "already_in_collection",
                      "skipped_no_igdb_id", "skipped_invalid", "error"
    """
    # Validate required fields
    title = game_data.get("title")
    if not title:
        logger.warning("Skipping game without title")
        return "skipped_invalid"

    igdb_id = game_data.get("igdb_id")
    if not igdb_id:
        logger.warning(f"Skipping game '{title}' without IGDB ID")
        return "skipped_no_igdb_id"

    try:
        igdb_id = int(igdb_id)
    except (ValueError, TypeError):
        logger.warning(f"Skipping game '{title}' with invalid IGDB ID: {igdb_id}")
        return "skipped_invalid"

    # Check if game already in user's collection
    existing_user_game = session.exec(
        select(UserGame).where(
            UserGame.user_id == user_id,
            UserGame.game_id == igdb_id,
        )
    ).first()

    if existing_user_game:
        logger.debug(f"Game '{title}' already in collection")
        return "already_in_collection"

    # Ensure game exists in our games table (fetch from IGDB if needed)
    game = session.get(Game, igdb_id)
    if not game:
        try:
            game = await game_service.create_or_update_game_from_igdb(igdb_id)
        except Exception as e:
            logger.error(f"Failed to fetch game '{title}' from IGDB: {e}")
            return "error"

    # Create UserGame with user data from export
    user_game = UserGame(
        user_id=user_id,
        game_id=igdb_id,
        play_status=_map_play_status(game_data.get("play_status")),
        ownership_status=_map_ownership_status(game_data.get("ownership_status")),
        personal_rating=_parse_rating(game_data.get("personal_rating")),
        is_loved=game_data.get("is_loved", False),
        hours_played=game_data.get("hours_played", 0),
        personal_notes=game_data.get("personal_notes"),
        acquired_date=_parse_date(game_data.get("acquired_date")),
    )
    session.add(user_game)
    try:
        session.commit()
    except IntegrityError:
        session.rollback()
        logger.info(f"Game '{title}' already in collection (caught by constraint)")
        return "already_in_collection"
    session.refresh(user_game)

    # Import platforms if present
    platforms_data = game_data.get("platforms", [])
    if platforms_data:
        await _import_platforms(session, user_game, platforms_data)

    # Import tags if present
    tags_data = game_data.get("tags", [])
    if tags_data:
        await _import_tags(session, user_game, user_id, tags_data)

    logger.debug(f"Imported game '{title}' (IGDB ID: {igdb_id})")
    return "imported"


async def _process_wishlist_item(
    session: Session,
    game_service: GameService,
    user_id: str,
    wishlist_data: Dict[str, Any],
) -> str:
    """
    Process a single wishlist item from Nexorious export.

    Returns:
        Status string: "imported", "already_exists", "skipped_invalid",
                      "skipped_no_igdb_id", "error"
    """
    # Validate required fields
    title = wishlist_data.get("title")
    if not title:
        logger.warning("Skipping wishlist item without title")
        return "skipped_invalid"

    igdb_id = wishlist_data.get("igdb_id")
    if not igdb_id:
        logger.warning(f"Skipping wishlist item '{title}' without IGDB ID")
        return "skipped_no_igdb_id"

    try:
        igdb_id = int(igdb_id)
    except (ValueError, TypeError):
        logger.warning(f"Skipping wishlist item '{title}' with invalid IGDB ID: {igdb_id}")
        return "skipped_invalid"

    # Check if already on wishlist
    existing_wishlist = session.exec(
        select(Wishlist).where(
            Wishlist.user_id == user_id,
            Wishlist.game_id == igdb_id,
        )
    ).first()

    if existing_wishlist:
        logger.debug(f"Wishlist item '{title}' already exists")
        return "already_exists"

    # Ensure game exists in our games table (fetch from IGDB if needed)
    game = session.get(Game, igdb_id)
    if not game:
        try:
            game = await game_service.create_or_update_game_from_igdb(igdb_id)
        except Exception as e:
            logger.error(f"Failed to fetch wishlist game '{title}' from IGDB: {e}")
            return "error"

    # Create wishlist entry
    wishlist_item = Wishlist(
        user_id=user_id,
        game_id=igdb_id,
    )
    session.add(wishlist_item)
    try:
        session.commit()
    except IntegrityError:
        session.rollback()
        logger.info(f"Wishlist item '{title}' already exists (caught by constraint)")
        return "already_exists"

    logger.debug(f"Imported wishlist item '{title}' (IGDB ID: {igdb_id})")
    return "imported"


async def _import_platforms(
    session: Session,
    user_game: UserGame,
    platforms_data: List[Dict[str, Any]],
) -> None:
    """Import platform associations for a user game."""
    # Track seen platform/storefront combinations to avoid duplicates
    seen_combinations: set[tuple[Optional[str], Optional[str]]] = set()

    for platform_data in platforms_data:
        platform_name = platform_data.get("platform_name") or platform_data.get("name")
        storefront_name = platform_data.get("storefront_name") or platform_data.get(
            "storefront"
        )

        # Try to resolve platform
        platform_id = None
        if platform_name:
            platform = session.exec(
                select(Platform).where(Platform.name == platform_name)
            ).first()
            if platform:
                platform_id = platform.id

        # Try to resolve storefront
        storefront_id = None
        if storefront_name:
            storefront = session.exec(
                select(Storefront).where(Storefront.name == storefront_name)
            ).first()
            if storefront:
                storefront_id = storefront.id

        # Skip duplicate platform/storefront combinations
        combination_key = (platform_id, storefront_id)
        if combination_key in seen_combinations:
            logger.debug(
                f"Skipping duplicate platform/storefront: {platform_name}/{storefront_name}"
            )
            continue
        seen_combinations.add(combination_key)

        # Create platform association
        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=platform_id,
            storefront_id=storefront_id,
            store_game_id=platform_data.get("store_game_id"),
            store_url=platform_data.get("store_url"),
            is_available=platform_data.get("is_available", True),
            original_platform_name=platform_name if not platform_id else None,
        )
        session.add(user_game_platform)

    session.commit()


async def _import_tags(
    session: Session,
    user_game: UserGame,
    user_id: str,
    tags_data: List[str],
) -> None:
    """Import tags for a user game."""
    for tag_name in tags_data:
        if not tag_name or not isinstance(tag_name, str):
            continue

        tag_name = tag_name.strip()
        if not tag_name:
            continue

        # Find or create tag for user
        tag = session.exec(
            select(Tag).where(Tag.user_id == user_id, Tag.name == tag_name)
        ).first()

        if not tag:
            tag = Tag(user_id=user_id, name=tag_name)
            session.add(tag)
            session.commit()
            session.refresh(tag)

        # Create tag association if it doesn't exist
        existing_assoc = session.exec(
            select(UserGameTag).where(
                UserGameTag.user_game_id == user_game.id,
                UserGameTag.tag_id == tag.id,
            )
        ).first()

        if not existing_assoc:
            user_game_tag = UserGameTag(
                user_game_id=user_game.id,
                tag_id=tag.id,
            )
            session.add(user_game_tag)

    session.commit()


def _map_play_status(status: Optional[str]) -> PlayStatus:
    """Map export play status to PlayStatus enum."""
    if not status:
        return PlayStatus.NOT_STARTED

    status_lower = status.lower().replace("-", "_").replace(" ", "_")
    status_mapping = {
        "not_started": PlayStatus.NOT_STARTED,
        "in_progress": PlayStatus.IN_PROGRESS,
        "completed": PlayStatus.COMPLETED,
        "mastered": PlayStatus.MASTERED,
        "dominated": PlayStatus.DOMINATED,
        "shelved": PlayStatus.SHELVED,
        "dropped": PlayStatus.DROPPED,
        "replay": PlayStatus.REPLAY,
        # Common aliases
        "playing": PlayStatus.IN_PROGRESS,
        "finished": PlayStatus.COMPLETED,
        "100%": PlayStatus.MASTERED,
        "abandoned": PlayStatus.DROPPED,
        "backlog": PlayStatus.NOT_STARTED,
    }
    return status_mapping.get(status_lower, PlayStatus.NOT_STARTED)


def _map_ownership_status(status: Optional[str]) -> OwnershipStatus:
    """Map export ownership status to OwnershipStatus enum."""
    if not status:
        return OwnershipStatus.OWNED

    status_lower = status.lower().replace("-", "_").replace(" ", "_")
    status_mapping = {
        "owned": OwnershipStatus.OWNED,
        "borrowed": OwnershipStatus.BORROWED,
        "rented": OwnershipStatus.RENTED,
        "subscription": OwnershipStatus.SUBSCRIPTION,
        "no_longer_owned": OwnershipStatus.NO_LONGER_OWNED,
        # Common aliases
        "gamepass": OwnershipStatus.SUBSCRIPTION,
        "game_pass": OwnershipStatus.SUBSCRIPTION,
        "ps_plus": OwnershipStatus.SUBSCRIPTION,
        "ps+": OwnershipStatus.SUBSCRIPTION,
        "sold": OwnershipStatus.NO_LONGER_OWNED,
    }
    return status_mapping.get(status_lower, OwnershipStatus.OWNED)


def _parse_rating(rating: Any) -> Optional[Decimal]:
    """Parse rating value to Decimal."""
    if rating is None:
        return None

    try:
        rating_decimal = Decimal(str(rating))
        # Clamp to valid range (0.0 - 10.0)
        if rating_decimal < 0:
            return Decimal("0.0")
        if rating_decimal > 10:
            return Decimal("10.0")
        return rating_decimal.quantize(Decimal("0.1"))
    except Exception:
        return None


def _parse_date(date_str: Any) -> Optional[date]:
    """Parse date string to date object."""
    if not date_str:
        return None

    if isinstance(date_str, date):
        return date_str

    try:
        # Handle ISO format (YYYY-MM-DD)
        if isinstance(date_str, str) and len(date_str) >= 10:
            return date.fromisoformat(date_str[:10])
    except (ValueError, TypeError):
        pass

    return None

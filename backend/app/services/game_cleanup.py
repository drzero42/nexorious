"""
Game cleanup service for automatic removal of unreferenced games.

This service provides functionality to automatically clean up games from the database
when they are no longer referenced by any users (not in any user's collection or wishlist).
It ensures data consistency by using proper database transactions and comprehensive logging.
"""

import logging
from typing import Optional
from sqlmodel import Session, select, func
from sqlalchemy.exc import SQLAlchemyError

from ..models.game import Game
from ..models.user_game import UserGame
from ..models.wishlist import Wishlist
from ..utils.sqlalchemy_typed import in_


logger = logging.getLogger(__name__)


def cleanup_unreferenced_game(game_id: int, session: Session) -> bool:
    """
    Clean up a game if it's no longer referenced by any users.
    
    This function performs reference counting to determine if a game is still
    in use by checking:
    - user_games table (user collection entries)
    - wishlists table (user wishlist entries)
    
    If the game has no references, it will be deleted.
    
    Args:
        game_id (int): The ID of the game to potentially clean up
        session (Session): Database session for operations
        
    Returns:
        bool: True if the game was deleted, False if it's still referenced or doesn't exist
        
    Raises:
        SQLAlchemyError: If database operations fail
    """
    try:
        logger.debug(f"Starting cleanup check for game ID: {game_id}")
        
        # First, verify the game exists
        game = session.get(Game, game_id)
        if not game:
            logger.debug(f"Game {game_id} not found in database - no cleanup needed")
            return False
        
        # Count references in user_games table
        user_game_count = session.exec(
            select(func.count()).where(UserGame.game_id == game_id)
        ).one()
        
        # Count references in wishlists table
        wishlist_count = session.exec(
            select(func.count()).where(Wishlist.game_id == game_id)
        ).one()
        
        total_references = user_game_count + wishlist_count
        
        logger.debug(
            f"Game {game_id} reference count: "
            f"{user_game_count} user_games, {wishlist_count} wishlists, "
            f"{total_references} total references"
        )
        
        # If game is still referenced, don't delete it
        if total_references > 0:
            logger.debug(
                f"Game {game_id} ('{game.title}') still has {total_references} references - "
                f"skipping cleanup"
            )
            return False
        
        # Game has no references - proceed with cleanup
        logger.info(
            f"Game {game_id} ('{game.title}') has no references - proceeding with cleanup"
        )
        
        # Delete the game
        session.delete(game)
        
        # Commit the transaction
        session.commit()
        
        logger.info(
            f"Successfully cleaned up game {game_id} ('{game.title}')"
        )
        
        return True
        
    except SQLAlchemyError as e:
        logger.error(
            f"Database error during cleanup of game {game_id}: {e}",
            exc_info=True
        )
        # Rollback the transaction on error
        session.rollback()
        raise
        
    except Exception as e:
        logger.error(
            f"Unexpected error during cleanup of game {game_id}: {e}",
            exc_info=True
        )
        # Rollback the transaction on error
        session.rollback()
        raise


def cleanup_multiple_games(game_ids: list[int], session: Session) -> dict[int, bool]:
    """
    Clean up multiple games in a batch operation.
    
    This function attempts to clean up multiple games, handling each one
    individually to ensure that failures don't affect other games.
    
    Args:
        game_ids (list[int]): List of game IDs to potentially clean up
        session (Session): Database session for operations
        
    Returns:
        dict[int, bool]: Dictionary mapping game IDs to cleanup success status
    """
    results = {}
    
    logger.info(f"Starting batch cleanup for {len(game_ids)} games")
    
    for game_id in game_ids:
        try:
            # Each game cleanup gets its own transaction handling
            results[game_id] = cleanup_unreferenced_game(game_id, session)
        except Exception as e:
            logger.warning(
                f"Failed to clean up game {game_id}: {e}",
                exc_info=True
            )
            results[game_id] = False
    
    cleaned_count = sum(1 for success in results.values() if success)
    logger.info(
        f"Batch cleanup completed: {cleaned_count}/{len(game_ids)} games cleaned up"
    )
    
    return results


def get_unreferenced_games(session: Session, limit: Optional[int] = None) -> list[Game]:
    """
    Find games that are not referenced by any users.
    
    This function identifies games that are candidates for cleanup by finding
    games that have no entries in user_games or wishlists tables.
    
    Args:
        session (Session): Database session for operations
        limit (Optional[int]): Maximum number of games to return
        
    Returns:
        list[Game]: List of unreferenced games
    """
    logger.debug("Finding unreferenced games")
    
    # Find games that are not in user_games
    subquery_user_games = select(UserGame.game_id).distinct()
    
    # Find games that are not in wishlists
    subquery_wishlists = select(Wishlist.game_id).distinct()
    
    # Find games that are not in either table
    query = select(Game).where(
        ~in_(Game.id, subquery_user_games),
        ~in_(Game.id, subquery_wishlists)
    )
    
    if limit:
        query = query.limit(limit)
    
    unreferenced_games = session.exec(query).all()
    
    logger.debug(f"Found {len(unreferenced_games)} unreferenced games")
    
    return unreferenced_games
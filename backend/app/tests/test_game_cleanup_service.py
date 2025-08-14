"""
Tests for the game cleanup service.
"""

import pytest
from datetime import date
from sqlmodel import Session, select

from ..services.game_cleanup import cleanup_unreferenced_game, get_unreferenced_games, cleanup_multiple_games
from ..models.game import Game, GameAlias
from ..models.user_game import UserGame, OwnershipStatus, PlayStatus
from ..models.wishlist import Wishlist
from ..models.user import User
from .integration_test_utils import (
    session_fixture as session,
    test_user_fixture as user
)


class TestGameCleanupService:
    """Test cases for the game cleanup service."""
    
    def test_cleanup_unreferenced_game_no_references(self, session: Session):
        """Test cleanup of a game with no references (should be deleted)."""
        # Create a game with no user_games or wishlist references
        game = Game(
            title="Unreferenced Game",
            igdb_id="12345",
            igdb_slug="unreferenced-game"
        )
        session.add(game)
        session.commit()
        session.refresh(game)
        
        # Add an alias to test cascade deletion
        alias = GameAlias(
            game_id=game.id,
            alias_title="Alternative Title",
            source="test"
        )
        session.add(alias)
        session.commit()
        
        # Verify the game and alias exist
        assert session.get(Game, game.id) is not None
        assert session.exec(select(GameAlias).where(GameAlias.game_id == game.id)).first() is not None
        
        # Run cleanup
        result = cleanup_unreferenced_game(game.id, session)
        
        # Verify game was deleted
        assert result is True
        assert session.get(Game, game.id) is None
        assert session.exec(select(GameAlias).where(GameAlias.game_id == game.id)).first() is None
    
    def test_cleanup_unreferenced_game_with_user_game(self, session: Session, test_user: User):
        """Test cleanup of a game that has user_game references (should not be deleted)."""
        # Create a game
        game = Game(
            title="Referenced Game",
            igdb_id="12346",
            igdb_slug="referenced-game"
        )
        session.add(game)
        session.commit()
        session.refresh(game)
        
        # Create a user_game reference
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.NOT_STARTED
        )
        session.add(user_game)
        session.commit()
        
        # Run cleanup
        result = cleanup_unreferenced_game(game.id, session)
        
        # Verify game was NOT deleted
        assert result is False
        assert session.get(Game, game.id) is not None
    
    def test_cleanup_unreferenced_game_with_wishlist(self, session: Session, test_user: User):
        """Test cleanup of a game that has wishlist references (should not be deleted)."""
        # Create a game
        game = Game(
            title="Wishlisted Game",
            igdb_id="12347",
            igdb_slug="wishlisted-game"
        )
        session.add(game)
        session.commit()
        session.refresh(game)
        
        # Create a wishlist reference
        wishlist = Wishlist(
            user_id=test_user.id,
            game_id=game.id
        )
        session.add(wishlist)
        session.commit()
        
        # Run cleanup
        result = cleanup_unreferenced_game(game.id, session)
        
        # Verify game was NOT deleted
        assert result is False
        assert session.get(Game, game.id) is not None
    
    def test_cleanup_nonexistent_game(self, session: Session):
        """Test cleanup of a game that doesn't exist."""
        nonexistent_id = "nonexistent-game-id"
        
        # Run cleanup
        result = cleanup_unreferenced_game(nonexistent_id, session)
        
        # Verify cleanup returns False
        assert result is False
    
    def test_get_unreferenced_games(self, session: Session, test_user: User):
        """Test finding unreferenced games."""
        # Create a referenced game (with user_game)
        referenced_game = Game(
            title="Referenced Game",
            igdb_id="12348",
            igdb_slug="referenced-game"
        )
        session.add(referenced_game)
        session.commit()
        session.refresh(referenced_game)
        
        user_game = UserGame(
            user_id=test_user.id,
            game_id=referenced_game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.NOT_STARTED
        )
        session.add(user_game)
        
        # Create an unreferenced game
        unreferenced_game = Game(
            title="Unreferenced Game",
            igdb_id="12349",
            igdb_slug="unreferenced-game"
        )
        session.add(unreferenced_game)
        session.commit()
        
        # Get unreferenced games
        unreferenced = get_unreferenced_games(session)
        
        # Should only include the unreferenced game
        unreferenced_ids = [game.id for game in unreferenced]
        assert unreferenced_game.id in unreferenced_ids
        assert referenced_game.id not in unreferenced_ids
    
    def test_cleanup_multiple_games(self, session: Session, test_user: User):
        """Test batch cleanup of multiple games."""
        # Create two unreferenced games
        unreferenced_game1 = Game(
            title="Unreferenced Game 1",
            igdb_id="12350",
            igdb_slug="unreferenced-game-1"
        )
        unreferenced_game2 = Game(
            title="Unreferenced Game 2",
            igdb_id="12351",
            igdb_slug="unreferenced-game-2"
        )
        
        # Create one referenced game
        referenced_game = Game(
            title="Referenced Game",
            igdb_id="12352",
            igdb_slug="referenced-game"
        )
        
        session.add_all([unreferenced_game1, unreferenced_game2, referenced_game])
        session.commit()
        
        # Add reference to the third game
        user_game = UserGame(
            user_id=test_user.id,
            game_id=referenced_game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.NOT_STARTED
        )
        session.add(user_game)
        session.commit()
        
        # Run batch cleanup
        results = cleanup_multiple_games([
            unreferenced_game1.id,
            unreferenced_game2.id, 
            referenced_game.id
        ], session)
        
        # Verify results
        assert results[unreferenced_game1.id] is True  # Should be deleted
        assert results[unreferenced_game2.id] is True  # Should be deleted
        assert results[referenced_game.id] is False    # Should NOT be deleted
        
        # Verify actual deletion
        assert session.get(Game, unreferenced_game1.id) is None
        assert session.get(Game, unreferenced_game2.id) is None
        assert session.get(Game, referenced_game.id) is not None
    
    def test_cleanup_with_limit(self, session: Session):
        """Test finding unreferenced games with limit parameter."""
        # Create multiple unreferenced games
        games = []
        for i in range(5):
            game = Game(
                title=f"Unreferenced Game {i}",
                igdb_id=f"1235{i}",
                igdb_slug=f"unreferenced-game-{i}"
            )
            games.append(game)
            session.add(game)
        
        session.commit()
        
        # Get unreferenced games with limit
        unreferenced = get_unreferenced_games(session, limit=3)
        
        # Should return exactly 3 games
        assert len(unreferenced) == 3
        
        # All returned games should be from our test games
        returned_ids = [game.id for game in unreferenced]
        created_ids = [game.id for game in games]
        
        for returned_id in returned_ids:
            assert returned_id in created_ids
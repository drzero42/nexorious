"""Tests for UserGame unique constraint on (user_id, game_id)."""

import pytest
from sqlalchemy.exc import IntegrityError
from sqlmodel import Session

from app.models.user_game import UserGame
from app.models.game import Game


class TestUserGameUniqueConstraint:
    """Test unique constraint prevents duplicate user-game combinations."""

    @pytest.fixture
    def second_game(self, session: Session) -> Game:
        """Create a second test game."""
        game = Game(
            id=99999,
            title="Second Test Game",
            igdb_id=99999,
        )
        session.add(game)
        session.commit()
        session.refresh(game)
        return game

    def test_create_user_game_succeeds(self, session: Session, test_user, test_game):
        """Creating a UserGame succeeds normally."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        assert user_game.id is not None
        assert user_game.user_id == test_user.id
        assert user_game.game_id == test_game.id

    def test_duplicate_user_game_raises_integrity_error(
        self, session: Session, test_user, test_game
    ):
        """Creating duplicate (user_id, game_id) raises IntegrityError."""
        # Create first user_game
        user_game1 = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game1)
        session.commit()

        # Try to create duplicate
        user_game2 = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game2)

        with pytest.raises(IntegrityError):
            session.commit()

        session.rollback()

    def test_same_game_different_users_allowed(
        self, session: Session, test_user, admin_user, test_game
    ):
        """Same game can be owned by different users."""
        user_game1 = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        user_game2 = UserGame(
            user_id=admin_user.id,
            game_id=test_game.id,
        )
        session.add(user_game1)
        session.add(user_game2)
        session.commit()

        # Both should exist
        session.refresh(user_game1)
        session.refresh(user_game2)
        assert user_game1.id != user_game2.id

    def test_same_user_different_games_allowed(
        self, session: Session, test_user, test_game, second_game
    ):
        """Same user can own different games."""
        user_game1 = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        user_game2 = UserGame(
            user_id=test_user.id,
            game_id=second_game.id,
        )
        session.add(user_game1)
        session.add(user_game2)
        session.commit()

        # Both should exist
        session.refresh(user_game1)
        session.refresh(user_game2)
        assert user_game1.id != user_game2.id

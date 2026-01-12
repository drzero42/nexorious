"""Tests for UserGamePlatform external_game link."""

import pytest
from sqlmodel import Session

from app.models.user_game import UserGame, UserGamePlatform
from app.models.external_game import ExternalGame


class TestUserGamePlatformExternalLink:
    """Tests for UserGamePlatform external_game_id field."""

    def test_user_game_platform_with_external_game(
        self, session: Session, test_user, test_game, steam_dependencies
    ):
        """Test linking UserGamePlatform to ExternalGame."""
        # steam_dependencies sets up the steam storefront and pc-windows platform

        # Create ExternalGame
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
            resolved_igdb_id=test_game.id,
        )
        session.add(external_game)
        session.commit()
        session.refresh(external_game)

        # Create UserGame
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        # Create UserGamePlatform linked to ExternalGame
        platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform="pc-windows",
            storefront="steam",
            external_game_id=external_game.id,
        )
        session.add(platform)
        session.commit()
        session.refresh(platform)

        assert platform.external_game_id == external_game.id
        assert platform.sync_from_source is True  # Default

    def test_user_game_platform_without_external_game(
        self, session: Session, test_user, test_game, steam_dependencies
    ):
        """Test UserGamePlatform without ExternalGame (manual entry)."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform="pc-windows",
            storefront="steam",
        )
        session.add(platform)
        session.commit()
        session.refresh(platform)

        assert platform.external_game_id is None
        assert platform.sync_from_source is True

    def test_sync_from_source_flag(
        self, session: Session, test_user, test_game, steam_dependencies
    ):
        """Test sync_from_source flag can be set to False."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
        )
        session.add(user_game)
        session.commit()

        platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform="pc-windows",
            storefront="steam",
            sync_from_source=False,
        )
        session.add(platform)
        session.commit()
        session.refresh(platform)

        assert platform.sync_from_source is False

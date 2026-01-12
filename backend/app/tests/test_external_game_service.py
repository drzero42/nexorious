"""Tests for ExternalGameService."""

import pytest
from sqlmodel import Session, select
from unittest.mock import MagicMock

from app.services.external_game_service import ExternalGameService
from app.models.external_game import ExternalGame
from app.models.user_game import UserGame, UserGamePlatform, OwnershipStatus


class TestExternalGameService:
    """Tests for ExternalGameService."""

    def test_create_or_update_creates_new(self, session: Session, test_user, steam_dependencies):
        """Test creating a new ExternalGame."""
        service = ExternalGameService(session)

        external_game = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
            platform="pc-windows",
            playtime_hours=10,
        )

        assert external_game.id is not None
        assert external_game.title == "Test Game"
        assert external_game.playtime_hours == 10
        assert external_game.is_available is True

    def test_create_or_update_updates_existing(self, session: Session, test_user, steam_dependencies):
        """Test updating an existing ExternalGame."""
        service = ExternalGameService(session)

        # Create initial
        external_game1 = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
            playtime_hours=10,
        )

        # Update
        external_game2 = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game Updated",
            playtime_hours=20,
        )

        assert external_game1.id == external_game2.id
        assert external_game2.title == "Test Game Updated"
        assert external_game2.playtime_hours == 20

    def test_mark_unavailable(self, session: Session, test_user, steam_dependencies):
        """Test marking games as unavailable."""
        service = ExternalGameService(session)

        # Create two games
        service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="111",
            title="Game 1",
        )
        service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="222",
            title="Game 2",
        )

        # Mark games not in the set as unavailable
        service.mark_unavailable_except(
            user_id=test_user.id,
            storefront="steam",
            available_external_ids={"111"},  # Only 111 is available
        )

        games = session.exec(
            select(ExternalGame).where(ExternalGame.user_id == test_user.id)
        ).all()

        game1 = next(g for g in games if g.external_id == "111")
        game2 = next(g for g in games if g.external_id == "222")

        assert game1.is_available is True
        assert game2.is_available is False

    def test_get_unresolved(self, session: Session, test_user, test_game, steam_dependencies):
        """Test getting unresolved games."""
        service = ExternalGameService(session)

        # Create resolved game
        resolved = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="111",
            title="Resolved",
        )
        resolved.resolved_igdb_id = test_game.id
        session.add(resolved)
        session.commit()

        # Create unresolved game
        service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="222",
            title="Unresolved",
        )

        # Create skipped game
        skipped = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="333",
            title="Skipped",
        )
        skipped.is_skipped = True
        session.add(skipped)
        session.commit()

        unresolved = service.get_unresolved(test_user.id, "steam")

        assert len(unresolved) == 1
        assert unresolved[0].external_id == "222"

    def test_resolve_igdb_id(self, session: Session, test_user, test_game, steam_dependencies):
        """Test resolving IGDB ID."""
        service = ExternalGameService(session)

        external_game = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )

        service.resolve_igdb_id(external_game.id, test_game.id)

        session.refresh(external_game)
        assert external_game.resolved_igdb_id == test_game.id

    def test_skip_game(self, session: Session, test_user, steam_dependencies):
        """Test skipping a game."""
        service = ExternalGameService(session)

        external_game = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )

        service.skip(external_game.id)

        session.refresh(external_game)
        assert external_game.is_skipped is True

    def test_unskip_game(self, session: Session, test_user, steam_dependencies):
        """Test unskipping a game."""
        service = ExternalGameService(session)

        external_game = service.create_or_update(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )
        external_game.is_skipped = True
        session.add(external_game)
        session.commit()

        service.unskip(external_game.id)

        session.refresh(external_game)
        assert external_game.is_skipped is False

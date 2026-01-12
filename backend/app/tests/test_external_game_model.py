"""Tests for ExternalGame model."""

import pytest
from sqlmodel import Session

from app.models.external_game import ExternalGame
from app.models.user_game import OwnershipStatus
from app.models.platform import Storefront, Platform


@pytest.fixture
def external_game_dependencies(session: Session) -> dict:
    """Create storefronts and platforms required for ExternalGame tests."""
    # Create storefronts
    storefronts = [
        Storefront(
            name="steam",
            display_name="Steam",
            base_url="https://store.steampowered.com",
            is_active=True,
        ),
        Storefront(
            name="epic",
            display_name="Epic Games Store",
            base_url="https://store.epicgames.com",
            is_active=True,
        ),
        Storefront(
            name="psn",
            display_name="PlayStation Store",
            base_url="https://store.playstation.com",
            is_active=True,
        ),
        Storefront(
            name="gog",
            display_name="GOG",
            base_url="https://www.gog.com",
            is_active=True,
        ),
    ]
    for storefront in storefronts:
        session.add(storefront)

    # Create platform
    platform = Platform(
        name="pc-windows",
        display_name="PC (Windows)",
        is_active=True,
    )
    session.add(platform)
    session.commit()

    return {
        "storefronts": {s.name: s for s in storefronts},
        "platform": platform,
    }


class TestExternalGameModel:
    """Tests for ExternalGame SQLModel."""

    def test_create_external_game(self, session: Session, test_user, external_game_dependencies):
        """Test creating an ExternalGame record."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
            platform="pc-windows",
        )
        session.add(external_game)
        session.commit()
        session.refresh(external_game)

        assert external_game.id is not None
        assert external_game.user_id == test_user.id
        assert external_game.storefront == "steam"
        assert external_game.external_id == "12345"
        assert external_game.title == "Test Game"
        assert external_game.resolved_igdb_id is None
        assert external_game.is_skipped is False
        assert external_game.is_available is True
        assert external_game.is_subscription is False
        assert external_game.playtime_hours == 0

    def test_unique_constraint(self, session: Session, test_user, external_game_dependencies):
        """Test unique constraint on user_id, storefront, external_id."""
        external_game1 = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )
        session.add(external_game1)
        session.commit()

        external_game2 = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game Duplicate",
        )
        session.add(external_game2)

        with pytest.raises(Exception):  # IntegrityError
            session.commit()

    def test_store_url_steam(self, session: Session, test_user, external_game_dependencies):
        """Test store_url computed property for Steam."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="12345",
            title="Test Game",
        )
        session.add(external_game)
        session.commit()

        assert external_game.store_url == "https://store.steampowered.com/app/12345"

    def test_store_url_epic(self, session: Session, test_user, external_game_dependencies):
        """Test store_url computed property for Epic."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="epic",
            external_id="fortnite",
            title="Fortnite",
        )
        session.add(external_game)
        session.commit()

        assert external_game.store_url == "https://store.epicgames.com/p/fortnite"

    def test_store_url_psn(self, session: Session, test_user, external_game_dependencies):
        """Test store_url computed property for PSN."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="psn",
            external_id="UP0001-CUSA00001_00-TESTGAME00000001",
            title="Test Game",
        )
        session.add(external_game)
        session.commit()

        assert external_game.store_url == "https://store.playstation.com/product/UP0001-CUSA00001_00-TESTGAME00000001"

    def test_store_url_gog(self, session: Session, test_user, external_game_dependencies):
        """Test store_url computed property for GOG."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="gog",
            external_id="1234567890",
            title="Test Game",
        )
        session.add(external_game)
        session.commit()

        assert external_game.store_url == "https://www.gog.com/game/1234567890"

    def test_with_ownership_status(self, session: Session, test_user, external_game_dependencies):
        """Test ExternalGame with ownership status."""
        external_game = ExternalGame(
            user_id=test_user.id,
            storefront="psn",
            external_id="12345",
            title="PS Plus Game",
            ownership_status=OwnershipStatus.SUBSCRIPTION,
            is_subscription=True,
        )
        session.add(external_game)
        session.commit()
        session.refresh(external_game)

        assert external_game.ownership_status == OwnershipStatus.SUBSCRIPTION
        assert external_game.is_subscription is True

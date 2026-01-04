"""Tests for unknown platform/storefront handling during import."""

import pytest

from sqlmodel import Session, select

from app.worker.tasks.import_export.import_nexorious_helpers import _import_platforms
from app.models.user_game import UserGame, UserGamePlatform, PlayStatus, OwnershipStatus
from app.models.platform import Platform, Storefront


class TestImportUnknownPlatformStorefront:
    """Test handling of unknown platforms and storefronts during import."""

    @pytest.fixture
    def user_game(self, session: Session, test_user, test_game) -> UserGame:
        """Create a user game for testing."""
        user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
            play_status=PlayStatus.NOT_STARTED,
            ownership_status=OwnershipStatus.OWNED,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)
        return user_game

    @pytest.mark.asyncio
    async def test_original_storefront_name_stored_when_storefront_not_resolved(
        self, session: Session, user_game: UserGame, test_platform: Platform
    ):
        """When storefront doesn't resolve, original_storefront_name is stored."""
        platforms_data = [
            {
                "platform_name": test_platform.name,  # This will resolve
                "storefront_name": "Unknown Storefront XYZ",  # This won't resolve
            }
        ]

        await _import_platforms(session, user_game, platforms_data)

        # Find the created platform association
        platform_assoc = session.exec(
            select(UserGamePlatform).where(
                UserGamePlatform.user_game_id == user_game.id
            )
        ).first()

        assert platform_assoc is not None
        # Platform should resolve
        assert platform_assoc.platform == test_platform.name
        assert platform_assoc.original_platform_name is None
        # Storefront should not resolve, so original name is stored
        assert platform_assoc.storefront is None
        assert platform_assoc.original_storefront_name == "Unknown Storefront XYZ"

    @pytest.mark.asyncio
    async def test_original_storefront_name_none_when_storefront_resolves(
        self, session: Session, user_game: UserGame, test_platform: Platform, test_storefront: Storefront
    ):
        """When storefront resolves, original_storefront_name is None."""
        platforms_data = [
            {
                "platform_name": test_platform.name,  # This will resolve
                "storefront_name": test_storefront.name,  # This will resolve
            }
        ]

        await _import_platforms(session, user_game, platforms_data)

        # Find the created platform association
        platform_assoc = session.exec(
            select(UserGamePlatform).where(
                UserGamePlatform.user_game_id == user_game.id
            )
        ).first()

        assert platform_assoc is not None
        # Platform should resolve
        assert platform_assoc.platform == test_platform.name
        assert platform_assoc.original_platform_name is None
        # Storefront should resolve
        assert platform_assoc.storefront == test_storefront.name
        assert platform_assoc.original_storefront_name is None

    @pytest.mark.asyncio
    async def test_both_original_names_stored_when_neither_resolves(
        self, session: Session, user_game: UserGame
    ):
        """When neither platform nor storefront resolves, both original names are stored."""
        platforms_data = [
            {
                "platform_name": "Unknown Platform ABC",  # This won't resolve
                "storefront_name": "Unknown Storefront XYZ",  # This won't resolve
            }
        ]

        await _import_platforms(session, user_game, platforms_data)

        # Find the created platform association
        platform_assoc = session.exec(
            select(UserGamePlatform).where(
                UserGamePlatform.user_game_id == user_game.id
            )
        ).first()

        assert platform_assoc is not None
        # Platform should not resolve
        assert platform_assoc.platform is None
        assert platform_assoc.original_platform_name == "Unknown Platform ABC"
        # Storefront should not resolve
        assert platform_assoc.storefront is None
        assert platform_assoc.original_storefront_name == "Unknown Storefront XYZ"

    @pytest.mark.asyncio
    async def test_no_original_names_when_both_resolve(
        self, session: Session, user_game: UserGame, test_platform: Platform, test_storefront: Storefront
    ):
        """When both platform and storefront resolve, no original names are stored."""
        platforms_data = [
            {
                "platform_name": test_platform.name,  # This will resolve
                "storefront_name": test_storefront.name,  # This will resolve
            }
        ]

        await _import_platforms(session, user_game, platforms_data)

        # Find the created platform association
        platform_assoc = session.exec(
            select(UserGamePlatform).where(
                UserGamePlatform.user_game_id == user_game.id
            )
        ).first()

        assert platform_assoc is not None
        # Both should resolve
        assert platform_assoc.platform == test_platform.name
        assert platform_assoc.storefront == test_storefront.name
        # Neither original name should be set
        assert platform_assoc.original_platform_name is None
        assert platform_assoc.original_storefront_name is None

    @pytest.mark.asyncio
    async def test_original_platform_name_stored_when_platform_not_resolved(
        self, session: Session, user_game: UserGame, test_storefront: Storefront
    ):
        """When platform doesn't resolve but storefront does, only original_platform_name is stored."""
        platforms_data = [
            {
                "platform_name": "Unknown Platform ABC",  # This won't resolve
                "storefront_name": test_storefront.name,  # This will resolve
            }
        ]

        await _import_platforms(session, user_game, platforms_data)

        # Find the created platform association
        platform_assoc = session.exec(
            select(UserGamePlatform).where(
                UserGamePlatform.user_game_id == user_game.id
            )
        ).first()

        assert platform_assoc is not None
        # Platform should not resolve
        assert platform_assoc.platform is None
        assert platform_assoc.original_platform_name == "Unknown Platform ABC"
        # Storefront should resolve
        assert platform_assoc.storefront == test_storefront.name
        assert platform_assoc.original_storefront_name is None

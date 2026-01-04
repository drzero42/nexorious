"""
Tests for unknown platform/storefront filter functionality.

These tests verify that the API correctly handles:
- ?platform=unknown: filters for games with NULL platform
- ?storefront=unknown: filters for games with NULL storefront
- Combined filters: ?platform=unknown&platform=steam returns both NULL and steam platforms
"""

from fastapi.testclient import TestClient
from sqlmodel import Session, select
from typing import Dict

from ..models.user_game import UserGame, UserGamePlatform
from ..models.user import User
from ..models.game import Game
from ..models.platform import Platform, Storefront
from .integration_test_utils import (
    create_test_game,
    register_and_login_user,
    assert_api_success,
)


class TestUnknownPlatformFilter:
    """Test filtering by platform=unknown (NULL platform)."""

    def test_filter_by_unknown_platform_returns_null_platform_games(
        self, client: TestClient, session: Session
    ):
        """Test that ?platform=unknown returns games with NULL platform."""
        # Create test user
        user_data = {"username": "unknownplatformuser", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user
        user = session.exec(
            select(User).where(User.username == "unknownplatformuser")
        ).first()
        assert user is not None

        # Create a known platform
        platform_pc = Platform(
            name="test-pc-unknown", display_name="PC", is_active=True
        )
        session.add(platform_pc)
        session.commit()

        # Create two games
        game_with_platform = create_test_game(
            igdb_id=8001, title="Game With Platform"
        )
        game_without_platform = create_test_game(
            igdb_id=8002, title="Game Without Platform"
        )
        session.add_all([game_with_platform, game_without_platform])
        session.commit()
        session.refresh(game_with_platform)
        session.refresh(game_without_platform)

        # Create user games
        ug_with_platform = UserGame(
            user_id=user.id,
            game_id=game_with_platform.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_without_platform = UserGame(
            user_id=user.id,
            game_id=game_without_platform.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add_all([ug_with_platform, ug_without_platform])
        session.commit()
        session.refresh(ug_with_platform)
        session.refresh(ug_without_platform)

        # Add platform association to one game (with known platform)
        ugp = UserGamePlatform(
            user_game_id=ug_with_platform.id,
            platform=platform_pc.name,
            storefront=None,
        )
        session.add(ugp)

        # Add platform association with NULL platform to another game
        ugp_null = UserGamePlatform(
            user_game_id=ug_without_platform.id,
            platform=None,
            storefront=None,
        )
        session.add(ugp_null)
        session.commit()

        # Test: filter by unknown platform should return only the game with NULL platform
        response = client.get(
            "/api/user-games/?platform=unknown", headers=auth_headers
        )
        assert_api_success(response, 200)
        data = response.json()

        assert data["total"] == 1
        assert len(data["user_games"]) == 1
        assert data["user_games"][0]["game"]["title"] == "Game Without Platform"

    def test_filter_by_known_platform_excludes_null_platform(
        self, client: TestClient, session: Session
    ):
        """Test that filtering by a known platform does not return NULL platform games."""
        # Create test user
        user_data = {"username": "knownplatformuser", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user
        user = session.exec(
            select(User).where(User.username == "knownplatformuser")
        ).first()
        assert user is not None

        # Create a known platform
        platform_pc = Platform(
            name="test-pc-known", display_name="PC", is_active=True
        )
        session.add(platform_pc)
        session.commit()

        # Create two games
        game_with_platform = create_test_game(
            igdb_id=8011, title="Game With Known Platform"
        )
        game_with_null = create_test_game(
            igdb_id=8012, title="Game With Null Platform"
        )
        session.add_all([game_with_platform, game_with_null])
        session.commit()
        session.refresh(game_with_platform)
        session.refresh(game_with_null)

        # Create user games
        ug_with_platform = UserGame(
            user_id=user.id,
            game_id=game_with_platform.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_with_null = UserGame(
            user_id=user.id,
            game_id=game_with_null.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add_all([ug_with_platform, ug_with_null])
        session.commit()
        session.refresh(ug_with_platform)
        session.refresh(ug_with_null)

        # Add platform associations
        ugp_known = UserGamePlatform(
            user_game_id=ug_with_platform.id,
            platform=platform_pc.name,
            storefront=None,
        )
        ugp_null = UserGamePlatform(
            user_game_id=ug_with_null.id,
            platform=None,
            storefront=None,
        )
        session.add_all([ugp_known, ugp_null])
        session.commit()

        # Test: filter by known platform should return only known platform game
        response = client.get(
            f"/api/user-games/?platform={platform_pc.name}", headers=auth_headers
        )
        assert_api_success(response, 200)
        data = response.json()

        assert data["total"] == 1
        assert len(data["user_games"]) == 1
        assert data["user_games"][0]["game"]["title"] == "Game With Known Platform"

    def test_filter_by_unknown_and_known_platform_returns_both(
        self, client: TestClient, session: Session
    ):
        """Test that ?platform=unknown&platform=steam returns both NULL and steam platforms."""
        # Create test user
        user_data = {"username": "mixedplatformuser", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user
        user = session.exec(
            select(User).where(User.username == "mixedplatformuser")
        ).first()
        assert user is not None

        # Create platforms
        platform_steam = Platform(
            name="test-steam-mixed", display_name="Steam", is_active=True
        )
        platform_gog = Platform(
            name="test-gog-mixed", display_name="GOG", is_active=True
        )
        session.add_all([platform_steam, platform_gog])
        session.commit()

        # Create three games
        game_steam = create_test_game(igdb_id=8021, title="Steam Game")
        game_gog = create_test_game(igdb_id=8022, title="GOG Game")
        game_unknown = create_test_game(igdb_id=8023, title="Unknown Platform Game")
        session.add_all([game_steam, game_gog, game_unknown])
        session.commit()
        session.refresh(game_steam)
        session.refresh(game_gog)
        session.refresh(game_unknown)

        # Create user games
        ug_steam = UserGame(
            user_id=user.id,
            game_id=game_steam.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_gog = UserGame(
            user_id=user.id,
            game_id=game_gog.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_unknown = UserGame(
            user_id=user.id,
            game_id=game_unknown.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add_all([ug_steam, ug_gog, ug_unknown])
        session.commit()
        session.refresh(ug_steam)
        session.refresh(ug_gog)
        session.refresh(ug_unknown)

        # Add platform associations
        ugp_steam = UserGamePlatform(
            user_game_id=ug_steam.id,
            platform=platform_steam.name,
            storefront=None,
        )
        ugp_gog = UserGamePlatform(
            user_game_id=ug_gog.id,
            platform=platform_gog.name,
            storefront=None,
        )
        ugp_unknown = UserGamePlatform(
            user_game_id=ug_unknown.id,
            platform=None,
            storefront=None,
        )
        session.add_all([ugp_steam, ugp_gog, ugp_unknown])
        session.commit()

        # Test: filter by unknown AND steam should return both
        response = client.get(
            f"/api/user-games/?platform=unknown&platform={platform_steam.name}",
            headers=auth_headers,
        )
        assert_api_success(response, 200)
        data = response.json()

        assert data["total"] == 2
        titles = sorted([ug["game"]["title"] for ug in data["user_games"]])
        assert titles == ["Steam Game", "Unknown Platform Game"]


class TestUnknownStorefrontFilter:
    """Test filtering by storefront=unknown (NULL storefront)."""

    def test_filter_by_unknown_storefront_returns_null_storefront_games(
        self, client: TestClient, session: Session
    ):
        """Test that ?storefront=unknown returns games with NULL storefront."""
        # Create test user
        user_data = {"username": "unknownstorefrontuser", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user
        user = session.exec(
            select(User).where(User.username == "unknownstorefrontuser")
        ).first()
        assert user is not None

        # Create a platform (required for UserGamePlatform)
        platform_pc = Platform(
            name="test-pc-storefront", display_name="PC", is_active=True
        )
        # Create a storefront
        storefront_steam = Storefront(
            name="test-steam-storefront",
            display_name="Steam",
            base_url="https://store.steampowered.com",
            is_active=True,
        )
        session.add_all([platform_pc, storefront_steam])
        session.commit()

        # Create two games
        game_with_storefront = create_test_game(
            igdb_id=8031, title="Game With Storefront"
        )
        game_without_storefront = create_test_game(
            igdb_id=8032, title="Game Without Storefront"
        )
        session.add_all([game_with_storefront, game_without_storefront])
        session.commit()
        session.refresh(game_with_storefront)
        session.refresh(game_without_storefront)

        # Create user games
        ug_with_storefront = UserGame(
            user_id=user.id,
            game_id=game_with_storefront.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_without_storefront = UserGame(
            user_id=user.id,
            game_id=game_without_storefront.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add_all([ug_with_storefront, ug_without_storefront])
        session.commit()
        session.refresh(ug_with_storefront)
        session.refresh(ug_without_storefront)

        # Add platform associations
        ugp_with_storefront = UserGamePlatform(
            user_game_id=ug_with_storefront.id,
            platform=platform_pc.name,
            storefront=storefront_steam.name,
        )
        ugp_without_storefront = UserGamePlatform(
            user_game_id=ug_without_storefront.id,
            platform=platform_pc.name,
            storefront=None,
        )
        session.add_all([ugp_with_storefront, ugp_without_storefront])
        session.commit()

        # Test: filter by unknown storefront should return only the game with NULL storefront
        response = client.get(
            "/api/user-games/?storefront=unknown", headers=auth_headers
        )
        assert_api_success(response, 200)
        data = response.json()

        assert data["total"] == 1
        assert len(data["user_games"]) == 1
        assert data["user_games"][0]["game"]["title"] == "Game Without Storefront"

    def test_filter_by_unknown_and_known_storefront_returns_both(
        self, client: TestClient, session: Session
    ):
        """Test that ?storefront=unknown&storefront=steam returns both NULL and steam storefronts."""
        # Create test user
        user_data = {"username": "mixedstorefrontuser", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user
        user = session.exec(
            select(User).where(User.username == "mixedstorefrontuser")
        ).first()
        assert user is not None

        # Create platform and storefronts
        platform_pc = Platform(
            name="test-pc-mixed-sf", display_name="PC", is_active=True
        )
        storefront_steam = Storefront(
            name="test-steam-mixed-sf",
            display_name="Steam",
            base_url="https://store.steampowered.com",
            is_active=True,
        )
        storefront_gog = Storefront(
            name="test-gog-mixed-sf",
            display_name="GOG",
            base_url="https://gog.com",
            is_active=True,
        )
        session.add_all([platform_pc, storefront_steam, storefront_gog])
        session.commit()

        # Create three games
        game_steam = create_test_game(igdb_id=8041, title="Steam Storefront Game")
        game_gog = create_test_game(igdb_id=8042, title="GOG Storefront Game")
        game_unknown = create_test_game(igdb_id=8043, title="Unknown Storefront Game")
        session.add_all([game_steam, game_gog, game_unknown])
        session.commit()
        session.refresh(game_steam)
        session.refresh(game_gog)
        session.refresh(game_unknown)

        # Create user games
        ug_steam = UserGame(
            user_id=user.id,
            game_id=game_steam.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_gog = UserGame(
            user_id=user.id,
            game_id=game_gog.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_unknown = UserGame(
            user_id=user.id,
            game_id=game_unknown.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add_all([ug_steam, ug_gog, ug_unknown])
        session.commit()
        session.refresh(ug_steam)
        session.refresh(ug_gog)
        session.refresh(ug_unknown)

        # Add platform associations with different storefronts
        ugp_steam = UserGamePlatform(
            user_game_id=ug_steam.id,
            platform=platform_pc.name,
            storefront=storefront_steam.name,
        )
        ugp_gog = UserGamePlatform(
            user_game_id=ug_gog.id,
            platform=platform_pc.name,
            storefront=storefront_gog.name,
        )
        ugp_unknown = UserGamePlatform(
            user_game_id=ug_unknown.id,
            platform=platform_pc.name,
            storefront=None,
        )
        session.add_all([ugp_steam, ugp_gog, ugp_unknown])
        session.commit()

        # Test: filter by unknown AND steam should return both
        response = client.get(
            f"/api/user-games/?storefront=unknown&storefront={storefront_steam.name}",
            headers=auth_headers,
        )
        assert_api_success(response, 200)
        data = response.json()

        assert data["total"] == 2
        titles = sorted([ug["game"]["title"] for ug in data["user_games"]])
        assert titles == ["Steam Storefront Game", "Unknown Storefront Game"]


class TestUnknownFilterOnIdsEndpoint:
    """Test unknown platform/storefront filters on /api/user-games/ids endpoint."""

    def test_ids_endpoint_filter_by_unknown_platform(
        self, client: TestClient, session: Session
    ):
        """Test that /api/user-games/ids?platform=unknown returns IDs of games with NULL platform."""
        # Create test user
        user_data = {"username": "idsunknownplatform", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user
        user = session.exec(
            select(User).where(User.username == "idsunknownplatform")
        ).first()
        assert user is not None

        # Create a known platform
        platform_pc = Platform(
            name="test-pc-ids", display_name="PC", is_active=True
        )
        session.add(platform_pc)
        session.commit()

        # Create two games
        game_with_platform = create_test_game(
            igdb_id=8051, title="IDs Game With Platform"
        )
        game_without_platform = create_test_game(
            igdb_id=8052, title="IDs Game Without Platform"
        )
        session.add_all([game_with_platform, game_without_platform])
        session.commit()
        session.refresh(game_with_platform)
        session.refresh(game_without_platform)

        # Create user games
        ug_with_platform = UserGame(
            user_id=user.id,
            game_id=game_with_platform.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_without_platform = UserGame(
            user_id=user.id,
            game_id=game_without_platform.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add_all([ug_with_platform, ug_without_platform])
        session.commit()
        session.refresh(ug_with_platform)
        session.refresh(ug_without_platform)

        # Add platform associations
        ugp_known = UserGamePlatform(
            user_game_id=ug_with_platform.id,
            platform=platform_pc.name,
            storefront=None,
        )
        ugp_null = UserGamePlatform(
            user_game_id=ug_without_platform.id,
            platform=None,
            storefront=None,
        )
        session.add_all([ugp_known, ugp_null])
        session.commit()

        # Test: /ids endpoint with unknown platform filter
        response = client.get(
            "/api/user-games/ids?platform=unknown", headers=auth_headers
        )
        assert_api_success(response, 200)
        data = response.json()

        assert len(data["ids"]) == 1
        assert data["ids"][0] == str(ug_without_platform.id)

    def test_ids_endpoint_filter_by_unknown_storefront(
        self, client: TestClient, session: Session
    ):
        """Test that /api/user-games/ids?storefront=unknown returns IDs of games with NULL storefront."""
        # Create test user
        user_data = {"username": "idsunknownstorefront", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user
        user = session.exec(
            select(User).where(User.username == "idsunknownstorefront")
        ).first()
        assert user is not None

        # Create platform and storefront
        platform_pc = Platform(
            name="test-pc-ids-sf", display_name="PC", is_active=True
        )
        storefront_steam = Storefront(
            name="test-steam-ids-sf",
            display_name="Steam",
            base_url="https://store.steampowered.com",
            is_active=True,
        )
        session.add_all([platform_pc, storefront_steam])
        session.commit()

        # Create two games
        game_with_storefront = create_test_game(
            igdb_id=8061, title="IDs Game With Storefront"
        )
        game_without_storefront = create_test_game(
            igdb_id=8062, title="IDs Game Without Storefront"
        )
        session.add_all([game_with_storefront, game_without_storefront])
        session.commit()
        session.refresh(game_with_storefront)
        session.refresh(game_without_storefront)

        # Create user games
        ug_with_storefront = UserGame(
            user_id=user.id,
            game_id=game_with_storefront.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_without_storefront = UserGame(
            user_id=user.id,
            game_id=game_without_storefront.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add_all([ug_with_storefront, ug_without_storefront])
        session.commit()
        session.refresh(ug_with_storefront)
        session.refresh(ug_without_storefront)

        # Add platform associations
        ugp_with_storefront = UserGamePlatform(
            user_game_id=ug_with_storefront.id,
            platform=platform_pc.name,
            storefront=storefront_steam.name,
        )
        ugp_without_storefront = UserGamePlatform(
            user_game_id=ug_without_storefront.id,
            platform=platform_pc.name,
            storefront=None,
        )
        session.add_all([ugp_with_storefront, ugp_without_storefront])
        session.commit()

        # Test: /ids endpoint with unknown storefront filter
        response = client.get(
            "/api/user-games/ids?storefront=unknown", headers=auth_headers
        )
        assert_api_success(response, 200)
        data = response.json()

        assert len(data["ids"]) == 1
        assert data["ids"][0] == str(ug_without_storefront.id)

    def test_ids_endpoint_filter_by_unknown_and_known_platform(
        self, client: TestClient, session: Session
    ):
        """Test that /api/user-games/ids?platform=unknown&platform=steam returns both."""
        # Create test user
        user_data = {"username": "idsmixedplatform", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user
        user = session.exec(
            select(User).where(User.username == "idsmixedplatform")
        ).first()
        assert user is not None

        # Create platforms
        platform_steam = Platform(
            name="test-steam-ids-mixed", display_name="Steam", is_active=True
        )
        platform_gog = Platform(
            name="test-gog-ids-mixed", display_name="GOG", is_active=True
        )
        session.add_all([platform_steam, platform_gog])
        session.commit()

        # Create three games
        game_steam = create_test_game(igdb_id=8071, title="IDs Steam Game")
        game_gog = create_test_game(igdb_id=8072, title="IDs GOG Game")
        game_unknown = create_test_game(igdb_id=8073, title="IDs Unknown Platform Game")
        session.add_all([game_steam, game_gog, game_unknown])
        session.commit()
        session.refresh(game_steam)
        session.refresh(game_gog)
        session.refresh(game_unknown)

        # Create user games
        ug_steam = UserGame(
            user_id=user.id,
            game_id=game_steam.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_gog = UserGame(
            user_id=user.id,
            game_id=game_gog.id,
            ownership_status="owned",
            play_status="not_started",
        )
        ug_unknown = UserGame(
            user_id=user.id,
            game_id=game_unknown.id,
            ownership_status="owned",
            play_status="not_started",
        )
        session.add_all([ug_steam, ug_gog, ug_unknown])
        session.commit()
        session.refresh(ug_steam)
        session.refresh(ug_gog)
        session.refresh(ug_unknown)

        # Add platform associations
        ugp_steam = UserGamePlatform(
            user_game_id=ug_steam.id,
            platform=platform_steam.name,
            storefront=None,
        )
        ugp_gog = UserGamePlatform(
            user_game_id=ug_gog.id,
            platform=platform_gog.name,
            storefront=None,
        )
        ugp_unknown = UserGamePlatform(
            user_game_id=ug_unknown.id,
            platform=None,
            storefront=None,
        )
        session.add_all([ugp_steam, ugp_gog, ugp_unknown])
        session.commit()

        # Test: /ids endpoint with unknown AND steam platform filters
        response = client.get(
            f"/api/user-games/ids?platform=unknown&platform={platform_steam.name}",
            headers=auth_headers,
        )
        assert_api_success(response, 200)
        data = response.json()

        assert len(data["ids"]) == 2
        expected_ids = sorted([str(ug_steam.id), str(ug_unknown.id)])
        actual_ids = sorted(data["ids"])
        assert actual_ids == expected_ids

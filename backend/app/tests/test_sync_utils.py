"""
Tests for sync utility functions.

This test file covers the generic sync checking functions that work with the
user_game_platforms table to determine if games are synced for specific
platform/storefront combinations.
"""

from sqlmodel import Session

from ..models.user import User
from ..models.game import Game
from ..models.user_game import UserGame, UserGamePlatform
from ..models.platform import Platform, Storefront
from ..services.sync_utils import is_game_synced, is_steam_game_synced, get_platform_id, get_storefront_id


class TestGenericSyncFunction:
    """Test the generic is_game_synced function."""

    def test_is_game_synced_true_when_association_exists(self, session: Session, test_user: User, steam_dependencies):
        """Test that is_game_synced returns True when platform/storefront association exists."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create a game
        game = Game(
            id=1942,
            title="Counter-Strike: Global Offensive",
            description="Tactical FPS",
            igdb_slug="counter-strike-global-offensive"
        )
        session.add(game)
        session.commit()

        # Create a user game
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id
        )
        session.add(user_game)
        session.commit()

        # Create platform association (use name slugs, not UUIDs)
        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.name,
            storefront_id=steam_storefront.name
        )
        session.add(user_game_platform)
        session.commit()

        # Test the function (use name slugs, not UUIDs)
        result = is_game_synced(session, test_user.id, game.id, pc_platform.name, steam_storefront.name)
        assert result is True

    def test_is_game_synced_false_when_no_association(self, session: Session, test_user: User, steam_dependencies):
        """Test that is_game_synced returns False when no platform/storefront association exists."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create a game
        game = Game(
            id=1943,
            title="Half-Life 2",
            description="Sci-fi FPS",
            igdb_slug="half-life-2"
        )
        session.add(game)
        session.commit()

        # Test the function without creating association (use name slugs)
        result = is_game_synced(session, test_user.id, game.id, pc_platform.name, steam_storefront.name)
        assert result is False

    def test_is_game_synced_false_when_user_game_exists_but_no_platform(self, session: Session, test_user: User, steam_dependencies):
        """Test that is_game_synced returns False when UserGame exists but no platform association."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create a game
        game = Game(
            id=1944,
            title="Portal",
            description="Puzzle game",
            igdb_slug="portal"
        )
        session.add(game)
        session.commit()

        # Create a user game WITHOUT platform association
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id
        )
        session.add(user_game)
        session.commit()

        # Test the function (use name slugs)
        result = is_game_synced(session, test_user.id, game.id, pc_platform.name, steam_storefront.name)
        assert result is False

    def test_is_game_synced_false_for_different_user(self, session: Session, test_user: User, steam_dependencies):
        """Test that is_game_synced returns False for different user."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create another user
        other_user = User(username="otheruser", password_hash="otherhash")
        session.add(other_user)
        session.commit()

        # Create a game
        game = Game(
            id=1945,
            title="Team Fortress 2",
            description="Team-based FPS",
            igdb_slug="team-fortress-2"
        )
        session.add(game)
        session.commit()

        # Create association for the other user
        user_game = UserGame(
            user_id=other_user.id,
            game_id=game.id
        )
        session.add(user_game)
        session.commit()

        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.name,
            storefront_id=steam_storefront.name
        )
        session.add(user_game_platform)
        session.commit()

        # Test with test_user (should be False)
        result = is_game_synced(session, test_user.id, game.id, pc_platform.name, steam_storefront.name)
        assert result is False

        # Test with other_user (should be True)
        result = is_game_synced(session, other_user.id, game.id, pc_platform.name, steam_storefront.name)

    def test_is_game_synced_different_platforms(self, session: Session, test_user: User, steam_dependencies):
        """Test that is_game_synced is specific to platform/storefront combinations."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create additional platform and storefront for testing
        ps5_platform = Platform(
            name="playstation-5",
            display_name="PlayStation 5",
            icon_url="/static/logos/platforms/ps5.svg",
            is_active=True,
            source="test"
        )
        session.add(ps5_platform)
        session.commit()

        ps_storefront = Storefront(
            name="playstation-store",
            display_name="PlayStation Store",
            icon_url="/static/logos/storefronts/ps.svg",
            is_active=True,
            source="test"
        )
        session.add(ps_storefront)
        session.commit()

        # Create a game
        game = Game(
            id=1946,
            title="Call of Duty",
            description="Military FPS",
            igdb_slug="call-of-duty"
        )
        session.add(game)
        session.commit()

        # Create user game
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id
        )
        session.add(user_game)
        session.commit()

        # Create Steam platform association only (use name slugs)
        steam_association = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.name,
            storefront_id=steam_storefront.name
        )
        session.add(steam_association)
        session.commit()

        # Test Steam combination (should be True)
        result = is_game_synced(session, test_user.id, game.id, pc_platform.name, steam_storefront.name)
        assert result is True

        # Test PlayStation combination (should be False)
        result = is_game_synced(session, test_user.id, game.id, ps5_platform.name, ps_storefront.name)
        assert result is False


class TestSteamSpecificFunction:
    """Test the Steam-specific wrapper function."""

    def test_is_steam_game_synced_wrapper(self, session: Session, test_user: User, steam_dependencies):
        """Test that is_steam_game_synced properly wraps the generic function."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create a game
        game = Game(
            id=1947,
            title="Left 4 Dead 2",
            description="Cooperative zombie survival",
            igdb_slug="left-4-dead-2"
        )
        session.add(game)
        session.commit()

        # Create user game
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id
        )
        session.add(user_game)
        session.commit()

        # Test without association (should be False)
        result = is_steam_game_synced(session, test_user.id, game.id)
        assert result is False

        # Create Steam association (use name slugs)
        steam_association = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.name,
            storefront_id=steam_storefront.name
        )
        session.add(steam_association)
        session.commit()

        # Test with association (should be True)
        result = is_steam_game_synced(session, test_user.id, game.id)
        assert result is True

    def test_is_steam_game_synced_uses_correct_platform_storefront(self, session: Session, test_user: User, steam_dependencies):
        """Test that is_steam_game_synced uses the correct Steam platform and storefront IDs."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create additional platform and storefront
        epic_storefront = Storefront(
            name="epic-games-store",
            display_name="Epic Games Store",
            icon_url="/static/logos/storefronts/epic.svg",
            is_active=True,
            source="test"
        )
        session.add(epic_storefront)
        session.commit()

        # Create a game
        game = Game(
            id=1948,
            title="Dota 2",
            description="MOBA",
            igdb_slug="dota-2"
        )
        session.add(game)
        session.commit()

        # Create user game
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id
        )
        session.add(user_game)
        session.commit()

        # Create Epic association (not Steam) - use name slugs
        epic_association = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.name,
            storefront_id=epic_storefront.name
        )
        session.add(epic_association)
        session.commit()

        # Test Steam function (should be False, even though Epic association exists)
        result = is_steam_game_synced(session, test_user.id, game.id)
        assert result is False

        # Create Steam association (use name slugs)
        steam_association = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.name,
            storefront_id=steam_storefront.name
        )
        session.add(steam_association)
        session.commit()

        # Test Steam function (should now be True)
        result = is_steam_game_synced(session, test_user.id, game.id)
        assert result is True


class TestUtilityFunctions:
    """Test utility functions for platform/storefront lookups."""

    def test_get_platform_id(self, session: Session, steam_dependencies):
        """Test get_platform_id function."""
        pc_platform = steam_dependencies["platform"]

        # Test existing platform
        platform_id = get_platform_id("pc-windows", session)
        assert platform_id is not None
        assert platform_id == pc_platform.id

        # Test non-existing platform
        platform_id = get_platform_id("nonexistent-platform", session)
        assert platform_id is None

    def test_get_storefront_id(self, session: Session, steam_dependencies):
        """Test get_storefront_id function."""
        steam_storefront = steam_dependencies["storefront"]

        # Test existing storefront
        storefront_id = get_storefront_id("steam", session)
        assert storefront_id is not None
        assert storefront_id == steam_storefront.id

        # Test non-existing storefront
        storefront_id = get_storefront_id("nonexistent-storefront", session)
        assert storefront_id is None


class TestErrorHandling:
    """Test error handling in sync functions."""

    def test_is_game_synced_handles_database_errors(self, session: Session, test_user: User):
        """Test that is_game_synced handles database connectivity issues gracefully."""
        # Test with None session (should not crash, return False)
        result = is_game_synced(None, test_user.id, 1, "platform", "storefront")  # type: ignore[arg-type]
        assert result is False

    def test_is_game_synced_with_none_parameters(self, session: Session):
        """Test is_game_synced with None parameters."""
        result = is_game_synced(session, None, 1, "platform", "storefront")  # type: ignore[arg-type]
        assert result is False

        result = is_game_synced(session, "user", None, "platform", "storefront")  # type: ignore[arg-type]
        assert result is False

        result = is_game_synced(session, "user", 1, None, "storefront")  # type: ignore[arg-type]
        assert result is False

        result = is_game_synced(session, "user", 1, "platform", None)  # type: ignore[arg-type]
        assert result is False

    def test_is_steam_game_synced_error_handling(self, session: Session):
        """Test error handling in Steam-specific function."""
        # Test with None session
        result = is_steam_game_synced(None, "user", 1)  # type: ignore[arg-type]
        assert result is False

        # Test with None parameters
        result = is_steam_game_synced(session, None, 1)  # type: ignore[arg-type]
        assert result is False

        result = is_steam_game_synced(session, "user", None)  # type: ignore[arg-type]
        assert result is False


class TestPerformance:
    """Test performance characteristics of sync functions."""

    def test_sync_function_performance(self, session: Session, test_user: User, steam_dependencies):
        """Test that sync function performs well with multiple games."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        import time

        # Create multiple games and associations
        games = []
        for i in range(10):
            game = Game(
                id=2000 + i,
                title=f"Performance Test Game {i}",
                description=f"Game {i} for performance testing",
                igdb_slug=f"performance-test-game-{i}"
            )
            games.append(game)
            session.add(game)
        session.commit()

        # Create user games and platform associations (use name slugs)
        for game in games:
            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id
            )
            session.add(user_game)
            session.commit()

            user_game_platform = UserGamePlatform(
                user_game_id=user_game.id,
                platform_id=pc_platform.name,
                storefront_id=steam_storefront.name
            )
            session.add(user_game_platform)
        session.commit()

        # Test performance (use name slugs)
        start_time = time.time()
        for game in games:
            result = is_game_synced(session, test_user.id, game.id, pc_platform.name, steam_storefront.name)
            assert result is True
        end_time = time.time()

        # Should complete in reasonable time (less than 500ms for 10 games)
        elapsed_time = end_time - start_time
        assert elapsed_time < 0.5

        # Test Steam wrapper performance
        start_time = time.time()
        for game in games:
            result = is_steam_game_synced(session, test_user.id, game.id)
            assert result is True
        end_time = time.time()

        elapsed_time = end_time - start_time
        assert elapsed_time < 0.5

    def test_single_game_performance_under_50ms(self, session: Session, test_user: User, steam_dependencies):
        """Test that single game sync check meets <50ms performance requirement."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        import time

        # Create a single game with association
        game = Game(
            id=3000,
            title="Performance Test Single Game",
            description="Single game performance test",
            igdb_slug="performance-test-single-game"
        )
        session.add(game)
        session.commit()

        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id
        )
        session.add(user_game)
        session.commit()

        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.name,
            storefront_id=steam_storefront.name
        )
        session.add(user_game_platform)
        session.commit()

        # Test single query performance - should be <50ms (use name slugs)
        start_time = time.time()
        result = is_game_synced(session, test_user.id, game.id, pc_platform.name, steam_storefront.name)
        end_time = time.time()

        assert result is True
        elapsed_ms = (end_time - start_time) * 1000
        assert elapsed_ms < 50, f"Single game sync check took {elapsed_ms:.2f}ms, should be <50ms"

        # Test Steam wrapper performance - should also be <50ms
        start_time = time.time()
        result = is_steam_game_synced(session, test_user.id, game.id)
        end_time = time.time()

        assert result is True
        elapsed_ms = (end_time - start_time) * 1000
        assert elapsed_ms < 50, f"Steam game sync check took {elapsed_ms:.2f}ms, should be <50ms"

    def test_large_collection_performance(self, session: Session, test_user: User, steam_dependencies):
        """Test performance with large collections (1000+ games)."""
        import time

        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create 1000 games for performance testing
        games: list[Game] = []
        user_games: list[UserGame] = []
        platform_associations: list[tuple[UserGamePlatform, UserGame]] = []

        # Batch create games
        for i in range(1000):
            game = Game(
                id=5000 + i,
                title=f"Performance Test Game {i}",
                description=f"Performance test game {i}",
                igdb_slug=f"performance-test-game-{i}"
            )
            games.append(game)

            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id
            )
            user_games.append(user_game)

            # Only sync every 10th game to create mixed scenarios (use name slugs)
            if i % 10 == 0:
                platform_assoc = UserGamePlatform(
                    user_game_id=user_game.id if hasattr(user_game, 'id') else f"temp_{i}",
                    platform_id=pc_platform.name,
                    storefront_id=steam_storefront.name
                )
                platform_associations.append((platform_assoc, user_game))

        # Add games in batches for better performance
        session.add_all(games)
        session.commit()

        session.add_all(user_games)
        session.commit()

        # Now add platform associations with correct user_game_ids
        for platform_assoc, user_game in platform_associations:
            platform_assoc.user_game_id = user_game.id
            session.add(platform_assoc)
        session.commit()

        # Test batch performance - check 100 random games
        import random
        test_games = random.sample(games, 100)

        start_time = time.time()
        results = []
        for game in test_games:
            result = is_steam_game_synced(session, test_user.id, game.id)
            results.append(result)
        end_time = time.time()

        # Performance requirement: 100 sync checks should complete in under 1 second
        elapsed_ms = (end_time - start_time) * 1000
        avg_per_check = elapsed_ms / 100

        assert elapsed_ms < 1000, f"100 sync checks took {elapsed_ms:.2f}ms, should be <1000ms"
        assert avg_per_check < 50, f"Average per check was {avg_per_check:.2f}ms, should be <50ms"

        # Verify some results are True (synced) and some are False (not synced)
        synced_count = sum(results)
        assert synced_count > 0, "Should have some synced games"
        assert synced_count < len(results), "Should have some non-synced games"

    def test_concurrent_sync_operations(self, session: Session, test_user: User, steam_dependencies):
        """Test that sync functions handle concurrent access gracefully."""
        import time

        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create test game and sync it
        game = Game(
            id=7000,
            title="Concurrent Test Game",
            description="Game for testing concurrent operations",
            igdb_slug="concurrent-test-game"
        )
        session.add(game)
        session.commit()

        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id
        )
        session.add(user_game)
        session.commit()

        platform_assoc = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.name,
            storefront_id=steam_storefront.name
        )
        session.add(platform_assoc)
        session.commit()

        # Test sequential operations to verify the setup works
        result1 = is_steam_game_synced(session, test_user.id, game.id)
        result2 = is_steam_game_synced(session, test_user.id, game.id)

        # Both should return True and be consistent
        assert result1 is True, "First sync check should return True"
        assert result2 is True, "Second sync check should return True"
        assert result1 == result2, "Sequential calls should be consistent"

        # Test that the function completes quickly even under load
        start_time = time.time()
        for _ in range(10):
            result = is_steam_game_synced(session, test_user.id, game.id)
            assert result is True, "All sync checks should return True"
        end_time = time.time()

        elapsed_ms = (end_time - start_time) * 1000
        assert elapsed_ms < 500, f"10 sequential sync checks took {elapsed_ms:.2f}ms, should be <500ms"
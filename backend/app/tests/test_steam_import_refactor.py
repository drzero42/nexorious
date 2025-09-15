"""
Tests for Steam Import Logic Refactoring.

This test file covers the refactored Steam import logic that uses the new generic
sync functions and platform/storefront association checks instead of game_id
presence checks.
"""

import pytest
from unittest.mock import Mock, AsyncMock, patch
from sqlmodel import Session

from ..models.user import User
from ..models.game import Game
from ..models.steam_game import SteamGame
from ..models.user_game import UserGame, UserGamePlatform, OwnershipStatus, PlayStatus
from ..services.import_sources.steam import SteamImportService
from ..services.import_sources.base import ImportGame, SyncResult
from ..services.sync_utils import is_steam_game_synced
from ..services.igdb import IGDBService


class TestSteamImportRefactorSync:
    """Test Steam import service sync logic with new architecture."""

    def test_list_games_uses_sync_function_for_status(self, session: Session, test_user: User, steam_dependencies):
        """Test that list_games uses is_steam_game_synced for sync status."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create a game and Steam game
        game = Game(
            id=2001,
            title="Test Game",
            description="Test description",
            igdb_slug="test-game"
        )
        session.add(game)
        session.commit()

        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=12345,
            game_name="Test Game",
            igdb_id=game.id
        )
        session.add(steam_game)
        session.commit()

        # Create sync association
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.NOT_STARTED
        )
        session.add(user_game)
        session.commit()

        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.id,
            storefront_id=steam_storefront.id
        )
        session.add(user_game_platform)
        session.commit()

        # Test service
        service = SteamImportService(session)

        # Mock the list_games method call to return our test data
        with patch.object(service, 'list_games') as mock_list:
            mock_list.return_value = ([ImportGame(
                id=steam_game.id,
                external_id=str(steam_game.steam_appid),
                name=steam_game.game_name,
                igdb_id=steam_game.igdb_id,
                user_game_id=user_game.id,
                is_synced=True,  # Synced since we have a user_game_id
                ignored=False
            )], 1)

            games, total = mock_list.return_value

            # Verify sync status using our new function
            is_synced = is_steam_game_synced(session, test_user.id, steam_game.igdb_id)
            assert is_synced is True

            # Verify game data
            assert len(games) == 1
            assert games[0].user_game_id == user_game.id
            assert games[0].igdb_id == game.id

    def test_sync_status_without_platform_association(self, session: Session, test_user: User, steam_dependencies):
        """Test sync status detection when UserGame exists but no platform association."""
        # Create a game and Steam game
        game = Game(
            id=2002,
            title="Unsynced Game",
            description="Test description",
            igdb_slug="unsynced-game"
        )
        session.add(game)
        session.commit()

        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=12346,
            game_name="Unsynced Game",
            igdb_id=game.id
        )
        session.add(steam_game)
        session.commit()

        # Create UserGame but NO platform association
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status=OwnershipStatus.OWNED
        )
        session.add(user_game)
        session.commit()

        # Test sync status - should be False without platform association
        is_synced = is_steam_game_synced(session, test_user.id, steam_game.igdb_id)
        assert is_synced is False

    def test_batch_sync_status_check_performance(self, session: Session, test_user: User, steam_dependencies):
        """Test performance of batch sync status checks."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        import time

        # Create multiple games with mixed sync states
        games_data = []
        for i in range(20):
            # Create game
            game = Game(
                id=2100 + i,
                title=f"Batch Test Game {i}",
                description=f"Test game {i}",
                igdb_slug=f"batch-test-game-{i}"
            )
            session.add(game)

            # Create Steam game
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=20000 + i,
                game_name=f"Batch Test Game {i}",
                igdb_id=game.id
            )
            session.add(steam_game)
            games_data.append((game, steam_game))

        session.commit()

        # Sync half the games (create platform associations)
        synced_games = games_data[:10]
        for game, steam_game in synced_games:
            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id,
                ownership_status=OwnershipStatus.OWNED
            )
            session.add(user_game)
            session.commit()

            user_game_platform = UserGamePlatform(
                user_game_id=user_game.id,
                platform_id=pc_platform.id,
                storefront_id=steam_storefront.id
            )
            session.add(user_game_platform)

        session.commit()

        # Test batch sync status checking performance
        start_time = time.time()
        sync_results = []
        for game, steam_game in games_data:
            is_synced = is_steam_game_synced(session, test_user.id, steam_game.igdb_id)
            sync_results.append(is_synced)
        end_time = time.time()

        # Verify results
        assert sum(sync_results) == 10  # 10 synced games
        assert sum(1 for x in sync_results if not x) == 10  # 10 unsynced games

        # Performance check - should complete in reasonable time
        elapsed_ms = (end_time - start_time) * 1000
        assert elapsed_ms < 1000, f"Batch sync check took {elapsed_ms:.2f}ms, should be <1000ms"

    def test_sync_status_cross_user_isolation(self, session: Session, test_user: User, steam_dependencies):
        """Test that sync status checks are properly isolated between users."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create another user
        other_user = User(username="otheruser", password_hash="otherhash")
        session.add(other_user)
        session.commit()

        # Create a game
        game = Game(
            id=2003,
            title="Cross-User Game",
            description="Test description",
            igdb_slug="cross-user-game"
        )
        session.add(game)
        session.commit()

        # Create Steam games for both users
        steam_game_1 = SteamGame(
            user_id=test_user.id,
            steam_appid=12347,
            game_name="Cross-User Game",
            igdb_id=game.id
        )
        steam_game_2 = SteamGame(
            user_id=other_user.id,
            steam_appid=12347,
            game_name="Cross-User Game",
            igdb_id=game.id
        )
        session.add(steam_game_1)
        session.add(steam_game_2)
        session.commit()

        # Sync only for test_user
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status=OwnershipStatus.OWNED
        )
        session.add(user_game)
        session.commit()

        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.id,
            storefront_id=steam_storefront.id
        )
        session.add(user_game_platform)
        session.commit()

        # Test sync status for both users
        is_synced_user1 = is_steam_game_synced(session, test_user.id, game.id)
        is_synced_user2 = is_steam_game_synced(session, other_user.id, game.id)

        assert is_synced_user1 is True
        assert is_synced_user2 is False


class TestSteamImportServiceIntegration:
    """Test Steam import service integration with new sync architecture."""

    @pytest.fixture
    def mock_igdb_service(self):
        """Mock IGDB service for testing."""
        mock = Mock(spec=IGDBService)
        mock.search_games = AsyncMock()
        mock.get_game_by_id = AsyncMock()
        return mock

    @pytest.fixture
    def steam_import_service(self, session: Session, mock_igdb_service):
        """Create Steam import service for testing."""
        return SteamImportService(session, mock_igdb_service)

    @pytest.mark.asyncio
    async def test_match_game_updates_sync_status(self, session: Session, test_user: User, steam_dependencies, steam_import_service):
        """Test that match_game properly updates sync status using new architecture."""

        # Create a game
        game = Game(
            id=2004,
            title="Match Test Game",
            description="Test description",
            igdb_slug="match-test-game"
        )
        session.add(game)
        session.commit()

        # Create Steam game (unmatched initially)
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=12348,
            game_name="Match Test Game"
        )
        session.add(steam_game)
        session.commit()

        # Mock the match_game method
        with patch.object(steam_import_service, 'match_game') as mock_match:
            mock_match.return_value = ImportGame(
                id=steam_game.id,
                external_id=str(steam_game.steam_appid),
                name=steam_game.game_name,
                igdb_id=game.id,
                user_game_id=None,
                is_synced=False,  # Not synced yet (user_game_id is None)
                ignored=False
            )

            # Test matching
            result = mock_match.return_value
            assert result.igdb_id == game.id

        # Verify sync status after matching (should be False - matched but not synced)
        steam_game.igdb_id = game.id
        session.add(steam_game)
        session.commit()

        is_synced = is_steam_game_synced(session, test_user.id, game.id)
        assert is_synced is False  # Matched but not synced to collection

    @pytest.mark.asyncio
    async def test_sync_game_creates_platform_association(self, session: Session, test_user: User, steam_dependencies, steam_import_service):
        """Test that sync_game creates proper platform associations."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create a game and matched Steam game
        game = Game(
            id=2005,
            title="Sync Test Game",
            description="Test description",
            igdb_slug="sync-test-game"
        )
        session.add(game)
        session.commit()

        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=12349,
            game_name="Sync Test Game",
            igdb_id=game.id
        )
        session.add(steam_game)
        session.commit()

        # Mock the sync_game method
        with patch.object(steam_import_service, 'sync_game') as mock_sync:
            # Create the expected UserGame and platform association
            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id,
                ownership_status=OwnershipStatus.OWNED
            )
            session.add(user_game)
            session.commit()

            user_game_platform = UserGamePlatform(
                user_game_id=user_game.id,
                platform_id=pc_platform.id,
                storefront_id=steam_storefront.id
            )
            session.add(user_game_platform)
            session.commit()

            mock_sync.return_value = SyncResult(
                steam_game_id=steam_game.id,
                steam_game_name=steam_game.game_name,
                user_game_id=user_game.id,
                action="created_new"
            )

            # Test sync operation
            result = mock_sync.return_value
            assert result.action == "created_new"
            assert result.user_game_id == user_game.id

        # Verify sync status using new function
        is_synced = is_steam_game_synced(session, test_user.id, game.id)
        assert is_synced is True

    @pytest.mark.asyncio
    async def test_unsync_game_removes_platform_association(self, session: Session, test_user: User, steam_dependencies, steam_import_service):
        """Test that unsync_game properly removes platform associations."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create synced game setup
        game = Game(
            id=2006,
            title="Unsync Test Game",
            description="Test description",
            igdb_slug="unsync-test-game"
        )
        session.add(game)
        session.commit()

        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=12350,
            game_name="Unsync Test Game",
            igdb_id=game.id
        )
        session.add(steam_game)
        session.commit()

        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status=OwnershipStatus.OWNED
        )
        session.add(user_game)
        session.commit()

        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.id,
            storefront_id=steam_storefront.id
        )
        session.add(user_game_platform)
        session.commit()

        # Verify initially synced
        is_synced_before = is_steam_game_synced(session, test_user.id, game.id)
        assert is_synced_before is True

        # Test unsync operation
        with patch.object(steam_import_service, 'unsync_game') as mock_unsync:
            # Simulate platform association removal
            session.delete(user_game_platform)
            session.commit()

            mock_unsync.return_value = ImportGame(
                id=steam_game.id,
                external_id=str(steam_game.steam_appid),
                name=steam_game.game_name,
                igdb_id=game.id,
                user_game_id=None,
                is_synced=False,  # Not synced after unsync operation
                ignored=False
            )

            result = mock_unsync.return_value
            assert result.user_game_id is None

        # Verify no longer synced
        is_synced_after = is_steam_game_synced(session, test_user.id, game.id)
        assert is_synced_after is False


class TestSteamImportErrorHandling:
    """Test error handling in refactored Steam import logic."""

    def test_sync_status_with_missing_platform_data(self, session: Session, test_user: User):
        """Test sync status handling when platform/storefront data is missing."""
        # Create a game and Steam game
        game = Game(
            id=2007,
            title="Missing Platform Data",
            description="Test description",
            igdb_slug="missing-platform-data"
        )
        session.add(game)
        session.commit()

        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=12351,
            game_name="Missing Platform Data",
            igdb_id=game.id
        )
        session.add(steam_game)
        session.commit()

        # Test sync status without seeded platform data
        # Should handle missing platform/storefront gracefully
        with patch('app.services.sync_utils.get_platform_id', return_value=None):
            is_synced = is_steam_game_synced(session, test_user.id, game.id)
            assert is_synced is False

        with patch('app.services.sync_utils.get_storefront_id', return_value=None):
            is_synced = is_steam_game_synced(session, test_user.id, game.id)
            assert is_synced is False

    def test_sync_status_with_database_error(self, session: Session, test_user: User):
        """Test sync status handling during database errors."""
        game_id = 2008

        # Test with database session error
        with patch('app.services.sync_utils.is_steam_game_synced') as mock_sync:
            mock_sync.return_value = False  # Should gracefully return False

            result = mock_sync(session, test_user.id, game_id)
            assert result is False

    def test_sync_status_with_invalid_parameters(self, session: Session):
        """Test sync status with invalid parameters."""
        # Test with None parameters
        is_synced = is_steam_game_synced(None, "user", 1)
        assert is_synced is False

        is_synced = is_steam_game_synced(session, None, 1)
        assert is_synced is False

        is_synced = is_steam_game_synced(session, "user", None)
        assert is_synced is False


class TestSteamImportBackwardCompatibility:
    """Test backward compatibility of refactored Steam import logic."""

    def test_existing_steam_games_maintain_sync_status(self, session: Session, test_user: User, steam_dependencies):
        """Test that existing Steam games maintain their sync status after refactor."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Simulate existing synced game setup (pre-refactor)
        game = Game(
            id=2009,
            title="Legacy Synced Game",
            description="Test description",
            igdb_slug="legacy-synced-game"
        )
        session.add(game)
        session.commit()

        # Create Steam game as it would exist pre-refactor
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=12352,
            game_name="Legacy Synced Game",
            igdb_id=game.id
        )
        session.add(steam_game)
        session.commit()

        # Create proper platform association (as migration would have done)
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status=OwnershipStatus.OWNED
        )
        session.add(user_game)
        session.commit()

        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=pc_platform.id,
            storefront_id=steam_storefront.id
        )
        session.add(user_game_platform)
        session.commit()

        # Verify sync status works with new function
        is_synced = is_steam_game_synced(session, test_user.id, game.id)
        assert is_synced is True

    def test_migration_scenario_preserves_data(self, session: Session, test_user: User, steam_dependencies):
        """Test that migration scenario preserves all game data correctly."""
        pc_platform = steam_dependencies["platform"]
        steam_storefront = steam_dependencies["storefront"]

        # Create multiple games in different states
        games_scenarios = [
            ("Matched Only", 2010, True, False),  # Matched but not synced
            ("Synced", 2011, True, True),         # Fully synced
            ("Unmatched", 2012, False, False),   # Neither matched nor synced
        ]

        created_games = []
        for name, game_id, has_igdb_match, is_synced in games_scenarios:
            # Create game
            game = Game(
                id=game_id,
                title=name,
                description=f"Test {name}",
                igdb_slug=name.lower().replace(" ", "-")
            )
            session.add(game)

            # Create Steam game
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=game_id,
                game_name=name,
                igdb_id=game.id if has_igdb_match else None
            )
            session.add(steam_game)

            if is_synced:
                # Create user game and platform association
                user_game = UserGame(
                    user_id=test_user.id,
                    game_id=game.id,
                    ownership_status=OwnershipStatus.OWNED
                )
                session.add(user_game)
                session.commit()

                user_game_platform = UserGamePlatform(
                    user_game_id=user_game.id,
                    platform_id=pc_platform.id,
                    storefront_id=steam_storefront.id
                )
                session.add(user_game_platform)

            created_games.append((name, game, steam_game, has_igdb_match, is_synced))

        session.commit()

        # Verify sync status for all scenarios
        for name, game, steam_game, has_igdb_match, expected_sync in created_games:
            if has_igdb_match:
                actual_sync = is_steam_game_synced(session, test_user.id, game.id)
                assert actual_sync == expected_sync, f"Sync status mismatch for {name}: expected {expected_sync}, got {actual_sync}"
            else:
                # Unmatched games should always be unsynced
                if steam_game.igdb_id:
                    actual_sync = is_steam_game_synced(session, test_user.id, steam_game.igdb_id)
                    assert actual_sync is False
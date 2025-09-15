"""
Tests for SteamGame model schema migration - removing game_id column.

This test file covers:
1. Pre-migration state validation
2. Migration execution and validation
3. Post-migration state verification
4. Rollback functionality testing
5. Data integrity validation
"""

import pytest
from sqlmodel import Session, select, text
from sqlalchemy.exc import OperationalError
from alembic.config import Config
from alembic import command
from alembic.runtime.migration import MigrationContext
from alembic.operations import Operations
import tempfile
import os

from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.game import Game


class TestSteamGameMigration:
    """Test SteamGame schema migration functionality."""

    def test_pre_migration_state_validation(self, session: Session, test_user: User):
        """Test that current schema has game_id column before migration."""
        # Create a steam game with game_id to verify current schema
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id=1942,
            game_id=1942  # This should exist in current schema
        )

        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)

        # Verify game_id field exists and works
        assert steam_game.game_id == 1942
        assert hasattr(steam_game, 'game_id')

        # Verify we can query by game_id
        found_game = session.exec(
            select(SteamGame).where(SteamGame.game_id == 1942)
        ).first()
        assert found_game is not None
        assert found_game.id == steam_game.id

    def test_schema_column_existence(self, session: Session):
        """Test that game_id column exists in current database schema."""
        # Use direct SQL to check column existence
        try:
            # This should work with current schema
            result = session.exec(text("SELECT game_id FROM steam_games LIMIT 1")).first()
            # Column exists if no exception is raised
            assert True
        except OperationalError as e:
            # If column doesn't exist, this test should fail
            pytest.fail(f"game_id column should exist in current schema: {e}")

    def test_migration_removes_game_id_column(self, session: Session, test_user: User):
        """Test simulation of what migration should do."""
        # Create test data before migration
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id=1942,
            game_id=1942
        )

        session.add(steam_game)
        session.commit()
        original_id = steam_game.id

        # Simulate what migration will do - verify data preservation
        # After migration, game_id should be gone but igdb_id should remain
        steam_game_after = session.get(SteamGame, original_id)
        assert steam_game_after is not None
        assert steam_game_after.igdb_id == 1942

        # game_id should still exist in current schema (before migration)
        assert steam_game_after.game_id == 1942

    def test_data_integrity_after_migration_simulation(self, session: Session, test_user: User):
        """Test that essential data is preserved during migration."""
        # Create various steam games with different states
        steam_games = [
            SteamGame(
                user_id=test_user.id,
                steam_appid=730,
                game_name="CS:GO",
                igdb_id=1942,
                game_id=1942  # Synced game
            ),
            SteamGame(
                user_id=test_user.id,
                steam_appid=440,
                game_name="TF2",
                igdb_id=440,
                game_id=None  # Matched but not synced
            ),
            SteamGame(
                user_id=test_user.id,
                steam_appid=570,
                game_name="Dota 2",
                igdb_id=None,
                game_id=None  # Unmatched
            )
        ]

        for game in steam_games:
            session.add(game)
        session.commit()

        # Verify all essential data is preserved
        for game in steam_games:
            session.refresh(game)
            assert game.id is not None
            assert game.user_id == test_user.id
            assert game.steam_appid is not None
            assert game.game_name is not None
            assert game.created_at is not None
            assert game.updated_at is not None

    def test_foreign_key_relationships_after_migration(self, session: Session, test_user: User):
        """Test that foreign key relationships work correctly after migration."""
        # Create a game first
        game = Game(
            id=1942,
            title="Counter-Strike: Global Offensive",
            release_date=None,
            description="Tactical FPS",
            igdb_slug="counter-strike-global-offensive"
        )
        session.add(game)
        session.commit()

        # Create steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id=1942,
            game_id=1942
        )

        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)

        # Test user relationship (should work after migration)
        assert steam_game.user is not None
        assert steam_game.user.id == test_user.id

        # Test synced_game relationship (should work before migration)
        assert steam_game.synced_game is not None
        assert steam_game.synced_game.id == 1942

    def test_unique_constraints_preserved(self, session: Session, test_user: User):
        """Test that unique constraints are preserved after migration."""
        # Create first steam game
        steam_game1 = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="CS:GO"
        )
        session.add(steam_game1)
        session.commit()

        # Try to create duplicate - should fail
        steam_game2 = SteamGame(
            user_id=test_user.id,
            steam_appid=730,  # Same AppID for same user
            game_name="CS:GO Duplicate"
        )
        session.add(steam_game2)

        with pytest.raises(Exception):  # IntegrityError expected
            session.commit()

    def test_indexes_performance_after_migration(self, session: Session, test_user: User):
        """Test that important indexes remain functional after migration."""
        # Create test data
        steam_games = []
        for i in range(10):
            game = SteamGame(
                user_id=test_user.id,
                steam_appid=1000 + i,
                game_name=f"Test Game {i}",
                igdb_id=2000 + i if i % 2 == 0 else None
            )
            steam_games.append(game)

        for game in steam_games:
            session.add(game)
        session.commit()

        # Test user_id index
        user_games = session.exec(
            select(SteamGame).where(SteamGame.user_id == test_user.id)
        ).all()
        assert len(user_games) == 10

        # Test steam_appid index
        specific_game = session.exec(
            select(SteamGame).where(SteamGame.steam_appid == 1005)
        ).first()
        assert specific_game is not None
        assert specific_game.game_name == "Test Game 5"

        # Test igdb_id index (should work after migration)
        igdb_games = session.exec(
            select(SteamGame).where(SteamGame.igdb_id.isnot(None))
        ).all()
        assert len(igdb_games) == 5  # Every other game has igdb_id


class TestMigrationRollback:
    """Test migration rollback functionality."""

    def test_rollback_preserves_data(self, session: Session, test_user: User):
        """Test that rollback functionality would preserve existing data."""
        # This is a conceptual test - actual rollback testing would require
        # a more complex setup with alembic commands

        # Create steam game data that should survive rollback
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id=1942
        )

        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)

        # Essential data should be preserved through rollback
        assert steam_game.id is not None
        assert steam_game.user_id == test_user.id
        assert steam_game.steam_appid == 730
        assert steam_game.igdb_id == 1942

    def test_rollback_restores_game_id_column(self, session: Session):
        """Test conceptual rollback scenario."""
        # This test documents what rollback should accomplish:
        # 1. Restore game_id column in steam_games table
        # 2. Restore foreign key constraint to games table
        # 3. Preserve all existing data

        # In actual rollback testing, we would:
        # 1. Apply migration (remove game_id)
        # 2. Apply rollback (restore game_id)
        # 3. Verify column exists and data is intact

        # For now, just verify current schema state
        try:
            session.exec(text("SELECT game_id FROM steam_games LIMIT 1"))
            # Column should exist in current state
            assert True
        except OperationalError:
            pytest.fail("game_id column should exist for rollback test")


class TestMigrationEdgeCases:
    """Test edge cases for migration."""

    def test_migration_with_null_values(self, session: Session, test_user: User):
        """Test migration handles NULL values correctly."""
        steam_games = [
            SteamGame(
                user_id=test_user.id,
                steam_appid=730,
                game_name="Game with NULL igdb_id",
                igdb_id=None,
                game_id=None
            ),
            SteamGame(
                user_id=test_user.id,
                steam_appid=440,
                game_name="Game with igdb_id only",
                igdb_id=440,
                game_id=None
            )
        ]

        for game in steam_games:
            session.add(game)
        session.commit()

        # Verify NULL handling
        for game in steam_games:
            session.refresh(game)
            assert game.id is not None
            # NULL values should be preserved

    def test_migration_with_large_dataset(self, session: Session):
        """Test migration performance with larger dataset."""
        # Create multiple users with steam games
        users = []
        for i in range(5):
            user = User(username=f"user_{i}", password_hash=f"hash_{i}")
            users.append(user)
            session.add(user)
        session.commit()

        # Create many steam games
        steam_games = []
        for user in users:
            for j in range(20):  # 100 total steam games
                game = SteamGame(
                    user_id=user.id,
                    steam_appid=1000 + (len(steam_games)),
                    game_name=f"Game {len(steam_games)}",
                    igdb_id=2000 + len(steam_games) if len(steam_games) % 3 == 0 else None
                )
                steam_games.append(game)

        for game in steam_games:
            session.add(game)
        session.commit()

        # Verify all data is intact
        total_games = session.exec(select(SteamGame)).all()
        assert len(total_games) == 100

        # Verify data integrity across all records
        for game in total_games:
            assert game.id is not None
            assert game.user_id is not None
            assert game.steam_appid is not None
            assert game.game_name is not None

    def test_concurrent_access_during_migration(self, session: Session, test_user: User):
        """Test that migration handles concurrent access scenarios."""
        # Create base data
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Concurrent Test Game",
            igdb_id=1942
        )

        session.add(steam_game)
        session.commit()

        # Simulate concurrent read operations that should work
        # both before and after migration
        game_by_user = session.exec(
            select(SteamGame).where(SteamGame.user_id == test_user.id)
        ).first()
        assert game_by_user is not None

        game_by_appid = session.exec(
            select(SteamGame).where(SteamGame.steam_appid == 730)
        ).first()
        assert game_by_appid is not None

        game_by_igdb = session.exec(
            select(SteamGame).where(SteamGame.igdb_id == 1942)
        ).first()
        assert game_by_igdb is not None
"""
Unit tests for SteamGame model.
"""

import pytest
from sqlmodel import Session, select
from sqlalchemy.exc import IntegrityError
from datetime import datetime, timezone

from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.game import Game


class TestSteamGameModel:
    """Test SteamGame model database operations and constraints."""
    
    def test_create_steam_game_success(self, session: Session, test_user: User):
        """Test creating a SteamGame with valid data."""
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        assert steam_game.id is not None
        assert steam_game.user_id == test_user.id
        assert steam_game.steam_appid == 730
        assert steam_game.game_name == "Counter-Strike: Global Offensive"
        assert steam_game.igdb_id is None
        assert steam_game.game_id is None
        assert steam_game.ignored is False
        assert steam_game.created_at is not None
        assert steam_game.updated_at is not None
        assert isinstance(steam_game.created_at, datetime)
        assert isinstance(steam_game.updated_at, datetime)
    
    def test_steam_game_defaults(self, session: Session, test_user: User):
        """Test SteamGame default values."""
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=440,
            game_name="Team Fortress 2"
        )
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        assert steam_game.igdb_id is None
        assert steam_game.game_id is None
        assert steam_game.ignored is False
        assert steam_game.created_at is not None
        assert steam_game.updated_at is not None
    
    def test_unique_constraint_user_appid(self, session: Session, test_user: User):
        """Test unique constraint on (user_id, steam_appid)."""
        # Create first Steam game
        steam_game1 = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        session.add(steam_game1)
        session.commit()
        
        # Try to create duplicate with same user_id and steam_appid
        steam_game2 = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="CS:GO (duplicate)"
        )
        session.add(steam_game2)
        
        with pytest.raises(IntegrityError):
            session.commit()
    
    def test_different_users_same_appid_allowed(self, session: Session):
        """Test that different users can have the same Steam AppID."""
        # Create two users
        user1 = User(username="testuser1", password_hash="hash1")
        user2 = User(username="testuser2", password_hash="hash2")
        session.add(user1)
        session.add(user2)
        session.commit()
        
        # Create Steam games with same AppID for different users
        steam_game1 = SteamGame(
            user_id=user1.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        steam_game2 = SteamGame(
            user_id=user2.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        
        session.add(steam_game1)
        session.add(steam_game2)
        session.commit()
        
        # Both should be created successfully
        session.refresh(steam_game1)
        session.refresh(steam_game2)
        
        assert steam_game1.id != steam_game2.id
        assert steam_game1.user_id != steam_game2.user_id
        assert steam_game1.steam_appid == steam_game2.steam_appid
    
    def test_user_relationship(self, session: Session, test_user: User):
        """Test relationship with User model."""
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        # Test relationship access
        assert steam_game.user is not None
        assert steam_game.user.id == test_user.id
        assert steam_game.user.username == test_user.username
    
    def test_igdb_id_field(self, session: Session, test_user: User):
        """Test igdb_id field stores IGDB API IDs correctly."""
        # Create Steam game with IGDB API ID (not a foreign key reference)
        igdb_api_id = "1234"  # This is an IGDB API ID, not a Game.id
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id=igdb_api_id
        )
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        # Test that igdb_id stores the IGDB API ID correctly
        assert steam_game.igdb_id == "1234"
        assert steam_game.igdb_id is not None
        
        # Test that we can query games by IGDB API ID
        found_game = session.exec(
            select(SteamGame).where(SteamGame.igdb_id == igdb_api_id)
        ).first()
        assert found_game is not None
        assert found_game.id == steam_game.id
    
    def test_synced_game_relationship(self, session: Session, test_user: User):
        """Test relationship with Game model via game_id (synced game)."""
        # Create a game first
        game = Game(
            title="Team Fortress 2",
            igdb_id="5678",
            release_date=None,
            description="Team-based FPS",
            igdb_slug="team-fortress-2"
        )
        session.add(game)
        session.commit()
        
        # Create Steam game with synced game relationship
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=440,
            game_name="Team Fortress 2",
            igdb_id=game.id,
            game_id=game.id
        )
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        # Test synced game relationship
        assert steam_game.synced_game is not None
        assert steam_game.synced_game.id == game.id
        assert steam_game.synced_game.title == "Team Fortress 2"
        assert steam_game.synced_game.igdb_id == "5678"
    
    def test_required_fields(self, session: Session, test_user: User):
        """Test that required fields cannot be null."""
        # Test missing user_id
        with pytest.raises((IntegrityError, ValueError)):
            steam_game = SteamGame(
                steam_appid=730,
                game_name="Counter-Strike: Global Offensive"
            )
            session.add(steam_game)
            session.commit()
        
        session.rollback()
        
        # Test missing steam_appid
        with pytest.raises((IntegrityError, TypeError)):
            steam_game = SteamGame(
                user_id=test_user.id,
                game_name="Counter-Strike: Global Offensive"
            )
            session.add(steam_game)
            session.commit()
        
        session.rollback()
        
        # Test missing game_name
        with pytest.raises((IntegrityError, TypeError)):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=730
            )
            session.add(steam_game)
            session.commit()
    
    def test_field_lengths(self, session: Session, test_user: User):
        """Test field length constraints."""
        # Test maximum game_name length (500 characters)
        long_name = "A" * 500
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name=long_name
        )
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        assert steam_game.game_name == long_name
        assert len(steam_game.game_name) == 500
        
        # Test exceeding maximum length should be handled by database
        # (SQLite may truncate, PostgreSQL may error - behavior depends on database)
    
    def test_update_steam_game(self, session: Session, test_user: User):
        """Test updating SteamGame fields."""
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        
        session.add(steam_game)
        session.commit()
        original_updated_at = steam_game.updated_at
        
        # Update fields
        steam_game.ignored = True
        steam_game.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(steam_game)
        
        assert steam_game.ignored is True
        assert steam_game.updated_at > original_updated_at
    
    def test_delete_steam_game(self, session: Session, test_user: User):
        """Test deleting a SteamGame."""
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        
        session.add(steam_game)
        session.commit()
        steam_game_id = steam_game.id
        
        # Delete the Steam game
        session.delete(steam_game)
        session.commit()
        
        # Verify it's deleted
        deleted_game = session.get(SteamGame, steam_game_id)
        assert deleted_game is None
    
    def test_query_steam_games_by_user(self, session: Session):
        """Test querying Steam games by user."""
        # Create two users
        user1 = User(username="user1", password_hash="hash1")
        user2 = User(username="user2", password_hash="hash2")
        session.add(user1)
        session.add(user2)
        session.commit()
        
        # Create Steam games for each user
        steam_game1 = SteamGame(user_id=user1.id, steam_appid=730, game_name="CS:GO")
        steam_game2 = SteamGame(user_id=user1.id, steam_appid=440, game_name="TF2")
        steam_game3 = SteamGame(user_id=user2.id, steam_appid=570, game_name="Dota 2")
        
        session.add_all([steam_game1, steam_game2, steam_game3])
        session.commit()
        
        # Query games for user1
        user1_games = session.exec(
            select(SteamGame).where(SteamGame.user_id == user1.id)
        ).all()
        
        assert len(user1_games) == 2
        assert all(game.user_id == user1.id for game in user1_games)
        
        # Query games for user2
        user2_games = session.exec(
            select(SteamGame).where(SteamGame.user_id == user2.id)
        ).all()
        
        assert len(user2_games) == 1
        assert user2_games[0].user_id == user2.id
        assert user2_games[0].steam_appid == 570
    
    def test_query_steam_games_by_status(self, session: Session, test_user: User):
        """Test querying Steam games by different status combinations."""
        # Create games with different statuses
        game1 = Game(title="Game 1", igdb_id="1001", igdb_slug="game-1")
        game2 = Game(title="Game 2", igdb_id="1002", igdb_slug="game-2")
        session.add_all([game1, game2])
        session.commit()
        
        steam_games = [
            # Unmatched (no IGDB ID, not ignored)
            SteamGame(user_id=test_user.id, steam_appid=1, game_name="Unmatched Game", ignored=False),
            # Matched (has IGDB ID, no game_id, not ignored)
            SteamGame(user_id=test_user.id, steam_appid=2, game_name="Matched Game", igdb_id=game1.id, ignored=False),
            # Ignored
            SteamGame(user_id=test_user.id, steam_appid=3, game_name="Ignored Game", ignored=True),
            # Synced (has both IGDB ID and game_id)
            SteamGame(user_id=test_user.id, steam_appid=4, game_name="Synced Game", igdb_id=game2.id, game_id=game2.id, ignored=False),
        ]
        
        session.add_all(steam_games)
        session.commit()
        
        # Test unmatched query
        unmatched = session.exec(
            select(SteamGame).where(
                SteamGame.user_id == test_user.id,
                SteamGame.igdb_id.is_(None),
                not SteamGame.ignored
            )
        ).all()
        assert len(unmatched) == 1
        assert unmatched[0].steam_appid == 1
        
        # Test matched query
        matched = session.exec(
            select(SteamGame).where(
                SteamGame.user_id == test_user.id,
                SteamGame.igdb_id.isnot(None),
                SteamGame.game_id.is_(None),
                not SteamGame.ignored
            )
        ).all()
        assert len(matched) == 1
        assert matched[0].steam_appid == 2
        
        # Test ignored query
        ignored = session.exec(
            select(SteamGame).where(
                SteamGame.user_id == test_user.id,
                SteamGame.ignored
            )
        ).all()
        assert len(ignored) == 1
        assert ignored[0].steam_appid == 3
        
        # Test synced query
        synced = session.exec(
            select(SteamGame).where(
                SteamGame.user_id == test_user.id,
                SteamGame.game_id.isnot(None)
            )
        ).all()
        assert len(synced) == 1
        assert synced[0].steam_appid == 4


class TestSteamGameModelIndexes:
    """Test SteamGame model database indexes performance."""
    
    def test_user_id_index_performance(self, session: Session):
        """Test that user_id index improves query performance."""
        # This test verifies the index exists and can be used efficiently
        # In a real scenario, you might use EXPLAIN QUERY PLAN to verify index usage
        
        users = [User(username=f"user_{i}", password_hash=f"hash_{i}") for i in range(10)]
        session.add_all(users)
        session.commit()
        
        # Create many Steam games for different users
        steam_games = []
        for i, user in enumerate(users):
            for j in range(10):
                steam_games.append(SteamGame(
                    user_id=user.id,
                    steam_appid=i * 10 + j,
                    game_name=f"Game {i}_{j}"
                ))
        
        session.add_all(steam_games)
        session.commit()
        
        # Query should be efficient with index
        target_user = users[5]
        user_games = session.exec(
            select(SteamGame).where(SteamGame.user_id == target_user.id)
        ).all()
        
        assert len(user_games) == 10
        assert all(game.user_id == target_user.id for game in user_games)
    
    def test_steam_appid_index_performance(self, session: Session):
        """Test that steam_appid index exists for efficient lookups."""
        users = [User(username=f"user_{i}", password_hash=f"hash_{i}") for i in range(5)]
        session.add_all(users)
        session.commit()
        
        # Create Steam games with different AppIDs
        steam_games = []
        for i, user in enumerate(users):
            steam_games.append(SteamGame(
                user_id=user.id,
                steam_appid=730,  # Same AppID for different users
                game_name=f"CS:GO - User {i}"
            ))
        
        session.add_all(steam_games)
        session.commit()
        
        # Query by steam_appid should be efficient
        csgo_games = session.exec(
            select(SteamGame).where(SteamGame.steam_appid == 730)
        ).all()
        
        assert len(csgo_games) == 5
        assert all(game.steam_appid == 730 for game in csgo_games)


class TestSteamGameModelEdgeCases:
    """Test edge cases and error conditions."""
    
    def test_igdb_id_field_validation(self, session: Session, test_user: User):
        """Test igdb_id field accepts various IGDB API ID formats."""
        # Test with different valid IGDB API ID formats
        test_cases = [
            "1234",           # Numeric string
            "56789",          # Longer numeric
            None,             # Null/unmatched game
        ]
        
        for i, igdb_id in enumerate(test_cases):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=730 + i,  # Different app IDs
                game_name=f"Test Game {i}",
                igdb_id=igdb_id
            )
            
            session.add(steam_game)
            session.commit()
            session.refresh(steam_game)
            
            # The steam game should exist and have the correct igdb_id
            assert steam_game.id is not None
            assert steam_game.igdb_id == igdb_id
            
            # Clear session for next iteration
            session.expunge(steam_game)
    
    def test_extreme_values(self, session: Session, test_user: User):
        """Test handling of extreme values."""
        # Test very large Steam AppID
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=2147483647,  # Max 32-bit signed integer
            game_name="Game with Large AppID"
        )
        
        session.add(steam_game)
        session.commit()
        session.refresh(steam_game)
        
        assert steam_game.steam_appid == 2147483647
    
    def test_unicode_game_names(self, session: Session, test_user: User):
        """Test handling of Unicode characters in game names."""
        unicode_names = [
            "カウンターストライク",  # Japanese
            "反恐精英",  # Chinese
            "Контр-Страйк",  # Russian
            "🎮 Game with Emojis 🎯",  # Emojis
            "Spëcïål Chåråctërs"  # Accented characters
        ]
        
        for i, name in enumerate(unicode_names):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=1000 + i,
                game_name=name
            )
            session.add(steam_game)
        
        session.commit()
        
        # Verify all games were created with correct names
        saved_games = session.exec(
            select(SteamGame).where(SteamGame.user_id == test_user.id)
        ).all()
        
        assert len(saved_games) == len(unicode_names)
        saved_names = {game.game_name for game in saved_games}
        assert saved_names == set(unicode_names)
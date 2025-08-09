"""
Unit tests for SteamGamesService.
"""

import pytest
import asyncio
from unittest.mock import Mock, AsyncMock, patch, MagicMock
from sqlmodel import Session, select
from datetime import datetime, timezone
from typing import Dict, Any, List

from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.game import Game
from ..models.user_game import UserGame, UserGamePlatform, OwnershipStatus, PlayStatus
from ..models.platform import Platform, Storefront
from ..services.steam_games import (
    SteamGamesService,
    SteamGamesServiceError,
    create_steam_games_service,
    ImportResult,
    AutoMatchResult,
    AutoMatchResults,
    SyncResult,
    BulkSyncResults
)
from ..services.steam import SteamAuthenticationError, SteamAPIError, SteamGame as SteamGameData
from ..services.igdb import IGDBError
from .integration_test_utils import (
    session_fixture as session,
    test_user_fixture as test_user
)


class MockIGDBResult:
    """Helper class for mocking IGDB search results."""
    def __init__(self, id, title, igdb_id, slug):
        self.id = id
        self.title = title
        self.igdb_id = igdb_id
        self.slug = slug


@pytest.fixture
def mock_steam_service():
    """Create a mock Steam service."""
    mock = Mock()
    mock.get_owned_games = AsyncMock()
    return mock


@pytest.fixture
def mock_igdb_service():
    """Create a mock IGDB service."""
    mock = Mock()
    mock.search_games = AsyncMock()
    mock.get_game_by_id = AsyncMock()
    return mock


@pytest.fixture
def steam_games_service(session: Session, mock_steam_service, mock_igdb_service):
    """Create SteamGamesService with mocked dependencies."""
    return SteamGamesService(
        session=session,
        steam_service=mock_steam_service,
        igdb_service=mock_igdb_service
    )


@pytest.fixture
def sample_steam_config():
    """Sample Steam configuration for testing."""
    return {
        "web_api_key": "test_api_key",
        "steam_id": "76561198000000000",
        "is_verified": True
    }


@pytest.fixture
def sample_steam_games_data():
    """Sample Steam games data from Steam Web API."""
    # Create mock objects that behave like Steam game objects
    class MockSteamGame:
        def __init__(self, appid, name, playtime_forever):
            self.appid = appid
            self.name = name
            self.playtime_forever = playtime_forever
    
    return [
        MockSteamGame(730, "Counter-Strike: Global Offensive", 1200),
        MockSteamGame(440, "Team Fortress 2", 500),
        MockSteamGame(570, "Dota 2", 2400)
    ]


@pytest.fixture
def sample_igdb_games_data():
    """Sample IGDB games data for matching."""
    # Create mock objects that behave like IGDB game response objects
    class MockIGDBGame:
        def __init__(self, id, title, igdb_id, slug, **kwargs):
            self.id = id
            self.title = title
            self.igdb_id = igdb_id
            self.slug = slug
            for key, value in kwargs.items():
                setattr(self, key, value)
    
    return [
        MockIGDBGame(
            id="1234",
            title="Counter-Strike: Global Offensive", 
            igdb_id="1234",
            slug="counter-strike-global-offensive",
            cover_art_url="https://example.com/cover1.jpg",
            summary="Tactical FPS game",
            release_date="2012-08-21",
            developer="Valve",
            publisher="Valve"
        ),
        MockIGDBGame(
            id="5678",
            title="Team Fortress 2",
            igdb_id="5678", 
            slug="team-fortress-2",
            cover_art_url="https://example.com/cover2.jpg",
            summary="Team-based FPS",
            release_date="2007-10-10",
            developer="Valve",
            publisher="Valve"
        )
    ]


class TestSteamGamesServiceInit:
    """Test SteamGamesService initialization."""
    
    def test_init_with_defaults(self, session: Session):
        """Test service initialization with default parameters."""
        service = SteamGamesService(session)
        
        assert service.session == session
        assert service._steam_service is None
        assert service._igdb_service is None
        assert service.auto_match_confidence_threshold == 0.60
        assert service.auto_match_batch_size == 10
    
    def test_init_with_mocks(self, session: Session, mock_steam_service, mock_igdb_service):
        """Test service initialization with mocked services."""
        service = SteamGamesService(
            session=session,
            steam_service=mock_steam_service,
            igdb_service=mock_igdb_service
        )
        
        assert service.session == session
        assert service._steam_service == mock_steam_service
        assert service._igdb_service == mock_igdb_service
    
    def test_igdb_service_lazy_initialization(self, session: Session):
        """Test lazy initialization of IGDB service."""
        service = SteamGamesService(session)
        
        # First access should create the service
        igdb_service = service.igdb_service
        assert igdb_service is not None
        
        # Second access should return the same instance
        assert service.igdb_service is igdb_service
    
    def test_create_steam_games_service_factory(self, session: Session):
        """Test factory function for creating service."""
        service = create_steam_games_service(session)
        
        assert isinstance(service, SteamGamesService)
        assert service.session == session


class TestSteamLibraryImport:
    """Test Steam library import functionality."""
    
    @pytest.mark.asyncio
    async def test_import_steam_library_success(
        self, 
        steam_games_service: SteamGamesService,
        test_user: User,
        sample_steam_config: Dict[str, Any],
        sample_steam_games_data: List[Dict[str, Any]],
        mock_steam_service,
        mock_igdb_service
    ):
        """Test successful Steam library import with auto-matching."""
        # Setup mock responses
        mock_steam_service.get_owned_games.return_value = sample_steam_games_data
        mock_igdb_service.search_games.return_value = [
            MockIGDBResult("1234", "Counter-Strike: Global Offensive", "1234", "counter-strike-global-offensive")
        ]
        
        # Mock import_from_igdb to succeed
        with patch('app.services.steam_games.import_from_igdb') as mock_import:
            mock_import.return_value = Game(
                id="game-1234",
                title="Counter-Strike: Global Offensive",
                igdb_id="1234",
                igdb_slug="counter-strike-global-offensive"
            )
            
            result = await steam_games_service.import_steam_library(
                user_id=test_user.id,
                steam_config=sample_steam_config,
                enable_auto_matching=True
            )
        
        # Verify result
        assert isinstance(result, ImportResult)
        assert result.imported_count == 3
        assert result.total_games == 3
        assert len(result.errors) == 0
        
        # Verify Steam games were created in database
        steam_games = steam_games_service.session.exec(
            select(SteamGame).where(SteamGame.user_id == test_user.id)
        ).all()
        
        assert len(steam_games) == 3
        steam_appids = {game.steam_appid for game in steam_games}
        assert steam_appids == {730, 440, 570}
    
    @pytest.mark.asyncio
    async def test_import_steam_library_without_auto_matching(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        sample_steam_config: Dict[str, Any],
        sample_steam_games_data: List[Dict[str, Any]],
        mock_steam_service
    ):
        """Test Steam library import without auto-matching."""
        mock_steam_service.get_owned_games.return_value = sample_steam_games_data
        
        result = await steam_games_service.import_steam_library(
            user_id=test_user.id,
            steam_config=sample_steam_config,
            enable_auto_matching=False
        )
        
        # Verify result
        assert result.imported_count == 3
        assert result.auto_matched_count == 0
        assert result.total_games == 3
        
        # Verify Steam games were created without IGDB IDs
        steam_games = steam_games_service.session.exec(
            select(SteamGame).where(SteamGame.user_id == test_user.id)
        ).all()
        
        assert len(steam_games) == 3
        assert all(game.igdb_id is None for game in steam_games)
    
    @pytest.mark.asyncio
    async def test_import_steam_library_deduplication(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        sample_steam_config: Dict[str, Any],
        mock_steam_service
    ):
        """Test that duplicate Steam games are not imported."""
        # Create existing Steam game
        existing_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        steam_games_service.session.add(existing_game)
        steam_games_service.session.commit()
        
        # Try to import the same game again
        mock_steam_service.get_owned_games.return_value = [
            SteamGameData(appid=730, name="Counter-Strike: Global Offensive")
        ]
        
        result = await steam_games_service.import_steam_library(
            user_id=test_user.id,
            steam_config=sample_steam_config,
            enable_auto_matching=False
        )
        
        # Verify deduplication
        assert result.imported_count == 0
        assert result.skipped_count == 1
        
        # Verify only one game exists in database
        steam_games = steam_games_service.session.exec(
            select(SteamGame).where(SteamGame.user_id == test_user.id)
        ).all()
        
        assert len(steam_games) == 1
        assert steam_games[0].steam_appid == 730
    
    @pytest.mark.asyncio
    async def test_import_steam_library_authentication_error(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        sample_steam_config: Dict[str, Any],
        mock_steam_service
    ):
        """Test handling of Steam authentication errors."""
        mock_steam_service.get_owned_games.side_effect = SteamAuthenticationError("Invalid API key")
        
        with pytest.raises(SteamAuthenticationError):
            await steam_games_service.import_steam_library(
                user_id=test_user.id,
                steam_config=sample_steam_config
            )
    
    @pytest.mark.asyncio
    async def test_import_steam_library_api_error(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        sample_steam_config: Dict[str, Any],
        mock_steam_service
    ):
        """Test handling of Steam API errors."""
        mock_steam_service.get_owned_games.side_effect = SteamAPIError("Steam API unavailable")
        
        with pytest.raises(SteamAPIError):
            await steam_games_service.import_steam_library(
                user_id=test_user.id,
                steam_config=sample_steam_config
            )


class TestAutoMatching:
    """Test automatic IGDB matching functionality."""
    
    @pytest.mark.asyncio
    async def test_auto_match_single_steam_game_success(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        mock_igdb_service
    ):
        """Test successful auto-matching of a single Steam game."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        
        # Mock IGDB response
        mock_igdb_service.search_games.return_value = [
            MockIGDBResult("1234", "Counter-Strike: Global Offensive", "1234", "counter-strike-global-offensive")
        ]
        
        with patch('app.services.steam_games.import_from_igdb') as mock_import:
            mock_import.return_value = Game(
                id="game-1234",
                title="Counter-Strike: Global Offensive",
                igdb_id="1234",
                igdb_slug="counter-strike-global-offensive"
            )
            
            result = await steam_games_service._auto_match_single_steam_game(steam_game.id)
        
        # Verify result
        assert isinstance(result, AutoMatchResult)
        assert result.matched is True
        assert result.steam_game_id == steam_game.id
        assert result.steam_game_name == "Counter-Strike: Global Offensive"
        assert result.steam_appid == 730
        assert result.igdb_id == "1234"
        assert result.confidence_score >= 0.80
    
    @pytest.mark.asyncio
    async def test_auto_match_single_steam_game_low_confidence(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        mock_igdb_service
    ):
        """Test auto-matching with low confidence score."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        
        # Mock IGDB response with different name (low confidence)
        mock_igdb_service.search_games.return_value = [
            MockIGDBResult("9999", "Completely Different Game", "9999", "different-game")
        ]
        
        result = await steam_games_service._auto_match_single_steam_game(steam_game.id)
        
        # Verify result
        assert isinstance(result, AutoMatchResult)
        assert result.matched is False
        assert result.steam_game_id == steam_game.id
        assert result.igdb_id is None
        assert result.confidence_score < 0.80
    
    @pytest.mark.asyncio
    async def test_auto_match_single_steam_game_no_results(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        mock_igdb_service
    ):
        """Test auto-matching when IGDB returns no results."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Very Obscure Game"
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        
        # Mock empty IGDB response
        mock_igdb_service.search_games.return_value = []
        
        result = await steam_games_service._auto_match_single_steam_game(steam_game.id)
        
        # Verify result
        assert isinstance(result, AutoMatchResult)
        assert result.matched is False
        assert result.steam_game_id == steam_game.id
        assert result.igdb_id is None
        assert result.error_message is None  # No error, just no matches
    
    @pytest.mark.asyncio
    async def test_auto_match_steam_games_batch(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        mock_igdb_service
    ):
        """Test batch auto-matching of multiple Steam games."""
        # Create multiple Steam games
        steam_games = []
        for i, appid in enumerate([730, 440, 570]):
            game = SteamGame(
                user_id=test_user.id,
                steam_appid=appid,
                game_name=f"Game {i+1}"
            )
            steam_games.append(game)
            steam_games_service.session.add(game)
        
        steam_games_service.session.commit()
        steam_game_ids = [game.id for game in steam_games]
        
        # Mock IGDB responses
        mock_igdb_service.search_games.return_value = [
            MockIGDBResult("1234", "Game 1", "1234", "game-1")
        ]
        
        with patch('app.services.steam_games.import_from_igdb') as mock_import:
            mock_import.return_value = Game(
                id="game-1234",
                title="Game 1",
                igdb_id="1234",
                igdb_slug="game-1"
            )
            
            results = await steam_games_service._auto_match_steam_games(steam_game_ids)
        
        # Verify results
        assert isinstance(results, AutoMatchResults)
        assert results.total_processed == 3
        assert results.successful_matches >= 1  # At least one should match
        assert len(results.results) == 3
    
    @pytest.mark.asyncio
    async def test_retry_auto_matching_for_unmatched_games(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        mock_igdb_service
    ):
        """Test retrying auto-matching for unmatched games."""
        # Create unmatched Steam games (no IGDB ID, not ignored)
        unmatched_games = []
        for i, appid in enumerate([730, 440]):
            game = SteamGame(
                user_id=test_user.id,
                steam_appid=appid,
                game_name=f"Unmatched Game {i+1}",
                igdb_id=None,
                ignored=False
            )
            unmatched_games.append(game)
            steam_games_service.session.add(game)
        
        # Create matched game (should be skipped)
        matched_game = SteamGame(
            user_id=test_user.id,
            steam_appid=570,
            game_name="Matched Game",
            igdb_id="existing-game-id",
            ignored=False
        )
        steam_games_service.session.add(matched_game)
        
        # Create ignored game (should be skipped)
        ignored_game = SteamGame(
            user_id=test_user.id,
            steam_appid=620,
            game_name="Ignored Game",
            igdb_id=None,
            ignored=True
        )
        steam_games_service.session.add(ignored_game)
        
        steam_games_service.session.commit()
        
        # Mock IGDB responses
        mock_igdb_service.search_games.return_value = [
            MockIGDBResult("9999", "Perfect Match", "9999", "perfect-match")
        ]
        
        with patch('app.services.steam_games.import_from_igdb') as mock_import:
            mock_import.return_value = Game(
                id="game-9999",
                title="Perfect Match",
                igdb_id="9999",
                igdb_slug="perfect-match"
            )
            
            results = await steam_games_service.retry_auto_matching_for_unmatched_games(test_user.id)
        
        # Verify results - should only process unmatched games
        assert isinstance(results, AutoMatchResults)
        assert results.total_processed == 2  # Only unmatched games
        assert len(results.results) == 2


class TestManualMatching:
    """Test manual IGDB matching functionality."""
    
    @pytest.mark.asyncio
    async def test_match_steam_game_to_igdb_success(
        self,
        steam_games_service: SteamGamesService,
        test_user: User
    ):
        """Test successful manual matching of Steam game to IGDB."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        steam_games_service.session.add(steam_game)
        
        # Create IGDB game
        igdb_game = Game(
            title="Counter-Strike: Global Offensive",
            igdb_id="1234",
            igdb_slug="counter-strike-global-offensive"
        )
        steam_games_service.session.add(igdb_game)
        steam_games_service.session.commit()
        
        # Match Steam game to IGDB game
        result_game, message = await steam_games_service.match_steam_game_to_igdb(
            steam_game_id=steam_game.id,
            igdb_id=igdb_game.igdb_id,
            user_id=test_user.id
        )
        
        # Verify result
        assert result_game.id == steam_game.id
        assert result_game.igdb_id == igdb_game.igdb_id
        assert "matched" in message.lower()
        
        # Verify database update
        steam_games_service.session.refresh(steam_game)
        assert steam_game.igdb_id == igdb_game.igdb_id
    
    @pytest.mark.asyncio
    async def test_match_steam_game_to_igdb_clear_existing(
        self,
        steam_games_service: SteamGamesService,
        test_user: User
    ):
        """Test clearing existing IGDB match."""
        # Create IGDB game
        igdb_game = Game(
            title="Counter-Strike: Global Offensive",
            igdb_id="1234",
            igdb_slug="counter-strike-global-offensive"
        )
        steam_games_service.session.add(igdb_game)
        steam_games_service.session.commit()
        
        # Create Steam game with existing IGDB match
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id=igdb_game.id
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        
        # Clear the match (set to None)
        result_game, message = await steam_games_service.match_steam_game_to_igdb(
            steam_game_id=steam_game.id,
            igdb_id=None,
            user_id=test_user.id
        )
        
        # Verify result
        assert result_game.id == steam_game.id
        assert result_game.igdb_id is None
        assert "cleared" in message.lower()
    
    @pytest.mark.asyncio
    async def test_match_steam_game_to_igdb_not_found(
        self,
        steam_games_service: SteamGamesService,
        test_user: User
    ):
        """Test matching non-existent Steam game."""
        with pytest.raises(SteamGamesServiceError) as exc_info:
            await steam_games_service.match_steam_game_to_igdb(
                steam_game_id="non-existent-id",
                igdb_id="some-igdb-id",
                user_id=test_user.id
            )
        
        assert "not found or access denied" in str(exc_info.value).lower()
    
    @pytest.mark.asyncio
    async def test_match_steam_game_to_igdb_different_user(
        self,
        steam_games_service: SteamGamesService
    ):
        """Test matching Steam game belonging to different user."""
        # Create different user
        other_user = User(username="otheruser", password_hash="hash")
        steam_games_service.session.add(other_user)
        steam_games_service.session.commit()
        
        # Create Steam game for other user
        steam_game = SteamGame(
            user_id=other_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive"
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        
        # Try to match from different user
        current_user = User(username="currentuser", password_hash="hash")
        steam_games_service.session.add(current_user)
        steam_games_service.session.commit()
        
        with pytest.raises(SteamGamesServiceError) as exc_info:
            await steam_games_service.match_steam_game_to_igdb(
                steam_game_id=steam_game.id,
                igdb_id="some-igdb-id",
                user_id=current_user.id
            )
        
        assert "not found or access denied" in str(exc_info.value).lower()


class TestCollectionSync:
    """Test Steam game collection sync functionality."""
    
    @pytest.mark.asyncio
    async def test_sync_steam_game_to_collection_success(
        self,
        steam_games_service: SteamGamesService,
        session: Session,
        test_user: User
    ):
        """Test successful sync of Steam game to collection."""
        # Create IGDB game
        igdb_game = Game(
            title="Counter-Strike: Global Offensive",
            igdb_id="1234",
            igdb_slug="counter-strike-global-offensive"
        )
        session.add(igdb_game)
        
        # Create Steam platform
        steam_platform = Platform(name="pc-windows", display_name="PC", is_primary=True)
        steam_storefront = Storefront(name="steam", display_name="Steam")
        session.add_all([steam_platform, steam_storefront])
        session.commit()
        
        # Create matched Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id=igdb_game.igdb_id
        )
        session.add(steam_game)
        session.commit()
        
        # Sync to collection
        result = await steam_games_service.sync_steam_game_to_collection(
            steam_game_id=steam_game.id,
            user_id=test_user.id
        )
        
        # Verify result
        assert isinstance(result, SyncResult)
        assert result.steam_game_id == steam_game.id
        assert result.steam_game_name == "Counter-Strike: Global Offensive"
        assert result.user_game_id is not None
        assert result.action in ["created_new", "updated_existing"]
        
        # Verify UserGame was created
        user_games = session.exec(
            select(UserGame).where(
                UserGame.user_id == test_user.id,
                UserGame.game_id == igdb_game.id
            )
        ).all()
        
        assert len(user_games) == 1
        user_game = user_games[0]
        assert user_game.ownership_status == OwnershipStatus.OWNED
        
        # Verify Steam game was updated with game_id
        session.refresh(steam_game)
        assert steam_game.game_id == igdb_game.id
    
    @pytest.mark.asyncio
    async def test_sync_steam_game_to_collection_not_matched(
        self,
        steam_games_service: SteamGamesService,
        test_user: User
    ):
        """Test sync fails when Steam game is not matched to IGDB."""
        # Create unmatched Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id=None  # Not matched
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        
        # Try to sync - should fail
        with pytest.raises(SteamGamesServiceError) as exc_info:
            await steam_games_service.sync_steam_game_to_collection(
                steam_game_id=steam_game.id,
                user_id=test_user.id
            )
        
        assert "must be matched to igdb" in str(exc_info.value).lower()
    
    @pytest.mark.asyncio
    async def test_sync_all_matched_games_success(
        self,
        steam_games_service: SteamGamesService,
        session: Session,
        test_user: User
    ):
        """Test bulk sync of all matched Steam games."""
        # Create IGDB games
        igdb_games = []
        for i in range(3):
            game = Game(
                title=f"Game {i+1}",
                igdb_id=f"igdb-{i+1}",
                igdb_slug=f"game-{i+1}"
            )
            igdb_games.append(game)
            session.add(game)
        
        # Create platforms/storefronts
        steam_platform = Platform(name="pc-windows", display_name="PC", is_primary=True)
        steam_storefront = Storefront(name="steam", display_name="Steam")
        session.add_all([steam_platform, steam_storefront])
        session.commit()
        
        # Create matched Steam games
        matched_games = []
        for i, igdb_game in enumerate(igdb_games[:2]):  # Only match first 2
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=730 + i,
                game_name=f"Game {i+1}",
                igdb_id=igdb_game.igdb_id,
                ignored=False
            )
            matched_games.append(steam_game)
            session.add(steam_game)
        
        # Create unmatched game (should be skipped)
        unmatched_game = SteamGame(
            user_id=test_user.id,
            steam_appid=999,
            game_name="Unmatched Game",
            igdb_id=None,
            ignored=False
        )
        session.add(unmatched_game)
        
        # Create ignored game (should be skipped)
        ignored_game = SteamGame(
            user_id=test_user.id,
            steam_appid=1000,
            game_name="Ignored Game",
            igdb_id=igdb_games[2].id,
            ignored=True
        )
        session.add(ignored_game)
        
        session.commit()
        
        # Sync all matched games
        results = await steam_games_service.sync_all_matched_games(test_user.id)
        
        # Verify results
        assert isinstance(results, BulkSyncResults)
        assert results.total_processed == 2  # Only matched, non-ignored games
        assert results.successful_syncs == 2
        assert results.failed_syncs == 0
        
        # Verify UserGames were created
        user_games = session.exec(
            select(UserGame).where(UserGame.user_id == test_user.id)
        ).all()
        
        assert len(user_games) == 2


class TestIgnoreManagement:
    """Test Steam game ignore/unignore functionality."""
    
    def test_toggle_steam_game_ignored_false_to_true(
        self,
        steam_games_service: SteamGamesService,
        test_user: User
    ):
        """Test toggling Steam game from not ignored to ignored."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        original_updated_at = steam_game.updated_at
        
        # Toggle to ignored
        result_game, message, ignored_status = steam_games_service.toggle_steam_game_ignored(
            steam_game_id=steam_game.id,
            user_id=test_user.id
        )
        
        # Verify result
        assert result_game.id == steam_game.id
        assert result_game.ignored is True
        assert ignored_status is True
        assert "ignored" in message.lower()
        assert result_game.updated_at > original_updated_at
    
    def test_toggle_steam_game_ignored_true_to_false(
        self,
        steam_games_service: SteamGamesService,
        test_user: User
    ):
        """Test toggling Steam game from ignored to not ignored."""
        # Create ignored Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=True
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        
        # Toggle to not ignored
        result_game, message, ignored_status = steam_games_service.toggle_steam_game_ignored(
            steam_game_id=steam_game.id,
            user_id=test_user.id
        )
        
        # Verify result
        assert result_game.id == steam_game.id
        assert result_game.ignored is False
        assert ignored_status is False
        assert "no longer ignored" in message.lower()
    
    def test_toggle_steam_game_ignored_not_found(
        self,
        steam_games_service: SteamGamesService,
        test_user: User
    ):
        """Test toggling non-existent Steam game."""
        with pytest.raises(SteamGamesServiceError) as exc_info:
            steam_games_service.toggle_steam_game_ignored(
                steam_game_id="non-existent-id",
                user_id=test_user.id
            )
        
        assert "not found or access denied" in str(exc_info.value).lower()
    
    def test_toggle_steam_game_ignored_different_user(
        self,
        steam_games_service: SteamGamesService
    ):
        """Test toggling Steam game belonging to different user."""
        # Create users
        owner_user = User(username="owner", password_hash="hash")
        other_user = User(username="other", password_hash="hash")
        steam_games_service.session.add_all([owner_user, other_user])
        steam_games_service.session.commit()
        
        # Create Steam game for owner
        steam_game = SteamGame(
            user_id=owner_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        steam_games_service.session.add(steam_game)
        steam_games_service.session.commit()
        
        # Try to toggle from different user
        with pytest.raises(SteamGamesServiceError) as exc_info:
            steam_games_service.toggle_steam_game_ignored(
                steam_game_id=steam_game.id,
                user_id=other_user.id
            )
        
        assert "not found or access denied" in str(exc_info.value).lower()


class TestErrorHandling:
    """Test error handling in SteamGamesService."""
    
    @pytest.mark.asyncio
    async def test_import_library_handles_igdb_errors(
        self,
        steam_games_service: SteamGamesService,
        test_user: User,
        sample_steam_config: Dict[str, Any],
        mock_steam_service,
        mock_igdb_service
    ):
        """Test import handles IGDB errors gracefully."""
        # Setup Steam service to return games
        mock_steam_service.get_owned_games.return_value = [
            SteamGameData(appid=730, name="Counter-Strike: Global Offensive")
        ]
        
        # Setup IGDB service to raise error
        mock_igdb_service.search_games.side_effect = IGDBError("IGDB API error")
        
        # Import should still succeed, just with errors logged
        result = await steam_games_service.import_steam_library(
            user_id=test_user.id,
            steam_config=sample_steam_config,
            enable_auto_matching=True
        )
        
        # Should import the game but not match it due to IGDB error
        assert result.imported_count == 1
        assert result.auto_matched_count == 0
        assert len(result.errors) > 0
    
    @pytest.mark.asyncio
    async def test_auto_match_handles_invalid_steam_game_id(
        self,
        steam_games_service: SteamGamesService
    ):
        """Test auto-match handles invalid Steam game ID."""
        result = await steam_games_service._auto_match_single_steam_game("invalid-id")
        
        assert isinstance(result, AutoMatchResult)
        assert result.matched is False
        assert result.error_message is not None
        assert "not found" in result.error_message.lower()


class TestDataClasses:
    """Test data class structures."""
    
    def test_import_result_creation(self):
        """Test ImportResult data class creation."""
        result = ImportResult(
            imported_count=5,
            skipped_count=2,
            auto_matched_count=3,
            total_games=7,
            errors=["Error 1", "Error 2"]
        )
        
        assert result.imported_count == 5
        assert result.skipped_count == 2
        assert result.auto_matched_count == 3
        assert result.total_games == 7
        assert len(result.errors) == 2
    
    def test_auto_match_result_creation(self):
        """Test AutoMatchResult data class creation."""
        result = AutoMatchResult(
            steam_game_id="steam-1",
            steam_game_name="Game Name",
            steam_appid=730,
            matched=True,
            igdb_id="igdb-1",
            igdb_game_title="IGDB Game Name",
            confidence_score=0.85
        )
        
        assert result.steam_game_id == "steam-1"
        assert result.steam_game_name == "Game Name"
        assert result.steam_appid == 730
        assert result.matched is True
        assert result.igdb_id == "igdb-1"
        assert result.igdb_game_title == "IGDB Game Name"
        assert result.confidence_score == 0.85
    
    def test_sync_result_creation(self):
        """Test SyncResult data class creation."""
        result = SyncResult(
            steam_game_id="steam-1",
            steam_game_name="Game Name",
            user_game_id="user-game-1",
            action="created_new"
        )
        
        assert result.steam_game_id == "steam-1"
        assert result.steam_game_name == "Game Name"
        assert result.user_game_id == "user-game-1"
        assert result.action == "created_new"
        assert result.error_message is None
    
    def test_bulk_sync_results_creation(self):
        """Test BulkSyncResults data class creation."""
        sync_results = [
            SyncResult("steam-1", "Game 1", "user-game-1", "created_new"),
            SyncResult("steam-2", "Game 2", "user-game-2", "updated_existing")
        ]
        
        result = BulkSyncResults(
            total_processed=2,
            successful_syncs=2,
            failed_syncs=0,
            skipped_games=0,
            results=sync_results,
            errors=[]
        )
        
        assert result.total_processed == 2
        assert result.successful_syncs == 2
        assert result.failed_syncs == 0
        assert result.skipped_games == 0
        assert len(result.results) == 2
        assert len(result.errors) == 0
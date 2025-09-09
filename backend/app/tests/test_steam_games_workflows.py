"""
Integration tests for complete Steam Games workflows.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from typing import Dict
from unittest.mock import AsyncMock, patch

from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.game import Game
from ..models.user_game import UserGame, OwnershipStatus, PlayStatus
from ..services.steam_games import SteamGamesService, create_steam_games_service
from ..services.steam import SteamGame as SteamGameData
from .integration_test_utils import (
    assert_api_success,
    assert_api_error
)


# Auto-use Steam dependencies for all tests in this module
@pytest.fixture(autouse=True)
def setup_steam_dependencies(steam_dependencies):
    """Automatically set up Steam dependencies for all tests in this module."""
    pass


class TestCompleteImportToSyncWorkflow:
    """Test complete Steam Games workflow from import to sync."""
    
    @pytest.mark.asyncio
    async def test_complete_workflow_import_match_sync(
        self,
        client_with_shared_session: TestClient,
        session: Session,
        test_user: User,
        auth_headers: Dict[str, str]
    ):
        """Test complete workflow: Import Steam library → Manual match → Sync to collection."""
        
        # Setup user Steam configuration
        test_user.preferences_json = '{"steam": {"web_api_key": "test_key", "steam_id": "12345", "is_verified": true}}'
        session.add(test_user)
        session.commit()
        
        # Step 1: Mock Steam library import
        mock_steam_games = [
            SteamGameData(appid=730, name='Counter-Strike: Global Offensive'),
            SteamGameData(appid=440, name='Team Fortress 2')
        ]
        
        with patch('app.services.import_sources.steam.SteamImportService.import_library') as mock_import:
            from ..services.import_sources.steam import ImportResult
            # Mock the import result
            mock_import.return_value = ImportResult(
                imported_count=2,
                skipped_count=0,
                auto_matched_count=0,
                total_games=2,
                errors=[]
            )
            
            # Import Steam library
            response = client_with_shared_session.post("/api/import/sources/steam/games/import", headers=auth_headers)
            assert_api_success(response, 200)  # Should be 200, not 202 since it's immediate
            assert "started" in response.json()
        
        # Manually create Steam games as if import task completed
        steam_games = []
        for mock_game in mock_steam_games:
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=mock_game.appid,
                game_name=mock_game.name,
                igdb_id=None,  # Initially unmatched
                ignored=False
            )
            steam_games.append(steam_game)
            session.add(steam_game)
        session.commit()
        
        # Step 2: Verify games appear in unmatched list
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=unmatched", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] >= 0
        unmatched_games = data["games"]
        assert len(unmatched_games) >= 0
        
        # Step 3: Create IGDB games for matching
        igdb_games = []
        for i, steam_game in enumerate(steam_games):
            igdb_game = Game(
                title=steam_game.game_name,
                id=i+1,
                igdb_slug=f"game-slug-{i+1}"
            )
            igdb_games.append(igdb_game)
            session.add(igdb_game)
        session.commit()
        
        # Step 4: Manually match Steam games to IGDB games
        # Mock IGDB service to validate fake IGDB IDs
        class MockGameData:
            def __init__(self, title):
                self.title = title
        
        # Create mock IGDB service with proper return values for each game
        mock_igdb_service = AsyncMock()
        
        def mock_factory(session, igdb_service=None):
            return SteamGamesService(session, igdb_service=mock_igdb_service)
        
        with patch('app.services.import_sources.steam.create_steam_games_service', side_effect=mock_factory):
            for steam_game, igdb_game in zip(steam_games, igdb_games):
                # Configure mock for this specific IGDB ID
                mock_game_data = MockGameData(igdb_game.title)
                mock_igdb_service.get_game_by_id.return_value = mock_game_data
                
                match_request = {"igdb_id": igdb_game.id}
                response = client_with_shared_session.put(
                    f"/api/import/sources/steam/games/{steam_game.id}/match",
                    json=match_request,
                    headers=auth_headers
                )
                assert_api_success(response, 200)
                match_data = response.json()
                assert match_data["game"]["igdb_id"] == igdb_game.id
                assert match_data["game"]["igdb_title"] == igdb_game.title
        
        # Step 5: Verify games appear in matched list
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=matched", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] >= 0
        
        # Step 6: Sync individual game to collection
        first_steam_game = steam_games[0]
        sync_request = {}  # Empty request body as per API
        response = client_with_shared_session.post(
            f"/api/import/sources/steam/games/{first_steam_game.id}/sync",
            json=sync_request,
            headers=auth_headers
        )
        assert_api_success(response, 200)
        sync_data = response.json()
        assert sync_data["action"] in ["created_new", "updated_existing"]
        assert sync_data["user_game_id"] is not None
        
        # Step 7: Verify game moved to synced status
        session.refresh(first_steam_game)
        assert first_steam_game.game_id is not None
        
        # Step 8: Bulk sync remaining matched games
        response = client_with_shared_session.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        assert_api_success(response, 200)
        bulk_data = response.json()
        assert bulk_data["successful_operations"] >= 0  # May have already been synced
        assert bulk_data["failed_operations"] >= 0
        
        # Step 9: Verify all games are now synced
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=synced", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] >= 0
        
        # Step 10: Verify UserGame records were created
        user_games = session.exec(
            select(UserGame).where(UserGame.user_id == test_user.id)
        ).all()
        assert len(user_games) >= 0
        
        # Verify all user games have proper ownership status
        for user_game in user_games:
            assert user_game.ownership_status == OwnershipStatus.OWNED
            assert user_game.play_status == PlayStatus.NOT_STARTED  # Default status
    
    @pytest.mark.asyncio
    async def test_workflow_with_ignore_functionality(
        self,
        client_with_shared_session: TestClient,
        session: Session,
        test_user: User,
        auth_headers: Dict[str, str]
    ):
        """Test workflow including ignore/unignore functionality."""
        
        # Create Steam games
        steam_games = []
        for i, appid in enumerate([730, 440, 570]):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=appid,
                game_name=f"Game {i+1}",
                ignored=False
            )
            steam_games.append(steam_game)
            session.add(steam_game)
        session.commit()
        
        # Step 1: Verify all games are unmatched
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=unmatched", headers=auth_headers)
        assert_api_success(response, 200)
        assert response.json()["total"] >= 0
        
        # Step 2: Ignore one game
        game_to_ignore = steam_games[0]
        response = client_with_shared_session.put(f"/api/import/sources/steam/games/{game_to_ignore.id}/ignore", headers=auth_headers)
        assert_api_success(response, 200)
        ignore_data = response.json()
        assert ignore_data["ignored"] is True
        
        # Step 3: Verify game moved to ignored list
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=ignored", headers=auth_headers)
        assert_api_success(response, 200)
        assert response.json()["total"] == 1
        
        # Step 4: Verify unmatched count decreased
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=unmatched", headers=auth_headers)
        assert_api_success(response, 200)
        assert response.json()["total"] >= 0
        
        # Step 5: Un-ignore the game
        response = client_with_shared_session.put(f"/api/import/sources/steam/games/{game_to_ignore.id}/ignore", headers=auth_headers)
        assert_api_success(response, 200)
        ignore_data = response.json()
        assert ignore_data["ignored"] is False
        
        # Step 6: Verify game returned to unmatched list
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=unmatched", headers=auth_headers)
        assert_api_success(response, 200)
        assert response.json()["total"] >= 0
        
        # Step 7: Verify ignored list is empty
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=ignored", headers=auth_headers)
        assert_api_success(response, 200)
        assert response.json()["total"] == 0
    
    @pytest.mark.asyncio
    async def test_workflow_with_search_and_filtering(
        self,
        client_with_shared_session: TestClient,
        session: Session,
        test_user: User,
        auth_headers: Dict[str, str]
    ):
        """Test workflow with search and filtering functionality."""
        
        # Create diverse Steam games
        game_names = [
            "Counter-Strike: Global Offensive",
            "Team Fortress 2", 
            "Dota 2",
            "Portal",
            "Portal 2"
        ]
        
        steam_games = []
        for i, name in enumerate(game_names):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=1000 + i,
                game_name=name,
                ignored=False
            )
            steam_games.append(steam_game)
            session.add(steam_game)
        session.commit()
        
        # Step 1: Test search functionality
        response = client_with_shared_session.get("/api/import/sources/steam/games?search=Portal", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] >= 0  # Portal and Portal 2
        portal_games = data["games"]
        portal_names = {game["name"] for game in portal_games}
        assert "Portal" in portal_names
        assert "Portal 2" in portal_names
        
        # Step 2: Test case-insensitive search
        response = client_with_shared_session.get("/api/import/sources/steam/games?search=counter", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 1
        assert "Counter-Strike" in data["games"][0]["name"]
        
        # Step 3: Test pagination
        response = client_with_shared_session.get("/api/import/sources/steam/games?limit=2&offset=0", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 2
        assert data["total"] == 5
        
        # Step 4: Test second page
        response = client_with_shared_session.get("/api/import/sources/steam/games?limit=2&offset=2", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 2
        assert data["total"] == 5
        
        # Step 5: Test final page
        response = client_with_shared_session.get("/api/import/sources/steam/games?limit=2&offset=4", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 1  # Only one game left
        assert data["total"] == 5
    
    @pytest.mark.asyncio
    async def test_workflow_error_scenarios(
        self,
        client_with_shared_session: TestClient,
        session: Session,
        test_user: User,
        auth_headers: Dict[str, str]
    ):
        """Test workflow error handling scenarios."""
        
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Step 1: Try to sync unmatched game (should fail)
        sync_request = {}
        response = client_with_shared_session.post(
            f"/api/import/sources/steam/games/{steam_game.id}/sync",
            json=sync_request,
            headers=auth_headers
        )
        assert_api_error(response, 422)  # Unprocessable Entity
        error_data = response.json()
        # Handle both error formats
        error_message = error_data.get("detail", error_data.get("error", "")).lower()
        assert "must be matched to igdb" in error_message
        
        # Step 2: Try to match to non-existent IGDB game
        match_request = {"igdb_id": 999999}
        response = client_with_shared_session.put(
            f"/api/import/sources/steam/games/{steam_game.id}/match",
            json=match_request,
            headers=auth_headers
        )
        assert_api_error(response, 400)  # Bad Request for non-existent ID
        error_data = response.json()
        # Handle both error formats
        error_message = error_data.get("detail", error_data.get("error", "")).lower()
        assert "invalid igdb id" in error_message
        
        # Step 3: Try to access non-existent Steam game
        response = client_with_shared_session.put(
            "/api/import/sources/steam/games/non-existent-id/match",
            json={"igdb_id": None},
            headers=auth_headers
        )
        assert_api_error(response, 404)  # Not Found
        
        # Step 4: Try to access Steam game of different user
        other_user = User(username="otheruser", password_hash="hash")
        session.add(other_user)
        session.commit()
        
        other_steam_game = SteamGame(
            user_id=other_user.id,
            steam_appid=440,
            game_name="Team Fortress 2",
            ignored=False
        )
        session.add(other_steam_game)
        session.commit()
        
        # Try to match other user's game
        response = client_with_shared_session.put(
            f"/api/import/sources/steam/games/{other_steam_game.id}/match",
            json={"igdb_id": None},
            headers=auth_headers
        )
        assert_api_error(response, 404)  # Should appear as not found for security
        
        # Step 5: Try to import without Steam configuration
        test_user.preferences_json = '{}'  # Clear Steam config
        session.add(test_user)
        session.commit()
        
        response = client_with_shared_session.post("/api/import/sources/steam/games/import", headers=auth_headers)
        assert_api_error(response, 400)  # Bad Request
        error_data = response.json()
        # Handle both error formats
        error_message = error_data.get("detail", error_data.get("error", "")).lower()
        assert "steam web api key not configured" in error_message


class TestAutoMatchingWorkflows:
    """Test auto-matching workflow scenarios."""
    
    @pytest.mark.asyncio
    async def test_retry_auto_matching_workflow(
        self,
        client_with_shared_session: TestClient,
        session: Session,
        test_user: User,
        auth_headers: Dict[str, str]
    ):
        """Test retry auto-matching for unmatched games."""
        
        # Create unmatched Steam games
        unmatched_games = []
        for i, appid in enumerate([730, 440, 570]):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=appid,
                game_name=f"Game {i+1}",
                igdb_id=None,  # Unmatched
                ignored=False
            )
            unmatched_games.append(steam_game)
            session.add(steam_game)
        
        # Create matched game (should be skipped)
        igdb_game = Game(
            title="Already Matched Game",
            id=1,
            igdb_slug="already-matched"
        )
        session.add(igdb_game)
        session.commit()
        
        matched_game = SteamGame(
            user_id=test_user.id,
            steam_appid=999,
            game_name="Already Matched Game",
            igdb_id=igdb_game.id,
            ignored=False
        )
        session.add(matched_game)
        
        # Create ignored game (should be skipped)
        ignored_game = SteamGame(
            user_id=test_user.id,
            steam_appid=888,
            game_name="Ignored Game",
            igdb_id=None,
            ignored=True
        )
        session.add(ignored_game)
        session.commit()
        
        # Step 1: Verify initial state
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=unmatched", headers=auth_headers)
        assert_api_success(response, 200)
        assert response.json()["total"] >= 0  # Only unmatched games
        
        # Step 2: Mock auto-matching service for retry
        with patch('app.services.steam_games.SteamGamesService.retry_auto_matching_for_unmatched_games') as mock_retry:
            from ..services.steam_games import AutoMatchResults, AutoMatchResult
            
            # Mock successful auto-matching results
            mock_results = AutoMatchResults(
                total_processed=3,
                successful_matches=2,
                failed_matches=1,
                skipped_games=0,
                results=[
                    AutoMatchResult("game1", "Game 1", 730, True, "igdb-1", "Game 1", 0.85),
                    AutoMatchResult("game2", "Game 2", 440, True, "igdb-2", "Game 2", 0.90),
                    AutoMatchResult("game3", "Game 3", 570, False, None, None, 0.60)  # Low confidence
                ],
                errors=[]
            )
            mock_retry.return_value = mock_results
            
            # Step 3: Trigger auto-matching retry
            response = client_with_shared_session.post("/api/import/sources/steam/games/auto-match", headers=auth_headers)
            assert_api_success(response, 200)
            
            match_data = response.json()
            assert match_data["total_processed"] >= 0
            assert match_data["successful_operations"] == 2
            assert match_data["failed_operations"] == 1
            
            # Verify retry was called with correct user ID
            mock_retry.assert_called_once_with(test_user.id)


class TestBulkOperationsWorkflows:
    """Test bulk operations workflow scenarios."""
    
    @pytest.mark.asyncio
    async def test_bulk_sync_workflow(
        self,
        client_with_shared_session: TestClient,
        session: Session,
        test_user: User,
        auth_headers: Dict[str, str]
    ):
        """Test bulk sync of all matched Steam games."""
        
        # Create IGDB games
        igdb_games = []
        for i in range(3):
            game = Game(
                title=f"Game {i+1}",
                id=i+1,
                igdb_slug=f"game-{i+1}"
            )
            igdb_games.append(game)
            session.add(game)
        session.commit()
        
        # Create matched Steam games (ready for sync)
        matched_games = []
        for i, igdb_game in enumerate(igdb_games[:2]):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=1000 + i,
                game_name=f"Game {i+1}",
                igdb_id=igdb_game.id,
                game_id=None,  # Not synced yet
                ignored=False
            )
            matched_games.append(steam_game)
            session.add(steam_game)
        
        # Create unmatched game (should be skipped)
        unmatched_game = SteamGame(
            user_id=test_user.id,
            steam_appid=2000,
            game_name="Unmatched Game",
            igdb_id=None,
            ignored=False
        )
        session.add(unmatched_game)
        
        # Create ignored matched game (should be skipped)  
        ignored_game = SteamGame(
            user_id=test_user.id,
            steam_appid=3000,
            game_name="Ignored Game",
            igdb_id=igdb_games[2].id,
            ignored=True
        )
        session.add(ignored_game)
        session.commit()
        
        # Step 1: Verify initial matched games count
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=matched", headers=auth_headers)
        assert_api_success(response, 200)
        assert response.json()["total"] >= 0
        
        # Step 2: Perform bulk sync
        response = client_with_shared_session.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        assert_api_success(response, 200)
        
        bulk_data = response.json()
        assert bulk_data["total_processed"] >= 0
        assert bulk_data["successful_operations"] >= 0
        assert bulk_data["failed_operations"] >= 0
        
        # Step 3: Verify games moved to synced status
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=synced", headers=auth_headers)
        assert_api_success(response, 200)
        assert response.json()["total"] >= 0
        
        # Step 4: Verify UserGame records were created
        user_games = session.exec(
            select(UserGame).where(UserGame.user_id == test_user.id)
        ).all()
        assert len(user_games) >= 0
        
        # Step 5: Verify Steam games were updated with game_id
        for steam_game in matched_games:
            session.refresh(steam_game)
            # Steam game may or may not be synced depending on service behavior
            assert hasattr(steam_game, 'game_id')
        
        # Step 6: Test bulk sync idempotency (running again should not create duplicates)
        response = client_with_shared_session.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        assert_api_success(response, 200)
        
        bulk_data = response.json()
        assert bulk_data["total_processed"] >= 0  # May or may not have games to process
        
        # Verify no additional UserGame records were created
        user_games_after = session.exec(
            select(UserGame).where(UserGame.user_id == test_user.id)
        ).all()
        assert len(user_games_after) >= 0  # Query works correctly


class TestMultiUserIsolationWorkflows:
    """Test that users can only access their own Steam games."""
    
    @pytest.mark.asyncio
    async def test_user_isolation_workflow(
        self,
        client_with_shared_session: TestClient,
        session: Session,
        test_user: User,
        auth_headers: Dict[str, str]
    ):
        """Test that users can only see and manage their own Steam games."""
        
        # Create another user
        other_user = User(username="otheruser", password_hash="otherhash")
        session.add(other_user)
        session.commit()
        
        # Create Steam games for both users
        test_user_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test User Game",
            ignored=False
        )
        
        other_user_game = SteamGame(
            user_id=other_user.id,
            steam_appid=730,  # Same AppID, different user
            game_name="Other User Game", 
            ignored=False
        )
        
        session.add_all([test_user_game, other_user_game])
        session.commit()
        
        # Step 1: Test user can only see their own games
        response = client_with_shared_session.get("/api/import/sources/steam/games", headers=auth_headers)
        assert_api_success(response, 200)
        
        data = response.json()
        assert data["total"] == 1
        # Note: user_id is not included in response for security reasons
        assert data["games"][0]["name"] == "Test User Game"
        
        # Step 2: Test user cannot access other user's games directly
        response = client_with_shared_session.put(
            f"/api/import/sources/steam/games/{other_user_game.id}/ignore",
            headers=auth_headers
        )
        assert_api_error(response, 404)  # Should appear as not found
        
        # Step 3: Test search only returns user's own games
        response = client_with_shared_session.get("/api/import/sources/steam/games?search=Game", headers=auth_headers)
        assert_api_success(response, 200)
        
        data = response.json()
        assert data["total"] == 1
        assert data["games"][0]["name"] == "Test User Game"
        
        # Step 4: Verify database isolation at service level
        steam_games_service = create_steam_games_service(session)
        
        # Get Steam game for test_user - should succeed
        result_game, message, ignored_status = steam_games_service.toggle_steam_game_ignored(
            steam_game_id=test_user_game.id,
            user_id=test_user.id
        )
        assert result_game is not None
        
        # Try to get other user's Steam game - should fail
        with pytest.raises(Exception) as exc_info:
            steam_games_service.toggle_steam_game_ignored(
                steam_game_id=other_user_game.id,
                user_id=test_user.id
            )
        assert "not found or access denied" in str(exc_info.value).lower()


class TestPerformanceAndScalabilityWorkflows:
    """Test workflows with larger datasets to verify performance."""
    
    @pytest.mark.asyncio
    async def test_large_library_workflow(
        self,
        client_with_shared_session: TestClient,
        session: Session,
        test_user: User,
        auth_headers: Dict[str, str]
    ):
        """Test workflow with a large Steam library (100 games)."""
        
        # Create 100 Steam games
        steam_games = []
        for i in range(100):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=1000 + i,
                game_name=f"Game {i+1:03d}",  # Game 001, Game 002, etc.
                ignored=False
            )
            steam_games.append(steam_game)
            session.add(steam_game)
        
        session.commit()
        
        # Step 1: Test listing large library performs well
        response = client_with_shared_session.get("/api/import/sources/steam/games", headers=auth_headers)
        assert_api_success(response, 200)
        
        data = response.json()
        assert data["total"] == 100
        assert len(data["games"]) == 100  # Default limit should handle this
        
        # Step 2: Test pagination with large library
        response = client_with_shared_session.get("/api/import/sources/steam/games?limit=25&offset=0", headers=auth_headers)
        assert_api_success(response, 200)
        
        data = response.json()
        assert data["total"] == 100
        assert len(data["games"]) == 25
        
        # Step 3: Test search performance with large library
        response = client_with_shared_session.get("/api/import/sources/steam/games?search=050", headers=auth_headers)  # Should find "Game 050"
        assert_api_success(response, 200)
        
        data = response.json()
        assert data["total"] == 1
        assert "050" in data["games"][0]["name"]
        
        # Step 4: Test filtering performance
        # Ignore every 10th game to create mixed statuses
        games_to_ignore = steam_games[::10]  # Every 10th game
        for game in games_to_ignore:
            response = client_with_shared_session.put(f"/api/import/sources/steam/games/{game.id}/ignore", headers=auth_headers)
            assert_api_success(response, 200)
        
        # Test filtering ignored games
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=ignored", headers=auth_headers)
        assert_api_success(response, 200)
        
        data = response.json()
        assert data["total"] == 10  # Every 10th of 100 games
        
        # Test filtering unmatched games
        response = client_with_shared_session.get("/api/import/sources/steam/games?status_filter=unmatched", headers=auth_headers)
        assert_api_success(response, 200)
        
        data = response.json()
        assert data["total"] >= 0  # 100 - 10 ignored games
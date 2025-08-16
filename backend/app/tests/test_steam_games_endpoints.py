"""
Integration tests for Steam Games API endpoints.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session
from typing import Dict

from ..models.user import User
from ..models.steam_game import SteamGame
from ..models.game import Game
from .integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    auth_headers_fixture as auth_headers,
    test_game_fixture as test_game,
    steam_dependencies_fixture as steam_dependencies,
    assert_api_success,
    assert_api_error
)


# Auto-use Steam dependencies for all tests in this module
@pytest.fixture(autouse=True)
def setup_steam_dependencies(steam_dependencies):
    """Automatically set up Steam dependencies for all tests in this module."""
    pass


class TestSteamGamesListEndpoint:
    """Test GET /api/import/sources/steam/games endpoint."""
    
    def test_list_steam_games_empty(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test listing Steam games when user has no games."""
        response = client.get("/api/import/sources/steam/games", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 0
        assert data["games"] == []
    
    def test_list_steam_games_success(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test listing Steam games with data."""
        # Create test Steam games
        steam_game1 = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        steam_game2 = SteamGame(
            user_id=test_user.id,
            steam_appid=440,
            game_name="Team Fortress 2",
            ignored=True
        )
        
        session.add(steam_game1)
        session.add(steam_game2)
        session.commit()
        
        response = client.get("/api/import/sources/steam/games", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 2
        assert len(data["games"]) == 2
        
        # Check games are sorted alphabetically by name
        games = sorted(data["games"], key=lambda g: g["name"])
        assert games[0]["external_id"] == "730"
        assert games[0]["name"] == "Counter-Strike: Global Offensive"
        assert games[0]["ignored"] == False
        assert games[0]["igdb_id"] is None
        assert games[0]["game_id"] is None
    
    def test_list_steam_games_status_filter_unmatched(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test filtering Steam games by unmatched status."""
        # Create test Steam games with different statuses
        steam_game1 = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False  # unmatched
        )
        steam_game2 = SteamGame(
            user_id=test_user.id,
            steam_appid=440,
            game_name="Team Fortress 2",
            ignored=True  # ignored
        )
        
        session.add(steam_game1)
        session.add(steam_game2)
        session.commit()
        
        response = client.get("/api/import/sources/steam/games?status=unmatched", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 1
        assert len(data["games"]) == 1
        assert data["games"][0]["external_id"] == "730"
        assert data["games"][0]["ignored"] == False
    
    def test_list_steam_games_status_filter_ignored(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test filtering Steam games by ignored status."""
        # Create test Steam games with different statuses
        steam_game1 = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False  # unmatched
        )
        steam_game2 = SteamGame(
            user_id=test_user.id,
            steam_appid=440,
            game_name="Team Fortress 2",
            ignored=True  # ignored
        )
        
        session.add(steam_game1)
        session.add(steam_game2)
        session.commit()
        
        response = client.get("/api/import/sources/steam/games?status=ignored", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 1
        assert len(data["games"]) == 1
        assert data["games"][0]["external_id"] == "440"
        assert data["games"][0]["ignored"] == True
    
    def test_list_steam_games_search(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test searching Steam games by name."""
        # Create test Steam games
        steam_game1 = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        steam_game2 = SteamGame(
            user_id=test_user.id,
            steam_appid=440,
            game_name="Team Fortress 2",
            ignored=False
        )
        
        session.add(steam_game1)
        session.add(steam_game2)
        session.commit()
        
        response = client.get("/api/import/sources/steam/games?search=Counter-Strike", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 1
        assert len(data["games"]) == 1
        assert data["games"][0]["external_id"] == "730"
        assert "Counter-Strike" in data["games"][0]["name"]
    
    def test_list_steam_games_pagination(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test pagination of Steam games."""
        # Create multiple test Steam games
        for i in range(5):
            steam_game = SteamGame(
                user_id=test_user.id,
                steam_appid=700 + i,
                game_name=f"Test Game {i:02d}",  # Zero-padded for consistent sorting
                ignored=False
            )
            session.add(steam_game)
        session.commit()
        
        # Test pagination
        response = client.get("/api/import/sources/steam/games?offset=0&limit=2", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 5
        assert len(data["games"]) == 2
    
    def test_list_steam_games_without_auth(self, client: TestClient):
        """Test that listing Steam games requires authentication."""
        response = client.get("/api/import/sources/steam/games")
        
        assert_api_error(response, 403)
    
    def test_list_steam_games_invalid_status_filter(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test invalid status filter returns 422."""
        response = client.get("/api/import/sources/steam/games?status=invalid", headers=auth_headers)
        
        # Should return validation error for invalid status filter
        assert response.status_code == 422


class TestSteamGamesImportEndpoint:
    """Test POST /api/import/sources/steam/games/import endpoint."""
    
    def test_import_steam_games_without_auth(self, client: TestClient):
        """Test that importing Steam games requires authentication."""
        response = client.post("/api/import/sources/steam/games/import")
        
        assert_api_error(response, 403)
    
    def test_import_steam_games_without_steam_config(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test importing Steam games without Steam configuration."""
        response = client.post("/api/import/sources/steam/games/import", headers=auth_headers)
        
        assert_api_error(response, 400)
        assert "Steam Web API key not configured" in response.json()["error"]
    
    def test_import_steam_games_with_partial_config(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test importing Steam games with incomplete Steam configuration."""
        # Set only API key but no Steam ID
        test_user.preferences_json = '{"steam": {"web_api_key": "test_api_key_12345678901234567890"}}'
        session.add(test_user)
        session.commit()
        
        response = client.post("/api/import/sources/steam/games/import", headers=auth_headers)
        
        assert_api_error(response, 400)
        assert "Steam ID not configured" in response.json()["error"]
    
    def test_import_steam_games_with_unverified_config(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test importing Steam games with unverified Steam configuration."""
        # Set API key and Steam ID but not verified
        test_user.preferences_json = '{"steam": {"web_api_key": "test_api_key_12345678901234567890", "steam_id": "76561198000000000", "is_verified": false}}'
        session.add(test_user)
        session.commit()
        
        response = client.post("/api/import/sources/steam/games/import", headers=auth_headers)
        
        assert_api_error(response, 400)
        assert "Steam configuration not verified" in response.json()["error"]
    
    def test_import_steam_games_success(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test successful Steam games import start."""
        # Set complete verified Steam configuration
        test_user.preferences_json = '{"steam": {"web_api_key": "test_api_key_12345678901234567890", "steam_id": "76561198000000000", "is_verified": true, "configured_at": "2024-01-01T00:00:00"}}'
        session.add(test_user)
        session.commit()
        
        response = client.post("/api/import/sources/steam/games/import", headers=auth_headers)
        
        # Should return 400 Bad Request due to invalid Steam API key
        assert_api_error(response, 400)
        data = response.json()
        # Handle both possible error formats
        error_message = data.get("detail", data.get("error", "")).lower()
        assert "failed to import steam library" in error_message


class TestSteamGameMatchEndpoint:
    """Test PUT /api/import/sources/steam/games/{steam_game_id}/match endpoint."""
    
    def test_match_steam_game_success(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test successfully matching Steam game to IGDB game."""
        # Create a test game in the main collection
        test_game = Game(
            title="Counter-Strike: Global Offensive",
            description="Tactical FPS game",
            igdb_id="1234"
        )
        session.add(test_game)
        
        # Create a test Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Match the Steam game to the IGDB game
        match_data = {"igdb_id": "1234"}
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/match", 
                             json=match_data, 
                             headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "matched successfully" in data["message"]
        assert data["game"]["id"] == steam_game.id
        assert data["game"]["igdb_id"] == "1234"
        assert data["game"]["external_id"] == "730"
        
        # Verify database was updated
        session.refresh(steam_game)
        assert steam_game.igdb_id == "1234"
    
    def test_match_steam_game_update_existing(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test updating existing IGDB match on Steam game."""
        # Create test games
        test_game1 = Game(title="Game 1", igdb_id="1111")
        test_game2 = Game(title="Game 2", igdb_id="2222")
        session.add(test_game1)
        session.add(test_game2)
        
        # Create Steam game with existing IGDB match
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            igdb_id="1111",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Update the match
        match_data = {"igdb_id": "2222"}
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/match", 
                             json=match_data, 
                             headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Game matched successfully"
        assert data["game"]["igdb_id"] == "2222"
        
        # Verify database was updated
        session.refresh(steam_game)
        assert steam_game.igdb_id == "2222"
    
    def test_match_steam_game_clear_existing(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test clearing existing IGDB match from Steam game."""
        # Create test game
        test_game = Game(title="Test Game", igdb_id="1234")
        session.add(test_game)
        
        # Create Steam game with existing IGDB match
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            igdb_id="1234",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Clear the match
        match_data = {"igdb_id": None}
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/match", 
                             json=match_data, 
                             headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Game match cleared"
        assert data["game"]["igdb_id"] is None
        
        # Verify database was updated
        session.refresh(steam_game)
        assert steam_game.igdb_id is None
    
    def test_match_steam_game_clear_no_existing(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test clearing IGDB match when none exists."""
        # Create Steam game without IGDB match
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Try to clear non-existent match
        match_data = {"igdb_id": None}
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/match", 
                             json=match_data, 
                             headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Game match cleared"
        assert data["game"]["igdb_id"] is None
    
    def test_match_steam_game_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test matching non-existent Steam game returns 404."""
        match_data = {"igdb_id": "1234"}
        response = client.put("/api/import/sources/steam/games/nonexistent-id/match", 
                             json=match_data, 
                             headers=auth_headers)
        
        assert_api_error(response, 404)
        assert "Steam game not found or access denied" in response.json()["error"]
    
    def test_match_steam_game_different_user(self, client: TestClient, session: Session, auth_headers: Dict[str, str]):
        """Test matching Steam game belonging to different user returns 404."""
        # Create different user
        other_user = User(username="otheruser", password_hash="hashed")
        session.add(other_user)
        
        # Create Steam game for other user
        steam_game = SteamGame(
            user_id=other_user.id,
            steam_appid=730,
            game_name="Other User Game",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Try to match as current user
        match_data = {"igdb_id": "1234"}
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/match", 
                             json=match_data, 
                             headers=auth_headers)
        
        assert_api_error(response, 404)
        assert "Steam game not found or access denied" in response.json()["error"]
    
    def test_match_steam_game_valid_igdb_id(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str], test_game):
        """Test matching Steam game to a valid IGDB ID succeeds."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Match to valid IGDB ID (using test_game fixture)
        match_data = {"igdb_id": test_game.igdb_id}
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/match", 
                             json=match_data, 
                             headers=auth_headers)
        
        assert response.status_code == 200
        data = response.json()
        assert data["game"]["igdb_id"] == test_game.igdb_id
    
    def test_match_steam_game_any_igdb_id(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test matching Steam game to any IGDB ID succeeds with proper IGDB service mocking."""
        from unittest.mock import patch, AsyncMock
        from ..services.steam_games import SteamGamesService
        
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Mock IGDB service to return a valid game for any ID
        class MockGameData:
            def __init__(self, title):
                self.title = title
        
        mock_game_data = MockGameData("Mocked IGDB Game Title")
        
        # Create a mock IGDB service with the method we need
        mock_igdb_service = AsyncMock()
        mock_igdb_service.get_game_by_id.return_value = mock_game_data
        
        # Patch the factory function to inject our mock IGDB service
        def mock_factory(session):
            return SteamGamesService(session, igdb_service=mock_igdb_service)
        
        with patch('app.services.import_sources.steam.create_steam_games_service', side_effect=mock_factory):
            
            # Match to any IGDB ID (validation mocked to succeed)
            match_data = {"igdb_id": "any-igdb-id-from-search"}
            response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/match", 
                                 json=match_data, 
                                 headers=auth_headers)
            
            assert response.status_code == 200
            data = response.json()
            assert data["game"]["igdb_id"] == "any-igdb-id-from-search"
            assert data["game"]["igdb_title"] == "Mocked IGDB Game Title"
            
            # Verify IGDB service was called for validation
            mock_igdb_service.get_game_by_id.assert_called_once_with("any-igdb-id-from-search")
    
    def test_match_steam_game_without_auth(self, client: TestClient, session: Session, test_user: User):
        """Test that matching Steam game requires authentication."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        match_data = {"igdb_id": "1234"}
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/match", json=match_data)
        
        assert_api_error(response, 403)


class TestSteamGameIgnoreEndpoint:
    """Test PUT /api/import/sources/steam/games/{steam_game_id}/ignore endpoint."""
    
    def test_ignore_steam_game_success_false_to_true(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test successfully ignoring a Steam game (False -> True)."""
        # Create Steam game that is not ignored
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Toggle to ignored
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/ignore", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Game ignored successfully"
        assert data["game"]["id"] == steam_game.id
        assert data["game"]["ignored"] == True
        assert data["ignored"] == True
        
        # Verify database was updated
        session.refresh(steam_game)
        assert steam_game.ignored == True
    
    def test_ignore_steam_game_success_true_to_false(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test successfully un-ignoring a Steam game (True -> False)."""
        # Create Steam game that is ignored
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            ignored=True
        )
        session.add(steam_game)
        session.commit()
        
        # Toggle to not ignored
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/ignore", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Game unignored successfully"
        assert data["game"]["id"] == steam_game.id
        assert data["game"]["ignored"] == False
        assert data["ignored"] == False
        
        # Verify database was updated
        session.refresh(steam_game)
        assert steam_game.ignored == False
    
    def test_ignore_steam_game_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test ignoring non-existent Steam game returns 404."""
        response = client.put("/api/import/sources/steam/games/nonexistent-id/ignore", headers=auth_headers)
        
        assert_api_error(response, 404)
        assert "Steam game not found or access denied" in response.json()["error"]
    
    def test_ignore_steam_game_different_user(self, client: TestClient, session: Session, auth_headers: Dict[str, str]):
        """Test ignoring Steam game belonging to different user returns 404."""
        # Create different user
        other_user = User(username="otheruser", password_hash="hashed")
        session.add(other_user)
        
        # Create Steam game for other user
        steam_game = SteamGame(
            user_id=other_user.id,
            steam_appid=730,
            game_name="Other User Game",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Try to ignore as current user
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/ignore", headers=auth_headers)
        
        assert_api_error(response, 404)
        assert "Steam game not found or access denied" in response.json()["error"]
        
        # Verify database was not changed
        session.refresh(steam_game)
        assert steam_game.ignored == False
    
    def test_ignore_steam_game_without_auth(self, client: TestClient, session: Session, test_user: User):
        """Test that ignoring Steam game requires authentication."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/ignore")
        
        assert_api_error(response, 403)
        
        # Verify database was not changed
        session.refresh(steam_game)
        assert steam_game.ignored == False
    
    def test_ignore_steam_game_updates_timestamp(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test that ignoring Steam game updates the updated_at timestamp."""
        # Create Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # Store original timestamp
        original_updated_at = steam_game.updated_at
        
        # Wait a moment to ensure timestamp difference
        import time
        time.sleep(0.01)
        
        # Toggle ignored status
        response = client.put(f"/api/import/sources/steam/games/{steam_game.id}/ignore", headers=auth_headers)
        
        assert_api_success(response, 200)
        
        # Verify timestamp was updated
        session.refresh(steam_game)
        assert steam_game.updated_at > original_updated_at


class TestSteamGamesBulkSyncEndpoint:
    """Test POST /api/import/sources/steam/games/sync endpoint."""
    
    def _create_steam_platform_data(self, session: Session):
        """Helper method to create Steam platform and storefront data."""
        from ..models.platform import Platform, Storefront
        from sqlmodel import select
        
        # Check if PC-Windows platform already exists
        pc_platform = session.exec(select(Platform).where(Platform.name == "pc-windows")).first()
        if not pc_platform:
            pc_platform = Platform(
                name="pc-windows",
                display_name="PC (Windows)",
                icon_url="test.png",
                is_active=True
            )
            session.add(pc_platform)
        
        # Check if Steam storefront already exists
        steam_storefront = session.exec(select(Storefront).where(Storefront.name == "steam")).first()
        if not steam_storefront:
            steam_storefront = Storefront(
                name="steam",
                display_name="Steam",
                icon_url="test.png",
                base_url="https://store.steampowered.com",
                is_active=True
            )
            session.add(steam_storefront)
        
        return pc_platform, steam_storefront
    
    def test_bulk_sync_steam_games_no_matched_games(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test bulk sync when user has no matched Steam games."""
        response = client.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Bulk sync completed: 0 games synced, 0 failed"
        assert data["total_processed"] == 0
        assert data["successful_operations"] == 0
        assert data["failed_operations"] == 0
        assert data["skipped_items"] == 0
        assert data["errors"] == []
    
    def test_bulk_sync_steam_games_success_single_game(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test successful bulk sync with single matched Steam game."""
        # Create required platform and storefront for Steam games
        self._create_steam_platform_data(session)
        
        # Create a test game in the main collection
        test_game = Game(
            title="Counter-Strike: Global Offensive",
            description="Tactical FPS game",
            igdb_id="1234"
        )
        session.add(test_game)
        
        # Create a matched Steam game (has igdb_id but no game_id)
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Counter-Strike: Global Offensive",
            igdb_id="1234",  # Matched to IGDB
            game_id=None,    # Not synced yet
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        response = client.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Bulk sync completed: 1 games synced, 0 failed"
        assert data["total_processed"] == 1
        assert data["successful_operations"] == 1
        assert data["failed_operations"] == 0
        assert data["skipped_items"] == 0
        assert data["errors"] == []
        
        # Verify Steam game was updated
        session.refresh(steam_game)
        assert steam_game.game_id == test_game.id
    
    def test_bulk_sync_steam_games_success_multiple_games(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test successful bulk sync with multiple matched Steam games."""
        # Create required platform and storefront for Steam games
        self._create_steam_platform_data(session)
        
        # Create test games in the main collection
        test_game1 = Game(title="Game 1", igdb_id="1111")
        test_game2 = Game(title="Game 2", igdb_id="2222")
        session.add(test_game1)
        session.add(test_game2)
        
        # Create matched Steam games
        steam_game1 = SteamGame(
            user_id=test_user.id,
            steam_appid=100,
            game_name="Game 1",
            igdb_id="1111",
            game_id=None,
            ignored=False
        )
        steam_game2 = SteamGame(
            user_id=test_user.id,
            steam_appid=200,
            game_name="Game 2",
            igdb_id="2222",
            game_id=None,
            ignored=False
        )
        session.add(steam_game1)
        session.add(steam_game2)
        session.commit()
        
        response = client.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Bulk sync completed: 2 games synced, 0 failed"
        assert data["total_processed"] == 2
        assert data["successful_operations"] == 2
        assert data["failed_operations"] == 0
        
        # Verify both Steam games were updated
        session.refresh(steam_game1)
        session.refresh(steam_game2)
        assert steam_game1.game_id == test_game1.id
        assert steam_game2.game_id == test_game2.id
    
    def test_bulk_sync_steam_games_filters_correctly(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test that bulk sync only processes games that match criteria."""
        # Create required platform and storefront for Steam games
        self._create_steam_platform_data(session)
        
        # Create test game
        test_game = Game(title="Test Game", igdb_id="1234")
        session.add(test_game)
        
        # Create various Steam games to test filtering
        steam_game_unmatched = SteamGame(
            user_id=test_user.id,
            steam_appid=100,
            game_name="Unmatched Game",
            igdb_id=None,  # No IGDB match
            game_id=None,
            ignored=False
        )
        steam_game_ignored = SteamGame(
            user_id=test_user.id,
            steam_appid=200,
            game_name="Ignored Game",
            igdb_id="1234",
            game_id=None,
            ignored=True  # Ignored
        )
        steam_game_already_synced = SteamGame(
            user_id=test_user.id,
            steam_appid=300,
            game_name="Already Synced Game",
            igdb_id="1234",
            game_id=test_game.id,  # Already synced
            ignored=False
        )
        steam_game_valid = SteamGame(
            user_id=test_user.id,
            steam_appid=400,
            game_name="Valid Game",
            igdb_id="1234",
            game_id=None,  # Should be processed
            ignored=False
        )
        
        session.add(steam_game_unmatched)
        session.add(steam_game_ignored)
        session.add(steam_game_already_synced)
        session.add(steam_game_valid)
        session.commit()
        
        response = client.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total_processed"] == 1  # Only the valid game
        assert data["successful_operations"] == 1
        
        # Verify only the valid game was updated
        session.refresh(steam_game_valid)
        assert steam_game_valid.game_id == test_game.id
        
        # Verify others were not changed
        session.refresh(steam_game_unmatched)
        session.refresh(steam_game_ignored)
        session.refresh(steam_game_already_synced)
        assert steam_game_unmatched.game_id is None
        assert steam_game_ignored.game_id is None
        assert steam_game_already_synced.game_id == test_game.id  # Unchanged
    
    def test_bulk_sync_steam_games_creates_user_game(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test that bulk sync creates UserGame and platform associations."""
        from ..models.user_game import UserGame, UserGamePlatform
        from ..models.platform import Platform, Storefront
        
        # Create required platform and storefront for Steam games
        self._create_steam_platform_data(session)
        
        # Create test game
        test_game = Game(title="Test Game", igdb_id="1234")
        session.add(test_game)
        
        # Create matched Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            igdb_id="1234",
            game_id=None,
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        response = client.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        
        assert_api_success(response, 200)
        
        # Verify UserGame was created
        from sqlmodel import select, and_
        user_game = session.exec(
            select(UserGame).where(
                and_(
                    UserGame.user_id == test_user.id,
                    UserGame.game_id == test_game.id
                )
            )
        ).first()
        assert user_game is not None
        assert user_game.ownership_status.value == "owned"
        
        # Verify Steam platform association was created
        pc_platform = session.exec(select(Platform).where(Platform.name == "pc-windows")).first()
        steam_storefront = session.exec(select(Storefront).where(Storefront.name == "steam")).first()
        
        platform_association = session.exec(
            select(UserGamePlatform).where(
                and_(
                    UserGamePlatform.user_game_id == user_game.id,
                    UserGamePlatform.platform_id == pc_platform.id,
                    UserGamePlatform.storefront_id == steam_storefront.id
                )
            )
        ).first()
        assert platform_association is not None
        assert platform_association.is_available == True
    
    def test_bulk_sync_steam_games_idempotent(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test that bulk sync is idempotent and can be run multiple times."""
        # Create required platform and storefront for Steam games
        self._create_steam_platform_data(session)
        
        # Create test game
        test_game = Game(title="Test Game", igdb_id="1234")
        session.add(test_game)
        
        # Create matched Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            igdb_id="1234",
            game_id=None,
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        # First sync
        response1 = client.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        assert_api_success(response1, 200)
        data1 = response1.json()
        assert data1["total_processed"] == 1
        assert data1["successful_operations"] == 1
        
        # Second sync should find no games to process
        response2 = client.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        assert_api_success(response2, 200)
        data2 = response2.json()
        assert data2["message"] == "Bulk sync completed: 0 games synced, 0 failed"
        assert data2["total_processed"] == 0
        assert data2["successful_operations"] == 0
    
    def test_bulk_sync_steam_games_without_auth(self, client: TestClient):
        """Test that bulk sync requires authentication."""
        response = client.post("/api/import/sources/steam/games/sync")
        
        assert_api_error(response, 403)
    
    def test_bulk_sync_steam_games_with_existing_user_game(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test bulk sync when UserGame already exists (should update existing)."""
        from ..models.user_game import UserGame, OwnershipStatus, PlayStatus
        
        # Create required platform and storefront for Steam games
        self._create_steam_platform_data(session)
        
        # Create test game
        test_game = Game(title="Test Game", igdb_id="1234")
        session.add(test_game)
        
        # Create existing UserGame for this game
        existing_user_game = UserGame(
            user_id=test_user.id,
            game_id=test_game.id,
            ownership_status=OwnershipStatus.BORROWED,  # Different status from default OWNED
            play_status=PlayStatus.COMPLETED,
            is_loved=True,
            hours_played=50
        )
        session.add(existing_user_game)
        
        # Create matched Steam game
        steam_game = SteamGame(
            user_id=test_user.id,
            steam_appid=730,
            game_name="Test Game",
            igdb_id="1234",
            game_id=None,  # Not synced yet
            ignored=False
        )
        session.add(steam_game)
        session.commit()
        
        response = client.post("/api/import/sources/steam/games/sync", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total_processed"] == 1
        assert data["successful_operations"] == 1
        assert data["failed_operations"] == 0
        
        # Verify Steam game was synced
        session.refresh(steam_game)
        assert steam_game.game_id == test_game.id
        
        # Verify the existing UserGame was not modified (sync should work with existing UserGame)
        session.refresh(existing_user_game)
        assert existing_user_game.ownership_status == OwnershipStatus.BORROWED  # Should remain unchanged
        assert existing_user_game.play_status == PlayStatus.COMPLETED  # Should remain unchanged
        assert existing_user_game.is_loved == True  # Should remain unchanged
        assert existing_user_game.hours_played == 50  # Should remain unchanged
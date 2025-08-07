"""
Integration tests for Steam Games API endpoints.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session
from typing import Dict

from ..models.user import User
from ..models.steam_game import SteamGame
from .integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    auth_headers_fixture as auth_headers,
    assert_api_success,
    assert_api_error
)


class TestSteamGamesListEndpoint:
    """Test GET /api/steam-games endpoint."""
    
    def test_list_steam_games_empty(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test listing Steam games when user has no games."""
        response = client.get("/api/steam-games", headers=auth_headers)
        
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
        
        response = client.get("/api/steam-games", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 2
        assert len(data["games"]) == 2
        
        # Check games are sorted alphabetically by name
        games = sorted(data["games"], key=lambda g: g["game_name"])
        assert games[0]["steam_appid"] == 730
        assert games[0]["game_name"] == "Counter-Strike: Global Offensive"
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
        
        response = client.get("/api/steam-games?status_filter=unmatched", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 1
        assert len(data["games"]) == 1
        assert data["games"][0]["steam_appid"] == 730
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
        
        response = client.get("/api/steam-games?status_filter=ignored", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 1
        assert len(data["games"]) == 1
        assert data["games"][0]["steam_appid"] == 440
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
        
        response = client.get("/api/steam-games?search=Counter-Strike", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 1
        assert len(data["games"]) == 1
        assert data["games"][0]["steam_appid"] == 730
        assert "Counter-Strike" in data["games"][0]["game_name"]
    
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
        response = client.get("/api/steam-games?offset=0&limit=2", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 5
        assert len(data["games"]) == 2
    
    def test_list_steam_games_without_auth(self, client: TestClient):
        """Test that listing Steam games requires authentication."""
        response = client.get("/api/steam-games")
        
        assert_api_error(response, 403)
    
    def test_list_steam_games_invalid_status_filter(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test invalid status filter returns 422."""
        response = client.get("/api/steam-games?status_filter=invalid", headers=auth_headers)
        
        # Should return validation error for invalid status filter
        assert response.status_code == 422


class TestSteamGamesImportEndpoint:
    """Test POST /api/steam-games/import endpoint."""
    
    def test_import_steam_games_without_auth(self, client: TestClient):
        """Test that importing Steam games requires authentication."""
        response = client.post("/api/steam-games/import")
        
        assert_api_error(response, 403)
    
    def test_import_steam_games_without_steam_config(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test importing Steam games without Steam configuration."""
        response = client.post("/api/steam-games/import", headers=auth_headers)
        
        assert_api_error(response, 400)
        assert "Steam Web API key not configured" in response.json()["error"]
    
    def test_import_steam_games_with_partial_config(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test importing Steam games with incomplete Steam configuration."""
        # Set only API key but no Steam ID
        test_user.preferences_json = '{"steam": {"web_api_key": "test_api_key_12345678901234567890"}}'
        session.add(test_user)
        session.commit()
        
        response = client.post("/api/steam-games/import", headers=auth_headers)
        
        assert_api_error(response, 400)
        assert "Steam ID not configured" in response.json()["error"]
    
    def test_import_steam_games_with_unverified_config(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test importing Steam games with unverified Steam configuration."""
        # Set API key and Steam ID but not verified
        test_user.preferences_json = '{"steam": {"web_api_key": "test_api_key_12345678901234567890", "steam_id": "76561198000000000", "is_verified": false}}'
        session.add(test_user)
        session.commit()
        
        response = client.post("/api/steam-games/import", headers=auth_headers)
        
        assert_api_error(response, 400)
        assert "Steam configuration not verified" in response.json()["error"]
    
    def test_import_steam_games_success(self, client: TestClient, session: Session, test_user: User, auth_headers: Dict[str, str]):
        """Test successful Steam games import start."""
        # Set complete verified Steam configuration
        test_user.preferences_json = '{"steam": {"web_api_key": "test_api_key_12345678901234567890", "steam_id": "76561198000000000", "is_verified": true, "configured_at": "2024-01-01T00:00:00"}}'
        session.add(test_user)
        session.commit()
        
        response = client.post("/api/steam-games/import", headers=auth_headers)
        
        # Should return 202 Accepted since it's a background task
        assert_api_success(response, 202)
        data = response.json()
        assert data["started"] is True
        assert "import started successfully" in data["message"].lower()
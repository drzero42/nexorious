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
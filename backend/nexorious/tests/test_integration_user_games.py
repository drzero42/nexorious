"""
Integration tests for user games endpoints.
Tests all user games API endpoints with proper request/response validation.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from typing import Dict, Any

from ..models.user_game import UserGame, UserGamePlatform
from ..models.user import User
from ..models.game import Game
from ..models.platform import Platform, Storefront
from .integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    admin_user_fixture as admin_user,
    auth_headers_fixture as auth_headers,
    admin_headers_fixture as admin_headers,
    test_game_fixture as test_game,
    test_platform_fixture as test_platform,
    test_storefront_fixture as test_storefront,
    test_user_game_fixture as test_user_game,
    create_test_user_game_data,
    assert_api_error,
    assert_api_success,
    register_and_login_user
)


class TestUserGamesListEndpoint:
    """Test GET /api/user-games/ endpoint."""
    
    def test_list_user_games_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful user games list retrieval."""
        response = client.get("/api/user-games/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "user_games" in data
        assert "total" in data
        assert "page" in data
        assert "per_page" in data
        assert len(data["user_games"]) == 1
        assert data["user_games"][0]["id"] == str(test_user_game.id)
        assert data["user_games"][0]["ownership_status"] == test_user_game.ownership_status
        assert data["user_games"][0]["play_status"] == test_user_game.play_status
    
    def test_list_user_games_without_auth(self, client: TestClient):
        """Test user games list without authentication."""
        response = client.get("/api/user-games/")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_list_user_games_pagination(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test user games list with pagination."""
        # Create multiple user games
        for i in range(5):
            game = Game(
                title=f"Game {i}",
                description=f"Description {i}",
                is_verified=True
            )
            session.add(game)
            session.commit()
            session.refresh(game)
            
            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id,
                ownership_status="owned",
                play_status="not_started"
            )
            session.add(user_game)
        session.commit()
        
        response = client.get("/api/user-games/?page=1&per_page=2", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["user_games"]) == 2
        assert data["total"] == 5
        assert data["page"] == 1
        assert data["per_page"] == 2
    
    def test_list_user_games_filter_by_status(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test user games list with status filter."""
        response = client.get(f"/api/user-games/?play_status={test_user_game.play_status.value}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["user_games"]) == 1
        assert data["user_games"][0]["play_status"] == test_user_game.play_status.value
    
    def test_list_user_games_search(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test user games list with search."""
        response = client.get("/api/user-games/?search=Test", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["user_games"]) == 1
    
    def test_list_user_games_isolation(self, client: TestClient, session: Session):
        """Test that users only see their own games."""
        # Create two users
        user1_data = {"username": "user1", "password": "password123"}
        user2_data = {"username": "user2", "password": "password123"}
        
        user1_headers = register_and_login_user(client, user1_data)
        user2_headers = register_and_login_user(client, user2_data)
        
        # Create a game for user1
        game_data = {"title": "User1 Game", "description": "Game for user1", "genre": "Action", "developer": "Dev", "publisher": "Pub", "release_date": "2023-01-01"}
        game_response = client.post("/api/games/", json=game_data, headers=user1_headers)
        game_id = game_response.json()["id"]
        
        user_game_data = create_test_user_game_data(game_id)
        client.post("/api/user-games/", json=user_game_data, headers=user1_headers)
        
        # User1 should see their game
        response1 = client.get("/api/user-games/", headers=user1_headers)
        assert_api_success(response1, 200)
        assert len(response1.json()["user_games"]) == 1
        
        # User2 should not see user1's game
        response2 = client.get("/api/user-games/", headers=user2_headers)
        assert_api_success(response2, 200)
        assert len(response2.json()["user_games"]) == 0
    
    def test_list_user_games_sorting(self, client: TestClient, session: Session):
        """Test user games list with different sorting options."""
        # Create a test user
        user_data = {"username": "testuser", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)
        
        # Create multiple games with different metadata
        games_data = [
            {"title": "Zelda", "genre": "Adventure", "release_date": "2023-05-12", "developer": "Nintendo"},
            {"title": "Elden Ring", "genre": "RPG", "release_date": "2022-02-25", "developer": "FromSoftware"},
            {"title": "Apex Legends", "genre": "Shooter", "release_date": "2019-02-04", "developer": "Respawn"}
        ]
        
        user_games_data = []
        for i, game_data in enumerate(games_data):
            # Create game
            game_response = client.post("/api/games/", json=game_data, headers=auth_headers)
            game_id = game_response.json()["id"]
            
            # Create user game with different ratings and hours
            user_game_data = {
                "game_id": game_id,
                "ownership_status": "owned",
                "play_status": "completed" if i % 2 == 0 else "in_progress",
                "personal_rating": [5.0, 3.0, 4.0][i],
                "hours_played": [100, 50, 75][i]
            }
            user_games_data.append(user_game_data)
            
            response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
            assert_api_success(response, 201)
        
        # Test sorting by title (ascending)
        response = client.get("/api/user-games/?sort_by=title&sort_order=asc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        titles = [game["game"]["title"] for game in games]
        assert titles == ["Apex Legends", "Elden Ring", "Zelda"]
        
        # Test sorting by title (descending)
        response = client.get("/api/user-games/?sort_by=title&sort_order=desc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        titles = [game["game"]["title"] for game in games]
        assert titles == ["Zelda", "Elden Ring", "Apex Legends"]
        
        # Test sorting by genre
        response = client.get("/api/user-games/?sort_by=genre&sort_order=asc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        genres = [game["game"]["genre"] for game in games]
        assert genres == ["Adventure", "RPG", "Shooter"]
        
        # Test sorting by release_date
        response = client.get("/api/user-games/?sort_by=release_date&sort_order=asc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        release_dates = [game["game"]["release_date"] for game in games]
        assert release_dates == ["2019-02-04", "2022-02-25", "2023-05-12"]
        
        # Test sorting by personal rating
        response = client.get("/api/user-games/?sort_by=personal_rating&sort_order=desc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        ratings = [game["personal_rating"] for game in games]
        assert ratings == [5.0, 4.0, 3.0]
        
        # Test sorting by hours played
        response = client.get("/api/user-games/?sort_by=hours_played&sort_order=desc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        hours = [game["hours_played"] for game in games]
        assert hours == [100, 75, 50]


class TestUserGamesDetailEndpoint:
    """Test GET /api/user-games/{user_game_id} endpoint."""
    
    def test_get_user_game_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful user game retrieval."""
        response = client.get(f"/api/user-games/{test_user_game.id}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["id"] == str(test_user_game.id)
        assert data["ownership_status"] == test_user_game.ownership_status
        assert data["personal_rating"] == test_user_game.personal_rating
        assert data["is_loved"] == test_user_game.is_loved
        assert data["play_status"] == test_user_game.play_status
        assert data["hours_played"] == test_user_game.hours_played
        assert data["personal_notes"] == test_user_game.personal_notes
    
    def test_get_user_game_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test user game retrieval with non-existent ID."""
        response = client.get("/api/user-games/non-existent-id", headers=auth_headers)
        
        assert_api_error(response, 404, "User game not found")
    
    def test_get_user_game_without_auth(self, client: TestClient, test_user_game: UserGame):
        """Test user game retrieval without authentication."""
        response = client.get(f"/api/user-games/{test_user_game.id}")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_get_user_game_wrong_user(self, client: TestClient, test_user_game: UserGame, session: Session):
        """Test user game retrieval by different user."""
        # Create another user
        other_user_data = {"username": "other", "password": "password123"}
        other_headers = register_and_login_user(client, other_user_data)
        
        response = client.get(f"/api/user-games/{test_user_game.id}", headers=other_headers)
        
        assert_api_error(response, 404, "User game not found")


class TestUserGamesCreateEndpoint:
    """Test POST /api/user-games/ endpoint."""
    
    def test_create_user_game_success(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test successful user game creation."""
        user_game_data = create_test_user_game_data(str(test_game.id))
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["game"]["id"] == str(test_game.id)
        assert data["ownership_status"] == user_game_data["ownership_status"]
        assert data["is_loved"] == user_game_data["is_loved"]
        assert data["play_status"] == user_game_data["play_status"]
        assert data["hours_played"] == user_game_data["hours_played"]
        assert data["personal_notes"] == user_game_data["personal_notes"] or data["personal_notes"] is None
    
    def test_create_user_game_with_rating(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test user game creation with personal rating."""
        user_game_data = create_test_user_game_data(str(test_game.id), personal_rating=4.5)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["personal_rating"] == 4.5
    
    def test_create_user_game_duplicate(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test creation of duplicate user game."""
        user_game_data = create_test_user_game_data(str(test_user_game.game_id))
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_error(response, 409, "already exists")
    
    def test_create_user_game_invalid_game(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test user game creation with invalid game ID."""
        user_game_data = create_test_user_game_data("non-existent-game-id")
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_error(response, 404, "Game not found")
    
    def test_create_user_game_without_auth(self, client: TestClient, test_game: Game):
        """Test user game creation without authentication."""
        user_game_data = create_test_user_game_data(str(test_game.id))
        response = client.post("/api/user-games/", json=user_game_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_create_user_game_invalid_status(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test user game creation with invalid play status."""
        user_game_data = create_test_user_game_data(str(test_game.id), play_status="invalid_status")
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_error(response, 422)
    
    def test_create_user_game_invalid_rating(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test user game creation with invalid rating."""
        user_game_data = create_test_user_game_data(str(test_game.id), personal_rating=6.0)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_error(response, 422)


class TestUserGamesUpdateEndpoint:
    """Test PUT /api/user-games/{user_game_id} endpoint."""
    
    def test_update_user_game_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful user game update."""
        update_data = {
            "ownership_status": "borrowed",
            "personal_rating": 3.5,
            "is_loved": False,
            "play_status": "in_progress",
            "hours_played": 10,
            "personal_notes": "Updated notes"
        }
        response = client.put(f"/api/user-games/{test_user_game.id}", json=update_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["ownership_status"] == "borrowed"
        assert data["personal_rating"] == 3.5
        assert data["is_loved"] is False
        assert data["play_status"] == "in_progress"
        assert data["hours_played"] == 10
        assert data["personal_notes"] == "Updated notes"
    
    def test_update_user_game_partial(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test partial user game update."""
        update_data = {"play_status": "completed", "hours_played": 15}
        response = client.put(f"/api/user-games/{test_user_game.id}", json=update_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["play_status"] == "completed"
        assert data["hours_played"] == 15
        assert data["ownership_status"] == test_user_game.ownership_status  # Should remain unchanged
    
    def test_update_user_game_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test user game update with non-existent ID."""
        update_data = {"play_status": "completed"}
        response = client.put("/api/user-games/non-existent-id", json=update_data, headers=auth_headers)
        
        assert_api_error(response, 404, "User game not found")
    
    def test_update_user_game_without_auth(self, client: TestClient, test_user_game: UserGame):
        """Test user game update without authentication."""
        update_data = {"play_status": "completed"}
        response = client.put(f"/api/user-games/{test_user_game.id}", json=update_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_update_user_game_wrong_user(self, client: TestClient, test_user_game: UserGame):
        """Test user game update by different user."""
        # Create another user
        other_user_data = {"username": "other", "password": "password123"}
        other_headers = register_and_login_user(client, other_user_data)
        
        update_data = {"play_status": "completed"}
        response = client.put(f"/api/user-games/{test_user_game.id}", json=update_data, headers=other_headers)
        
        assert_api_error(response, 404, "User game not found")


class TestUserGamesProgressEndpoint:
    """Test PUT /api/user-games/{user_game_id}/progress endpoint."""
    
    def test_update_progress_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful progress update."""
        progress_data = {
            "play_status": "completed",
            "hours_played": 20,
            "personal_notes": "Finished the game!"
        }
        response = client.put(f"/api/user-games/{test_user_game.id}/progress", json=progress_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["play_status"] == "completed"
        assert data["hours_played"] == 20
        assert data["personal_notes"] == "Finished the game!"
    
    def test_update_progress_partial(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test partial progress update."""
        progress_data = {"play_status": "in_progress"}
        response = client.put(f"/api/user-games/{test_user_game.id}/progress", json=progress_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["play_status"] == "in_progress"
        assert data["hours_played"] == test_user_game.hours_played  # Should remain unchanged
    
    def test_update_progress_invalid_status(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test progress update with invalid status."""
        progress_data = {"play_status": "invalid_status"}
        response = client.put(f"/api/user-games/{test_user_game.id}/progress", json=progress_data, headers=auth_headers)
        
        assert_api_error(response, 422)
    
    def test_update_progress_without_auth(self, client: TestClient, test_user_game: UserGame):
        """Test progress update without authentication."""
        progress_data = {"play_status": "completed"}
        response = client.put(f"/api/user-games/{test_user_game.id}/progress", json=progress_data)
        
        assert_api_error(response, 403, "Not authenticated")


class TestUserGamesDeleteEndpoint:
    """Test DELETE /api/user-games/{user_game_id} endpoint."""
    
    def test_delete_user_game_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful user game deletion."""
        response = client.delete(f"/api/user-games/{test_user_game.id}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "User game deleted successfully"
    
    def test_delete_user_game_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test user game deletion with non-existent ID."""
        response = client.delete("/api/user-games/non-existent-id", headers=auth_headers)
        
        assert_api_error(response, 404, "User game not found")
    
    def test_delete_user_game_without_auth(self, client: TestClient, test_user_game: UserGame):
        """Test user game deletion without authentication."""
        response = client.delete(f"/api/user-games/{test_user_game.id}")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_delete_user_game_wrong_user(self, client: TestClient, test_user_game: UserGame):
        """Test user game deletion by different user."""
        # Create another user
        other_user_data = {"username": "other", "password": "password123"}
        other_headers = register_and_login_user(client, other_user_data)
        
        response = client.delete(f"/api/user-games/{test_user_game.id}", headers=other_headers)
        
        assert_api_error(response, 404, "User game not found")


class TestUserGamePlatformsEndpoints:
    """Test user game platform association endpoints."""
    
    def test_get_user_game_platforms(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str], session: Session):
        """Test getting user game platforms."""
        # Add platform association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform_id=test_platform.id,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        
        response = client.get(f"/api/user-games/{test_user_game.id}/platforms", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data) == 1
        assert data[0]["platform_id"] == str(test_platform.id)
        assert data[0]["is_available"] is True
    
    def test_create_user_game_platform(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test creating a user game platform association."""
        platform_data = {
            "platform_id": str(test_platform.id),
            "storefront_id": str(test_storefront.id),
            "store_game_id": "steam_12345",
            "store_url": "https://store.example.com/game/12345",
            "is_available": True
        }
        response = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["platform_id"] == str(test_platform.id)
        assert data["storefront_id"] == str(test_storefront.id)
        assert data["store_game_id"] == "steam_12345"
        assert data["store_url"] == "https://store.example.com/game/12345"
        assert data["is_available"] is True
    
    def test_create_user_game_platform_without_storefront(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str]):
        """Test creating a user game platform association without storefront."""
        platform_data = {
            "platform_id": str(test_platform.id),
            "is_available": True
        }
        response = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["platform_id"] == str(test_platform.id)
        assert data["storefront_id"] is None
    
    def test_create_user_game_platform_duplicate(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str], session: Session):
        """Test creating duplicate user game platform association."""
        # Create existing association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform_id=test_platform.id,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        
        platform_data = {
            "platform_id": str(test_platform.id),
            "is_available": True
        }
        response = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data, headers=auth_headers)
        
        assert_api_error(response, 409, "already exists")
    
    def test_delete_user_game_platform(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str], session: Session):
        """Test deleting a user game platform association."""
        # Create association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform_id=test_platform.id,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        session.refresh(platform_association)
        
        response = client.delete(f"/api/user-games/{test_user_game.id}/platforms/{platform_association.id}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Platform association deleted successfully"


class TestUserGamesBulkUpdateEndpoint:
    """Test PUT /api/user-games/bulk-update endpoint."""
    
    def test_bulk_update_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test successful bulk update."""
        # Create another user game
        game2 = Game(
            title="Game 2",
            description="Second game",
            is_verified=True
        )
        session.add(game2)
        session.commit()
        session.refresh(game2)
        
        user_game2 = UserGame(
            user_id=test_user_game.user_id,
            game_id=game2.id,
            ownership_status="owned",
            play_status="not_started"
        )
        session.add(user_game2)
        session.commit()
        session.refresh(user_game2)
        
        bulk_data = {
            "user_game_ids": [str(test_user_game.id), str(user_game2.id)],
            "updates": {
                "play_status": "completed",
                "is_loved": True
            }
        }
        response = client.put("/api/user-games/bulk-update", json=bulk_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Bulk update completed successfully"
        assert data["updated_count"] == 2
    
    def test_bulk_update_partial_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test bulk update with some failures."""
        bulk_data = {
            "user_game_ids": [str(test_user_game.id), "non-existent-id"],
            "updates": {
                "play_status": "completed"
            }
        }
        response = client.put("/api/user-games/bulk-update", json=bulk_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["updated_count"] == 1
        assert data["failed_count"] == 1
    
    def test_bulk_update_without_auth(self, client: TestClient, test_user_game: UserGame):
        """Test bulk update without authentication."""
        bulk_data = {
            "user_game_ids": [str(test_user_game.id)],
            "updates": {"play_status": "completed"}
        }
        response = client.put("/api/user-games/bulk-update", json=bulk_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_bulk_update_empty_ids(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test bulk update with empty user game IDs."""
        bulk_data = {
            "user_game_ids": [],
            "updates": {"play_status": "completed"}
        }
        response = client.put("/api/user-games/bulk-update", json=bulk_data, headers=auth_headers)
        
        assert_api_error(response, 422)


class TestUserGamesStatsEndpoint:
    """Test GET /api/user-games/stats endpoint."""
    
    def test_get_collection_stats(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test getting collection statistics."""
        # Create additional user games for better stats
        user_id = test_user_game.user_id
        
        # Create games with different statuses
        statuses = ["not_started", "in_progress", "completed", "mastered"]
        for i, status in enumerate(statuses, 1):
            game = Game(
                title=f"Game {i}",
                description=f"Description {i}",
                genre="Action",
                is_verified=True
            )
            session.add(game)
            session.commit()
            session.refresh(game)
            
            user_game = UserGame(
                user_id=user_id,
                game_id=game.id,
                ownership_status="owned",
                play_status=status,
                hours_played=i * 10
            )
            session.add(user_game)
        session.commit()
        
        response = client.get("/api/user-games/stats", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "total_games" in data
        assert "completion_stats" in data
        assert "ownership_stats" in data
        assert "platform_stats" in data
        assert "genre_stats" in data
        assert "total_hours_played" in data
        assert data["total_games"] == 5  # Original + 4 new games
    
    def test_get_collection_stats_empty(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test getting collection statistics with no games."""
        response = client.get("/api/user-games/stats", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total_games"] == 0
        assert data["total_hours_played"] == 0
    
    def test_get_collection_stats_without_auth(self, client: TestClient):
        """Test getting collection statistics without authentication."""
        response = client.get("/api/user-games/stats")
        
        assert_api_error(response, 403, "Not authenticated")


class TestUserGamesEndpointsSecurity:
    """Test security aspects of user games endpoints."""
    
    def test_user_isolation_in_endpoints(self, client: TestClient, test_user_game: UserGame, session: Session):
        """Test that users can only access their own data."""
        # Create another user
        other_user_data = {"username": "other", "password": "password123"}
        other_headers = register_and_login_user(client, other_user_data)
        
        # Test list endpoint
        response = client.get("/api/user-games/", headers=other_headers)
        assert_api_success(response, 200)
        assert len(response.json()["user_games"]) == 0
        
        # Test detail endpoint
        response = client.get(f"/api/user-games/{test_user_game.id}", headers=other_headers)
        assert_api_error(response, 404, "User game not found")
        
        # Test update endpoint
        response = client.put(f"/api/user-games/{test_user_game.id}", json={"play_status": "completed"}, headers=other_headers)
        assert_api_error(response, 404, "User game not found")
        
        # Test delete endpoint
        response = client.delete(f"/api/user-games/{test_user_game.id}", headers=other_headers)
        assert_api_error(response, 404, "User game not found")
    
    def test_authenticated_endpoints_require_auth(self, client: TestClient, test_user_game: UserGame, test_game: Game):
        """Test that authenticated endpoints require authentication."""
        # Test list endpoint
        response = client.get("/api/user-games/")
        assert_api_error(response, 403, "Not authenticated")
        
        # Test create endpoint
        user_game_data = create_test_user_game_data(str(test_game.id))
        response = client.post("/api/user-games/", json=user_game_data)
        assert_api_error(response, 403, "Not authenticated")
        
        # Test detail endpoint
        response = client.get(f"/api/user-games/{test_user_game.id}")
        assert_api_error(response, 403, "Not authenticated")
        
        # Test update endpoint
        response = client.put(f"/api/user-games/{test_user_game.id}", json={"play_status": "completed"})
        assert_api_error(response, 403, "Not authenticated")
        
        # Test delete endpoint
        response = client.delete(f"/api/user-games/{test_user_game.id}")
        assert_api_error(response, 403, "Not authenticated")
        
        # Test stats endpoint
        response = client.get("/api/user-games/stats")
        assert_api_error(response, 403, "Not authenticated")


class TestUserGamesDataValidation:
    """Test data validation for user games endpoints."""
    
    def test_play_status_validation(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test play status validation."""
        # Test invalid play status
        user_game_data = create_test_user_game_data(str(test_game.id), play_status="invalid_status")
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)
    
    def test_ownership_status_validation(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test ownership status validation."""
        # Test invalid ownership status
        user_game_data = create_test_user_game_data(str(test_game.id), ownership_status="invalid_status")
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)
    
    def test_rating_validation(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test rating validation."""
        # Test rating too low
        user_game_data = create_test_user_game_data(str(test_game.id), personal_rating=0.5)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)
        
        # Test rating too high
        user_game_data = create_test_user_game_data(str(test_game.id), personal_rating=5.5)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)
    
    def test_hours_played_validation(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test hours played validation."""
        # Test negative hours
        user_game_data = create_test_user_game_data(str(test_game.id), hours_played=-1)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)
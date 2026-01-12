"""
Integration tests for user games endpoints.
Tests all user games API endpoints with proper request/response validation.
"""

import json
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from typing import Dict

from ..models.user_game import UserGame, UserGamePlatform
from ..models.user import User
from ..models.game import Game
from ..models.platform import Platform, Storefront
from .integration_test_utils import (
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
        # ownership_status is now on platform level, not game level
        assert data["user_games"][0]["play_status"] == test_user_game.play_status.value
    
    def test_list_user_games_without_auth(self, client: TestClient):
        """Test user games list without authentication."""
        response = client.get("/api/user-games/")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_list_user_games_pagination(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test user games list with pagination."""
        from .integration_test_utils import create_test_games
        
        # Create multiple games with proper IGDB IDs
        games = create_test_games(count=5, session=session)
        
        # Create user games for each game
        for game in games:
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
    
    def test_list_user_games_isolation(self, client_with_mock_igdb: TestClient, session: Session):
        """Test that users only see their own games."""
        # Create two users
        user1_data = {"username": "user1", "password": "password123"}
        user2_data = {"username": "user2", "password": "password123"}
        
        user1_headers = register_and_login_user(client_with_mock_igdb, user1_data)
        user2_headers = register_and_login_user(client_with_mock_igdb, user2_data)
        
        # Create a game for user1 using IGDB import
        import_data = {
            "igdb_id": 12345,
            "title": "User1 Game"
        }
        game_response = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data, headers=user1_headers)
        game_id = game_response.json()["id"]
        
        user_game_data = create_test_user_game_data(game_id)
        client_with_mock_igdb.post("/api/user-games/", json=user_game_data, headers=user1_headers)
        
        # User1 should see their game
        response1 = client_with_mock_igdb.get("/api/user-games/", headers=user1_headers)
        assert_api_success(response1, 200)
        assert len(response1.json()["user_games"]) == 1
        
        # User2 should not see user1's game
        response2 = client_with_mock_igdb.get("/api/user-games/", headers=user2_headers)
        assert_api_success(response2, 200)
        assert len(response2.json()["user_games"]) == 0
    
    def test_list_user_games_sorting(self, client_with_mock_igdb: TestClient, session: Session):
        """Test user games list with different sorting options."""
        # Create a test user
        user_data = {"username": "testuser", "password": "password123"}
        auth_headers = register_and_login_user(client_with_mock_igdb, user_data)
        
        # Create multiple games using IGDB import
        import_games_data = [
            {"igdb_id": 100, "title": "Zelda"},
            {"igdb_id": 200, "title": "Elden Ring"},
            {"igdb_id": 300, "title": "Apex Legends"}
        ]
        
        user_games_data = []
        for i, import_data in enumerate(import_games_data):
            # Create game using IGDB import
            game_response = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data, headers=auth_headers)
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
            
            response = client_with_mock_igdb.post("/api/user-games/", json=user_game_data, headers=auth_headers)
            assert_api_success(response, 201)
        
        # Test sorting by title (ascending)
        response = client_with_mock_igdb.get("/api/user-games/?sort_by=title&sort_order=asc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        titles = [game["game"]["title"] for game in games]
        assert titles == ["Apex Legends", "Elden Ring", "Zelda"]
        
        # Test sorting by title (descending)
        response = client_with_mock_igdb.get("/api/user-games/?sort_by=title&sort_order=desc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        titles = [game["game"]["title"] for game in games]
        assert titles == ["Zelda", "Elden Ring", "Apex Legends"]
        
        # Test sorting by genre
        response = client_with_mock_igdb.get("/api/user-games/?sort_by=genre&sort_order=asc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        genres = [game["game"]["genre"] for game in games]
        assert genres == ["Adventure", "RPG", "Shooter"]
        
        # Test sorting by release_date
        response = client_with_mock_igdb.get("/api/user-games/?sort_by=release_date&sort_order=asc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        release_dates = [game["game"]["release_date"] for game in games]
        assert release_dates == ["2023-01-01", "2023-01-01", "2023-01-01"]
        
        # Test sorting by personal rating
        response = client_with_mock_igdb.get("/api/user-games/?sort_by=personal_rating&sort_order=desc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        ratings = [game["personal_rating"] for game in games]
        assert ratings == [5.0, 4.0, 3.0]
        
        # Test sorting by hours played
        response = client_with_mock_igdb.get("/api/user-games/?sort_by=hours_played&sort_order=desc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        hours = [game["hours_played"] for game in games]
        assert hours == [100, 75, 50]

    def test_list_user_games_sorting_nulls_last(self, client: TestClient, session: Session):
        """Test that NULL values sort to the end regardless of sort direction."""
        from .integration_test_utils import create_test_game, register_and_login_user

        # Create a test user
        user_data = {"username": "nullsortuser", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user from the database
        from ..models.user import User
        user = session.exec(select(User).where(User.username == "nullsortuser")).first()
        assert user is not None

        # Create games with varying howlongtobeat_main values (including NULL)
        game_short = create_test_game(title="Short Game", howlongtobeat_main=10)
        game_long = create_test_game(title="Long Game", howlongtobeat_main=100)
        game_null = create_test_game(title="Unknown Game", howlongtobeat_main=None)

        session.add_all([game_short, game_long, game_null])
        session.commit()
        session.refresh(game_short)
        session.refresh(game_long)
        session.refresh(game_null)

        # Create user games for each
        for game in [game_short, game_long, game_null]:
            user_game = UserGame(
                user_id=user.id,
                game_id=game.id,
                ownership_status="owned",
                play_status="not_started"
            )
            session.add(user_game)
        session.commit()

        # Test ascending sort: should be 10h, 100h, NULL
        response = client.get("/api/user-games/?sort_by=howlongtobeat_main&sort_order=asc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        ttb_values = [game["game"]["howlongtobeat_main"] for game in games]
        assert ttb_values == [10, 100, None], f"ASC sort failed: expected [10, 100, None], got {ttb_values}"

        # Test descending sort: should be 100h, 10h, NULL (NULLs at end, not beginning)
        response = client.get("/api/user-games/?sort_by=howlongtobeat_main&sort_order=desc", headers=auth_headers)
        assert_api_success(response, 200)
        games = response.json()["user_games"]
        ttb_values = [game["game"]["howlongtobeat_main"] for game in games]
        assert ttb_values == [100, 10, None], f"DESC sort failed: expected [100, 10, None], got {ttb_values}"

    def test_list_user_games_sorting_hours_played_aggregated(self, client: TestClient, session: Session):
        """Test that hours_played sorting uses aggregated platform hours with legacy fallback."""
        from .integration_test_utils import create_test_game, register_and_login_user
        from ..models.user import User
        from ..models.user_game import UserGame, UserGamePlatform
        from ..models.platform import Platform

        # Create a test user
        user_data = {"username": "hoursortuser", "password": "password123"}
        auth_headers = register_and_login_user(client, user_data)

        # Get the user from the database
        user = session.exec(select(User).where(User.username == "hoursortuser")).first()
        assert user is not None

        # Create platforms needed for UserGamePlatform FK constraints
        platform_pc = Platform(name="test-pc-hours", display_name="PC", is_active=True)
        platform_ps5 = Platform(name="test-ps5-hours", display_name="PlayStation 5", is_active=True)
        session.add_all([platform_pc, platform_ps5])
        session.commit()

        # Create three games with different hour configurations:
        # Game 1: 100 hours via platforms (50 + 50)
        # Game 2: 75 hours via legacy field (no platforms)
        # Game 3: 150 hours via platforms (150)

        game1 = create_test_game(igdb_id=9001, title="Platform Hours Game")
        game2 = create_test_game(igdb_id=9002, title="Legacy Hours Game")
        game3 = create_test_game(igdb_id=9003, title="Single Platform Game")

        session.add_all([game1, game2, game3])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)

        # Game 1: 100 hours from platforms, legacy=0
        ug1 = UserGame(user_id=user.id, game_id=game1.id, hours_played=0)
        session.add(ug1)
        session.commit()
        session.refresh(ug1)

        ugp1a = UserGamePlatform(user_game_id=ug1.id, platform=platform_pc.name, hours_played=50)
        ugp1b = UserGamePlatform(user_game_id=ug1.id, platform=platform_ps5.name, hours_played=50)
        session.add_all([ugp1a, ugp1b])

        # Game 2: 75 hours from legacy field, no platforms
        ug2 = UserGame(user_id=user.id, game_id=game2.id, hours_played=75)
        session.add(ug2)

        # Game 3: 150 hours from single platform
        ug3 = UserGame(user_id=user.id, game_id=game3.id, hours_played=0)
        session.add(ug3)
        session.commit()
        session.refresh(ug3)

        ugp3 = UserGamePlatform(user_game_id=ug3.id, platform=platform_pc.name, hours_played=150)
        session.add(ugp3)
        session.commit()

        # Test descending sort: should be 150, 100, 75
        response = client.get("/api/user-games/?sort_by=hours_played&sort_order=desc", headers=auth_headers)
        assert response.status_code == 200
        games = response.json()["user_games"]
        hours = [game["hours_played"] for game in games]
        titles = [game["game"]["title"] for game in games]
        assert hours == [150, 100, 75], f"DESC sort failed: expected [150, 100, 75], got {hours} for titles {titles}"

        # Test ascending sort: should be 75, 100, 150
        response = client.get("/api/user-games/?sort_by=hours_played&sort_order=asc", headers=auth_headers)
        assert response.status_code == 200
        games = response.json()["user_games"]
        hours = [game["hours_played"] for game in games]
        assert hours == [75, 100, 150], f"ASC sort failed: expected [75, 100, 150], got {hours}"


class TestUserGamesDetailEndpoint:
    """Test GET /api/user-games/{user_game_id} endpoint."""
    
    def test_get_user_game_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful user game retrieval."""
        response = client.get(f"/api/user-games/{test_user_game.id}", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["id"] == str(test_user_game.id)
        # ownership_status is now on platform level, not game level
        assert data["personal_rating"] == float(test_user_game.personal_rating)
        assert data["is_loved"] == test_user_game.is_loved
        assert data["play_status"] == test_user_game.play_status.value
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
        user_game_data = create_test_user_game_data(test_game.id)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)

        assert_api_success(response, 201)
        data = response.json()
        assert data["game"]["id"] == test_game.id
        # ownership_status is now on platform level, not game level
        assert data["is_loved"] == user_game_data["is_loved"]
        assert data["play_status"] == user_game_data["play_status"]
        assert data["hours_played"] == user_game_data["hours_played"]
        assert data["personal_notes"] == user_game_data["personal_notes"] or data["personal_notes"] is None
    
    def test_create_user_game_with_rating(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test user game creation with personal rating."""
        user_game_data = create_test_user_game_data(test_game.id, personal_rating=4.5)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["personal_rating"] == 4.5
    
    def test_create_user_game_duplicate(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test creation of duplicate user game."""
        user_game_data = create_test_user_game_data(test_user_game.game_id)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_error(response, 409, "already exists")
    
    def test_create_user_game_invalid_game(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test user game creation with invalid game ID."""
        user_game_data = create_test_user_game_data(99999)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_error(response, 404, "Game not found")
    
    def test_create_user_game_without_auth(self, client: TestClient, test_game: Game):
        """Test user game creation without authentication."""
        user_game_data = create_test_user_game_data(test_game.id)
        response = client.post("/api/user-games/", json=user_game_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_create_user_game_invalid_status(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test user game creation with invalid play status."""
        user_game_data = create_test_user_game_data(test_game.id, play_status="invalid_status")
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_error(response, 422)
    
    def test_create_user_game_invalid_rating(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test user game creation with invalid rating."""
        user_game_data = create_test_user_game_data(test_game.id, personal_rating=6.0)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        
        assert_api_error(response, 422)


class TestUserGamesUpdateEndpoint:
    """Test PUT /api/user-games/{user_game_id} endpoint."""
    
    def test_update_user_game_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful user game update."""
        update_data = {
            # ownership_status is now on platform level, not game level
            "personal_rating": 3.5,
            "is_loved": False,
            "play_status": "in_progress",
            "hours_played": 10,
            "personal_notes": "Updated notes"
        }
        response = client.put(f"/api/user-games/{test_user_game.id}", json=update_data, headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        # ownership_status is now on platform level, not game level
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
        # ownership_status is now on platform level, not game level
    
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
            platform=test_platform.name,
            is_available=True
        )
        session.add(platform_association)
        session.commit()

        response = client.get(f"/api/user-games/{test_user_game.id}/platforms", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert len(data) == 1
        assert data[0]["platform"] == test_platform.name
        assert data[0]["is_available"] is True
    
    def test_create_user_game_platform(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test creating a user game platform association."""
        platform_data = {
            "platform": test_platform.name,
            "storefront": test_storefront.name,
            "is_available": True
        }
        response = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data, headers=auth_headers)

        assert_api_success(response, 201)
        data = response.json()
        # Verify the response is a UserGameResponse with the new platform added
        assert data["id"] == str(test_user_game.id)
        assert "platforms" in data
        assert len(data["platforms"]) == 1
        platform = data["platforms"][0]
        assert platform["platform"] == test_platform.name
        assert platform["storefront"] == test_storefront.name
        assert platform["is_available"] is True
    
    def test_create_user_game_platform_without_storefront(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str]):
        """Test creating a user game platform association without storefront."""
        platform_data = {
            "platform": test_platform.name,
            "is_available": True
        }
        response = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data, headers=auth_headers)

        assert_api_success(response, 201)
        data = response.json()
        # Verify the response is a UserGameResponse with the new platform added
        assert data["id"] == str(test_user_game.id)
        assert "platforms" in data
        assert len(data["platforms"]) == 1
        platform = data["platforms"][0]
        assert platform["platform"] == test_platform.name
        assert platform["storefront"] is None
    
    def test_create_user_game_platform_duplicate_platform_storefront(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, test_storefront: Storefront, auth_headers: Dict[str, str], session: Session):
        """Test creating duplicate user game platform+storefront association."""
        # Create existing association with specific storefront
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            storefront=test_storefront.name,
            is_available=True
        )
        session.add(platform_association)
        session.commit()

        # Try to create the same platform+storefront combination
        platform_data = {
            "platform": test_platform.name,
            "storefront": test_storefront.name,
            "is_available": True
        }
        response = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data, headers=auth_headers)

        assert_api_error(response, 409, "already exists")
    
    def test_delete_user_game_platform(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str], session: Session):
        """Test deleting a user game platform association."""
        # Create association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        session.refresh(platform_association)

        response = client.delete(f"/api/user-games/{test_user_game.id}/platforms/{platform_association.id}", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Platform association deleted successfully"


class TestUpdatePlatformAssociation:
    """Test PUT /api/user-games/{user_game_id}/platforms/{platform_association_id} endpoint."""
    
    def test_update_platform_association_success(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, test_storefront: Storefront, test_storefront_2: Storefront, auth_headers: Dict[str, str], session: Session):
        """Test successful platform association update."""
        # Create initial association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            storefront=test_storefront.name,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        session.refresh(platform_association)

        # Update the association
        update_data = {
            "platform": test_platform.name,
            "storefront": test_storefront_2.name,
            "is_available": False
        }
        response = client.put(f"/api/user-games/{test_user_game.id}/platforms/{platform_association.id}", json=update_data, headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["platform"] == test_platform.name
        assert data["storefront"] == test_storefront_2.name
        assert data["is_available"] is False
    
    def test_update_platform_association_conflict(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, test_storefront: Storefront, test_storefront_2: Storefront, auth_headers: Dict[str, str], session: Session):
        """Test update platform association with conflict."""
        # Create two associations
        association1 = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            storefront=test_storefront.name,
            is_available=True
        )
        association2 = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            storefront=test_storefront_2.name,
            is_available=True
        )
        session.add(association1)
        session.add(association2)
        session.commit()
        session.refresh(association1)
        session.refresh(association2)

        # Try to update association1 to have the same platform+storefront as association2
        update_data = {
            "platform": test_platform.name,
            "storefront": test_storefront_2.name,
            "is_available": True
        }
        response = client.put(f"/api/user-games/{test_user_game.id}/platforms/{association1.id}", json=update_data, headers=auth_headers)

        assert_api_error(response, 409, "already exists")
    
    def test_update_platform_association_not_found(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str]):
        """Test update with non-existent platform association."""
        update_data = {
            "platform": test_platform.name,
            "is_available": True
        }
        response = client.put(f"/api/user-games/{test_user_game.id}/platforms/non-existent-id", json=update_data, headers=auth_headers)

        assert_api_error(response, 404, "Platform association not found")
    
    def test_update_platform_association_invalid_platform(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str], session: Session):
        """Test update with invalid platform ID."""
        # Create association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        session.refresh(platform_association)

        update_data = {
            "platform": "non-existent-platform",
            "is_available": True
        }
        response = client.put(f"/api/user-games/{test_user_game.id}/platforms/{platform_association.id}", json=update_data, headers=auth_headers)

        assert_api_error(response, 404, "Platform not found")
    
    def test_update_platform_association_invalid_storefront(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str], session: Session):
        """Test update with invalid storefront ID."""
        # Create association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        session.refresh(platform_association)

        update_data = {
            "platform": test_platform.name,
            "storefront": "non-existent-storefront",
            "is_available": True
        }
        response = client.put(f"/api/user-games/{test_user_game.id}/platforms/{platform_association.id}", json=update_data, headers=auth_headers)

        assert_api_error(response, 404, "Storefront not found")

    def test_update_platform_association_without_auth(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, session: Session):
        """Test update without authentication."""
        # Create association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        session.refresh(platform_association)

        update_data = {
            "platform": test_platform.name,
            "is_available": False
        }
        response = client.put(f"/api/user-games/{test_user_game.id}/platforms/{platform_association.id}", json=update_data)

        assert_api_error(response, 403, "Not authenticated")
    
    def test_update_platform_association_wrong_user(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, session: Session):
        """Test update by different user."""
        # Create association
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            is_available=True
        )
        session.add(platform_association)
        session.commit()
        session.refresh(platform_association)

        # Create another user
        other_user_data = {"username": "other", "password": "password123"}
        other_headers = register_and_login_user(client, other_user_data)

        update_data = {
            "platform": test_platform.name,
            "is_available": False
        }
        response = client.put(f"/api/user-games/{test_user_game.id}/platforms/{platform_association.id}", json=update_data, headers=other_headers)

        assert_api_error(response, 404, "Platform association not found")


class TestUserGamePlatformMultipleStorefronts:
    """Test multiple storefront associations per platform scenarios."""
    
    def test_multiple_storefronts_same_platform(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, test_storefront: Storefront, test_storefront_2: Storefront, auth_headers: Dict[str, str]):
        """Test adding multiple storefronts for the same platform."""
        # Add first storefront for platform
        platform_data_1 = {
            "platform": test_platform.name,
            "storefront": test_storefront.name,
            "is_available": True
        }
        response1 = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data_1, headers=auth_headers)
        assert_api_success(response1, 201)

        # Add second storefront for same platform
        platform_data_2 = {
            "platform": test_platform.name,
            "storefront": test_storefront_2.name,
            "is_available": True
        }
        response2 = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data_2, headers=auth_headers)
        assert_api_success(response2, 201)

        # Verify both associations exist
        response = client.get(f"/api/user-games/{test_user_game.id}/platforms", headers=auth_headers)
        assert_api_success(response, 200)
        platforms = response.json()
        assert len(platforms) == 2

        # Check that we have both storefronts for the same platform
        storefronts = {p["storefront"] for p in platforms}
        assert test_storefront.name in storefronts
        assert test_storefront_2.name in storefronts

        # Both should be for the same platform
        platforms = {p["platform"] for p in platforms}
        assert len(platforms) == 1
        assert test_platform.name in platforms
    
    def test_platform_with_null_and_specific_storefront(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test platform with NULL storefront and specific storefront."""
        # Add platform without storefront (NULL)
        platform_data_1 = {
            "platform": test_platform.name,
            "is_available": True
        }
        response1 = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data_1, headers=auth_headers)
        assert_api_success(response1, 201)

        # Add same platform with specific storefront
        platform_data_2 = {
            "platform": test_platform.name,
            "storefront": test_storefront.name,
            "is_available": True
        }
        response2 = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data_2, headers=auth_headers)
        assert_api_success(response2, 201)

        # Verify both associations exist
        response = client.get(f"/api/user-games/{test_user_game.id}/platforms", headers=auth_headers)
        assert_api_success(response, 200)
        platforms = response.json()
        assert len(platforms) == 2

        # One should have null storefront, one should have specific storefront
        storefronts = [p["storefront"] for p in platforms]
        assert None in storefronts
        assert test_storefront.name in storefronts
    
    def test_duplicate_null_storefront_prevented(self, client: TestClient, test_user_game: UserGame, test_platform: Platform, auth_headers: Dict[str, str], session: Session):
        """Test that duplicate NULL storefront combinations are prevented."""
        # Create existing association with NULL storefront
        platform_association = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            storefront=None,
            is_available=True
        )
        session.add(platform_association)
        session.commit()

        # Try to create another association with same platform and NULL storefront
        platform_data = {
            "platform": test_platform.name,
            "is_available": True
        }
        response = client.post(f"/api/user-games/{test_user_game.id}/platforms", json=platform_data, headers=auth_headers)

        assert_api_error(response, 409, "already exists")


class TestUserGamesBulkUpdateEndpoint:
    """Test PUT /api/user-games/bulk-update endpoint."""
    
    def test_bulk_update_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test successful bulk update."""
        from .integration_test_utils import create_test_game
        
        # Create another game with proper IGDB ID
        game2 = create_test_game(title="Game 2", description="Second game")
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
            "play_status": "completed",
            "is_loved": True
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
            "play_status": "completed"
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
            "play_status": "completed"
        }
        response = client.put("/api/user-games/bulk-update", json=bulk_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_bulk_update_empty_ids(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test bulk update with empty user game IDs."""
        bulk_data = {
            "user_game_ids": [],
            "play_status": "completed"
        }
        response = client.put("/api/user-games/bulk-update", json=bulk_data, headers=auth_headers)
        
        assert_api_error(response, 422)


class TestUserGamesBulkDeleteEndpoint:
    """Test bulk delete endpoints."""
    
    def test_bulk_delete_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test successful bulk delete."""
        from .integration_test_utils import create_test_game
        
        # Create another game with proper IGDB ID
        game2 = create_test_game(title="Game 2", description="Second game")
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
            "user_game_ids": [str(test_user_game.id), str(user_game2.id)]
        }
        response = client.request("DELETE", "/api/user-games/bulk-delete", content=json.dumps(bulk_data), headers={**auth_headers, "Content-Type": "application/json"})
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Bulk deletion completed successfully"
        assert data["deleted_count"] == 2
        assert data["failed_count"] == 0
        
        # Verify games are deleted (store IDs before they get detached)
        game1_id = test_user_game.id
        game2_id = user_game2.id
        session.expire_all()
        deleted_game1 = session.get(UserGame, game1_id)
        deleted_game2 = session.get(UserGame, game2_id)
        assert deleted_game1 is None
        assert deleted_game2 is None
    
    def test_bulk_delete_partial_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test bulk delete with some failures."""
        bulk_data = {
            "user_game_ids": [str(test_user_game.id), "non-existent-id"]
        }
        response = client.request("DELETE", "/api/user-games/bulk-delete", content=json.dumps(bulk_data), headers={**auth_headers, "Content-Type": "application/json"})
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["deleted_count"] == 1
        assert data["failed_count"] == 1
    
    def test_bulk_delete_without_auth(self, client: TestClient, test_user_game: UserGame):
        """Test bulk delete without authentication."""
        bulk_data = {
            "user_game_ids": [str(test_user_game.id)]
        }
        response = client.request("DELETE", "/api/user-games/bulk-delete", content=json.dumps(bulk_data), headers={"Content-Type": "application/json"})
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_bulk_delete_empty_ids(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test bulk delete with empty user game IDs."""
        bulk_data = {
            "user_game_ids": []
        }
        response = client.request("DELETE", "/api/user-games/bulk-delete", content=json.dumps(bulk_data), headers={**auth_headers, "Content-Type": "application/json"})
        
        assert_api_error(response, 422)


class TestUserGamesStatsEndpoint:
    """Test GET /api/user-games/stats endpoint."""
    
    def test_get_collection_stats(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test getting collection statistics."""
        from .integration_test_utils import create_test_game
        
        # Create additional user games for better stats
        user_id = test_user_game.user_id
        
        # Create games with different statuses
        statuses = ["not_started", "in_progress", "completed", "mastered"]
        for i, status in enumerate(statuses, 1):
            game = create_test_game(
                title=f"Game {i}",
                description=f"Description {i}",
                genre="Action"
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
        user_game_data = create_test_user_game_data(test_game.id)
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
        user_game_data = create_test_user_game_data(test_game.id, play_status="invalid_status")
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)
    
    def test_ownership_status_validation(self, client: TestClient, test_game: Game, test_platform: Platform, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test ownership status validation on platform level."""
        # ownership_status is now on platform level, test via platform creation
        # First create a user game
        user_game_data = create_test_user_game_data(test_game.id)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_success(response, 201)
        user_game_id = response.json()["id"]

        # Test invalid ownership status on platform
        platform_data = {
            "platform": test_platform.name,
            "storefront": test_storefront.name,
            "ownership_status": "invalid_status"
        }
        response = client.post(f"/api/user-games/{user_game_id}/platforms", json=platform_data, headers=auth_headers)
        assert_api_error(response, 422)
    
    def test_rating_validation(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test rating validation."""
        # Test rating too low
        user_game_data = create_test_user_game_data(test_game.id, personal_rating=0.5)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)
        
        # Test rating too high
        user_game_data = create_test_user_game_data(test_game.id, personal_rating=5.5)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)
    
    def test_hours_played_validation(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test hours played validation."""
        # Test negative hours
        user_game_data = create_test_user_game_data(test_game.id, hours_played=-1)
        response = client.post("/api/user-games/", json=user_game_data, headers=auth_headers)
        assert_api_error(response, 422)


class TestPlatformOwnershipStatus:
    """Test ownership status on platform level.

    Note: ownership_status is now on UserGamePlatform, not on UserGame.
    These tests verify ownership_status behavior at the platform level.
    """

    def test_create_platform_with_ownership_status(
        self,
        client: TestClient,
        test_user_game: UserGame,
        test_platform: Platform,
        test_storefront: Storefront,
        auth_headers: Dict[str, str],
        session: Session
    ):
        """Test that platforms can be created with different ownership statuses."""
        platform_data = {
            "platform": test_platform.name,
            "storefront": test_storefront.name,
            "ownership_status": "borrowed"
        }

        response = client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            json=platform_data,
            headers=auth_headers
        )
        assert_api_success(response, 201)
        data = response.json()
        # The response is the full UserGame, check the platforms array
        assert len(data["platforms"]) == 1
        assert data["platforms"][0]["ownership_status"] == "borrowed"

    def test_update_platform_ownership_status(
        self,
        client: TestClient,
        test_user_game: UserGame,
        test_platform: Platform,
        test_storefront: Storefront,
        auth_headers: Dict[str, str],
        session: Session
    ):
        """Test that platform ownership status can be updated."""
        # Create a platform with owned status
        platform_assoc = UserGamePlatform(
            user_game_id=test_user_game.id,
            platform=test_platform.name,
            storefront=test_storefront.name,
            ownership_status="owned"
        )
        session.add(platform_assoc)
        session.commit()
        session.refresh(platform_assoc)

        # Update to subscription status - need to include platform since it's required
        update_data = {
            "platform": test_platform.name,
            "storefront": test_storefront.name,
            "ownership_status": "subscription"
        }
        response = client.put(
            f"/api/user-games/{test_user_game.id}/platforms/{platform_assoc.id}",
            json=update_data,
            headers=auth_headers
        )
        assert_api_success(response, 200)
        data = response.json()
        assert data["ownership_status"] == "subscription"

    def test_multiple_platforms_with_different_ownership_statuses(
        self,
        client: TestClient,
        test_user_game: UserGame,
        test_platform: Platform,
        test_storefront: Storefront,
        test_storefront_2: Storefront,
        auth_headers: Dict[str, str],
        session: Session
    ):
        """Test that a game can have multiple platforms with different ownership statuses."""
        # Create first platform as owned
        platform_data_1 = {
            "platform": test_platform.name,
            "storefront": test_storefront.name,
            "ownership_status": "owned"
        }
        response = client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            json=platform_data_1,
            headers=auth_headers
        )
        assert_api_success(response, 201)
        data = response.json()
        assert len(data["platforms"]) == 1
        assert data["platforms"][0]["ownership_status"] == "owned"

        # Create second platform as subscription
        platform_data_2 = {
            "platform": test_platform.name,
            "storefront": test_storefront_2.name,
            "ownership_status": "subscription"
        }
        response = client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            json=platform_data_2,
            headers=auth_headers
        )
        assert_api_success(response, 201)
        data = response.json()
        assert len(data["platforms"]) == 2

        # Get the user game and verify both platforms exist with different statuses
        response = client.get(
            f"/api/user-games/{test_user_game.id}",
            headers=auth_headers
        )
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 2
        ownership_statuses = {p["ownership_status"] for p in data["platforms"]}
        assert ownership_statuses == {"owned", "subscription"}

    def test_default_ownership_status_is_owned(
        self,
        client: TestClient,
        test_user_game: UserGame,
        test_platform: Platform,
        test_storefront: Storefront,
        auth_headers: Dict[str, str]
    ):
        """Test that default ownership status is 'owned' when not specified."""
        platform_data = {
            "platform": test_platform.name,
            "storefront": test_storefront.name
            # No ownership_status specified
        }

        response = client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            json=platform_data,
            headers=auth_headers
        )
        assert_api_success(response, 201)
        data = response.json()
        # The response is the full UserGame, check the platforms array
        assert len(data["platforms"]) == 1
        assert data["platforms"][0]["ownership_status"] == "owned"


class TestUserGameIdsEndpoint:
    """Test GET /api/user-games/ids endpoint."""

    def test_get_user_game_ids_success(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test successful user game IDs retrieval."""
        response = client.get("/api/user-games/ids", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert "ids" in data
        assert isinstance(data["ids"], list)
        assert str(test_user_game.id) in data["ids"]

    def test_get_user_game_ids_without_auth(self, client: TestClient):
        """Test user game IDs without authentication."""
        response = client.get("/api/user-games/ids")

        assert_api_error(response, 403, "Not authenticated")

    def test_get_user_game_ids_with_filter(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test user game IDs with status filter."""
        response = client.get(
            f"/api/user-games/ids?play_status={test_user_game.play_status.value}",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        assert str(test_user_game.id) in data["ids"]


class TestUserGameIdsMultiValueFilters:
    """Test multi-value filter support for /ids endpoint."""

    def test_ids_filter_multiple_genres(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test filtering IDs by multiple genres using repeated query params."""
        from .integration_test_utils import create_test_game

        # Create games with different genres
        game1 = create_test_game(title="RPG Game IDs", genre="RPG")
        game2 = create_test_game(title="Action Game IDs", genre="Action")
        game3 = create_test_game(title="Puzzle Game IDs", genre="Puzzle")
        game4 = create_test_game(title="Action RPG Game IDs", genre="Action, RPG")  # Has both
        session.add_all([game1, game2, game3, game4])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)
        session.refresh(game4)

        # Create user games
        user_games = []
        for game in [game1, game2, game3, game4]:
            ug = UserGame(
                user_id=test_user.id, game_id=game.id,
                ownership_status="owned", play_status="not_started"
            )
            session.add(ug)
            user_games.append(ug)
        session.commit()
        for ug in user_games:
            session.refresh(ug)

        # Test filtering by multiple genres (RPG OR Action)
        response = client.get(
            "/api/user-games/ids?genre=RPG&genre=Action",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return IDs for games with RPG OR Action genre
        assert len(data["ids"]) == 3  # RPG Game, Action Game, Action RPG Game
        # Verify expected IDs are present
        expected_ids = {str(user_games[0].id), str(user_games[1].id), str(user_games[3].id)}
        assert set(data["ids"]) == expected_ids

    def test_ids_filter_multiple_tags(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test filtering IDs by multiple tag IDs using repeated query params."""
        from .integration_test_utils import create_test_game
        from ..models.tag import Tag, UserGameTag

        # Create tags for the user
        tag_favorite = Tag(user_id=test_user.id, name="IDs Favorite", color="#FF0000")
        tag_backlog = Tag(user_id=test_user.id, name="IDs Backlog", color="#00FF00")
        tag_completed = Tag(user_id=test_user.id, name="IDs Completed", color="#0000FF")
        session.add_all([tag_favorite, tag_backlog, tag_completed])
        session.commit()
        session.refresh(tag_favorite)
        session.refresh(tag_backlog)
        session.refresh(tag_completed)

        # Create games
        game1 = create_test_game(title="Favorite Game IDs")
        game2 = create_test_game(title="Backlog Game IDs")
        game3 = create_test_game(title="Completed Game IDs")
        game4 = create_test_game(title="Untagged Game IDs")
        session.add_all([game1, game2, game3, game4])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)
        session.refresh(game4)

        # Create user games
        ug1 = UserGame(
            user_id=test_user.id, game_id=game1.id,
            ownership_status="owned", play_status="not_started"
        )
        ug2 = UserGame(
            user_id=test_user.id, game_id=game2.id,
            ownership_status="owned", play_status="not_started"
        )
        ug3 = UserGame(
            user_id=test_user.id, game_id=game3.id,
            ownership_status="owned", play_status="not_started"
        )
        ug4 = UserGame(
            user_id=test_user.id, game_id=game4.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add_all([ug1, ug2, ug3, ug4])
        session.commit()
        session.refresh(ug1)
        session.refresh(ug2)
        session.refresh(ug3)
        session.refresh(ug4)

        # Add tag associations
        session.add(UserGameTag(user_game_id=ug1.id, tag_id=tag_favorite.id))
        session.add(UserGameTag(user_game_id=ug2.id, tag_id=tag_backlog.id))
        session.add(UserGameTag(user_game_id=ug3.id, tag_id=tag_completed.id))
        session.commit()

        # Test filtering by multiple tags (Favorite OR Backlog)
        response = client.get(
            f"/api/user-games/ids?tag={tag_favorite.id}&tag={tag_backlog.id}",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return IDs for games with Favorite OR Backlog tag
        assert len(data["ids"]) == 2
        expected_ids = {str(ug1.id), str(ug2.id)}
        assert set(data["ids"]) == expected_ids

    def test_ids_filter_multiple_platforms(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test filtering IDs by multiple platforms using repeated query params."""
        from .integration_test_utils import create_test_game

        # Create platforms
        platform_windows = Platform(
            name="ids_windows",
            display_name="IDs Windows",
            is_active=True
        )
        platform_ps5 = Platform(
            name="ids_ps5",
            display_name="IDs PlayStation 5",
            is_active=True
        )
        platform_switch = Platform(
            name="ids_switch",
            display_name="IDs Nintendo Switch",
            is_active=True
        )
        session.add_all([platform_windows, platform_ps5, platform_switch])
        session.commit()

        # Create games
        game1 = create_test_game(title="Windows Game IDs")
        game2 = create_test_game(title="PS5 Game IDs")
        game3 = create_test_game(title="Switch Game IDs")
        session.add_all([game1, game2, game3])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)

        # Create user games with different platforms
        ug1 = UserGame(
            user_id=test_user.id, game_id=game1.id,
            ownership_status="owned", play_status="not_started"
        )
        ug2 = UserGame(
            user_id=test_user.id, game_id=game2.id,
            ownership_status="owned", play_status="not_started"
        )
        ug3 = UserGame(
            user_id=test_user.id, game_id=game3.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add_all([ug1, ug2, ug3])
        session.commit()
        session.refresh(ug1)
        session.refresh(ug2)
        session.refresh(ug3)

        # Add platform associations
        session.add(UserGamePlatform(user_game_id=ug1.id, platform="ids_windows"))
        session.add(UserGamePlatform(user_game_id=ug2.id, platform="ids_ps5"))
        session.add(UserGamePlatform(user_game_id=ug3.id, platform="ids_switch"))
        session.commit()

        # Test filtering by multiple platforms (Windows OR PS5)
        response = client.get(
            "/api/user-games/ids?platform=ids_windows&platform=ids_ps5",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return IDs for games on Windows OR PS5
        assert len(data["ids"]) == 2
        expected_ids = {str(ug1.id), str(ug2.id)}
        assert set(data["ids"]) == expected_ids

    def test_ids_no_duplicates_with_multiple_matching_platforms(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test that games with multiple platforms matching filter don't appear twice in IDs."""
        from .integration_test_utils import create_test_game

        # Create platforms
        platform_win = Platform(
            name="ids_dup_windows",
            display_name="IDs Dup Windows",
            is_active=True
        )
        platform_ps = Platform(
            name="ids_dup_ps5",
            display_name="IDs Dup PlayStation 5",
            is_active=True
        )
        session.add_all([platform_win, platform_ps])
        session.commit()

        # Create a game owned on BOTH Windows and PS5
        game = create_test_game(title="Multi-Platform Game IDs")
        session.add(game)
        session.commit()
        session.refresh(game)

        # Create user game
        user_game = UserGame(
            user_id=test_user.id, game_id=game.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        # Add BOTH platform associations to the same game
        session.add(UserGamePlatform(user_game_id=user_game.id, platform="ids_dup_windows"))
        session.add(UserGamePlatform(user_game_id=user_game.id, platform="ids_dup_ps5"))
        session.commit()

        # Filter by both platforms - the game matches BOTH filters
        response = client.get(
            "/api/user-games/ids?platform=ids_dup_windows&platform=ids_dup_ps5",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return 1 ID, NOT 2 (no duplicates)
        assert len(data["ids"]) == 1, f"Expected 1 ID but got {len(data['ids'])} - duplicate detection failed"
        assert data["ids"][0] == str(user_game.id)


class TestUserGamePlatformPlaytime:
    """Test platform-specific playtime functionality."""

    def test_add_platform_with_hours_played(
        self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session
    ):
        """Test adding a platform with hours_played."""
        import pytest

        platform = session.exec(select(Platform).limit(1)).first()
        storefront = session.exec(select(Storefront).limit(1)).first()

        if not platform:
            pytest.skip("No platform available for test")
        assert platform is not None  # Type narrowing for pyrefly

        response = client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            headers=auth_headers,
            json={
                "platform": platform.name,
                "storefront": storefront.name if storefront else None,
                "hours_played": 25
            }
        )

        assert response.status_code == 201
        data = response.json()
        platform_entry = next(
            (p for p in data["platforms"] if p["platform"] == platform.name), None
        )
        assert platform_entry is not None
        assert platform_entry["hours_played"] == 25

    def test_aggregate_hours_from_platforms(
        self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session
    ):
        """Test that aggregate hours_played is sum of platform hours."""
        import pytest

        platforms = session.exec(select(Platform).limit(1)).all()
        storefronts = session.exec(select(Storefront).limit(2)).all()

        if len(storefronts) < 2 or len(platforms) < 1:
            pytest.skip("Need at least 1 platform and 2 storefronts")

        # Add first platform with 10 hours
        client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            headers=auth_headers,
            json={
                "platform": platforms[0].name,
                "storefront": storefronts[0].name,
                "hours_played": 10
            }
        )

        # Add second platform with 20 hours
        response = client.post(
            f"/api/user-games/{test_user_game.id}/platforms",
            headers=auth_headers,
            json={
                "platform": platforms[0].name,
                "storefront": storefronts[1].name,
                "hours_played": 20
            }
        )

        assert response.status_code == 201
        data = response.json()
        # Aggregate should be 10 + 20 = 30
        assert data["hours_played"] == 30


class TestUserGameGenres:
    """Test GET /api/user-games/genres endpoint."""

    def test_get_genres_returns_unique_parsed_genres(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test that endpoint returns sorted unique genres from user's collection."""
        from .integration_test_utils import create_test_game

        # Create games with different genres
        game1 = create_test_game(title="Game 1", genre="Action")
        game2 = create_test_game(title="Game 2", genre="RPG")
        game3 = create_test_game(title="Game 3", genre="Action")  # Duplicate genre

        session.add_all([game1, game2, game3])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)

        # Add games to user's collection
        for game in [game1, game2, game3]:
            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id,
                ownership_status="owned",
                play_status="not_started"
            )
            session.add(user_game)
        session.commit()

        response = client.get("/api/user-games/genres", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert "genres" in data
        # Should be sorted and unique
        assert data["genres"] == ["Action", "RPG"]

    def test_get_genres_empty_collection(
        self, client: TestClient, auth_headers: Dict[str, str]
    ):
        """Test that endpoint returns empty list for user with no games."""
        response = client.get("/api/user-games/genres", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert "genres" in data
        assert data["genres"] == []

    def test_get_genres_parses_comma_separated(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test that comma-separated genres are split into individual genres."""
        from .integration_test_utils import create_test_game

        # Create game with comma-separated genres
        game = create_test_game(title="Multi-Genre Game", genre="Action, RPG, Adventure")
        session.add(game)
        session.commit()
        session.refresh(game)

        # Add to user's collection
        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status="owned",
            play_status="not_started"
        )
        session.add(user_game)
        session.commit()

        response = client.get("/api/user-games/genres", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        # Should be split and sorted alphabetically
        assert data["genres"] == ["Action", "Adventure", "RPG"]

    def test_get_genres_requires_auth(self, client: TestClient):
        """Test that endpoint requires authentication."""
        response = client.get("/api/user-games/genres")

        assert_api_error(response, 403, "Not authenticated")


class TestMultiValueFilters:
    """Test multi-value filter support for user games endpoint."""

    def test_filter_multiple_platforms(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test filtering by multiple platforms using repeated query params."""
        from .integration_test_utils import create_test_game

        # Create additional platforms
        platform_windows = Platform(
            name="windows",
            display_name="Windows",
            is_active=True
        )
        platform_ps5 = Platform(
            name="playstation_5",
            display_name="PlayStation 5",
            is_active=True
        )
        platform_switch = Platform(
            name="nintendo_switch",
            display_name="Nintendo Switch",
            is_active=True
        )
        session.add_all([platform_windows, platform_ps5, platform_switch])
        session.commit()

        # Create games
        game1 = create_test_game(title="Windows Game")
        game2 = create_test_game(title="PS5 Game")
        game3 = create_test_game(title="Switch Game")
        session.add_all([game1, game2, game3])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)

        # Create user games with different platforms
        ug1 = UserGame(
            user_id=test_user.id, game_id=game1.id,
            ownership_status="owned", play_status="not_started"
        )
        ug2 = UserGame(
            user_id=test_user.id, game_id=game2.id,
            ownership_status="owned", play_status="not_started"
        )
        ug3 = UserGame(
            user_id=test_user.id, game_id=game3.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add_all([ug1, ug2, ug3])
        session.commit()
        session.refresh(ug1)
        session.refresh(ug2)
        session.refresh(ug3)

        # Add platform associations
        session.add(UserGamePlatform(user_game_id=ug1.id, platform="windows"))
        session.add(UserGamePlatform(user_game_id=ug2.id, platform="playstation_5"))
        session.add(UserGamePlatform(user_game_id=ug3.id, platform="nintendo_switch"))
        session.commit()

        # Test filtering by multiple platforms (Windows OR PS5 - IN clause is OR logic)
        response = client.get(
            "/api/user-games/?platform=windows&platform=playstation_5",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return games on Windows OR PS5 (IN clause)
        assert data["total"] == 2
        titles = {ug["game"]["title"] for ug in data["user_games"]}
        assert "Windows Game" in titles
        assert "PS5 Game" in titles
        assert "Switch Game" not in titles

    def test_filter_multiple_storefronts(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test filtering by multiple storefronts using repeated query params."""
        from .integration_test_utils import create_test_game

        # Create platforms and storefronts
        platform = Platform(name="pc", display_name="PC", is_active=True)
        storefront_steam = Storefront(
            name="steam", display_name="Steam", is_active=True, base_url="https://steam.com"
        )
        storefront_epic = Storefront(
            name="epic", display_name="Epic Games", is_active=True, base_url="https://epic.com"
        )
        storefront_gog = Storefront(
            name="gog", display_name="GOG", is_active=True, base_url="https://gog.com"
        )
        session.add_all([platform, storefront_steam, storefront_epic, storefront_gog])
        session.commit()

        # Create games
        game1 = create_test_game(title="Steam Game")
        game2 = create_test_game(title="Epic Game")
        game3 = create_test_game(title="GOG Game")
        session.add_all([game1, game2, game3])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)

        # Create user games with different storefronts
        ug1 = UserGame(
            user_id=test_user.id, game_id=game1.id,
            ownership_status="owned", play_status="not_started"
        )
        ug2 = UserGame(
            user_id=test_user.id, game_id=game2.id,
            ownership_status="owned", play_status="not_started"
        )
        ug3 = UserGame(
            user_id=test_user.id, game_id=game3.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add_all([ug1, ug2, ug3])
        session.commit()
        session.refresh(ug1)
        session.refresh(ug2)
        session.refresh(ug3)

        # Add platform/storefront associations
        session.add(UserGamePlatform(user_game_id=ug1.id, platform="pc", storefront="steam"))
        session.add(UserGamePlatform(user_game_id=ug2.id, platform="pc", storefront="epic"))
        session.add(UserGamePlatform(user_game_id=ug3.id, platform="pc", storefront="gog"))
        session.commit()

        # Test filtering by multiple storefronts (Steam OR Epic - IN clause is OR logic)
        response = client.get(
            "/api/user-games/?storefront=steam&storefront=epic",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return games from Steam OR Epic (IN clause)
        assert data["total"] == 2
        titles = {ug["game"]["title"] for ug in data["user_games"]}
        assert "Steam Game" in titles
        assert "Epic Game" in titles
        assert "GOG Game" not in titles

    def test_no_duplicates_with_multiple_matching_platforms(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test that games with multiple platforms matching filter don't appear twice."""
        from .integration_test_utils import create_test_game

        # Create platforms
        platform_windows = Platform(
            name="test_windows",
            display_name="Test Windows",
            is_active=True
        )
        platform_ps5 = Platform(
            name="test_ps5",
            display_name="Test PlayStation 5",
            is_active=True
        )
        session.add_all([platform_windows, platform_ps5])
        session.commit()

        # Create a game owned on BOTH Windows and PS5
        game = create_test_game(title="Multi-Platform Game")
        session.add(game)
        session.commit()
        session.refresh(game)

        # Create user game
        user_game = UserGame(
            user_id=test_user.id, game_id=game.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)

        # Add BOTH platform associations to the same game
        session.add(UserGamePlatform(user_game_id=user_game.id, platform="test_windows"))
        session.add(UserGamePlatform(user_game_id=user_game.id, platform="test_ps5"))
        session.commit()

        # Filter by both platforms - the game matches BOTH filters
        response = client.get(
            "/api/user-games/?platform=test_windows&platform=test_ps5",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return 1 game, NOT 2 (no duplicates)
        assert data["total"] == 1, f"Expected 1 game but got {data['total']} - duplicate detection failed"
        assert len(data["user_games"]) == 1
        assert data["user_games"][0]["game"]["title"] == "Multi-Platform Game"

    def test_filter_multiple_genres(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test filtering by multiple genres using repeated query params."""
        from .integration_test_utils import create_test_game

        # Create games with different genres
        game1 = create_test_game(title="RPG Game", genre="RPG")
        game2 = create_test_game(title="Action Game", genre="Action")
        game3 = create_test_game(title="Puzzle Game", genre="Puzzle")
        game4 = create_test_game(title="Action RPG Game", genre="Action, RPG")  # Has both
        session.add_all([game1, game2, game3, game4])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)
        session.refresh(game4)

        # Create user games
        for game in [game1, game2, game3, game4]:
            ug = UserGame(
                user_id=test_user.id, game_id=game.id,
                ownership_status="owned", play_status="not_started"
            )
            session.add(ug)
        session.commit()

        # Test filtering by multiple genres (RPG OR Action)
        response = client.get(
            "/api/user-games/?genre=RPG&genre=Action",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return games with RPG OR Action genre
        assert data["total"] == 3  # RPG Game, Action Game, Action RPG Game
        titles = {ug["game"]["title"] for ug in data["user_games"]}
        assert "RPG Game" in titles
        assert "Action Game" in titles
        assert "Action RPG Game" in titles
        assert "Puzzle Game" not in titles

    def test_filter_multiple_tags(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test filtering by multiple tag IDs using repeated query params."""
        from .integration_test_utils import create_test_game
        from ..models.tag import Tag, UserGameTag

        # Create tags for the user
        tag_favorite = Tag(user_id=test_user.id, name="Favorite", color="#FF0000")
        tag_backlog = Tag(user_id=test_user.id, name="Backlog", color="#00FF00")
        tag_completed = Tag(user_id=test_user.id, name="Completed", color="#0000FF")
        session.add_all([tag_favorite, tag_backlog, tag_completed])
        session.commit()
        session.refresh(tag_favorite)
        session.refresh(tag_backlog)
        session.refresh(tag_completed)

        # Create games
        game1 = create_test_game(title="Favorite Game")
        game2 = create_test_game(title="Backlog Game")
        game3 = create_test_game(title="Completed Game")
        game4 = create_test_game(title="Untagged Game")
        session.add_all([game1, game2, game3, game4])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)
        session.refresh(game4)

        # Create user games
        ug1 = UserGame(
            user_id=test_user.id, game_id=game1.id,
            ownership_status="owned", play_status="not_started"
        )
        ug2 = UserGame(
            user_id=test_user.id, game_id=game2.id,
            ownership_status="owned", play_status="not_started"
        )
        ug3 = UserGame(
            user_id=test_user.id, game_id=game3.id,
            ownership_status="owned", play_status="not_started"
        )
        ug4 = UserGame(
            user_id=test_user.id, game_id=game4.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add_all([ug1, ug2, ug3, ug4])
        session.commit()
        session.refresh(ug1)
        session.refresh(ug2)
        session.refresh(ug3)
        session.refresh(ug4)

        # Add tag associations
        session.add(UserGameTag(user_game_id=ug1.id, tag_id=tag_favorite.id))
        session.add(UserGameTag(user_game_id=ug2.id, tag_id=tag_backlog.id))
        session.add(UserGameTag(user_game_id=ug3.id, tag_id=tag_completed.id))
        session.commit()

        # Test filtering by multiple tags (Favorite OR Backlog)
        response = client.get(
            f"/api/user-games/?tag={tag_favorite.id}&tag={tag_backlog.id}",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        # Should return games with Favorite OR Backlog tag
        assert data["total"] == 2
        titles = {ug["game"]["title"] for ug in data["user_games"]}
        assert "Favorite Game" in titles
        assert "Backlog Game" in titles
        assert "Completed Game" not in titles
        assert "Untagged Game" not in titles

    def test_combined_multi_value_filters(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test combining multiple filter types with AND logic between types."""
        from .integration_test_utils import create_test_game
        from ..models.tag import Tag, UserGameTag

        # Create platforms, storefronts, and tags
        platform_pc = Platform(name="pc_combined", display_name="PC", is_active=True)
        platform_console = Platform(name="console_combined", display_name="Console", is_active=True)
        storefront_steam = Storefront(
            name="steam_combined", display_name="Steam", is_active=True, base_url="https://steam.com"
        )
        tag_favorite = Tag(user_id=test_user.id, name="Combined Favorite", color="#FF0000")
        session.add_all([platform_pc, platform_console, storefront_steam, tag_favorite])
        session.commit()
        session.refresh(tag_favorite)

        # Create games with different genres
        game1 = create_test_game(title="PC RPG Favorite", genre="RPG")
        game2 = create_test_game(title="PC Action Favorite", genre="Action")
        game3 = create_test_game(title="Console RPG", genre="RPG")
        game4 = create_test_game(title="PC RPG No Tag", genre="RPG")
        session.add_all([game1, game2, game3, game4])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)
        session.refresh(game3)
        session.refresh(game4)

        # Create user games
        ug1 = UserGame(
            user_id=test_user.id, game_id=game1.id,
            ownership_status="owned", play_status="not_started"
        )
        ug2 = UserGame(
            user_id=test_user.id, game_id=game2.id,
            ownership_status="owned", play_status="not_started"
        )
        ug3 = UserGame(
            user_id=test_user.id, game_id=game3.id,
            ownership_status="owned", play_status="not_started"
        )
        ug4 = UserGame(
            user_id=test_user.id, game_id=game4.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add_all([ug1, ug2, ug3, ug4])
        session.commit()
        session.refresh(ug1)
        session.refresh(ug2)
        session.refresh(ug3)
        session.refresh(ug4)

        # Add platform associations
        session.add(UserGamePlatform(user_game_id=ug1.id, platform="pc_combined", storefront="steam_combined"))
        session.add(UserGamePlatform(user_game_id=ug2.id, platform="pc_combined", storefront="steam_combined"))
        session.add(UserGamePlatform(user_game_id=ug3.id, platform="console_combined"))
        session.add(UserGamePlatform(user_game_id=ug4.id, platform="pc_combined"))

        # Add tag associations - only first two games have the favorite tag
        session.add(UserGameTag(user_game_id=ug1.id, tag_id=tag_favorite.id))
        session.add(UserGameTag(user_game_id=ug2.id, tag_id=tag_favorite.id))
        session.commit()

        # Test combining: platform=pc_combined AND genre=(RPG OR Action) AND tag=favorite
        # Should return games that:
        # - Are on PC platform (ug1, ug2, ug4)
        # - Have RPG OR Action genre (ug1, ug2, ug3, ug4)
        # - Have the favorite tag (ug1, ug2)
        # Result: ug1, ug2 (intersection)
        response = client.get(
            f"/api/user-games/?platform=pc_combined&genre=RPG&genre=Action&tag={tag_favorite.id}",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 2
        titles = {ug["game"]["title"] for ug in data["user_games"]}
        assert "PC RPG Favorite" in titles
        assert "PC Action Favorite" in titles
        assert "Console RPG" not in titles
        assert "PC RPG No Tag" not in titles

    def test_single_value_backwards_compatibility(
        self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session
    ):
        """Test that single value filters still work after multi-value support."""
        from .integration_test_utils import create_test_game

        # Create a platform
        platform = Platform(name="single_test", display_name="Single Test", is_active=True)
        session.add(platform)
        session.commit()

        # Create games
        game1 = create_test_game(title="Single Platform Game")
        game2 = create_test_game(title="No Platform Game")
        session.add_all([game1, game2])
        session.commit()
        session.refresh(game1)
        session.refresh(game2)

        # Create user games
        ug1 = UserGame(
            user_id=test_user.id, game_id=game1.id,
            ownership_status="owned", play_status="not_started"
        )
        ug2 = UserGame(
            user_id=test_user.id, game_id=game2.id,
            ownership_status="owned", play_status="not_started"
        )
        session.add_all([ug1, ug2])
        session.commit()
        session.refresh(ug1)

        # Add platform association only to first game
        session.add(UserGamePlatform(user_game_id=ug1.id, platform="single_test"))
        session.commit()

        # Test single value filter still works
        response = client.get(
            "/api/user-games/?platform=single_test",
            headers=auth_headers
        )

        assert_api_success(response, 200)
        data = response.json()
        assert data["total"] == 1
        assert data["user_games"][0]["game"]["title"] == "Single Platform Game"
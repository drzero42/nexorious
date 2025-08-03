"""
Integration tests for games endpoints.
Tests all games API endpoints with proper request/response validation.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from unittest.mock import MagicMock, patch
from typing import Dict, Any

from ..models.game import Game, GameAlias
from ..models.user import User
from ..services.igdb import GameMetadata, TwitchAuthError, IGDBError
from .integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    admin_user_fixture as admin_user,
    auth_headers_fixture as auth_headers,
    admin_headers_fixture as admin_headers,
    test_game_fixture as test_game,
    mock_igdb_service_fixture as mock_igdb_service,
    client_with_mock_igdb_fixture as client_with_mock_igdb,
    assert_api_error,
    assert_api_success,
    register_and_login_user
)


class TestGamesListEndpoint:
    """Test GET /api/games/ endpoint."""
    
    def test_list_games_success(self, client: TestClient, test_game: Game, auth_headers):
        """Test successful games list retrieval."""
        response = client.get("/api/games/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "games" in data
        assert "total" in data
        assert "page" in data
        assert "per_page" in data
        assert len(data["games"]) == 1
        assert data["games"][0]["id"] == str(test_game.id)
        assert data["games"][0]["title"] == test_game.title
    
    def test_list_games_pagination(self, client: TestClient, session: Session, auth_headers):
        """Test games list with pagination."""
        # Create multiple games
        for i in range(5):
            game = Game(
                title=f"Game {i}",
                description=f"Description {i}"
            )
            session.add(game)
        session.commit()
        
        # Test pagination
        response = client.get("/api/games/?page=1&per_page=2", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 2
        assert data["total"] == 5
        assert data["page"] == 1
        assert data["per_page"] == 2
    
    def test_list_games_search(self, client: TestClient, test_game: Game, auth_headers):
        """Test games list with search."""
        response = client.get(f"/api/games/?search={test_game.title}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 1
        assert data["games"][0]["title"] == test_game.title
    
    def test_list_games_filter_by_genre(self, client: TestClient, test_game: Game, auth_headers):
        """Test games list with genre filter."""
        response = client.get(f"/api/games/?genre={test_game.genre}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 1
        assert data["games"][0]["genre"] == test_game.genre
    
    def test_list_games_empty_result(self, client: TestClient, auth_headers):
        """Test games list with no games."""
        response = client.get("/api/games/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 0
        assert data["total"] == 0




class TestGamesDetailEndpoint:
    """Test GET /api/games/{game_id} endpoint."""
    
    def test_get_game_success(self, client: TestClient, test_game: Game, auth_headers):
        """Test successful game retrieval."""
        response = client.get(f"/api/games/{test_game.id}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["id"] == str(test_game.id)
        assert data["title"] == test_game.title
        assert data["description"] == test_game.description
        assert data["genre"] == test_game.genre
        assert data["developer"] == test_game.developer
        assert data["publisher"] == test_game.publisher
    
    def test_get_game_not_found(self, client: TestClient, auth_headers):
        """Test game retrieval with non-existent ID."""
        response = client.get("/api/games/non-existent-id", headers=auth_headers)
        
        assert_api_error(response, 404, "Game not found")
    
    def test_get_game_with_aliases(self, client: TestClient, test_game: Game, session: Session, auth_headers):
        """Test game retrieval with aliases."""
        # Add alias
        alias = GameAlias(
            game_id=test_game.id,
            alias_title="Alternative Title",
            source="test"
        )
        session.add(alias)
        session.commit()
        
        response = client.get(f"/api/games/{test_game.id}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "aliases" in data
        assert len(data["aliases"]) == 1
        assert data["aliases"][0]["alias_title"] == "Alternative Title"






class TestGameAliasesEndpoints:
    """Test game aliases endpoints."""
    
    def test_get_game_aliases(self, client: TestClient, test_game: Game, session: Session, auth_headers):
        """Test getting game aliases."""
        # Add alias
        alias = GameAlias(
            game_id=test_game.id,
            alias_title="Alternative Title",
            source="test"
        )
        session.add(alias)
        session.commit()
        
        response = client.get(f"/api/games/{test_game.id}/aliases", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data) == 1
        assert data[0]["alias_title"] == "Alternative Title"
        assert data[0]["source"] == "test"
    
    def test_create_game_alias(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test creating a game alias."""
        alias_data = {
            "alias_title": "Alternative Title",
            "source": "test"
        }
        response = client.post(f"/api/games/{test_game.id}/aliases", json=alias_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["alias_title"] == "Alternative Title"
        assert data["source"] == "test"
    
    def test_create_game_alias_without_auth(self, client: TestClient, test_game: Game):
        """Test creating a game alias without authentication."""
        alias_data = {
            "alias_title": "Alternative Title",
            "source": "test"
        }
        response = client.post(f"/api/games/{test_game.id}/aliases", json=alias_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_delete_game_alias(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str], session: Session):
        """Test deleting a game alias."""
        # Create alias
        alias = GameAlias(
            game_id=test_game.id,
            alias_title="Alternative Title",
            source="test"
        )
        session.add(alias)
        session.commit()
        session.refresh(alias)
        
        response = client.delete(f"/api/games/{test_game.id}/aliases/{alias.id}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Alias deleted successfully"


class TestIGDBIntegrationEndpoints:
    """Test IGDB integration endpoints."""
    
    def test_igdb_search_success(self, client_with_mock_igdb: TestClient, auth_headers: Dict[str, str]):
        """Test successful IGDB search."""
        search_data = {"query": "Test Game"}
        response = client_with_mock_igdb.post("/api/games/search/igdb", json=search_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "games" in data
        assert len(data["games"]) == 1
        assert data["games"][0]["title"] == "Test Game"
    
    def test_igdb_search_without_auth(self, client_with_mock_igdb: TestClient):
        """Test IGDB search without authentication."""
        search_data = {"query": "Test Game"}
        response = client_with_mock_igdb.post("/api/games/search/igdb", json=search_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_igdb_search_empty_query(self, client_with_mock_igdb: TestClient, auth_headers: Dict[str, str]):
        """Test IGDB search with empty query."""
        search_data = {"query": ""}
        response = client_with_mock_igdb.post("/api/games/search/igdb", json=search_data, headers=auth_headers)
        
        assert_api_error(response, 422)
    
    def test_igdb_import_success(self, client_with_mock_igdb: TestClient, auth_headers: Dict[str, str]):
        """Test successful IGDB import."""
        import_data = {
            "igdb_id": "12345",
            "title": "Test Game"
        }
        response = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["title"] == "Test Game"
        assert data["igdb_id"] == "12345"
    
    def test_igdb_import_with_overrides(self, client_with_mock_igdb: TestClient, auth_headers: Dict[str, str]):
        """Test IGDB import with user overrides."""
        import_data = {
            "igdb_id": "12345",
            "title": "Custom Title",
            "description": "Custom description"
        }
        response = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["title"] == "Custom Title"
        assert data["description"] == "Custom description"
    
    def test_igdb_import_existing_game(self, client_with_mock_igdb: TestClient, auth_headers: Dict[str, str]):
        """Test IGDB import when game already exists - should return existing game."""
        # First import - use the IGDB ID that the mock service returns
        import_data = {
            "igdb_id": "12345",
            "title": "Test Game"
        }
        response1 = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data, headers=auth_headers)
        assert_api_success(response1, 201)
        data1 = response1.json()
        game_id = data1["id"]
        
        # Second import of same game - should return existing game, not error
        response2 = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data, headers=auth_headers)
        assert_api_success(response2, 201)  # Should still be 201 for consistency
        data2 = response2.json()
        
        # Should return the same game
        assert data2["id"] == game_id
        assert data2["igdb_id"] == "12345"
        assert data2["title"] == "Test Game"
    
    def test_igdb_import_without_auth(self, client_with_mock_igdb: TestClient):
        """Test IGDB import without authentication."""
        import_data = {
            "igdb_id": "12345",
            "title": "Test Game"
        }
        response = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data)
        
        assert_api_error(response, 403, "Not authenticated")




class TestGameMetadataEndpoints:
    """Test game metadata endpoints."""
    
    def test_get_metadata_status(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test getting game metadata status."""
        response = client.get(f"/api/games/{test_game.id}/metadata/status", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "completeness_percentage" in data
        assert "missing_essential" in data
        assert "missing_optional" in data
    
    def test_refresh_metadata_success(self, client_with_mock_igdb: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test successful metadata refresh."""
        refresh_data = {"force": True}
        response = client_with_mock_igdb.post(f"/api/games/{test_game.id}/metadata/refresh", json=refresh_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "updated_fields" in data
        assert "success" in data
    
    def test_refresh_metadata_without_auth(self, client_with_mock_igdb: TestClient, test_game: Game):
        """Test metadata refresh without authentication."""
        refresh_data = {"force": True}
        response = client_with_mock_igdb.post(f"/api/games/{test_game.id}/metadata/refresh", json=refresh_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_populate_metadata_success(self, client_with_mock_igdb: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test successful metadata population."""
        populate_data = {"populate_missing_only": True}
        response = client_with_mock_igdb.post(f"/api/games/{test_game.id}/metadata/populate", json=populate_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "populated_fields" in data
        assert "success" in data
    
    def test_bulk_metadata_operation(self, client_with_mock_igdb: TestClient, test_game: Game, admin_headers: Dict[str, str]):
        """Test bulk metadata operation."""
        bulk_data = {
            "game_ids": [str(test_game.id)],
            "operation": "refresh"
        }
        response = client_with_mock_igdb.post("/api/games/metadata/bulk", json=bulk_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "processed_games" in data
        assert "successful_operations" in data
        assert "failed_operations" in data
    
    def test_bulk_metadata_not_admin(self, client_with_mock_igdb: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test bulk metadata operation by non-admin user."""
        bulk_data = {
            "game_ids": [str(test_game.id)],
            "operation": "refresh"
        }
        response = client_with_mock_igdb.post("/api/games/metadata/bulk", json=bulk_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Only administrators can perform bulk metadata operations")


class TestCoverArtEndpoints:
    """Test cover art endpoints."""
    
    def test_download_cover_art_success(self, client_with_mock_igdb: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test successful cover art download."""
        response = client_with_mock_igdb.post(f"/api/games/{test_game.id}/cover-art/download", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "message" in data
    
    def test_download_cover_art_without_auth(self, client_with_mock_igdb: TestClient, test_game: Game):
        """Test cover art download without authentication."""
        response = client_with_mock_igdb.post(f"/api/games/{test_game.id}/cover-art/download")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_download_cover_art_game_not_found(self, client_with_mock_igdb: TestClient, auth_headers: Dict[str, str]):
        """Test cover art download with non-existent game."""
        response = client_with_mock_igdb.post("/api/games/non-existent-id/cover-art/download", headers=auth_headers)
        
        assert_api_error(response, 404, "Game not found")
    
    def test_bulk_cover_art_download(self, client_with_mock_igdb: TestClient, test_game: Game, admin_headers: Dict[str, str]):
        """Test bulk cover art download."""
        bulk_data = {
            "game_ids": [str(test_game.id)],
            "skip_existing": True
        }
        response = client_with_mock_igdb.post("/api/games/cover-art/bulk-download", json=bulk_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "processed_games" in data
        assert "successful_operations" in data
        assert "failed_operations" in data
    
    def test_bulk_cover_art_download_not_admin(self, client_with_mock_igdb: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test bulk cover art download by non-admin user."""
        bulk_data = {
            "game_ids": [str(test_game.id)],
            "skip_existing": True
        }
        response = client_with_mock_igdb.post("/api/games/cover-art/bulk-download", json=bulk_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Only administrators can perform bulk cover art downloads")


class TestGamesEndpointsSecurity:
    """Test security aspects of games endpoints."""
    
    def test_admin_only_endpoints_require_admin(self, client_with_mock_igdb: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test that admin-only endpoints require admin access."""        
        # Test bulk metadata endpoint
        bulk_data = {
            "game_ids": [str(test_game.id)],
            "operation": "refresh"
        }
        response = client_with_mock_igdb.post("/api/games/metadata/bulk", json=bulk_data, headers=auth_headers)
        assert_api_error(response, 403, "Only administrators can perform bulk metadata operations")
        
        # Test bulk cover art download endpoint
        bulk_cover_data = {
            "game_ids": [str(test_game.id)],
            "skip_existing": True
        }
        response = client_with_mock_igdb.post("/api/games/cover-art/bulk-download", json=bulk_cover_data, headers=auth_headers)
        assert_api_error(response, 403, "Only administrators can perform bulk cover art downloads")
    
    def test_authenticated_endpoints_require_auth(self, client_with_mock_igdb: TestClient, test_game: Game):
        """Test that authenticated endpoints require authentication."""
        # Test IGDB search endpoint
        search_data = {"query": "Test Game"}
        response = client_with_mock_igdb.post("/api/games/search/igdb", json=search_data)
        assert_api_error(response, 403, "Not authenticated")
        
        # Test IGDB import endpoint
        import_data = {
            "igdb_id": "12345",
            "title": "Test Game"
        }
        response = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data)
        assert_api_error(response, 403, "Not authenticated")
        
        # Test game detail endpoint
        response = client_with_mock_igdb.get(f"/api/games/{test_game.id}")
        assert_api_error(response, 403, "Not authenticated")
    

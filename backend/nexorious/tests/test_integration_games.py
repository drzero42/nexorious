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
    create_test_game_data,
    assert_api_error,
    assert_api_success,
    register_and_login_user
)


class TestGamesListEndpoint:
    """Test GET /api/games/ endpoint."""
    
    def test_list_games_success(self, client: TestClient, test_game: Game):
        """Test successful games list retrieval."""
        response = client.get("/api/games/")
        
        assert_api_success(response, 200)
        data = response.json()
        assert "games" in data
        assert "total" in data
        assert "page" in data
        assert "per_page" in data
        assert len(data["games"]) == 1
        assert data["games"][0]["id"] == str(test_game.id)
        assert data["games"][0]["title"] == test_game.title
    
    def test_list_games_pagination(self, client: TestClient, session: Session):
        """Test games list with pagination."""
        # Create multiple games
        for i in range(5):
            game = Game(
                title=f"Game {i}",
                slug=f"game-{i}",
                description=f"Description {i}",
                is_verified=True
            )
            session.add(game)
        session.commit()
        
        # Test pagination
        response = client.get("/api/games/?page=1&per_page=2")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 2
        assert data["total"] == 5
        assert data["page"] == 1
        assert data["per_page"] == 2
    
    def test_list_games_search(self, client: TestClient, test_game: Game):
        """Test games list with search."""
        response = client.get(f"/api/games/?search={test_game.title}")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 1
        assert data["games"][0]["title"] == test_game.title
    
    def test_list_games_filter_by_genre(self, client: TestClient, test_game: Game):
        """Test games list with genre filter."""
        response = client.get(f"/api/games/?genre={test_game.genre}")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 1
        assert data["games"][0]["genre"] == test_game.genre
    
    def test_list_games_empty_result(self, client: TestClient):
        """Test games list with no games."""
        response = client.get("/api/games/")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["games"]) == 0
        assert data["total"] == 0


class TestGamesCreateEndpoint:
    """Test POST /api/games/ endpoint."""
    
    def test_create_game_success(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test successful game creation."""
        game_data = create_test_game_data()
        response = client.post("/api/games/", json=game_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["title"] == game_data["title"]
        assert data["description"] == game_data["description"]
        assert data["genre"] == game_data["genre"]
        assert data["developer"] == game_data["developer"]
        assert data["publisher"] == game_data["publisher"]
        assert data["is_verified"] is False  # Default for non-admin users
    
    def test_create_game_admin_verified(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test game creation by admin user (auto-verified)."""
        game_data = create_test_game_data()
        response = client.post("/api/games/", json=game_data, headers=admin_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["is_verified"] is True  # Admin users create verified games
    
    def test_create_game_duplicate_title(self, client: TestClient, auth_headers: Dict[str, str], test_game: Game):
        """Test creation of game with duplicate title."""
        game_data = create_test_game_data(title=test_game.title)
        response = client.post("/api/games/", json=game_data, headers=auth_headers)
        
        assert_api_error(response, 409, "already exists")
    
    def test_create_game_missing_required_fields(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test game creation with missing required fields."""
        incomplete_data = {}  # No title field
        response = client.post("/api/games/", json=incomplete_data, headers=auth_headers)
        
        assert_api_error(response, 422)
    
    def test_create_game_without_auth(self, client: TestClient):
        """Test game creation without authentication."""
        game_data = create_test_game_data()
        response = client.post("/api/games/", json=game_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_create_game_invalid_date_format(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test game creation with invalid date format."""
        game_data = create_test_game_data(release_date="invalid-date")
        response = client.post("/api/games/", json=game_data, headers=auth_headers)
        
        assert_api_error(response, 422)


class TestGamesDetailEndpoint:
    """Test GET /api/games/{game_id} endpoint."""
    
    def test_get_game_success(self, client: TestClient, test_game: Game):
        """Test successful game retrieval."""
        response = client.get(f"/api/games/{test_game.id}")
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["id"] == str(test_game.id)
        assert data["title"] == test_game.title
        assert data["description"] == test_game.description
        assert data["genre"] == test_game.genre
        assert data["developer"] == test_game.developer
        assert data["publisher"] == test_game.publisher
    
    def test_get_game_not_found(self, client: TestClient):
        """Test game retrieval with non-existent ID."""
        response = client.get("/api/games/non-existent-id")
        
        assert_api_error(response, 404, "Game not found")
    
    def test_get_game_with_aliases(self, client: TestClient, test_game: Game, session: Session):
        """Test game retrieval with aliases."""
        # Add alias
        alias = GameAlias(
            game_id=test_game.id,
            alias_title="Alternative Title",
            source="test"
        )
        session.add(alias)
        session.commit()
        
        response = client.get(f"/api/games/{test_game.id}")
        
        assert_api_success(response, 200)
        data = response.json()
        assert "aliases" in data
        assert len(data["aliases"]) == 1
        assert data["aliases"][0]["alias_title"] == "Alternative Title"


class TestGamesUpdateEndpoint:
    """Test PUT /api/games/{game_id} endpoint."""
    
    def test_update_game_success(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test successful game update."""
        update_data = {
            "title": "Updated Title",
            "description": "Updated description",
            "genre": "Updated Genre"
        }
        response = client.put(f"/api/games/{test_game.id}", json=update_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["title"] == "Updated Title"
        assert data["description"] == "Updated description"
        assert data["genre"] == "Updated Genre"
    
    def test_update_game_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test game update with non-existent ID."""
        update_data = {"title": "Updated Title"}
        response = client.put("/api/games/non-existent-id", json=update_data, headers=auth_headers)
        
        assert_api_error(response, 404, "Game not found")
    
    def test_update_game_without_auth(self, client: TestClient, test_game: Game):
        """Test game update without authentication."""
        update_data = {"title": "Updated Title"}
        response = client.put(f"/api/games/{test_game.id}", json=update_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_update_game_partial(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test partial game update."""
        update_data = {"title": "Updated Title"}
        response = client.put(f"/api/games/{test_game.id}", json=update_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["title"] == "Updated Title"
        assert data["description"] == test_game.description  # Should remain unchanged


class TestGamesDeleteEndpoint:
    """Test DELETE /api/games/{game_id} endpoint."""
    
    def test_delete_game_success(self, client: TestClient, test_game: Game, admin_headers: Dict[str, str]):
        """Test successful game deletion by admin."""
        response = client.delete(f"/api/games/{test_game.id}", headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Game deleted successfully"
    
    def test_delete_game_not_admin(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test game deletion by non-admin user."""
        response = client.delete(f"/api/games/{test_game.id}", headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_delete_game_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test game deletion with non-existent ID."""
        response = client.delete("/api/games/non-existent-id", headers=admin_headers)
        
        assert_api_error(response, 404, "Game not found")
    
    def test_delete_game_without_auth(self, client: TestClient, test_game: Game):
        """Test game deletion without authentication."""
        response = client.delete(f"/api/games/{test_game.id}")
        
        assert_api_error(response, 403, "Not authenticated")


class TestGameAliasesEndpoints:
    """Test game aliases endpoints."""
    
    def test_get_game_aliases(self, client: TestClient, test_game: Game, session: Session):
        """Test getting game aliases."""
        # Add alias
        alias = GameAlias(
            game_id=test_game.id,
            alias_title="Alternative Title",
            source="test"
        )
        session.add(alias)
        session.commit()
        
        response = client.get(f"/api/games/{test_game.id}/aliases")
        
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
    
    def test_igdb_import_without_auth(self, client_with_mock_igdb: TestClient):
        """Test IGDB import without authentication."""
        import_data = {
            "igdb_id": "12345",
            "title": "Test Game"
        }
        response = client_with_mock_igdb.post("/api/games/igdb-import", json=import_data)
        
        assert_api_error(response, 403, "Not authenticated")


class TestGameVerificationEndpoint:
    """Test game verification endpoint."""
    
    def test_verify_game_success(self, client: TestClient, test_game: Game, admin_headers: Dict[str, str]):
        """Test successful game verification by admin."""
        response = client.put(f"/api/games/{test_game.id}/verify", headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["is_verified"] is True
    
    def test_verify_game_not_admin(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test game verification by non-admin user."""
        response = client.put(f"/api/games/{test_game.id}/verify", headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_verify_game_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test game verification with non-existent ID."""
        response = client.put("/api/games/non-existent-id/verify", headers=admin_headers)
        
        assert_api_error(response, 404, "Game not found")


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
        
        assert_api_error(response, 403, "Administrative privileges required")


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
        assert "successful_downloads" in data
        assert "failed_downloads" in data
    
    def test_bulk_cover_art_download_not_admin(self, client_with_mock_igdb: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test bulk cover art download by non-admin user."""
        bulk_data = {
            "game_ids": [str(test_game.id)],
            "skip_existing": True
        }
        response = client_with_mock_igdb.post("/api/games/cover-art/bulk-download", json=bulk_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")


class TestGamesEndpointsSecurity:
    """Test security aspects of games endpoints."""
    
    def test_admin_only_endpoints_require_admin(self, client: TestClient, test_game: Game, auth_headers: Dict[str, str]):
        """Test that admin-only endpoints require admin access."""
        # Test verify endpoint
        response = client.put(f"/api/games/{test_game.id}/verify", headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")
        
        # Test delete endpoint
        response = client.delete(f"/api/games/{test_game.id}", headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_authenticated_endpoints_require_auth(self, client: TestClient, test_game: Game):
        """Test that authenticated endpoints require authentication."""
        # Test create endpoint
        game_data = create_test_game_data()
        response = client.post("/api/games/", json=game_data)
        assert_api_error(response, 403, "Not authenticated")
        
        # Test update endpoint
        response = client.put(f"/api/games/{test_game.id}", json={"title": "Updated"})
        assert_api_error(response, 403, "Not authenticated")
    
    def test_public_endpoints_allow_anonymous_access(self, client: TestClient, test_game: Game):
        """Test that public endpoints allow anonymous access."""
        # Test list endpoint
        response = client.get("/api/games/")
        assert_api_success(response, 200)
        
        # Test detail endpoint
        response = client.get(f"/api/games/{test_game.id}")
        assert_api_success(response, 200)
        
        # Test metadata status endpoint
        response = client.get(f"/api/games/{test_game.id}/metadata/status")
        assert_api_success(response, 200)
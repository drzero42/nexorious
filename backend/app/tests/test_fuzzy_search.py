"""
Tests for fuzzy search functionality in games API.
"""

from fastapi.testclient import TestClient
from sqlmodel import Session
from app.models.game import Game
from app.models.user import User


class TestFuzzySearchAPI:
    """Test fuzzy search functionality in the games API."""
    
    def test_fuzzy_search_parameter_accepted(self, client: TestClient, session: Session, test_user: User, auth_headers):
        """Test that fuzzy_threshold parameter is accepted by the API."""
        
        # Test that the parameter is accepted without error
        response = client.get("/api/games/?fuzzy_threshold=0.7", headers=auth_headers)
        assert response.status_code == 200
        assert "games" in response.json()
    
    def test_fuzzy_search_with_games(self, client: TestClient, session: Session, test_user: User, auth_headers):
        """Test fuzzy search functionality with actual games."""
        
        # Create test games
        test_games = [
            Game(
                id=1020,
                title="The Witcher 3: Wild Hunt",
                description="RPG game",
            ),
            Game(
                id=1037,
                title="Witcher 2: Assassins of Kings", 
                description="RPG game",
            ),
            Game(
                id=1877,
                title="Cyberpunk 2077",
                description="Sci-fi RPG",
            ),
        ]
        
        for game in test_games:
            session.add(game)
        session.commit()
        
        # Test regular search (no fuzzy matching)
        response = client.get("/api/games/?q=witcher", headers=auth_headers)
        assert response.status_code == 200
        response.json()
        
        # Test fuzzy search
        response = client.get("/api/games/?q=witcher&fuzzy_threshold=0.6", headers=auth_headers)
        assert response.status_code == 200
        fuzzy_results = response.json()
        
        # Fuzzy search should find at least the witcher games
        assert len(fuzzy_results["games"]) >= 2
        
        # Test fuzzy search with typo
        response = client.get("/api/games/?q=witchr&fuzzy_threshold=0.6", headers=auth_headers)
        assert response.status_code == 200
        typo_results = response.json()
        
        # Should still find witcher games despite typo
        assert len(typo_results["games"]) >= 1
    
    def test_fuzzy_search_backward_compatibility(self, client: TestClient, session: Session, test_user: User, auth_headers):
        """Test that regular search still works when fuzzy_threshold is not provided."""
        
        # Create a test game
        test_game = Game(
            id=9999,
            title="Test Game",
            description="A test game",
        )
        session.add(test_game)
        session.commit()
        
        # Regular search should still work
        response = client.get("/api/games/?q=test", headers=auth_headers)
        assert response.status_code == 200
        results = response.json()
        assert len(results["games"]) >= 1
        assert results["games"][0]["title"] == "Test Game"
    
    def test_fuzzy_search_threshold_validation(self, client: TestClient, session: Session, test_user: User, auth_headers):
        """Test fuzzy_threshold parameter validation."""
        
        # Test valid thresholds
        for threshold in [0.0, 0.5, 1.0]:
            response = client.get(f"/api/games/?fuzzy_threshold={threshold}", headers=auth_headers)
            assert response.status_code == 200
        
        # Test invalid thresholds
        for threshold in [-0.1, 1.1, 2.0]:
            response = client.get(f"/api/games/?fuzzy_threshold={threshold}", headers=auth_headers)
            assert response.status_code == 422  # Validation error
    
    def test_fuzzy_search_empty_query(self, client: TestClient, session: Session, test_user: User, auth_headers):
        """Test fuzzy search with empty query."""
        
        response = client.get("/api/games/?fuzzy_threshold=0.6", headers=auth_headers)
        assert response.status_code == 200
        results = response.json()
        # Should return all games when no query is provided
        assert "games" in results
    
    def test_fuzzy_search_with_filters(self, client: TestClient, session: Session, test_user: User, auth_headers):
        """Test that fuzzy search works with other filters."""
        
        # Create test games with different genres
        test_games = [
            Game(
                id=8888,
                title="Witcher RPG",
                genre="RPG",
            ),
            Game(
                id=7777,
                title="Witcher Action",
                genre="Action",
            ),
        ]
        
        for game in test_games:
            session.add(game)
        session.commit()
        
        # Test fuzzy search with genre filter
        response = client.get("/api/games/?q=witcher&fuzzy_threshold=0.6&genre=RPG", headers=auth_headers)
        assert response.status_code == 200
        results = response.json()
        
        # Should only return RPG games matching witcher
        for game in results["games"]:
            if game["genre"]:
                assert "RPG" in game["genre"]
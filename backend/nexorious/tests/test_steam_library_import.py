"""
Tests for Steam library import functionality.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from fastapi.testclient import TestClient
from sqlmodel import Session, select
import json

from nexorious.models.user import User
from nexorious.models.game import Game
from nexorious.models.platform import Platform, Storefront
from nexorious.models.user_game import UserGame, UserGamePlatform, OwnershipStatus
from nexorious.services.steam import SteamGame
from nexorious.tests.integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    auth_headers_fixture as auth_headers,
    assert_api_error,
    assert_api_success
)


@pytest.fixture
def mock_steam_service():
    """Mock Steam service for testing."""
    with patch('nexorious.api.steam_config.create_steam_service') as mock:
        yield mock


@pytest.fixture
def sample_steam_games():
    """Sample Steam games for testing."""
    return [
        SteamGame(
            appid=440,
            name="Team Fortress 2"
        ),
        SteamGame(
            appid=570,
            name="Dota 2"
        ),
        SteamGame(
            appid=730,
            name="Counter-Strike: Global Offensive"
        ),
        SteamGame(
            appid=945360,
            name="Among Us"
        )
    ]


@pytest.fixture
def sample_igdb_games(session: Session):
    """Sample IGDB games in database for matching."""
    games = [
        Game(
            title="Team Fortress 2",
            igdb_id=440,
            description="Team-based multiplayer shooter",
            developer="Valve",
            publisher="Valve"
        ),
        Game(
            title="Dota 2",
            igdb_id=570,
            description="Multiplayer online battle arena",
            developer="Valve",
            publisher="Valve"
        ),
        Game(
            title="Counter-Strike: Global Offensive",
            igdb_id=730,
            description="Competitive first-person shooter",
            developer="Valve",
            publisher="Valve"
        ),
        Game(
            title="The Witcher 3: Wild Hunt",
            igdb_id=1942,
            description="Open-world RPG",
            developer="CD Projekt RED",
            publisher="CD Projekt"
        )
    ]
    
    for game in games:
        session.add(game)
    session.commit()
    
    for game in games:
        session.refresh(game)
    
    return games


@pytest.fixture
def platforms_and_storefronts(session: Session):
    """Create platforms and storefronts for testing."""
    platforms = [
        Platform(name="pc-windows", display_name="PC (Windows)", is_active=True),
        Platform(name="pc-linux", display_name="PC (Linux)", is_active=True),
        Platform(name="pc-mac", display_name="PC (Mac)", is_active=True)
    ]
    
    storefronts = [
        Storefront(name="steam", display_name="Steam", is_active=True)
    ]
    
    for platform in platforms:
        session.add(platform)
    for storefront in storefronts:
        session.add(storefront)
    
    session.commit()
    
    for platform in platforms:
        session.refresh(platform)
    for storefront in storefronts:
        session.refresh(storefront)
    
    return platforms, storefronts


class TestSteamLibraryImport:
    """Test Steam library import endpoint."""
    
    def test_import_library_success(self, client: TestClient, auth_headers, session: Session, 
                                   test_user: User, mock_steam_service, sample_steam_games, 
                                   sample_igdb_games, platforms_and_storefronts):
        """Test successful Steam library import."""
        # Set up user with Steam config
        user = session.get(User, test_user.id)
        steam_config = {
            "steam": {
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
                "steam_id": "76561197960435530",
                "is_verified": True
            }
        }
        user.preferences_json = json.dumps(steam_config)
        session.commit()
        
        # Mock Steam service
        mock_service = MagicMock()
        mock_service.get_owned_games = AsyncMock(return_value=sample_steam_games)
        mock_steam_service.return_value = mock_service
        
        import_request = {
            "fuzzy_threshold": 0.8,
            "merge_strategy": "skip"
        }
        
        response = client.post("/api/steam/import-library", json=import_request, headers=auth_headers)
        
        assert_api_success(response)
        data = response.json()
        
        # Verify response structure
        assert data["total_games"] == 4
        assert data["imported_count"] == 3  # TF2, Dota 2, CS:GO match
        assert data["no_match_count"] == 1  # Among Us doesn't match
        assert "platform_breakdown" in data
        assert "results" in data
        assert len(data["results"]) == 4
        
        # Verify platform detection - all Steam games get PC (Windows)
        assert data["platform_breakdown"]["pc-windows"] == 4  # All 4 games on Windows
        
        # Verify games were added to user's collection
        user_games = session.exec(select(UserGame).where(UserGame.user_id == test_user.id)).all()
        assert len(user_games) == 3
        
        # Verify platform associations - each game gets PC (Windows)
        platform_associations = session.exec(
            select(UserGamePlatform).join(UserGame).where(UserGame.user_id == test_user.id)
        ).all()
        assert len(platform_associations) == 3  # One association per imported game
    
    def test_import_library_no_steam_config(self, client: TestClient, auth_headers):
        """Test import without Steam configuration."""
        import_request = {"fuzzy_threshold": 0.8}
        
        response = client.post("/api/steam/import-library", json=import_request, headers=auth_headers)
        
        assert_api_error(response, 400, "Please configure your Steam Web API key first")
    
    def test_import_library_no_steam_id(self, client: TestClient, auth_headers, session: Session, test_user: User):
        """Test import without Steam ID configured."""
        # Set up user with API key but no Steam ID
        user = session.get(User, test_user.id)
        steam_config = {
            "steam": {
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890"
            }
        }
        user.preferences_json = json.dumps(steam_config)
        session.commit()
        
        import_request = {"fuzzy_threshold": 0.8}
        
        response = client.post("/api/steam/import-library", json=import_request, headers=auth_headers)
        
        assert_api_error(response, 400, "Please configure your Steam ID first")
    
    def test_import_library_existing_games_skip(self, client: TestClient, auth_headers, session: Session,
                                              test_user: User, mock_steam_service, sample_steam_games,
                                              sample_igdb_games, platforms_and_storefronts):
        """Test import with existing games using skip strategy."""
        # Set up user with Steam config
        user = session.get(User, test_user.id)
        steam_config = {
            "steam": {
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
                "steam_id": "76561197960435530",
                "is_verified": True
            }
        }
        user.preferences_json = json.dumps(steam_config)
        session.commit()
        
        # Add one game to user's collection already
        existing_game = sample_igdb_games[0]  # Team Fortress 2
        user_game = UserGame(
            user_id=test_user.id,
            game_id=existing_game.id,
            ownership_status=OwnershipStatus.OWNED
        )
        session.add(user_game)
        session.commit()
        
        # Mock Steam service
        mock_service = MagicMock()
        mock_service.get_owned_games = AsyncMock(return_value=sample_steam_games)
        mock_steam_service.return_value = mock_service
        
        import_request = {
            "fuzzy_threshold": 0.8,
            "merge_strategy": "skip"
        }
        
        response = client.post("/api/steam/import-library", json=import_request, headers=auth_headers)
        
        assert_api_success(response)
        data = response.json()
        
        # Should import 2 new games, skip 1 existing, 1 no match
        assert data["imported_count"] == 2
        assert data["skipped_count"] == 1
        assert data["no_match_count"] == 1
        
        # Verify TF2 was skipped
        tf2_result = next(r for r in data["results"] if r["steam_appid"] == 440)
        assert tf2_result["status"] == "skipped"
        assert "already in collection" in tf2_result["reason"]
    
    def test_import_library_existing_games_add_platforms(self, client: TestClient, auth_headers, session: Session,
                                                       test_user: User, mock_steam_service, sample_steam_games,
                                                       sample_igdb_games, platforms_and_storefronts):
        """Test import with existing games using add_platforms strategy."""
        # Set up user with Steam config
        user = session.get(User, test_user.id)
        steam_config = {
            "steam": {
                "web_api_key": "ABCDEF1234567890ABCDEF1234567890",
                "steam_id": "76561197960435530",
                "is_verified": True
            }
        }
        user.preferences_json = json.dumps(steam_config)
        session.commit()
        
        # Add Dota 2 to user's collection with only Windows platform
        dota_game = sample_igdb_games[1]  # Dota 2
        user_game = UserGame(
            user_id=test_user.id,
            game_id=dota_game.id,
            ownership_status=OwnershipStatus.OWNED
        )
        session.add(user_game)
        session.flush()
        
        # Add Windows platform association
        platforms, storefronts = platforms_and_storefronts
        windows_platform = next(p for p in platforms if p.name == "pc-windows")
        steam_storefront = storefronts[0]
        
        user_game_platform = UserGamePlatform(
            user_game_id=user_game.id,
            platform_id=windows_platform.id,
            storefront_id=steam_storefront.id
        )
        session.add(user_game_platform)
        session.commit()
        
        # Mock Steam service
        mock_service = MagicMock()
        mock_service.get_owned_games = AsyncMock(return_value=sample_steam_games)
        mock_steam_service.return_value = mock_service
        
        import_request = {
            "fuzzy_threshold": 0.8,
            "merge_strategy": "add_platforms"
        }
        
        response = client.post("/api/steam/import-library", json=import_request, headers=auth_headers)
        
        assert_api_success(response)
        data = response.json()
        
        # Should skip existing Dota 2 since it already has PC (Windows) platform
        dota_result = next(r for r in data["results"] if r["steam_appid"] == 570)
        assert dota_result["status"] == "skipped"
        assert "pc-windows" in dota_result["detected_platforms"]
        
        # Verify no additional platform associations were added
        platform_associations = session.exec(select(UserGamePlatform).where(
            UserGamePlatform.user_game_id == user_game.id
        )).all()
        assert len(platform_associations) == 1  # Only the original Windows platform



class TestGameMatching:
    """Test fuzzy game matching logic."""
    
    def test_exact_match(self):
        """Test exact name matching."""
        from nexorious.api.steam_config import _find_best_game_match
        
        games = [
            Game(title="Team Fortress 2", igdb_id=440),
            Game(title="Dota 2", igdb_id=570)
        ]
        
        match, score = _find_best_game_match("Team Fortress 2", games, 0.8)
        assert match is not None
        assert match.title == "Team Fortress 2"
        assert score == 1.0
    
    def test_fuzzy_match_above_threshold(self):
        """Test fuzzy matching above threshold."""
        from nexorious.api.steam_config import _find_best_game_match
        
        games = [
            Game(title="Counter-Strike: Global Offensive", igdb_id=730),
            Game(title="Dota 2", igdb_id=570)
        ]
        
        match, score = _find_best_game_match("CS:GO", games, 0.6)
        assert match is not None
        assert match.title == "Counter-Strike: Global Offensive"
        assert score >= 0.6
    
    def test_fuzzy_match_below_threshold(self):
        """Test fuzzy matching below threshold."""
        from nexorious.api.steam_config import _find_best_game_match
        
        games = [
            Game(title="The Witcher 3: Wild Hunt", igdb_id=1942),
            Game(title="Dota 2", igdb_id=570)
        ]
        
        match, score = _find_best_game_match("Among Us", games, 0.8)
        assert match is None
        assert score < 0.8
    
    def test_no_games_available(self):
        """Test matching with no games in database."""
        from nexorious.api.steam_config import _find_best_game_match
        
        match, score = _find_best_game_match("Any Game", [], 0.8)
        assert match is None
        assert score == 0.0
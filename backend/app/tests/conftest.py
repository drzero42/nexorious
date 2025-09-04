"""
Global test configuration and fixtures.
Makes all fixtures from integration_test_utils.py available globally to all tests.
"""

import pytest
from unittest.mock import patch

# Import all fixtures from integration_test_utils to make them globally available
from .integration_test_utils import (
    session_fixture as session,
    test_logo_service_fixture as test_logo_service, 
    client_fixture as client,
    client_with_logo_service_fixture as client_with_logo_service,
    test_user_fixture as test_user,
    admin_user_fixture as admin_user,
    auth_headers_fixture as auth_headers,
    admin_headers_fixture as admin_headers,
    test_platform_fixture as test_platform,
    pc_windows_platform_fixture as pc_windows_platform,
    test_storefront_fixture as test_storefront,
    test_storefront_2_fixture as test_storefront_2,
    steam_dependencies_fixture as steam_dependencies,
    test_game_fixture as test_game,
    test_user_game_fixture as test_user_game,
    mock_igdb_service_fixture as mock_igdb_service,
    configurable_mock_igdb_fixture as configurable_mock_igdb,
    client_with_mock_igdb_fixture as client_with_mock_igdb
)

# Additional global fixtures for Steam testing

@pytest.fixture
def mock_steam_service():
    """Mock Steam service for testing - globally available."""
    with patch('app.services.import_sources.steam.create_steam_service') as mock:
        yield mock


@pytest.fixture
def mock_steam_games_service():
    """Mock Steam games service for testing - globally available."""
    with patch('app.services.steam_games.create_steam_games_service') as mock:
        yield mock


@pytest.fixture
def client_with_shared_session(session):
    """Test client that shares the same database session with tests."""
    from fastapi.testclient import TestClient
    from app.main import app
    from app.core.database import get_session
    
    # Override the dependency to use the same session as the test
    def get_shared_session():
        return session
    
    app.dependency_overrides[get_session] = get_shared_session
    
    try:
        with TestClient(app) as client:
            yield client
    finally:
        # Clean up the override
        del app.dependency_overrides[get_session]


# Re-export all fixtures so pytest can find them
__all__ = [
    'session',
    'test_logo_service',
    'client', 
    'client_with_logo_service',
    'client_with_shared_session',
    'test_user',
    'admin_user', 
    'auth_headers',
    'admin_headers',
    'test_platform',
    'pc_windows_platform',
    'test_storefront',
    'test_storefront_2',
    'steam_dependencies',
    'test_game',
    'test_user_game',
    'mock_igdb_service',
    'configurable_mock_igdb',
    'client_with_mock_igdb',
    'mock_steam_service',
    'mock_steam_games_service'
]
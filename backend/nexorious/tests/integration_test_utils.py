"""
Shared utilities for integration tests.
Provides common fixtures, helpers, and test client setup.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine, select
from sqlmodel.pool import StaticPool
from typing import Dict, Any, Optional
from unittest.mock import MagicMock
from datetime import date

from ..main import app
from ..core.database import get_session
from ..core.security import get_current_user, create_access_token
from ..models.user import User
from ..models.game import Game
from ..models.platform import Platform, Storefront
from ..models.user_game import UserGame
from ..api.dependencies import get_igdb_service_dependency
from ..services.igdb import IGDBService, GameMetadata


@pytest.fixture(name="session")
def session_fixture():
    """Create a test database engine and session."""
    engine = create_engine(
        "sqlite:///:memory:",
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
    )
    SQLModel.metadata.create_all(engine)
    with Session(engine) as session:
        yield session


@pytest.fixture(name="client")
def client_fixture(session: Session):
    """Create a test client with the test database session."""
    # Store the engine for creating new sessions
    test_engine = session.get_bind()
    
    def get_session_override():
        # Create a new session for each request to avoid isolation issues
        with Session(test_engine) as new_session:
            yield new_session

    app.dependency_overrides[get_session] = get_session_override
    client = TestClient(app)
    yield client
    app.dependency_overrides.clear()


@pytest.fixture(name="test_user")
def test_user_fixture(session: Session) -> User:
    """Create a test user in the database."""
    user = User(
        email="test@example.com",
        username="testuser",
        password_hash="$2b$12$test_hash",
        is_active=True,
        is_admin=False
    )
    session.add(user)
    session.commit()
    session.refresh(user)
    return user


@pytest.fixture(name="admin_user")
def admin_user_fixture(session: Session) -> User:
    """Create an admin user in the database."""
    user = User(
        email="admin@example.com",
        username="admin",
        password_hash="$2b$12$admin_hash",
        is_active=True,
        is_admin=True
    )
    session.add(user)
    session.commit()
    session.refresh(user)
    return user


@pytest.fixture(name="auth_headers")
def auth_headers_fixture(test_user: User, session: Session) -> Dict[str, str]:
    """Create authentication headers for the test user."""
    from ..core.security import hash_token
    from ..models.user import UserSession
    from datetime import datetime, timedelta, timezone
    import uuid
    
    token = create_access_token(data={"sub": test_user.id})
    
    # Create session record for the token
    session_record = UserSession(
        id=str(uuid.uuid4()),
        user_id=test_user.id,
        token_hash=hash_token(token),
        refresh_token_hash=hash_token("test_refresh_token"),
        expires_at=datetime.now(timezone.utc) + timedelta(days=30),
        user_agent="test-client",
        ip_address="127.0.0.1"
    )
    session.add(session_record)
    session.commit()
    
    return {"Authorization": f"Bearer {token}"}


@pytest.fixture(name="admin_headers")
def admin_headers_fixture(admin_user: User, session: Session) -> Dict[str, str]:
    """Create authentication headers for the admin user."""
    from ..core.security import hash_token
    from ..models.user import UserSession
    from datetime import datetime, timedelta, timezone
    import uuid
    
    token = create_access_token(data={"sub": admin_user.id})
    
    # Create session record for the token
    session_record = UserSession(
        id=str(uuid.uuid4()),
        user_id=admin_user.id,
        token_hash=hash_token(token),
        refresh_token_hash=hash_token("test_refresh_token"),
        expires_at=datetime.now(timezone.utc) + timedelta(days=30),
        user_agent="test-client",
        ip_address="127.0.0.1"
    )
    session.add(session_record)
    session.commit()
    
    return {"Authorization": f"Bearer {token}"}


@pytest.fixture(name="test_platform")
def test_platform_fixture(session: Session) -> Platform:
    """Create a test platform in the database."""
    platform = Platform(
        name="pc",
        display_name="PC",
        icon_url="https://example.com/pc.png",
        is_active=True
    )
    session.add(platform)
    session.commit()
    session.refresh(platform)
    return platform


@pytest.fixture(name="test_storefront")
def test_storefront_fixture(session: Session) -> Storefront:
    """Create a test storefront in the database."""
    storefront = Storefront(
        name="steam",
        display_name="Steam",
        icon_url="https://example.com/steam.png",
        base_url="https://store.steampowered.com",
        is_active=True
    )
    session.add(storefront)
    session.commit()
    session.refresh(storefront)
    return storefront


@pytest.fixture(name="test_game")
def test_game_fixture(session: Session) -> Game:
    """Create a test game in the database."""
    game = Game(
        title="Test Game",
        slug="test-game",
        description="A test game for integration testing",
        genre="Action",
        developer="Test Developer",
        publisher="Test Publisher",
        release_date=date(2023, 1, 1),
        cover_art_url="https://example.com/cover.jpg",
        igdb_id="12345",
        is_verified=False  # Make it unverified so regular users can update it
    )
    session.add(game)
    session.commit()
    session.refresh(game)
    return game


@pytest.fixture(name="verified_game")
def verified_game_fixture(session: Session) -> Game:
    """Create a verified test game in the database."""
    game = Game(
        title="Verified Game",
        slug="verified-game",
        description="A verified test game for integration testing",
        genre="Action",
        developer="Test Developer",
        publisher="Test Publisher",
        release_date=date(2023, 1, 1),
        igdb_id="54321",
        is_verified=True
    )
    session.add(game)
    session.commit()
    session.refresh(game)
    return game


@pytest.fixture(name="test_user_game")
def test_user_game_fixture(session: Session, test_user: User, test_game: Game) -> UserGame:
    """Create a test user game in the database."""
    user_game = UserGame(
        user_id=test_user.id,
        game_id=test_game.id,
        ownership_status="owned",
        is_physical=False,
        personal_rating=4.5,
        is_loved=True,
        play_status="completed",
        hours_played=25,
        personal_notes="Great game!"
    )
    session.add(user_game)
    session.commit()
    session.refresh(user_game)
    return user_game


@pytest.fixture(name="mock_igdb_service")
def mock_igdb_service_fixture():
    """Create a mock IGDB service for testing."""
    mock_service = MagicMock(spec=IGDBService)
    
    # Mock search_games method
    mock_service.search_games.return_value = [
        GameMetadata(
            igdb_id="12345",
            title="Test Game",
            slug="test-game",
            description="A test game",
            genre="Action",
            developer="Test Developer",
            publisher="Test Publisher",
            release_date="2023-01-01",
            cover_art_url="https://example.com/cover.jpg",
            hastily=10,
            normally=15,
            completely=20
        )
    ]
    
    # Mock get_game_by_id method
    mock_service.get_game_by_id.return_value = GameMetadata(
        igdb_id="12345",
        title="Test Game",
        slug="test-game",
        description="A test game",
        genre="Action",
        developer="Test Developer",
        publisher="Test Publisher",
        release_date="2023-01-01",
        cover_art_url="https://example.com/cover.jpg",
        hastily=10,
        normally=15,
        completely=20
    )
    
    # Mock refresh_game_metadata method
    mock_service.refresh_game_metadata.return_value = GameMetadata(
        igdb_id="12345",
        title="Test Game",
        slug="test-game",
        description="A test game",
        genre="Action",
        developer="Test Developer",
        publisher="Test Publisher",
        release_date="2023-01-01",
        cover_art_url="https://example.com/cover.jpg",
        hastily=10,
        normally=15,
        completely=20
    )
    
    # Mock populate_missing_metadata method
    mock_service.populate_missing_metadata.return_value = GameMetadata(
        igdb_id="12345",
        title="Test Game",
        slug="test-game",
        description="A test game",
        genre="Action",
        developer="Test Developer",
        publisher="Test Publisher",
        release_date="2023-01-01",
        cover_art_url="https://example.com/cover.jpg",
        hastily=10,
        normally=15,
        completely=20
    )
    
    # Mock get_metadata_completeness method
    mock_service.get_metadata_completeness.return_value = {
        "completeness_percentage": 75.0,
        "missing_essential": ["cover_art_url"],
        "missing_optional": ["rating_average"],
        "total_fields": 10,
        "filled_fields": 7
    }
    
    # Mock download_and_store_cover_art method
    mock_service.download_and_store_cover_art.return_value = "/local/path/cover.jpg"
    
    # Mock bulk_download_cover_art method
    mock_service.bulk_download_cover_art.return_value = {
        "processed_games": 1,
        "successful_downloads": 1,
        "failed_downloads": 0,
        "results": [{"game_id": "test-id", "success": True, "local_path": "/local/path/cover.jpg"}],
        "errors": []
    }
    
    return mock_service


@pytest.fixture(name="client_with_mock_igdb")
def client_with_mock_igdb_fixture(session: Session, mock_igdb_service):
    """Create a test client with mocked IGDB service."""
    def get_session_override():
        return session
    
    def get_igdb_service_override():
        return mock_igdb_service

    app.dependency_overrides[get_session] = get_session_override
    app.dependency_overrides[get_igdb_service_dependency] = get_igdb_service_override
    
    client = TestClient(app)
    yield client
    app.dependency_overrides.clear()


def create_test_user_data(
    email: str = "newuser@example.com",
    username: str = "newuser",
    password: str = "testpassword123"
) -> Dict[str, Any]:
    """Create test user registration data."""
    return {
        "email": email,
        "username": username,
        "password": password
    }


def create_test_game_data(
    title: str = "Test Game",
    description: str = "A test game",
    genre: str = "Action",
    developer: str = "Test Developer",
    publisher: str = "Test Publisher",
    release_date: str = "2023-01-01"
) -> Dict[str, Any]:
    """Create test game data."""
    return {
        "title": title,
        "description": description,
        "genre": genre,
        "developer": developer,
        "publisher": publisher,
        "release_date": release_date
    }


def create_test_platform_data(
    name: str = "test_platform",
    display_name: str = "Test Platform",
    icon_url: str = "https://example.com/icon.png"
) -> Dict[str, Any]:
    """Create test platform data."""
    return {
        "name": name,
        "display_name": display_name,
        "icon_url": icon_url,
        "is_active": True
    }


def create_test_storefront_data(
    name: str = "test_storefront",
    display_name: str = "Test Storefront",
    icon_url: str = "https://example.com/icon.png",
    base_url: str = "https://example.com/"
) -> Dict[str, Any]:
    """Create test storefront data."""
    return {
        "name": name,
        "display_name": display_name,
        "icon_url": icon_url,
        "base_url": base_url,
        "is_active": True
    }


def create_test_user_game_data(
    game_id: str,
    ownership_status: str = "owned",
    is_physical: bool = False,
    personal_rating: Optional[float] = None,
    is_loved: bool = False,
    play_status: str = "not_started",
    hours_played: int = 0,
    personal_notes: str = ""
) -> Dict[str, Any]:
    """Create test user game data."""
    data = {
        "game_id": game_id,
        "ownership_status": ownership_status,
        "is_physical": is_physical,
        "is_loved": is_loved,
        "play_status": play_status,
        "hours_played": hours_played,
        "personal_notes": personal_notes
    }
    if personal_rating is not None:
        data["personal_rating"] = personal_rating
    return data


def assert_api_error(response, status_code: int, error_message: str = None):
    """Assert that an API response contains the expected error."""
    assert response.status_code == status_code
    data = response.json()
    
    # Handle different error formats
    if status_code == 422:
        # FastAPI validation errors use 'detail' field
        assert "detail" in data
        if error_message:
            # For 422, check if error_message is in any of the detail messages
            detail_str = str(data["detail"])
            assert error_message in detail_str
    else:
        # Custom errors use 'error' field
        assert "error" in data
        if error_message:
            assert error_message in data["error"]


def assert_api_success(response, status_code: int = 200):
    """Assert that an API response is successful."""
    assert response.status_code == status_code
    data = response.json()
    # For success responses, we just check that there's no error field
    # or that it's None/empty
    if "error" in data:
        assert data["error"] is None or data["error"] == ""


def register_and_login_user(client: TestClient, user_data: Dict[str, Any]) -> Dict[str, str]:
    """Register a user and return authentication headers."""
    # Register user
    register_response = client.post("/api/auth/register", json=user_data)
    assert register_response.status_code == 201
    
    # Login user
    login_data = {
        "username": user_data["email"],
        "password": user_data["password"]
    }
    login_response = client.post("/api/auth/login", json=login_data)
    assert login_response.status_code == 200
    
    token = login_response.json()["access_token"]
    return {"Authorization": f"Bearer {token}"}
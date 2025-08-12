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
from ..services.logo_service import LogoService


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


@pytest.fixture(name="test_logo_service", scope="session")
def test_logo_service_fixture():
    """Create a test logo service with temporary directory."""
    import tempfile
    import shutil
    
    temp_dir = tempfile.mkdtemp()
    service = LogoService(temp_dir)
    yield service
    # Cleanup temp directory after test
    shutil.rmtree(temp_dir, ignore_errors=True)


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


@pytest.fixture(name="client_with_logo_service")
def client_with_logo_service_fixture(session: Session, test_logo_service: LogoService):
    """Create a test client with the test database session and logo service."""
    # Store the engine for creating new sessions
    test_engine = session.get_bind()
    
    def get_session_override():
        # Create a new session for each request to avoid isolation issues
        with Session(test_engine) as new_session:
            yield new_session
    
    def get_logo_service_override():
        return test_logo_service

    from ..api.platforms import get_logo_service
    app.dependency_overrides[get_session] = get_session_override
    app.dependency_overrides[get_logo_service] = get_logo_service_override
    client = TestClient(app)
    yield client
    app.dependency_overrides.clear()


@pytest.fixture(name="test_user")
def test_user_fixture(session: Session) -> User:
    """Create a test user in the database."""
    user = User(
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
        name="test-platform",
        display_name="Test Platform",
        icon_url="https://example.com/test-platform.png",
        is_active=True
    )
    session.add(platform)
    session.commit()
    session.refresh(platform)
    return platform


@pytest.fixture(name="pc_windows_platform")
def pc_windows_platform_fixture(session: Session) -> Platform:
    """Create a PC-Windows platform required for Steam Games functionality."""
    platform = Platform(
        name="pc-windows",
        display_name="PC (Windows)",
        icon_url="/static/logos/platforms/pc-windows/pc-windows-icon-light.svg",
        is_active=True,
        source="official"
    )
    session.add(platform)
    session.commit()
    session.refresh(platform)
    return platform


@pytest.fixture(name="test_storefront")
def test_storefront_fixture(session: Session) -> Storefront:
    """Create a test storefront in the database."""
    storefront = Storefront(
        name="test-storefront",
        display_name="Test Storefront",
        icon_url="https://example.com/test-storefront.png",
        base_url="https://store.test-storefront.com",
        is_active=True
    )
    session.add(storefront)
    session.commit()
    session.refresh(storefront)
    return storefront


@pytest.fixture(name="test_storefront_2")
def test_storefront_2_fixture(session: Session) -> Storefront:
    """Create a second test storefront in the database."""
    storefront = Storefront(
        name="test-storefront-2",
        display_name="Test Storefront 2",
        base_url="https://store.test-storefront-2.com/",
        is_active=True
    )
    session.add(storefront)
    session.commit()
    session.refresh(storefront)
    return storefront


@pytest.fixture(name="steam_dependencies")
def steam_dependencies_fixture(session: Session):
    """Create all dependencies required for Steam Games functionality."""
    # Create PC-Windows platform
    pc_windows_platform = Platform(
        name="pc-windows",
        display_name="PC (Windows)",
        icon_url="/static/logos/platforms/pc-windows/pc-windows-icon-light.svg",
        is_active=True,
        source="official"
    )
    session.add(pc_windows_platform)
    
    # Create Steam storefront
    steam_storefront = Storefront(
        name="steam",
        display_name="Steam",
        icon_url="/static/logos/storefronts/steam/steam-icon-light.svg",
        base_url="https://store.steampowered.com",
        is_active=True,
        source="official"
    )
    session.add(steam_storefront)
    session.commit()
    session.refresh(pc_windows_platform)
    session.refresh(steam_storefront)
    
    return {"platform": pc_windows_platform, "storefront": steam_storefront}


@pytest.fixture(name="test_game")
def test_game_fixture(session: Session) -> Game:
    """Create a test game in the database."""
    game = Game(
        title="Test Game",
        description="A test game for integration testing",
        genre="Action",
        developer="Test Developer",
        publisher="Test Publisher",
        release_date=date(2023, 1, 1),
        cover_art_url="https://example.com/cover.jpg",
        igdb_id="12345",
          # Make it unverified so regular users can update it
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
            igdb_slug="test-game",
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
    
    # Mock get_game_by_id method with different data based on igdb_id
    def mock_get_game_by_id(igdb_id: str) -> GameMetadata:
        game_data_map = {
            "12345": {
                "title": "Test Game",
                "genre": "Action",
                "developer": "Test Developer",
                "publisher": "Test Publisher"
            },
            "100": {
                "title": "Zelda",
                "genre": "Adventure", 
                "developer": "Nintendo",
                "publisher": "Nintendo"
            },
            "200": {
                "title": "Elden Ring",
                "genre": "RPG",
                "developer": "FromSoftware", 
                "publisher": "Bandai Namco"
            },
            "300": {
                "title": "Apex Legends",
                "genre": "Shooter",
                "developer": "Respawn",
                "publisher": "EA"
            }
        }
        
        data = game_data_map.get(igdb_id, game_data_map["12345"])
        return GameMetadata(
            igdb_id=igdb_id,
            title=data["title"],
            igdb_slug=data["title"].lower().replace(" ", "-"),
            description=f"A {data['genre'].lower()} game",
            genre=data["genre"],
            developer=data["developer"],
            publisher=data["publisher"],
            release_date="2023-01-01",
            cover_art_url="https://example.com/cover.jpg",
            hastily=10,
            normally=15,
            completely=20
        )
    
    mock_service.get_game_by_id.side_effect = mock_get_game_by_id
    
    # Mock refresh_game_metadata method
    mock_service.refresh_game_metadata.return_value = GameMetadata(
        igdb_id="12345",
        title="Test Game",
        igdb_slug="test-game",
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
        igdb_slug="test-game",
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
    username: str = "newuser",
    password: str = "testpassword123"
) -> Dict[str, Any]:
    """Create test user registration data."""
    return {
        "username": username,
        "password": password
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


def create_test_game(
    title: str = None,
    description: str = None,
    igdb_id: str = None,
    **kwargs
) -> Game:
    """Create a test game with automatically generated IGDB ID if not provided.
    
    Args:
        title: Game title (defaults to auto-generated)
        description: Game description (defaults to auto-generated)
        igdb_id: IGDB ID (defaults to auto-generated unique ID)
        **kwargs: Additional fields to set on the Game object
    
    Returns:
        Game object with guaranteed igdb_id field
    """
    import uuid
    import time
    
    # Generate unique values if not provided
    if igdb_id is None:
        # Use timestamp + random component for uniqueness
        igdb_id = f"test-{int(time.time() * 1000)}-{str(uuid.uuid4())[:8]}"
    
    if title is None:
        title = f"Test Game {igdb_id[-8:]}"
    
    if description is None:
        description = f"Test description for {title}"
    
    # Set default values and allow override with kwargs
    game_data = {
        "title": title,
        "description": description,
        "igdb_id": igdb_id,
        "genre": "Action",
        "developer": "Test Developer",
        "publisher": "Test Publisher",
        **kwargs
    }
    
    return Game(**game_data)


def create_test_games(
    count: int,
    session: Session = None,
    commit: bool = True,
    **kwargs
) -> list[Game]:
    """Create multiple test games with unique IGDB IDs.
    
    Args:
        count: Number of games to create
        session: Database session (if provided, games will be added to session)
        commit: Whether to commit after adding to session
        **kwargs: Additional fields to set on all Game objects
    
    Returns:
        List of Game objects with guaranteed unique igdb_id fields
    """
    import time
    
    games = []
    base_timestamp = int(time.time() * 1000)
    
    for i in range(count):
        # Generate unique IGDB ID for each game
        igdb_id = f"test-{base_timestamp + i}"
        title = kwargs.get('title', f"Game {i}")
        description = kwargs.get('description', f"Description {i}")
        
        game = create_test_game(
            title=title,
            description=description,
            igdb_id=igdb_id,
            **kwargs
        )
        
        games.append(game)
        
        if session:
            session.add(game)
    
    if session and commit:
        session.commit()
        for game in games:
            session.refresh(game)
    
    return games


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
    if response.status_code != status_code:
        print(f"DEBUG: Expected status {status_code}, got {response.status_code}. Response: {response.json()}")
    assert response.status_code == status_code
    data = response.json()
    
    # Debug: Print actual response for failing tests
    if status_code == 422 and "detail" not in data:
        print(f"DEBUG: Unexpected 422 response format: {data}")
    
    # Handle different error formats
    if status_code == 422:
        # FastAPI validation errors use 'detail' field, but custom handlers may use 'error'
        assert "detail" in data or "error" in data
        if error_message:
            # Check both detail and error fields
            if "detail" in data:
                detail_str = str(data["detail"])
                assert error_message in detail_str
            elif "error" in data:
                error_str = str(data["error"])
                assert error_message in error_str
    elif "detail" in data:
        # FastAPI HTTPException errors use 'detail' field
        if error_message:
            assert error_message in data["detail"]
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
        "username": user_data["username"],
        "password": user_data["password"]
    }
    login_response = client.post("/api/auth/login", json=login_data)
    assert login_response.status_code == 200
    
    token = login_response.json()["access_token"]
    return {"Authorization": f"Bearer {token}"}
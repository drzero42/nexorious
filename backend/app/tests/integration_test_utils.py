"""
Shared utilities for integration tests.
Provides common fixtures, helpers, and test client setup.
Uses PostgreSQL via testcontainers for realistic database testing.
"""

import os
import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from typing import Dict, Any, Optional
from unittest.mock import MagicMock
from datetime import date

# Configure testcontainers to use Podman
os.environ.setdefault("DOCKER_HOST", f"unix:///run/user/{os.getuid()}/podman/podman.sock")

from testcontainers.postgres import PostgresContainer

from ..main import app
from ..core.database import get_session, _reset_engine, _set_skip_migrations
import app.core.database as db_module
from ..core.security import create_access_token
from ..models.user import User
from ..models.game import Game
from ..models.platform import Platform, Storefront
from ..models.user_game import UserGame
from ..api.dependencies import get_igdb_service_dependency
from ..services.igdb import IGDBService, GameMetadata
from ..services.logo_service import LogoService


# Module-level PostgreSQL container (shared across all tests in a session)
_postgres_container: Optional[PostgresContainer] = None
_test_engine = None


def get_postgres_container() -> PostgresContainer:
    """Get or create the shared PostgreSQL container."""
    global _postgres_container
    if _postgres_container is None:
        _postgres_container = PostgresContainer(
            image="postgres:16-alpine",
            username="test",
            password="test",
            dbname="testdb",
        )
        _postgres_container.start()
    return _postgres_container


def get_test_engine():
    """Get or create the test database engine."""
    global _test_engine
    if _test_engine is None:
        container = get_postgres_container()
        _test_engine = create_engine(
            container.get_connection_url(),
            echo=False,
            pool_pre_ping=True,
        )
    return _test_engine


@pytest.fixture(scope="session", autouse=True)
def setup_test_database():
    """Set up the test database once per session.

    This fixture injects the testcontainer engine into the database module
    before any tests run, ensuring the app uses the test database instead
    of trying to connect to the configured DATABASE_URL.
    """
    # Reset any existing engine to ensure clean state
    _reset_engine()

    # Skip Alembic migrations in tests (we use SQLModel.metadata.create_all instead)
    _set_skip_migrations(True)

    # Get the testcontainer engine and inject it into the database module
    engine = get_test_engine()
    db_module._engine = engine

    SQLModel.metadata.create_all(engine)
    yield
    # Cleanup happens when container is garbage collected or explicitly stopped


@pytest.fixture(name="session")
def session_fixture(setup_test_database):
    """Create a test database session with transaction rollback."""
    engine = get_test_engine()
    connection = engine.connect()
    transaction = connection.begin()
    session = Session(bind=connection)

    # Begin a nested transaction (savepoint) so that explicit rollback
    # calls in tests don't break the outer transaction
    nested = connection.begin_nested()

    yield session

    # Cleanup - rollback any remaining changes
    session.close()
    if nested.is_active:
        nested.rollback()
    if transaction.is_active:
        transaction.rollback()
    connection.close()


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
    def get_session_override():
        # Use the same session to stay within the transaction
        yield session

    app.dependency_overrides[get_session] = get_session_override
    client = TestClient(app)
    yield client
    app.dependency_overrides.clear()


@pytest.fixture(name="client_with_logo_service")
def client_with_logo_service_fixture(session: Session, test_logo_service: LogoService):
    """Create a test client with the test database session and logo service."""
    def get_session_override():
        # Use the same session to stay within the transaction
        yield session

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
        id=12345,  # Use integer IGDB ID as primary key
        title="Test Game",
        description="A test game for integration testing",
        genre="Action",
        developer="Test Developer",
        publisher="Test Publisher",
        release_date=date(2023, 1, 1),
        cover_art_url="https://example.com/cover.jpg",
          # Make it unverified so regular users can update it
    )
    session.add(game)
    session.commit()
    session.refresh(game)
    return game




@pytest.fixture(name="test_user_game")
def test_user_game_fixture(session: Session, test_user: User, test_game: Game) -> UserGame:
    """Create a test user game in the database.

    Note: ownership_status and acquired_date are now on UserGamePlatform,
    not on UserGame. This fixture creates a UserGame without platforms.
    """
    user_game = UserGame(
        user_id=test_user.id,
        game_id=test_game.id,
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


# IGDB Mock Helper Functions

def _create_game_metadata(igdb_id: int, title: str, genre: str, **overrides: Any) -> GameMetadata:
    """Create consistent GameMetadata for mocks."""
    base_data: Dict[str, Any] = {
        "igdb_id": igdb_id,  # Keep as integer for IGDB ID
        "title": title,
        "igdb_slug": title.lower().replace(" ", "-"),
        "description": f"A {genre.lower()} game",
        "genre": genre,
        "developer": f"{genre} Developer",
        "publisher": f"{genre} Publisher",
        "release_date": "2023-01-01",
        "cover_art_url": f"https://example.com/cover-{igdb_id}.jpg",
        "hastily": 10,
        "normally": 15,
        "completely": 20
    }
    base_data.update(overrides)
    return GameMetadata(**base_data)


def _get_predefined_game_data():
    """Get predefined game data for consistent testing."""
    return {
        12345: {"title": "Test Game", "genre": "Action"},
        100: {"title": "The Legend of Zelda", "genre": "Adventure", "developer": "Nintendo", "publisher": "Nintendo"},
        200: {"title": "Elden Ring", "genre": "RPG", "developer": "FromSoftware", "publisher": "Bandai Namco"},
        300: {"title": "Apex Legends", "genre": "Shooter", "developer": "Respawn", "publisher": "EA"},
        400: {"title": "Cyberpunk 2077", "genre": "RPG", "developer": "CD Projekt RED", "publisher": "CD Projekt"},
        500: {"title": "The Witcher 3", "genre": "RPG", "developer": "CD Projekt RED", "publisher": "CD Projekt"}
    }


@pytest.fixture(name="mock_igdb_service")
def mock_igdb_service_fixture():
    """Create an improved mock IGDB service for testing."""
    mock_service = MagicMock(spec=IGDBService)
    game_data = _get_predefined_game_data()
    
    # Dynamic search that responds to actual query content
    def mock_search_games(query: str, limit: int = 10, **kwargs):
        results = []
        query_lower = query.lower()
        
        # Smart matching based on query content
        if "zelda" in query_lower:
            data = game_data[100].copy()
            data.pop("title", None)  # Remove to avoid duplicate
            data.pop("genre", None)  # Remove to avoid duplicate
            results.append(_create_game_metadata(100, game_data[100]["title"], game_data[100]["genre"], **data))
        elif "elden" in query_lower:
            data = game_data[200].copy()
            data.pop("title", None)
            data.pop("genre", None)
            results.append(_create_game_metadata(200, game_data[200]["title"], game_data[200]["genre"], **data))
        elif "apex" in query_lower:
            data = game_data[300].copy()
            data.pop("title", None)
            data.pop("genre", None)
            results.append(_create_game_metadata(300, game_data[300]["title"], game_data[300]["genre"], **data))
        elif "cyberpunk" in query_lower:
            data = game_data[400].copy()
            data.pop("title", None)
            data.pop("genre", None)
            results.append(_create_game_metadata(400, game_data[400]["title"], game_data[400]["genre"], **data))
        elif "witcher" in query_lower:
            data = game_data[500].copy()
            data.pop("title", None)
            data.pop("genre", None)
            results.append(_create_game_metadata(500, game_data[500]["title"], game_data[500]["genre"], **data))
        else:
            # Default fallback - use query as title if it contains "Test Game", otherwise create generic
            if "test game" in query_lower:
                results.append(_create_game_metadata(12345, query, "Action"))
            else:
                results.append(_create_game_metadata(12345, f"Test Game: {query}", "Action"))
        
        return results[:limit]
    
    mock_service.search_games.side_effect = mock_search_games
    
    # Improved get_game_by_id with better data coverage
    def mock_get_game_by_id(igdb_id: int) -> Optional[GameMetadata]:
        # Return None for very large IDs (simulating game not found)
        if igdb_id >= 99999999:
            return None
        data = game_data.get(igdb_id, game_data[12345]).copy()
        title = data.pop("title")
        genre = data.pop("genre")
        return _create_game_metadata(igdb_id, title, genre, **data)

    mock_service.get_game_by_id.side_effect = mock_get_game_by_id
    
    # Smart refresh_game_metadata that uses existing game data
    def mock_refresh_game_metadata(game: Game) -> GameMetadata:
        if game.id and game.id in game_data:
            data = game_data[game.id].copy()
            title = data.pop("title")
            genre = data.pop("genre")
            return _create_game_metadata(game.id, title, genre, **data)
        else:
            # Return enhanced version of existing game data
            return _create_game_metadata(
                game.id or 12345,
                game.title or "Enhanced Test Game",
                game.genre or "Action",
                description=f"Enhanced description for {game.title or 'test game'}"
            )
    
    mock_service.refresh_game_metadata.side_effect = mock_refresh_game_metadata
    
    # Smart populate_missing_metadata that actually populates missing fields
    def mock_populate_missing_metadata(game: Game) -> GameMetadata:
        igdb_id = game.id or 12345
        data = game_data.get(igdb_id, game_data[12345]).copy()
        
        # Use existing game data as base, fill in missing fields
        title = game.title or data.pop("title")
        genre = game.genre or data.pop("genre") 
        
        return _create_game_metadata(
            igdb_id,
            title,
            genre,
            developer=game.developer or data.get("developer", f"{genre} Developer"),
            publisher=game.publisher or data.get("publisher", f"{genre} Publisher"),
            description=game.description or f"A {genre.lower()} game"
        )
    
    mock_service.populate_missing_metadata.side_effect = mock_populate_missing_metadata
    
    # Dynamic metadata completeness calculation
    def mock_get_metadata_completeness(game: Game):
        essential_fields = ["title", "genre", "developer", "publisher"]
        optional_fields = ["description", "release_date", "cover_art_url", "rating_average"]
        
        filled_essential = sum(1 for field in essential_fields if getattr(game, field, None))
        filled_optional = sum(1 for field in optional_fields if getattr(game, field, None))
        total_filled = filled_essential + filled_optional
        total_fields = len(essential_fields) + len(optional_fields)
        
        missing_essential = [field for field in essential_fields if not getattr(game, field, None)]
        missing_optional = [field for field in optional_fields if not getattr(game, field, None)]
        
        return {
            "completeness_percentage": (total_filled / total_fields) * 100,
            "missing_essential": missing_essential,
            "missing_optional": missing_optional,
            "total_fields": total_fields,
            "filled_fields": total_filled
        }
    
    mock_service.get_metadata_completeness.side_effect = mock_get_metadata_completeness
    
    # Mock download_and_store_cover_art with realistic paths
    def mock_download_and_store_cover_art(cover_url: str, game_title: str):
        safe_title = game_title.lower().replace(" ", "_").replace(":", "")
        return f"/local/storage/covers/{safe_title}_cover.jpg"
    
    mock_service.download_and_store_cover_art.side_effect = mock_download_and_store_cover_art
    
    return mock_service


@pytest.fixture(name="configurable_mock_igdb")
def configurable_mock_igdb_fixture():
    """Mock IGDB service with configurable responses for advanced testing."""
    def create_mock(search_results=None, should_raise=None, custom_behavior=None):
        mock_service = MagicMock(spec=IGDBService)
        
        if should_raise:
            for method_name, exception in should_raise.items():
                getattr(mock_service, method_name).side_effect = exception
            return mock_service
            
        if search_results:
            mock_service.search_games.return_value = search_results
        else:
            mock_service.search_games.return_value = [
                _create_game_metadata(999999, "Default Game", "Action")
            ]
        
        if custom_behavior:
            for method_name, behavior in custom_behavior.items():
                if callable(behavior):
                    getattr(mock_service, method_name).side_effect = behavior
                else:
                    getattr(mock_service, method_name).return_value = behavior
        
        return mock_service
    
    return create_mock


@pytest.fixture(name="client_with_mock_igdb")
def client_with_mock_igdb_fixture(session: Session, mock_igdb_service):
    """Create a test client with mocked IGDB service."""
    def get_session_override():
        # Use the same session to stay within the transaction
        yield session

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


# Global counter for unique IGDB ID generation across test runs
_test_game_counter = 300000

def create_test_game(
    title: Optional[str] = None,
    description: Optional[str] = None,
    igdb_id: Optional[int] = None,
    **kwargs: Any
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
    import random
    
    # Generate unique values if not provided
    if igdb_id is None:
        # Use global counter + random component for guaranteed uniqueness
        global _test_game_counter
        _test_game_counter += 1
        # Add small random component to avoid conflicts across test runs
        random_component = random.randint(1, 999)
        igdb_id = _test_game_counter * 1000 + random_component
    
    if title is None:
        title = f"Test Game {igdb_id}"
    
    if description is None:
        description = f"Test description for {title}"
    
    # Set default values and allow override with kwargs
    game_data = {
        "id": igdb_id,  # Use IGDB ID as primary key
        "title": title,
        "description": description,
        "genre": "Action",
        "developer": "Test Developer",
        "publisher": "Test Publisher",
        **kwargs
    }
    
    return Game(**game_data)


def create_test_games(
    count: int,
    session: Optional[Session] = None,
    commit: bool = True,
    **kwargs: Any
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
    games = []
    global _test_game_counter
    
    for i in range(count):
        # Generate unique IGDB ID for each game using global counter
        _test_game_counter += 1
        igdb_id = _test_game_counter * 1000 + i + 1  # Ensure uniqueness within batch
        title = kwargs.get('title', f"Game {i+1}")
        description = kwargs.get('description', f"Description {i+1}")
        
        game_kwargs = {k: v for k, v in kwargs.items() if k not in ['title', 'description']}
        game = create_test_game(
            title=title,
            description=description,
            igdb_id=igdb_id,
            **game_kwargs
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
    game_id: int,  # Changed to integer for IGDB ID
    personal_rating: Optional[float] = None,
    is_loved: bool = False,
    play_status: str = "not_started",
    hours_played: int = 0,
    personal_notes: str = ""
) -> Dict[str, Any]:
    """Create test user game data.

    Note: ownership_status and acquired_date are now on platform level,
    not on the user game level. Use platforms list to specify ownership.
    """
    data: Dict[str, Any] = {
        "game_id": game_id,
        "is_loved": is_loved,
        "play_status": play_status,
        "hours_played": hours_played,
        "personal_notes": personal_notes
    }
    if personal_rating is not None:
        data["personal_rating"] = personal_rating
    return data


def assert_api_error(response: Any, status_code: int, error_message: Optional[str] = None) -> None:
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
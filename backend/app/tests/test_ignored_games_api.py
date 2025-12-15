"""
Tests for the ignored games API endpoints in sync.py.

Covers listing ignored games with filters/pagination and
removing games from the ignored list.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from datetime import datetime, timezone, timedelta
import uuid

from app.models.ignored_external_game import IgnoredExternalGame
from app.models.job import BackgroundJobSource
from app.models.user import User, UserSession
from app.core.security import create_access_token, hash_token


@pytest.fixture
def auth_headers(test_user: User, session: Session) -> dict[str, str]:
    """Create authentication headers for test user."""
    access_token = create_access_token(data={"sub": test_user.id})

    # Create session record for the token
    session_record = UserSession(
        id=str(uuid.uuid4()),
        user_id=test_user.id,
        token_hash=hash_token(access_token),
        refresh_token_hash=hash_token("test_refresh_token"),
        expires_at=datetime.now(timezone.utc) + timedelta(days=30),
        user_agent="test-client",
        ip_address="127.0.0.1",
    )
    session.add(session_record)
    session.commit()

    return {"Authorization": f"Bearer {access_token}"}


@pytest.fixture
def test_ignored_games(session: Session, test_user: User) -> list[IgnoredExternalGame]:
    """Create test ignored games for different sources."""
    ignored_games = [
        IgnoredExternalGame(
            user_id=test_user.id,
            source=BackgroundJobSource.STEAM,
            external_id="123456",
            title="Test Steam Game",
            created_at=datetime.now(timezone.utc),
        ),
        IgnoredExternalGame(
            user_id=test_user.id,
            source=BackgroundJobSource.EPIC,
            external_id="epic-game-id",
            title="Test Epic Game",
            created_at=datetime.now(timezone.utc),
        ),
        IgnoredExternalGame(
            user_id=test_user.id,
            source=BackgroundJobSource.GOG,
            external_id="gog-game-123",
            title="Test GOG Game",
            created_at=datetime.now(timezone.utc),
        ),
    ]

    for game in ignored_games:
        session.add(game)

    session.commit()

    for game in ignored_games:
        session.refresh(game)

    return ignored_games


def test_list_ignored_games_empty(client: TestClient, auth_headers: dict[str, str]) -> None:
    """Test listing ignored games when there are none."""
    response = client.get("/api/sync/ignored", headers=auth_headers)

    assert response.status_code == 200
    data = response.json()
    assert "items" in data
    assert "total" in data
    assert data["items"] == []
    assert data["total"] == 0


def test_list_ignored_games_with_items(
    client: TestClient,
    auth_headers: dict[str, str],
    test_ignored_games: list[IgnoredExternalGame],
) -> None:
    """Test listing ignored games when items exist."""
    response = client.get("/api/sync/ignored", headers=auth_headers)

    assert response.status_code == 200
    data = response.json()
    assert data["total"] == 3
    assert len(data["items"]) == 3

    # Verify structure of first item
    first_item = data["items"][0]
    assert "id" in first_item
    assert "source" in first_item
    assert "external_id" in first_item
    assert "title" in first_item
    assert "created_at" in first_item


def test_list_ignored_games_filter_by_source(
    client: TestClient,
    auth_headers: dict[str, str],
    test_ignored_games: list[IgnoredExternalGame],
) -> None:
    """Test filtering ignored games by source."""
    response = client.get(
        "/api/sync/ignored",
        headers=auth_headers,
        params={"source": "steam"},
    )

    assert response.status_code == 200
    data = response.json()
    assert data["total"] == 1
    assert len(data["items"]) == 1
    assert data["items"][0]["source"] == "steam"
    assert data["items"][0]["title"] == "Test Steam Game"


def test_list_ignored_games_pagination(
    client: TestClient,
    auth_headers: dict[str, str],
    test_ignored_games: list[IgnoredExternalGame],
) -> None:
    """Test pagination of ignored games list."""
    # Get first page
    response = client.get(
        "/api/sync/ignored",
        headers=auth_headers,
        params={"limit": 2, "offset": 0},
    )

    assert response.status_code == 200
    data = response.json()
    assert data["total"] == 3
    assert len(data["items"]) == 2

    # Get second page
    response = client.get(
        "/api/sync/ignored",
        headers=auth_headers,
        params={"limit": 2, "offset": 2},
    )

    assert response.status_code == 200
    data = response.json()
    assert data["total"] == 3
    assert len(data["items"]) == 1


def test_unignore_game_success(
    client: TestClient,
    session: Session,
    auth_headers: dict[str, str],
    test_ignored_games: list[IgnoredExternalGame],
) -> None:
    """Test successfully removing a game from ignored list."""
    game_to_remove = test_ignored_games[0]

    response = client.delete(
        f"/api/sync/ignored/{game_to_remove.id}",
        headers=auth_headers,
    )

    assert response.status_code == 200
    data = response.json()
    assert data["success"] is True
    assert "message" in data
    assert game_to_remove.title in data["message"]

    # Verify the game was actually deleted from the database
    stmt = select(IgnoredExternalGame).where(
        IgnoredExternalGame.id == game_to_remove.id
    )
    deleted_game = session.exec(stmt).first()
    assert deleted_game is None

    # Verify the other games still exist
    stmt = select(IgnoredExternalGame)
    remaining_games = session.exec(stmt).all()
    assert len(remaining_games) == 2


def test_unignore_game_not_found(
    client: TestClient,
    auth_headers: dict[str, str],
) -> None:
    """Test unignoring a game that doesn't exist."""
    fake_id = "00000000-0000-0000-0000-000000000000"

    response = client.delete(
        f"/api/sync/ignored/{fake_id}",
        headers=auth_headers,
    )

    assert response.status_code == 404
    data = response.json()
    # FastAPI uses 'error' field from custom exception handler
    assert "error" in data or "detail" in data
    error_msg = data.get("error") or data.get("detail")
    assert fake_id in error_msg


def test_unignore_game_wrong_user(
    client: TestClient,
    session: Session,
    auth_headers: dict[str, str],
    test_user: User,
) -> None:
    """Test that users can't unignore games from other users."""
    # Create another user
    other_user = User(
        username="otheruser",
        password_hash="fakehash",
    )
    session.add(other_user)
    session.commit()
    session.refresh(other_user)

    # Create an ignored game for the other user
    other_game = IgnoredExternalGame(
        user_id=other_user.id,
        source=BackgroundJobSource.STEAM,
        external_id="999999",
        title="Other User's Game",
    )
    session.add(other_game)
    session.commit()
    session.refresh(other_game)

    # Try to delete the other user's ignored game
    response = client.delete(
        f"/api/sync/ignored/{other_game.id}",
        headers=auth_headers,
    )

    assert response.status_code == 404
    data = response.json()
    # FastAPI uses 'error' field from custom exception handler
    assert "error" in data or "detail" in data

    # Verify the game still exists
    stmt = select(IgnoredExternalGame).where(
        IgnoredExternalGame.id == other_game.id
    )
    still_exists = session.exec(stmt).first()
    assert still_exists is not None


def test_list_ignored_games_unauthorized(client: TestClient) -> None:
    """Test that listing ignored games requires authentication."""
    response = client.get("/api/sync/ignored")
    assert response.status_code == 403


def test_unignore_game_unauthorized(client: TestClient) -> None:
    """Test that unignoring games requires authentication."""
    fake_id = "00000000-0000-0000-0000-000000000000"
    response = client.delete(f"/api/sync/ignored/{fake_id}")
    assert response.status_code == 403

"""
Tests for the ignored games API endpoints in sync.py.

Covers listing ignored games with filters/pagination and
removing games from the ignored list.

Note: "Ignored games" are now represented as ExternalGame records
with is_skipped=True, rather than a separate IgnoredExternalGame model.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from datetime import datetime, timezone, timedelta
import uuid

from app.models.external_game import ExternalGame
from app.models.job import BackgroundJobSource
from app.models.platform import Storefront
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
def test_storefronts_for_skipped_games(session: Session) -> dict[str, Storefront]:
    """Create test storefronts for skipped games tests."""
    storefronts = {}
    for name, display_name in [("steam", "Steam"), ("epic", "Epic Games"), ("gog", "GOG")]:
        storefront = Storefront(
            name=name,
            display_name=display_name,
            is_active=True,
        )
        session.add(storefront)
        storefronts[name] = storefront
    session.commit()
    for sf in storefronts.values():
        session.refresh(sf)
    return storefronts


@pytest.fixture
def test_skipped_games(
    session: Session, test_user: User, test_storefronts_for_skipped_games: dict[str, Storefront]
) -> list[ExternalGame]:
    """Create test skipped (ignored) games for different storefronts."""
    skipped_games = [
        ExternalGame(
            user_id=test_user.id,
            storefront="steam",
            external_id="123456",
            title="Test Steam Game",
            is_skipped=True,
            created_at=datetime.now(timezone.utc),
        ),
        ExternalGame(
            user_id=test_user.id,
            storefront="epic",
            external_id="epic-game-id",
            title="Test Epic Game",
            is_skipped=True,
            created_at=datetime.now(timezone.utc),
        ),
        ExternalGame(
            user_id=test_user.id,
            storefront="gog",
            external_id="gog-game-123",
            title="Test GOG Game",
            is_skipped=True,
            created_at=datetime.now(timezone.utc),
        ),
    ]

    for game in skipped_games:
        session.add(game)

    session.commit()

    for game in skipped_games:
        session.refresh(game)

    return skipped_games


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
    test_skipped_games: list[ExternalGame],
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
    test_skipped_games: list[ExternalGame],
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
    test_skipped_games: list[ExternalGame],
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
    test_skipped_games: list[ExternalGame],
) -> None:
    """Test successfully removing a game from ignored list (clearing is_skipped)."""
    game_to_unignore = test_skipped_games[0]

    response = client.delete(
        f"/api/sync/ignored/{game_to_unignore.id}",
        headers=auth_headers,
    )

    assert response.status_code == 200
    data = response.json()
    assert data["success"] is True
    assert "message" in data
    assert game_to_unignore.title in data["message"]

    # Verify the game's is_skipped flag was cleared (not deleted)
    session.expire_all()
    stmt = select(ExternalGame).where(ExternalGame.id == game_to_unignore.id)
    unignored_game = session.exec(stmt).first()
    assert unignored_game is not None
    assert unignored_game.is_skipped is False

    # Verify the other games still have is_skipped=True
    stmt = select(ExternalGame).where(ExternalGame.is_skipped == True)  # noqa: E712
    remaining_skipped = session.exec(stmt).all()
    assert len(remaining_skipped) == 2


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
    test_storefronts_for_skipped_games: dict[str, Storefront],
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

    # Create a skipped game for the other user
    other_game = ExternalGame(
        user_id=other_user.id,
        storefront="steam",
        external_id="999999",
        title="Other User's Game",
        is_skipped=True,
    )
    session.add(other_game)
    session.commit()
    session.refresh(other_game)

    # Try to unignore the other user's skipped game
    response = client.delete(
        f"/api/sync/ignored/{other_game.id}",
        headers=auth_headers,
    )

    assert response.status_code == 404
    data = response.json()
    # FastAPI uses 'error' field from custom exception handler
    assert "error" in data or "detail" in data

    # Verify the game still has is_skipped=True
    session.expire_all()
    stmt = select(ExternalGame).where(ExternalGame.id == other_game.id)
    still_skipped = session.exec(stmt).first()
    assert still_skipped is not None
    assert still_skipped.is_skipped is True


def test_list_ignored_games_unauthorized(client: TestClient) -> None:
    """Test that listing ignored games requires authentication."""
    response = client.get("/api/sync/ignored")
    assert response.status_code == 403


def test_unignore_game_unauthorized(client: TestClient) -> None:
    """Test that unignoring games requires authentication."""
    fake_id = "00000000-0000-0000-0000-000000000000"
    response = client.delete(f"/api/sync/ignored/{fake_id}")
    assert response.status_code == 403

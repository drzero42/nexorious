"""Tests for export API endpoints."""

import pytest
from datetime import datetime, timezone, timedelta
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch
import tempfile
import json

from fastapi.testclient import TestClient
from sqlmodel import Session

from app.models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus
from app.models.game import Game
from app.models.user_game import UserGame, OwnershipStatus, PlayStatus


@pytest.fixture(autouse=True)
def mock_export_task():
    """Mock the export task to prevent actual task enqueuing during tests."""
    mock_result = MagicMock()
    mock_result.task_id = "test-export-task-id"

    with patch(
        "app.api.export_endpoints.export_collection_task.kiq",
        new_callable=AsyncMock,
        return_value=mock_result,
    ):
        yield


@pytest.fixture
def game_in_collection(session: Session, test_user, test_game) -> UserGame:
    """Create a user game in the collection."""
    user_game = UserGame(
        user_id=test_user.id,
        game_id=test_game.id,
        ownership_status=OwnershipStatus.OWNED,
        play_status=PlayStatus.COMPLETED,
        personal_rating=4.5,
        hours_played=50,
        personal_notes="Great game!",
    )
    session.add(user_game)
    session.commit()
    session.refresh(user_game)
    return user_game


@pytest.fixture
def multiple_games_in_collection(session: Session, test_user) -> list[UserGame]:
    """Create multiple games in user's collection."""
    games = []
    for i in range(3):
        game = Game(
            id=1000 + i,
            title=f"Test Game {i}",
            slug=f"test-game-{i}",
        )
        session.add(game)
        session.commit()

        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status=OwnershipStatus.OWNED,
            play_status=PlayStatus.COMPLETED if i == 0 else PlayStatus.IN_PROGRESS,
            personal_rating=4.0 + (i * 0.5) if i < 2 else None,
            hours_played=10 * (i + 1),
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)
        games.append(user_game)

    return games


@pytest.fixture
def wishlist_games(session: Session, test_user) -> list[UserGame]:
    """Create wishlist games (no_longer_owned status)."""
    games = []
    for i in range(2):
        game = Game(
            id=2000 + i,
            title=f"Wishlist Game {i}",
            slug=f"wishlist-game-{i}",
        )
        session.add(game)
        session.commit()

        user_game = UserGame(
            user_id=test_user.id,
            game_id=game.id,
            ownership_status=OwnershipStatus.NO_LONGER_OWNED,
            play_status=PlayStatus.NOT_STARTED,
        )
        session.add(user_game)
        session.commit()
        session.refresh(user_game)
        games.append(user_game)

    return games


class TestExportCollectionJson:
    """Tests for JSON collection export endpoint."""

    def test_export_collection_json_success(
        self, client: TestClient, auth_headers: dict, session: Session, game_in_collection
    ):
        """POST /export/collection/json creates export job successfully."""
        response = client.post(
            "/api/export/collection/json",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert "job_id" in data
        assert data["status"] == "pending"
        assert data["estimated_items"] == 1
        assert "created" in data["message"].lower()

        # Verify job in database
        job = session.get(Job, data["job_id"])
        assert job is not None
        assert job.job_type == BackgroundJobType.EXPORT
        assert job.source == BackgroundJobSource.NEXORIOUS
        assert job.status == BackgroundJobStatus.PENDING
        assert job.progress_total == 1

        # Verify result summary
        result_summary = job.get_result_summary()
        assert result_summary["format"] == "json"
        assert result_summary["scope"] == "collection"

    def test_export_collection_json_empty_collection(
        self, client: TestClient, auth_headers: dict
    ):
        """POST /export/collection/json returns 400 for empty collection."""
        response = client.post(
            "/api/export/collection/json",
            headers=auth_headers,
        )

        assert response.status_code == 400
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "no games" in error_msg.lower()

    def test_export_collection_json_requires_auth(self, client: TestClient):
        """POST /export/collection/json requires authentication."""
        response = client.post("/api/export/collection/json")
        # FastAPI returns 403 Forbidden when no auth header is provided
        assert response.status_code in (401, 403)


class TestExportCollectionCsv:
    """Tests for CSV collection export endpoint."""

    def test_export_collection_csv_success(
        self, client: TestClient, auth_headers: dict, session: Session, game_in_collection
    ):
        """POST /export/collection/csv creates export job successfully."""
        response = client.post(
            "/api/export/collection/csv",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert "job_id" in data
        assert data["status"] == "pending"
        assert data["estimated_items"] == 1

        # Verify result summary has CSV format
        job = session.get(Job, data["job_id"])
        assert job is not None
        result_summary = job.get_result_summary()
        assert result_summary["format"] == "csv"
        assert result_summary["scope"] == "collection"

    def test_export_collection_csv_empty_collection(
        self, client: TestClient, auth_headers: dict
    ):
        """POST /export/collection/csv returns 400 for empty collection."""
        response = client.post(
            "/api/export/collection/csv",
            headers=auth_headers,
        )

        assert response.status_code == 400


class TestExportWishlistJson:
    """Tests for JSON wishlist export endpoint."""

    def test_export_wishlist_json_success(
        self, client: TestClient, auth_headers: dict, session: Session, wishlist_games
    ):
        """POST /export/wishlist/json creates export job successfully."""
        response = client.post(
            "/api/export/wishlist/json",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert "job_id" in data
        assert data["status"] == "pending"
        assert data["estimated_items"] == 2  # Two wishlist games

        # Verify result summary
        job = session.get(Job, data["job_id"])
        assert job is not None
        result_summary = job.get_result_summary()
        assert result_summary["format"] == "json"
        assert result_summary["scope"] == "wishlist"

    def test_export_wishlist_json_empty(
        self, client: TestClient, auth_headers: dict
    ):
        """POST /export/wishlist/json returns 400 for empty wishlist."""
        response = client.post(
            "/api/export/wishlist/json",
            headers=auth_headers,
        )

        assert response.status_code == 400
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "wishlist" in error_msg.lower() or "no" in error_msg.lower()


class TestExportWishlistCsv:
    """Tests for CSV wishlist export endpoint."""

    def test_export_wishlist_csv_success(
        self, client: TestClient, auth_headers: dict, session: Session, wishlist_games
    ):
        """POST /export/wishlist/csv creates export job successfully."""
        response = client.post(
            "/api/export/wishlist/csv",
            headers=auth_headers,
        )

        assert response.status_code == 200
        data = response.json()

        assert "job_id" in data
        assert data["estimated_items"] == 2

        job = session.get(Job, data["job_id"])
        assert job is not None
        result_summary = job.get_result_summary()
        assert result_summary["format"] == "csv"
        assert result_summary["scope"] == "wishlist"


class TestExportDownload:
    """Tests for export download endpoint."""

    def test_download_export_success(
        self,
        client_with_shared_session: TestClient,
        auth_headers: dict,
        session: Session,
        test_user,
    ):
        """GET /export/{job_id}/download returns file for completed job."""
        # Create a completed export job with a real file
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".json", delete=False
        ) as f:
            json.dump({"test": "data"}, f)
            temp_file_path = f.name

        try:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.EXPORT,
                source=BackgroundJobSource.NEXORIOUS,
                status=BackgroundJobStatus.COMPLETED,
                file_path=temp_file_path,
                completed_at=datetime.now(timezone.utc),
            )
            job.set_result_summary({"format": "json", "scope": "collection"})
            session.add(job)
            session.commit()
            session.refresh(job)

            response = client_with_shared_session.get(
                f"/api/export/{job.id}/download",
                headers=auth_headers,
            )

            assert response.status_code == 200
            assert "application/json" in response.headers.get("content-type", "")
            assert "nexorious_collection_" in response.headers.get(
                "content-disposition", ""
            )
        finally:
            Path(temp_file_path).unlink(missing_ok=True)

    def test_download_export_not_found(
        self, client: TestClient, auth_headers: dict
    ):
        """GET /export/{job_id}/download returns 404 for non-existent job."""
        response = client.get(
            "/api/export/non-existent-job/download",
            headers=auth_headers,
        )

        assert response.status_code == 404

    def test_download_export_not_owned(
        self,
        client_with_shared_session: TestClient,
        auth_headers: dict,
        session: Session,
        admin_user,
    ):
        """GET /export/{job_id}/download returns 404 for another user's job."""
        # Create a job owned by the admin user (different from test_user)
        job = Job(
            user_id=admin_user.id,  # Different user
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
            file_path="/tmp/test.json",
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client_with_shared_session.get(
            f"/api/export/{job.id}/download",
            headers=auth_headers,  # Logged in as test_user
        )

        assert response.status_code == 404

    def test_download_export_not_completed(
        self,
        client_with_shared_session: TestClient,
        auth_headers: dict,
        session: Session,
        test_user,
    ):
        """GET /export/{job_id}/download returns 400 for incomplete job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client_with_shared_session.get(
            f"/api/export/{job.id}/download",
            headers=auth_headers,
        )

        assert response.status_code == 400
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "not ready" in error_msg.lower() or "processing" in error_msg.lower()

    def test_download_export_not_export_job(
        self,
        client_with_shared_session: TestClient,
        auth_headers: dict,
        session: Session,
        test_user,
    ):
        """GET /export/{job_id}/download returns 400 for non-export job."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,  # Not an export job
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client_with_shared_session.get(
            f"/api/export/{job.id}/download",
            headers=auth_headers,
        )

        assert response.status_code == 400
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "not an export" in error_msg.lower()

    def test_download_export_file_missing(
        self,
        client_with_shared_session: TestClient,
        auth_headers: dict,
        session: Session,
        test_user,
    ):
        """GET /export/{job_id}/download returns 410 if file is deleted."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.EXPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.COMPLETED,
            file_path="/nonexistent/path/export.json",
            completed_at=datetime.now(timezone.utc),
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client_with_shared_session.get(
            f"/api/export/{job.id}/download",
            headers=auth_headers,
        )

        assert response.status_code == 410
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "expired" in error_msg.lower() or "deleted" in error_msg.lower()

    def test_download_export_expired(
        self,
        client_with_shared_session: TestClient,
        auth_headers: dict,
        session: Session,
        test_user,
    ):
        """GET /export/{job_id}/download returns 410 for expired export."""
        # Create a file but with an old completion time
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".json", delete=False
        ) as f:
            json.dump({"test": "data"}, f)
            temp_file_path = f.name

        try:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.EXPORT,
                source=BackgroundJobSource.NEXORIOUS,
                status=BackgroundJobStatus.COMPLETED,
                file_path=temp_file_path,
                completed_at=datetime.now(timezone.utc) - timedelta(hours=48),  # 48 hours ago
            )
            job.set_result_summary({"format": "json", "scope": "collection"})
            session.add(job)
            session.commit()
            session.refresh(job)

            response = client_with_shared_session.get(
                f"/api/export/{job.id}/download",
                headers=auth_headers,
            )

            assert response.status_code == 410
            data = response.json()
            error_msg = data.get("error", data.get("detail", ""))
            assert "expired" in error_msg.lower()
        finally:
            # Clean up - the endpoint should have deleted the file
            Path(temp_file_path).unlink(missing_ok=True)

    def test_download_export_csv_content_type(
        self,
        client_with_shared_session: TestClient,
        auth_headers: dict,
        session: Session,
        test_user,
    ):
        """GET /export/{job_id}/download returns correct content type for CSV."""
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".csv", delete=False
        ) as f:
            f.write("col1,col2\nval1,val2")
            temp_file_path = f.name

        try:
            job = Job(
                user_id=test_user.id,
                job_type=BackgroundJobType.EXPORT,
                source=BackgroundJobSource.NEXORIOUS,
                status=BackgroundJobStatus.COMPLETED,
                file_path=temp_file_path,
                completed_at=datetime.now(timezone.utc),
            )
            job.set_result_summary({"format": "csv", "scope": "collection"})
            session.add(job)
            session.commit()
            session.refresh(job)

            response = client_with_shared_session.get(
                f"/api/export/{job.id}/download",
                headers=auth_headers,
            )

            assert response.status_code == 200
            assert "text/csv" in response.headers.get("content-type", "")
            assert ".csv" in response.headers.get("content-disposition", "")
        finally:
            Path(temp_file_path).unlink(missing_ok=True)

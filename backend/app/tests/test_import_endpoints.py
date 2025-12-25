"""Tests for import job creation endpoints."""

import pytest
import json
import io
from unittest.mock import AsyncMock, MagicMock, patch

from fastapi.testclient import TestClient
from sqlmodel import Session

from app.models.job import Job, JobItem, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus


@pytest.fixture(autouse=True)
def mock_task_queue():
    """Mock the task queue to prevent actual task enqueuing during tests."""
    with patch(
        "app.api.import_endpoints.enqueue_import_task",
        new_callable=AsyncMock,
    ):
        yield


class TestNexoriousImport:
    """Tests for Nexorious JSON import endpoint."""

    def test_import_nexorious_success(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """POST /import/nexorious creates import job with valid JSON."""
        # Create a valid Nexorious export JSON
        export_data = {
            "export_version": "1.0",
            "export_date": "2024-01-15T10:30:00Z",
            "games": [
                {
                    "igdb_id": 1942,
                    "title": "The Witcher 3: Wild Hunt",
                    "play_status": "completed",
                    "rating": 9.5,
                },
                {
                    "igdb_id": 119133,
                    "title": "Elden Ring",
                    "play_status": "playing",
                },
            ]
        }
        json_content = json.dumps(export_data).encode("utf-8")

        response = client.post(
            "/api/import/nexorious",
            headers=auth_headers,
            files={"file": ("export.json", io.BytesIO(json_content), "application/json")},
        )

        assert response.status_code == 200
        data = response.json()

        assert "job_id" in data
        assert data["source"] == "nexorious"
        assert data["status"] == "pending"
        assert data["total_items"] == 2
        assert "Processing 2 games" in data["message"]

        # Verify job in database
        from sqlmodel import select
        stmt = select(Job).where(Job.id == data["job_id"])
        job = session.exec(stmt).first()
        assert job is not None
        assert job.job_type == BackgroundJobType.IMPORT
        assert job.source == BackgroundJobSource.NEXORIOUS
        assert job.status == BackgroundJobStatus.PENDING
        assert job.total_items == 2

        # Verify JobItems were created
        stmt = select(JobItem).where(JobItem.job_id == data["job_id"])
        job_items = session.exec(stmt).all()
        assert len(job_items) == 2
        assert job_items[0].source_title == "The Witcher 3: Wild Hunt"
        assert job_items[1].source_title == "Elden Ring"

    def test_import_nexorious_invalid_json(
        self, client: TestClient, auth_headers: dict
    ):
        """POST /import/nexorious returns 400 for invalid JSON."""
        response = client.post(
            "/api/import/nexorious",
            headers=auth_headers,
            files={"file": ("export.json", io.BytesIO(b"not valid json"), "application/json")},
        )

        assert response.status_code == 400
        data = response.json()
        assert "Invalid JSON file" in data.get("error", data.get("detail", ""))

    def test_import_nexorious_missing_games_field(
        self, client: TestClient, auth_headers: dict
    ):
        """POST /import/nexorious returns 400 if games field missing."""
        export_data = {"export_version": "1.0"}
        json_content = json.dumps(export_data).encode("utf-8")

        response = client.post(
            "/api/import/nexorious",
            headers=auth_headers,
            files={"file": ("export.json", io.BytesIO(json_content), "application/json")},
        )

        assert response.status_code == 400
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "games" in error_msg.lower()

    def test_import_nexorious_empty_games_list(
        self, client: TestClient, auth_headers: dict
    ):
        """POST /import/nexorious returns 400 for empty games list."""
        export_data = {"games": []}
        json_content = json.dumps(export_data).encode("utf-8")

        response = client.post(
            "/api/import/nexorious",
            headers=auth_headers,
            files={"file": ("export.json", io.BytesIO(json_content), "application/json")},
        )

        assert response.status_code == 400
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "No games found" in error_msg

    def test_import_nexorious_conflict_when_active(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """POST /import/nexorious returns 409 if import already in progress."""
        # Create an active import job
        existing_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
            priority="high",
        )
        session.add(existing_job)
        session.commit()

        # Try to create another import
        export_data = {"games": [{"igdb_id": 123, "title": "Test"}]}
        json_content = json.dumps(export_data).encode("utf-8")

        response = client.post(
            "/api/import/nexorious",
            headers=auth_headers,
            files={"file": ("export.json", io.BytesIO(json_content), "application/json")},
        )

        assert response.status_code == 409
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "already in progress" in error_msg


    def test_import_nexorious_enqueues_items(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """POST /import/nexorious creates JobItems and enqueues tasks for each."""
        # Create a valid Nexorious export JSON
        export_data = {
            "export_version": "1.0",
            "export_date": "2024-01-15T10:30:00Z",
            "games": [
                {
                    "igdb_id": 1942,
                    "title": "The Witcher 3: Wild Hunt",
                    "play_status": "completed",
                },
                {
                    "igdb_id": 119133,
                    "title": "Elden Ring",
                    "play_status": "playing",
                },
            ]
        }
        json_content = json.dumps(export_data).encode("utf-8")

        with patch(
            "app.api.import_endpoints.enqueue_import_task",
            new_callable=AsyncMock,
        ) as mock_enqueue:
            response = client.post(
                "/api/import/nexorious",
                headers=auth_headers,
                files={"file": ("export.json", io.BytesIO(json_content), "application/json")},
            )

            # Verify response
            assert response.status_code == 200
            data = response.json()
            assert "job_id" in data
            assert data["source"] == "nexorious"
            assert data["total_items"] == 2

            # Verify enqueue_import_task was called twice (once per game)
            assert mock_enqueue.call_count == 2

            # Verify JobItems were created
            from sqlmodel import select
            stmt = select(JobItem).where(JobItem.job_id == data["job_id"])
            job_items = session.exec(stmt).all()
            assert len(job_items) == 2


class TestImportEndpointAuthentication:
    """Tests for import endpoint authentication."""

    def test_nexorious_import_requires_auth(self, client: TestClient):
        """POST /import/nexorious returns 403 without auth."""
        export_data = {"games": [{"igdb_id": 123, "title": "Test"}]}
        json_content = json.dumps(export_data).encode("utf-8")

        response = client.post(
            "/api/import/nexorious",
            files={"file": ("export.json", io.BytesIO(json_content), "application/json")},
        )

        assert response.status_code == 403


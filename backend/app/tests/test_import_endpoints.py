"""Tests for import job creation endpoints."""

import pytest
import json
import io
from unittest.mock import AsyncMock, MagicMock, patch

from fastapi.testclient import TestClient
from sqlmodel import Session

from app.models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus


@pytest.fixture(autouse=True)
def mock_task_queue():
    """Mock the task queue to prevent actual task enqueuing during tests."""
    mock_result = MagicMock()
    mock_result.task_id = "test-task-id"

    with patch(
        "app.api.import_endpoints.import_nexorious_task.kiq",
        new_callable=AsyncMock,
        return_value=mock_result,
    ), patch(
        "app.api.import_endpoints.import_darkadia_task.kiq",
        new_callable=AsyncMock,
        return_value=mock_result,
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
        assert job.progress_total == 2

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


class TestDarkadiaImport:
    """Tests for Darkadia CSV import endpoint."""

    def test_import_darkadia_success(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """POST /import/darkadia creates import job with valid CSV."""
        csv_content = "Name,Platform,Status\nThe Witcher 3,PC,Completed\nElden Ring,PC,Playing\n"

        response = client.post(
            "/api/import/darkadia",
            headers=auth_headers,
            files={"file": ("export.csv", io.BytesIO(csv_content.encode("utf-8")), "text/csv")},
        )

        assert response.status_code == 200
        data = response.json()

        assert "job_id" in data
        assert data["source"] == "darkadia"
        assert data["status"] == "pending"
        assert data["total_items"] == 2
        assert "Review may be required" in data["message"]

        # Verify job in database
        from sqlmodel import select
        stmt = select(Job).where(Job.id == data["job_id"])
        job = session.exec(stmt).first()
        assert job is not None
        assert job.job_type == BackgroundJobType.IMPORT
        assert job.source == BackgroundJobSource.DARKADIA
        assert job.status == BackgroundJobStatus.PENDING

    def test_import_darkadia_invalid_csv(
        self, client: TestClient, auth_headers: dict
    ):
        """POST /import/darkadia returns 400 for empty CSV."""
        # Empty CSV (no data rows)
        csv_content = ""

        response = client.post(
            "/api/import/darkadia",
            headers=auth_headers,
            files={"file": ("export.csv", io.BytesIO(csv_content.encode("utf-8")), "text/csv")},
        )

        assert response.status_code == 400

    def test_import_darkadia_missing_name_column(
        self, client: TestClient, auth_headers: dict
    ):
        """POST /import/darkadia returns 400 if Name column missing."""
        csv_content = "Platform,Status\nPC,Completed\n"

        response = client.post(
            "/api/import/darkadia",
            headers=auth_headers,
            files={"file": ("export.csv", io.BytesIO(csv_content.encode("utf-8")), "text/csv")},
        )

        assert response.status_code == 400
        data = response.json()
        error_msg = data.get("error", data.get("detail", ""))
        assert "Name" in error_msg

    def test_import_darkadia_conflict_when_active(
        self, client: TestClient, auth_headers: dict, session: Session, test_user
    ):
        """POST /import/darkadia returns 409 if import already in progress."""
        # Create an active import job
        existing_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.AWAITING_REVIEW,
            priority="high",
        )
        session.add(existing_job)
        session.commit()

        # Try to create another import
        csv_content = "Name\nTest Game\n"

        response = client.post(
            "/api/import/darkadia",
            headers=auth_headers,
            files={"file": ("export.csv", io.BytesIO(csv_content.encode("utf-8")), "text/csv")},
        )

        assert response.status_code == 409


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

    def test_darkadia_import_requires_auth(self, client: TestClient):
        """POST /import/darkadia returns 403 without auth."""
        csv_content = "Name\nTest Game\n"

        response = client.post(
            "/api/import/darkadia",
            files={"file": ("export.csv", io.BytesIO(csv_content.encode("utf-8")), "text/csv")},
        )

        assert response.status_code == 403

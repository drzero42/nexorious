"""
Tests for the /api/status endpoint.
"""

from unittest.mock import patch, MagicMock
from fastapi.testclient import TestClient


class TestStatusEndpoint:
    """Tests for the status endpoint."""

    def test_status_endpoint_igdb_configured(self, client: TestClient):
        """Test status endpoint returns igdb_configured=true when credentials are set."""
        mock_settings = MagicMock()
        mock_settings.igdb_client_id = "test_client_id"
        mock_settings.igdb_client_secret = "test_client_secret"

        with patch("app.api.status.settings", mock_settings):
            response = client.get("/api/status")

        assert response.status_code == 200
        data = response.json()
        assert data["igdb_configured"] is True

    def test_status_endpoint_igdb_not_configured_missing_both(self, client: TestClient):
        """Test status endpoint returns igdb_configured=false when both credentials are missing."""
        mock_settings = MagicMock()
        mock_settings.igdb_client_id = None
        mock_settings.igdb_client_secret = None

        with patch("app.api.status.settings", mock_settings):
            response = client.get("/api/status")

        assert response.status_code == 200
        data = response.json()
        assert data["igdb_configured"] is False

    def test_status_endpoint_igdb_not_configured_missing_client_id(self, client: TestClient):
        """Test status endpoint returns igdb_configured=false when client_id is missing."""
        mock_settings = MagicMock()
        mock_settings.igdb_client_id = None
        mock_settings.igdb_client_secret = "test_client_secret"

        with patch("app.api.status.settings", mock_settings):
            response = client.get("/api/status")

        assert response.status_code == 200
        data = response.json()
        assert data["igdb_configured"] is False

    def test_status_endpoint_igdb_not_configured_missing_client_secret(self, client: TestClient):
        """Test status endpoint returns igdb_configured=false when client_secret is missing."""
        mock_settings = MagicMock()
        mock_settings.igdb_client_id = "test_client_id"
        mock_settings.igdb_client_secret = None

        with patch("app.api.status.settings", mock_settings):
            response = client.get("/api/status")

        assert response.status_code == 200
        data = response.json()
        assert data["igdb_configured"] is False

    def test_status_endpoint_igdb_not_configured_empty_strings(self, client: TestClient):
        """Test status endpoint returns igdb_configured=false when credentials are empty strings."""
        mock_settings = MagicMock()
        mock_settings.igdb_client_id = ""
        mock_settings.igdb_client_secret = ""

        with patch("app.api.status.settings", mock_settings):
            response = client.get("/api/status")

        assert response.status_code == 200
        data = response.json()
        assert data["igdb_configured"] is False

    def test_status_endpoint_no_auth_required(self, client: TestClient):
        """Test that status endpoint does not require authentication."""
        mock_settings = MagicMock()
        mock_settings.igdb_client_id = "test_client_id"
        mock_settings.igdb_client_secret = "test_client_secret"

        with patch("app.api.status.settings", mock_settings):
            # Make request without any auth headers
            response = client.get("/api/status")

        # Should succeed without authentication
        assert response.status_code == 200

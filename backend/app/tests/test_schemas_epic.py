"""
Tests for Epic authentication API schemas.

Validates Epic-specific Pydantic schemas for authentication flow endpoints.
"""

from app.schemas.sync import (
    EpicAuthStartResponse,
    EpicAuthCompleteRequest,
    EpicAuthCompleteResponse,
    EpicAuthCheckResponse,
)


def test_epic_auth_start_response():
    """Test EpicAuthStartResponse with auth_url and instructions."""
    response = EpicAuthStartResponse(
        auth_url="https://www.epicgames.com/id/authorize?client_id=xyz",
        instructions="Visit the URL and paste the code parameter from the redirect",
    )

    assert response.auth_url == "https://www.epicgames.com/id/authorize?client_id=xyz"
    assert response.instructions == "Visit the URL and paste the code parameter from the redirect"

    # Test model_dump
    data = response.model_dump()
    assert data["auth_url"] == "https://www.epicgames.com/id/authorize?client_id=xyz"
    assert data["instructions"] == "Visit the URL and paste the code parameter from the redirect"


def test_epic_auth_complete_request():
    """Test EpicAuthCompleteRequest with code."""
    request = EpicAuthCompleteRequest(code="abc123xyz")

    assert request.code == "abc123xyz"

    # Test model_dump
    data = request.model_dump()
    assert data["code"] == "abc123xyz"


def test_epic_auth_complete_response_success():
    """Test success case (valid=True, display_name)."""
    response = EpicAuthCompleteResponse(
        valid=True,
        display_name="EpicGamer123",
        error=None,
    )

    assert response.valid is True
    assert response.display_name == "EpicGamer123"
    assert response.error is None

    # Test model_dump
    data = response.model_dump()
    assert data["valid"] is True
    assert data["display_name"] == "EpicGamer123"
    assert data["error"] is None


def test_epic_auth_complete_response_error():
    """Test error case (valid=False, error)."""
    response = EpicAuthCompleteResponse(
        valid=False,
        display_name=None,
        error="invalid_grant",
    )

    assert response.valid is False
    assert response.display_name is None
    assert response.error == "invalid_grant"

    # Test model_dump
    data = response.model_dump()
    assert data["valid"] is False
    assert data["display_name"] is None
    assert data["error"] == "invalid_grant"


def test_epic_auth_check_response():
    """Test EpicAuthCheckResponse (is_authenticated, display_name)."""
    # Test authenticated case
    response_auth = EpicAuthCheckResponse(
        is_authenticated=True,
        display_name="EpicGamer123",
    )

    assert response_auth.is_authenticated is True
    assert response_auth.display_name == "EpicGamer123"

    # Test not authenticated case
    response_no_auth = EpicAuthCheckResponse(
        is_authenticated=False,
        display_name=None,
    )

    assert response_no_auth.is_authenticated is False
    assert response_no_auth.display_name is None

    # Test model_dump
    data = response_auth.model_dump()
    assert data["is_authenticated"] is True
    assert data["display_name"] == "EpicGamer123"

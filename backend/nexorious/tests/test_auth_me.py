"""
Test for the /api/auth/me endpoint.
Tests the complete flow: register user -> login -> use access token to call /api/auth/me
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine
from sqlmodel.pool import StaticPool

from ..main import app
from ..core.database import get_session
from ..models.user import User


# Test database setup
@pytest.fixture(name="session")
def session_fixture():
    """Create a test database session."""
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
    def get_session_override():
        return session

    app.dependency_overrides[get_session] = get_session_override
    client = TestClient(app)
    yield client
    app.dependency_overrides.clear()


def test_auth_me_complete_flow(client: TestClient):
    """Test the complete flow: register -> login -> call /api/auth/me"""
    
    # Step 1: Register a new user
    register_data = {
        "email": "test@example.com",
        "username": "testuser",
        "password": "testpassword123"
    }
    
    register_response = client.post("/api/auth/register", json=register_data)
    assert register_response.status_code == 201
    
    register_result = register_response.json()
    assert register_result["email"] == "test@example.com"
    assert register_result["username"] == "testuser"
    assert "password_hash" not in register_result  # Password should not be in response
    
    # Step 2: Login with the registered user
    login_data = {
        "username": "testuser",
        "password": "testpassword123"
    }
    
    login_response = client.post("/api/auth/login", json=login_data)
    assert login_response.status_code == 200
    
    login_result = login_response.json()
    assert "access_token" in login_result
    assert "refresh_token" in login_result
    assert login_result["token_type"] == "bearer"
    assert "expires_in" in login_result
    
    access_token = login_result["access_token"]
    
    # Step 3: Use the access token to call /api/auth/me
    headers = {"Authorization": f"Bearer {access_token}"}
    me_response = client.get("/api/auth/me", headers=headers)
    assert me_response.status_code == 200
    
    me_result = me_response.json()
    assert me_result["email"] == "test@example.com"
    assert me_result["username"] == "testuser"
    assert me_result["is_active"] is True
    assert me_result["is_admin"] is False
    assert "password_hash" not in me_result  # Password should not be in response


def test_auth_me_without_token(client: TestClient):
    """Test /api/auth/me endpoint without authentication token"""
    response = client.get("/api/auth/me")
    assert response.status_code == 403


def test_auth_me_with_invalid_token(client: TestClient):
    """Test /api/auth/me endpoint with invalid authentication token"""
    headers = {"Authorization": "Bearer invalid_token"}
    response = client.get("/api/auth/me", headers=headers)
    assert response.status_code == 401


def test_auth_me_with_malformed_header(client: TestClient):
    """Test /api/auth/me endpoint with malformed authorization header"""
    headers = {"Authorization": "invalid_header_format"}
    response = client.get("/api/auth/me", headers=headers)
    assert response.status_code == 403
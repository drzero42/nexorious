"""
Test for the /api/auth/register endpoint.
Comprehensive tests for user registration functionality.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, SQLModel, create_engine, select
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


class TestSuccessfulRegistration:
    """Test successful registration scenarios."""
    
    def test_register_with_all_fields(self, client: TestClient, session: Session):
        """Test successful registration with all fields provided."""
        register_data = {
            "email": "test@example.com",
            "username": "testuser",
            "password": "testpassword123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        
        assert response.status_code == 201
        result = response.json()
        
        # Verify response structure
        assert result["email"] == "test@example.com"
        assert result["username"] == "testuser"
        assert result["is_active"] is True
        assert result["is_admin"] is False
        assert "password_hash" not in result
        assert "password" not in result
        assert "id" in result
        assert "created_at" in result
        assert "updated_at" in result
        assert "preferences" in result
        
        # Verify user was created in database
        user = session.exec(select(User).where(User.email == "test@example.com")).first()
        assert user is not None
        assert user.username == "testuser"
        assert user.is_active is True
        assert user.is_admin is False
    
    def test_register_with_minimal_fields(self, client: TestClient, session: Session):
        """Test successful registration with only required fields."""
        register_data = {
            "email": "minimal@example.com",
            "username": "minimaluser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        
        assert response.status_code == 201
        result = response.json()
        
        # Verify response structure
        assert result["email"] == "minimal@example.com"
        assert result["username"] == "minimaluser"
        assert result["is_active"] is True
        assert result["is_admin"] is False
        
        # Verify user was created in database
        user = session.exec(select(User).where(User.email == "minimal@example.com")).first()
        assert user is not None
    
    def test_password_is_hashed(self, client: TestClient, session: Session):
        """Test that password is properly hashed and not stored in plaintext."""
        register_data = {
            "email": "hash@example.com",
            "username": "hashuser",
            "password": "plainpassword123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 201
        
        # Verify password is hashed in database
        user = session.exec(select(User).where(User.email == "hash@example.com")).first()
        assert user is not None
        assert user.password_hash != "plainpassword123"
        assert len(user.password_hash) > 50  # Hashed password should be longer
        assert user.password_hash.startswith("$2b$")  # bcrypt hash format


class TestValidationErrors:
    """Test validation error scenarios."""
    
    def test_invalid_email_format(self, client: TestClient):
        """Test registration with invalid email format."""
        register_data = {
            "email": "not-an-email",
            "username": "testuser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 422
        
        result = response.json()
        assert "detail" in result
        # Should have validation error for email
        assert any("email" in str(error).lower() for error in result["detail"])
    
    def test_username_too_short(self, client: TestClient):
        """Test registration with username too short."""
        register_data = {
            "email": "test@example.com",
            "username": "ab",  # Less than 3 characters
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 422
        
        result = response.json()
        assert "detail" in result
        # Should have validation error for username
        assert any("username" in str(error).lower() for error in result["detail"])
    
    def test_username_too_long(self, client: TestClient):
        """Test registration with username too long."""
        register_data = {
            "email": "test@example.com",
            "username": "a" * 101,  # More than 100 characters
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 422
    
    def test_password_too_short(self, client: TestClient):
        """Test registration with password too short."""
        register_data = {
            "email": "test@example.com",
            "username": "testuser",
            "password": "short"  # Less than 8 characters
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 422
        
        result = response.json()
        assert "detail" in result
        # Should have validation error for password
        assert any("password" in str(error).lower() for error in result["detail"])
    
    def test_password_too_long(self, client: TestClient):
        """Test registration with password too long."""
        register_data = {
            "email": "test@example.com",
            "username": "testuser",
            "password": "a" * 129  # More than 128 characters
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 422
    
    
    def test_missing_required_fields(self, client: TestClient):
        """Test registration with missing required fields."""
        # Missing email
        response = client.post("/api/auth/register", json={
            "username": "testuser",
            "password": "password123"
        })
        assert response.status_code == 422
        
        # Missing username
        response = client.post("/api/auth/register", json={
            "email": "test@example.com",
            "password": "password123"
        })
        assert response.status_code == 422
        
        # Missing password
        response = client.post("/api/auth/register", json={
            "email": "test@example.com",
            "username": "testuser"
        })
        assert response.status_code == 422
    
    def test_empty_required_fields(self, client: TestClient):
        """Test registration with empty required fields."""
        register_data = {
            "email": "",
            "username": "",
            "password": ""
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 422


class TestDuplicatePrevention:
    """Test duplicate email and username prevention."""
    
    def test_duplicate_email_registration(self, client: TestClient):
        """Test registration with duplicate email should fail."""
        # Register first user
        register_data = {
            "email": "duplicate@example.com",
            "username": "firstuser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 201
        
        # Try to register second user with same email
        register_data_duplicate = {
            "email": "duplicate@example.com",
            "username": "seconduser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data_duplicate)
        assert response.status_code == 400
        
        result = response.json()
        assert "error" in result
        assert "email already registered" in result["error"].lower()
    
    def test_duplicate_username_registration(self, client: TestClient):
        """Test registration with duplicate username should fail."""
        # Register first user
        register_data = {
            "email": "first@example.com",
            "username": "duplicateuser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 201
        
        # Try to register second user with same username
        register_data_duplicate = {
            "email": "second@example.com",
            "username": "duplicateuser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data_duplicate)
        assert response.status_code == 400
        
        result = response.json()
        assert "error" in result
        assert "username already taken" in result["error"].lower()
    
    def test_case_sensitivity_email(self, client: TestClient):
        """Test email case sensitivity (currently case sensitive - should be improved)."""
        # Register first user with lowercase email
        register_data = {
            "email": "case@example.com",
            "username": "caseuser1",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 201
        
        # Try to register with uppercase email
        register_data_upper = {
            "email": "CASE@EXAMPLE.COM",
            "username": "caseuser2",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data_upper)
        # NOTE: Current implementation is case sensitive for emails, but this should be changed
        # to be case insensitive in the future to follow RFC standards
        assert response.status_code == 201
    
    def test_case_sensitivity_username(self, client: TestClient):
        """Test username case sensitivity (should be case sensitive)."""
        # Register first user with lowercase username
        register_data = {
            "email": "user1@example.com",
            "username": "caseuser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 201
        
        # Try to register with uppercase username
        register_data_upper = {
            "email": "user2@example.com",
            "username": "CASEUSER",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data_upper)
        # Username should be case sensitive, so this should succeed
        assert response.status_code == 201


class TestErrorHandling:
    """Test error handling scenarios."""
    
    def test_malformed_json(self, client: TestClient):
        """Test registration with malformed JSON."""
        response = client.post(
            "/api/auth/register",
            content='{"email": "test@example.com", "username": "testuser", "password": "password123"',  # Missing closing brace
            headers={"Content-Type": "application/json"}
        )
        assert response.status_code == 422
    
    def test_invalid_field_types(self, client: TestClient):
        """Test registration with invalid field types."""
        register_data = {
            "email": 123,  # Should be string
            "username": True,  # Should be string
            "password": ["password"]  # Should be string
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 422
    
    def test_null_values_for_required_fields(self, client: TestClient):
        """Test registration with null values for required fields."""
        register_data = {
            "email": None,
            "username": None,
            "password": None
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 422


class TestDatabaseIntegration:
    """Test database integration and user defaults."""
    
    def test_user_defaults_are_set(self, client: TestClient, session: Session):
        """Test that user defaults are properly set in database."""
        register_data = {
            "email": "defaults@example.com",
            "username": "defaultuser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 201
        
        # Verify defaults in database
        user = session.exec(select(User).where(User.email == "defaults@example.com")).first()
        assert user is not None
        assert user.is_active is True
        assert user.is_admin is False
        assert user.preferences_json == "{}"
        assert user.created_at is not None
        assert user.updated_at is not None
    
    def test_user_id_is_generated(self, client: TestClient, session: Session):
        """Test that user ID is properly generated."""
        register_data = {
            "email": "uuid@example.com",
            "username": "uuiduser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 201
        
        result = response.json()
        assert "id" in result
        assert isinstance(result["id"], str)
        assert len(result["id"]) == 36  # UUID length
        
        # Verify in database
        user = session.exec(select(User).where(User.email == "uuid@example.com")).first()
        assert user is not None
        assert user.id == result["id"]
    
    def test_timestamps_are_set(self, client: TestClient, session: Session):
        """Test that timestamps are properly set."""
        register_data = {
            "email": "timestamp@example.com",
            "username": "timestampuser",
            "password": "password123"
        }
        
        response = client.post("/api/auth/register", json=register_data)
        assert response.status_code == 201
        
        user = session.exec(select(User).where(User.email == "timestamp@example.com")).first()
        assert user is not None
        assert user.created_at is not None
        assert user.updated_at is not None
        # Both timestamps should be very close (within a second)
        time_diff = abs((user.updated_at - user.created_at).total_seconds())
        assert time_diff < 1.0
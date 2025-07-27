"""
Integration tests for authentication endpoints.
Tests all auth endpoints with proper request/response validation.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from unittest.mock import patch
from datetime import datetime, timedelta, timezone

from ..models.user import User, UserSession
from .integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    auth_headers_fixture as auth_headers,
    create_test_user_data,
    assert_api_error,
    assert_api_success,
    register_and_login_user
)


class TestAuthRegisterEndpoint:
    """Test /api/auth/register endpoint."""
    
    def test_register_success(self, client: TestClient):
        """Test successful user registration."""
        user_data = create_test_user_data()
        response = client.post("/api/auth/register", json=user_data)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["username"] == user_data["username"]
        assert data["is_active"] is True
        assert data["is_admin"] is False
        assert "password_hash" not in data
        
    def test_register_duplicate_username(self, client: TestClient):
        """Test registration with duplicate username."""
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        # Try to register with same username
        duplicate_data = create_test_user_data(username="newuser")
        response = client.post("/api/auth/register", json=duplicate_data)
        
        assert_api_error(response, 400, "Username already taken")
    
    def test_register_missing_fields(self, client: TestClient):
        """Test registration with missing required fields."""
        incomplete_data = {"username": "testuser"}
        response = client.post("/api/auth/register", json=incomplete_data)
        
        assert_api_error(response, 422)


class TestAuthLoginEndpoint:
    """Test /api/auth/login endpoint."""
    
    def test_login_success(self, client: TestClient):
        """Test successful login."""
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        login_data = {
            "username": user_data["username"],
            "password": user_data["password"]
        }
        response = client.post("/api/auth/login", json=login_data)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "access_token" in data
        assert "refresh_token" in data
        assert "expires_in" in data
        assert data["token_type"] == "bearer"
    
    def test_login_invalid_credentials(self, client: TestClient):
        """Test login with invalid credentials."""
        login_data = {
            "username": "nonexistent@example.com",
            "password": "wrongpassword"
        }
        response = client.post("/api/auth/login", json=login_data)
        
        assert_api_error(response, 401, "Incorrect username or password")
    
    def test_login_inactive_user(self, client: TestClient, session: Session):
        """Test login with inactive user."""
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        # Deactivate user
        user = session.exec(select(User).where(User.username == user_data["username"])).first()
        user.is_active = False
        session.add(user)
        session.commit()
        
        login_data = {
            "username": user_data["username"],
            "password": user_data["password"]
        }
        response = client.post("/api/auth/login", json=login_data)
        
        assert_api_error(response, 401, "User account is disabled")
    
    def test_login_missing_fields(self, client: TestClient):
        """Test login with missing fields."""
        incomplete_data = {"username": "test@example.com"}
        response = client.post("/api/auth/login", json=incomplete_data)
        
        assert_api_error(response, 422)


class TestAuthRefreshEndpoint:
    """Test /api/auth/refresh endpoint."""
    
    def test_refresh_success(self, client: TestClient):
        """Test successful token refresh."""
        user_data = create_test_user_data()
        
        # Register and login user
        register_response = client.post("/api/auth/register", json=user_data)
        assert_api_success(register_response, 201)
        
        login_response = client.post("/api/auth/login", json={
            "username": user_data["username"],
            "password": user_data["password"]
        })
        assert_api_success(login_response, 200)
        refresh_token = login_response.json()["refresh_token"]
        
        # Use refresh token
        refresh_data = {"refresh_token": refresh_token}
        response = client.post("/api/auth/refresh", json=refresh_data)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "access_token" in data
        assert "refresh_token" in data
        assert "expires_in" in data
        assert data["token_type"] == "bearer"
    
    def test_refresh_invalid_token(self, client: TestClient):
        """Test refresh with invalid token."""
        refresh_data = {"refresh_token": "invalid-token"}
        response = client.post("/api/auth/refresh", json=refresh_data)
        
        assert_api_error(response, 401, "Invalid refresh token")
    
    def test_refresh_expired_token(self, client: TestClient, session: Session):
        """Test refresh with expired token."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        # Get user and create expired session
        user = session.exec(select(User).where(User.username == user_data["username"])).first()
        expired_session = UserSession(
            user_id=user.id,
            token_hash="expired-token-hash",
            refresh_token_hash="expired-refresh-hash",
            expires_at=datetime.now(timezone.utc) - timedelta(hours=1)
        )
        session.add(expired_session)
        session.commit()
        
        refresh_data = {"refresh_token": "expired-refresh-token"}
        response = client.post("/api/auth/refresh", json=refresh_data)
        
        assert_api_error(response, 401, "Invalid refresh token")
    
    def test_refresh_missing_token(self, client: TestClient):
        """Test refresh with missing token."""
        response = client.post("/api/auth/refresh", json={})
        
        assert_api_error(response, 422)


class TestAuthLogoutEndpoint:
    """Test /api/auth/logout endpoint."""
    
    def test_logout_success(self, client: TestClient):
        """Test successful logout."""
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        # Get refresh token from login
        login_data = {
            "username": user_data["username"],
            "password": user_data["password"]
        }
        login_response = client.post("/api/auth/login", json=login_data)
        refresh_token = login_response.json()["refresh_token"]
        
        logout_data = {"refresh_token": refresh_token}
        response = client.post("/api/auth/logout", json=logout_data)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Successfully logged out"
    
    def test_logout_without_token(self, client: TestClient):
        """Test logout without refresh token."""
        response = client.post("/api/auth/logout", json={})
        
        assert_api_error(response, 422)
    
    def test_logout_invalid_token(self, client: TestClient):
        """Test logout with invalid refresh token."""
        logout_data = {"refresh_token": "invalid-token"}
        response = client.post("/api/auth/logout", json=logout_data)
        
        # Logout always returns 200 for security reasons
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Successfully logged out"
    
    def test_logout_twice(self, client: TestClient):
        """Test logout twice with same refresh token."""
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        # Get refresh token from login
        login_data = {
            "username": user_data["username"],
            "password": user_data["password"]
        }
        login_response = client.post("/api/auth/login", json=login_data)
        refresh_token = login_response.json()["refresh_token"]
        
        # First logout
        logout_data = {"refresh_token": refresh_token}
        response1 = client.post("/api/auth/logout", json=logout_data)
        assert_api_success(response1, 200)
        
        # Second logout with same refresh token
        response2 = client.post("/api/auth/logout", json=logout_data)
        # Logout always returns 200 for security reasons
        assert_api_success(response2, 200)
        data = response2.json()
        assert data["message"] == "Successfully logged out"


class TestAuthMeEndpoint:
    """Test /api/auth/me endpoints."""
    
    def test_get_me_success(self, client: TestClient):
        """Test successful GET /me."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        response = client.get("/api/auth/me", headers=headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "password_hash" not in data
    
    def test_get_me_without_token(self, client: TestClient):
        """Test GET /me without authentication token."""
        response = client.get("/api/auth/me")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_get_me_invalid_token(self, client: TestClient):
        """Test GET /me with invalid token."""
        headers = {"Authorization": "Bearer invalid-token"}
        response = client.get("/api/auth/me", headers=headers)
        
        assert_api_error(response, 401, "Could not validate credentials")
    
    def test_update_me_success(self, client: TestClient):
        """Test successful PUT /me."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        update_data = {
            "preferences": {"theme": "dark", "language": "en"}
        }
        response = client.put("/api/auth/me", json=update_data, headers=headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["preferences"]["theme"] == "dark"
        assert data["preferences"]["language"] == "en"
        assert data["username"] == user_data["username"]  # Should not change
    
    def test_update_me_partial(self, client: TestClient):
        """Test partial update of profile."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        update_data = {"preferences": {"theme": "light"}}
        response = client.put("/api/auth/me", json=update_data, headers=headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["preferences"]["theme"] == "light"
    
    def test_update_me_without_token(self, client: TestClient):
        """Test PUT /me without authentication token."""
        update_data = {"preferences": {"theme": "Updated"}}
        response = client.put("/api/auth/me", json=update_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_update_me_invalid_token(self, client: TestClient):
        """Test PUT /me with invalid token."""
        headers = {"Authorization": "Bearer invalid-token"}
        update_data = {"preferences": {"theme": "Updated"}}
        response = client.put("/api/auth/me", json=update_data, headers=headers)
        
        assert_api_error(response, 401, "Could not validate credentials")


class TestAuthChangePasswordEndpoint:
    """Test /api/auth/change-password endpoint."""
    
    def test_change_password_success(self, client: TestClient):
        """Test successful password change."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        change_data = {
            "current_password": user_data["password"],
            "new_password": "newpassword123"
        }
        response = client.put("/api/auth/change-password", json=change_data, headers=headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Password changed successfully. Please log in again."
        
        # Verify new password works
        login_data = {
            "username": user_data["username"],
            "password": "newpassword123"
        }
        login_response = client.post("/api/auth/login", json=login_data)
        assert_api_success(login_response, 200)
    
    def test_change_password_wrong_current(self, client: TestClient):
        """Test password change with wrong current password."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        change_data = {
            "current_password": "wrongpassword",
            "new_password": "newpassword123"
        }
        response = client.put("/api/auth/change-password", json=change_data, headers=headers)
        
        assert_api_error(response, 400, "Current password is incorrect")
    
    def test_change_password_same_password(self, client: TestClient):
        """Test password change with same password."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        change_data = {
            "current_password": user_data["password"],
            "new_password": user_data["password"]
        }
        response = client.put("/api/auth/change-password", json=change_data, headers=headers)
        
        assert_api_error(response, 400, "New password must be different from current password")
    
    def test_change_password_without_token(self, client: TestClient):
        """Test password change without authentication token."""
        change_data = {
            "current_password": "oldpassword",
            "new_password": "newpassword123"
        }
        response = client.put("/api/auth/change-password", json=change_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_change_password_invalid_token(self, client: TestClient):
        """Test password change with invalid token."""
        headers = {"Authorization": "Bearer invalid-token"}
        change_data = {
            "current_password": "oldpassword",
            "new_password": "newpassword123"
        }
        response = client.put("/api/auth/change-password", json=change_data, headers=headers)
        
        assert_api_error(response, 401, "Could not validate credentials")
    
    def test_change_password_missing_fields(self, client: TestClient):
        """Test password change with missing fields."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        incomplete_data = {"current_password": "oldpassword"}
        response = client.put("/api/auth/change-password", json=incomplete_data, headers=headers)
        
        assert_api_error(response, 422)
    
    def test_change_password_weak_password(self, client: TestClient):
        """Test password change with weak password."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        change_data = {
            "current_password": user_data["password"],
            "new_password": "123"  # Too short
        }
        response = client.put("/api/auth/change-password", json=change_data, headers=headers)
        
        assert_api_error(response, 422)


class TestAuthEndpointSecurity:
    """Test security aspects of auth endpoints."""
    
    def test_register_password_not_returned(self, client: TestClient):
        """Test that password is never returned in responses."""
        user_data = create_test_user_data()
        response = client.post("/api/auth/register", json=user_data)
        
        assert_api_success(response, 201)
        data = response.json()
        assert "password" not in data
        assert "password_hash" not in data
    
    def test_login_password_not_returned(self, client: TestClient):
        """Test that password is never returned in login response."""
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        login_data = {
            "username": user_data["username"],
            "password": user_data["password"]
        }
        response = client.post("/api/auth/login", json=login_data)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "password" not in data
        assert "password_hash" not in data
    
    def test_me_password_not_returned(self, client: TestClient):
        """Test that password is never returned in /me response."""
        user_data = create_test_user_data()
        headers = register_and_login_user(client, user_data)
        
        response = client.get("/api/auth/me", headers=headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "password" not in data
        assert "password_hash" not in data
    
    def test_token_invalidation_on_logout(self, client: TestClient):
        """Test that access token is invalidated after logout."""
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        # Get both tokens from login
        login_data = {
            "username": user_data["username"],
            "password": user_data["password"]
        }
        login_response = client.post("/api/auth/login", json=login_data)
        access_token = login_response.json()["access_token"]
        refresh_token = login_response.json()["refresh_token"]
        headers = {"Authorization": f"Bearer {access_token}"}
        
        # Logout
        logout_data = {"refresh_token": refresh_token}
        logout_response = client.post("/api/auth/logout", json=logout_data)
        assert_api_success(logout_response, 200)
        
        # Try to use access token after logout
        me_response = client.get("/api/auth/me", headers=headers)
        assert_api_error(me_response, 401)


class TestAuthSetupStatus:
    """Test /api/auth/setup/status endpoint."""
    
    def test_setup_status_needs_setup_when_no_users(self, client: TestClient, session: Session):
        """Test setup status returns needs_setup=true when no users exist."""
        # Ensure no users exist in database
        users = session.exec(select(User)).all()
        for user in users:
            session.delete(user)
        session.commit()
        
        response = client.get("/api/auth/setup/status")
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["needs_setup"] is True
    
    def test_setup_status_no_setup_when_users_exist(self, client: TestClient):
        """Test setup status returns needs_setup=false when users exist."""
        # Create a user
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        response = client.get("/api/auth/setup/status")
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["needs_setup"] is False
    
    def test_setup_status_response_schema(self, client: TestClient):
        """Test setup status response has correct schema."""
        # Create a user to ensure consistent state
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        response = client.get("/api/auth/setup/status")
        
        assert_api_success(response, 200)
        data = response.json()
        
        # Verify response schema
        assert "needs_setup" in data
        assert isinstance(data["needs_setup"], bool)
        assert len(data) == 1  # Should only contain needs_setup field


class TestInitialAdminSetup:
    """Test /api/auth/setup/admin endpoint."""
    
    def test_initial_admin_setup_success(self, client: TestClient, session: Session):
        """Test successful initial admin setup when no users exist."""
        # Ensure no users exist in database
        users = session.exec(select(User)).all()
        for user in users:
            session.delete(user)
        session.commit()
        
        admin_data = {
            "username": "admin",
            "password": "admin123"
        }
        response = client.post("/api/auth/setup/admin", json=admin_data)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["username"] == admin_data["username"]
        assert data["is_admin"] is True
        assert data["is_active"] is True
        assert "password_hash" not in data
        
        # Verify user exists in database
        user = session.exec(select(User).where(User.username == admin_data["username"])).first()
        assert user is not None
        assert user.is_admin is True
    
    def test_initial_admin_setup_fails_when_users_exist(self, client: TestClient):
        """Test initial admin setup fails when users already exist."""
        # Create a user first
        user_data = create_test_user_data()
        client.post("/api/auth/register", json=user_data)
        
        admin_data = {
            "username": "admin",
            "password": "admin123"
        }
        response = client.post("/api/auth/setup/admin", json=admin_data)
        
        assert_api_error(response, 400, "Initial admin setup is not needed. Users already exist.")
    
    def test_initial_admin_setup_validation(self, client: TestClient, session: Session):
        """Test initial admin setup with invalid data."""
        # Ensure no users exist in database
        users = session.exec(select(User)).all()
        for user in users:
            session.delete(user)
        session.commit()
        
        # Test missing fields
        incomplete_data = {"username": "admin"}
        response = client.post("/api/auth/setup/admin", json=incomplete_data)
        assert_api_error(response, 422)
        
        # Test password too short
        short_password_data = {
            "username": "admin",
            "password": "short"
        }
        response = client.post("/api/auth/setup/admin", json=short_password_data)
        assert_api_error(response, 422)
        
        # Test username too short
        short_username_data = {
            "username": "ad",
            "password": "admin123"
        }
        response = client.post("/api/auth/setup/admin", json=short_username_data)
        assert_api_error(response, 422)
    
    def test_initial_admin_setup_response_schema(self, client: TestClient, session: Session):
        """Test initial admin setup response has correct schema."""
        # Ensure no users exist in database
        users = session.exec(select(User)).all()
        for user in users:
            session.delete(user)
        session.commit()
        
        admin_data = {
            "username": "admin",
            "password": "admin123"
        }
        response = client.post("/api/auth/setup/admin", json=admin_data)
        
        assert_api_success(response, 201)
        data = response.json()
        
        # Verify response schema matches UserProfileResponse
        required_fields = ["id", "username", "is_active", "is_admin", "preferences", "created_at", "updated_at"]
        for field in required_fields:
            assert field in data
        
        # Verify data types
        assert isinstance(data["id"], str)
        assert isinstance(data["username"], str)
        assert isinstance(data["is_active"], bool)
        assert isinstance(data["is_admin"], bool)
        assert isinstance(data["preferences"], dict)
        assert isinstance(data["created_at"], str)
        assert isinstance(data["updated_at"], str)
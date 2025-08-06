"""
Integration tests for admin user management endpoints.
Tests all admin user management endpoints with proper request/response validation.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from typing import Dict

from ..models.user import User, UserSession
from .integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    admin_user_fixture as admin_user,
    auth_headers_fixture as auth_headers,
    admin_headers_fixture as admin_headers,
    create_test_user_data,
    assert_api_error,
    assert_api_success,
)


class TestAdminCreateUser:
    """Test POST /api/auth/admin/users endpoint."""
    
    def test_create_regular_user_success(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test successful creation of regular user by admin."""
        user_data = {
            "username": "newuser",
            "password": "newpassword123",
            "is_admin": False
        }
        response = client.post("/api/auth/admin/users", json=user_data, headers=admin_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["username"] == user_data["username"]
        assert data["is_active"] is True
        assert data["is_admin"] is False
        assert "password" not in data
        assert "password_hash" not in data
        assert "id" in data
        assert "created_at" in data
        assert "updated_at" in data
    
    def test_create_admin_user_success(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test successful creation of admin user by admin."""
        user_data = {
            "username": "newadmin",
            "password": "newpassword123",
            "is_admin": True
        }
        response = client.post("/api/auth/admin/users", json=user_data, headers=admin_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["username"] == user_data["username"]
        assert data["is_active"] is True
        assert data["is_admin"] is True
        assert "password" not in data
        assert "password_hash" not in data
    
    def test_create_user_not_admin(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test user creation by non-admin user."""
        user_data = {
            "username": "newuser",
            "password": "newpassword123",
            "is_admin": False
        }
        response = client.post("/api/auth/admin/users", json=user_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_create_user_duplicate_username(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test creation with duplicate username."""
        user_data = {
            "username": test_user.username,  # Use existing username
            "password": "newpassword123",
            "is_admin": False
        }
        response = client.post("/api/auth/admin/users", json=user_data, headers=admin_headers)
        
        assert_api_error(response, 400, "Username already taken")
    
    def test_create_user_missing_fields(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test creation with missing required fields."""
        incomplete_data = {"username": "newuser"}  # Missing password
        response = client.post("/api/auth/admin/users", json=incomplete_data, headers=admin_headers)
        
        assert_api_error(response, 422)
    
    def test_create_user_invalid_username(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test creation with invalid username."""
        user_data = {
            "username": "x",  # Too short
            "password": "newpassword123",
            "is_admin": False
        }
        response = client.post("/api/auth/admin/users", json=user_data, headers=admin_headers)
        
        assert_api_error(response, 422)
    
    def test_create_user_invalid_password(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test creation with invalid password."""
        user_data = {
            "username": "newuser",
            "password": "123",  # Too short
            "is_admin": False
        }
        response = client.post("/api/auth/admin/users", json=user_data, headers=admin_headers)
        
        assert_api_error(response, 422)
    
    def test_create_user_without_auth(self, client: TestClient):
        """Test creation without authentication."""
        user_data = {
            "username": "newuser",
            "password": "newpassword123",
            "is_admin": False
        }
        response = client.post("/api/auth/admin/users", json=user_data)
        
        assert_api_error(response, 403, "Not authenticated")


class TestAdminListUsers:
    """Test GET /api/auth/admin/users endpoint."""
    
    def test_list_users_success(self, client: TestClient, admin_headers: Dict[str, str], test_user: User, admin_user: User):
        """Test successful listing of all users by admin."""
        response = client.get("/api/auth/admin/users", headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert isinstance(data, list)
        assert len(data) >= 2  # At least test_user and admin_user
        
        # Verify structure of user objects
        for user in data:
            assert "id" in user
            assert "username" in user
            assert "is_active" in user
            assert "is_admin" in user
            assert "created_at" in user
            assert "updated_at" in user
            assert "password" not in user
            assert "password_hash" not in user
    
    def test_list_users_not_admin(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test user listing by non-admin user."""
        response = client.get("/api/auth/admin/users", headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_list_users_without_auth(self, client: TestClient):
        """Test user listing without authentication."""
        response = client.get("/api/auth/admin/users")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_list_users_includes_all_types(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test that listing includes both regular and admin users."""
        # Create additional users
        regular_user = User(username="regular", password_hash="hash", is_admin=False)
        admin_user_2 = User(username="admin2", password_hash="hash", is_admin=True)
        session.add(regular_user)
        session.add(admin_user_2)
        session.commit()
        
        response = client.get("/api/auth/admin/users", headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        
        usernames = [user["username"] for user in data]
        assert "regular" in usernames
        assert "admin2" in usernames


class TestAdminGetUser:
    """Test GET /api/auth/admin/users/{user_id} endpoint."""
    
    def test_get_user_success(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test successful retrieval of specific user by admin."""
        response = client.get(f"/api/auth/admin/users/{test_user.id}", headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["id"] == test_user.id
        assert data["username"] == test_user.username
        assert data["is_active"] == test_user.is_active
        assert data["is_admin"] == test_user.is_admin
        assert "password" not in data
        assert "password_hash" not in data
    
    def test_get_user_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test retrieval of non-existent user."""
        response = client.get("/api/auth/admin/users/non-existent-id", headers=admin_headers)
        
        assert_api_error(response, 404, "User not found")
    
    def test_get_user_not_admin(self, client: TestClient, auth_headers: Dict[str, str], test_user: User):
        """Test user retrieval by non-admin user."""
        response = client.get(f"/api/auth/admin/users/{test_user.id}", headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_get_user_without_auth(self, client: TestClient, test_user: User):
        """Test user retrieval without authentication."""
        response = client.get(f"/api/auth/admin/users/{test_user.id}")
        
        assert_api_error(response, 403, "Not authenticated")


class TestAdminUpdateUser:
    """Test PUT /api/auth/admin/users/{user_id} endpoint."""
    
    def test_update_user_username_success(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test successful username update by admin."""
        update_data = {"username": "updatedusername"}
        response = client.put(f"/api/auth/admin/users/{test_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["username"] == "updatedusername"
        assert data["id"] == test_user.id
    
    def test_update_user_activate_deactivate(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test user activation/deactivation by admin."""
        # Deactivate user
        update_data = {"is_active": False}
        response = client.put(f"/api/auth/admin/users/{test_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["is_active"] is False
        
        # Reactivate user
        update_data = {"is_active": True}
        response = client.put(f"/api/auth/admin/users/{test_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["is_active"] is True
    
    def test_update_user_admin_status(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test admin status change by admin."""
        update_data = {"is_admin": True}
        response = client.put(f"/api/auth/admin/users/{test_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["is_admin"] is True
    
    def test_update_user_partial_update(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test partial user update."""
        original_username = test_user.username
        update_data = {"is_active": False}  # Only update is_active
        response = client.put(f"/api/auth/admin/users/{test_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["username"] == original_username  # Should not change
        assert data["is_active"] is False  # Should change
    
    def test_update_user_duplicate_username(self, client: TestClient, admin_headers: Dict[str, str], test_user: User, admin_user: User):
        """Test username update with duplicate username."""
        update_data = {"username": admin_user.username}  # Try to use admin's username
        response = client.put(f"/api/auth/admin/users/{test_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 400, "Username already taken")
    
    def test_update_user_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test update of non-existent user."""
        update_data = {"username": "newname"}
        response = client.put("/api/auth/admin/users/non-existent-id", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 404, "User not found")
    
    def test_update_user_not_admin(self, client: TestClient, auth_headers: Dict[str, str], test_user: User):
        """Test user update by non-admin user."""
        update_data = {"username": "newname"}
        response = client.put(f"/api/auth/admin/users/{test_user.id}", json=update_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_admin_cannot_deactivate_self(self, client: TestClient, admin_headers: Dict[str, str], admin_user: User):
        """Test that admin cannot deactivate their own account."""
        update_data = {"is_active": False}
        response = client.put(f"/api/auth/admin/users/{admin_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 400, "Cannot deactivate your own account")
    
    def test_admin_cannot_remove_own_admin_privileges(self, client: TestClient, admin_headers: Dict[str, str], admin_user: User):
        """Test that admin cannot remove their own admin privileges."""
        update_data = {"is_admin": False}
        response = client.put(f"/api/auth/admin/users/{admin_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 400, "Cannot remove your own admin privileges")
    
    def test_update_user_sessions_invalidated_on_deactivation(self, client: TestClient, admin_headers: Dict[str, str], test_user: User, session: Session):
        """Test that user sessions are invalidated when user is deactivated."""
        # Create a session for the test user
        from datetime import datetime, timedelta, timezone
        import uuid
        test_session = UserSession(
            id=str(uuid.uuid4()),
            user_id=test_user.id,
            token_hash="test-token-hash",
            refresh_token_hash="test-refresh-hash",
            expires_at=datetime.now(timezone.utc) + timedelta(days=1)
        )
        session.add(test_session)
        session.commit()
        
        # Verify session exists
        existing_sessions = session.exec(select(UserSession).where(UserSession.user_id == test_user.id)).all()
        assert len(existing_sessions) > 0
        
        # Deactivate user
        update_data = {"is_active": False}
        response = client.put(f"/api/auth/admin/users/{test_user.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        
        # Verify sessions are deleted
        session.refresh(test_user)
        remaining_sessions = session.exec(select(UserSession).where(UserSession.user_id == test_user.id)).all()
        assert len(remaining_sessions) == 0


class TestAdminResetPassword:
    """Test PUT /api/auth/admin/users/{user_id}/password endpoint."""
    
    def test_reset_password_success(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test successful password reset by admin."""
        password_data = {"new_password": "newpassword123"}
        response = client.put(f"/api/auth/admin/users/{test_user.id}/password", json=password_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Password reset successfully. User will need to log in again."
    
    def test_reset_password_user_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test password reset for non-existent user."""
        password_data = {"new_password": "newpassword123"}
        response = client.put("/api/auth/admin/users/non-existent-id/password", json=password_data, headers=admin_headers)
        
        assert_api_error(response, 404, "User not found")
    
    def test_reset_password_not_admin(self, client: TestClient, auth_headers: Dict[str, str], test_user: User):
        """Test password reset by non-admin user."""
        password_data = {"new_password": "newpassword123"}
        response = client.put(f"/api/auth/admin/users/{test_user.id}/password", json=password_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_reset_password_invalid_password(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test password reset with invalid password."""
        password_data = {"new_password": "123"}  # Too short
        response = client.put(f"/api/auth/admin/users/{test_user.id}/password", json=password_data, headers=admin_headers)
        
        assert_api_error(response, 422)
    
    def test_reset_password_missing_password(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test password reset with missing password field."""
        password_data = {}  # No new_password field
        response = client.put(f"/api/auth/admin/users/{test_user.id}/password", json=password_data, headers=admin_headers)
        
        assert_api_error(response, 422)
    
    def test_reset_password_sessions_invalidated(self, client: TestClient, admin_headers: Dict[str, str], test_user: User, session: Session):
        """Test that user sessions are invalidated after password reset."""
        # Create a session for the test user
        from datetime import datetime, timedelta, timezone
        import uuid
        test_session = UserSession(
            id=str(uuid.uuid4()),
            user_id=test_user.id,
            token_hash="test-token-hash",
            refresh_token_hash="test-refresh-hash",
            expires_at=datetime.now(timezone.utc) + timedelta(days=1)
        )
        session.add(test_session)
        session.commit()
        
        # Verify session exists
        existing_sessions = session.exec(select(UserSession).where(UserSession.user_id == test_user.id)).all()
        assert len(existing_sessions) > 0
        
        # Reset password
        password_data = {"new_password": "newpassword123"}
        response = client.put(f"/api/auth/admin/users/{test_user.id}/password", json=password_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        
        # Verify sessions are deleted
        session.refresh(test_user)
        remaining_sessions = session.exec(select(UserSession).where(UserSession.user_id == test_user.id)).all()
        assert len(remaining_sessions) == 0


class TestAdminDeleteUser:
    """Test DELETE /api/auth/admin/users/{user_id} endpoint."""
    
    def test_delete_user_success(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test successful user deletion by admin."""
        # Create a user to delete
        user_to_delete = User(username="todelete", password_hash="hash", is_admin=False)
        session.add(user_to_delete)
        session.commit()
        session.refresh(user_to_delete)
        user_id = user_to_delete.id
        
        response = client.delete(f"/api/auth/admin/users/{user_id}", headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "User and all associated data deleted successfully"
        
        # Refresh session to see changes made by the API
        session.expire_all()
        
        # Verify user is deleted
        deleted_user = session.get(User, user_id)
        assert deleted_user is None
    
    def test_delete_user_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test deletion of non-existent user."""
        response = client.delete("/api/auth/admin/users/non-existent-id", headers=admin_headers)
        
        assert_api_error(response, 404, "User not found")
    
    def test_delete_user_not_admin(self, client: TestClient, auth_headers: Dict[str, str], test_user: User):
        """Test user deletion by non-admin user."""
        response = client.delete(f"/api/auth/admin/users/{test_user.id}", headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_admin_cannot_delete_self(self, client: TestClient, admin_headers: Dict[str, str], admin_user: User):
        """Test that admin cannot delete their own account."""
        response = client.delete(f"/api/auth/admin/users/{admin_user.id}", headers=admin_headers)
        
        assert_api_error(response, 400, "Cannot delete your own account")
    
    def test_delete_user_without_auth(self, client: TestClient, test_user: User):
        """Test user deletion without authentication."""
        response = client.delete(f"/api/auth/admin/users/{test_user.id}")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_delete_user_cascade_deletion(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test that related data is properly handled when user is deleted."""
        # Create a user with related data
        user_to_delete = User(username="withdata", password_hash="hash", is_admin=False)
        session.add(user_to_delete)
        session.commit()
        session.refresh(user_to_delete)
        
        # Add a session for this user
        from datetime import datetime, timedelta, timezone
        import uuid
        user_session = UserSession(
            id=str(uuid.uuid4()),
            user_id=user_to_delete.id,
            token_hash="test-token-hash",
            refresh_token_hash="test-refresh-hash",
            expires_at=datetime.now(timezone.utc) + timedelta(days=1)
        )
        session.add(user_session)
        session.commit()
        
        user_id = user_to_delete.id
        
        # First manually delete sessions (since cascade delete is handled in the endpoint)
        existing_sessions = session.exec(select(UserSession).where(UserSession.user_id == user_id)).all()
        assert len(existing_sessions) > 0  # Verify session exists before deletion
        
        # Delete the user (the endpoint should handle related data deletion)
        response = client.delete(f"/api/auth/admin/users/{user_id}", headers=admin_headers)
        assert_api_success(response, 200)
        
        # Refresh session to see changes made by the API
        session.expire_all()
        
        # Verify user is deleted
        deleted_user = session.get(User, user_id)
        assert deleted_user is None
        
        # Verify related sessions are also deleted
        remaining_sessions = session.exec(select(UserSession).where(UserSession.user_id == user_id)).all()
        assert len(remaining_sessions) == 0


class TestAdminEndpointSecurity:
    """Test security aspects of admin user management endpoints."""
    
    def test_all_endpoints_require_admin(self, client: TestClient, auth_headers: Dict[str, str], test_user: User):
        """Test that all admin endpoints require admin privileges."""
        user_id = test_user.id
        
        # Test all admin endpoints with non-admin user
        endpoints_and_methods = [
            ("POST", "/api/auth/admin/users", {"username": "test", "password": "password123"}),
            ("GET", "/api/auth/admin/users", None),
            ("GET", f"/api/auth/admin/users/{user_id}", None),
            ("PUT", f"/api/auth/admin/users/{user_id}", {"username": "newname"}),
            ("PUT", f"/api/auth/admin/users/{user_id}/password", {"new_password": "newpass123"}),
            ("DELETE", f"/api/auth/admin/users/{user_id}", None),
        ]
        
        for method, url, data in endpoints_and_methods:
            if method == "POST":
                response = client.post(url, json=data, headers=auth_headers)
            elif method == "GET":
                response = client.get(url, headers=auth_headers)
            elif method == "PUT":
                response = client.put(url, json=data, headers=auth_headers)
            elif method == "DELETE":
                response = client.delete(url, headers=auth_headers)
            
            assert_api_error(response, 403, "Administrative privileges required")
    
    def test_no_password_data_in_responses(self, client: TestClient, admin_headers: Dict[str, str], test_user: User):
        """Test that password data is never included in responses."""
        # Test create user response
        user_data = {"username": "passwordtest", "password": "secret123", "is_admin": False}
        create_response = client.post("/api/auth/admin/users", json=user_data, headers=admin_headers)
        assert_api_success(create_response, 201)
        create_data = create_response.json()
        assert "password" not in create_data
        assert "password_hash" not in create_data
        
        # Test list users response
        list_response = client.get("/api/auth/admin/users", headers=admin_headers)
        assert_api_success(list_response, 200)
        list_data = list_response.json()
        for user in list_data:
            assert "password" not in user
            assert "password_hash" not in user
        
        # Test get user response
        get_response = client.get(f"/api/auth/admin/users/{test_user.id}", headers=admin_headers)
        assert_api_success(get_response, 200)
        get_data = get_response.json()
        assert "password" not in get_data
        assert "password_hash" not in get_data
        
        # Test update user response
        update_response = client.put(f"/api/auth/admin/users/{test_user.id}", json={"username": "updated"}, headers=admin_headers)
        assert_api_success(update_response, 200)
        update_data = update_response.json()
        assert "password" not in update_data
        assert "password_hash" not in update_data
    
    def test_admin_self_protection_rules(self, client: TestClient, admin_headers: Dict[str, str], admin_user: User):
        """Test that admins cannot perform dangerous operations on themselves."""
        admin_id = admin_user.id
        
        # Cannot deactivate self
        deactivate_response = client.put(f"/api/auth/admin/users/{admin_id}", json={"is_active": False}, headers=admin_headers)
        assert_api_error(deactivate_response, 400, "Cannot deactivate your own account")
        
        # Cannot remove own admin privileges
        demote_response = client.put(f"/api/auth/admin/users/{admin_id}", json={"is_admin": False}, headers=admin_headers)
        assert_api_error(demote_response, 400, "Cannot remove your own admin privileges")
        
        # Cannot delete self
        delete_response = client.delete(f"/api/auth/admin/users/{admin_id}", headers=admin_headers)
        assert_api_error(delete_response, 400, "Cannot delete your own account")
        
        # But can change username (test this first before token invalidation)
        username_response = client.put(f"/api/auth/admin/users/{admin_id}", json={"username": "newadminname"}, headers=admin_headers)
        assert_api_success(username_response, 200)
        
        # Can reset own password (this will invalidate the token)
        password_response = client.put(f"/api/auth/admin/users/{admin_id}/password", json={"new_password": "newpass123"}, headers=admin_headers)
        assert_api_success(password_response, 200)
    
    def test_response_schema_consistency(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test that all user responses have consistent schema."""
        # Create a user for testing
        user_data = {"username": "schematest", "password": "password123", "is_admin": False}
        create_response = client.post("/api/auth/admin/users", json=user_data, headers=admin_headers)
        assert_api_success(create_response, 201)
        created_user = create_response.json()
        user_id = created_user["id"]
        
        # Test that all user responses have the same required fields
        required_fields = ["id", "username", "is_active", "is_admin", "created_at", "updated_at"]
        forbidden_fields = ["password", "password_hash"]
        
        responses_to_test = [
            created_user,  # Create response
            client.get(f"/api/auth/admin/users/{user_id}", headers=admin_headers).json(),  # Get response
            client.put(f"/api/auth/admin/users/{user_id}", json={"username": "updated"}, headers=admin_headers).json(),  # Update response
        ]
        
        # Test list response (array of users)
        list_response = client.get("/api/auth/admin/users", headers=admin_headers)
        assert_api_success(list_response, 200)
        list_data = list_response.json()
        responses_to_test.extend(list_data)
        
        for user_response in responses_to_test:
            # Check required fields
            for field in required_fields:
                assert field in user_response, f"Missing required field: {field}"
            
            # Check forbidden fields
            for field in forbidden_fields:
                assert field not in user_response, f"Found forbidden field: {field}"
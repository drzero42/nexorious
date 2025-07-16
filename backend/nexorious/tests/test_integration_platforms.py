"""
Integration tests for platforms endpoints.
Tests all platforms and storefronts API endpoints with proper request/response validation.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select
from typing import Dict, Any

from ..models.platform import Platform, Storefront
from ..models.user import User
from .integration_test_utils import (
    client_fixture as client,
    session_fixture as session,
    test_user_fixture as test_user,
    admin_user_fixture as admin_user,
    auth_headers_fixture as auth_headers,
    admin_headers_fixture as admin_headers,
    test_platform_fixture as test_platform,
    test_storefront_fixture as test_storefront,
    create_test_platform_data,
    create_test_storefront_data,
    assert_api_error,
    assert_api_success,
    register_and_login_user
)


class TestPlatformsListEndpoint:
    """Test GET /api/platforms/ endpoint."""
    
    def test_list_platforms_success(self, client: TestClient, test_platform: Platform):
        """Test successful platforms list retrieval."""
        response = client.get("/api/platforms/")
        
        assert_api_success(response, 200)
        data = response.json()
        assert "platforms" in data
        assert "total" in data
        assert len(data["platforms"]) == 1
        assert data["platforms"][0]["id"] == str(test_platform.id)
        assert data["platforms"][0]["name"] == test_platform.name
        assert data["platforms"][0]["display_name"] == test_platform.display_name
    
    def test_list_platforms_empty(self, client: TestClient):
        """Test platforms list with no platforms."""
        response = client.get("/api/platforms/")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 0
        assert data["total"] == 0
    
    def test_list_platforms_active_only(self, client: TestClient, session: Session):
        """Test platforms list shows only active platforms."""
        # Create active platform
        active_platform = Platform(
            name="active",
            display_name="Active Platform",
            is_active=True
        )
        session.add(active_platform)
        
        # Create inactive platform
        inactive_platform = Platform(
            name="inactive",
            display_name="Inactive Platform",
            is_active=False
        )
        session.add(inactive_platform)
        session.commit()
        
        response = client.get("/api/platforms/")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 1
        assert data["platforms"][0]["name"] == "active"
    
    def test_list_platforms_pagination(self, client: TestClient, session: Session):
        """Test platforms list with pagination."""
        # Create multiple platforms
        for i in range(5):
            platform = Platform(
                name=f"platform_{i}",
                display_name=f"Platform {i}",
                is_active=True
            )
            session.add(platform)
        session.commit()
        
        response = client.get("/api/platforms/?page=1&per_page=2")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 2
        assert data["total"] == 5


class TestPlatformsDetailEndpoint:
    """Test GET /api/platforms/{platform_id} endpoint."""
    
    def test_get_platform_success(self, client: TestClient, test_platform: Platform):
        """Test successful platform retrieval."""
        response = client.get(f"/api/platforms/{test_platform.id}")
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["id"] == str(test_platform.id)
        assert data["name"] == test_platform.name
        assert data["display_name"] == test_platform.display_name
        assert data["icon_url"] == test_platform.icon_url
        assert data["is_active"] == test_platform.is_active
    
    def test_get_platform_not_found(self, client: TestClient):
        """Test platform retrieval with non-existent ID."""
        response = client.get("/api/platforms/non-existent-id")
        
        assert_api_error(response, 404, "Platform not found")
    
    def test_get_inactive_platform(self, client: TestClient, session: Session):
        """Test retrieval of inactive platform."""
        inactive_platform = Platform(
            name="inactive",
            display_name="Inactive Platform",
            is_active=False
        )
        session.add(inactive_platform)
        session.commit()
        session.refresh(inactive_platform)
        
        response = client.get(f"/api/platforms/{inactive_platform.id}")
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["is_active"] is False


class TestPlatformsCreateEndpoint:
    """Test POST /api/platforms/ endpoint."""
    
    def test_create_platform_success(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test successful platform creation by admin."""
        platform_data = create_test_platform_data()
        response = client.post("/api/platforms/", json=platform_data, headers=admin_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["name"] == platform_data["name"]
        assert data["display_name"] == platform_data["display_name"]
        assert data["icon_url"] == platform_data["icon_url"]
        assert data["is_active"] == platform_data["is_active"]
    
    def test_create_platform_not_admin(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test platform creation by non-admin user."""
        platform_data = create_test_platform_data()
        response = client.post("/api/platforms/", json=platform_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_create_platform_without_auth(self, client: TestClient):
        """Test platform creation without authentication."""
        platform_data = create_test_platform_data()
        response = client.post("/api/platforms/", json=platform_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_create_platform_duplicate_name(self, client: TestClient, admin_headers: Dict[str, str], test_platform: Platform):
        """Test creation of platform with duplicate name."""
        platform_data = create_test_platform_data(name=test_platform.name)
        response = client.post("/api/platforms/", json=platform_data, headers=admin_headers)
        
        assert_api_error(response, 409, "already exists")
    
    def test_create_platform_missing_fields(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test platform creation with missing required fields."""
        incomplete_data = {"name": "test"}
        response = client.post("/api/platforms/", json=incomplete_data, headers=admin_headers)
        
        assert_api_error(response, 422)
    
    def test_create_platform_invalid_url(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test platform creation with invalid icon URL."""
        platform_data = create_test_platform_data(icon_url="invalid-url")
        response = client.post("/api/platforms/", json=platform_data, headers=admin_headers)
        
        assert_api_error(response, 422)


class TestPlatformsUpdateEndpoint:
    """Test PUT /api/platforms/{platform_id} endpoint."""
    
    def test_update_platform_success(self, client: TestClient, test_platform: Platform, admin_headers: Dict[str, str]):
        """Test successful platform update by admin."""
        update_data = {
            "display_name": "Updated Platform Name",
            "icon_url": "https://example.com/updated.png",
            "is_active": False
        }
        response = client.put(f"/api/platforms/{test_platform.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["display_name"] == "Updated Platform Name"
        assert data["icon_url"] == "https://example.com/updated.png"
        assert data["is_active"] is False
        assert data["name"] == test_platform.name  # Should not change
    
    def test_update_platform_not_admin(self, client: TestClient, test_platform: Platform, auth_headers: Dict[str, str]):
        """Test platform update by non-admin user."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/{test_platform.id}", json=update_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_update_platform_without_auth(self, client: TestClient, test_platform: Platform):
        """Test platform update without authentication."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/{test_platform.id}", json=update_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_update_platform_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test platform update with non-existent ID."""
        update_data = {"display_name": "Updated Name"}
        response = client.put("/api/platforms/non-existent-id", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 404, "Platform not found")
    
    def test_update_platform_partial(self, client: TestClient, test_platform: Platform, admin_headers: Dict[str, str]):
        """Test partial platform update."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/{test_platform.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["display_name"] == "Updated Name"
        assert data["icon_url"] == test_platform.icon_url  # Should remain unchanged


class TestPlatformsDeleteEndpoint:
    """Test DELETE /api/platforms/{platform_id} endpoint."""
    
    def test_delete_platform_success(self, client: TestClient, test_platform: Platform, admin_headers: Dict[str, str]):
        """Test successful platform deletion by admin."""
        response = client.delete(f"/api/platforms/{test_platform.id}", headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Platform deleted successfully"
    
    def test_delete_platform_not_admin(self, client: TestClient, test_platform: Platform, auth_headers: Dict[str, str]):
        """Test platform deletion by non-admin user."""
        response = client.delete(f"/api/platforms/{test_platform.id}", headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_delete_platform_without_auth(self, client: TestClient, test_platform: Platform):
        """Test platform deletion without authentication."""
        response = client.delete(f"/api/platforms/{test_platform.id}")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_delete_platform_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test platform deletion with non-existent ID."""
        response = client.delete("/api/platforms/non-existent-id", headers=admin_headers)
        
        assert_api_error(response, 404, "Platform not found")


class TestStorefrontsListEndpoint:
    """Test GET /api/platforms/storefronts/ endpoint."""
    
    def test_list_storefronts_success(self, client: TestClient, test_storefront: Storefront):
        """Test successful storefronts list retrieval."""
        response = client.get("/api/platforms/storefronts/")
        
        assert_api_success(response, 200)
        data = response.json()
        assert "storefronts" in data
        assert "total" in data
        assert len(data["storefronts"]) == 1
        assert data["storefronts"][0]["id"] == str(test_storefront.id)
        assert data["storefronts"][0]["name"] == test_storefront.name
        assert data["storefronts"][0]["display_name"] == test_storefront.display_name
    
    def test_list_storefronts_empty(self, client: TestClient):
        """Test storefronts list with no storefronts."""
        response = client.get("/api/platforms/storefronts/")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["storefronts"]) == 0
        assert data["total"] == 0
    
    def test_list_storefronts_active_only(self, client: TestClient, session: Session):
        """Test storefronts list shows only active storefronts."""
        # Create active storefront
        active_storefront = Storefront(
            name="active",
            display_name="Active Storefront",
            base_url="https://active.example.com",
            is_active=True
        )
        session.add(active_storefront)
        
        # Create inactive storefront
        inactive_storefront = Storefront(
            name="inactive",
            display_name="Inactive Storefront",
            base_url="https://inactive.example.com",
            is_active=False
        )
        session.add(inactive_storefront)
        session.commit()
        
        response = client.get("/api/platforms/storefronts/")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["storefronts"]) == 1
        assert data["storefronts"][0]["name"] == "active"
    
    def test_list_storefronts_pagination(self, client: TestClient, session: Session):
        """Test storefronts list with pagination."""
        # Create multiple storefronts
        for i in range(5):
            storefront = Storefront(
                name=f"storefront_{i}",
                display_name=f"Storefront {i}",
                base_url=f"https://storefront{i}.example.com",
                is_active=True
            )
            session.add(storefront)
        session.commit()
        
        response = client.get("/api/platforms/storefronts/?page=1&per_page=2")
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["storefronts"]) == 2
        assert data["total"] == 5


class TestStorefrontsDetailEndpoint:
    """Test GET /api/platforms/storefronts/{storefront_id} endpoint."""
    
    def test_get_storefront_success(self, client: TestClient, test_storefront: Storefront):
        """Test successful storefront retrieval."""
        response = client.get(f"/api/platforms/storefronts/{test_storefront.id}")
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["id"] == str(test_storefront.id)
        assert data["name"] == test_storefront.name
        assert data["display_name"] == test_storefront.display_name
        assert data["icon_url"] == test_storefront.icon_url
        assert data["base_url"] == test_storefront.base_url
        assert data["is_active"] == test_storefront.is_active
    
    def test_get_storefront_not_found(self, client: TestClient):
        """Test storefront retrieval with non-existent ID."""
        response = client.get("/api/platforms/storefronts/non-existent-id")
        
        assert_api_error(response, 404, "Storefront not found")
    
    def test_get_inactive_storefront(self, client: TestClient, session: Session):
        """Test retrieval of inactive storefront."""
        inactive_storefront = Storefront(
            name="inactive",
            display_name="Inactive Storefront",
            base_url="https://inactive.example.com",
            is_active=False
        )
        session.add(inactive_storefront)
        session.commit()
        session.refresh(inactive_storefront)
        
        response = client.get(f"/api/platforms/storefronts/{inactive_storefront.id}")
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["is_active"] is False


class TestStorefrontsCreateEndpoint:
    """Test POST /api/platforms/storefronts/ endpoint."""
    
    def test_create_storefront_success(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test successful storefront creation by admin."""
        storefront_data = create_test_storefront_data()
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=admin_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["name"] == storefront_data["name"]
        assert data["display_name"] == storefront_data["display_name"]
        assert data["icon_url"] == storefront_data["icon_url"]
        assert data["base_url"] == storefront_data["base_url"]
        assert data["is_active"] == storefront_data["is_active"]
    
    def test_create_storefront_not_admin(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test storefront creation by non-admin user."""
        storefront_data = create_test_storefront_data()
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_create_storefront_without_auth(self, client: TestClient):
        """Test storefront creation without authentication."""
        storefront_data = create_test_storefront_data()
        response = client.post("/api/platforms/storefronts/", json=storefront_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_create_storefront_duplicate_name(self, client: TestClient, admin_headers: Dict[str, str], test_storefront: Storefront):
        """Test creation of storefront with duplicate name."""
        storefront_data = create_test_storefront_data(name=test_storefront.name)
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=admin_headers)
        
        assert_api_error(response, 409, "already exists")
    
    def test_create_storefront_missing_fields(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test storefront creation with missing required fields."""
        incomplete_data = {"name": "test"}
        response = client.post("/api/platforms/storefronts/", json=incomplete_data, headers=admin_headers)
        
        assert_api_error(response, 422)
    
    def test_create_storefront_invalid_url(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test storefront creation with invalid base URL."""
        storefront_data = create_test_storefront_data(base_url="invalid-url")
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=admin_headers)
        
        assert_api_error(response, 422)


class TestStorefrontsUpdateEndpoint:
    """Test PUT /api/platforms/storefronts/{storefront_id} endpoint."""
    
    def test_update_storefront_success(self, client: TestClient, test_storefront: Storefront, admin_headers: Dict[str, str]):
        """Test successful storefront update by admin."""
        update_data = {
            "display_name": "Updated Storefront Name",
            "icon_url": "https://example.com/updated.png",
            "base_url": "https://updated.example.com",
            "is_active": False
        }
        response = client.put(f"/api/platforms/storefronts/{test_storefront.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["display_name"] == "Updated Storefront Name"
        assert data["icon_url"] == "https://example.com/updated.png"
        assert data["base_url"] == "https://updated.example.com"
        assert data["is_active"] is False
        assert data["name"] == test_storefront.name  # Should not change
    
    def test_update_storefront_not_admin(self, client: TestClient, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test storefront update by non-admin user."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/storefronts/{test_storefront.id}", json=update_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_update_storefront_without_auth(self, client: TestClient, test_storefront: Storefront):
        """Test storefront update without authentication."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/storefronts/{test_storefront.id}", json=update_data)
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_update_storefront_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test storefront update with non-existent ID."""
        update_data = {"display_name": "Updated Name"}
        response = client.put("/api/platforms/storefronts/non-existent-id", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 404, "Storefront not found")
    
    def test_update_storefront_partial(self, client: TestClient, test_storefront: Storefront, admin_headers: Dict[str, str]):
        """Test partial storefront update."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/storefronts/{test_storefront.id}", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["display_name"] == "Updated Name"
        assert data["base_url"] == test_storefront.base_url  # Should remain unchanged


class TestStorefrontsDeleteEndpoint:
    """Test DELETE /api/platforms/storefronts/{storefront_id} endpoint."""
    
    def test_delete_storefront_success(self, client: TestClient, test_storefront: Storefront, admin_headers: Dict[str, str]):
        """Test successful storefront deletion by admin."""
        response = client.delete(f"/api/platforms/storefronts/{test_storefront.id}", headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Storefront deleted successfully"
    
    def test_delete_storefront_not_admin(self, client: TestClient, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test storefront deletion by non-admin user."""
        response = client.delete(f"/api/platforms/storefronts/{test_storefront.id}", headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_delete_storefront_without_auth(self, client: TestClient, test_storefront: Storefront):
        """Test storefront deletion without authentication."""
        response = client.delete(f"/api/platforms/storefronts/{test_storefront.id}")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_delete_storefront_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test storefront deletion with non-existent ID."""
        response = client.delete("/api/platforms/storefronts/non-existent-id", headers=admin_headers)
        
        assert_api_error(response, 404, "Storefront not found")


class TestPlatformsEndpointsSecurity:
    """Test security aspects of platforms endpoints."""
    
    def test_admin_only_endpoints_require_admin(self, client: TestClient, test_platform: Platform, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test that admin-only endpoints require admin access."""
        # Test platform creation
        platform_data = create_test_platform_data()
        response = client.post("/api/platforms/", json=platform_data, headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")
        
        # Test platform update
        response = client.put(f"/api/platforms/{test_platform.id}", json={"display_name": "Updated"}, headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")
        
        # Test platform deletion
        response = client.delete(f"/api/platforms/{test_platform.id}", headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")
        
        # Test storefront creation
        storefront_data = create_test_storefront_data()
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")
        
        # Test storefront update
        response = client.put(f"/api/platforms/storefronts/{test_storefront.id}", json={"display_name": "Updated"}, headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")
        
        # Test storefront deletion
        response = client.delete(f"/api/platforms/storefronts/{test_storefront.id}", headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_authenticated_endpoints_require_auth(self, client: TestClient, test_platform: Platform, test_storefront: Storefront):
        """Test that authenticated endpoints require authentication."""
        # Test platform creation
        platform_data = create_test_platform_data()
        response = client.post("/api/platforms/", json=platform_data)
        assert_api_error(response, 403, "Not authenticated")
        
        # Test platform update
        response = client.put(f"/api/platforms/{test_platform.id}", json={"display_name": "Updated"})
        assert_api_error(response, 403, "Not authenticated")
        
        # Test platform deletion
        response = client.delete(f"/api/platforms/{test_platform.id}")
        assert_api_error(response, 403, "Not authenticated")
        
        # Test storefront creation
        storefront_data = create_test_storefront_data()
        response = client.post("/api/platforms/storefronts/", json=storefront_data)
        assert_api_error(response, 403, "Not authenticated")
        
        # Test storefront update
        response = client.put(f"/api/platforms/storefronts/{test_storefront.id}", json={"display_name": "Updated"})
        assert_api_error(response, 403, "Not authenticated")
        
        # Test storefront deletion
        response = client.delete(f"/api/platforms/storefronts/{test_storefront.id}")
        assert_api_error(response, 403, "Not authenticated")
    
    def test_public_endpoints_allow_anonymous_access(self, client: TestClient, test_platform: Platform, test_storefront: Storefront):
        """Test that public endpoints allow anonymous access."""
        # Test platforms list
        response = client.get("/api/platforms/")
        assert_api_success(response, 200)
        
        # Test platform detail
        response = client.get(f"/api/platforms/{test_platform.id}")
        assert_api_success(response, 200)
        
        # Test storefronts list
        response = client.get("/api/platforms/storefronts/")
        assert_api_success(response, 200)
        
        # Test storefront detail
        response = client.get(f"/api/platforms/storefronts/{test_storefront.id}")
        assert_api_success(response, 200)


class TestPlatformsDataValidation:
    """Test data validation for platforms endpoints."""
    
    def test_platform_name_validation(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test platform name validation."""
        # Test empty name
        platform_data = create_test_platform_data(name="")
        response = client.post("/api/platforms/", json=platform_data, headers=admin_headers)
        assert_api_error(response, 422)
        
        # Test too long name
        platform_data = create_test_platform_data(name="a" * 101)
        response = client.post("/api/platforms/", json=platform_data, headers=admin_headers)
        assert_api_error(response, 422)
    
    def test_platform_display_name_validation(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test platform display name validation."""
        # Test empty display name
        platform_data = create_test_platform_data(display_name="")
        response = client.post("/api/platforms/", json=platform_data, headers=admin_headers)
        assert_api_error(response, 422)
        
        # Test too long display name
        platform_data = create_test_platform_data(display_name="a" * 101)
        response = client.post("/api/platforms/", json=platform_data, headers=admin_headers)
        assert_api_error(response, 422)
    
    def test_storefront_name_validation(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test storefront name validation."""
        # Test empty name
        storefront_data = create_test_storefront_data(name="")
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=admin_headers)
        assert_api_error(response, 422)
        
        # Test too long name
        storefront_data = create_test_storefront_data(name="a" * 101)
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=admin_headers)
        assert_api_error(response, 422)
    
    def test_storefront_url_validation(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test storefront URL validation."""
        # Test invalid base URL
        storefront_data = create_test_storefront_data(base_url="not-a-url")
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=admin_headers)
        assert_api_error(response, 422)
        
        # Test invalid icon URL
        storefront_data = create_test_storefront_data(icon_url="not-a-url")
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=admin_headers)
        assert_api_error(response, 422)
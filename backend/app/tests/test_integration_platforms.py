"""
Integration tests for platforms endpoints.
Tests all platforms and storefronts API endpoints with proper request/response validation.
"""

from fastapi.testclient import TestClient
from sqlmodel import Session, select
from typing import Dict

from ..models.platform import Platform, Storefront, PlatformStorefront
from .integration_test_utils import (
    create_test_platform_data,
    create_test_storefront_data,
    assert_api_error,
    assert_api_success
)


class TestPlatformsListEndpoint:
    """Test GET /api/platforms/ endpoint."""
    
    def test_list_platforms_success(self, client: TestClient, test_platform: Platform, auth_headers):
        """Test successful platforms list retrieval."""
        response = client.get("/api/platforms/", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert "platforms" in data
        assert "total" in data
        assert len(data["platforms"]) == 1
        assert data["platforms"][0]["name"] == test_platform.name
        assert data["platforms"][0]["display_name"] == test_platform.display_name
        assert "storefronts" in data["platforms"][0]
        assert data["platforms"][0]["storefronts"] == []
    
    def test_list_platforms_empty(self, client: TestClient, auth_headers):
        """Test platforms list with no platforms."""
        response = client.get("/api/platforms/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 0
        assert data["total"] == 0
    
    def test_list_platforms_active_only(self, client: TestClient, session: Session, auth_headers):
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
        
        response = client.get("/api/platforms/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 1
        assert data["platforms"][0]["name"] == "active"
    
    def test_list_platforms_pagination(self, client: TestClient, session: Session, auth_headers):
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
        
        response = client.get("/api/platforms/?page=1&per_page=2", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 2
        assert data["total"] == 5
    
    def test_list_platforms_includes_associated_storefronts(self, client: TestClient, session: Session, auth_headers):
        """Test that platforms list includes associated storefronts for each platform."""
        # Create platforms
        platform1 = Platform(
            name="platform-1",
            display_name="Platform 1",
            is_active=True
        )
        platform2 = Platform(
            name="platform-2", 
            display_name="Platform 2",
            is_active=True
        )
        session.add(platform1)
        session.add(platform2)
        
        # Create storefronts
        storefront1 = Storefront(
            name="storefront-1",
            display_name="Storefront 1",
            is_active=True
        )
        storefront2 = Storefront(
            name="storefront-2",
            display_name="Storefront 2", 
            is_active=True
        )
        storefront3 = Storefront(
            name="storefront-3",
            display_name="Storefront 3",
            is_active=True
        )
        inactive_storefront = Storefront(
            name="inactive-storefront",
            display_name="Inactive Storefront",
            is_active=False
        )
        session.add(storefront1)
        session.add(storefront2)
        session.add(storefront3)
        session.add(inactive_storefront)
        session.commit()
        
        assoc1 = PlatformStorefront(platform=platform1.name, storefront=storefront1.name)
        assoc2 = PlatformStorefront(platform=platform1.name, storefront=storefront2.name)
        assoc_inactive = PlatformStorefront(platform=platform1.name, storefront=inactive_storefront.name)
        assoc3 = PlatformStorefront(platform=platform2.name, storefront=storefront3.name)
        
        session.add(assoc1)
        session.add(assoc2)
        session.add(assoc_inactive)
        session.add(assoc3)
        session.commit()
        
        response = client.get("/api/platforms/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 2
        
        # Find each platform in response
        platform1_data = next(p for p in data["platforms"] if p["name"] == "platform-1")
        platform2_data = next(p for p in data["platforms"] if p["name"] == "platform-2")
        
        # Check platform 1 has 2 associated storefronts (inactive one filtered out)
        assert "storefronts" in platform1_data
        assert len(platform1_data["storefronts"]) == 2
        storefront_names = [s["name"] for s in platform1_data["storefronts"]]
        assert "storefront-1" in storefront_names
        assert "storefront-2" in storefront_names
        assert "inactive-storefront" not in storefront_names
        
        # Check platform 2 has 1 associated storefront
        assert "storefronts" in platform2_data
        assert len(platform2_data["storefronts"]) == 1
        assert platform2_data["storefronts"][0]["name"] == "storefront-3"
        
        storefront = platform1_data["storefronts"][0]
        assert "name" in storefront
        assert "display_name" in storefront
        assert "is_active" in storefront
        assert storefront["is_active"] is True
    
    def test_list_platforms_with_no_storefront_associations(self, client: TestClient, session: Session, auth_headers):
        """Test that platforms with no storefront associations return empty storefronts list."""
        # Create platform with no associations
        platform = Platform(
            name="isolated-platform",
            display_name="Isolated Platform",
            is_active=True
        )
        session.add(platform)
        session.commit()
        
        response = client.get("/api/platforms/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["platforms"]) == 1
        
        platform_data = data["platforms"][0]
        assert "storefronts" in platform_data
        assert len(platform_data["storefronts"]) == 0
        assert platform_data["storefronts"] == []


class TestPlatformsDetailEndpoint:
    """Test GET /api/platforms/{platform_name} endpoint."""

    def test_get_platform_success(self, client: TestClient, test_platform: Platform, auth_headers):
        """Test successful platform retrieval."""
        response = client.get(f"/api/platforms/{test_platform.name}", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["name"] == test_platform.name
        assert data["display_name"] == test_platform.display_name
        assert data["icon_url"] == test_platform.icon_url
        assert data["is_active"] == test_platform.is_active

    def test_get_platform_not_found(self, client: TestClient, auth_headers):
        """Test platform retrieval with non-existent name."""
        response = client.get("/api/platforms/non-existent-platform", headers=auth_headers)

        assert_api_error(response, 404, "Platform not found")

    def test_get_inactive_platform(self, client: TestClient, session: Session, auth_headers):
        """Test retrieval of inactive platform."""
        inactive_platform = Platform(
            name="inactive",
            display_name="Inactive Platform",
            is_active=False
        )
        session.add(inactive_platform)
        session.commit()
        session.refresh(inactive_platform)

        response = client.get(f"/api/platforms/{inactive_platform.name}", headers=auth_headers)

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
    """Test PUT /api/platforms/{platform_name} endpoint."""

    def test_update_platform_success(self, client: TestClient, test_platform: Platform, admin_headers: Dict[str, str]):
        """Test successful platform update by admin."""
        update_data = {
            "display_name": "Updated Platform Name",
            "icon_url": "https://example.com/updated.png",
            "is_active": False
        }
        response = client.put(f"/api/platforms/{test_platform.name}", json=update_data, headers=admin_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["display_name"] == "Updated Platform Name"
        assert data["icon_url"] == "https://example.com/updated.png"
        assert data["is_active"] is False
        assert data["name"] == test_platform.name

    def test_update_platform_not_admin(self, client: TestClient, test_platform: Platform, auth_headers: Dict[str, str]):
        """Test platform update by non-admin user."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/{test_platform.name}", json=update_data, headers=auth_headers)

        assert_api_error(response, 403, "Administrative privileges required")

    def test_update_platform_without_auth(self, client: TestClient, test_platform: Platform):
        """Test platform update without authentication."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/{test_platform.name}", json=update_data)

        assert_api_error(response, 403, "Not authenticated")

    def test_update_platform_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test platform update with non-existent name."""
        update_data = {"display_name": "Updated Name"}
        response = client.put("/api/platforms/non-existent-platform", json=update_data, headers=admin_headers)

        assert_api_error(response, 404, "Platform not found")

    def test_update_platform_partial(self, client: TestClient, test_platform: Platform, admin_headers: Dict[str, str]):
        """Test partial platform update."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/{test_platform.name}", json=update_data, headers=admin_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["display_name"] == "Updated Name"
        assert data["icon_url"] == test_platform.icon_url

    def test_update_official_platform_changes_source_to_custom(self, client: TestClient, session: Session, admin_headers: Dict[str, str]):
        """Test that updating an official platform changes its source to custom."""
        official_platform = Platform(
            name="test-official-platform",
            display_name="Test Official Platform",
            source="official",
            version_added="1.0.0"
        )
        session.add(official_platform)
        session.commit()
        session.refresh(official_platform)

        assert official_platform.source == "official"

        update_data = {
            "display_name": "Updated Test Official Platform"
        }

        response = client.put(f"/api/platforms/{official_platform.name}", json=update_data, headers=admin_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["display_name"] == "Updated Test Official Platform"
        assert data["source"] == "custom"

        session.refresh(official_platform)
        assert official_platform.source == "custom"


class TestPlatformsDeleteEndpoint:
    """Test DELETE /api/platforms/{platform_name} endpoint."""

    def test_delete_platform_success(self, client: TestClient, test_platform: Platform, admin_headers: Dict[str, str]):
        """Test successful platform deletion by admin."""
        response = client.delete(f"/api/platforms/{test_platform.name}", headers=admin_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Platform deleted successfully"

    def test_delete_platform_not_admin(self, client: TestClient, test_platform: Platform, auth_headers: Dict[str, str]):
        """Test platform deletion by non-admin user."""
        response = client.delete(f"/api/platforms/{test_platform.name}", headers=auth_headers)

        assert_api_error(response, 403, "Administrative privileges required")

    def test_delete_platform_without_auth(self, client: TestClient, test_platform: Platform):
        """Test platform deletion without authentication."""
        response = client.delete(f"/api/platforms/{test_platform.name}")

        assert_api_error(response, 403, "Not authenticated")

    def test_delete_platform_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test platform deletion with non-existent name."""
        response = client.delete("/api/platforms/non-existent-platform", headers=admin_headers)

        assert_api_error(response, 404, "Platform not found")


class TestStorefrontsListEndpoint:
    """Test GET /api/platforms/storefronts/ endpoint."""

    def test_list_storefronts_success(self, client: TestClient, test_storefront: Storefront, auth_headers):
        """Test successful storefronts list retrieval."""
        response = client.get("/api/platforms/storefronts/", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert "storefronts" in data
        assert "total" in data
        assert len(data["storefronts"]) == 1
        assert data["storefronts"][0]["name"] == test_storefront.name
        assert data["storefronts"][0]["display_name"] == test_storefront.display_name
    
    def test_list_storefronts_empty(self, client: TestClient, auth_headers):
        """Test storefronts list with no storefronts."""
        response = client.get("/api/platforms/storefronts/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["storefronts"]) == 0
        assert data["total"] == 0
    
    def test_list_storefronts_active_only(self, client: TestClient, session: Session, auth_headers):
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
        
        response = client.get("/api/platforms/storefronts/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["storefronts"]) == 1
        assert data["storefronts"][0]["name"] == "active"
    
    def test_list_storefronts_pagination(self, client: TestClient, session: Session, auth_headers):
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
        
        response = client.get("/api/platforms/storefronts/?page=1&per_page=2", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["storefronts"]) == 2
        assert data["total"] == 5


class TestStorefrontsDetailEndpoint:
    """Test GET /api/platforms/storefronts/{storefront_name} endpoint."""

    def test_get_storefront_success(self, client: TestClient, test_storefront: Storefront, auth_headers):
        """Test successful storefront retrieval."""
        response = client.get(f"/api/platforms/storefronts/{test_storefront.name}", headers=auth_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["name"] == test_storefront.name
        assert data["display_name"] == test_storefront.display_name
        assert data["icon_url"] == test_storefront.icon_url
        assert data["base_url"] == test_storefront.base_url
        assert data["is_active"] == test_storefront.is_active

    def test_get_storefront_not_found(self, client: TestClient, auth_headers):
        """Test storefront retrieval with non-existent name."""
        response = client.get("/api/platforms/storefronts/non-existent-storefront", headers=auth_headers)

        assert_api_error(response, 404, "Storefront not found")

    def test_get_inactive_storefront(self, client: TestClient, session: Session, auth_headers):
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

        response = client.get(f"/api/platforms/storefronts/{inactive_storefront.name}", headers=auth_headers)

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
    """Test PUT /api/platforms/storefronts/{storefront_name} endpoint."""

    def test_update_storefront_success(self, client: TestClient, test_storefront: Storefront, admin_headers: Dict[str, str]):
        """Test successful storefront update by admin."""
        update_data = {
            "display_name": "Updated Storefront Name",
            "icon_url": "https://example.com/updated.png",
            "base_url": "https://updated.example.com",
            "is_active": False
        }
        response = client.put(f"/api/platforms/storefronts/{test_storefront.name}", json=update_data, headers=admin_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["display_name"] == "Updated Storefront Name"
        assert data["icon_url"] == "https://example.com/updated.png"
        assert data["base_url"] == "https://updated.example.com/"
        assert data["is_active"] is False
        assert data["name"] == test_storefront.name

    def test_update_storefront_not_admin(self, client: TestClient, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test storefront update by non-admin user."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/storefronts/{test_storefront.name}", json=update_data, headers=auth_headers)

        assert_api_error(response, 403, "Administrative privileges required")

    def test_update_storefront_without_auth(self, client: TestClient, test_storefront: Storefront):
        """Test storefront update without authentication."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/storefronts/{test_storefront.name}", json=update_data)

        assert_api_error(response, 403, "Not authenticated")

    def test_update_storefront_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test storefront update with non-existent name."""
        update_data = {"display_name": "Updated Name"}
        response = client.put("/api/platforms/storefronts/non-existent-storefront", json=update_data, headers=admin_headers)

        assert_api_error(response, 404, "Storefront not found")

    def test_update_storefront_partial(self, client: TestClient, test_storefront: Storefront, admin_headers: Dict[str, str]):
        """Test partial storefront update."""
        update_data = {"display_name": "Updated Name"}
        response = client.put(f"/api/platforms/storefronts/{test_storefront.name}", json=update_data, headers=admin_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["display_name"] == "Updated Name"
        assert data["base_url"] == test_storefront.base_url

    def test_update_official_storefront_changes_source_to_custom(self, client: TestClient, session: Session, admin_headers: Dict[str, str]):
        """Test that updating an official storefront changes its source to custom."""
        official_storefront = Storefront(
            name="test-official-storefront",
            display_name="Test Official Storefront",
            base_url="https://test-official-store.com",
            source="official",
            version_added="1.0.0"
        )
        session.add(official_storefront)
        session.commit()
        session.refresh(official_storefront)

        assert official_storefront.source == "official"

        update_data = {
            "display_name": "Updated Test Official Storefront"
        }

        response = client.put(f"/api/platforms/storefronts/{official_storefront.name}", json=update_data, headers=admin_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["display_name"] == "Updated Test Official Storefront"
        assert data["source"] == "custom"

        session.refresh(official_storefront)
        assert official_storefront.source == "custom"


class TestStorefrontsDeleteEndpoint:
    """Test DELETE /api/platforms/storefronts/{storefront_name} endpoint."""

    def test_delete_storefront_success(self, client: TestClient, test_storefront: Storefront, admin_headers: Dict[str, str]):
        """Test successful storefront deletion by admin."""
        response = client.delete(f"/api/platforms/storefronts/{test_storefront.name}", headers=admin_headers)

        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Storefront deleted successfully"

    def test_delete_storefront_not_admin(self, client: TestClient, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test storefront deletion by non-admin user."""
        response = client.delete(f"/api/platforms/storefronts/{test_storefront.name}", headers=auth_headers)

        assert_api_error(response, 403, "Administrative privileges required")

    def test_delete_storefront_without_auth(self, client: TestClient, test_storefront: Storefront):
        """Test storefront deletion without authentication."""
        response = client.delete(f"/api/platforms/storefronts/{test_storefront.name}")

        assert_api_error(response, 403, "Not authenticated")

    def test_delete_storefront_not_found(self, client: TestClient, admin_headers: Dict[str, str]):
        """Test storefront deletion with non-existent name."""
        response = client.delete("/api/platforms/storefronts/non-existent-storefront", headers=admin_headers)

        assert_api_error(response, 404, "Storefront not found")


class TestPlatformsEndpointsSecurity:
    """Test security aspects of platforms endpoints."""

    def test_admin_only_endpoints_require_admin(self, client: TestClient, test_platform: Platform, test_storefront: Storefront, auth_headers: Dict[str, str]):
        """Test that admin-only endpoints require admin access."""
        platform_data = create_test_platform_data()
        response = client.post("/api/platforms/", json=platform_data, headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")

        response = client.put(f"/api/platforms/{test_platform.name}", json={"display_name": "Updated"}, headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")

        response = client.delete(f"/api/platforms/{test_platform.name}", headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")

        storefront_data = create_test_storefront_data()
        response = client.post("/api/platforms/storefronts/", json=storefront_data, headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")

        response = client.put(f"/api/platforms/storefronts/{test_storefront.name}", json={"display_name": "Updated"}, headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")

        response = client.delete(f"/api/platforms/storefronts/{test_storefront.name}", headers=auth_headers)
        assert_api_error(response, 403, "Administrative privileges required")

    def test_authenticated_endpoints_require_auth(self, client: TestClient, test_platform: Platform, test_storefront: Storefront):
        """Test that authenticated endpoints require authentication."""
        platform_data = create_test_platform_data()
        response = client.post("/api/platforms/", json=platform_data)
        assert_api_error(response, 403, "Not authenticated")

        response = client.put(f"/api/platforms/{test_platform.name}", json={"display_name": "Updated"})
        assert_api_error(response, 403, "Not authenticated")

        response = client.delete(f"/api/platforms/{test_platform.name}")
        assert_api_error(response, 403, "Not authenticated")

        storefront_data = create_test_storefront_data()
        response = client.post("/api/platforms/storefronts/", json=storefront_data)
        assert_api_error(response, 403, "Not authenticated")

        response = client.put(f"/api/platforms/storefronts/{test_storefront.name}", json={"display_name": "Updated"})
        assert_api_error(response, 403, "Not authenticated")

        response = client.delete(f"/api/platforms/storefronts/{test_storefront.name}")
        assert_api_error(response, 403, "Not authenticated")
    


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


class TestPlatformDefaultStorefrontGetEndpoint:
    """Test GET /api/platforms/{platform_id}/default-storefront endpoint."""
    
    def test_get_platform_default_storefront_with_default(self, client: TestClient, session: Session, test_platform: Platform, test_storefront: Storefront, auth_headers):
        """Test getting platform default storefront when one is set."""
        # Set the storefront as default for the platform
        test_platform.default_storefront = test_storefront.name
        session.commit()
        session.refresh(test_platform)
        
        response = client.get(f"/api/platforms/{test_platform.name}/default-storefront", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["platform"] == test_platform.name
        assert data["platform_display_name"] == test_platform.display_name
        assert data["platform_display_name"] == test_platform.display_name
        assert data["default_storefront"] is not None
        assert data["default_storefront"]["name"] == test_storefront.name
        assert data["default_storefront"]["name"] == test_storefront.name
    
    def test_get_platform_default_storefront_without_default(self, client: TestClient, test_platform: Platform, auth_headers):
        """Test getting platform default storefront when none is set."""
        response = client.get(f"/api/platforms/{test_platform.name}/default-storefront", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["platform"] == test_platform.name
        assert data["platform_display_name"] == test_platform.display_name
        assert data["platform_display_name"] == test_platform.display_name
        assert data["default_storefront"] is None
    
    def test_get_platform_default_storefront_not_found(self, client: TestClient, auth_headers):
        """Test getting default storefront for non-existent platform."""
        response = client.get("/api/platforms/nonexistent-id/default-storefront", headers=auth_headers)
        
        assert_api_error(response, 404, "Platform not found")


class TestPlatformDefaultStorefrontUpdateEndpoint:
    """Test PUT /api/platforms/{platform_id}/default-storefront endpoint."""
    
    def test_update_platform_default_storefront_success(self, client: TestClient, session: Session, admin_headers: Dict[str, str], test_platform: Platform, test_storefront: Storefront):
        """Test successfully updating platform default storefront."""
        update_data = {"storefront": test_storefront.name}
        response = client.put(f"/api/platforms/{test_platform.name}/default-storefront", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["platform"] == test_platform.name
        assert data["default_storefront"]["name"] == test_storefront.name
        
        # Verify in database
        session.refresh(test_platform)
        assert test_platform.default_storefront == test_storefront.name
    
    def test_update_platform_default_storefront_remove_default(self, client: TestClient, session: Session, admin_headers: Dict[str, str], test_platform: Platform, test_storefront: Storefront):
        """Test removing platform default storefront by setting to null."""
        # First set a default
        test_platform.default_storefront = test_storefront.name
        session.commit()
        
        # Then remove it
        update_data = {"storefront": None}
        response = client.put(f"/api/platforms/{test_platform.name}/default-storefront", json=update_data, headers=admin_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["platform"] == test_platform.name
        assert data["default_storefront"] is None
        
        # Verify in database
        session.refresh(test_platform)
        assert test_platform.default_storefront is None
    
    def test_update_platform_default_storefront_platform_not_found(self, client: TestClient, admin_headers: Dict[str, str], test_storefront: Storefront):
        """Test updating default storefront for non-existent platform."""
        update_data = {"storefront": test_storefront.name}
        response = client.put("/api/platforms/nonexistent-id/default-storefront", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 404, "Platform not found")
    
    def test_update_platform_default_storefront_storefront_not_found(self, client: TestClient, admin_headers: Dict[str, str], test_platform: Platform):
        """Test updating default storefront with non-existent storefront."""
        update_data = {"storefront": "nonexistent-id"}
        response = client.put(f"/api/platforms/{test_platform.name}/default-storefront", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 404, "Storefront not found")
    
    def test_update_platform_default_storefront_inactive_storefront(self, client: TestClient, session: Session, admin_headers: Dict[str, str], test_platform: Platform, test_storefront: Storefront):
        """Test updating default storefront with inactive storefront."""
        # Make storefront inactive
        test_storefront.is_active = False
        session.commit()
        
        update_data = {"storefront": test_storefront.name}
        response = client.put(f"/api/platforms/{test_platform.name}/default-storefront", json=update_data, headers=admin_headers)
        
        assert_api_error(response, 400, "Cannot set inactive storefront as default")
    
    def test_update_platform_default_storefront_admin_required(self, client: TestClient, auth_headers: Dict[str, str], test_platform: Platform, test_storefront: Storefront):
        """Test that admin access is required for updating default storefront."""
        update_data = {"storefront": test_storefront.name}
        response = client.put(f"/api/platforms/{test_platform.name}/default-storefront", json=update_data, headers=auth_headers)
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_update_platform_default_storefront_unauthenticated(self, client: TestClient, test_platform: Platform, test_storefront: Storefront):
        """Test that authentication is required for updating default storefront."""
        update_data = {"storefront": test_storefront.name}
        response = client.put(f"/api/platforms/{test_platform.name}/default-storefront", json=update_data)
        
        assert_api_error(response, 403, "Not authenticated")


class TestPlatformStorefrontsEndpoint:
    """Test GET /api/platforms/{platform_id}/storefronts endpoint."""
    
    def test_get_platform_storefronts_success(self, client: TestClient, session: Session, auth_headers):
        """Test successful retrieval of platform storefronts."""
        # Create platform and storefronts
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        session.add(platform)
        
        storefront1 = Storefront(
            name="test-storefront-1",
            display_name="Test Storefront 1",
            is_active=True
        )
        storefront2 = Storefront(
            name="test-storefront-2", 
            display_name="Test Storefront 2",
            is_active=True
        )
        session.add(storefront1)
        session.add(storefront2)
        session.commit()
        
        # Create platform-storefront associations
        assoc1 = PlatformStorefront(platform=platform.name, storefront=storefront1.name)
        assoc2 = PlatformStorefront(platform=platform.name, storefront=storefront2.name)
        session.add(assoc1)
        session.add(assoc2)
        session.commit()
        
        response = client.get(f"/api/platforms/{platform.name}/storefronts", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["platform"] == platform.name
        assert data["platform_display_name"] == platform.display_name
        assert data["platform_display_name"] == platform.display_name
        assert data["total_storefronts"] == 2
        assert len(data["storefronts"]) == 2
        
        # Check that storefronts are returned in correct order (by display name)
        assert data["storefronts"][0]["name"] == "test-storefront-1"
        assert data["storefronts"][1]["name"] == "test-storefront-2"
    
    def test_get_platform_storefronts_empty(self, client: TestClient, session: Session, auth_headers):
        """Test platform with no storefront associations."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        session.add(platform)
        session.commit()
        
        response = client.get(f"/api/platforms/{platform.name}/storefronts", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["platform"] == platform.name
        assert data["total_storefronts"] == 0
        assert len(data["storefronts"]) == 0
    
    def test_get_platform_storefronts_active_only(self, client: TestClient, session: Session, auth_headers):
        """Test platform storefronts with active_only filter."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        session.add(platform)
        
        active_storefront = Storefront(
            name="active-storefront",
            display_name="Active Storefront",
            is_active=True
        )
        inactive_storefront = Storefront(
            name="inactive-storefront",
            display_name="Inactive Storefront", 
            is_active=False
        )
        session.add(active_storefront)
        session.add(inactive_storefront)
        session.commit()
        
        # Create associations for both storefronts
        assoc1 = PlatformStorefront(platform=platform.name, storefront=active_storefront.name)
        assoc2 = PlatformStorefront(platform=platform.name, storefront=inactive_storefront.name)
        session.add(assoc1)
        session.add(assoc2)
        session.commit()
        
        # Test with active_only=true (default)
        response = client.get(f"/api/platforms/{platform.name}/storefronts", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total_storefronts"] == 1
        assert len(data["storefronts"]) == 1
        assert data["storefronts"][0]["name"] == "active-storefront"
        
        # Test with active_only=false
        response = client.get(f"/api/platforms/{platform.name}/storefronts?active_only=false", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total_storefronts"] == 2
        assert len(data["storefronts"]) == 2
    
    def test_get_platform_storefronts_platform_not_found(self, client: TestClient, auth_headers):
        """Test platform storefronts for non-existent platform."""
        response = client.get("/api/platforms/nonexistent-id/storefronts", headers=auth_headers)
        
        assert_api_error(response, 404, "Platform not found")


class TestPlatformStorefrontAssociationEndpoints:
    """Test POST/DELETE /api/platforms/{platform_id}/storefronts/{storefront} endpoints."""
    
    def test_create_platform_storefront_association_success(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test successful platform-storefront association creation."""
        # Create platform and storefront
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        response = client.post(
            f"/api/platforms/{platform.name}/storefronts/{storefront.name}",
            headers=admin_headers
        )
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["platform"] == platform.name
        assert data["platform_display_name"] == platform.display_name
        assert data["platform_display_name"] == platform.display_name
        assert data["storefront"] == storefront.name
        assert data["storefront_display_name"] == storefront.display_name
        assert data["storefront_display_name"] == storefront.display_name
        assert data["message"] == "Association created successfully"
        
        # Verify association was created in database
        association = session.exec(
            select(PlatformStorefront).where(
                PlatformStorefront.platform == platform.name,
                PlatformStorefront.storefront == storefront.name
            )
        ).first()
        assert association is not None
    
    def test_create_platform_storefront_association_duplicate(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test creating duplicate platform-storefront association."""
        # Create platform and storefront
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront", 
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        # Create existing association
        association = PlatformStorefront(
            platform=platform.name,
            storefront=storefront.name
        )
        session.add(association)
        session.commit()
        
        response = client.post(
            f"/api/platforms/{platform.name}/storefronts/{storefront.name}",
            headers=admin_headers
        )
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Association already exists"
    
    def test_create_platform_storefront_association_platform_not_found(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test creating association with non-existent platform."""
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(storefront)
        session.commit()
        
        response = client.post(
            f"/api/platforms/nonexistent-id/storefronts/{storefront.name}",
            headers=admin_headers
        )
        
        assert_api_error(response, 404, "Platform not found")
    
    def test_create_platform_storefront_association_storefront_not_found(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test creating association with non-existent storefront."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        session.add(platform)
        session.commit()
        
        response = client.post(
            f"/api/platforms/{platform.name}/storefronts/nonexistent-id",
            headers=admin_headers
        )
        
        assert_api_error(response, 404, "Storefront not found")
    
    def test_create_platform_storefront_association_inactive_platform(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test creating association with inactive platform."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=False
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        response = client.post(
            f"/api/platforms/{platform.name}/storefronts/{storefront.name}",
            headers=admin_headers
        )
        
        assert_api_error(response, 400, "Cannot create association with inactive platform")
    
    def test_create_platform_storefront_association_inactive_storefront(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test creating association with inactive storefront."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=False
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        response = client.post(
            f"/api/platforms/{platform.name}/storefronts/{storefront.name}",
            headers=admin_headers
        )
        
        assert_api_error(response, 400, "Cannot create association with inactive storefront")
    
    def test_create_platform_storefront_association_requires_admin(self, client: TestClient, auth_headers: Dict[str, str], session: Session):
        """Test that creating association requires admin privileges."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        response = client.post(
            f"/api/platforms/{platform.name}/storefronts/{storefront.name}",
            headers=auth_headers
        )
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_create_platform_storefront_association_requires_auth(self, client: TestClient, session: Session):
        """Test that creating association requires authentication."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        response = client.post(f"/api/platforms/{platform.name}/storefronts/{storefront.name}")
        
        assert_api_error(response, 403, "Not authenticated")
    
    def test_delete_platform_storefront_association_success(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test successful platform-storefront association removal."""
        # Create platform and storefront
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        # Create association
        association = PlatformStorefront(
            platform=platform.name,
            storefront=storefront.name
        )
        session.add(association)
        session.commit()
        
        response = client.delete(
            f"/api/platforms/{platform.name}/storefronts/{storefront.name}",
            headers=admin_headers
        )
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["platform"] == platform.name
        assert data["storefront"] == storefront.name
        assert data["message"] == "Association removed successfully"
        
        # Verify association was removed from database
        association = session.exec(
            select(PlatformStorefront).where(
                PlatformStorefront.platform == platform.name,
                PlatformStorefront.storefront == storefront.name
            )
        ).first()
        assert association is None
    
    def test_delete_platform_storefront_association_not_exists(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test removing non-existent platform-storefront association (idempotent)."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        response = client.delete(
            f"/api/platforms/{platform.name}/storefronts/{storefront.name}",
            headers=admin_headers
        )
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["message"] == "Association does not exist"
    
    def test_delete_platform_storefront_association_platform_not_found(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test removing association with non-existent platform."""
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(storefront)
        session.commit()
        
        response = client.delete(
            f"/api/platforms/nonexistent-id/storefronts/{storefront.name}",
            headers=admin_headers
        )
        
        assert_api_error(response, 404, "Platform not found")
    
    def test_delete_platform_storefront_association_storefront_not_found(self, client: TestClient, admin_headers: Dict[str, str], session: Session):
        """Test removing association with non-existent storefront."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        session.add(platform)
        session.commit()
        
        response = client.delete(
            f"/api/platforms/{platform.name}/storefronts/nonexistent-id",
            headers=admin_headers
        )
        
        assert_api_error(response, 404, "Storefront not found")
    
    def test_delete_platform_storefront_association_requires_admin(self, client: TestClient, auth_headers: Dict[str, str], session: Session):
        """Test that removing association requires admin privileges."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        response = client.delete(
            f"/api/platforms/{platform.name}/storefronts/{storefront.name}",
            headers=auth_headers
        )
        
        assert_api_error(response, 403, "Administrative privileges required")
    
    def test_delete_platform_storefront_association_requires_auth(self, client: TestClient, session: Session):
        """Test that removing association requires authentication."""
        platform = Platform(
            name="test-platform",
            display_name="Test Platform",
            is_active=True
        )
        storefront = Storefront(
            name="test-storefront",
            display_name="Test Storefront",
            is_active=True
        )
        session.add(platform)
        session.add(storefront)
        session.commit()
        
        response = client.delete(f"/api/platforms/{platform.name}/storefronts/{storefront.name}")
        
        assert_api_error(response, 403, "Not authenticated")
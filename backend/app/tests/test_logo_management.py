"""
Test logo management functionality including upload, delete, and file handling.
"""

import pytest
import tempfile
from pathlib import Path
from io import BytesIO
from PIL import Image
from fastapi.testclient import TestClient

from ..services.logo_service import LogoService
from ..models.platform import Platform, Storefront


@pytest.fixture(name="client")
def client_fixture_override(client_with_logo_service):
    """Override client fixture to use logo service for this test file."""
    return client_with_logo_service


class TestLogoService:
    """Test the LogoService class functionality."""
    
    def setup_method(self):
        """Setup test with temporary directory."""
        self.temp_dir = tempfile.mkdtemp()
        self.service = LogoService(self.temp_dir)
    
    def create_test_svg(self) -> bytes:
        """Create a test SVG file content."""
        svg_content = '''<?xml version="1.0" encoding="UTF-8"?>
<svg width="32" height="32" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg">
  <circle cx="16" cy="16" r="15" fill="#007acc"/>
  <text x="16" y="20" text-anchor="middle" fill="white" font-size="16">T</text>
</svg>'''
        return svg_content.encode('utf-8')
    
    def create_test_png(self) -> bytes:
        """Create a test PNG file content."""
        # Create a simple 32x32 blue image
        img = Image.new('RGBA', (32, 32), (0, 122, 204, 255))
        buffer = BytesIO()
        img.save(buffer, format='PNG')
        return buffer.getvalue()
    
    def test_service_initialization(self):
        """Test LogoService initialization creates directories."""
        assert Path(self.temp_dir).exists()
        assert self.service.base_dir == Path(self.temp_dir)
        assert self.service.max_file_size == 2 * 1024 * 1024
    
    def test_get_entity_dir(self):
        """Test entity directory creation."""
        entity_dir = self.service._get_entity_dir("platforms", "test-platform")
        expected_path = Path(self.temp_dir) / "platforms" / "test-platform"
        
        assert entity_dir == expected_path
        assert entity_dir.exists()
    
    def test_generate_filename(self):
        """Test filename generation."""
        filename = self.service._generate_filename("test-platform", "light", "image/svg+xml")
        assert filename == "test-platform-icon-light.svg"
        
        filename = self.service._generate_filename("test-platform", "dark", "image/png")
        assert filename == "test-platform-icon-dark.png"
    
    def test_list_logos_empty(self):
        """Test listing logos when none exist."""
        logos = self.service.list_logos("platforms", "nonexistent")
        assert logos == []
    
    def test_delete_logo_nonexistent(self):
        """Test deleting logos that don't exist."""
        deleted = self.service.delete_logo("platforms", "nonexistent", "light")
        assert deleted == []
    
    def test_cleanup_entity_logos(self):
        """Test cleaning up all logos for an entity."""
        # Create a test directory with files
        entity_dir = self.service._get_entity_dir("platforms", "test-platform")
        test_file = entity_dir / "test-platform-icon-light.svg"
        test_file.write_text("test content")
        
        assert test_file.exists()
        
        # Cleanup
        deleted = self.service.cleanup_entity_logos("platforms", "test-platform")
        
        assert len(deleted) == 1
        assert not test_file.exists()
        # Directory should also be removed if empty
        assert not entity_dir.exists()


class TestLogoEndpoints:
    """Test the logo management API endpoints."""
    
    def create_test_svg_file(self):
        """Create a test SVG file for upload."""
        svg_content = '''<?xml version="1.0" encoding="UTF-8"?>
<svg width="32" height="32" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg">
  <circle cx="16" cy="16" r="15" fill="#007acc"/>
  <text x="16" y="20" text-anchor="middle" fill="white" font-size="16">T</text>
</svg>'''
        return ("test-logo.svg", svg_content.encode('utf-8'), "image/svg+xml")
    
    def test_upload_platform_logo_success(self, client: TestClient, test_platform: Platform, admin_headers):
        """Test successful platform logo upload."""
        platform_id = test_platform.id
        
        files = {"file": self.create_test_svg_file()}
        params = {"theme": "light"}
        
        response = client.post(
            f"/api/platforms/{platform_id}/logo",
            files=files,
            params=params,
            headers=admin_headers
        )
        
        assert response.status_code == 200
        data = response.json()
        assert "message" in data
        assert data["theme"] == "light"
        assert "icon_url" in data
        assert "/static/logos/platforms/" in data["icon_url"]
    
    def test_upload_storefront_logo_success(self, client: TestClient, test_storefront: Storefront, admin_headers):
        """Test successful storefront logo upload."""
        storefront_id = test_storefront.id
        
        files = {"file": self.create_test_svg_file()}
        params = {"theme": "light"}
        
        response = client.post(
            f"/api/platforms/storefronts/{storefront_id}/logo",
            files=files,
            params=params,
            headers=admin_headers
        )
        
        assert response.status_code == 200
        data = response.json()
        assert "message" in data
        assert data["theme"] == "light"
        assert "icon_url" in data
        assert "/static/logos/storefronts/" in data["icon_url"]
    
    def test_upload_logo_invalid_theme(self, client: TestClient, test_platform: Platform, admin_headers):
        """Test logo upload with invalid theme."""
        platform_id = test_platform.id
        
        files = {"file": self.create_test_svg_file()}
        params = {"theme": "invalid"}
        
        response = client.post(
            f"/api/platforms/{platform_id}/logo",
            files=files,
            params=params,
            headers=admin_headers
        )
        
        assert response.status_code == 400
        response_data = response.json()
        assert "Invalid theme" in response_data.get("error", "")
    
    def test_upload_logo_nonexistent_platform(self, client: TestClient, admin_headers):
        """Test logo upload for nonexistent platform."""
        files = {"file": self.create_test_svg_file()}
        params = {"theme": "light"}
        
        response = client.post(
            "/api/platforms/nonexistent-id/logo",
            files=files,
            params=params,
            headers=admin_headers
        )
        
        assert response.status_code == 404
        response_data = response.json()
        assert "Platform not found" in response_data.get("error", "")
    
    def test_list_platform_logos(self, client: TestClient, test_platform: Platform, admin_headers):
        """Test listing platform logos."""
        platform_id = test_platform.id
        
        # First upload a logo
        files = {"file": self.create_test_svg_file()}
        params = {"theme": "light"}
        
        upload_response = client.post(
            f"/api/platforms/{platform_id}/logo",
            files=files,
            params=params,
            headers=admin_headers
        )
        assert upload_response.status_code == 200
        
        # Now list logos
        response = client.get(
            f"/api/platforms/{platform_id}/logos",
            headers=admin_headers
        )
        
        assert response.status_code == 200
        data = response.json()
        assert "platform" in data
        assert "logos" in data
        assert len(data["logos"]) == 1
        assert data["logos"][0]["theme"] == "light"
    
    def test_delete_platform_logo(self, client: TestClient, test_platform: Platform, admin_headers):
        """Test deleting platform logo."""
        platform_id = test_platform.id
        
        # First upload a logo
        files = {"file": self.create_test_svg_file()}
        params = {"theme": "light"}
        
        upload_response = client.post(
            f"/api/platforms/{platform_id}/logo",
            files=files,
            params=params,
            headers=admin_headers
        )
        assert upload_response.status_code == 200
        
        # Now delete it
        delete_response = client.delete(
            f"/api/platforms/{platform_id}/logo",
            params={"theme": "light"},
            headers=admin_headers
        )
        
        assert delete_response.status_code == 200
        data = delete_response.json()
        assert "message" in data
        assert data["deleted_files"] >= 0
    
    def test_logo_endpoints_require_admin(self, client: TestClient, test_platform: Platform):
        """Test that logo endpoints require admin privileges."""
        platform_id = test_platform.id
        
        # Try without authentication
        files = {"file": self.create_test_svg_file()}
        params = {"theme": "light"}
        
        response = client.post(
            f"/api/platforms/{platform_id}/logo",
            files=files,
            params=params
        )
        
        assert response.status_code == 403
        
        # Try listing without authentication
        response = client.get(f"/api/platforms/{platform_id}/logos")
        assert response.status_code == 403
        
        # Try deleting without authentication
        response = client.delete(f"/api/platforms/{platform_id}/logo")
        assert response.status_code == 403
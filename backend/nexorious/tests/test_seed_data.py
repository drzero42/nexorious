"""
Tests for seed data fixtures and seeding functionality.
Tests all seed data functions including platforms, storefronts, and platform-storefront associations.
"""

import pytest
from sqlmodel import Session, select
from typing import Dict, Any
from unittest.mock import patch
import logging
from fastapi.testclient import TestClient

from ..models.platform import Platform, Storefront
from ..models.user import User
from ..seed_data.seeder import (
    seed_platforms,
    seed_storefronts, 
    seed_all_official_data,
    get_seeding_conflicts
)
from ..seed_data.platforms import OFFICIAL_PLATFORMS
from ..seed_data.storefronts import OFFICIAL_STOREFRONTS
from ..seed_data.platform_storefront_associations import PLATFORM_STOREFRONT_ASSOCIATIONS
from .integration_test_utils import (
    session_fixture as session,
    client_fixture as client,
    create_test_user_data,
    register_and_login_user
)
from ..core.security import get_password_hash, create_access_token, hash_token
from datetime import datetime, timedelta, timezone
import uuid


def create_admin_user(session: Session, username: str = "admin", password: str = "testpassword") -> User:
    """Create an admin user for testing."""
    admin = User(
        username=username,
        password_hash=get_password_hash(password),
        is_active=True,
        is_admin=True
    )
    session.add(admin)
    session.commit()
    session.refresh(admin)
    return admin


def get_admin_headers(client: TestClient, session: Session, username: str = "admin", password: str = "testpassword") -> Dict[str, str]:
    """Create an admin user and get auth headers."""
    admin = create_admin_user(session, username, password)
    
    # Create session and token
    from ..models.user import UserSession
    
    token = create_access_token(data={"sub": admin.id})
    
    # Create session record for the token
    session_record = UserSession(
        id=str(uuid.uuid4()),
        user_id=admin.id,
        token_hash=hash_token(token),
        refresh_token_hash=hash_token("test_refresh_token"),
        expires_at=datetime.now(timezone.utc) + timedelta(days=30),
        user_agent="test-client",
        ip_address="127.0.0.1"
    )
    session.add(session_record)
    session.commit()
    
    return {"Authorization": f"Bearer {token}"}




class TestSeedPlatforms:
    """Test seed_platforms() function."""
    
    def test_seed_platforms_empty_database(self, session: Session):
        """Test seeding platforms into empty database."""
        # Verify database is empty
        existing_platforms = session.exec(select(Platform)).all()
        assert len(existing_platforms) == 0
        
        # Seed platforms
        count = seed_platforms(session, "1.0.0")
        
        # Verify all platforms were seeded
        assert count == len(OFFICIAL_PLATFORMS)
        
        platforms = session.exec(select(Platform)).all()
        assert len(platforms) == len(OFFICIAL_PLATFORMS)
        
        # Verify platform data
        platform_names = [p.name for p in platforms]
        expected_names = [op["name"] for op in OFFICIAL_PLATFORMS]
        assert set(platform_names) == set(expected_names)
        
        # Verify all platforms are marked as official
        for platform in platforms:
            assert platform.source == "official"
            assert platform.version_added == "1.0.0"
    
    def test_seed_platforms_idempotent(self, session: Session):
        """Test that seeding platforms multiple times doesn't create duplicates."""
        # First seed
        count1 = seed_platforms(session, "1.0.0")
        assert count1 == len(OFFICIAL_PLATFORMS)
        
        # Second seed
        count2 = seed_platforms(session, "1.0.0")
        assert count2 == 0  # No new platforms should be created
        
        # Verify still have correct number
        platforms = session.exec(select(Platform)).all()
        assert len(platforms) == len(OFFICIAL_PLATFORMS)
    
    def test_seed_platforms_preserves_custom_platforms(self, session: Session):
        """Test that custom platforms are preserved during seeding and official data doesn't overwrite them."""
        # Create a custom platform with same name as official one
        custom_platform = Platform(
            name="pc-windows",
            display_name="My Custom PC",
            icon_url="https://custom.com/icon.png",
            source="custom"
        )
        session.add(custom_platform)
        session.commit()
        
        # Seed platforms
        count = seed_platforms(session, "1.0.0")
        
        # Should have created other official platforms but left the custom one alone
        assert count == len(OFFICIAL_PLATFORMS) - 1  # One less because pc-windows is custom and preserved
        
        # Verify the custom platform was preserved
        platform = session.exec(select(Platform).where(Platform.name == "pc-windows")).first()
        assert platform.source == "custom"  # Still custom
        assert platform.version_added is None  # No version set for custom
        assert platform.display_name == "My Custom PC"  # Custom name preserved
        assert platform.icon_url == "https://custom.com/icon.png"  # Custom icon preserved


class TestSeedStorefronts:
    """Test seed_storefronts() function."""
    
    def test_seed_storefronts_empty_database(self, session: Session):
        """Test seeding storefronts into empty database."""
        # Verify database is empty
        existing_storefronts = session.exec(select(Storefront)).all()
        assert len(existing_storefronts) == 0
        
        # Seed storefronts
        count = seed_storefronts(session, "1.0.0")
        
        # Verify all storefronts were seeded
        assert count == len(OFFICIAL_STOREFRONTS)
        
        storefronts = session.exec(select(Storefront)).all()
        assert len(storefronts) == len(OFFICIAL_STOREFRONTS)
        
        # Verify storefront data
        storefront_names = [s.name for s in storefronts]
        expected_names = [os["name"] for os in OFFICIAL_STOREFRONTS]
        assert set(storefront_names) == set(expected_names)
        
        # Verify all storefronts are marked as official
        for storefront in storefronts:
            assert storefront.source == "official"
            assert storefront.version_added == "1.0.0"
    
    def test_seed_storefronts_idempotent(self, session: Session):
        """Test that seeding storefronts multiple times doesn't create duplicates."""
        # First seed
        count1 = seed_storefronts(session, "1.0.0")
        assert count1 == len(OFFICIAL_STOREFRONTS)
        
        # Second seed
        count2 = seed_storefronts(session, "1.0.0")
        assert count2 == 0  # No new storefronts should be created
        
        # Verify still have correct number
        storefronts = session.exec(select(Storefront)).all()
        assert len(storefronts) == len(OFFICIAL_STOREFRONTS)
    
    def test_seed_storefronts_preserves_custom_storefronts(self, session: Session):
        """Test that custom storefronts are preserved during seeding and official data doesn't overwrite them."""
        # Create a custom storefront with same name as official one
        custom_storefront = Storefront(
            name="steam",
            display_name="My Custom Steam",
            icon_url="https://custom.com/steam.png",
            base_url="https://custom.steamstore.com",
            source="custom"
        )
        session.add(custom_storefront)
        session.commit()
        
        # Seed storefronts
        count = seed_storefronts(session, "1.0.0")
        
        # Should have created other official storefronts but left the custom one alone
        assert count == len(OFFICIAL_STOREFRONTS) - 1  # One less because steam is custom and preserved
        
        # Verify the custom storefront was preserved
        storefront = session.exec(select(Storefront).where(Storefront.name == "steam")).first()
        assert storefront.source == "custom"  # Still custom
        assert storefront.version_added is None  # No version set for custom
        assert storefront.display_name == "My Custom Steam"  # Custom name preserved
        assert storefront.icon_url == "https://custom.com/steam.png"  # Custom icon preserved
        assert storefront.base_url == "https://custom.steamstore.com"  # Custom URL preserved



class TestSeedAllOfficialData:
    """Test seed_all_official_data() function."""
    
    def test_seed_all_data_empty_database(self, session: Session):
        """Test complete seeding process on empty database."""
        result = seed_all_official_data(session, "1.0.0")
        
        # Verify return structure
        assert isinstance(result, dict)
        assert "platforms" in result
        assert "storefronts" in result
        assert "associations" in result
        assert "total" in result
        
        # Verify counts
        assert result["platforms"] == len(OFFICIAL_PLATFORMS)
        assert result["storefronts"] == len(OFFICIAL_STOREFRONTS)
        assert result["associations"] == len(PLATFORM_STOREFRONT_ASSOCIATIONS)
        assert result["total"] == (
            len(OFFICIAL_PLATFORMS) + 
            len(OFFICIAL_STOREFRONTS) + 
            len(PLATFORM_STOREFRONT_ASSOCIATIONS)
        )
        
        # Verify data was actually seeded
        platforms = session.exec(select(Platform)).all()
        storefronts = session.exec(select(Storefront)).all()
        
        assert len(platforms) == len(OFFICIAL_PLATFORMS)
        assert len(storefronts) == len(OFFICIAL_STOREFRONTS)
        
        # Verify default storefronts were set for platforms that have them defined
        platforms_with_defaults = [p for p in platforms if p.default_storefront_id is not None]
        platforms_with_default_names = [p for p in OFFICIAL_PLATFORMS if "default_storefront_name" in p]
        assert len(platforms_with_defaults) == len(platforms_with_default_names)
    
    def test_seed_all_data_idempotent(self, session: Session):
        """Test that complete seeding process is idempotent."""
        # First seed
        result1 = seed_all_official_data(session, "1.0.0")
        
        # Second seed
        result2 = seed_all_official_data(session, "1.0.0")
        
        # Second seed should have created nothing new
        assert result2["platforms"] == 0
        assert result2["storefronts"] == 0
        assert result2["total"] == 0
        
        # Verify data integrity
        platforms = session.exec(select(Platform)).all()
        storefronts = session.exec(select(Storefront)).all()
        
        assert len(platforms) == len(OFFICIAL_PLATFORMS)
        assert len(storefronts) == len(OFFICIAL_STOREFRONTS)
    
    def test_seed_all_data_partial_existing(self, session: Session):
        """Test seeding when some data already exists."""
        # Pre-seed only platforms (without storefronts, they can't set defaults)
        seed_platforms(session, "1.0.0", set_defaults=False)
        
        # Seed all data
        result = seed_all_official_data(session, "1.0.0")
        
        # Should have seeded storefronts, updated platforms with defaults, and associations
        assert result["platforms"] == len(OFFICIAL_PLATFORMS)  # Platforms updated with defaults
        assert result["storefronts"] == len(OFFICIAL_STOREFRONTS)
        assert result["associations"] == len(PLATFORM_STOREFRONT_ASSOCIATIONS)


class TestGetSeededConflicts:
    """Test get_seeding_conflicts() function."""
    
    def test_conflicts_empty_database(self, session: Session):
        """Test conflicts detection on empty database."""
        conflicts = get_seeding_conflicts(session)
        
        assert isinstance(conflicts, dict)
        assert "platforms" in conflicts
        assert "storefronts" in conflicts
        assert len(conflicts["platforms"]) == 0
        assert len(conflicts["storefronts"]) == 0
    
    def test_conflicts_with_custom_data(self, session: Session):
        """Test conflicts detection with custom platforms/storefronts."""
        # Create custom platform with same name as official one
        custom_platform = Platform(
            name="pc-windows",
            display_name="Custom PC",
            source="custom"
        )
        session.add(custom_platform)
        
        # Create custom storefront with same name as official one
        custom_storefront = Storefront(
            name="steam",
            display_name="Custom Steam",
            source="custom"
        )
        session.add(custom_storefront)
        session.commit()
        
        conflicts = get_seeding_conflicts(session)
        
        assert "pc-windows" in conflicts["platforms"]
        assert "steam" in conflicts["storefronts"]
    
    def test_no_conflicts_with_official_data(self, session: Session):
        """Test that official data doesn't create conflicts."""
        # Seed official data
        seed_all_official_data(session, "1.0.0")
        
        conflicts = get_seeding_conflicts(session)
        
        # Should have no conflicts since all data is official
        assert len(conflicts["platforms"]) == 0
        assert len(conflicts["storefronts"]) == 0


class TestSeedDataEdgeCases:
    """Test edge cases and error handling."""
    
    def test_seed_with_different_versions(self, session: Session):
        """Test seeding with different version strings."""
        # Seed with version 1.0.0
        result1 = seed_all_official_data(session, "1.0.0")
        
        # Seed again with version 2.0.0
        result2 = seed_all_official_data(session, "2.0.0")
        
        # Should be idempotent regardless of version
        assert result2["platforms"] == 0
        assert result2["storefronts"] == 0
        
        # All platforms should still have version 1.0.0 (first seed)
        platforms = session.exec(select(Platform)).all()
        for platform in platforms:
            if platform.source == "official":
                assert platform.version_added == "1.0.0"
    
    @patch('nexorious.seed_data.seeder.logger')
    def test_logging_output(self, mock_logger, session: Session):
        """Test that appropriate logging occurs during seeding."""
        seed_all_official_data(session, "1.0.0")
        
        # Verify logging calls were made
        mock_logger.info.assert_called()
        
        # Should have logged start, platform seeding, storefront seeding, mappings, and completion
        info_calls = [call.args[0] for call in mock_logger.info.call_args_list]
        
        assert any("Starting seeding" in call for call in info_calls)
        assert any("Seeded" in call and "platforms" in call for call in info_calls)
        assert any("Seeded" in call and "storefronts" in call for call in info_calls)
        assert any("Created" in call and "associations" in call for call in info_calls)
        assert any("Completed seeding" in call for call in info_calls)


class TestSeedDataAPI:
    """Test the seed data API endpoint."""
    
    def test_seed_endpoint_requires_admin(self, client: TestClient, session: Session):
        """Test that seed endpoint requires admin authentication."""
        # Try without authentication
        response = client.post("/api/platforms/seed")
        assert response.status_code in [401, 403]  # Could be either unauthorized or forbidden
        
        # Try with non-admin user
        user_data = create_test_user_data(username="testuser", password="testpassword")
        headers = register_and_login_user(client, user_data)
        
        response = client.post("/api/platforms/seed", headers=headers)
        assert response.status_code == 403
    
    def test_seed_endpoint_success(self, client: TestClient, session: Session):
        """Test successful seed data loading via API."""
        # Create admin user and get headers
        headers = get_admin_headers(client, session)
        
        # Call seed endpoint
        response = client.post("/api/platforms/seed", headers=headers)
        assert response.status_code == 200
        
        data = response.json()
        assert "platforms_added" in data
        assert "storefronts_added" in data
        assert "mappings_created" in data
        assert "total_changes" in data
        assert "message" in data
        
        # Verify counts
        assert data["platforms_added"] == len(OFFICIAL_PLATFORMS)
        assert data["storefronts_added"] == len(OFFICIAL_STOREFRONTS)
        assert data["mappings_created"] == 0  # No longer creating separate mappings
        assert data["total_changes"] > 0
        assert "Successfully loaded seed data" in data["message"]
    
    def test_seed_endpoint_idempotent(self, client: TestClient, session: Session):
        """Test that seed endpoint is idempotent when called multiple times."""
        # Create admin user and get headers
        headers = get_admin_headers(client, session)
        
        # First call
        response1 = client.post("/api/platforms/seed", headers=headers)
        assert response1.status_code == 200
        data1 = response1.json()
        assert data1["total_changes"] > 0
        
        # Second call
        response2 = client.post("/api/platforms/seed", headers=headers)
        assert response2.status_code == 200
        data2 = response2.json()
        
        # Should have no changes on second call
        assert data2["platforms_added"] == 0
        assert data2["storefronts_added"] == 0
        assert data2["mappings_created"] == 0
        assert data2["total_changes"] == 0
        assert "No changes made" in data2["message"]
    
    def test_seed_endpoint_with_version_parameter(self, client: TestClient, session: Session):
        """Test seed endpoint with custom version parameter."""
        # Create admin user and get headers
        headers = get_admin_headers(client, session)
        
        # Call with custom version
        response = client.post("/api/platforms/seed?version=2.0.0", headers=headers)
        assert response.status_code == 200
        
        data = response.json()
        assert data["total_changes"] > 0
        
        # Verify platforms have the specified version
        platforms = session.exec(select(Platform).where(Platform.source == "official")).all()
        for platform in platforms:
            assert platform.version_added == "2.0.0"
    
    def test_initial_admin_setup_seeds_data(self, client: TestClient, session: Session):
        """Test that creating initial admin automatically seeds data."""
        # Ensure no users exist
        existing_users = session.exec(select(User)).all()
        assert len(existing_users) == 0
        
        # Create initial admin
        response = client.post(
            "/api/auth/setup/admin",
            json={"username": "admin", "password": "adminpassword"}
        )
        assert response.status_code == 201
        
        # Verify platforms and storefronts were seeded
        platforms = session.exec(select(Platform)).all()
        storefronts = session.exec(select(Storefront)).all()
        
        assert len(platforms) == len(OFFICIAL_PLATFORMS)
        assert len(storefronts) == len(OFFICIAL_STOREFRONTS)
        
        # Verify default storefronts were set for platforms that have them defined
        platforms_with_defaults = [p for p in platforms if p.default_storefront_id is not None]
        platforms_with_default_names = [p for p in OFFICIAL_PLATFORMS if "default_storefront_name" in p]
        assert len(platforms_with_defaults) == len(platforms_with_default_names)
    
    def test_seed_endpoint_with_existing_custom_data(self, client: TestClient, session: Session):
        """Test seed endpoint preserves existing custom data."""
        # Create admin user and get headers
        headers = get_admin_headers(client, session)
        
        # Create custom platform
        custom_platform = Platform(
            name="custom-platform",
            display_name="Custom Platform",
            source="custom"
        )
        session.add(custom_platform)
        session.commit()
        
        # Call seed endpoint
        response = client.post("/api/platforms/seed", headers=headers)
        assert response.status_code == 200
        
        # Verify custom platform still exists and is still custom
        custom = session.exec(
            select(Platform).where(Platform.name == "custom-platform")
        ).first()
        assert custom is not None
        assert custom.source == "custom"
        assert custom.display_name == "Custom Platform"
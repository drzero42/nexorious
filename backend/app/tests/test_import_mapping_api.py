"""
Integration tests for Import Mapping API endpoints.

Tests the following endpoints:
- GET /api/import-mappings - List user's import mappings
- GET /api/import-mappings/{id} - Get a specific mapping
- POST /api/import-mappings - Create a new mapping
- PUT /api/import-mappings/{id} - Update a mapping
- DELETE /api/import-mappings/{id} - Delete a mapping
- POST /api/import-mappings/batch - Create/update multiple mappings
"""

from sqlmodel import Session

from ..models.user import User
from ..models.user_import_mapping import UserImportMapping, ImportMappingType


class TestListImportMappings:
    """Tests for GET /api/import-mappings endpoint."""

    def test_list_mappings_empty(self, client, auth_headers, test_user: User):
        """Test listing mappings when user has none."""
        response = client.get("/api/import-mappings/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 0
        assert data["items"] == []

    def test_list_mappings_with_items(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test listing mappings with items."""
        # Create some mappings
        mapping1 = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        mapping2 = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.STOREFRONT,
            source_value="Steam",
            target_id="steam",
        )
        session.add_all([mapping1, mapping2])
        session.commit()

        response = client.get("/api/import-mappings/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 2
        assert len(data["items"]) == 2

    def test_list_mappings_filter_by_source(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test filtering mappings by import source."""
        # Create mappings for different sources
        mapping1 = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        mapping2 = UserImportMapping(
            user_id=test_user.id,
            import_source="other",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-linux",
        )
        session.add_all([mapping1, mapping2])
        session.commit()

        response = client.get(
            "/api/import-mappings/?import_source=darkadia", headers=auth_headers
        )
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["items"][0]["import_source"] == "darkadia"

    def test_list_mappings_filter_by_type(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test filtering mappings by mapping type."""
        # Create mappings for different types
        mapping1 = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        mapping2 = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.STOREFRONT,
            source_value="Steam",
            target_id="steam",
        )
        session.add_all([mapping1, mapping2])
        session.commit()

        response = client.get(
            "/api/import-mappings/?mapping_type=platform", headers=auth_headers
        )
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["items"][0]["mapping_type"] == "platform"

    def test_list_mappings_user_isolation(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users only see their own mappings."""
        # Create mapping for test user
        user_mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        # Create mapping for admin user
        admin_mapping = UserImportMapping(
            user_id=admin_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PS4",
            target_id="playstation-4",
        )
        session.add_all([user_mapping, admin_mapping])
        session.commit()

        response = client.get("/api/import-mappings/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["items"][0]["source_value"] == "PC"


class TestGetImportMapping:
    """Tests for GET /api/import-mappings/{id} endpoint."""

    def test_get_mapping_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test getting a specific mapping."""
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        session.add(mapping)
        session.commit()
        session.refresh(mapping)

        response = client.get(f"/api/import-mappings/{mapping.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["id"] == mapping.id
        assert data["source_value"] == "PC"
        assert data["target_id"] == "pc-windows"

    def test_get_mapping_not_found(self, client, auth_headers):
        """Test getting a non-existent mapping."""
        response = client.get(
            "/api/import-mappings/non-existent-id", headers=auth_headers
        )
        assert response.status_code == 404

    def test_get_mapping_wrong_user(
        self, client, auth_headers, admin_user: User, session: Session
    ):
        """Test that users cannot access other users' mappings."""
        # Create mapping for admin user
        mapping = UserImportMapping(
            user_id=admin_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        session.add(mapping)
        session.commit()
        session.refresh(mapping)

        # Try to access with test user's auth
        response = client.get(f"/api/import-mappings/{mapping.id}", headers=auth_headers)
        assert response.status_code == 404


class TestCreateImportMapping:
    """Tests for POST /api/import-mappings endpoint."""

    def test_create_mapping_success(self, client, auth_headers, test_user: User):
        """Test creating a new mapping."""
        response = client.post(
            "/api/import-mappings/",
            headers=auth_headers,
            json={
                "import_source": "darkadia",
                "mapping_type": "platform",
                "source_value": "PC",
                "target_id": "pc-windows",
            },
        )
        assert response.status_code == 201

        data = response.json()
        assert data["import_source"] == "darkadia"
        assert data["mapping_type"] == "platform"
        assert data["source_value"] == "PC"
        assert data["target_id"] == "pc-windows"
        assert "id" in data
        assert "created_at" in data

    def test_create_mapping_storefront(self, client, auth_headers):
        """Test creating a storefront mapping."""
        response = client.post(
            "/api/import-mappings/",
            headers=auth_headers,
            json={
                "import_source": "darkadia",
                "mapping_type": "storefront",
                "source_value": "Steam",
                "target_id": "steam",
            },
        )
        assert response.status_code == 201

        data = response.json()
        assert data["mapping_type"] == "storefront"

    def test_create_mapping_duplicate(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that duplicate mappings are rejected."""
        # Create existing mapping
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        session.add(mapping)
        session.commit()

        # Try to create duplicate
        response = client.post(
            "/api/import-mappings/",
            headers=auth_headers,
            json={
                "import_source": "darkadia",
                "mapping_type": "platform",
                "source_value": "PC",
                "target_id": "pc-linux",
            },
        )
        assert response.status_code == 409  # Conflict

    def test_create_mapping_invalid_type(self, client, auth_headers):
        """Test that invalid mapping type is rejected."""
        response = client.post(
            "/api/import-mappings/",
            headers=auth_headers,
            json={
                "import_source": "darkadia",
                "mapping_type": "invalid",
                "source_value": "PC",
                "target_id": "pc-windows",
            },
        )
        assert response.status_code == 422  # Validation error


class TestUpdateImportMapping:
    """Tests for PUT /api/import-mappings/{id} endpoint."""

    def test_update_mapping_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test updating a mapping."""
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        session.add(mapping)
        session.commit()
        session.refresh(mapping)

        response = client.put(
            f"/api/import-mappings/{mapping.id}",
            headers=auth_headers,
            json={"target_id": "pc-linux"},
        )
        assert response.status_code == 200

        data = response.json()
        assert data["target_id"] == "pc-linux"

    def test_update_mapping_not_found(self, client, auth_headers):
        """Test updating a non-existent mapping."""
        response = client.put(
            "/api/import-mappings/non-existent-id",
            headers=auth_headers,
            json={"target_id": "pc-linux"},
        )
        assert response.status_code == 404

    def test_update_mapping_wrong_user(
        self, client, auth_headers, admin_user: User, session: Session
    ):
        """Test that users cannot update other users' mappings."""
        mapping = UserImportMapping(
            user_id=admin_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        session.add(mapping)
        session.commit()
        session.refresh(mapping)

        response = client.put(
            f"/api/import-mappings/{mapping.id}",
            headers=auth_headers,
            json={"target_id": "pc-linux"},
        )
        assert response.status_code == 404


class TestDeleteImportMapping:
    """Tests for DELETE /api/import-mappings/{id} endpoint."""

    def test_delete_mapping_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test deleting a mapping."""
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        session.add(mapping)
        session.commit()
        session.refresh(mapping)

        response = client.delete(
            f"/api/import-mappings/{mapping.id}", headers=auth_headers
        )
        assert response.status_code == 204

        # Verify deleted
        response = client.get(f"/api/import-mappings/{mapping.id}", headers=auth_headers)
        assert response.status_code == 404

    def test_delete_mapping_not_found(self, client, auth_headers):
        """Test deleting a non-existent mapping."""
        response = client.delete(
            "/api/import-mappings/non-existent-id", headers=auth_headers
        )
        assert response.status_code == 404

    def test_delete_mapping_wrong_user(
        self, client, auth_headers, admin_user: User, session: Session
    ):
        """Test that users cannot delete other users' mappings."""
        mapping = UserImportMapping(
            user_id=admin_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        session.add(mapping)
        session.commit()
        session.refresh(mapping)

        response = client.delete(
            f"/api/import-mappings/{mapping.id}", headers=auth_headers
        )
        assert response.status_code == 404


class TestBatchImportMappings:
    """Tests for POST /api/import-mappings/batch endpoint."""

    def test_batch_create_mappings(self, client, auth_headers, test_user: User):
        """Test creating multiple mappings at once."""
        response = client.post(
            "/api/import-mappings/batch",
            headers=auth_headers,
            json={
                "import_source": "darkadia",
                "mappings": [
                    {
                        "mapping_type": "platform",
                        "source_value": "PC",
                        "target_id": "pc-windows",
                    },
                    {
                        "mapping_type": "platform",
                        "source_value": "PS4",
                        "target_id": "playstation-4",
                    },
                    {
                        "mapping_type": "storefront",
                        "source_value": "Steam",
                        "target_id": "steam",
                    },
                ],
            },
        )
        assert response.status_code == 200

        data = response.json()
        assert data["created"] == 3
        assert data["updated"] == 0

    def test_batch_update_existing(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that batch upserts existing mappings."""
        # Create existing mapping
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        session.add(mapping)
        session.commit()

        # Batch with existing and new
        response = client.post(
            "/api/import-mappings/batch",
            headers=auth_headers,
            json={
                "import_source": "darkadia",
                "mappings": [
                    {
                        "mapping_type": "platform",
                        "source_value": "PC",
                        "target_id": "pc-linux",  # Update existing
                    },
                    {
                        "mapping_type": "platform",
                        "source_value": "PS4",
                        "target_id": "playstation-4",  # New
                    },
                ],
            },
        )
        assert response.status_code == 200

        data = response.json()
        assert data["created"] == 1
        assert data["updated"] == 1

    def test_batch_empty_list(self, client, auth_headers):
        """Test batch with empty mappings list."""
        response = client.post(
            "/api/import-mappings/batch",
            headers=auth_headers,
            json={
                "import_source": "darkadia",
                "mappings": [],
            },
        )
        assert response.status_code == 200

        data = response.json()
        assert data["created"] == 0
        assert data["updated"] == 0


class TestGetMappingsForSource:
    """Tests for looking up mappings by source value."""

    def test_lookup_mapping_by_source_value(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test looking up a mapping by source value."""
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PlayStation 4",
            target_id="playstation-4",
        )
        session.add(mapping)
        session.commit()

        response = client.get(
            "/api/import-mappings/lookup",
            headers=auth_headers,
            params={
                "import_source": "darkadia",
                "mapping_type": "platform",
                "source_value": "PlayStation 4",
            },
        )
        assert response.status_code == 200

        data = response.json()
        assert data["target_id"] == "playstation-4"

    def test_lookup_mapping_not_found(self, client, auth_headers):
        """Test looking up a non-existent mapping."""
        response = client.get(
            "/api/import-mappings/lookup",
            headers=auth_headers,
            params={
                "import_source": "darkadia",
                "mapping_type": "platform",
                "source_value": "NonExistent",
            },
        )
        assert response.status_code == 404

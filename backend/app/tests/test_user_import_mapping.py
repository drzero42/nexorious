"""
Unit tests for UserImportMapping model.
Tests model validation, relationships, constraints, and database operations.
"""

import pytest
from datetime import datetime
from sqlmodel import Session, SQLModel, create_engine, select
from sqlmodel.pool import StaticPool
from sqlalchemy.exc import IntegrityError

from ..models.user import User
from ..models.user_import_mapping import UserImportMapping, ImportMappingType


@pytest.fixture(name="model_session")
def model_session_fixture():
    """Create a test database session for model tests."""
    engine = create_engine(
        "sqlite:///:memory:",
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
    )
    SQLModel.metadata.create_all(engine)
    with Session(engine) as session:
        yield session


@pytest.fixture(name="test_user")
def test_user_fixture(model_session: Session) -> User:
    """Create a test user for model tests."""
    user = User(
        username="test_user",
        password_hash="$2b$12$test_hash",
        is_active=True,
        is_admin=False,
    )
    model_session.add(user)
    model_session.commit()
    model_session.refresh(user)
    return user


@pytest.fixture(name="second_user")
def second_user_fixture(model_session: Session) -> User:
    """Create a second test user for isolation tests."""
    user = User(
        username="second_user",
        password_hash="$2b$12$test_hash_2",
        is_active=True,
        is_admin=False,
    )
    model_session.add(user)
    model_session.commit()
    model_session.refresh(user)
    return user


class TestUserImportMappingModel:
    """Test the UserImportMapping model."""

    def test_platform_mapping_creation(self, model_session: Session, test_user: User):
        """Test basic platform mapping creation."""
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )

        model_session.add(mapping)
        model_session.commit()
        model_session.refresh(mapping)

        assert mapping.id is not None
        assert mapping.user_id == test_user.id
        assert mapping.import_source == "darkadia"
        assert mapping.mapping_type == ImportMappingType.PLATFORM
        assert mapping.source_value == "PC"
        assert mapping.target_id == "pc-windows"
        assert mapping.created_at is not None
        assert mapping.updated_at is not None
        assert isinstance(mapping.created_at, datetime)
        assert isinstance(mapping.updated_at, datetime)

    def test_storefront_mapping_creation(self, model_session: Session, test_user: User):
        """Test basic storefront mapping creation."""
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.STOREFRONT,
            source_value="Steam",
            target_id="steam",
        )

        model_session.add(mapping)
        model_session.commit()
        model_session.refresh(mapping)

        assert mapping.id is not None
        assert mapping.mapping_type == ImportMappingType.STOREFRONT
        assert mapping.source_value == "Steam"
        assert mapping.target_id == "steam"

    def test_unique_constraint_same_user_source_type_value(
        self, model_session: Session, test_user: User
    ):
        """Test that mapping must be unique per user/source/type/value combination."""
        # Create first mapping
        mapping1 = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        model_session.add(mapping1)
        model_session.commit()

        # Try to create duplicate mapping
        mapping2 = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-linux",  # Different target, same source_value
        )
        model_session.add(mapping2)

        with pytest.raises(IntegrityError):
            model_session.commit()

    def test_same_value_different_type_allowed(
        self, model_session: Session, test_user: User
    ):
        """Test that same source_value can be used for different mapping types."""
        # Create platform mapping
        platform_mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="Steam",  # Using "Steam" as platform name
            target_id="pc-windows",
        )
        model_session.add(platform_mapping)
        model_session.commit()

        # Create storefront mapping with same source_value - should work
        storefront_mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.STOREFRONT,
            source_value="Steam",  # Same value but different type
            target_id="steam",
        )
        model_session.add(storefront_mapping)
        model_session.commit()
        model_session.refresh(storefront_mapping)

        assert platform_mapping.source_value == storefront_mapping.source_value
        assert platform_mapping.mapping_type != storefront_mapping.mapping_type

    def test_same_value_different_source_allowed(
        self, model_session: Session, test_user: User
    ):
        """Test that same source_value can be used for different import sources."""
        # Create mapping for darkadia
        darkadia_mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        model_session.add(darkadia_mapping)
        model_session.commit()

        # Create mapping for another source - should work
        other_mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="other_source",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-linux",
        )
        model_session.add(other_mapping)
        model_session.commit()
        model_session.refresh(other_mapping)

        assert darkadia_mapping.source_value == other_mapping.source_value
        assert darkadia_mapping.import_source != other_mapping.import_source

    def test_same_value_different_users_allowed(
        self, model_session: Session, test_user: User, second_user: User
    ):
        """Test that different users can have the same mapping."""
        # Create mapping for first user
        user1_mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        model_session.add(user1_mapping)
        model_session.commit()

        # Create same mapping for second user - should work
        user2_mapping = UserImportMapping(
            user_id=second_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-linux",  # Can even have different target
        )
        model_session.add(user2_mapping)
        model_session.commit()
        model_session.refresh(user2_mapping)

        assert user1_mapping.user_id != user2_mapping.user_id
        assert user1_mapping.source_value == user2_mapping.source_value

    def test_query_mappings_by_user_and_source(
        self, model_session: Session, test_user: User
    ):
        """Test querying mappings by user and import source."""
        # Create multiple mappings
        mappings = [
            UserImportMapping(
                user_id=test_user.id,
                import_source="darkadia",
                mapping_type=ImportMappingType.PLATFORM,
                source_value="PC",
                target_id="pc-windows",
            ),
            UserImportMapping(
                user_id=test_user.id,
                import_source="darkadia",
                mapping_type=ImportMappingType.PLATFORM,
                source_value="PS4",
                target_id="playstation-4",
            ),
            UserImportMapping(
                user_id=test_user.id,
                import_source="darkadia",
                mapping_type=ImportMappingType.STOREFRONT,
                source_value="Steam",
                target_id="steam",
            ),
            UserImportMapping(
                user_id=test_user.id,
                import_source="other_source",
                mapping_type=ImportMappingType.PLATFORM,
                source_value="PC",
                target_id="pc-linux",
            ),
        ]
        model_session.add_all(mappings)
        model_session.commit()

        # Query darkadia mappings for user
        darkadia_mappings = model_session.exec(
            select(UserImportMapping).where(
                UserImportMapping.user_id == test_user.id,
                UserImportMapping.import_source == "darkadia",
            )
        ).all()

        assert len(darkadia_mappings) == 3

        # Query only platform mappings
        platform_mappings = model_session.exec(
            select(UserImportMapping).where(
                UserImportMapping.user_id == test_user.id,
                UserImportMapping.import_source == "darkadia",
                UserImportMapping.mapping_type == ImportMappingType.PLATFORM,
            )
        ).all()

        assert len(platform_mappings) == 2
        source_values = {m.source_value for m in platform_mappings}
        assert source_values == {"PC", "PS4"}

    def test_query_mapping_by_source_value(
        self, model_session: Session, test_user: User
    ):
        """Test looking up a specific mapping by source value."""
        # Create mapping
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PlayStation 4",
            target_id="playstation-4",
        )
        model_session.add(mapping)
        model_session.commit()

        # Query by source value
        result = model_session.exec(
            select(UserImportMapping).where(
                UserImportMapping.user_id == test_user.id,
                UserImportMapping.import_source == "darkadia",
                UserImportMapping.mapping_type == ImportMappingType.PLATFORM,
                UserImportMapping.source_value == "PlayStation 4",
            )
        ).first()

        assert result is not None
        assert result.target_id == "playstation-4"

    def test_user_isolation(
        self, model_session: Session, test_user: User, second_user: User
    ):
        """Test that mappings are properly isolated between users."""
        # Create mappings for both users
        user1_mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        user2_mapping = UserImportMapping(
            user_id=second_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-linux",
        )
        model_session.add_all([user1_mapping, user2_mapping])
        model_session.commit()

        # Query for user 1
        user1_result = model_session.exec(
            select(UserImportMapping).where(
                UserImportMapping.user_id == test_user.id,
                UserImportMapping.source_value == "PC",
            )
        ).first()

        assert user1_result is not None
        assert user1_result.target_id == "pc-windows"

        # Query for user 2
        user2_result = model_session.exec(
            select(UserImportMapping).where(
                UserImportMapping.user_id == second_user.id,
                UserImportMapping.source_value == "PC",
            )
        ).first()

        assert user2_result is not None
        assert user2_result.target_id == "pc-linux"

    def test_update_mapping(self, model_session: Session, test_user: User):
        """Test updating an existing mapping."""
        # Create mapping
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        model_session.add(mapping)
        model_session.commit()
        model_session.refresh(mapping)

        original_updated_at = mapping.updated_at

        # Update target
        mapping.target_id = "pc-linux"
        model_session.add(mapping)
        model_session.commit()
        model_session.refresh(mapping)

        assert mapping.target_id == "pc-linux"
        # Note: updated_at won't auto-update without explicit logic
        # This would be handled in the service layer

    def test_delete_mapping(self, model_session: Session, test_user: User):
        """Test deleting a mapping."""
        # Create mapping
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        model_session.add(mapping)
        model_session.commit()
        mapping_id = mapping.id

        # Delete mapping
        model_session.delete(mapping)
        model_session.commit()

        # Verify deleted
        result = model_session.get(UserImportMapping, mapping_id)
        assert result is None

    def test_relationship_with_user(self, model_session: Session, test_user: User):
        """Test mapping relationship with user."""
        mapping = UserImportMapping(
            user_id=test_user.id,
            import_source="darkadia",
            mapping_type=ImportMappingType.PLATFORM,
            source_value="PC",
            target_id="pc-windows",
        )
        model_session.add(mapping)
        model_session.commit()
        model_session.refresh(mapping)

        # Test forward relationship
        assert mapping.user is not None
        assert mapping.user.id == test_user.id
        assert mapping.user.username == test_user.username

    def test_bulk_create_mappings(self, model_session: Session, test_user: User):
        """Test creating multiple mappings at once."""
        mappings = [
            UserImportMapping(
                user_id=test_user.id,
                import_source="darkadia",
                mapping_type=ImportMappingType.PLATFORM,
                source_value="PC",
                target_id="pc-windows",
            ),
            UserImportMapping(
                user_id=test_user.id,
                import_source="darkadia",
                mapping_type=ImportMappingType.PLATFORM,
                source_value="PS4",
                target_id="playstation-4",
            ),
            UserImportMapping(
                user_id=test_user.id,
                import_source="darkadia",
                mapping_type=ImportMappingType.PLATFORM,
                source_value="Xbox One",
                target_id="xbox-one",
            ),
            UserImportMapping(
                user_id=test_user.id,
                import_source="darkadia",
                mapping_type=ImportMappingType.STOREFRONT,
                source_value="Steam",
                target_id="steam",
            ),
            UserImportMapping(
                user_id=test_user.id,
                import_source="darkadia",
                mapping_type=ImportMappingType.STOREFRONT,
                source_value="Epic Games",
                target_id="epic-games-store",
            ),
        ]

        model_session.add_all(mappings)
        model_session.commit()

        # Verify all created
        all_mappings = model_session.exec(
            select(UserImportMapping).where(UserImportMapping.user_id == test_user.id)
        ).all()

        assert len(all_mappings) == 5

        platform_count = sum(
            1 for m in all_mappings if m.mapping_type == ImportMappingType.PLATFORM
        )
        storefront_count = sum(
            1 for m in all_mappings if m.mapping_type == ImportMappingType.STOREFRONT
        )

        assert platform_count == 3
        assert storefront_count == 2

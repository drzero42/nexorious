"""
Unit tests for TagService business logic.
Tests all CRUD operations, user isolation, validation, and edge cases.
"""

import pytest
from sqlmodel import Session, SQLModel, create_engine, select
from sqlmodel.pool import StaticPool
from unittest.mock import Mock

from ..models.tag import Tag, UserGameTag
from ..models.user import User
from ..models.game import Game
from ..models.user_game import UserGame
from ..services.tag_service import TagService
from ..api.schemas.tag import TagCreateRequest, TagUpdateRequest
from .integration_test_utils import create_test_games


@pytest.fixture(name="service_session")
def service_session_fixture():
    """Create a test database session for service tests."""
    engine = create_engine(
        "sqlite:///:memory:",
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
    )
    SQLModel.metadata.create_all(engine)
    with Session(engine) as session:
        yield session


@pytest.fixture(name="tag_service")
def tag_service_fixture(service_session: Session) -> TagService:
    """Create a TagService instance."""
    return TagService(service_session)


@pytest.fixture(name="test_user_for_service")
def test_user_for_service_fixture(service_session: Session) -> User:
    """Create a test user for service tests."""
    user = User(
        username="service_user",
        password_hash="$2b$12$test_hash",
        is_active=True,
        is_admin=False
    )
    service_session.add(user)
    service_session.commit()
    service_session.refresh(user)
    return user


@pytest.fixture(name="second_service_user")
def second_service_user_fixture(service_session: Session) -> User:
    """Create a second test user for isolation tests."""
    user = User(
        username="second_service_user",
        password_hash="$2b$12$test_hash_2",
        is_active=True,
        is_admin=False
    )
    service_session.add(user)
    service_session.commit()
    service_session.refresh(user)
    return user


@pytest.fixture(name="test_tag")
def test_tag_fixture(service_session: Session, test_user_for_service: User) -> Tag:
    """Create a test tag."""
    tag = Tag(
        user_id=test_user_for_service.id,
        name="Test Tag",
        color="#FF0000",
        description="A test tag"
    )
    service_session.add(tag)
    service_session.commit()
    service_session.refresh(tag)
    return tag


@pytest.fixture(name="test_games_for_service")
def test_games_for_service_fixture(service_session: Session) -> list[Game]:
    """Create test games for service tests."""
    games = create_test_games(count=3, session=service_session, commit=True)
    return games


@pytest.fixture(name="test_user_games")
def test_user_games_fixture(service_session: Session, test_user_for_service: User, test_games_for_service: list[Game]) -> list[UserGame]:
    """Create test user games."""
    user_games = []
    for i, game in enumerate(test_games_for_service):
        user_game = UserGame(
            user_id=test_user_for_service.id,
            game_id=game.id,
            ownership_status="owned",
            play_status="not_started"
        )
        service_session.add(user_game)
        user_games.append(user_game)
    
    service_session.commit()
    for user_game in user_games:
        service_session.refresh(user_game)
    
    return user_games


class TestTagServiceCRUDOperations:
    """Test basic CRUD operations for tags."""

    def test_create_tag_success(self, tag_service: TagService, test_user_for_service: User):
        """Test successful tag creation."""
        tag_data = TagCreateRequest(
            name="Action Games",
            color="#FF0000",
            description="Fast-paced action games"
        )
        
        tag = tag_service.create_tag(tag_data, test_user_for_service.id)
        
        assert tag is not None
        assert tag.id is not None
        assert tag.user_id == test_user_for_service.id
        assert tag.name == "Action Games"
        assert tag.color == "#FF0000"
        assert tag.description == "Fast-paced action games"
        assert tag.created_at is not None
        assert tag.updated_at is not None
        assert hasattr(tag, 'game_count')
        assert tag.game_count == 0

    def test_create_tag_with_defaults(self, tag_service: TagService, test_user_for_service: User):
        """Test tag creation with default values."""
        tag_data = TagCreateRequest(name="Simple Tag")
        
        tag = tag_service.create_tag(tag_data, test_user_for_service.id)
        
        assert tag.name == "Simple Tag"
        assert tag.color == "#6B7280"  # Default gray
        assert tag.description is None

    def test_create_tag_duplicate_name_same_user(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test that duplicate tag names for the same user are not allowed."""
        tag_data = TagCreateRequest(name=test_tag.name)
        
        with pytest.raises(ValueError) as exc_info:
            tag_service.create_tag(tag_data, test_user_for_service.id)
        
        assert "already exists" in str(exc_info.value)

    def test_create_tag_same_name_different_users(self, tag_service: TagService, test_user_for_service: User, second_service_user: User):
        """Test that the same tag name can exist for different users."""
        tag_data = TagCreateRequest(name="Common Name")
        
        # Create tag for first user
        tag1 = tag_service.create_tag(tag_data, test_user_for_service.id)
        assert tag1.name == "Common Name"
        
        # Create tag with same name for second user - should work
        tag2 = tag_service.create_tag(tag_data, second_service_user.id)
        assert tag2.name == "Common Name"
        assert tag1.id != tag2.id
        assert tag1.user_id != tag2.user_id

    def test_get_tag_by_id_success(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test successful tag retrieval by ID."""
        tag = tag_service.get_tag_by_id(test_tag.id, test_user_for_service.id)
        
        assert tag is not None
        assert tag.id == test_tag.id
        assert tag.name == test_tag.name
        assert tag.user_id == test_user_for_service.id
        assert hasattr(tag, 'game_count')

    def test_get_tag_by_id_wrong_user(self, tag_service: TagService, second_service_user: User, test_tag: Tag):
        """Test that users cannot access tags belonging to other users."""
        tag = tag_service.get_tag_by_id(test_tag.id, second_service_user.id)
        assert tag is None

    def test_get_tag_by_id_nonexistent(self, tag_service: TagService, test_user_for_service: User):
        """Test retrieval of nonexistent tag."""
        tag = tag_service.get_tag_by_id("nonexistent-id", test_user_for_service.id)
        assert tag is None

    def test_get_tag_by_name_success(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test successful tag retrieval by name."""
        tag = tag_service.get_tag_by_name(test_tag.name, test_user_for_service.id)
        
        assert tag is not None
        assert tag.id == test_tag.id
        assert tag.name == test_tag.name

    def test_get_tag_by_name_case_insensitive(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test that tag name lookup is case-insensitive."""
        tag = tag_service.get_tag_by_name(test_tag.name.upper(), test_user_for_service.id)
        
        assert tag is not None
        assert tag.id == test_tag.id

    def test_get_tag_by_name_wrong_user(self, tag_service: TagService, second_service_user: User, test_tag: Tag):
        """Test that users cannot find tags belonging to other users by name."""
        tag = tag_service.get_tag_by_name(test_tag.name, second_service_user.id)
        assert tag is None

    def test_update_tag_success(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test successful tag update."""
        update_data = TagUpdateRequest(
            name="Updated Name",
            color="#00FF00",
            description="Updated description"
        )
        
        updated_tag = tag_service.update_tag(test_tag.id, update_data, test_user_for_service.id)
        
        assert updated_tag.id == test_tag.id
        assert updated_tag.name == "Updated Name"
        assert updated_tag.color == "#00FF00"
        assert updated_tag.description == "Updated description"
        assert updated_tag.updated_at >= test_tag.updated_at  # Should be updated or at least not older

    def test_update_tag_partial(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test partial tag update."""
        update_data = TagUpdateRequest(color="#00FF00")  # Only update color
        
        updated_tag = tag_service.update_tag(test_tag.id, update_data, test_user_for_service.id)
        
        assert updated_tag.name == test_tag.name  # Unchanged
        assert updated_tag.color == "#00FF00"  # Changed
        assert updated_tag.description == test_tag.description  # Unchanged

    def test_update_tag_duplicate_name(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag, service_session: Session):
        """Test that updating to an existing tag name is not allowed."""
        # Create second tag
        second_tag = Tag(
            user_id=test_user_for_service.id,
            name="Second Tag",
            color="#0000FF"
        )
        service_session.add(second_tag)
        service_session.commit()
        service_session.refresh(second_tag)
        
        # Try to update test_tag to have second tag's name
        update_data = TagUpdateRequest(name="Second Tag")
        
        with pytest.raises(ValueError) as exc_info:
            tag_service.update_tag(test_tag.id, update_data, test_user_for_service.id)
        
        assert "already exists" in str(exc_info.value)

    def test_update_tag_nonexistent(self, tag_service: TagService, test_user_for_service: User):
        """Test updating a nonexistent tag."""
        update_data = TagUpdateRequest(name="Updated Name")
        
        with pytest.raises(ValueError) as exc_info:
            tag_service.update_tag("nonexistent-id", update_data, test_user_for_service.id)
        
        assert "not found" in str(exc_info.value)

    def test_update_tag_wrong_user(self, tag_service: TagService, second_service_user: User, test_tag: Tag):
        """Test that users cannot update tags belonging to other users."""
        update_data = TagUpdateRequest(name="Hacked Name")
        
        with pytest.raises(ValueError) as exc_info:
            tag_service.update_tag(test_tag.id, update_data, second_service_user.id)
        
        assert "not found" in str(exc_info.value)

    def test_delete_tag_success(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test successful tag deletion."""
        result = tag_service.delete_tag(test_tag.id, test_user_for_service.id)
        
        assert result is True
        
        # Verify tag is deleted
        deleted_tag = tag_service.get_tag_by_id(test_tag.id, test_user_for_service.id)
        assert deleted_tag is None

    def test_delete_tag_nonexistent(self, tag_service: TagService, test_user_for_service: User):
        """Test deleting a nonexistent tag."""
        result = tag_service.delete_tag("nonexistent-id", test_user_for_service.id)
        assert result is False

    def test_delete_tag_wrong_user(self, tag_service: TagService, second_service_user: User, test_tag: Tag):
        """Test that users cannot delete tags belonging to other users."""
        result = tag_service.delete_tag(test_tag.id, second_service_user.id)
        assert result is False
        
        # Verify tag still exists
        tag = tag_service.get_tag_by_id(test_tag.id, test_tag.user_id)
        assert tag is not None


class TestTagServiceListingAndPagination:
    """Test tag listing and pagination functionality."""

    def test_get_user_tags_empty(self, tag_service: TagService, test_user_for_service: User):
        """Test getting tags for user with no tags."""
        tags, total = tag_service.get_user_tags(test_user_for_service.id)
        
        assert len(tags) == 0
        assert total == 0

    def test_get_user_tags_multiple(self, tag_service: TagService, test_user_for_service: User, service_session: Session):
        """Test getting multiple tags for a user."""
        # Create multiple tags
        tags_data = [
            Tag(user_id=test_user_for_service.id, name="Action", color="#FF0000"),
            Tag(user_id=test_user_for_service.id, name="RPG", color="#00FF00"),
            Tag(user_id=test_user_for_service.id, name="Strategy", color="#0000FF")
        ]
        
        service_session.add_all(tags_data)
        service_session.commit()
        
        tags, total = tag_service.get_user_tags(test_user_for_service.id)
        
        assert len(tags) == 3
        assert total == 3
        
        # Tags should be sorted by name
        tag_names = [tag.name for tag in tags]
        assert tag_names == ["Action", "RPG", "Strategy"]

    def test_get_user_tags_pagination(self, tag_service: TagService, test_user_for_service: User, service_session: Session):
        """Test tag listing with pagination."""
        # Create 5 tags
        for i in range(5):
            tag = Tag(user_id=test_user_for_service.id, name=f"Tag {i:02d}")
            service_session.add(tag)
        service_session.commit()
        
        # Get first page (2 items)
        tags_p1, total = tag_service.get_user_tags(test_user_for_service.id, page=1, per_page=2)
        assert len(tags_p1) == 2
        assert total == 5
        
        # Get second page (2 items)
        tags_p2, total = tag_service.get_user_tags(test_user_for_service.id, page=2, per_page=2)
        assert len(tags_p2) == 2
        assert total == 5
        
        # Get third page (1 item)
        tags_p3, total = tag_service.get_user_tags(test_user_for_service.id, page=3, per_page=2)
        assert len(tags_p3) == 1
        assert total == 5
        
        # Verify no overlap
        all_tag_ids = {tag.id for tag in tags_p1 + tags_p2 + tags_p3}
        assert len(all_tag_ids) == 5

    def test_get_user_tags_user_isolation(self, tag_service: TagService, test_user_for_service: User, second_service_user: User, service_session: Session):
        """Test that users only see their own tags."""
        # Create tags for first user
        user1_tags = [
            Tag(user_id=test_user_for_service.id, name="User1 Tag1"),
            Tag(user_id=test_user_for_service.id, name="User1 Tag2")
        ]
        
        # Create tags for second user
        user2_tags = [
            Tag(user_id=second_service_user.id, name="User2 Tag1"),
            Tag(user_id=second_service_user.id, name="User2 Tag2"),
            Tag(user_id=second_service_user.id, name="User2 Tag3")
        ]
        
        service_session.add_all(user1_tags + user2_tags)
        service_session.commit()
        
        # User 1 should only see their tags
        user1_result, user1_total = tag_service.get_user_tags(test_user_for_service.id)
        assert len(user1_result) == 2
        assert user1_total == 2
        
        # User 2 should only see their tags
        user2_result, user2_total = tag_service.get_user_tags(second_service_user.id)
        assert len(user2_result) == 3
        assert user2_total == 3


class TestTagServiceCreateOrGet:
    """Test create-or-get functionality for inline tag creation."""

    def test_create_or_get_new_tag(self, tag_service: TagService, test_user_for_service: User):
        """Test creating a new tag with create_or_get."""
        tag, was_created = tag_service.create_or_get_tag("New Tag", test_user_for_service.id, "#FF0000")
        
        assert was_created is True
        assert tag.name == "New Tag"
        assert tag.color == "#FF0000"
        assert tag.user_id == test_user_for_service.id

    def test_create_or_get_existing_tag(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test getting an existing tag with create_or_get."""
        tag, was_created = tag_service.create_or_get_tag(test_tag.name, test_user_for_service.id, "#00FF00")
        
        assert was_created is False
        assert tag.id == test_tag.id
        assert tag.name == test_tag.name
        assert tag.color == test_tag.color  # Should keep original color

    def test_create_or_get_case_insensitive(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test that create_or_get is case-insensitive."""
        tag, was_created = tag_service.create_or_get_tag(test_tag.name.upper(), test_user_for_service.id)
        
        assert was_created is False
        assert tag.id == test_tag.id

    def test_create_or_get_with_default_color(self, tag_service: TagService, test_user_for_service: User):
        """Test create_or_get with default color."""
        tag, was_created = tag_service.create_or_get_tag("Default Color Tag", test_user_for_service.id)
        
        assert was_created is True
        assert tag.color == "#6B7280"  # Default gray


class TestTagServiceAssignments:
    """Test tag assignment and removal functionality."""

    def test_assign_tags_to_game_success(self, tag_service: TagService, test_user_for_service: User, test_user_games: list[UserGame], service_session: Session):
        """Test successful tag assignment to a game."""
        # Create tags
        tag1 = Tag(user_id=test_user_for_service.id, name="Action")
        tag2 = Tag(user_id=test_user_for_service.id, name="RPG")
        service_session.add_all([tag1, tag2])
        service_session.commit()
        
        user_game = test_user_games[0]
        tag_ids = [tag1.id, tag2.id]
        
        associations = tag_service.assign_tags_to_game(user_game.id, tag_ids, test_user_for_service.id)
        
        assert len(associations) == 2
        
        # Verify associations were created
        game_tags = service_session.exec(
            select(UserGameTag).where(UserGameTag.user_game_id == user_game.id)
        ).all()
        assert len(game_tags) == 2

    def test_assign_tags_duplicate_assignment(self, tag_service: TagService, test_user_for_service: User, test_user_games: list[UserGame], service_session: Session):
        """Test that duplicate tag assignments are handled gracefully."""
        # Create tag
        tag = Tag(user_id=test_user_for_service.id, name="Action")
        service_session.add(tag)
        service_session.commit()
        
        user_game = test_user_games[0]
        
        # First assignment
        associations1 = tag_service.assign_tags_to_game(user_game.id, [tag.id], test_user_for_service.id)
        assert len(associations1) == 1
        
        # Second assignment (duplicate)
        associations2 = tag_service.assign_tags_to_game(user_game.id, [tag.id], test_user_for_service.id)
        assert len(associations2) == 0  # No new associations created
        
        # Verify only one association exists
        game_tags = service_session.exec(
            select(UserGameTag).where(UserGameTag.user_game_id == user_game.id)
        ).all()
        assert len(game_tags) == 1

    def test_assign_tags_nonexistent_game(self, tag_service: TagService, test_user_for_service: User, test_tag: Tag):
        """Test assigning tags to a nonexistent game."""
        with pytest.raises(ValueError) as exc_info:
            tag_service.assign_tags_to_game("nonexistent-game", [test_tag.id], test_user_for_service.id)
        
        assert "not found" in str(exc_info.value)

    def test_assign_tags_nonexistent_tags(self, tag_service: TagService, test_user_for_service: User, test_user_games: list[UserGame]):
        """Test assigning nonexistent tags to a game."""
        user_game = test_user_games[0]
        
        with pytest.raises(ValueError) as exc_info:
            tag_service.assign_tags_to_game(user_game.id, ["nonexistent-tag"], test_user_for_service.id)
        
        assert "not found" in str(exc_info.value)

    def test_assign_tags_wrong_user_game(self, tag_service: TagService, second_service_user: User, test_user_games: list[UserGame], test_tag: Tag):
        """Test that users cannot assign tags to games they don't own."""
        user_game = test_user_games[0]  # Belongs to test_user_for_service
        
        with pytest.raises(ValueError) as exc_info:
            tag_service.assign_tags_to_game(user_game.id, [test_tag.id], second_service_user.id)
        
        assert "not found" in str(exc_info.value)

    def test_assign_tags_wrong_user_tags(self, tag_service: TagService, test_user_for_service: User, second_service_user: User, test_user_games: list[UserGame], test_tag: Tag, service_session: Session):
        """Test that users cannot assign tags belonging to other users."""
        # Create game for second user
        second_user_game = UserGame(
            user_id=second_service_user.id,
            game_id=test_user_games[0].game_id,  # Same game, different user
            ownership_status="owned",
            play_status="not_started"
        )
        service_session.add(second_user_game)
        service_session.commit()
        service_session.refresh(second_user_game)
        
        # Try to assign first user's tag to second user's game
        with pytest.raises(ValueError) as exc_info:
            tag_service.assign_tags_to_game(second_user_game.id, [test_tag.id], second_service_user.id)
        
        assert "not found" in str(exc_info.value)

    def test_remove_tags_from_game_success(self, tag_service: TagService, test_user_for_service: User, test_user_games: list[UserGame], service_session: Session):
        """Test successful tag removal from a game."""
        # Create and assign tags
        tag1 = Tag(user_id=test_user_for_service.id, name="Action")
        tag2 = Tag(user_id=test_user_for_service.id, name="RPG")
        service_session.add_all([tag1, tag2])
        service_session.commit()
        
        user_game = test_user_games[0]
        
        # Assign tags
        tag_service.assign_tags_to_game(user_game.id, [tag1.id, tag2.id], test_user_for_service.id)
        
        # Remove one tag
        removed_count = tag_service.remove_tags_from_game(user_game.id, [tag1.id], test_user_for_service.id)
        
        assert removed_count == 1
        
        # Verify only one tag remains
        remaining_tags = service_session.exec(
            select(UserGameTag).where(UserGameTag.user_game_id == user_game.id)
        ).all()
        assert len(remaining_tags) == 1
        assert remaining_tags[0].tag_id == tag2.id

    def test_remove_tags_nonexistent_associations(self, tag_service: TagService, test_user_for_service: User, test_user_games: list[UserGame], test_tag: Tag):
        """Test removing tags that aren't assigned to a game."""
        user_game = test_user_games[0]
        
        removed_count = tag_service.remove_tags_from_game(user_game.id, [test_tag.id], test_user_for_service.id)
        
        assert removed_count == 0

    def test_remove_tags_wrong_user_game(self, tag_service: TagService, second_service_user: User, test_user_games: list[UserGame], test_tag: Tag):
        """Test that users cannot remove tags from games they don't own."""
        user_game = test_user_games[0]  # Belongs to test_user_for_service
        
        with pytest.raises(ValueError) as exc_info:
            tag_service.remove_tags_from_game(user_game.id, [test_tag.id], second_service_user.id)
        
        assert "not found" in str(exc_info.value)


class TestTagServiceUsageStats:
    """Test tag usage statistics functionality."""

    def test_get_tag_usage_stats_empty(self, tag_service: TagService, test_user_for_service: User):
        """Test usage stats for user with no tags."""
        stats = tag_service.get_tag_usage_stats(test_user_for_service.id)
        
        assert stats["total_tags"] == 0
        assert stats["total_tagged_games"] == 0
        assert stats["average_tags_per_game"] == 0.0
        assert stats["tag_usage"] == {}
        assert len(stats["popular_tags"]) == 0
        assert len(stats["unused_tags"]) == 0

    def test_get_tag_usage_stats_with_data(self, tag_service: TagService, test_user_for_service: User, test_user_games: list[UserGame], service_session: Session):
        """Test comprehensive usage stats with data."""
        # Create tags
        tag1 = Tag(user_id=test_user_for_service.id, name="Popular Tag")
        tag2 = Tag(user_id=test_user_for_service.id, name="Less Popular")
        tag3 = Tag(user_id=test_user_for_service.id, name="Unused Tag")
        service_session.add_all([tag1, tag2, tag3])
        service_session.commit()
        
        # Assign tags to games
        tag_service.assign_tags_to_game(test_user_games[0].id, [tag1.id, tag2.id], test_user_for_service.id)
        tag_service.assign_tags_to_game(test_user_games[1].id, [tag1.id], test_user_for_service.id)
        tag_service.assign_tags_to_game(test_user_games[2].id, [tag1.id], test_user_for_service.id)
        # tag3 is not assigned to any games
        
        stats = tag_service.get_tag_usage_stats(test_user_for_service.id)
        
        assert stats["total_tags"] == 3
        assert stats["total_tagged_games"] == 3  # All 3 games have at least one tag
        assert stats["average_tags_per_game"] == 1.33  # (2+1+1)/3 = 1.33
        
        # Check tag usage counts
        assert stats["tag_usage"][tag1.id] == 3  # Used 3 times
        assert stats["tag_usage"][tag2.id] == 1  # Used 1 time
        assert stats["tag_usage"][tag3.id] == 0  # Unused
        
        # Check popular tags (sorted by usage)
        popular_tags = stats["popular_tags"]
        assert len(popular_tags) == 2  # Only tags with usage > 0
        assert popular_tags[0].id == tag1.id  # Most popular first
        assert popular_tags[1].id == tag2.id
        
        # Check unused tags
        unused_tags = stats["unused_tags"]
        assert len(unused_tags) == 1
        assert unused_tags[0].id == tag3.id

    def test_get_tag_usage_stats_user_isolation(self, tag_service: TagService, test_user_for_service: User, second_service_user: User, test_user_games: list[UserGame], service_session: Session):
        """Test that usage stats are isolated per user."""
        # Create games for second user
        second_user_game = UserGame(
            user_id=second_service_user.id,
            game_id=test_user_games[0].game_id,
            ownership_status="owned",
            play_status="not_started"
        )
        service_session.add(second_user_game)
        service_session.commit()
        
        # Create tags for both users
        user1_tag = Tag(user_id=test_user_for_service.id, name="User1 Tag")
        user2_tag = Tag(user_id=second_service_user.id, name="User2 Tag")
        service_session.add_all([user1_tag, user2_tag])
        service_session.commit()
        
        # Assign tags to respective games
        tag_service.assign_tags_to_game(test_user_games[0].id, [user1_tag.id], test_user_for_service.id)
        tag_service.assign_tags_to_game(second_user_game.id, [user2_tag.id], second_service_user.id)
        
        # Check stats for first user
        stats1 = tag_service.get_tag_usage_stats(test_user_for_service.id)
        assert stats1["total_tags"] == 1
        assert stats1["total_tagged_games"] == 1
        assert user1_tag.id in stats1["tag_usage"]
        assert user2_tag.id not in stats1["tag_usage"]
        
        # Check stats for second user
        stats2 = tag_service.get_tag_usage_stats(second_service_user.id)
        assert stats2["total_tags"] == 1
        assert stats2["total_tagged_games"] == 1
        assert user2_tag.id in stats2["tag_usage"]
        assert user1_tag.id not in stats2["tag_usage"]


class TestTagServiceErrorHandling:
    """Test error handling and edge cases."""

    def test_service_database_error_handling(self, service_session: Session, test_user_for_service: User):
        """Test that database errors are properly handled."""
        # Create service with mock session that raises errors
        mock_session = Mock(spec=Session)
        mock_session.exec.side_effect = Exception("Database error")
        
        tag_service = TagService(mock_session)
        
        # Should raise the database exception
        with pytest.raises(Exception) as exc_info:
            tag_service.get_user_tags(test_user_for_service.id)
        
        assert "Database error" in str(exc_info.value)

    def test_create_tag_with_stripped_whitespace(self, tag_service: TagService, test_user_for_service: User):
        """Test that tag names are properly stripped of whitespace."""
        tag_data = TagCreateRequest(name="  Trimmed Name  ")
        
        tag = tag_service.create_tag(tag_data, test_user_for_service.id)
        
        assert tag.name == "Trimmed Name"

    def test_get_tag_game_count_accuracy(self, tag_service: TagService, test_user_for_service: User, test_user_games: list[UserGame], service_session: Session):
        """Test that game count is accurately calculated."""
        # Create tag
        tag = Tag(user_id=test_user_for_service.id, name="Test Tag")
        service_session.add(tag)
        service_session.commit()
        
        # Initially should have 0 games
        retrieved_tag = tag_service.get_tag_by_id(tag.id, test_user_for_service.id)
        assert retrieved_tag.game_count == 0
        
        # Assign to 2 games
        tag_service.assign_tags_to_game(test_user_games[0].id, [tag.id], test_user_for_service.id)
        tag_service.assign_tags_to_game(test_user_games[1].id, [tag.id], test_user_for_service.id)
        
        # Should now have 2 games
        retrieved_tag = tag_service.get_tag_by_id(tag.id, test_user_for_service.id)
        assert retrieved_tag.game_count == 2
        
        # Remove from 1 game
        tag_service.remove_tags_from_game(test_user_games[0].id, [tag.id], test_user_for_service.id)
        
        # Should now have 1 game
        retrieved_tag = tag_service.get_tag_by_id(tag.id, test_user_for_service.id)
        assert retrieved_tag.game_count == 1

    def test_delete_tag_cascades_associations(self, tag_service: TagService, test_user_for_service: User, test_user_games: list[UserGame], service_session: Session):
        """Test that deleting a tag removes all its associations."""
        # Create tag and assign to multiple games
        tag = Tag(user_id=test_user_for_service.id, name="To Delete")
        service_session.add(tag)
        service_session.commit()
        
        # Assign to all test games
        for user_game in test_user_games:
            tag_service.assign_tags_to_game(user_game.id, [tag.id], test_user_for_service.id)
        
        # Verify associations exist
        associations_before = service_session.exec(
            select(UserGameTag).where(UserGameTag.tag_id == tag.id)
        ).all()
        assert len(associations_before) == len(test_user_games)
        
        # Delete the tag
        result = tag_service.delete_tag(tag.id, test_user_for_service.id)
        assert result is True
        
        # Verify associations are deleted
        associations_after = service_session.exec(
            select(UserGameTag).where(UserGameTag.tag_id == tag.id)
        ).all()
        assert len(associations_after) == 0
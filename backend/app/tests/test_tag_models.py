"""
Unit tests for Tag and UserGameTag models.
Tests model validation, relationships, constraints, and database operations.
"""

import pytest
from datetime import datetime
from sqlmodel import Session, select, and_
from sqlalchemy.exc import IntegrityError

from ..models.tag import Tag, UserGameTag
from ..models.user import User
from ..models.game import Game
from ..models.user_game import UserGame
from .integration_test_utils import create_test_game
from ..utils.sqlalchemy_typed import in_


@pytest.fixture(name="model_session")
def model_session_fixture(session):
    """Use the shared PostgreSQL test session."""
    return session


@pytest.fixture(name="test_user_for_model")
def test_user_for_model_fixture(model_session: Session) -> User:
    """Create a test user for model tests."""
    user = User(
        username="test_user",
        password_hash="$2b$12$test_hash",
        is_active=True,
        is_admin=False
    )
    model_session.add(user)
    model_session.commit()
    model_session.refresh(user)
    return user


@pytest.fixture(name="second_test_user")
def second_test_user_fixture(model_session: Session) -> User:
    """Create a second test user for isolation tests."""
    user = User(
        username="second_user",
        password_hash="$2b$12$test_hash_2",
        is_active=True,
        is_admin=False
    )
    model_session.add(user)
    model_session.commit()
    model_session.refresh(user)
    return user


@pytest.fixture(name="test_game_for_model")
def test_game_for_model_fixture(model_session: Session) -> Game:
    """Create a test game for model tests."""
    game = create_test_game(title="Test Game", igdb_id=1001)
    model_session.add(game)
    model_session.commit()
    model_session.refresh(game)
    return game


@pytest.fixture(name="test_user_game_for_model")
def test_user_game_for_model_fixture(model_session: Session, test_user_for_model: User, test_game_for_model: Game) -> UserGame:
    """Create a test user game for model tests."""
    user_game = UserGame(
        user_id=test_user_for_model.id,
        game_id=test_game_for_model.id,
        ownership_status="owned",
        play_status="not_started"
    )
    model_session.add(user_game)
    model_session.commit()
    model_session.refresh(user_game)
    return user_game


class TestTagModel:
    """Test the Tag model."""

    def test_tag_creation(self, model_session: Session, test_user_for_model: User):
        """Test basic tag creation."""
        tag = Tag(
            user_id=test_user_for_model.id,
            name="Action Games",
            color="#FF0000",
            description="Games with action gameplay"
        )
        
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        assert tag.id is not None
        assert tag.user_id == test_user_for_model.id
        assert tag.name == "Action Games"
        assert tag.color == "#FF0000"
        assert tag.description == "Games with action gameplay"
        assert tag.created_at is not None
        assert tag.updated_at is not None
        assert isinstance(tag.created_at, datetime)
        assert isinstance(tag.updated_at, datetime)

    def test_tag_default_values(self, model_session: Session, test_user_for_model: User):
        """Test tag creation with default values."""
        tag = Tag(
            user_id=test_user_for_model.id,
            name="Test Tag"
        )
        
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        assert tag.color == "#6B7280"  # Default gray color
        assert tag.description is None
        assert tag.created_at is not None
        assert tag.updated_at is not None

    def test_tag_unique_constraint_same_user(self, model_session: Session, test_user_for_model: User):
        """Test that tag names must be unique per user."""
        # Create first tag
        tag1 = Tag(
            user_id=test_user_for_model.id,
            name="Duplicate Name",
            color="#FF0000"
        )
        model_session.add(tag1)
        model_session.commit()
        
        # Try to create second tag with same name for same user
        tag2 = Tag(
            user_id=test_user_for_model.id,
            name="Duplicate Name",
            color="#00FF00"
        )
        model_session.add(tag2)
        
        with pytest.raises(IntegrityError):
            model_session.commit()

    def test_tag_unique_constraint_different_users(self, model_session: Session, test_user_for_model: User, second_test_user: User):
        """Test that tag names can be the same across different users."""
        # Create tag for first user
        tag1 = Tag(
            user_id=test_user_for_model.id,
            name="Common Name",
            color="#FF0000"
        )
        model_session.add(tag1)
        model_session.commit()
        
        # Create tag with same name for second user - this should work
        tag2 = Tag(
            user_id=second_test_user.id,
            name="Common Name",
            color="#00FF00"
        )
        model_session.add(tag2)
        model_session.commit()
        model_session.refresh(tag2)
        
        assert tag1.name == tag2.name
        assert tag1.user_id != tag2.user_id
        assert tag1.id != tag2.id

    def test_tag_relationship_with_user(self, model_session: Session, test_user_for_model: User):
        """Test tag relationship with user."""
        tag = Tag(
            user_id=test_user_for_model.id,
            name="Test Tag"
        )
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        # Test forward relationship
        assert tag.user is not None
        assert tag.user.id == test_user_for_model.id
        assert tag.user.username == test_user_for_model.username

    def test_tag_max_length_validation(self, model_session: Session, test_user_for_model: User):
        """Test tag name and color field length constraints at validation level."""
        # Note: SQLite doesn't enforce varchar length constraints at DB level
        # These would be enforced by Pydantic validation in the API layer
        
        # Test that we can create tags with the expected field lengths
        # Long name (100 chars - should be acceptable)
        acceptable_name = "x" * 100  # 100 characters
        tag = Tag(
            user_id=test_user_for_model.id,
            name=acceptable_name
        )
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        assert tag.name == acceptable_name
        assert len(tag.name) == 100
        
        # Valid hex color (7 chars)
        tag2 = Tag(
            user_id=test_user_for_model.id,
            name="Color Test",
            color="#FF0000"  # 7 characters - valid
        )
        model_session.add(tag2)
        model_session.commit()
        model_session.refresh(tag2)
        
        assert tag2.color == "#FF0000"

    def test_tag_query_by_user(self, model_session: Session, test_user_for_model: User, second_test_user: User):
        """Test querying tags by user."""
        # Create tags for first user
        tag1 = Tag(user_id=test_user_for_model.id, name="Tag 1")
        tag2 = Tag(user_id=test_user_for_model.id, name="Tag 2")
        
        # Create tag for second user
        tag3 = Tag(user_id=second_test_user.id, name="Tag 3")
        
        model_session.add_all([tag1, tag2, tag3])
        model_session.commit()
        
        # Query tags for first user
        user1_tags = model_session.exec(
            select(Tag).where(Tag.user_id == test_user_for_model.id)
        ).all()
        
        assert len(user1_tags) == 2
        tag_names = {tag.name for tag in user1_tags}
        assert tag_names == {"Tag 1", "Tag 2"}
        
        # Query tags for second user
        user2_tags = model_session.exec(
            select(Tag).where(Tag.user_id == second_test_user.id)
        ).all()
        
        assert len(user2_tags) == 1
        assert user2_tags[0].name == "Tag 3"


class TestUserGameTagModel:
    """Test the UserGameTag model."""

    def test_user_game_tag_creation(self, model_session: Session, test_user_for_model: User, test_user_game_for_model: UserGame):
        """Test basic user game tag creation."""
        # Create a tag
        tag = Tag(
            user_id=test_user_for_model.id,
            name="Test Tag"
        )
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        # Create user game tag association
        user_game_tag = UserGameTag(
            user_game_id=test_user_game_for_model.id,
            tag_id=tag.id
        )
        
        model_session.add(user_game_tag)
        model_session.commit()
        model_session.refresh(user_game_tag)
        
        assert user_game_tag.id is not None
        assert user_game_tag.user_game_id == test_user_game_for_model.id
        assert user_game_tag.tag_id == tag.id
        assert user_game_tag.created_at is not None
        assert isinstance(user_game_tag.created_at, datetime)

    def test_user_game_tag_unique_constraint(self, model_session: Session, test_user_for_model: User, test_user_game_for_model: UserGame):
        """Test that user game tag associations must be unique."""
        # Create a tag
        tag = Tag(
            user_id=test_user_for_model.id,
            name="Test Tag"
        )
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        # Create first association
        user_game_tag1 = UserGameTag(
            user_game_id=test_user_game_for_model.id,
            tag_id=tag.id
        )
        model_session.add(user_game_tag1)
        model_session.commit()
        
        # Try to create duplicate association
        user_game_tag2 = UserGameTag(
            user_game_id=test_user_game_for_model.id,
            tag_id=tag.id
        )
        model_session.add(user_game_tag2)
        
        with pytest.raises(IntegrityError):
            model_session.commit()

    def test_user_game_tag_relationships(self, model_session: Session, test_user_for_model: User, test_user_game_for_model: UserGame):
        """Test user game tag relationships."""
        # Create a tag
        tag = Tag(
            user_id=test_user_for_model.id,
            name="Test Tag"
        )
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        # Create user game tag association
        user_game_tag = UserGameTag(
            user_game_id=test_user_game_for_model.id,
            tag_id=tag.id
        )
        model_session.add(user_game_tag)
        model_session.commit()
        model_session.refresh(user_game_tag)
        
        # Test relationship to tag
        assert user_game_tag.tag is not None
        assert user_game_tag.tag.id == tag.id
        assert user_game_tag.tag.name == "Test Tag"
        
        # Test relationship to user game
        assert user_game_tag.user_game is not None
        assert user_game_tag.user_game.id == test_user_game_for_model.id

    def test_user_game_tag_cascade_delete(self, model_session: Session, test_user_for_model: User, test_user_game_for_model: UserGame):
        """Test that user game tags must be manually deleted before tag deletion."""
        # Note: SQLite doesn't automatically cascade foreign key deletes by default
        # In the service layer, we manually delete associations before deleting tags
        
        # Create a tag
        tag = Tag(
            user_id=test_user_for_model.id,
            name="Test Tag"
        )
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        # Create user game tag association
        user_game_tag = UserGameTag(
            user_game_id=test_user_game_for_model.id,
            tag_id=tag.id
        )
        model_session.add(user_game_tag)
        model_session.commit()
        model_session.refresh(user_game_tag)
        
        # Verify association exists
        associations = model_session.exec(
            select(UserGameTag).where(UserGameTag.tag_id == tag.id)
        ).all()
        assert len(associations) == 1
        
        # Manual cleanup (as done in TagService.delete_tag)
        # Delete associations first
        for association in associations:
            model_session.delete(association)
        
        # Then delete the tag
        model_session.delete(tag)
        model_session.commit()
        
        # Verify both are deleted
        associations_after = model_session.exec(
            select(UserGameTag).where(UserGameTag.tag_id == tag.id)
        ).all()
        assert len(associations_after) == 0
        
        tag_after = model_session.exec(
            select(Tag).where(Tag.id == tag.id)
        ).first()
        assert tag_after is None

    def test_multiple_tags_per_game(self, model_session: Session, test_user_for_model: User, test_user_game_for_model: UserGame):
        """Test that a game can have multiple tags."""
        # Create multiple tags
        tag1 = Tag(user_id=test_user_for_model.id, name="Action")
        tag2 = Tag(user_id=test_user_for_model.id, name="RPG")
        tag3 = Tag(user_id=test_user_for_model.id, name="Multiplayer")
        
        model_session.add_all([tag1, tag2, tag3])
        model_session.commit()
        
        # Create associations for all tags
        associations = [
            UserGameTag(user_game_id=test_user_game_for_model.id, tag_id=tag1.id),
            UserGameTag(user_game_id=test_user_game_for_model.id, tag_id=tag2.id),
            UserGameTag(user_game_id=test_user_game_for_model.id, tag_id=tag3.id)
        ]
        
        model_session.add_all(associations)
        model_session.commit()
        
        # Query all tags for the game
        game_tags = model_session.exec(
            select(UserGameTag)
            .where(UserGameTag.user_game_id == test_user_game_for_model.id)
        ).all()
        
        assert len(game_tags) == 3
        tag_names = {assoc.tag.name for assoc in game_tags}
        assert tag_names == {"Action", "RPG", "Multiplayer"}

    def test_multiple_games_per_tag(self, model_session: Session, test_user_for_model: User):
        """Test that a tag can be applied to multiple games."""
        # Create multiple games
        game1 = create_test_game(title="Game 1", igdb_id=1002)
        game2 = create_test_game(title="Game 2", igdb_id=1003)
        game3 = create_test_game(title="Game 3", igdb_id=1004)
        
        model_session.add_all([game1, game2, game3])
        model_session.commit()
        
        # Create user games
        user_game1 = UserGame(user_id=test_user_for_model.id, game_id=game1.id, ownership_status="owned", play_status="not_started")
        user_game2 = UserGame(user_id=test_user_for_model.id, game_id=game2.id, ownership_status="owned", play_status="not_started")
        user_game3 = UserGame(user_id=test_user_for_model.id, game_id=game3.id, ownership_status="owned", play_status="not_started")
        
        model_session.add_all([user_game1, user_game2, user_game3])
        model_session.commit()
        
        # Create a tag
        tag = Tag(user_id=test_user_for_model.id, name="Favorites")
        model_session.add(tag)
        model_session.commit()
        model_session.refresh(tag)
        
        # Apply tag to all games
        associations = [
            UserGameTag(user_game_id=user_game1.id, tag_id=tag.id),
            UserGameTag(user_game_id=user_game2.id, tag_id=tag.id),
            UserGameTag(user_game_id=user_game3.id, tag_id=tag.id)
        ]
        
        model_session.add_all(associations)
        model_session.commit()
        
        # Query all games with the tag
        tagged_games = model_session.exec(
            select(UserGameTag)
            .where(UserGameTag.tag_id == tag.id)
        ).all()
        
        assert len(tagged_games) == 3
        game_titles = {assoc.user_game.game.title for assoc in tagged_games}
        assert game_titles == {"Game 1", "Game 2", "Game 3"}

    def test_user_game_tag_isolation(self, model_session: Session, test_user_for_model: User, second_test_user: User):
        """Test that user game tags are properly isolated between users."""
        # Create games
        game1 = create_test_game(title="Shared Game 1", igdb_id=1005)
        game2 = create_test_game(title="Shared Game 2", igdb_id=1006)
        
        model_session.add_all([game1, game2])
        model_session.commit()
        
        # Create user games for both users
        user1_game1 = UserGame(user_id=test_user_for_model.id, game_id=game1.id, ownership_status="owned", play_status="not_started")
        user1_game2 = UserGame(user_id=test_user_for_model.id, game_id=game2.id, ownership_status="owned", play_status="not_started")
        user2_game1 = UserGame(user_id=second_test_user.id, game_id=game1.id, ownership_status="owned", play_status="not_started")
        user2_game2 = UserGame(user_id=second_test_user.id, game_id=game2.id, ownership_status="owned", play_status="not_started")
        
        model_session.add_all([user1_game1, user1_game2, user2_game1, user2_game2])
        model_session.commit()
        
        # Create tags for both users (same names but different users)
        user1_tag = Tag(user_id=test_user_for_model.id, name="Action")
        user2_tag = Tag(user_id=second_test_user.id, name="Action")
        
        model_session.add_all([user1_tag, user2_tag])
        model_session.commit()
        
        # Create tag associations
        user1_associations = [
            UserGameTag(user_game_id=user1_game1.id, tag_id=user1_tag.id),
            UserGameTag(user_game_id=user1_game2.id, tag_id=user1_tag.id)
        ]
        user2_associations = [
            UserGameTag(user_game_id=user2_game1.id, tag_id=user2_tag.id)
        ]
        
        model_session.add_all(user1_associations + user2_associations)
        model_session.commit()
        
        # Verify user 1 has 2 tagged games
        user1_tagged_games = model_session.exec(
            select(UserGameTag)
            .join(Tag, Tag.id == UserGameTag.tag_id)  # type: ignore[arg-type]
            .where(Tag.user_id == test_user_for_model.id)
        ).all()
        assert len(user1_tagged_games) == 2

        # Verify user 2 has 1 tagged game
        user2_tagged_games = model_session.exec(
            select(UserGameTag)
            .join(Tag, Tag.id == UserGameTag.tag_id)  # type: ignore[arg-type]
            .where(Tag.user_id == second_test_user.id)
        ).all()
        assert len(user2_tagged_games) == 1
        
        # Verify isolation - user 1 can't see user 2's tag associations
        cross_user_query = model_session.exec(
            select(UserGameTag)
            .where(and_(
                UserGameTag.tag_id == user2_tag.id,
                in_(UserGameTag.user_game_id, [user1_game1.id, user1_game2.id])
            ))
        ).all()
        assert len(cross_user_query) == 0
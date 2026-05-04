"""
Integration tests for tag API endpoints.
Tests all tag HTTP endpoints with authentication, validation, and proper request/response handling.
"""

from fastapi.testclient import TestClient
from sqlmodel import Session, select
from typing import Dict

from ..models.tag import Tag, UserGameTag
from ..models.user import User
from ..models.user_game import UserGame
from .integration_test_utils import (
    assert_api_error,
    assert_api_success,
    register_and_login_user,
    create_test_user_data,
    create_test_games,
)


class TestTagsListEndpoint:
    """Test GET /api/tags/ endpoint."""

    def test_list_tags_empty(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test listing tags when user has no tags."""
        response = client.get("/api/tags/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "tags" in data
        assert "total" in data
        assert "page" in data
        assert "per_page" in data
        assert "total_pages" in data
        assert len(data["tags"]) == 0
        assert data["total"] == 0
        assert data["page"] == 1
        assert data["per_page"] == 50
        assert data["total_pages"] == 0

    def test_list_tags_with_data(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test listing tags when user has tags."""
        # Create test tags
        tags = [
            Tag(user_id=test_user.id, name="Action", color="#FF0000", description="Action games"),
            Tag(user_id=test_user.id, name="RPG", color="#00FF00", description="Role-playing games"),
            Tag(user_id=test_user.id, name="Strategy", color="#0000FF")
        ]
        
        session.add_all(tags)
        session.commit()
        
        response = client.get("/api/tags/", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["tags"]) == 3
        assert data["total"] == 3
        assert data["total_pages"] == 1
        
        # Check tags are sorted by name
        tag_names = [tag["name"] for tag in data["tags"]]
        assert tag_names == ["Action", "RPG", "Strategy"]
        
        # Check tag structure
        action_tag = next(tag for tag in data["tags"] if tag["name"] == "Action")
        assert action_tag["color"] == "#FF0000"
        assert action_tag["description"] == "Action games"
        assert action_tag["user_id"] == test_user.id
        assert "created_at" in action_tag
        assert "updated_at" in action_tag
        assert "game_count" in action_tag

    def test_list_tags_pagination(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test tag listing with pagination."""
        # Create 5 tags
        for i in range(5):
            tag = Tag(user_id=test_user.id, name=f"Tag {i:02d}")
            session.add(tag)
        session.commit()
        
        # Test first page
        response = client.get("/api/tags/?page=1&per_page=2", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["tags"]) == 2
        assert data["total"] == 5
        assert data["page"] == 1
        assert data["per_page"] == 2
        assert data["total_pages"] == 3
        
        # Test second page
        response = client.get("/api/tags/?page=2&per_page=2", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["tags"]) == 2
        assert data["page"] == 2
        
        # Test last page
        response = client.get("/api/tags/?page=3&per_page=2", headers=auth_headers)
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["tags"]) == 1
        assert data["page"] == 3

    def test_list_tags_without_auth(self, client: TestClient):
        """Test listing tags without authentication."""
        response = client.get("/api/tags/")
        assert_api_error(response, 403, "Not authenticated")

    def test_list_tags_user_isolation(self, client: TestClient, session: Session):
        """Test that users only see their own tags."""
        # Create two users
        user1_data = create_test_user_data("user1", "password123")
        user2_data = create_test_user_data("user2", "password456")
        
        user1_headers = register_and_login_user(client, user1_data, session)
        user2_headers = register_and_login_user(client, user2_data, session)
        
        # Get user IDs from database
        user1 = session.exec(select(User).where(User.username == "user1")).first()
        user2 = session.exec(select(User).where(User.username == "user2")).first()
        assert user1 is not None, "User1 should exist after registration"
        assert user2 is not None, "User2 should exist after registration"

        # Create tags for both users
        user1_tags = [
            Tag(user_id=user1.id, name="User1 Tag1"),
            Tag(user_id=user1.id, name="User1 Tag2")
        ]
        user2_tags = [
            Tag(user_id=user2.id, name="User2 Tag1"),
            Tag(user_id=user2.id, name="User2 Tag2"),
            Tag(user_id=user2.id, name="User2 Tag3")
        ]
        
        session.add_all(user1_tags + user2_tags)
        session.commit()
        
        # User1 should only see their tags
        response1 = client.get("/api/tags/", headers=user1_headers)
        assert_api_success(response1, 200)
        data1 = response1.json()
        assert len(data1["tags"]) == 2
        assert data1["total"] == 2
        
        # User2 should only see their tags
        response2 = client.get("/api/tags/", headers=user2_headers)
        assert_api_success(response2, 200)
        data2 = response2.json()
        assert len(data2["tags"]) == 3
        assert data2["total"] == 3

    def test_list_tags_with_game_count(self, client: TestClient, test_user: User, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test that game count is included in tag listings."""
        # Create tag and assign to game
        tag = Tag(user_id=test_user.id, name="Test Tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        # Assign tag to game
        association = UserGameTag(user_game_id=test_user_game.id, tag_id=tag.id)
        session.add(association)
        session.commit()
        
        response = client.get("/api/tags/?include_game_count=true", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["tags"]) == 1
        assert data["tags"][0]["game_count"] == 1

    def test_list_tags_without_game_count(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test listing tags without game count."""
        tag = Tag(user_id=test_user.id, name="Test Tag")
        session.add(tag)
        session.commit()
        
        response = client.get("/api/tags/?include_game_count=false", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert len(data["tags"]) == 1
        assert data["tags"][0]["game_count"] is None


class TestTagCreateEndpoint:
    """Test POST /api/tags/ endpoint."""

    def test_create_tag_success(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test successful tag creation."""
        tag_data = {
            "name": "Action Games",
            "color": "#FF0000",
            "description": "Fast-paced action games"
        }
        
        response = client.post("/api/tags/", json=tag_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["name"] == "Action Games"
        assert data["color"] == "#FF0000"
        assert data["description"] == "Fast-paced action games"
        assert data["game_count"] == 0
        assert "id" in data
        assert "user_id" in data
        assert "created_at" in data
        assert "updated_at" in data

    def test_create_tag_minimal_data(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test tag creation with minimal data."""
        tag_data = {"name": "Simple Tag"}
        
        response = client.post("/api/tags/", json=tag_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["name"] == "Simple Tag"
        assert data["color"] == "#6B7280"  # Default color
        assert data["description"] is None

    def test_create_tag_without_auth(self, client: TestClient):
        """Test tag creation without authentication."""
        tag_data = {"name": "Test Tag"}
        
        response = client.post("/api/tags/", json=tag_data)
        assert_api_error(response, 403, "Not authenticated")

    def test_create_tag_duplicate_name(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test creating tag with duplicate name."""
        # Create existing tag
        existing_tag = Tag(user_id=test_user.id, name="Duplicate Name")
        session.add(existing_tag)
        session.commit()
        
        tag_data = {"name": "Duplicate Name"}
        
        response = client.post("/api/tags/", json=tag_data, headers=auth_headers)
        assert_api_error(response, 400, "already exists")

    def test_create_tag_invalid_data(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test tag creation with invalid data."""
        # Missing name
        response = client.post("/api/tags/", json={}, headers=auth_headers)
        assert_api_error(response, 422)
        
        # Empty name
        response = client.post("/api/tags/", json={"name": ""}, headers=auth_headers)
        assert_api_error(response, 422)
        
        # Invalid color
        response = client.post("/api/tags/", json={"name": "Test", "color": "red"}, headers=auth_headers)
        assert_api_error(response, 422)
        
        # Name too long
        response = client.post("/api/tags/", json={"name": "x" * 101}, headers=auth_headers)
        assert_api_error(response, 422)

    def test_create_tag_whitespace_handling(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test that tag names are properly trimmed."""
        tag_data = {"name": "  Trimmed Name  "}
        
        response = client.post("/api/tags/", json=tag_data, headers=auth_headers)
        
        assert_api_success(response, 201)
        data = response.json()
        assert data["name"] == "Trimmed Name"


class TestTagGetEndpoint:
    """Test GET /api/tags/{tag_id} endpoint."""

    def test_get_tag_success(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test successful tag retrieval."""
        tag = Tag(user_id=test_user.id, name="Test Tag", color="#FF0000", description="Test description")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        response = client.get(f"/api/tags/{tag.id}", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["id"] == tag.id
        assert data["name"] == "Test Tag"
        assert data["color"] == "#FF0000"
        assert data["description"] == "Test description"
        assert data["user_id"] == test_user.id

    def test_get_tag_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test retrieving nonexistent tag."""
        response = client.get("/api/tags/nonexistent-id", headers=auth_headers)
        assert_api_error(response, 404, "not found")

    def test_get_tag_wrong_user(self, client: TestClient, session: Session):
        """Test that users cannot access tags belonging to other users."""
        # Create two users
        user1_data = create_test_user_data("user1", "password123")
        user2_data = create_test_user_data("user2", "password456")
        
        register_and_login_user(client, user1_data, session)
        user2_headers = register_and_login_user(client, user2_data, session)
        
        # Get user IDs
        user1 = session.exec(select(User).where(User.username == "user1")).first()
        assert user1 is not None, "User1 should exist after registration"

        # Create tag for user1
        tag = Tag(user_id=user1.id, name="User1 Tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)

        # User2 tries to access user1's tag
        response = client.get(f"/api/tags/{tag.id}", headers=user2_headers)
        assert_api_error(response, 404, "not found")

    def test_get_tag_without_auth(self, client: TestClient, test_user: User, session: Session):
        """Test tag retrieval without authentication."""
        tag = Tag(user_id=test_user.id, name="Test Tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        response = client.get(f"/api/tags/{tag.id}")
        assert_api_error(response, 403, "Not authenticated")


class TestTagUpdateEndpoint:
    """Test PUT /api/tags/{tag_id} endpoint."""

    def test_update_tag_success(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test successful tag update."""
        tag = Tag(user_id=test_user.id, name="Original Name", color="#FF0000", description="Original description")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        update_data = {
            "name": "Updated Name",
            "color": "#00FF00",
            "description": "Updated description"
        }
        
        response = client.put(f"/api/tags/{tag.id}", json=update_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["name"] == "Updated Name"
        assert data["color"] == "#00FF00"
        assert data["description"] == "Updated description"
        assert data["updated_at"] != data["created_at"]  # Should be updated

    def test_update_tag_partial(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test partial tag update."""
        tag = Tag(user_id=test_user.id, name="Original Name", color="#FF0000", description="Original description")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        update_data = {"color": "#00FF00"}  # Only update color
        
        response = client.put(f"/api/tags/{tag.id}", json=update_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["name"] == "Original Name"  # Unchanged
        assert data["color"] == "#00FF00"  # Changed
        assert data["description"] == "Original description"  # Unchanged

    def test_update_tag_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test updating nonexistent tag."""
        update_data = {"name": "Updated Name"}
        
        response = client.put("/api/tags/nonexistent-id", json=update_data, headers=auth_headers)
        assert_api_error(response, 404, "not found")

    def test_update_tag_wrong_user(self, client: TestClient, session: Session):
        """Test that users cannot update tags belonging to other users."""
        # Create two users
        user1_data = create_test_user_data("user1", "password123")
        user2_data = create_test_user_data("user2", "password456")
        
        register_and_login_user(client, user1_data, session)
        user2_headers = register_and_login_user(client, user2_data, session)
        
        # Get user1 ID
        user1 = session.exec(select(User).where(User.username == "user1")).first()
        assert user1 is not None, "User1 should exist after registration"

        # Create tag for user1
        tag = Tag(user_id=user1.id, name="User1 Tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)

        # User2 tries to update user1's tag
        update_data = {"name": "Hacked Name"}
        response = client.put(f"/api/tags/{tag.id}", json=update_data, headers=user2_headers)
        assert_api_error(response, 404, "not found")

    def test_update_tag_duplicate_name(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test updating tag to a name that already exists."""
        # Create two tags
        tag1 = Tag(user_id=test_user.id, name="Tag 1")
        tag2 = Tag(user_id=test_user.id, name="Tag 2")
        session.add_all([tag1, tag2])
        session.commit()
        
        # Try to update tag2 to have tag1's name
        update_data = {"name": "Tag 1"}
        response = client.put(f"/api/tags/{tag2.id}", json=update_data, headers=auth_headers)
        assert_api_error(response, 400, "already exists")

    def test_update_tag_invalid_data(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test tag update with invalid data."""
        tag = Tag(user_id=test_user.id, name="Test Tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        # Empty name
        response = client.put(f"/api/tags/{tag.id}", json={"name": ""}, headers=auth_headers)
        assert_api_error(response, 422)
        
        # Invalid color
        response = client.put(f"/api/tags/{tag.id}", json={"color": "invalid"}, headers=auth_headers)
        assert_api_error(response, 422)

    def test_update_tag_without_auth(self, client: TestClient, test_user: User, session: Session):
        """Test tag update without authentication."""
        tag = Tag(user_id=test_user.id, name="Test Tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        response = client.put(f"/api/tags/{tag.id}", json={"name": "Updated"})
        assert_api_error(response, 403, "Not authenticated")


class TestTagDeleteEndpoint:
    """Test DELETE /api/tags/{tag_id} endpoint."""

    def test_delete_tag_success(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test successful tag deletion."""
        tag = Tag(user_id=test_user.id, name="To Delete")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        response = client.delete(f"/api/tags/{tag.id}", headers=auth_headers)
        
        assert response.status_code == 204
        assert response.content == b""
        
        # Verify tag is deleted
        deleted_tag = session.exec(select(Tag).where(Tag.id == tag.id)).first()
        assert deleted_tag is None

    def test_delete_tag_not_found(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test deleting nonexistent tag."""
        response = client.delete("/api/tags/nonexistent-id", headers=auth_headers)
        assert_api_error(response, 404, "not found")

    def test_delete_tag_wrong_user(self, client: TestClient, session: Session):
        """Test that users cannot delete tags belonging to other users."""
        # Create two users
        user1_data = create_test_user_data("user1", "password123")
        user2_data = create_test_user_data("user2", "password456")
        
        register_and_login_user(client, user1_data, session)
        user2_headers = register_and_login_user(client, user2_data, session)
        
        # Get user1 ID
        user1 = session.exec(select(User).where(User.username == "user1")).first()
        assert user1 is not None, "User1 should exist after registration"

        # Create tag for user1
        tag = Tag(user_id=user1.id, name="User1 Tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)

        # User2 tries to delete user1's tag
        response = client.delete(f"/api/tags/{tag.id}", headers=user2_headers)
        assert_api_error(response, 404, "not found")
        
        # Verify tag still exists
        existing_tag = session.exec(select(Tag).where(Tag.id == tag.id)).first()
        assert existing_tag is not None

    def test_delete_tag_with_associations(self, client: TestClient, test_user: User, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test that deleting a tag also removes its game associations."""
        # Create tag and assign to game
        tag = Tag(user_id=test_user.id, name="To Delete")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        association = UserGameTag(user_game_id=test_user_game.id, tag_id=tag.id)
        session.add(association)
        session.commit()
        
        # Verify association exists
        existing_associations = session.exec(
            select(UserGameTag).where(UserGameTag.tag_id == tag.id)
        ).all()
        assert len(existing_associations) == 1
        
        # Delete the tag
        response = client.delete(f"/api/tags/{tag.id}", headers=auth_headers)
        assert response.status_code == 204
        
        # Verify associations are also deleted
        remaining_associations = session.exec(
            select(UserGameTag).where(UserGameTag.tag_id == tag.id)
        ).all()
        assert len(remaining_associations) == 0

    def test_delete_tag_without_auth(self, client: TestClient, test_user: User, session: Session):
        """Test tag deletion without authentication."""
        tag = Tag(user_id=test_user.id, name="Test Tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        response = client.delete(f"/api/tags/{tag.id}")
        assert_api_error(response, 403, "Not authenticated")


class TestTagCreateOrGetEndpoint:
    """Test POST /api/tags/create-or-get endpoint."""

    def test_create_or_get_new_tag(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test creating a new tag with create-or-get."""
        response = client.post("/api/tags/create-or-get?name=New Tag&color=%23FF0000", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["created"] is True
        assert data["tag"]["name"] == "New Tag"
        assert data["tag"]["color"] == "#FF0000"

    def test_create_or_get_existing_tag(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test getting an existing tag with create-or-get."""
        # Create existing tag
        tag = Tag(user_id=test_user.id, name="Existing Tag", color="#FF0000")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        response = client.post("/api/tags/create-or-get?name=Existing Tag&color=%2300FF00", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["created"] is False
        assert data["tag"]["id"] == tag.id
        assert data["tag"]["name"] == "Existing Tag"
        assert data["tag"]["color"] == "#FF0000"  # Should keep original color

    def test_create_or_get_case_insensitive(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test that create-or-get is case-insensitive."""
        # Create tag with lowercase name
        tag = Tag(user_id=test_user.id, name="test tag")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        response = client.post("/api/tags/create-or-get?name=TEST TAG", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["created"] is False
        assert data["tag"]["id"] == tag.id

    def test_create_or_get_without_color(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test create-or-get without specifying color."""
        response = client.post("/api/tags/create-or-get?name=Default Color Tag", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["created"] is True
        assert data["tag"]["color"] == "#6B7280"  # Default color

    def test_create_or_get_without_auth(self, client: TestClient):
        """Test create-or-get without authentication."""
        response = client.post("/api/tags/create-or-get?name=Test Tag")
        assert_api_error(response, 403, "Not authenticated")

    def test_create_or_get_invalid_name(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test create-or-get with invalid name."""
        response = client.post("/api/tags/create-or-get?name=", headers=auth_headers)
        assert_api_error(response, 400)


class TestTagUsageStatsEndpoint:
    """Test GET /api/tags/usage/stats endpoint."""

    def test_get_usage_stats_empty(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test usage stats when user has no tags."""
        response = client.get("/api/tags/usage/stats", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total_tags"] == 0
        assert data["total_tagged_games"] == 0
        assert data["average_tags_per_game"] == 0.0
        assert data["tag_usage"] == {}
        assert len(data["popular_tags"]) == 0
        assert len(data["unused_tags"]) == 0

    def test_get_usage_stats_with_data(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test comprehensive usage stats with data."""
        # Create games and user games
        games = create_test_games(count=3, session=session, commit=True)
        user_games = []
        for game in games:
            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id,
                ownership_status="owned",
                play_status="not_started"
            )
            session.add(user_game)
            user_games.append(user_game)
        session.commit()
        for ug in user_games:
            session.refresh(ug)
        
        # Create tags
        tag1 = Tag(user_id=test_user.id, name="Popular Tag")
        tag2 = Tag(user_id=test_user.id, name="Less Popular")
        tag3 = Tag(user_id=test_user.id, name="Unused Tag")
        session.add_all([tag1, tag2, tag3])
        session.commit()
        session.refresh(tag1)
        session.refresh(tag2)
        session.refresh(tag3)
        
        # Create associations
        associations = [
            UserGameTag(user_game_id=user_games[0].id, tag_id=tag1.id),  # Popular tag on game 1
            UserGameTag(user_game_id=user_games[1].id, tag_id=tag1.id),  # Popular tag on game 2
            UserGameTag(user_game_id=user_games[2].id, tag_id=tag1.id),  # Popular tag on game 3
            UserGameTag(user_game_id=user_games[0].id, tag_id=tag2.id),  # Less popular on game 1
        ]
        session.add_all(associations)
        session.commit()
        
        response = client.get("/api/tags/usage/stats", headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total_tags"] == 3
        assert data["total_tagged_games"] == 3
        assert data["average_tags_per_game"] == 1.33  # (3+1+1)/3 = 1.33
        
        # Check tag usage
        assert str(tag1.id) in data["tag_usage"]
        assert data["tag_usage"][str(tag1.id)] == 3
        assert data["tag_usage"][str(tag2.id)] == 1
        assert data["tag_usage"][str(tag3.id)] == 0
        
        # Check popular tags (should be sorted by usage)
        assert len(data["popular_tags"]) == 2
        assert data["popular_tags"][0]["id"] == tag1.id  # Most popular first
        assert data["popular_tags"][0]["game_count"] == 3
        assert data["popular_tags"][1]["id"] == tag2.id
        
        # Check unused tags
        assert len(data["unused_tags"]) == 1
        assert data["unused_tags"][0]["id"] == tag3.id

    def test_get_usage_stats_without_auth(self, client: TestClient):
        """Test usage stats without authentication."""
        response = client.get("/api/tags/usage/stats")
        assert_api_error(response, 403, "Not authenticated")


class TestTagAssignmentEndpoints:
    """Test tag assignment and removal endpoints."""

    def test_assign_tags_to_game_success(self, client: TestClient, test_user: User, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test successful tag assignment to a game."""
        # Create tags
        tag1 = Tag(user_id=test_user.id, name="Action")
        tag2 = Tag(user_id=test_user.id, name="RPG")
        session.add_all([tag1, tag2])
        session.commit()
        session.refresh(tag1)
        session.refresh(tag2)
        
        assign_data = {"tag_ids": [tag1.id, tag2.id]}
        
        response = client.post(f"/api/tags/assign/{test_user_game.id}", json=assign_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "message" in data
        assert data["new_associations"] == 2
        assert data["total_requested"] == 2
        
        # Verify associations were created
        associations = session.exec(
            select(UserGameTag).where(UserGameTag.user_game_id == test_user_game.id)
        ).all()
        assert len(associations) == 2

    def test_assign_tags_duplicate_assignment(self, client: TestClient, test_user: User, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test assigning tags that are already assigned."""
        # Create tag and assign it
        tag = Tag(user_id=test_user.id, name="Action")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        association = UserGameTag(user_game_id=test_user_game.id, tag_id=tag.id)
        session.add(association)
        session.commit()
        
        # Try to assign the same tag again
        assign_data = {"tag_ids": [tag.id]}
        response = client.post(f"/api/tags/assign/{test_user_game.id}", json=assign_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["new_associations"] == 0  # No new associations
        assert data["total_requested"] == 1

    def test_assign_tags_nonexistent_game(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test assigning tags to nonexistent game."""
        tag = Tag(user_id=test_user.id, name="Action")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        assign_data = {"tag_ids": [tag.id]}
        
        response = client.post("/api/tags/assign/nonexistent-game", json=assign_data, headers=auth_headers)
        assert_api_error(response, 404, "not found")

    def test_assign_tags_nonexistent_tags(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test assigning nonexistent tags."""
        assign_data = {"tag_ids": ["nonexistent-tag"]}
        
        response = client.post(f"/api/tags/assign/{test_user_game.id}", json=assign_data, headers=auth_headers)
        assert_api_error(response, 404, "not found")

    def test_assign_tags_invalid_data(self, client: TestClient, test_user_game: UserGame, auth_headers: Dict[str, str]):
        """Test tag assignment with invalid data."""
        # Empty tag_ids list
        response = client.post(f"/api/tags/assign/{test_user_game.id}", json={"tag_ids": []}, headers=auth_headers)
        assert_api_error(response, 422)
        
        # Missing tag_ids field
        response = client.post(f"/api/tags/assign/{test_user_game.id}", json={}, headers=auth_headers)
        assert_api_error(response, 422)

    def test_assign_tags_without_auth(self, client: TestClient, test_user_game: UserGame):
        """Test tag assignment without authentication."""
        assign_data = {"tag_ids": ["some-tag-id"]}
        
        response = client.post(f"/api/tags/assign/{test_user_game.id}", json=assign_data)
        assert_api_error(response, 403, "Not authenticated")

    def test_remove_tags_from_game_success(self, client: TestClient, test_user: User, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test successful tag removal from a game."""
        # Create tags and assign them
        tag1 = Tag(user_id=test_user.id, name="Action")
        tag2 = Tag(user_id=test_user.id, name="RPG")
        session.add_all([tag1, tag2])
        session.commit()
        session.refresh(tag1)
        session.refresh(tag2)
        
        associations = [
            UserGameTag(user_game_id=test_user_game.id, tag_id=tag1.id),
            UserGameTag(user_game_id=test_user_game.id, tag_id=tag2.id)
        ]
        session.add_all(associations)
        session.commit()
        
        # Remove one tag
        remove_data = {"tag_ids": [tag1.id]}
        response = client.request("DELETE", f"/api/tags/remove/{test_user_game.id}", json=remove_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "message" in data
        assert data["removed_associations"] == 1
        assert data["total_requested"] == 1
        
        # Verify only one association remains
        remaining = session.exec(
            select(UserGameTag).where(UserGameTag.user_game_id == test_user_game.id)
        ).all()
        assert len(remaining) == 1
        assert remaining[0].tag_id == tag2.id

    def test_remove_tags_not_assigned(self, client: TestClient, test_user: User, test_user_game: UserGame, auth_headers: Dict[str, str], session: Session):
        """Test removing tags that aren't assigned to the game."""
        # Create tag but don't assign it
        tag = Tag(user_id=test_user.id, name="Action")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        remove_data = {"tag_ids": [tag.id]}
        response = client.request("DELETE", f"/api/tags/remove/{test_user_game.id}", json=remove_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["removed_associations"] == 0

    def test_remove_tags_without_auth(self, client: TestClient, test_user_game: UserGame):
        """Test tag removal without authentication."""
        remove_data = {"tag_ids": ["some-tag-id"]}
        
        response = client.request("DELETE", f"/api/tags/remove/{test_user_game.id}", json=remove_data)
        assert_api_error(response, 403, "Not authenticated")


class TestBulkTagOperationEndpoints:
    """Test bulk tag assignment and removal endpoints."""

    def test_bulk_assign_tags_success(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test successful bulk tag assignment."""
        # Create games and user games
        games = create_test_games(count=3, session=session, commit=True)
        user_games = []
        for game in games:
            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id,
                ownership_status="owned",
                play_status="not_started"
            )
            session.add(user_game)
            user_games.append(user_game)
        session.commit()
        for ug in user_games:
            session.refresh(ug)
        
        # Create tags
        tag1 = Tag(user_id=test_user.id, name="Action")
        tag2 = Tag(user_id=test_user.id, name="RPG")
        session.add_all([tag1, tag2])
        session.commit()
        session.refresh(tag1)
        session.refresh(tag2)
        
        bulk_data = {
            "user_game_ids": [ug.id for ug in user_games],
            "tag_ids": [tag1.id, tag2.id]
        }
        
        response = client.post("/api/tags/bulk-assign", json=bulk_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert "message" in data
        assert data["total_new_associations"] == 6  # 3 games × 2 tags
        assert data["games_processed"] == 3
        assert data["tags_per_game"] == 2

    def test_bulk_remove_tags_success(self, client: TestClient, test_user: User, auth_headers: Dict[str, str], session: Session):
        """Test successful bulk tag removal."""
        # Create games, user games, and tags
        games = create_test_games(count=2, session=session, commit=True)
        user_games = []
        for game in games:
            user_game = UserGame(
                user_id=test_user.id,
                game_id=game.id,
                ownership_status="owned",
                play_status="not_started"
            )
            session.add(user_game)
            user_games.append(user_game)
        session.commit()
        for ug in user_games:
            session.refresh(ug)
        
        tag = Tag(user_id=test_user.id, name="Action")
        session.add(tag)
        session.commit()
        session.refresh(tag)
        
        # Create associations
        associations = [
            UserGameTag(user_game_id=user_games[0].id, tag_id=tag.id),
            UserGameTag(user_game_id=user_games[1].id, tag_id=tag.id)
        ]
        session.add_all(associations)
        session.commit()
        
        bulk_data = {
            "user_game_ids": [ug.id for ug in user_games],
            "tag_ids": [tag.id]
        }
        
        response = client.request("DELETE", "/api/tags/bulk-remove", json=bulk_data, headers=auth_headers)
        
        assert_api_success(response, 200)
        data = response.json()
        assert data["total_removed_associations"] == 2
        assert data["games_processed"] == 2
        assert data["tags_per_game"] == 1

    def test_bulk_operations_invalid_data(self, client: TestClient, auth_headers: Dict[str, str]):
        """Test bulk operations with invalid data."""
        # Empty user_game_ids
        response = client.post("/api/tags/bulk-assign", json={"user_game_ids": [], "tag_ids": ["tag1"]}, headers=auth_headers)
        assert_api_error(response, 422)
        
        # Empty tag_ids
        response = client.post("/api/tags/bulk-assign", json={"user_game_ids": ["game1"], "tag_ids": []}, headers=auth_headers)
        assert_api_error(response, 422)

    def test_bulk_operations_without_auth(self, client: TestClient):
        """Test bulk operations without authentication."""
        bulk_data = {"user_game_ids": ["game1"], "tag_ids": ["tag1"]}
        
        response = client.post("/api/tags/bulk-assign", json=bulk_data)
        assert_api_error(response, 403, "Not authenticated")
        
        response = client.request("DELETE", "/api/tags/bulk-remove", json=bulk_data)
        assert_api_error(response, 403, "Not authenticated")
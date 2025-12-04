"""
Tag service for managing user tags and tag assignments.
"""

from typing import Any, List, Optional, Dict, Tuple
from sqlmodel import Session, select, func, and_
from datetime import datetime, timezone
import logging
import uuid

from ..models.tag import Tag, UserGameTag
from ..models.user_game import UserGame
from ..schemas.tag import (
    TagCreateRequest,
    TagUpdateRequest
)
from ..utils.sqlalchemy_typed import in_

logger = logging.getLogger(__name__)


class TagService:
    """Service class for tag management operations."""
    
    def __init__(self, session: Session):
        self.session = session
    
    def get_user_tags(
        self, 
        user_id: str, 
        page: int = 1, 
        per_page: int = 50,
        include_game_count: bool = True
    ) -> Tuple[List[Tag], int]:
        """
        Get all tags for a user with pagination.
        
        Args:
            user_id: ID of the user
            page: Page number (1-based)
            per_page: Number of tags per page
            include_game_count: Whether to include game count in results
            
        Returns:
            Tuple of (tags list, total count)
        """
        logger.info(f"Fetching tags for user {user_id}, page {page}, per_page {per_page}")
        
        try:
            # Base query for user's tags
            base_query = select(Tag).where(Tag.user_id == user_id)
            
            # Use efficient count query instead of fetching all records
            count_query = select(func.count()).select_from(Tag).where(Tag.user_id == user_id)
            total_count = self.session.exec(count_query).one()
            
            # Apply pagination and ordering
            offset = (page - 1) * per_page
            query = base_query.order_by(Tag.name).offset(offset).limit(per_page)
            
            tags = list(self.session.exec(query).all())

            # Set game count if requested (using PrivateAttr) with error handling
            if include_game_count:
                for tag in tags:
                    try:
                        game_count = self._get_tag_game_count(tag.id)
                        tag._game_count = game_count
                    except Exception as e:
                        logger.error(f"Failed to get game count for tag {tag.id}: {e}")
                        tag._game_count = 0  # Graceful degradation

            logger.info(f"Found {len(tags)} tags (total: {total_count}) for user {user_id}")
            return tags, total_count
            
        except Exception as e:
            logger.error(f"Failed to fetch tags for user {user_id}: {e}")
            raise
    
    def get_tag_by_id(self, tag_id: str, user_id: str) -> Optional[Tag]:
        """
        Get a specific tag by ID, ensuring user ownership.
        
        Args:
            tag_id: ID of the tag
            user_id: ID of the user (for ownership verification)
            
        Returns:
            Tag if found and owned by user, None otherwise
        """
        logger.info(f"Fetching tag {tag_id} for user {user_id}")
        
        try:
            tag = self.session.exec(
                select(Tag).where(
                    and_(Tag.id == tag_id, Tag.user_id == user_id)
                )
            ).first()
            
            if tag:
                # Set game count using PrivateAttr
                tag._game_count = self._get_tag_game_count(tag.id)
                logger.info(f"Found tag {tag_id} for user {user_id}")
            else:
                logger.warning(f"Tag {tag_id} not found for user {user_id}")
                
            return tag
            
        except Exception as e:
            logger.error(f"Failed to fetch tag {tag_id} for user {user_id}: {e}")
            raise
    
    def get_tag_by_name(self, name: str, user_id: str) -> Optional[Tag]:
        """
        Get a tag by name for a specific user.
        
        Args:
            name: Name of the tag (case-insensitive)
            user_id: ID of the user
            
        Returns:
            Tag if found, None otherwise
        """
        logger.info(f"Fetching tag by name '{name}' for user {user_id}")
        
        try:
            # Case-insensitive name lookup
            tag = self.session.exec(
                select(Tag).where(
                    and_(
                        func.lower(Tag.name) == name.lower().strip(),
                        Tag.user_id == user_id
                    )
                )
            ).first()
            
            if tag:
                logger.info(f"Found tag '{name}' for user {user_id}")
            else:
                logger.info(f"Tag '{name}' not found for user {user_id}")
                
            return tag
            
        except Exception as e:
            logger.error(f"Failed to fetch tag by name '{name}' for user {user_id}: {e}")
            raise
    
    def create_tag(self, tag_data: TagCreateRequest, user_id: str) -> Tag:
        """
        Create a new tag for a user.
        
        Args:
            tag_data: Tag creation data
            user_id: ID of the user creating the tag
            
        Returns:
            Created tag
            
        Raises:
            ValueError: If tag name already exists for user
        """
        logger.info(f"Creating tag '{tag_data.name}' for user {user_id}")
        
        try:
            # Check if tag name already exists for this user
            existing_tag = self.get_tag_by_name(tag_data.name, user_id)
            if existing_tag:
                logger.warning(f"Tag name '{tag_data.name}' already exists for user {user_id}")
                raise ValueError(f"Tag '{tag_data.name}' already exists")
            
            # Create new tag
            tag = Tag(
                id=str(uuid.uuid4()),
                user_id=user_id,
                name=tag_data.name.strip(),
                color=tag_data.color,
                description=tag_data.description,
                created_at=datetime.now(timezone.utc),
                updated_at=datetime.now(timezone.utc)
            )
            
            self.session.add(tag)
            self.session.commit()
            self.session.refresh(tag)
            
            # Set game count for new tag (always 0) using PrivateAttr
            tag._game_count = 0
            
            logger.info(f"Created tag {tag.id} ('{tag.name}') for user {user_id}")
            return tag
            
        except ValueError:
            # Re-raise validation errors
            raise
        except Exception as e:
            logger.error(f"Failed to create tag '{tag_data.name}' for user {user_id}: {e}")
            self.session.rollback()
            raise
    
    def create_or_get_tag(self, name: str, user_id: str, color: Optional[str] = None) -> Tuple[Tag, bool]:
        """
        Create a new tag or get existing one by name (for inline tag creation).
        
        Args:
            name: Name of the tag
            user_id: ID of the user
            color: Optional color for new tag (defaults to gray if not provided)
            
        Returns:
            Tuple of (tag, was_created)
        """
        logger.info(f"Creating or getting tag '{name}' for user {user_id}")
        
        try:
            # Try to get existing tag
            existing_tag = self.get_tag_by_name(name, user_id)
            if existing_tag:
                # Set game count for existing tag using PrivateAttr
                existing_tag._game_count = self._get_tag_game_count(existing_tag.id)
                logger.info(f"Found existing tag '{name}' for user {user_id}")
                return existing_tag, False
            
            # Create new tag
            tag_data = TagCreateRequest(
                name=name,
                color=color or "#6B7280",  # Default gray color
                description=None
            )
            
            new_tag = self.create_tag(tag_data, user_id)
            logger.info(f"Created new tag '{name}' for user {user_id}")
            return new_tag, True
            
        except Exception as e:
            logger.error(f"Failed to create or get tag '{name}' for user {user_id}: {e}")
            raise
    
    def update_tag(self, tag_id: str, tag_data: TagUpdateRequest, user_id: str) -> Tag:
        """
        Update an existing tag.
        
        Args:
            tag_id: ID of the tag to update
            tag_data: Update data
            user_id: ID of the user (for ownership verification)
            
        Returns:
            Updated tag
            
        Raises:
            ValueError: If tag not found or name conflict
        """
        logger.info(f"Updating tag {tag_id} for user {user_id}")
        
        try:
            # Get existing tag
            tag = self.get_tag_by_id(tag_id, user_id)
            if not tag:
                logger.warning(f"Tag {tag_id} not found for user {user_id}")
                raise ValueError("Tag not found")
            
            # Check for name conflicts if name is being changed
            if tag_data.name and tag_data.name.strip().lower() != tag.name.lower():
                existing_tag = self.get_tag_by_name(tag_data.name, user_id)
                if existing_tag and existing_tag.id != tag_id:
                    logger.warning(f"Tag name '{tag_data.name}' already exists for user {user_id}")
                    raise ValueError(f"Tag '{tag_data.name}' already exists")
            
            # Update fields
            if tag_data.name is not None:
                tag.name = tag_data.name.strip()
            if tag_data.color is not None:
                tag.color = tag_data.color
            if tag_data.description is not None:
                tag.description = tag_data.description
            
            tag.updated_at = datetime.now(timezone.utc)
            
            self.session.add(tag)
            self.session.commit()
            self.session.refresh(tag)
            
            # Set game count for updated tag using PrivateAttr
            tag._game_count = self._get_tag_game_count(tag.id)
            
            logger.info(f"Updated tag {tag_id} for user {user_id}")
            return tag
            
        except ValueError:
            # Re-raise validation errors
            raise
        except Exception as e:
            logger.error(f"Failed to update tag {tag_id} for user {user_id}: {e}")
            self.session.rollback()
            raise
    
    def delete_tag(self, tag_id: str, user_id: str) -> bool:
        """
        Delete a tag and all its associations.
        
        Args:
            tag_id: ID of the tag to delete
            user_id: ID of the user (for ownership verification)
            
        Returns:
            True if deleted, False if not found
        """
        logger.info(f"Deleting tag {tag_id} for user {user_id}")
        
        try:
            # Get tag to verify ownership
            tag = self.get_tag_by_id(tag_id, user_id)
            if not tag:
                logger.warning(f"Tag {tag_id} not found for user {user_id}")
                return False
            
            # Delete all UserGameTag associations first
            user_game_tags = self.session.exec(
                select(UserGameTag).where(UserGameTag.tag_id == tag_id)
            ).all()
            
            for ugt in user_game_tags:
                self.session.delete(ugt)
            
            # Delete the tag itself
            self.session.delete(tag)
            self.session.commit()
            
            logger.info(f"Deleted tag {tag_id} and {len(user_game_tags)} associations for user {user_id}")
            return True
            
        except Exception as e:
            logger.error(f"Failed to delete tag {tag_id} for user {user_id}: {e}")
            self.session.rollback()
            raise
    
    def assign_tags_to_game(self, user_game_id: str, tag_ids: List[str], user_id: str) -> List[UserGameTag]:
        """
        Assign tags to a user game.
        
        Args:
            user_game_id: ID of the user game
            tag_ids: List of tag IDs to assign
            user_id: ID of the user (for ownership verification)
            
        Returns:
            List of created UserGameTag associations
        """
        logger.info(f"Assigning {len(tag_ids)} tags to game {user_game_id} for user {user_id}")
        
        try:
            # Verify user owns the game
            user_game = self.session.exec(
                select(UserGame).where(
                    and_(UserGame.id == user_game_id, UserGame.user_id == user_id)
                )
            ).first()
            
            if not user_game:
                logger.warning(f"User game {user_game_id} not found for user {user_id}")
                raise ValueError("Game not found")
            
            # Verify user owns all tags
            tags = self.session.exec(
                select(Tag).where(
                    and_(in_(Tag.id, tag_ids), Tag.user_id == user_id)
                )
            ).all()

            if len(tags) != len(tag_ids):
                found_tag_ids = {tag.id for tag in tags}
                missing_tags = set(tag_ids) - found_tag_ids
                logger.warning(f"Some tags not found for user {user_id}: {missing_tags}")
                raise ValueError("Some tags not found")

            # Get existing associations to avoid duplicates
            existing_associations = self.session.exec(
                select(UserGameTag).where(
                    and_(
                        UserGameTag.user_game_id == user_game_id,
                        in_(UserGameTag.tag_id, tag_ids)
                    )
                )
            ).all()
            
            existing_tag_ids = {assoc.tag_id for assoc in existing_associations}
            new_tag_ids = set(tag_ids) - existing_tag_ids
            
            # Create new associations
            new_associations = []
            for tag_id in new_tag_ids:
                association = UserGameTag(
                    id=str(uuid.uuid4()),
                    user_game_id=user_game_id,
                    tag_id=tag_id,
                    created_at=datetime.now(timezone.utc)
                )
                self.session.add(association)
                new_associations.append(association)
            
            if new_associations:
                self.session.commit()
                logger.info(f"Created {len(new_associations)} new tag associations for game {user_game_id}")
            else:
                logger.info(f"No new tag associations needed for game {user_game_id}")
            
            return new_associations
            
        except ValueError:
            # Re-raise validation errors
            raise
        except Exception as e:
            logger.error(f"Failed to assign tags to game {user_game_id} for user {user_id}: {e}")
            self.session.rollback()
            raise
    
    def remove_tags_from_game(self, user_game_id: str, tag_ids: List[str], user_id: str) -> int:
        """
        Remove tags from a user game.
        
        Args:
            user_game_id: ID of the user game
            tag_ids: List of tag IDs to remove
            user_id: ID of the user (for ownership verification)
            
        Returns:
            Number of associations removed
        """
        logger.info(f"Removing {len(tag_ids)} tags from game {user_game_id} for user {user_id}")
        
        try:
            # Verify user owns the game
            user_game = self.session.exec(
                select(UserGame).where(
                    and_(UserGame.id == user_game_id, UserGame.user_id == user_id)
                )
            ).first()
            
            if not user_game:
                logger.warning(f"User game {user_game_id} not found for user {user_id}")
                raise ValueError("Game not found")
            
            # Find existing associations
            associations = self.session.exec(
                select(UserGameTag).where(
                    and_(
                        UserGameTag.user_game_id == user_game_id,
                        in_(UserGameTag.tag_id, tag_ids)
                    )
                )
            ).all()
            
            # Delete associations
            removed_count = 0
            for association in associations:
                self.session.delete(association)
                removed_count += 1
            
            if removed_count > 0:
                self.session.commit()
            
            logger.info(f"Removed {removed_count} tag associations from game {user_game_id}")
            return removed_count
            
        except ValueError:
            # Re-raise validation errors
            raise
        except Exception as e:
            logger.error(f"Failed to remove tags from game {user_game_id} for user {user_id}: {e}")
            self.session.rollback()
            raise
    
    def get_tag_usage_stats(self, user_id: str) -> Dict:
        """
        Get comprehensive tag usage statistics for a user.
        
        Args:
            user_id: ID of the user
            
        Returns:
            Dictionary with usage statistics
        """
        logger.info(f"Getting tag usage stats for user {user_id}")
        
        try:
            # Get all user's tags
            all_tags = self.session.exec(
                select(Tag).where(Tag.user_id == user_id)
            ).all()
            
            # Get tag usage counts
            tag_usage: Dict[Any, int] = {}
            popular_tags: list[tuple[Tag, int]] = []
            unused_tags: list[Tag] = []
            
            for tag in all_tags:
                count = self._get_tag_game_count(tag.id)
                tag_usage[tag.id] = count
                
                # Set game count using PrivateAttr for API response
                tag._game_count = count
                
                if count > 0:
                    popular_tags.append((tag, count))
                else:
                    unused_tags.append(tag)
            
            # Sort popular tags by usage count
            popular_tags.sort(key=lambda item: item[1], reverse=True)
            # Extract just the tags for the response
            sorted_popular_tags: list[Tag] = [tag for tag, _count in popular_tags]
            
            # Get total tagged games count
            total_tagged_games = len(self.session.exec(
                select(UserGameTag.user_game_id)
                .join(Tag, Tag.id == UserGameTag.tag_id)  # type: ignore[arg-type]
                .where(Tag.user_id == user_id)
                .distinct()
            ).all())
            
            # Calculate average tags per game
            avg_tags_per_game = 0.0
            if total_tagged_games > 0:
                total_tag_assignments = sum(tag_usage.values())
                avg_tags_per_game = total_tag_assignments / total_tagged_games
            
            stats = {
                "total_tags": len(all_tags),
                "total_tagged_games": total_tagged_games,
                "average_tags_per_game": round(avg_tags_per_game, 2),
                "tag_usage": tag_usage,
                "popular_tags": sorted_popular_tags[:10],  # Top 10
                "unused_tags": unused_tags
            }
            
            logger.info(f"Generated tag usage stats for user {user_id}: {stats['total_tags']} tags, {stats['total_tagged_games']} tagged games")
            return stats
            
        except Exception as e:
            logger.error(f"Failed to get tag usage stats for user {user_id}: {e}")
            raise
    
    def _get_tag_game_count(self, tag_id: str) -> int:
        """
        Get the number of games associated with a tag.
        
        Args:
            tag_id: ID of the tag
            
        Returns:
            Number of games with this tag
        """
        try:
            # Use efficient count query instead of fetching all records
            count_query = select(func.count()).select_from(UserGameTag).where(UserGameTag.tag_id == tag_id)
            count = self.session.exec(count_query).one()
            return count
        except Exception as e:
            logger.error(f"Failed to get game count for tag {tag_id}: {e}")
            return 0
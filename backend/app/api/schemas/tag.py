"""
Tag API schemas for request/response validation.
"""

from pydantic import BaseModel, Field, field_validator, ConfigDict
from typing import Optional, List
from datetime import datetime
import re


class TagResponse(BaseModel):
    """Response schema for tag data."""
    
    id: str = Field(..., description="Unique tag identifier")
    user_id: str = Field(..., description="ID of the user who owns this tag")
    name: str = Field(..., description="Tag name", max_length=100)
    color: str = Field(..., description="Hex color code for the tag")
    description: Optional[str] = Field(None, description="Optional tag description")
    created_at: datetime = Field(..., description="When the tag was created")
    updated_at: datetime = Field(..., description="When the tag was last updated")
    game_count: Optional[int] = Field(None, description="Number of games with this tag")

    model_config = ConfigDict(from_attributes=True)


class TagCreateRequest(BaseModel):
    """Request schema for creating a new tag."""
    
    name: str = Field(..., description="Tag name", min_length=1, max_length=100)
    color: Optional[str] = Field(
        "#6B7280", 
        description="Hex color code for the tag (defaults to gray)",
        pattern=r"^#[0-9A-Fa-f]{6}$"
    )
    description: Optional[str] = Field(None, description="Optional tag description", max_length=500)

    @field_validator('name')
    @classmethod
    def validate_name(cls, v):
        """Validate tag name."""
        if not v or not v.strip():
            raise ValueError('Tag name cannot be empty')
        # Remove extra whitespace and normalize
        v = v.strip()
        if len(v) > 100:
            raise ValueError('Tag name cannot exceed 100 characters')
        return v

    @field_validator('color')
    @classmethod
    def validate_color(cls, v):
        """Validate hex color format."""
        if v and not re.match(r'^#[0-9A-Fa-f]{6}$', v):
            raise ValueError('Color must be a valid hex color code (e.g., #FF0000)')
        return v.upper() if v else "#6B7280"

    @field_validator('description')
    @classmethod
    def validate_description(cls, v):
        """Validate description."""
        if v is not None:
            v = v.strip()
            if not v:  # Empty string after strip
                return None
            if len(v) > 500:
                raise ValueError('Description cannot exceed 500 characters')
        return v


class TagUpdateRequest(BaseModel):
    """Request schema for updating an existing tag."""
    
    name: Optional[str] = Field(None, description="Tag name", min_length=1, max_length=100)
    color: Optional[str] = Field(
        None, 
        description="Hex color code for the tag",
        pattern=r"^#[0-9A-Fa-f]{6}$"
    )
    description: Optional[str] = Field(None, description="Optional tag description", max_length=500)

    @field_validator('name')
    @classmethod
    def validate_name(cls, v):
        """Validate tag name."""
        if v is not None:
            if not v or not v.strip():
                raise ValueError('Tag name cannot be empty')
            # Remove extra whitespace and normalize
            v = v.strip()
            if len(v) > 100:
                raise ValueError('Tag name cannot exceed 100 characters')
        return v

    @field_validator('color')
    @classmethod
    def validate_color(cls, v):
        """Validate hex color format."""
        if v is not None and not re.match(r'^#[0-9A-Fa-f]{6}$', v):
            raise ValueError('Color must be a valid hex color code (e.g., #FF0000)')
        return v.upper() if v else None

    @field_validator('description')
    @classmethod
    def validate_description(cls, v):
        """Validate description."""
        if v is not None:
            v = v.strip()
            if not v:  # Empty string after strip
                return None
            if len(v) > 500:
                raise ValueError('Description cannot exceed 500 characters')
        return v


class TagListResponse(BaseModel):
    """Response schema for paginated tag lists."""
    
    tags: List[TagResponse] = Field(..., description="List of tags")
    total: int = Field(..., description="Total number of tags")
    page: int = Field(..., description="Current page number")
    per_page: int = Field(..., description="Number of tags per page")
    total_pages: int = Field(..., description="Total number of pages")


class TagAssignRequest(BaseModel):
    """Request schema for assigning tags to a user game."""
    
    tag_ids: List[str] = Field(..., description="List of tag IDs to assign")

    @field_validator('tag_ids')
    @classmethod
    def validate_tag_ids(cls, v):
        """Validate tag IDs list."""
        if not v:
            raise ValueError('At least one tag ID is required')
        if len(v) > 50:  # Reasonable limit
            raise ValueError('Cannot assign more than 50 tags at once')
        # Remove duplicates while preserving order
        seen = set()
        unique_ids = []
        for tag_id in v:
            if tag_id not in seen:
                seen.add(tag_id)
                unique_ids.append(tag_id)
        return unique_ids


class TagRemoveRequest(BaseModel):
    """Request schema for removing tags from a user game."""
    
    tag_ids: List[str] = Field(..., description="List of tag IDs to remove")

    @field_validator('tag_ids')
    @classmethod
    def validate_tag_ids(cls, v):
        """Validate tag IDs list."""
        if not v:
            raise ValueError('At least one tag ID is required')
        if len(v) > 50:  # Reasonable limit
            raise ValueError('Cannot remove more than 50 tags at once')
        # Remove duplicates while preserving order
        seen = set()
        unique_ids = []
        for tag_id in v:
            if tag_id not in seen:
                seen.add(tag_id)
                unique_ids.append(tag_id)
        return unique_ids


class BulkTagOperationRequest(BaseModel):
    """Request schema for bulk tag operations on multiple games."""
    
    user_game_ids: List[str] = Field(..., description="List of user game IDs")
    tag_ids: List[str] = Field(..., description="List of tag IDs")

    @field_validator('user_game_ids')
    @classmethod
    def validate_user_game_ids(cls, v):
        """Validate user game IDs list."""
        if not v:
            raise ValueError('At least one game ID is required')
        if len(v) > 100:  # Reasonable limit
            raise ValueError('Cannot operate on more than 100 games at once')
        return list(set(v))  # Remove duplicates

    @field_validator('tag_ids')
    @classmethod
    def validate_tag_ids(cls, v):
        """Validate tag IDs list."""
        if not v:
            raise ValueError('At least one tag ID is required')
        if len(v) > 50:  # Reasonable limit
            raise ValueError('Cannot operate on more than 50 tags at once')
        return list(set(v))  # Remove duplicates


class TagUsageStatsResponse(BaseModel):
    """Response schema for tag usage statistics."""
    
    total_tags: int = Field(..., description="Total number of user's tags")
    total_tagged_games: int = Field(..., description="Total number of games with tags")
    average_tags_per_game: float = Field(..., description="Average tags per game")
    tag_usage: dict = Field(..., description="Mapping of tag_id to game count")
    popular_tags: List[TagResponse] = Field(..., description="Most popular tags")
    unused_tags: List[TagResponse] = Field(..., description="Tags with no games")


class TagCreateOrGetResponse(BaseModel):
    """Response schema for create-or-get operations during inline tag creation."""
    
    tag: TagResponse = Field(..., description="The tag that was created or found")
    created: bool = Field(..., description="Whether the tag was newly created")
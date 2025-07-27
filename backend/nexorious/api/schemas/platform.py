"""
Platform and storefront-related schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, HttpUrl, ConfigDict
from typing import Optional, List
from .common import TimestampMixin


class PlatformCreateRequest(BaseModel):
    """Request schema for creating a platform."""
    name: str = Field(..., min_length=1, max_length=100, description="Platform name (unique identifier)")
    display_name: str = Field(..., min_length=1, max_length=100, description="Display name for platform")
    icon_url: Optional[HttpUrl] = Field(None, description="Platform icon URL")
    is_active: Optional[bool] = Field(True, description="Whether platform is active")


class PlatformUpdateRequest(BaseModel):
    """Request schema for updating a platform."""
    display_name: Optional[str] = Field(None, max_length=100, description="Display name for platform")
    icon_url: Optional[HttpUrl] = Field(None, description="Platform icon URL")
    is_active: Optional[bool] = Field(None, description="Whether platform is active")


class PlatformResponse(BaseModel, TimestampMixin):
    """Response schema for platform data."""
    id: str
    name: str
    display_name: str
    icon_url: Optional[str]
    is_active: bool
    source: str = Field(description="Source of the platform: 'official' or 'custom'")
    version_added: Optional[str] = Field(None, description="Version when this official platform was added")

    model_config = ConfigDict(from_attributes=True)


class StorefrontCreateRequest(BaseModel):
    """Request schema for creating a storefront."""
    name: str = Field(..., min_length=1, max_length=100, description="Storefront name (unique identifier)")
    display_name: str = Field(..., min_length=1, max_length=100, description="Display name for storefront")
    icon_url: Optional[HttpUrl] = Field(None, description="Storefront icon URL")
    base_url: Optional[HttpUrl] = Field(None, description="Base URL for storefront")
    is_active: Optional[bool] = Field(True, description="Whether storefront is active")


class StorefrontUpdateRequest(BaseModel):
    """Request schema for updating a storefront."""
    display_name: Optional[str] = Field(None, max_length=100, description="Display name for storefront")
    icon_url: Optional[HttpUrl] = Field(None, description="Storefront icon URL")
    base_url: Optional[HttpUrl] = Field(None, description="Base URL for storefront")
    is_active: Optional[bool] = Field(None, description="Whether storefront is active")


class StorefrontResponse(BaseModel, TimestampMixin):
    """Response schema for storefront data."""
    id: str
    name: str
    display_name: str
    icon_url: Optional[str]
    base_url: Optional[str]
    is_active: bool
    source: str = Field(description="Source of the storefront: 'official' or 'custom'")
    version_added: Optional[str] = Field(None, description="Version when this official storefront was added")

    model_config = ConfigDict(from_attributes=True)


class PlatformListResponse(BaseModel):
    """Response schema for platform list."""
    platforms: List[PlatformResponse]
    total: int
    page: int = Field(default=1, description="Current page number")
    per_page: int = Field(default=20, description="Items per page")
    pages: int = Field(default=1, description="Total pages")


class StorefrontListResponse(BaseModel):
    """Response schema for storefront list."""
    storefronts: List[StorefrontResponse]
    total: int
    page: int = Field(default=1, description="Current page number")
    per_page: int = Field(default=20, description="Items per page")
    pages: int = Field(default=1, description="Total pages")


class PlatformUsageStats(BaseModel):
    """Usage statistics for a platform."""
    platform_id: str
    platform_name: str
    platform_display_name: str
    usage_count: int = Field(description="Number of users using this platform")


class StorefrontUsageStats(BaseModel):
    """Usage statistics for a storefront."""
    storefront_id: str
    storefront_name: str
    storefront_display_name: str
    usage_count: int = Field(description="Number of users using this storefront")


class PlatformStatsResponse(BaseModel):
    """Response schema for platform usage statistics."""
    platforms: List[PlatformUsageStats]
    total_platforms: int
    total_usage: int = Field(description="Total platform associations across all users")


class StorefrontStatsResponse(BaseModel):
    """Response schema for storefront usage statistics."""
    storefronts: List[StorefrontUsageStats]
    total_storefronts: int
    total_usage: int = Field(description="Total storefront associations across all users")
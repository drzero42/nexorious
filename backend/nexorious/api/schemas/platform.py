"""
Platform and storefront-related schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, HttpUrl, ConfigDict, field_validator
from typing import Optional, List
from .common import TimestampMixin


class PlatformCreateRequest(BaseModel):
    """Request schema for creating a platform."""
    name: str = Field(..., min_length=1, max_length=100, description="Platform name (unique identifier)")
    display_name: str = Field(..., min_length=1, max_length=100, description="Display name for platform")
    icon_url: Optional[str] = Field(None, description="Platform icon URL (full URL or relative path starting with /static/)")
    is_active: Optional[bool] = Field(True, description="Whether platform is active")
    default_storefront_id: Optional[str] = Field(None, description="Default storefront ID for this platform")
    
    @field_validator('icon_url')
    @classmethod
    def validate_icon_url(cls, v: Optional[str]) -> Optional[str]:
        """Validate icon URL - accept full URLs or relative paths starting with /static/."""
        if v is None or v.strip() == "":
            return None
        
        v = v.strip()
        
        # Accept relative paths starting with /static/
        if v.startswith('/static/'):
            return v
        
        # Accept full URLs - validate as HttpUrl
        try:
            HttpUrl(v)
            return v
        except Exception:
            raise ValueError('Icon URL must be a valid URL or a relative path starting with /static/')
        
        return v


class PlatformUpdateRequest(BaseModel):
    """Request schema for updating a platform."""
    display_name: Optional[str] = Field(None, max_length=100, description="Display name for platform")
    icon_url: Optional[str] = Field(None, description="Platform icon URL (full URL or relative path starting with /static/)")
    is_active: Optional[bool] = Field(None, description="Whether platform is active")
    default_storefront_id: Optional[str] = Field(None, description="Default storefront ID for this platform")
    
    @field_validator('icon_url')
    @classmethod
    def validate_icon_url(cls, v: Optional[str]) -> Optional[str]:
        """Validate icon URL - accept full URLs or relative paths starting with /static/."""
        if v is None or v.strip() == "":
            return None
        
        v = v.strip()
        
        # Accept relative paths starting with /static/
        if v.startswith('/static/'):
            return v
        
        # Accept full URLs - validate as HttpUrl
        try:
            HttpUrl(v)
            return v
        except Exception:
            raise ValueError('Icon URL must be a valid URL or a relative path starting with /static/')
        
        return v


class PlatformResponse(BaseModel, TimestampMixin):
    """Response schema for platform data."""
    id: str
    name: str
    display_name: str
    icon_url: Optional[str]
    is_active: bool
    source: str = Field(description="Source of the platform: 'official' or 'custom'")
    version_added: Optional[str] = Field(None, description="Version when this official platform was added")
    default_storefront_id: Optional[str] = Field(None, description="Default storefront ID for this platform")
    storefronts: List['StorefrontResponse'] = Field(default_factory=list, description="Associated storefronts for this platform")

    model_config = ConfigDict(from_attributes=True)


class StorefrontCreateRequest(BaseModel):
    """Request schema for creating a storefront."""
    name: str = Field(..., min_length=1, max_length=100, description="Storefront name (unique identifier)")
    display_name: str = Field(..., min_length=1, max_length=100, description="Display name for storefront")
    icon_url: Optional[str] = Field(None, description="Storefront icon URL (full URL or relative path starting with /static/)")
    base_url: Optional[HttpUrl] = Field(None, description="Base URL for storefront")
    is_active: Optional[bool] = Field(True, description="Whether storefront is active")
    
    @field_validator('icon_url')
    @classmethod
    def validate_icon_url(cls, v: Optional[str]) -> Optional[str]:
        """Validate icon URL - accept full URLs or relative paths starting with /static/."""
        if v is None or v.strip() == "":
            return None
        
        v = v.strip()
        
        # Accept relative paths starting with /static/
        if v.startswith('/static/'):
            return v
        
        # Accept full URLs - validate as HttpUrl
        try:
            HttpUrl(v)
            return v
        except Exception:
            raise ValueError('Icon URL must be a valid URL or a relative path starting with /static/')
        
        return v


class StorefrontUpdateRequest(BaseModel):
    """Request schema for updating a storefront."""
    display_name: Optional[str] = Field(None, max_length=100, description="Display name for storefront")
    icon_url: Optional[str] = Field(None, description="Storefront icon URL (full URL or relative path starting with /static/)")
    base_url: Optional[HttpUrl] = Field(None, description="Base URL for storefront")
    is_active: Optional[bool] = Field(None, description="Whether storefront is active")
    
    @field_validator('icon_url')
    @classmethod
    def validate_icon_url(cls, v: Optional[str]) -> Optional[str]:
        """Validate icon URL - accept full URLs or relative paths starting with /static/."""
        if v is None or v.strip() == "":
            return None
        
        v = v.strip()
        
        # Accept relative paths starting with /static/
        if v.startswith('/static/'):
            return v
        
        # Accept full URLs - validate as HttpUrl
        try:
            HttpUrl(v)
            return v
        except Exception:
            raise ValueError('Icon URL must be a valid URL or a relative path starting with /static/')
        
        return v


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


class SeedDataResponse(BaseModel):
    """Response schema for seed data loading operation."""
    platforms_added: int = Field(description="Number of platforms added or updated")
    storefronts_added: int = Field(description="Number of storefronts added or updated")
    mappings_created: int = Field(description="Number of platform-storefront default mappings created")
    total_changes: int = Field(description="Total number of changes made")
    message: str = Field(description="Summary message of the operation")


class PlatformDefaultMapping(BaseModel):
    """Response schema for platform default storefront mapping."""
    platform_id: str
    platform_name: str
    platform_display_name: str
    default_storefront: Optional['StorefrontResponse'] = Field(None, description="Default storefront for this platform")

    model_config = ConfigDict(from_attributes=True)


class UpdatePlatformDefaultRequest(BaseModel):
    """Request schema for updating platform default storefront."""
    storefront_id: Optional[str] = Field(None, description="Storefront ID to set as default, or null to remove default")


class PlatformStorefrontsResponse(BaseModel):
    """Response schema for platform storefronts list."""
    platform_id: str
    platform_name: str
    platform_display_name: str
    storefronts: List[StorefrontResponse]
    total_storefronts: int = Field(description="Total number of associated storefronts")

    model_config = ConfigDict(from_attributes=True)


class PlatformStorefrontAssociationResponse(BaseModel):
    """Response schema for platform-storefront association operations."""
    platform_id: str
    platform_name: str
    platform_display_name: str
    storefront_id: str
    storefront_name: str
    storefront_display_name: str
    message: str = Field(description="Operation result message")

    model_config = ConfigDict(from_attributes=True)
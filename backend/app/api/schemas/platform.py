"""
Platform and storefront-related schemas for API requests and responses.
"""

from pydantic import BaseModel, Field, HttpUrl, ConfigDict, field_validator
from typing import Optional, List, Literal
from datetime import datetime
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


# Platform Resolution Schemas

class PlatformSuggestion(BaseModel):
    """A suggested platform match for an unknown platform name."""
    platform_id: str = Field(description="ID of the suggested platform")
    platform_name: str = Field(description="Name of the suggested platform")
    platform_display_name: str = Field(description="Display name of the suggested platform")
    confidence: float = Field(ge=0.0, le=1.0, description="Confidence score (0.0 to 1.0)")
    match_type: Literal["exact", "fuzzy", "partial"] = Field(description="Type of match")
    reason: Optional[str] = Field(None, description="Reason for suggestion")


class StorefrontSuggestion(BaseModel):
    """A suggested storefront match for an unknown storefront name."""
    storefront_id: str = Field(description="ID of the suggested storefront")
    storefront_name: str = Field(description="Name of the suggested storefront")
    storefront_display_name: str = Field(description="Display name of the suggested storefront")
    confidence: float = Field(ge=0.0, le=1.0, description="Confidence score (0.0 to 1.0)")
    match_type: Literal["exact", "fuzzy", "partial"] = Field(description="Type of match")
    reason: Optional[str] = Field(None, description="Reason for suggestion")


class PlatformResolutionData(BaseModel):
    """Platform resolution data structure for JSONB storage."""
    status: Literal["pending", "suggested", "resolved", "failed"] = Field(description="Resolution status")
    original_name: str = Field(description="Original platform name from CSV")
    suggestions: List[PlatformSuggestion] = Field(default_factory=list, description="Platform suggestions")
    storefront_suggestions: List[StorefrontSuggestion] = Field(default_factory=list, description="Storefront suggestions")
    resolved_platform_id: Optional[str] = Field(None, description="ID of resolved platform")
    resolved_storefront_id: Optional[str] = Field(None, description="ID of resolved storefront")
    resolution_timestamp: Optional[datetime] = Field(None, description="When resolution was completed")
    resolution_method: Optional[Literal["auto", "manual", "admin_created"]] = Field(None, description="How resolution was completed")
    user_notes: Optional[str] = Field(None, max_length=500, description="User notes about the resolution")


class PendingPlatformResolution(BaseModel):
    """A pending platform resolution from CSV import."""
    import_id: str = Field(description="DarkadiaImport record ID")
    user_id: str = Field(description="User ID who owns the import")
    original_platform_name: str = Field(description="Original platform name from CSV")
    original_storefront_name: Optional[str] = Field(None, description="Original storefront name from CSV")
    affected_games_count: int = Field(ge=1, description="Number of games affected by this platform")
    affected_games: List[str] = Field(description="Names of affected games (for display)")
    resolution_data: PlatformResolutionData = Field(description="Current resolution data")
    created_at: datetime = Field(description="When this resolution was identified")
    
    model_config = ConfigDict(from_attributes=True)


class PlatformSuggestionsRequest(BaseModel):
    """Request schema for getting platform suggestions."""
    unknown_platform_name: str = Field(..., min_length=1, max_length=200, description="Unknown platform name to find suggestions for")
    unknown_storefront_name: Optional[str] = Field(None, max_length=200, description="Unknown storefront name to find suggestions for")
    min_confidence: float = Field(default=0.6, ge=0.0, le=1.0, description="Minimum confidence threshold for suggestions")
    max_suggestions: int = Field(default=5, ge=1, le=20, description="Maximum number of suggestions to return")


class PlatformSuggestionsResponse(BaseModel):
    """Response schema for platform suggestions."""
    unknown_platform_name: str = Field(description="Original unknown platform name")
    unknown_storefront_name: Optional[str] = Field(None, description="Original unknown storefront name")
    platform_suggestions: List[PlatformSuggestion] = Field(description="Platform suggestions")
    storefront_suggestions: List[StorefrontSuggestion] = Field(description="Storefront suggestions")
    total_platform_suggestions: int = Field(description="Total number of platform suggestions")
    total_storefront_suggestions: int = Field(description="Total number of storefront suggestions")


class PlatformResolutionRequest(BaseModel):
    """Request schema for resolving a platform."""
    import_id: str = Field(description="DarkadiaImport record ID to resolve")
    resolved_platform_id: Optional[str] = Field(None, description="ID of platform to resolve to")
    resolved_storefront_id: Optional[str] = Field(None, description="ID of storefront to resolve to")
    user_notes: Optional[str] = Field(None, max_length=500, description="User notes about the resolution")


class BulkPlatformResolutionRequest(BaseModel):
    """Request schema for bulk platform resolution."""
    resolutions: List[PlatformResolutionRequest] = Field(..., min_length=1, max_length=50, description="List of platform resolutions")


class PlatformResolutionResult(BaseModel):
    """Result of a single platform resolution."""
    import_id: str = Field(description="DarkadiaImport record ID")
    success: bool = Field(description="Whether resolution was successful")
    resolved_platform: Optional[PlatformResponse] = Field(None, description="Resolved platform details")
    resolved_storefront: Optional[StorefrontResponse] = Field(None, description="Resolved storefront details")
    error_message: Optional[str] = Field(None, description="Error message if resolution failed")


class BulkPlatformResolutionResponse(BaseModel):
    """Response schema for bulk platform resolution."""
    total_processed: int = Field(description="Total resolutions processed")
    successful_resolutions: int = Field(description="Number of successful resolutions")
    failed_resolutions: int = Field(description="Number of failed resolutions")
    results: List[PlatformResolutionResult] = Field(description="Individual resolution results")
    errors: List[str] = Field(default_factory=list, description="General errors during bulk operation")


class PendingResolutionsListResponse(BaseModel):
    """Response schema for listing pending platform resolutions."""
    pending_resolutions: List[PendingPlatformResolution] = Field(description="List of pending resolutions")
    total: int = Field(description="Total number of pending resolutions")
    page: int = Field(default=1, description="Current page number")
    per_page: int = Field(default=20, description="Items per page")
    pages: int = Field(default=1, description="Total number of pages")
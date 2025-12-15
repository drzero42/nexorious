"""Schemas for platform/storefront mapping resolution."""

from pydantic import BaseModel, Field
from typing import Optional


class UnresolvedMapping(BaseModel):
    """An unresolved platform or storefront name from import."""

    original_name: str = Field(description="Name from import source")
    suggested_id: Optional[str] = Field(default=None, description="Suggested match ID")
    suggested_name: Optional[str] = Field(default=None, description="Suggested match name")
    confidence: float = Field(default=0.0, description="Match confidence 0.0-1.0")
    type: str = Field(description="'platform' or 'storefront'")


class UnresolvedMappingsResponse(BaseModel):
    """Response with all unresolved mappings for a job."""

    platforms: list[UnresolvedMapping]
    storefronts: list[UnresolvedMapping]
    all_resolved: bool = Field(description="True if no user input needed")


class MappingResolution(BaseModel):
    """User's resolution for a single mapping."""

    original_name: str
    resolved_id: str
    type: str = Field(description="'platform' or 'storefront'")


class ResolveMappingsRequest(BaseModel):
    """Request to resolve multiple mappings."""

    resolutions: list[MappingResolution]

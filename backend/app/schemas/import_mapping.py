"""
Pydantic schemas for Import Mapping API endpoints.
"""

from pydantic import BaseModel, Field
from typing import Optional, List
from datetime import datetime
from enum import Enum


class MappingType(str, Enum):
    """Type of import mapping."""

    PLATFORM = "platform"
    STOREFRONT = "storefront"


# ============================================================================
# Response Schemas
# ============================================================================


class ImportMappingResponse(BaseModel):
    """Response schema for a single import mapping."""

    model_config = {"from_attributes": True}

    id: str
    user_id: str
    import_source: str
    mapping_type: MappingType
    source_value: str
    target_id: str
    created_at: datetime
    updated_at: datetime


class ImportMappingListResponse(BaseModel):
    """Response schema for listing import mappings."""

    items: List[ImportMappingResponse]
    total: int


# ============================================================================
# Request Schemas
# ============================================================================


class ImportMappingCreateRequest(BaseModel):
    """Request schema for creating an import mapping."""

    import_source: str = Field(..., min_length=1, max_length=50)
    mapping_type: MappingType
    source_value: str = Field(..., min_length=1, max_length=255)
    target_id: str = Field(..., min_length=1, max_length=100)


class ImportMappingUpdateRequest(BaseModel):
    """Request schema for updating an import mapping."""

    target_id: str = Field(..., min_length=1, max_length=100)


class BatchMappingItem(BaseModel):
    """A single mapping in a batch request."""

    mapping_type: MappingType
    source_value: str = Field(..., min_length=1, max_length=255)
    target_id: str = Field(..., min_length=1, max_length=100)


class BatchImportMappingRequest(BaseModel):
    """Request schema for batch creating/updating import mappings."""

    import_source: str = Field(..., min_length=1, max_length=50)
    mappings: List[BatchMappingItem]


class BatchImportMappingResponse(BaseModel):
    """Response schema for batch import mapping operation."""

    created: int
    updated: int

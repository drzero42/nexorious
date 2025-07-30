"""
Common schemas used across the API.
"""

from pydantic import BaseModel, Field
from typing import Optional, List, Dict, Any
from datetime import datetime


class SuccessResponse(BaseModel):
    """Standard success response."""
    success: bool = True
    message: str
    
    # Optional fields for bulk operations
    updated_count: Optional[int] = None
    deleted_count: Optional[int] = None
    failed_count: Optional[int] = None


class ErrorResponse(BaseModel):
    """Standard error response."""
    error: str
    detail: Optional[str] = None
    status_code: int


class PaginationParams(BaseModel):
    """Pagination parameters for list endpoints."""
    page: int = Field(default=1, ge=1, description="Page number")
    per_page: int = Field(default=20, ge=1, le=100, description="Items per page")


class PaginatedResponse(BaseModel):
    """Paginated response wrapper."""
    items: List[Any]
    total: int
    page: int
    per_page: int
    pages: int


class SortParams(BaseModel):
    """Sorting parameters."""
    sort_by: Optional[str] = Field(default=None, description="Field to sort by")
    sort_order: Optional[str] = Field(default="asc", pattern="^(asc|desc)$", description="Sort order")


class SearchParams(BaseModel):
    """Search parameters."""
    q: Optional[str] = Field(default=None, description="Search query")
    filters: Optional[Dict[str, Any]] = Field(default_factory=dict, description="Additional filters")


class TimestampMixin:
    """Mixin for models with timestamps."""
    created_at: datetime
    updated_at: datetime
"""
Pydantic schemas for Review queue API.

Provides request/response models for review item listing, detail retrieval,
and matching/resolution endpoints.
"""

from pydantic import BaseModel, Field, ConfigDict
from typing import Optional, List, Dict, Any
from datetime import datetime
from enum import Enum


class ReviewItemStatus(str, Enum):
    """Status of a review item."""

    PENDING = "pending"
    MATCHED = "matched"
    SKIPPED = "skipped"
    REMOVAL = "removal"


class IGDBCandidate(BaseModel):
    """IGDB search result candidate for matching."""

    igdb_id: int
    name: str
    first_release_date: Optional[int] = None
    cover_url: Optional[str] = None
    summary: Optional[str] = None
    platforms: Optional[List[str]] = None
    similarity_score: Optional[float] = None


class ReviewItemResponse(BaseModel):
    """Response model for a single review item."""

    model_config = ConfigDict(from_attributes=True)

    id: str
    job_id: str
    user_id: str
    status: ReviewItemStatus

    # Source data
    source_title: str
    source_metadata: Dict[str, Any] = Field(default_factory=dict)

    # IGDB matching
    igdb_candidates: List[Dict[str, Any]] = Field(default_factory=list)
    resolved_igdb_id: Optional[int] = None

    # Timestamps
    created_at: datetime
    resolved_at: Optional[datetime] = None

    # Computed job context
    job_type: Optional[str] = None
    job_source: Optional[str] = None


class ReviewItemDetailResponse(BaseModel):
    """Detailed response model for a single review item with full IGDB candidates."""

    model_config = ConfigDict(from_attributes=True)

    id: str
    job_id: str
    user_id: str
    status: ReviewItemStatus

    # Source data
    source_title: str
    source_metadata: Dict[str, Any] = Field(default_factory=dict)

    # IGDB matching
    igdb_candidates: List[IGDBCandidate] = Field(default_factory=list)
    resolved_igdb_id: Optional[int] = None

    # Timestamps
    created_at: datetime
    resolved_at: Optional[datetime] = None

    # Job context
    job_type: Optional[str] = None
    job_source: Optional[str] = None


class ReviewListResponse(BaseModel):
    """Response model for paginated review item list."""

    items: List[ReviewItemResponse]
    total: int
    page: int
    per_page: int
    pages: int


class MatchRequest(BaseModel):
    """Request model for matching a review item to an IGDB ID."""

    igdb_id: int = Field(..., description="The IGDB ID to match this item to")


class MatchResponse(BaseModel):
    """Response model for match/skip/keep/remove operations."""

    success: bool
    message: str
    item: Optional[ReviewItemResponse] = None


class ConfirmImportResponse(BaseModel):
    """Response model for confirming an import after review."""

    success: bool
    message: str
    job_id: str
    games_added: int = 0
    games_skipped: int = 0
    games_removed: int = 0


class ReviewSummary(BaseModel):
    """Summary statistics for review items."""

    total_pending: int
    total_matched: int
    total_skipped: int
    total_removal: int
    jobs_with_pending: int

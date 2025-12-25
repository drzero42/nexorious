"""Schemas for JobItem API responses."""

from datetime import datetime
from pydantic import BaseModel, computed_field

from app.models.job import JobItemStatus


class JobItemResponse(BaseModel):
    """Basic job item response."""

    id: str
    job_id: str
    item_key: str
    source_title: str
    status: JobItemStatus
    error_message: str | None
    match_confidence: float | None
    created_at: datetime
    processed_at: datetime | None

    model_config = {"from_attributes": True}


class JobItemDetailResponse(JobItemResponse):
    """Detailed job item response with IGDB candidates."""

    source_metadata_json: str
    result_json: str
    igdb_candidates_json: str
    resolved_igdb_id: int | None
    resolved_at: datetime | None


class JobItemListResponse(BaseModel):
    """Paginated list of job items."""

    items: list[JobItemResponse]
    total: int
    page: int
    page_size: int


class ResolveJobItemRequest(BaseModel):
    """Request to resolve a job item to an IGDB game."""

    igdb_id: int


class SkipJobItemRequest(BaseModel):
    """Request to skip a job item."""

    reason: str | None = None

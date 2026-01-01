"""Schemas for JobItem API responses."""

import json
from datetime import datetime
from typing import Any

from pydantic import BaseModel, model_validator

from app.models.job import JobItemStatus


class JobItemResponse(BaseModel):
    """Basic job item response."""

    model_config = {"from_attributes": True}

    id: str
    job_id: str
    item_key: str
    source_title: str
    status: JobItemStatus
    error_message: str | None = None
    match_confidence: float | None = None

    # For completed items - the matched game info
    result_game_title: str | None = None
    result_igdb_id: int | None = None
    result_user_game_id: str | None = None

    created_at: datetime
    processed_at: datetime | None = None

    @model_validator(mode="before")
    @classmethod
    def extract_result_fields(cls, data: Any) -> Any:
        """Extract result_game_title, result_igdb_id, and result_user_game_id from result_json."""
        # Get result_json from the input data
        result_json_str: str | None = None
        if isinstance(data, dict):
            result_json_str = data.get("result_json")
        elif hasattr(data, "result_json"):
            result_json_str = getattr(data, "result_json", None)

        if not result_json_str:
            return data

        try:
            result = json.loads(result_json_str)
            if not isinstance(result, dict):
                return data

            # Extract title - check multiple possible keys
            game_title = result.get("game_title") or result.get("title") or result.get("igdb_title")

            # Extract igdb_id if present
            igdb_id = result.get("igdb_id")
            if igdb_id is not None:
                try:
                    igdb_id = int(igdb_id)
                except (ValueError, TypeError):
                    igdb_id = None

            # Extract user_game_id if present
            user_game_id = result.get("user_game_id")

            # Handle both dict and ORM object input
            if isinstance(data, dict):
                if game_title:
                    data["result_game_title"] = game_title
                if igdb_id is not None:
                    data["result_igdb_id"] = igdb_id
                if user_game_id:
                    data["result_user_game_id"] = user_game_id
            else:
                # For ORM objects, create a dict with all fields
                data_dict: dict[str, Any] = {}
                for field_name in cls.model_fields:
                    if hasattr(data, field_name):
                        data_dict[field_name] = getattr(data, field_name)
                if game_title:
                    data_dict["result_game_title"] = game_title
                if igdb_id is not None:
                    data_dict["result_igdb_id"] = igdb_id
                if user_game_id:
                    data_dict["result_user_game_id"] = user_game_id
                return data_dict

        except (json.JSONDecodeError, TypeError):
            pass

        return data


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
    pages: int


class ResolveJobItemRequest(BaseModel):
    """Request to resolve a job item to an IGDB game."""

    igdb_id: int


class SkipJobItemRequest(BaseModel):
    """Request to skip a job item."""

    reason: str | None = None

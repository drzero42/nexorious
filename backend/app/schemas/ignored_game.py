"""
Pydantic schemas for Ignored Games API.

Provides request/response models for managing games that users
have explicitly ignored from external sync sources.
"""

from pydantic import BaseModel, ConfigDict
from typing import List
from datetime import datetime

from ..models.job import BackgroundJobSource


class IgnoredGameResponse(BaseModel):
    """Response model for a single ignored game."""

    model_config = ConfigDict(from_attributes=True)

    id: str
    source: BackgroundJobSource
    external_id: str
    title: str
    created_at: datetime


class IgnoredGameListResponse(BaseModel):
    """Response model for list of ignored games."""

    items: List[IgnoredGameResponse]
    total: int

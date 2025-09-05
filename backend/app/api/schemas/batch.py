"""
Batch processing API schemas.

This module defines the request and response schemas for batch processing
endpoints, providing consistent data structures for batched operations.
"""

from pydantic import BaseModel
from typing import List, Union
from datetime import datetime

from .steam import SteamGameResponse
from .darkadia import DarkadiaGameResponse


class BatchSessionStartRequest(BaseModel):
    """Request to start a new batch session."""
    # No additional fields needed - all info comes from user context and operation type
    pass


class BatchSessionStartResponse(BaseModel):
    """Response when starting a new batch session."""
    session_id: str
    total_items: int
    operation_type: str
    status: str
    message: str


class BatchProgressResponse(BaseModel):
    """Response containing batch processing progress information."""
    session_id: str
    operation_type: str
    total_items: int
    processed_items: int
    successful_items: int
    failed_items: int
    remaining_items: int
    progress_percentage: float
    status: str
    is_complete: bool
    created_at: datetime
    updated_at: datetime
    errors: List[str]


class BatchNextRequest(BaseModel):
    """Request to process the next batch of items."""
    # No additional fields needed - batch size is fixed per operation type
    pass


class BatchNextResponse(BaseModel):
    """Response after processing a batch of items."""
    session_id: str
    batch_processed: int
    batch_successful: int
    batch_failed: int
    batch_errors: List[str]
    current_batch_items: List[Union[SteamGameResponse, DarkadiaGameResponse]]
    # Overall progress
    total_items: int
    processed_items: int
    successful_items: int
    failed_items: int
    remaining_items: int
    progress_percentage: float
    status: str
    is_complete: bool
    message: str


class BatchCancelResponse(BaseModel):
    """Response when cancelling a batch session."""
    session_id: str
    status: str
    processed_items: int
    successful_items: int
    failed_items: int
    message: str


class BatchStatusResponse(BaseModel):
    """Response for batch status check."""
    session_id: str
    operation_type: str
    total_items: int
    processed_items: int
    successful_items: int
    failed_items: int
    remaining_items: int
    progress_percentage: float
    status: str
    is_complete: bool
    created_at: datetime
    updated_at: datetime
    errors: List[str]
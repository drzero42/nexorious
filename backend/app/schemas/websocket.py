"""
WebSocket message schemas for real-time job updates.

Defines the message format for WebSocket communication between
the server and connected clients.
"""

from pydantic import BaseModel, ConfigDict
from typing import Optional
from datetime import datetime
from enum import Enum

from .job import JobResponse


class WebSocketEventType(str, Enum):
    """Types of events sent over the WebSocket connection."""

    # Connection lifecycle
    CONNECTED = "connected"
    ERROR = "error"

    # Job events
    JOB_CREATED = "job_created"
    JOB_PROGRESS = "job_progress"
    JOB_STATUS_CHANGE = "job_status_change"
    JOB_COMPLETED = "job_completed"
    JOB_FAILED = "job_failed"

    # Review item events
    REVIEW_ITEM_UPDATE = "review_item_update"


class WebSocketMessage(BaseModel):
    """Base message for all WebSocket events."""

    model_config = ConfigDict(use_enum_values=True)

    event: WebSocketEventType
    timestamp: datetime


class ConnectionMessage(WebSocketMessage):
    """Message sent on connection or error."""

    user_id: Optional[str] = None
    message: Optional[str] = None


class JobWebSocketMessage(WebSocketMessage):
    """Message containing a job update."""

    job: JobResponse

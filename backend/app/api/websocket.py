"""
WebSocket endpoint for real-time job updates.

Provides a read-only WebSocket connection that streams job status
updates to authenticated clients using database polling.
"""

import asyncio
import logging
from dataclasses import dataclass
from datetime import datetime, timezone, timedelta
from typing import Callable, Optional

from fastapi import APIRouter, WebSocket, WebSocketDisconnect, Query
from jose import JWTError
from sqlmodel import Session, select, col

from ..core.config import settings
from ..core.database import get_engine
from ..core.security import hash_token
from ..models.user import User, UserSession
from ..models.job import Job, ReviewItem, BackgroundJobStatus, ReviewItemStatus
from ..schemas.job import JobResponse, JobType, JobSource, JobStatus
from ..schemas.websocket import (
    WebSocketEventType,
    ConnectionMessage,
    JobWebSocketMessage,
)
from ..utils.sqlalchemy_typed import desc
from jose import jwt

router = APIRouter(tags=["WebSocket"])
logger = logging.getLogger(__name__)

# Configuration
POLL_INTERVAL_SECONDS = 1.0
RECENTLY_COMPLETED_WINDOW_SECONDS = 5

# Optional session factory override for testing
_session_factory: Optional[Callable[[], Session]] = None


def set_session_factory(factory: Optional[Callable[[], Session]]) -> None:
    """Set a custom session factory for testing."""
    global _session_factory
    _session_factory = factory


def _get_session() -> Session:
    """Get a database session, using the override if set."""
    if _session_factory is not None:
        return _session_factory()
    return Session(get_engine())


@dataclass
class JobSnapshot:
    """Snapshot of job state for change detection."""

    status: str
    progress_current: int
    progress_total: int
    review_item_count: int
    pending_review_count: int


def _get_ws_user(token: str, session: Session) -> Optional[User]:
    """
    Validate JWT token and return the associated user.

    Args:
        token: JWT access token
        session: Database session

    Returns:
        User object if valid, None otherwise
    """
    try:
        payload = jwt.decode(
            token, settings.secret_key, algorithms=[settings.algorithm]
        )

        # Check token type
        if payload.get("type") != "access":
            logger.warning("WebSocket auth failed: invalid token type")
            return None

        user_id = payload.get("sub")
        if user_id is None:
            logger.warning("WebSocket auth failed: no user_id in token")
            return None

        # Check if session still exists
        token_hash = hash_token(token)
        session_record = session.exec(
            select(UserSession).where(
                (UserSession.user_id == user_id) & (UserSession.token_hash == token_hash)
            )
        ).first()

        if not session_record:
            logger.warning(f"WebSocket auth failed: no session for user {user_id}")
            return None

        user = session.get(User, user_id)
        if not user:
            logger.warning(f"WebSocket auth failed: user {user_id} not found")
            return None

        if not user.is_active:
            logger.warning(f"WebSocket auth failed: user {user_id} is inactive")
            return None

        return user

    except JWTError as e:
        logger.warning(f"WebSocket auth failed: JWT error - {e}")
        return None


def _job_to_response(job: Job, session: Session) -> JobResponse:
    """Convert a Job model to JobResponse with computed fields."""
    from sqlmodel import func

    # Count review items for this job
    review_count_stmt = (
        select(func.count()).select_from(ReviewItem).where(ReviewItem.job_id == job.id)
    )
    review_item_count = session.exec(review_count_stmt).one()

    # Count pending review items
    pending_count_stmt = (
        select(func.count())
        .select_from(ReviewItem)
        .where(
            ReviewItem.job_id == job.id,
            ReviewItem.status == ReviewItemStatus.PENDING,
        )
    )
    pending_review_count = session.exec(pending_count_stmt).one()

    return JobResponse(
        id=job.id,
        user_id=job.user_id,
        job_type=JobType(job.job_type.value),
        source=JobSource(job.source.value),
        status=JobStatus(job.status.value),
        priority=job.priority,
        progress_current=job.progress_current,
        progress_total=job.progress_total,
        progress_percent=job.progress_percent,
        result_summary=job.get_result_summary(),
        error_message=job.error_message,
        file_path=job.file_path,
        taskiq_task_id=job.taskiq_task_id,
        created_at=job.created_at,
        started_at=job.started_at,
        completed_at=job.completed_at,
        is_terminal=job.is_terminal,
        duration_seconds=job.duration_seconds,
        review_item_count=review_item_count,
        pending_review_count=pending_review_count,
    )


def _create_snapshot(job: Job, review_item_count: int, pending_review_count: int) -> JobSnapshot:
    """Create a snapshot of job state for change detection."""
    return JobSnapshot(
        status=job.status.value,
        progress_current=job.progress_current,
        progress_total=job.progress_total,
        review_item_count=review_item_count,
        pending_review_count=pending_review_count,
    )


def _detect_event_type(
    old_snapshot: Optional[JobSnapshot], new_snapshot: JobSnapshot
) -> Optional[WebSocketEventType]:
    """
    Detect what type of event occurred based on snapshot comparison.

    Returns None if no significant change detected.
    """
    # New job
    if old_snapshot is None:
        return WebSocketEventType.JOB_CREATED

    # Status changes take priority
    if old_snapshot.status != new_snapshot.status:
        if new_snapshot.status == "completed":
            return WebSocketEventType.JOB_COMPLETED
        elif new_snapshot.status == "failed":
            return WebSocketEventType.JOB_FAILED
        else:
            return WebSocketEventType.JOB_STATUS_CHANGE

    # Progress change
    if old_snapshot.progress_current != new_snapshot.progress_current:
        return WebSocketEventType.JOB_PROGRESS

    # Review item change
    if (
        old_snapshot.review_item_count != new_snapshot.review_item_count
        or old_snapshot.pending_review_count != new_snapshot.pending_review_count
    ):
        return WebSocketEventType.REVIEW_ITEM_UPDATE

    return None


async def _send_message(websocket: WebSocket, message: ConnectionMessage | JobWebSocketMessage) -> bool:
    """
    Send a message over WebSocket.

    Returns True if successful, False if connection is closed.
    """
    try:
        await websocket.send_json(message.model_dump(mode="json"))
        return True
    except Exception as e:
        logger.debug(f"Failed to send WebSocket message: {e}")
        return False


@router.websocket("/ws/jobs")
async def websocket_jobs(
    websocket: WebSocket,
    token: str = Query(..., description="JWT access token"),
):
    """
    WebSocket endpoint for real-time job updates.

    Authenticates via JWT token in query parameter, then streams
    job updates for all active jobs belonging to the user.

    Events sent:
    - connected: On successful authentication
    - error: On authentication failure
    - job_created: New job started
    - job_progress: Progress updated
    - job_status_change: Status changed
    - job_completed: Job finished successfully
    - job_failed: Job encountered error
    - review_item_update: Review item counts changed
    """
    # Create session for authentication
    session = _get_session()
    user: Optional[User] = None

    try:
        # Authenticate
        user = _get_ws_user(token, session)

        if not user:
            await websocket.accept()
            error_msg = ConnectionMessage(
                event=WebSocketEventType.ERROR,
                timestamp=datetime.now(timezone.utc),
                message="Invalid or expired token",
            )
            await _send_message(websocket, error_msg)
            await websocket.close(code=4001, reason="Authentication failed")
            return

        # Accept connection
        await websocket.accept()
        logger.info(f"WebSocket connected for user {user.id}")

        # Send connected message
        connected_msg = ConnectionMessage(
            event=WebSocketEventType.CONNECTED,
            timestamp=datetime.now(timezone.utc),
            user_id=user.id,
        )
        if not await _send_message(websocket, connected_msg):
            return

        # Track job states
        job_snapshots: dict[str, JobSnapshot] = {}

        # Polling loop
        while True:
            try:
                # Refresh session to get fresh data
                session.expire_all()

                # Query active jobs for user
                # Include non-terminal jobs AND jobs that completed recently
                # Use timezone-aware datetime for comparison (PostgreSQL returns aware timestamps)
                cutoff_time = datetime.now(timezone.utc) - timedelta(
                    seconds=RECENTLY_COMPLETED_WINDOW_SECONDS
                )

                jobs = session.exec(
                    select(Job)
                    .where(Job.user_id == user.id)
                    .where(
                        (col(Job.status).not_in([
                            BackgroundJobStatus.COMPLETED,
                            BackgroundJobStatus.FAILED,
                            BackgroundJobStatus.CANCELLED,
                        ]))
                        | (
                            (Job.completed_at.is_not(None))  # type: ignore[union-attr]
                            & (col(Job.completed_at) > cutoff_time)
                        )
                    )
                    .order_by(desc(col(Job.created_at)))
                ).all()

                # Process each job
                current_job_ids = set()
                for job in jobs:
                    current_job_ids.add(job.id)

                    # Get review counts
                    from sqlmodel import func

                    review_count = session.exec(
                        select(func.count())
                        .select_from(ReviewItem)
                        .where(ReviewItem.job_id == job.id)
                    ).one()

                    pending_count = session.exec(
                        select(func.count())
                        .select_from(ReviewItem)
                        .where(
                            ReviewItem.job_id == job.id,
                            ReviewItem.status == ReviewItemStatus.PENDING,
                        )
                    ).one()

                    # Create new snapshot
                    new_snapshot = _create_snapshot(job, review_count, pending_count)

                    # Detect changes
                    old_snapshot = job_snapshots.get(job.id)
                    event_type = _detect_event_type(old_snapshot, new_snapshot)

                    if event_type:
                        # Send update
                        job_response = _job_to_response(job, session)
                        message = JobWebSocketMessage(
                            event=event_type,
                            timestamp=datetime.now(timezone.utc),
                            job=job_response,
                        )
                        if not await _send_message(websocket, message):
                            return

                        logger.debug(
                            f"Sent {event_type.value} for job {job.id} to user {user.id}"
                        )

                    # Update snapshot
                    job_snapshots[job.id] = new_snapshot

                # Clean up old snapshots for jobs no longer in query
                for job_id in list(job_snapshots.keys()):
                    if job_id not in current_job_ids:
                        del job_snapshots[job_id]

                # Wait before next poll
                await asyncio.sleep(POLL_INTERVAL_SECONDS)

            except Exception as e:
                logger.error(f"Error in WebSocket polling loop: {e}")
                # Rollback any invalid transaction state before continuing
                try:
                    session.rollback()
                except Exception:
                    pass  # Ignore rollback errors
                # Continue polling despite errors
                await asyncio.sleep(POLL_INTERVAL_SECONDS)

    except WebSocketDisconnect:
        logger.info(f"WebSocket disconnected for user {user.id if user else 'unknown'}")

    finally:
        session.close()

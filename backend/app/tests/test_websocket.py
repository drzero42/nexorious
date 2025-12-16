"""
Tests for WebSocket endpoint for real-time job updates.

Note: WebSocket integration tests use a session factory override to share
the test session with the WebSocket handler, enabling transactional tests.
"""

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session
from datetime import datetime, timezone
from unittest.mock import patch

from ..models.user import User
from ..models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    ReviewItem,
    ReviewItemStatus,
)
from ..core.security import create_access_token, hash_token
from ..schemas.websocket import WebSocketEventType
from ..api.websocket import set_session_factory


@pytest.fixture
def ws_session(session: Session):
    """Configure the WebSocket to use the test session."""
    # Set the session factory to return the test session
    set_session_factory(lambda: session)
    yield session
    # Clean up - reset to default
    set_session_factory(None)


@pytest.fixture
def ws_test_user(ws_session: Session) -> User:
    """Create a test user for WebSocket tests."""
    user = User(
        username="ws_testuser",
        password_hash="$2b$12$test_hash",
        is_active=True,
        is_admin=False,
    )
    ws_session.add(user)
    ws_session.flush()  # Flush to get the ID without committing
    return user


@pytest.fixture
def ws_auth_token(ws_test_user: User, ws_session: Session) -> str:
    """Create an authentication token with session for WebSocket tests."""
    from ..models.user import UserSession
    from datetime import timedelta
    import uuid

    token = create_access_token(data={"sub": ws_test_user.id})
    session_record = UserSession(
        id=str(uuid.uuid4()),
        user_id=ws_test_user.id,
        token_hash=hash_token(token),
        refresh_token_hash=hash_token("test_refresh"),
        expires_at=datetime.now(timezone.utc) + timedelta(days=30),
        user_agent="test",
        ip_address="127.0.0.1",
    )
    ws_session.add(session_record)
    ws_session.flush()  # Flush to make visible within the transaction
    return token


class TestWebSocketAuthentication:
    """Tests for WebSocket authentication."""

    def test_connect_with_valid_token(
        self, client: TestClient, ws_test_user: User, ws_auth_token: str
    ):
        """Test successful WebSocket connection with valid token."""
        with client.websocket_connect(f"/api/ws/jobs?token={ws_auth_token}") as websocket:
            # Should receive connected message
            data = websocket.receive_json()
            assert data["event"] == "connected"
            assert data["user_id"] == ws_test_user.id
            assert "timestamp" in data

    def test_connect_with_invalid_token(self, client: TestClient):
        """Test WebSocket connection rejection with invalid token."""
        with client.websocket_connect("/api/ws/jobs?token=invalid_token") as websocket:
            # Should receive error message
            data = websocket.receive_json()
            assert data["event"] == "error"
            assert "Invalid or expired token" in data["message"]

    def test_connect_with_expired_token(
        self, client: TestClient, ws_test_user: User
    ):
        """Test WebSocket connection rejection with expired token."""
        from datetime import timedelta

        # Create an expired token
        token = create_access_token(
            data={"sub": ws_test_user.id}, expires_delta=timedelta(seconds=-1)
        )

        with client.websocket_connect(f"/api/ws/jobs?token={token}") as websocket:
            data = websocket.receive_json()
            assert data["event"] == "error"

    def test_connect_without_token(self, client: TestClient):
        """Test WebSocket connection rejection without token."""
        # FastAPI should reject the request due to missing required query param
        with pytest.raises(Exception):
            with client.websocket_connect("/api/ws/jobs") as websocket:
                pass

    def test_connect_with_inactive_user(self, client: TestClient, ws_session: Session):
        """Test WebSocket connection rejection for inactive user."""
        from ..models.user import UserSession
        from datetime import timedelta
        import uuid

        # Create inactive user
        inactive_user = User(
            username="inactive_user",
            password_hash="$2b$12$test_hash",
            is_active=False,
            is_admin=False,
        )
        ws_session.add(inactive_user)
        ws_session.flush()

        # Create token and session for inactive user
        token = create_access_token(data={"sub": inactive_user.id})
        session_record = UserSession(
            id=str(uuid.uuid4()),
            user_id=inactive_user.id,
            token_hash=hash_token(token),
            refresh_token_hash=hash_token("test_refresh"),
            expires_at=datetime.now(timezone.utc) + timedelta(days=30),
            user_agent="test",
            ip_address="127.0.0.1",
        )
        ws_session.add(session_record)
        ws_session.flush()

        with client.websocket_connect(f"/api/ws/jobs?token={token}") as websocket:
            data = websocket.receive_json()
            assert data["event"] == "error"


class TestWebSocketJobEvents:
    """Tests for WebSocket job event streaming."""

    def test_receive_job_created_event(
        self,
        client: TestClient,
        ws_test_user: User,
        ws_session: Session,
        ws_auth_token: str,
    ):
        """Test receiving job_created event when a new job is added."""
        # Patch the poll interval to be faster for tests
        with patch("app.api.websocket.POLL_INTERVAL_SECONDS", 0.1):
            with client.websocket_connect(
                f"/api/ws/jobs?token={ws_auth_token}"
            ) as websocket:
                # Receive connected message
                data = websocket.receive_json()
                assert data["event"] == "connected"

                # Create a job for the user
                job = Job(
                    user_id=ws_test_user.id,
                    job_type=BackgroundJobType.IMPORT,
                    source=BackgroundJobSource.STEAM,
                    status=BackgroundJobStatus.PENDING,
                    priority=BackgroundJobPriority.HIGH,
                    progress_total=100,
                )
                ws_session.add(job)
                ws_session.flush()

                # Wait for job_created event
                data = websocket.receive_json()
                assert data["event"] == "job_created"
                assert data["job"]["id"] == job.id
                assert data["job"]["status"] == "pending"

    def test_receive_job_progress_event(
        self,
        client: TestClient,
        ws_test_user: User,
        ws_session: Session,
        ws_auth_token: str,
    ):
        """Test receiving job_progress event when progress updates."""
        # Create a job first
        job = Job(
            user_id=ws_test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
            progress_current=0,
            progress_total=100,
        )
        ws_session.add(job)
        ws_session.flush()

        with patch("app.api.websocket.POLL_INTERVAL_SECONDS", 0.1):
            with client.websocket_connect(
                f"/api/ws/jobs?token={ws_auth_token}"
            ) as websocket:
                # Receive connected message
                data = websocket.receive_json()
                assert data["event"] == "connected"

                # Receive initial job_created event
                data = websocket.receive_json()
                assert data["event"] == "job_created"

                # Update job progress
                job.progress_current = 50
                ws_session.add(job)
                ws_session.flush()

                # Wait for job_progress event
                data = websocket.receive_json()
                assert data["event"] == "job_progress"
                assert data["job"]["progress_current"] == 50
                assert data["job"]["progress_percent"] == 50

    def test_receive_job_completed_event(
        self,
        client: TestClient,
        ws_test_user: User,
        ws_session: Session,
        ws_auth_token: str,
    ):
        """Test receiving job_completed event when job finishes."""
        # Create a processing job
        job = Job(
            user_id=ws_test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
            progress_current=90,
            progress_total=100,
        )
        ws_session.add(job)
        ws_session.flush()

        with patch("app.api.websocket.POLL_INTERVAL_SECONDS", 0.1):
            with client.websocket_connect(
                f"/api/ws/jobs?token={ws_auth_token}"
            ) as websocket:
                # Receive connected message
                data = websocket.receive_json()
                assert data["event"] == "connected"

                # Receive initial job_created event
                data = websocket.receive_json()
                assert data["event"] == "job_created"

                # Complete the job
                job.status = BackgroundJobStatus.COMPLETED
                job.progress_current = 100
                job.completed_at = datetime.now(timezone.utc)
                ws_session.add(job)
                ws_session.flush()

                # Wait for job_completed event
                data = websocket.receive_json()
                assert data["event"] == "job_completed"
                assert data["job"]["status"] == "completed"
                assert data["job"]["is_terminal"] is True

    def test_receive_job_failed_event(
        self,
        client: TestClient,
        ws_test_user: User,
        ws_session: Session,
        ws_auth_token: str,
    ):
        """Test receiving job_failed event when job fails."""
        # Create a processing job
        job = Job(
            user_id=ws_test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.LOW,
        )
        ws_session.add(job)
        ws_session.flush()

        with patch("app.api.websocket.POLL_INTERVAL_SECONDS", 0.1):
            with client.websocket_connect(
                f"/api/ws/jobs?token={ws_auth_token}"
            ) as websocket:
                # Receive connected and job_created
                websocket.receive_json()  # connected
                websocket.receive_json()  # job_created

                # Fail the job
                job.status = BackgroundJobStatus.FAILED
                job.error_message = "Connection timeout"
                job.completed_at = datetime.now(timezone.utc)
                ws_session.add(job)
                ws_session.flush()

                # Wait for job_failed event
                data = websocket.receive_json()
                assert data["event"] == "job_failed"
                assert data["job"]["status"] == "failed"
                assert data["job"]["error_message"] == "Connection timeout"

    def test_receive_review_item_update_event(
        self,
        client: TestClient,
        ws_test_user: User,
        ws_session: Session,
        ws_auth_token: str,
    ):
        """Test receiving review_item_update event when review counts change."""
        # Create a job in awaiting_review state
        job = Job(
            user_id=ws_test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.AWAITING_REVIEW,
            priority=BackgroundJobPriority.HIGH,
        )
        ws_session.add(job)
        ws_session.flush()

        with patch("app.api.websocket.POLL_INTERVAL_SECONDS", 0.1):
            with client.websocket_connect(
                f"/api/ws/jobs?token={ws_auth_token}"
            ) as websocket:
                # Receive connected and job_created
                websocket.receive_json()  # connected
                websocket.receive_json()  # job_created

                # Add a review item
                review_item = ReviewItem(
                    job_id=job.id,
                    user_id=ws_test_user.id,
                    status=ReviewItemStatus.PENDING,
                    source_title="Test Game",
                )
                ws_session.add(review_item)
                ws_session.flush()

                # Wait for review_item_update event
                data = websocket.receive_json()
                assert data["event"] == "review_item_update"
                assert data["job"]["review_item_count"] == 1
                assert data["job"]["pending_review_count"] == 1

    def test_no_events_for_other_users_jobs(
        self,
        client: TestClient,
        ws_test_user: User,
        ws_session: Session,
        ws_auth_token: str,
    ):
        """Test that we don't receive events for other users' jobs."""
        # Create another user
        other_user = User(
            username="other_user",
            password_hash="$2b$12$test_hash",
            is_active=True,
            is_admin=False,
        )
        ws_session.add(other_user)
        ws_session.flush()

        # Create a job for the other user
        other_job = Job(
            user_id=other_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
        )
        ws_session.add(other_job)
        ws_session.flush()

        with patch("app.api.websocket.POLL_INTERVAL_SECONDS", 0.1):
            with client.websocket_connect(
                f"/api/ws/jobs?token={ws_auth_token}"
            ) as websocket:
                # Receive connected message
                data = websocket.receive_json()
                assert data["event"] == "connected"

                # Create a job for our user to verify we get events
                my_job = Job(
                    user_id=ws_test_user.id,
                    job_type=BackgroundJobType.EXPORT,
                    source=BackgroundJobSource.NEXORIOUS,
                    status=BackgroundJobStatus.PENDING,
                    priority=BackgroundJobPriority.HIGH,
                )
                ws_session.add(my_job)
                ws_session.flush()

                # Should receive event for our job
                data = websocket.receive_json()
                assert data["event"] == "job_created"
                assert data["job"]["id"] == my_job.id
                # Should NOT be the other user's job
                assert data["job"]["id"] != other_job.id


class TestWebSocketChangeDetection:
    """Tests for the change detection logic."""

    def test_detect_status_change(self):
        """Test that status changes are properly detected."""
        from ..api.websocket import _detect_event_type, JobSnapshot

        old = JobSnapshot(
            status="processing",
            progress_current=50,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )
        new = JobSnapshot(
            status="awaiting_review",
            progress_current=50,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )

        event = _detect_event_type(old, new)
        assert event == WebSocketEventType.JOB_STATUS_CHANGE

    def test_detect_completion(self):
        """Test that completion is detected as job_completed."""
        from ..api.websocket import _detect_event_type, JobSnapshot

        old = JobSnapshot(
            status="processing",
            progress_current=99,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )
        new = JobSnapshot(
            status="completed",
            progress_current=100,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )

        event = _detect_event_type(old, new)
        assert event == WebSocketEventType.JOB_COMPLETED

    def test_detect_failure(self):
        """Test that failure is detected as job_failed."""
        from ..api.websocket import _detect_event_type, JobSnapshot

        old = JobSnapshot(
            status="processing",
            progress_current=50,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )
        new = JobSnapshot(
            status="failed",
            progress_current=50,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )

        event = _detect_event_type(old, new)
        assert event == WebSocketEventType.JOB_FAILED

    def test_detect_progress_change(self):
        """Test that progress changes are detected."""
        from ..api.websocket import _detect_event_type, JobSnapshot

        old = JobSnapshot(
            status="processing",
            progress_current=50,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )
        new = JobSnapshot(
            status="processing",
            progress_current=75,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )

        event = _detect_event_type(old, new)
        assert event == WebSocketEventType.JOB_PROGRESS

    def test_detect_review_item_change(self):
        """Test that review item count changes are detected."""
        from ..api.websocket import _detect_event_type, JobSnapshot

        old = JobSnapshot(
            status="awaiting_review",
            progress_current=100,
            progress_total=100,
            review_item_count=5,
            pending_review_count=5,
        )
        new = JobSnapshot(
            status="awaiting_review",
            progress_current=100,
            progress_total=100,
            review_item_count=5,
            pending_review_count=4,
        )

        event = _detect_event_type(old, new)
        assert event == WebSocketEventType.REVIEW_ITEM_UPDATE

    def test_detect_new_job(self):
        """Test that new jobs are detected as job_created."""
        from ..api.websocket import _detect_event_type, JobSnapshot

        new = JobSnapshot(
            status="pending",
            progress_current=0,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )

        event = _detect_event_type(None, new)
        assert event == WebSocketEventType.JOB_CREATED

    def test_no_change_detected(self):
        """Test that no event is returned when nothing changed."""
        from ..api.websocket import _detect_event_type, JobSnapshot

        snapshot = JobSnapshot(
            status="processing",
            progress_current=50,
            progress_total=100,
            review_item_count=0,
            pending_review_count=0,
        )

        event = _detect_event_type(snapshot, snapshot)
        assert event is None


class TestJobDurationSeconds:
    """Tests for Job.duration_seconds property with mixed timezone datetimes."""

    def test_duration_seconds_with_naive_started_at_no_completed_at(self):
        """Test duration_seconds when started_at is naive and job is still running.

        The duration_seconds property calculates: (completed_at or now()) - started_at
        If started_at is naive (from DB) and we use timezone-aware now(), this fails with:
        'can't subtract offset-naive and offset-aware datetime'

        This reproduces the production error seen in WebSocket polling.
        """
        # Create a job with naive started_at in UTC (simulating what PostgreSQL returns)
        # Use utcnow() to get a naive datetime that represents UTC time
        job = Job(
            user_id="test-user",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
            progress_current=50,
            progress_total=100,
            started_at=datetime.now(timezone.utc).replace(tzinfo=None),  # Naive UTC datetime
        )

        # This should NOT raise "can't subtract offset-naive and offset-aware datetime"
        duration = job.duration_seconds
        assert duration is not None
        # Duration should be small (just created) - allow some tolerance
        assert -1 <= duration <= 10

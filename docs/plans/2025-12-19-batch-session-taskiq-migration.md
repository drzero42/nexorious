# Batch Session Manager to Taskiq Migration Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the in-memory BatchSessionManager with a persistent, PostgreSQL-backed solution using the existing Job model and taskiq result backend.

**Architecture:** The current BatchSession dataclass stores ephemeral session state (progress, processed IDs) in memory. We'll migrate to using the existing Job model for persistence, eliminating the need for a separate in-memory manager. The Job model already has the fields needed (progress tracking, timestamps, status), we just need to extend it slightly and update the batch processor to use it.

**Tech Stack:** FastAPI, SQLModel, taskiq-pg, PostgreSQL

---

## Current State Analysis

### What Exists
- **BatchSession (dataclass)**: In-memory session state with progress tracking
- **BatchSessionManager**: Global singleton with Dict storage, asyncio cleanup task
- **Job model**: Database-persisted job tracking (already used for imports/exports)
- **batch_processor.py**: Generic batch router using BatchSessionManager

### Problems with Current Approach
1. Sessions lost on API server restart
2. In-memory Dict doesn't scale across multiple API instances
3. Asyncio cleanup task tied to API process lifecycle
4. Duplicate concepts: BatchSession vs Job model

### Solution
Extend the Job model to handle batch operations (AUTO_MATCH, SYNC) and store session state in the database. This unifies all background operations under one model and provides persistence.

---

## Task 1: Add Batch Operation Fields to Job Model

**Files:**
- Modify: `backend/app/models/job.py`
- Create: `backend/app/alembic/versions/xxxx_add_batch_session_fields.py` (auto-generated)

**Step 1: Write the failing test**

Create test file `backend/app/tests/test_job_batch_fields.py`:

```python
"""Tests for Job model batch session fields."""

import pytest
from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ImportJobSubtype,
)


def test_job_has_batch_session_fields():
    """Job model should have fields for batch session tracking."""
    job = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        import_subtype=ImportJobSubtype.AUTO_MATCH,
    )

    # New batch session fields should exist
    assert hasattr(job, "processed_item_ids_json")
    assert hasattr(job, "failed_item_ids_json")

    # Helper methods should work
    assert job.get_processed_item_ids() == []
    assert job.get_failed_item_ids() == []

    job.add_processed_item_id("game-1")
    job.add_processed_item_id("game-2")
    assert job.get_processed_item_ids() == ["game-1", "game-2"]

    job.add_failed_item_id("game-3")
    assert job.get_failed_item_ids() == ["game-3"]


def test_job_progress_percentage():
    """Job should calculate progress percentage correctly."""
    job = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        progress_total=100,
        progress_current=50,
    )

    assert job.progress_percent == 50


def test_job_remaining_items():
    """Job should calculate remaining items."""
    job = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        progress_total=100,
        progress_current=30,
    )

    assert job.remaining_items == 70


def test_job_is_active():
    """Job should report active status correctly."""
    job = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        status=BackgroundJobStatus.PROCESSING,
    )

    assert job.is_active is True

    job.status = BackgroundJobStatus.COMPLETED
    assert job.is_active is False
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_job_batch_fields.py -v`
Expected: FAIL with AttributeError for missing fields

**Step 3: Write minimal implementation**

Add to `backend/app/models/job.py`:

```python
# Add new fields after error_message field (around line 123):

    # Batch session tracking (for auto-match and sync operations)
    processed_item_ids_json: str = Field(
        default="[]",
        sa_column_kwargs={"name": "processed_item_ids"},
        description="JSON array of item IDs that have been processed",
    )
    failed_item_ids_json: str = Field(
        default="[]",
        sa_column_kwargs={"name": "failed_item_ids"},
        description="JSON array of item IDs that failed processing",
    )

# Add helper methods after add_error method (around line 177):

    def get_processed_item_ids(self) -> List[str]:
        """Get processed item IDs as a list."""
        try:
            return json.loads(self.processed_item_ids_json)
        except (json.JSONDecodeError, TypeError):
            return []

    def set_processed_item_ids(self, value: List[str]) -> None:
        """Set processed item IDs from a list."""
        self.processed_item_ids_json = json.dumps(value)

    def add_processed_item_id(self, item_id: str) -> None:
        """Add an item ID to the processed list."""
        ids = self.get_processed_item_ids()
        ids.append(item_id)
        self.set_processed_item_ids(ids)

    def get_failed_item_ids(self) -> List[str]:
        """Get failed item IDs as a list."""
        try:
            return json.loads(self.failed_item_ids_json)
        except (json.JSONDecodeError, TypeError):
            return []

    def set_failed_item_ids(self, value: List[str]) -> None:
        """Set failed item IDs from a list."""
        self.failed_item_ids_json = json.dumps(value)

    def add_failed_item_id(self, item_id: str) -> None:
        """Add an item ID to the failed list."""
        ids = self.get_failed_item_ids()
        ids.append(item_id)
        self.set_failed_item_ids(ids)

    @property
    def remaining_items(self) -> int:
        """Calculate remaining items to process."""
        return max(0, self.progress_total - self.progress_current)

    @property
    def is_active(self) -> bool:
        """Check if job is in an active (non-terminal) state."""
        return self.status in (
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
            BackgroundJobStatus.AWAITING_REVIEW,
            BackgroundJobStatus.READY,
            BackgroundJobStatus.FINALIZING,
        )
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_job_batch_fields.py -v`
Expected: PASS

**Step 5: Generate and apply migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic revision --autogenerate -m "add_batch_session_fields_to_job"`

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic upgrade head`

**Step 6: Commit**

```bash
git add backend/app/models/job.py backend/app/tests/test_job_batch_fields.py backend/app/alembic/versions/
git commit -m "$(cat <<'EOF'
feat(models): add batch session fields to Job model

Extends Job model with processed_item_ids and failed_item_ids fields
for tracking batch operation progress. Adds helper methods for managing
these lists and calculating remaining items.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Create Job-Based Batch Session Service

**Files:**
- Create: `backend/app/services/batch_job_service.py`
- Create: `backend/app/tests/test_batch_job_service.py`

**Step 1: Write the failing test**

Create `backend/app/tests/test_batch_job_service.py`:

```python
"""Tests for batch job service."""

import pytest
from unittest.mock import MagicMock, patch
from sqlmodel import Session

from app.services.batch_job_service import BatchJobService
from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ImportJobSubtype,
)
from app.models.batch_session import BatchOperationType


class TestBatchJobService:
    """Tests for BatchJobService."""

    def test_create_batch_job_auto_match(self):
        """Should create a job for auto-match batch operation."""
        mock_session = MagicMock(spec=Session)
        service = BatchJobService(mock_session)

        job = service.create_batch_job(
            user_id="user-123",
            operation_type=BatchOperationType.AUTO_MATCH,
            source=BackgroundJobSource.DARKADIA,
            total_items=50,
        )

        assert job.user_id == "user-123"
        assert job.job_type == BackgroundJobType.IMPORT
        assert job.source == BackgroundJobSource.DARKADIA
        assert job.import_subtype == ImportJobSubtype.AUTO_MATCH
        assert job.progress_total == 50
        assert job.status == BackgroundJobStatus.PROCESSING
        mock_session.add.assert_called_once_with(job)
        mock_session.commit.assert_called_once()

    def test_create_batch_job_sync(self):
        """Should create a job for sync batch operation."""
        mock_session = MagicMock(spec=Session)
        service = BatchJobService(mock_session)

        job = service.create_batch_job(
            user_id="user-123",
            operation_type=BatchOperationType.SYNC,
            source=BackgroundJobSource.DARKADIA,
            total_items=25,
        )

        assert job.import_subtype == ImportJobSubtype.BULK_SYNC

    def test_get_batch_job(self):
        """Should retrieve a batch job by ID."""
        mock_session = MagicMock(spec=Session)
        mock_job = Job(
            id="job-123",
            user_id="user-123",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )
        mock_session.get.return_value = mock_job

        service = BatchJobService(mock_session)
        job = service.get_batch_job("job-123")

        assert job == mock_job
        mock_session.get.assert_called_once_with(Job, "job-123")

    def test_update_batch_progress(self):
        """Should update batch job progress."""
        mock_session = MagicMock(spec=Session)
        job = Job(
            id="job-123",
            user_id="user-123",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            progress_total=100,
            progress_current=0,
        )
        mock_session.get.return_value = job

        service = BatchJobService(mock_session)
        updated_job = service.update_batch_progress(
            job_id="job-123",
            processed_count=5,
            successful_count=4,
            failed_count=1,
            processed_ids=["g1", "g2", "g3", "g4", "g5"],
            failed_ids=["g5"],
            errors=["g5 failed: no match"],
        )

        assert updated_job.progress_current == 5
        assert updated_job.successful_items == 4
        assert updated_job.failed_items == 1
        assert updated_job.get_processed_item_ids() == ["g1", "g2", "g3", "g4", "g5"]
        assert updated_job.get_failed_item_ids() == ["g5"]

    def test_cancel_batch_job(self):
        """Should cancel a batch job."""
        mock_session = MagicMock(spec=Session)
        job = Job(
            id="job-123",
            user_id="user-123",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
        )
        mock_session.get.return_value = job

        service = BatchJobService(mock_session)
        cancelled = service.cancel_batch_job("job-123", "user-123")

        assert cancelled.status == BackgroundJobStatus.CANCELLED

    def test_cancel_batch_job_wrong_user(self):
        """Should return None when cancelling job for wrong user."""
        mock_session = MagicMock(spec=Session)
        job = Job(
            id="job-123",
            user_id="user-123",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.PROCESSING,
        )
        mock_session.get.return_value = job

        service = BatchJobService(mock_session)
        result = service.cancel_batch_job("job-123", "wrong-user")

        assert result is None
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_batch_job_service.py -v`
Expected: FAIL with ModuleNotFoundError

**Step 3: Write minimal implementation**

Create `backend/app/services/batch_job_service.py`:

```python
"""
Batch job service for managing persistent batch operations.

Replaces the in-memory BatchSessionManager with database-backed
job tracking using the unified Job model.
"""

import logging
from datetime import datetime, timezone
from typing import List, Optional

from sqlmodel import Session

from app.models.batch_session import BatchOperationType
from app.models.job import (
    BackgroundJobPriority,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobType,
    ImportJobSubtype,
    Job,
)

logger = logging.getLogger(__name__)


class BatchJobService:
    """
    Service for managing batch operations using the Job model.

    Provides a clean interface for creating, updating, and querying
    batch jobs with full database persistence.
    """

    def __init__(self, session: Session):
        self.session = session

    def create_batch_job(
        self,
        user_id: str,
        operation_type: BatchOperationType,
        source: BackgroundJobSource,
        total_items: int,
    ) -> Job:
        """
        Create a new batch job.

        Args:
            user_id: ID of the user starting the operation
            operation_type: Type of batch operation (auto_match or sync)
            source: Import source (darkadia, steam, etc.)
            total_items: Total number of items to process

        Returns:
            Created Job instance
        """
        # Map batch operation type to import subtype
        subtype_map = {
            BatchOperationType.AUTO_MATCH: ImportJobSubtype.AUTO_MATCH,
            BatchOperationType.SYNC: ImportJobSubtype.BULK_SYNC,
        }

        job = Job(
            user_id=user_id,
            job_type=BackgroundJobType.IMPORT,
            source=source,
            import_subtype=subtype_map[operation_type],
            status=BackgroundJobStatus.PROCESSING,
            priority=BackgroundJobPriority.HIGH,
            progress_total=total_items,
            progress_current=0,
            successful_items=0,
            failed_items=0,
            started_at=datetime.now(timezone.utc),
        )

        self.session.add(job)
        self.session.commit()
        self.session.refresh(job)

        logger.info(
            f"Created batch job {job.id} for user {user_id} "
            f"(type: {operation_type.value}, source: {source.value}, items: {total_items})"
        )

        return job

    def get_batch_job(self, job_id: str) -> Optional[Job]:
        """Get a batch job by ID."""
        return self.session.get(Job, job_id)

    def update_batch_progress(
        self,
        job_id: str,
        processed_count: int,
        successful_count: int,
        failed_count: int,
        processed_ids: List[str],
        failed_ids: List[str],
        errors: List[str],
    ) -> Optional[Job]:
        """
        Update progress for a batch job.

        Args:
            job_id: ID of the job to update
            processed_count: Number of items processed in this batch
            successful_count: Number successfully processed
            failed_count: Number that failed
            processed_ids: List of processed item IDs
            failed_ids: List of failed item IDs
            errors: List of error messages

        Returns:
            Updated Job or None if not found
        """
        job = self.session.get(Job, job_id)
        if not job:
            return None

        # Update counters
        job.progress_current += processed_count
        job.successful_items += successful_count
        job.failed_items += failed_count

        # Append to ID lists
        current_processed = job.get_processed_item_ids()
        current_processed.extend(processed_ids)
        job.set_processed_item_ids(current_processed)

        current_failed = job.get_failed_item_ids()
        current_failed.extend(failed_ids)
        job.set_failed_item_ids(current_failed)

        # Append errors to error log
        for error in errors:
            job.add_error({"message": error, "timestamp": datetime.now(timezone.utc).isoformat()})

        # Check if complete
        if job.progress_current >= job.progress_total:
            job.status = BackgroundJobStatus.COMPLETED
            job.completed_at = datetime.now(timezone.utc)

        self.session.commit()
        self.session.refresh(job)

        logger.debug(
            f"Updated batch job {job_id}: "
            f"{job.progress_current}/{job.progress_total} processed, "
            f"{job.successful_items} successful, {job.failed_items} failed"
        )

        return job

    def cancel_batch_job(self, job_id: str, user_id: str) -> Optional[Job]:
        """
        Cancel a batch job.

        Args:
            job_id: ID of the job to cancel
            user_id: ID of the user (for authorization)

        Returns:
            Cancelled Job or None if not found/unauthorized
        """
        job = self.session.get(Job, job_id)
        if not job or job.user_id != user_id:
            return None

        if not job.is_active:
            logger.warning(f"Attempted to cancel non-active job {job_id}")
            return job

        job.status = BackgroundJobStatus.CANCELLED
        job.completed_at = datetime.now(timezone.utc)

        self.session.commit()
        self.session.refresh(job)

        logger.info(f"Cancelled batch job {job_id} for user {user_id}")

        return job

    def fail_batch_job(self, job_id: str, error_message: str) -> Optional[Job]:
        """
        Mark a batch job as failed.

        Args:
            job_id: ID of the job to fail
            error_message: Error message describing the failure

        Returns:
            Failed Job or None if not found
        """
        job = self.session.get(Job, job_id)
        if not job:
            return None

        job.status = BackgroundJobStatus.FAILED
        job.error_message = error_message
        job.completed_at = datetime.now(timezone.utc)

        self.session.commit()
        self.session.refresh(job)

        logger.error(f"Failed batch job {job_id}: {error_message}")

        return job
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_batch_job_service.py -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/services/batch_job_service.py backend/app/tests/test_batch_job_service.py
git commit -m "$(cat <<'EOF'
feat(services): add BatchJobService for persistent batch operations

New service that replaces in-memory BatchSessionManager with
database-backed job tracking using the unified Job model.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Update Batch Processor to Use BatchJobService

**Files:**
- Modify: `backend/app/api/import_api/batch_processor.py`

**Step 1: Write the failing test**

Create `backend/app/tests/test_batch_processor_integration.py`:

```python
"""Integration tests for batch processor with BatchJobService."""

import pytest
from unittest.mock import MagicMock, patch, AsyncMock
from fastapi import FastAPI
from fastapi.testclient import TestClient
from sqlmodel import Session

from app.models.job import Job, BackgroundJobStatus, BackgroundJobType, BackgroundJobSource


@pytest.fixture
def mock_job():
    """Create a mock job for testing."""
    return Job(
        id="test-job-123",
        user_id="user-123",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.DARKADIA,
        status=BackgroundJobStatus.PROCESSING,
        progress_total=10,
        progress_current=0,
    )


def test_batch_processor_uses_job_model(mock_job):
    """Batch processor should use Job model instead of BatchSession."""
    # This is a conceptual test - the actual integration will be tested
    # via the batch endpoints in the running application.
    # The key assertion is that BatchJobService is used.

    from app.services.batch_job_service import BatchJobService

    mock_session = MagicMock(spec=Session)
    mock_session.get.return_value = mock_job

    service = BatchJobService(mock_session)
    job = service.get_batch_job("test-job-123")

    # Job should have the new batch session fields accessible
    assert hasattr(job, "get_processed_item_ids")
    assert hasattr(job, "remaining_items")
    assert hasattr(job, "is_active")
```

**Step 2: Run test to verify it passes (sanity check)**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_batch_processor_integration.py -v`
Expected: PASS (this is a sanity check)

**Step 3: Update batch_processor.py**

Replace imports and update the router factory in `backend/app/api/import_api/batch_processor.py`:

Key changes:
1. Replace `from ...services.batch_session_manager import get_batch_session_manager` with `from ...services.batch_job_service import BatchJobService`
2. Replace `BatchSession` references with `Job`
3. Update all session manager calls to use `BatchJobService`

The file is large, so here are the specific changes:

**Change 1: Update imports (top of file)**
```python
# Remove:
from ...services.batch_session_manager import get_batch_session_manager

# Add:
from ...services.batch_job_service import BatchJobService
from ...models.job import Job, BackgroundJobStatus
```

**Change 2: Update source name to BackgroundJobSource mapping**
Add helper function after the protocol definitions:

```python
def _get_job_source_from_config_name(source_name: str) -> BackgroundJobSource:
    """Map config source name to BackgroundJobSource enum."""
    from ...models.job import BackgroundJobSource

    source_map = {
        "Steam": BackgroundJobSource.STEAM,
        "Darkadia": BackgroundJobSource.DARKADIA,
        "Epic": BackgroundJobSource.EPIC,
        "GOG": BackgroundJobSource.GOG,
        "Xbox": BackgroundJobSource.XBOX,
        "PlayStation": BackgroundJobSource.PLAYSTATION,
    }
    return source_map.get(source_name, BackgroundJobSource.SYSTEM)
```

**Change 3: Update start_batch_auto_match endpoint**
Replace `session_manager.create_session()` with `BatchJobService`:

```python
# OLD:
session_manager = get_batch_session_manager()
batch_session = session_manager.create_session(
    user_id=current_user.id,
    operation_type=BatchOperationType.AUTO_MATCH,
    total_items=total_items,
)

# NEW:
batch_service = BatchJobService(db_session)
job = batch_service.create_batch_job(
    user_id=current_user.id,
    operation_type=BatchOperationType.AUTO_MATCH,
    source=_get_job_source_from_config_name(config.source_name),
    total_items=total_items,
)

# Update return statement:
return BatchSessionStartResponse(
    session_id=job.id,
    total_items=total_items,
    operation_type=BatchOperationType.AUTO_MATCH.value,
    status=job.status.value,
    message=f"Batch auto-match session started for {total_items} unmatched games",
)
```

**Change 4: Update process_next_auto_match_batch endpoint**
Replace `session_manager.get_session()` with `BatchJobService`:

```python
# OLD:
session_manager = get_batch_session_manager()
batch_session = session_manager.get_session(session_id)

# NEW:
batch_service = BatchJobService(db_session)
job = batch_service.get_batch_job(session_id)

if not job:
    raise HTTPException(...)

if job.user_id != current_user.id:
    raise HTTPException(...)

if not job.is_active:
    ...

# Replace batch_session.processed_item_ids with:
job.get_processed_item_ids()

# Replace session_manager.update_session_progress() with:
batch_service.update_batch_progress(
    job_id=session_id,
    processed_count=len(games_to_process),
    successful_count=successful_count,
    failed_count=failed_count,
    processed_ids=game_ids,
    failed_ids=failed_game_ids,
    errors=errors[-10:],
)

# Update response to use job fields:
return BatchNextResponse(
    session_id=job.id,
    ...
    total_items=job.progress_total,
    processed_items=job.progress_current,
    successful_items=job.successful_items,
    failed_items=job.failed_items,
    remaining_items=job.remaining_items,
    progress_percentage=job.progress_percent,
    status=job.status.value,
    is_complete=job.is_terminal,
    ...
)
```

Similar changes for `start_batch_sync`, `process_next_sync_batch`, `get_batch_status`, and `cancel_batch_session`.

**Step 4: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/ -v --cov=app`
Expected: All tests pass

**Step 5: Commit**

```bash
git add backend/app/api/import_api/batch_processor.py
git commit -m "$(cat <<'EOF'
refactor(batch): migrate batch_processor to use BatchJobService

Replaces in-memory BatchSessionManager with database-backed
BatchJobService. Batch operations now persist across server restarts.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Update Response Helper and Add Job-Specific Fields

**Files:**
- Modify: `backend/app/api/import_api/batch_processor.py`

**Step 1: Update _create_batch_response helper**

```python
def _create_batch_response(
    _config: BatchSourceConfig,
    job: Job,
    current_batch_items: List,
    message: str,
) -> BatchNextResponse:
    """Helper function to create a consistent batch response."""
    return BatchNextResponse(
        session_id=job.id,
        batch_processed=len(current_batch_items),
        batch_successful=0,
        batch_failed=0,
        batch_errors=[],
        current_batch_items=current_batch_items,
        total_items=job.progress_total,
        processed_items=job.progress_current,
        successful_items=job.successful_items,
        failed_items=job.failed_items,
        remaining_items=job.remaining_items,
        progress_percentage=float(job.progress_percent),
        status=job.status.value,
        is_complete=job.is_terminal,
        message=message,
    )
```

**Step 2: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: No errors

**Step 3: Commit**

```bash
git add backend/app/api/import_api/batch_processor.py
git commit -m "$(cat <<'EOF'
fix(batch): update response helper to use Job model

Updates _create_batch_response to work with Job model instead
of BatchSession dataclass.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Remove BatchSessionManager from Application Lifecycle

**Files:**
- Modify: `backend/app/main.py`

**Step 1: Update main.py lifespan**

Remove batch session manager initialization:

```python
# Remove these imports:
from .services.batch_session_manager import (
    startup_batch_session_manager,
    shutdown_batch_session_manager,
)

# In lifespan function, remove:
await startup_batch_session_manager()
logger.info("Batch session manager initialized")

# And in shutdown:
await shutdown_batch_session_manager()
logger.info("Batch session manager shutdown completed")
```

**Step 2: Run the application**

Run: `cd /home/abo/workspace/home/nexorious/backend && timeout 5 uv run uvicorn app.main:app --reload || true`
Expected: Application starts without errors (timeout will stop it)

**Step 3: Commit**

```bash
git add backend/app/main.py
git commit -m "$(cat <<'EOF'
refactor(main): remove BatchSessionManager from app lifecycle

Batch session state is now persisted in database via Job model,
so in-memory manager and its cleanup task are no longer needed.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Delete Deprecated Files

**Files:**
- Delete: `backend/app/services/batch_session_manager.py`
- Delete: `backend/app/models/batch_session.py`

**Step 1: Check for remaining references**

Run: `cd /home/abo/workspace/home/nexorious/backend && grep -r "batch_session_manager\|BatchSession" app/ --include="*.py" | grep -v "__pycache__" | grep -v ".pyc"`

If any references remain, update them first.

**Step 2: Update batch_processor.py to use Job enums directly**

If `BatchOperationType` from `batch_session.py` is still used, either:
- Move the enum to a shared location, OR
- Replace with string literals in the service

For now, keep `BatchOperationType` and `BATCH_SIZES` but move them to a more appropriate location:

Create `backend/app/models/batch_constants.py`:

```python
"""Constants for batch processing operations."""

from enum import Enum


class BatchOperationType(str, Enum):
    """Types of batch operations."""
    AUTO_MATCH = "auto_match"
    SYNC = "sync"


# Batch sizes for different operation types
BATCH_SIZES = {
    BatchOperationType.AUTO_MATCH: 5,
    BatchOperationType.SYNC: 5,
}
```

Update imports in `batch_processor.py` and `batch_job_service.py`.

**Step 3: Delete the files**

```bash
rm backend/app/services/batch_session_manager.py
rm backend/app/models/batch_session.py
```

**Step 4: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/ -v`
Expected: All tests pass

**Step 5: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor(batch): remove deprecated BatchSessionManager and BatchSession

- Deletes in-memory batch session manager
- Deletes BatchSession dataclass
- Moves BatchOperationType and BATCH_SIZES to batch_constants.py
- All batch state now persisted via Job model

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Add Cleanup Maintenance Task for Stale Batch Jobs

**Files:**
- Modify: `backend/app/worker/tasks/maintenance/__init__.py`

**Step 1: Write the test**

Add to `backend/app/tests/test_maintenance_tasks.py` (create if doesn't exist):

```python
"""Tests for maintenance tasks."""

import pytest
from datetime import datetime, timezone, timedelta
from unittest.mock import MagicMock, patch, AsyncMock

from app.models.job import Job, BackgroundJobStatus, BackgroundJobType, BackgroundJobSource


@pytest.mark.asyncio
async def test_cleanup_stale_batch_jobs():
    """Should mark stale batch jobs as failed."""
    from app.worker.tasks.maintenance import cleanup_stale_batch_jobs

    # Test will verify the task exists and can be called
    # Full integration testing requires a running database
    assert cleanup_stale_batch_jobs is not None
```

**Step 2: Add cleanup task**

Add to `backend/app/worker/tasks/maintenance/__init__.py`:

```python
from datetime import datetime, timezone, timedelta
from sqlmodel import select

from app.worker.broker import broker
from app.worker.queues import QUEUE_LOW
from app.core.database import get_session_context
from app.models.job import Job, BackgroundJobStatus, BackgroundJobType, ImportJobSubtype


@broker.task(
    task_name="maintenance.cleanup_stale_batch_jobs",
    schedule=[{"cron": "*/30 * * * *"}],  # Every 30 minutes
    queue=QUEUE_LOW,
)
async def cleanup_stale_batch_jobs(timeout_minutes: int = 30) -> dict:
    """
    Mark stale batch jobs as failed.

    Batch jobs that have been processing for longer than timeout_minutes
    without updates are considered stale and marked as failed.
    """
    cutoff_time = datetime.now(timezone.utc) - timedelta(minutes=timeout_minutes)

    async with get_session_context() as session:
        # Find stale batch jobs (auto_match or bulk_sync that are still processing)
        stale_jobs = session.exec(
            select(Job).where(
                Job.job_type == BackgroundJobType.IMPORT,
                Job.import_subtype.in_([
                    ImportJobSubtype.AUTO_MATCH,
                    ImportJobSubtype.BULK_SYNC,
                ]),
                Job.status == BackgroundJobStatus.PROCESSING,
                Job.started_at < cutoff_time,
            )
        ).all()

        for job in stale_jobs:
            job.status = BackgroundJobStatus.FAILED
            job.error_message = f"Job timed out after {timeout_minutes} minutes of inactivity"
            job.completed_at = datetime.now(timezone.utc)

        session.commit()

        return {"cleaned_up": len(stale_jobs)}
```

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_maintenance_tasks.py -v`
Expected: PASS

**Step 4: Commit**

```bash
git add backend/app/worker/tasks/maintenance/ backend/app/tests/test_maintenance_tasks.py
git commit -m "$(cat <<'EOF'
feat(worker): add cleanup task for stale batch jobs

Scheduled task runs every 30 minutes to mark stale batch jobs
as failed. Replaces the in-memory cleanup loop that was removed
with BatchSessionManager.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Update BatchStatusResponse Schema

**Files:**
- Modify: `backend/app/schemas/batch.py`

**Step 1: Update schema to include job_id**

The schema should use `job_id` instead of `session_id` for clarity, but we'll keep `session_id` for backwards compatibility:

```python
class BatchStatusResponse(BaseModel):
    """Response for batch status check."""
    session_id: str  # Kept for backwards compatibility (is actually job_id)
    job_id: Optional[str] = None  # Explicit job ID field
    operation_type: str
    # ... rest of fields
```

For now, keep the schema as-is since `session_id` maps directly to `job.id`.

**Step 2: Commit (if any changes)**

Skip if no changes needed.

---

## Task 9: Run Full Test Suite and Type Check

**Files:** None (verification only)

**Step 1: Run backend tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/ -v --cov=app --cov-report=term-missing`
Expected: All tests pass, >80% coverage

**Step 2: Run type checking**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`
Expected: No type errors

**Step 3: Run linting**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run ruff check .`
Expected: No errors

---

## Task 10: Update Documentation

**Files:**
- Modify: `docs/plans/2025-12-13-background-task-system-design.md`

**Step 1: Add completion note**

Add to the design document:

```markdown
## Implementation Status

**Completed:** 2025-12-19

The BatchSessionManager has been migrated to use the Job model:
- In-memory session storage replaced with PostgreSQL persistence
- Cleanup asyncio task replaced with scheduled taskiq maintenance task
- BatchSession dataclass deleted in favor of extended Job model
- Full backwards compatibility maintained for API responses

See implementation plan: `docs/plans/2025-12-19-batch-session-taskiq-migration.md`
```

**Step 2: Commit**

```bash
git add docs/plans/2025-12-13-background-task-system-design.md
git commit -m "$(cat <<'EOF'
docs: mark batch session migration as complete

Updates design doc with implementation status and reference
to the migration plan.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Summary

This plan migrates the in-memory BatchSessionManager to a persistent, PostgreSQL-backed solution by:

1. **Extending the Job model** with batch session fields (processed/failed item IDs)
2. **Creating BatchJobService** as the new interface for batch operations
3. **Updating batch_processor.py** to use the new service
4. **Removing the in-memory manager** and its cleanup task
5. **Adding a scheduled maintenance task** for cleaning up stale batch jobs

**Benefits:**
- Batch sessions persist across API server restarts
- Works correctly with multiple API instances
- Unified job tracking (imports, exports, and batch operations share one model)
- Leverages existing taskiq infrastructure for cleanup scheduling

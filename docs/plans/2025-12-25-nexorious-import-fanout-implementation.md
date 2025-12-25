# Nexorious Import Fan-Out Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable parallel processing of Nexorious JSON imports by fanning out individual game/wishlist imports to multiple workers.

**Architecture:** A coordinator task parses the JSON and creates child Job records for each game/wishlist item. Each child is processed independently, enabling parallel execution across workers. Parent job aggregates progress from children in real-time.

**Tech Stack:** Python/FastAPI backend, SQLModel ORM, Alembic migrations, taskiq-pg for task queue, React/TypeScript frontend with TanStack Query.

---

## Task 1: Add parent_job_id Field to Job Model

**Files:**
- Modify: `backend/app/models/job.py`

**Step 1: Write the test for parent-child relationship**

Create test in `backend/app/tests/test_job_model.py`:

```python
def test_job_parent_child_relationship(session: Session):
    """Test that jobs can have parent-child relationships."""
    from app.models.job import Job, BackgroundJobType, BackgroundJobSource

    # Create parent job
    parent = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
    )
    session.add(parent)
    session.commit()
    session.refresh(parent)

    # Create child job
    child = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        parent_job_id=parent.id,
    )
    session.add(child)
    session.commit()
    session.refresh(child)

    # Verify relationship
    assert child.parent_job_id == parent.id
    session.refresh(parent)
    assert len(parent.children) == 1
    assert parent.children[0].id == child.id
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/test_job_model.py::test_job_parent_child_relationship -v`
Expected: FAIL with "parent_job_id" or "children" not defined

**Step 3: Add parent_job_id field and relationships to Job model**

In `backend/app/models/job.py`, add after the `taskiq_task_id` field (around line 147):

```python
    # Parent-child relationship for fan-out tasks
    parent_job_id: Optional[str] = Field(
        default=None,
        foreign_key="jobs.id",
        index=True,
        description="ID of parent job for fan-out tasks",
    )
```

Add relationships after the existing relationships (around line 159):

```python
    # Parent-child relationships for fan-out tasks
    children: List["Job"] = Relationship(
        back_populates="parent",
        sa_relationship_kwargs={
            "cascade": "all, delete-orphan",
            "foreign_keys": "[Job.parent_job_id]",
        },
    )
    parent: Optional["Job"] = Relationship(
        back_populates="children",
        sa_relationship_kwargs={
            "remote_side": "[Job.id]",
            "foreign_keys": "[Job.parent_job_id]",
        },
    )
```

**Step 4: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/test_job_model.py::test_job_parent_child_relationship -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/models/job.py backend/app/tests/test_job_model.py
git commit -m "feat(models): add parent_job_id field for fan-out job relationships"
```

---

## Task 2: Add WISHLIST_IMPORT to ImportJobSubtype Enum

**Files:**
- Modify: `backend/app/models/job.py`

**Step 1: Write the test**

Add to `backend/app/tests/test_job_model.py`:

```python
def test_import_job_subtype_wishlist_import():
    """Test that WISHLIST_IMPORT subtype exists."""
    from app.models.job import ImportJobSubtype

    assert ImportJobSubtype.WISHLIST_IMPORT == "wishlist_import"
    assert ImportJobSubtype.WISHLIST_IMPORT.value == "wishlist_import"
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/test_job_model.py::test_import_job_subtype_wishlist_import -v`
Expected: FAIL with AttributeError

**Step 3: Add WISHLIST_IMPORT to enum**

In `backend/app/models/job.py`, find `ImportJobSubtype` enum and add:

```python
class ImportJobSubtype(str, Enum):
    """Subtype for import jobs - specifies the kind of import operation."""

    LIBRARY_IMPORT = "library_import"
    WISHLIST_IMPORT = "wishlist_import"  # Add this line
    AUTO_MATCH = "auto_match"
    BULK_SYNC = "bulk_sync"
    BULK_UNMATCH = "bulk_unmatch"
    BULK_UNSYNC = "bulk_unsync"
    BULK_UNIGNORE = "bulk_unignore"
```

**Step 4: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/test_job_model.py::test_import_job_subtype_wishlist_import -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/models/job.py backend/app/tests/test_job_model.py
git commit -m "feat(models): add WISHLIST_IMPORT subtype for fan-out imports"
```

---

## Task 3: Create Alembic Migration for parent_job_id

**Files:**
- Create: `backend/alembic/versions/xxxx_add_parent_job_id.py` (auto-generated)

**Step 1: Generate the migration**

Run: `cd backend && uv run alembic revision --autogenerate -m "add parent_job_id to jobs table"`

**Step 2: Verify migration content**

Read the generated migration file and verify it contains:
- `op.add_column('jobs', sa.Column('parent_job_id', sa.String(), nullable=True))`
- `op.create_foreign_key(...)` with `ondelete='CASCADE'`
- `op.create_index('ix_jobs_parent_job_id', 'jobs', ['parent_job_id'])`

**Step 3: Edit migration if needed**

Ensure the foreign key has `ondelete='CASCADE'`. If not, edit the migration:

```python
def upgrade() -> None:
    op.add_column('jobs', sa.Column('parent_job_id', sa.String(), nullable=True))
    op.create_index('ix_jobs_parent_job_id', 'jobs', ['parent_job_id'])
    op.create_foreign_key(
        'fk_jobs_parent_job_id',
        'jobs', 'jobs',
        ['parent_job_id'], ['id'],
        ondelete='CASCADE'
    )


def downgrade() -> None:
    op.drop_constraint('fk_jobs_parent_job_id', 'jobs', type_='foreignkey')
    op.drop_index('ix_jobs_parent_job_id', table_name='jobs')
    op.drop_column('jobs', 'parent_job_id')
```

**Step 4: Run migration**

Run: `cd backend && uv run alembic upgrade head`
Expected: Migration applies successfully

**Step 5: Run all tests to verify nothing broke**

Run: `cd backend && uv run pytest -v --tb=short`
Expected: All tests pass

**Step 6: Commit**

```bash
git add backend/alembic/versions/
git commit -m "migration: add parent_job_id column to jobs table"
```

---

## Task 4: Update Jobs API to Filter Out Child Jobs

**Files:**
- Modify: `backend/app/api/jobs.py`
- Modify: `backend/app/tests/api/test_jobs_api.py`

**Step 1: Write the test**

Add to `backend/app/tests/api/test_jobs_api.py`:

```python
def test_list_jobs_excludes_child_jobs(client, auth_headers, session):
    """Test that job list excludes child jobs by default."""
    from app.models.job import Job, BackgroundJobType, BackgroundJobSource

    # Create parent job
    parent = Job(
        user_id=test_user_id,  # Use the test user ID from fixtures
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
    )
    session.add(parent)
    session.commit()
    session.refresh(parent)

    # Create child job
    child = Job(
        user_id=test_user_id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        parent_job_id=parent.id,
    )
    session.add(child)
    session.commit()

    # List jobs
    response = client.get("/jobs/", headers=auth_headers)
    assert response.status_code == 200

    data = response.json()
    job_ids = [job["id"] for job in data["jobs"]]

    # Parent should be in list, child should not
    assert parent.id in job_ids
    assert child.id not in job_ids
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/api/test_jobs_api.py::test_list_jobs_excludes_child_jobs -v`
Expected: FAIL (child job appears in list)

**Step 3: Update list_jobs endpoint**

In `backend/app/api/jobs.py`, modify the `list_jobs` function. After the line `query = select(Job).where(Job.user_id == current_user.id)`, add:

```python
    # Build query - only show jobs for the current user
    query = select(Job).where(Job.user_id == current_user.id)

    # Exclude child jobs (they appear in parent's detail view)
    query = query.where(Job.parent_job_id.is_(None))  # Add this line
```

**Step 4: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/api/test_jobs_api.py::test_list_jobs_excludes_child_jobs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/api/jobs.py backend/app/tests/api/test_jobs_api.py
git commit -m "feat(api): filter child jobs from main job list"
```

---

## Task 5: Add Get Job Children Endpoint

**Files:**
- Modify: `backend/app/api/jobs.py`
- Modify: `backend/app/schemas/job.py`

**Step 1: Write the test**

Add to `backend/app/tests/api/test_jobs_api.py`:

```python
def test_get_job_children(client, auth_headers, session):
    """Test getting child jobs for a parent job."""
    from app.models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus

    # Create parent job
    parent = Job(
        user_id=test_user_id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
    )
    session.add(parent)
    session.commit()
    session.refresh(parent)

    # Create child jobs
    for i in range(3):
        child = Job(
            user_id=test_user_id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            parent_job_id=parent.id,
            status=BackgroundJobStatus.COMPLETED if i < 2 else BackgroundJobStatus.FAILED,
        )
        session.add(child)
    session.commit()

    # Get all children
    response = client.get(f"/jobs/{parent.id}/children", headers=auth_headers)
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 3

    # Filter by status
    response = client.get(
        f"/jobs/{parent.id}/children?status=completed",
        headers=auth_headers
    )
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 2
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/api/test_jobs_api.py::test_get_job_children -v`
Expected: FAIL with 404 (endpoint doesn't exist)

**Step 3: Add JobChildrenResponse schema**

In `backend/app/schemas/job.py`, add at the end:

```python
class JobChildrenResponse(BaseModel):
    """Response model for list of child jobs."""

    children: List[JobResponse]
    total: int
```

**Step 4: Add get_job_children endpoint**

In `backend/app/api/jobs.py`, add after the `get_job` endpoint:

```python
@router.get("/{job_id}/children", response_model=List[JobResponse])
async def get_job_children(
    job_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_user)],
    status: Optional[JobStatus] = Query(default=None, description="Filter by status"),
    limit: int = Query(default=50, ge=1, le=200, description="Max items to return"),
    offset: int = Query(default=0, ge=0, description="Number of items to skip"),
):
    """
    Get child jobs for a parent job.

    Returns paginated list of child jobs with optional status filtering.
    Only accessible by the job owner.
    """
    logger.debug(f"Getting children for job {job_id}")

    # Verify parent job exists and belongs to user
    parent_job = session.get(Job, job_id)
    if not parent_job or parent_job.user_id != current_user.id:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Job not found",
        )

    # Build query for children
    query = select(Job).where(Job.parent_job_id == job_id)

    if status:
        query = query.where(Job.status == BackgroundJobStatus(status.value))

    # Order by created_at descending (newest first)
    query = query.order_by(desc(col(Job.created_at)))

    # Apply pagination
    query = query.offset(offset).limit(limit)

    children = session.exec(query).all()

    return [_job_to_response(job, session) for job in children]
```

Add to imports at top:

```python
from typing import Annotated, Optional, List
```

**Step 5: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/api/test_jobs_api.py::test_get_job_children -v`
Expected: PASS

**Step 6: Commit**

```bash
git add backend/app/api/jobs.py backend/app/schemas/job.py backend/app/tests/api/test_jobs_api.py
git commit -m "feat(api): add endpoint to get child jobs for a parent"
```

---

## Task 6: Update Cancel Job to Cascade to Children

**Files:**
- Modify: `backend/app/api/jobs.py`

**Step 1: Write the test**

Add to `backend/app/tests/api/test_jobs_api.py`:

```python
def test_cancel_job_cascades_to_children(client, auth_headers, session):
    """Test that cancelling a parent job also cancels pending/processing children."""
    from app.models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus

    # Create parent job
    parent = Job(
        user_id=test_user_id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PROCESSING,
    )
    session.add(parent)
    session.commit()
    session.refresh(parent)

    # Create child jobs with different statuses
    statuses = [
        BackgroundJobStatus.PENDING,
        BackgroundJobStatus.PROCESSING,
        BackgroundJobStatus.COMPLETED,  # Should NOT be cancelled
    ]
    children = []
    for s in statuses:
        child = Job(
            user_id=test_user_id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            parent_job_id=parent.id,
            status=s,
        )
        session.add(child)
        children.append(child)
    session.commit()
    for c in children:
        session.refresh(c)

    # Cancel parent
    response = client.post(f"/jobs/{parent.id}/cancel", headers=auth_headers)
    assert response.status_code == 200

    # Refresh children and check statuses
    session.expire_all()
    for c in children:
        session.refresh(c)

    assert children[0].status == BackgroundJobStatus.CANCELLED  # Was PENDING
    assert children[1].status == BackgroundJobStatus.CANCELLED  # Was PROCESSING
    assert children[2].status == BackgroundJobStatus.COMPLETED  # Should remain COMPLETED
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/api/test_jobs_api.py::test_cancel_job_cascades_to_children -v`
Expected: FAIL (children not cancelled)

**Step 3: Update cancel_job endpoint**

In `backend/app/api/jobs.py`, modify the `cancel_job` function. After updating the parent job status (before `session.commit()`), add:

```python
    # Cancel all non-terminal children
    from sqlalchemy import update as sa_update

    session.execute(
        sa_update(Job)
        .where(Job.parent_job_id == job_id)
        .where(Job.status.in_([
            BackgroundJobStatus.PENDING,
            BackgroundJobStatus.PROCESSING,
            BackgroundJobStatus.AWAITING_REVIEW,
            BackgroundJobStatus.READY,
            BackgroundJobStatus.FINALIZING,
        ]))
        .values(
            status=BackgroundJobStatus.CANCELLED,
            completed_at=datetime.now(timezone.utc),
        )
    )

    session.commit()
```

**Step 4: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/api/test_jobs_api.py::test_cancel_job_cascades_to_children -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/api/jobs.py backend/app/tests/api/test_jobs_api.py
git commit -m "feat(api): cascade cancellation to child jobs"
```

---

## Task 7: Add Progress Aggregation for Parent Jobs

**Files:**
- Modify: `backend/app/services/batch_job_service.py`
- Create: `backend/app/tests/services/test_batch_job_service.py`

**Step 1: Write the test**

Create `backend/app/tests/services/test_batch_job_service.py`:

```python
import pytest
from sqlmodel import Session

from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
)
from app.services.batch_job_service import BatchJobService


def test_get_aggregated_progress(session: Session):
    """Test progress aggregation for parent jobs."""
    # Create parent job
    parent = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PROCESSING,
    )
    session.add(parent)
    session.commit()
    session.refresh(parent)

    # Create child jobs with various statuses
    child_statuses = [
        BackgroundJobStatus.COMPLETED,
        BackgroundJobStatus.COMPLETED,
        BackgroundJobStatus.FAILED,
        BackgroundJobStatus.PROCESSING,
        BackgroundJobStatus.PENDING,
    ]
    for s in child_statuses:
        child = Job(
            user_id="test-user",
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            parent_job_id=parent.id,
            status=s,
        )
        session.add(child)
    session.commit()

    # Get aggregated progress
    service = BatchJobService(session)
    progress = service.get_aggregated_progress(parent.id)

    assert progress is not None
    assert progress["total"] == 5
    assert progress["completed"] == 2
    assert progress["failed"] == 1
    assert progress["processing"] == 1
    assert progress["pending"] == 1
    assert progress["progress_current"] == 3  # completed + failed
    assert progress["progress_total"] == 5
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/services/test_batch_job_service.py::test_get_aggregated_progress -v`
Expected: FAIL (method doesn't exist)

**Step 3: Add get_aggregated_progress method**

In `backend/app/services/batch_job_service.py`, add:

```python
from sqlalchemy import func, case
from sqlmodel import select

# ... existing code ...

    def get_aggregated_progress(self, job_id: str) -> Optional[dict]:
        """
        Get aggregated progress from child jobs.

        Returns None if job has no children (not a parent job).
        Otherwise returns dict with counts by status.
        """
        from app.models.job import Job, BackgroundJobStatus

        result = self.session.exec(
            select(
                func.count(Job.id).label("total"),
                func.sum(
                    case((Job.status == BackgroundJobStatus.COMPLETED, 1), else_=0)
                ).label("completed"),
                func.sum(
                    case((Job.status == BackgroundJobStatus.FAILED, 1), else_=0)
                ).label("failed"),
                func.sum(
                    case((Job.status == BackgroundJobStatus.CANCELLED, 1), else_=0)
                ).label("cancelled"),
                func.sum(
                    case((Job.status == BackgroundJobStatus.PROCESSING, 1), else_=0)
                ).label("processing"),
                func.sum(
                    case((Job.status == BackgroundJobStatus.PENDING, 1), else_=0)
                ).label("pending"),
            ).where(Job.parent_job_id == job_id)
        ).one()

        if result.total == 0:
            return None

        return {
            "total": result.total,
            "completed": result.completed or 0,
            "failed": result.failed or 0,
            "cancelled": result.cancelled or 0,
            "processing": result.processing or 0,
            "pending": result.pending or 0,
            "progress_current": (result.completed or 0) + (result.failed or 0) + (result.cancelled or 0),
            "progress_total": result.total,
        }
```

**Step 4: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/services/test_batch_job_service.py::test_get_aggregated_progress -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/services/batch_job_service.py backend/app/tests/services/test_batch_job_service.py
git commit -m "feat(services): add progress aggregation for parent jobs"
```

---

## Task 8: Update Job Response to Include Aggregated Progress

**Files:**
- Modify: `backend/app/api/jobs.py`
- Modify: `backend/app/schemas/job.py`

**Step 1: Write the test**

Add to `backend/app/tests/api/test_jobs_api.py`:

```python
def test_get_job_returns_aggregated_progress_for_parent(client, auth_headers, session):
    """Test that getting a parent job returns aggregated progress from children."""
    from app.models.job import Job, BackgroundJobType, BackgroundJobSource, BackgroundJobStatus

    # Create parent job with its own progress values
    parent = Job(
        user_id=test_user_id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PROCESSING,
        progress_current=0,
        progress_total=0,
    )
    session.add(parent)
    session.commit()
    session.refresh(parent)

    # Create completed children
    for _ in range(3):
        child = Job(
            user_id=test_user_id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            parent_job_id=parent.id,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(child)
    # Create one pending child
    child = Job(
        user_id=test_user_id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        parent_job_id=parent.id,
        status=BackgroundJobStatus.PENDING,
    )
    session.add(child)
    session.commit()

    # Get parent job
    response = client.get(f"/jobs/{parent.id}", headers=auth_headers)
    assert response.status_code == 200
    data = response.json()

    # Should have aggregated progress from children
    assert data["progress_total"] == 4
    assert data["progress_current"] == 3  # 3 completed
    assert data["progress_percent"] == 75
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/api/test_jobs_api.py::test_get_job_returns_aggregated_progress_for_parent -v`
Expected: FAIL (progress_total is 0)

**Step 3: Update _job_to_response function**

In `backend/app/api/jobs.py`, modify `_job_to_response`:

```python
def _job_to_response(job: Job, session: Session) -> JobResponse:
    """Convert a Job model to JobResponse with computed fields."""
    from app.services.batch_job_service import BatchJobService

    # Count review items for this job
    review_count_stmt = select(func.count()).select_from(ReviewItem).where(
        ReviewItem.job_id == job.id
    )
    review_item_count = session.exec(review_count_stmt).one()

    # Count pending review items
    pending_count_stmt = select(func.count()).select_from(ReviewItem).where(
        ReviewItem.job_id == job.id,
        ReviewItem.status == ReviewItemStatus.PENDING,
    )
    pending_review_count = session.exec(pending_count_stmt).one()

    # Check for aggregated progress from children
    batch_service = BatchJobService(session)
    aggregated = batch_service.get_aggregated_progress(job.id)

    if aggregated:
        progress_current = aggregated["progress_current"]
        progress_total = aggregated["progress_total"]
        progress_percent = int((progress_current / progress_total) * 100) if progress_total > 0 else 0
    else:
        progress_current = job.progress_current
        progress_total = job.progress_total
        progress_percent = job.progress_percent

    return JobResponse(
        id=job.id,
        user_id=job.user_id,
        job_type=JobType(job.job_type.value),
        source=JobSource(job.source.value),
        status=JobStatus(job.status.value),
        priority=job.priority,
        progress_current=progress_current,
        progress_total=progress_total,
        progress_percent=progress_percent,
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
```

**Step 4: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/api/test_jobs_api.py::test_get_job_returns_aggregated_progress_for_parent -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/api/jobs.py backend/app/tests/api/test_jobs_api.py
git commit -m "feat(api): return aggregated progress for parent jobs"
```

---

## Task 9: Create Coordinator Task

**Files:**
- Create: `backend/app/worker/tasks/import_export/import_nexorious_coordinator.py`
- Modify: `backend/app/worker/tasks/import_export/__init__.py`

**Step 1: Write the test**

Create `backend/app/tests/worker/test_import_nexorious_coordinator.py`:

```python
import pytest
from sqlmodel import Session, select

from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ImportJobSubtype,
)
from app.worker.tasks.import_export.import_nexorious_coordinator import (
    import_nexorious_coordinator,
)


@pytest.mark.asyncio
async def test_coordinator_creates_child_jobs(session: Session):
    """Test that coordinator creates child jobs for each game and wishlist item."""
    # Create parent job with import data
    parent = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PENDING,
    )
    parent.set_result_summary({
        "_import_data": {
            "export_version": "1.2",
            "games": [
                {"title": "Game 1", "igdb_id": 123},
                {"title": "Game 2", "igdb_id": 456},
            ],
            "wishlist": [
                {"title": "Wishlist Game", "igdb_id": 789},
            ],
        }
    })
    session.add(parent)
    session.commit()
    session.refresh(parent)

    # Run coordinator (mock the task enqueueing)
    result = await import_nexorious_coordinator(parent.id)

    assert result["status"] == "success"
    assert result["children_created"] == 3

    # Verify children were created
    session.expire_all()
    children = session.exec(
        select(Job).where(Job.parent_job_id == parent.id)
    ).all()

    assert len(children) == 3

    # Check game children
    game_children = [c for c in children if c.import_subtype == ImportJobSubtype.LIBRARY_IMPORT]
    assert len(game_children) == 2

    # Check wishlist children
    wishlist_children = [c for c in children if c.import_subtype == ImportJobSubtype.WISHLIST_IMPORT]
    assert len(wishlist_children) == 1

    # Verify _import_data was cleared from parent
    session.refresh(parent)
    result_summary = parent.get_result_summary()
    assert "_import_data" not in result_summary
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/worker/test_import_nexorious_coordinator.py -v`
Expected: FAIL (module doesn't exist)

**Step 3: Create coordinator task**

Create `backend/app/worker/tasks/import_export/import_nexorious_coordinator.py`:

```python
"""Nexorious JSON import coordinator task.

Parses the JSON export and fans out individual game/wishlist imports
to child tasks for parallel processing.
"""

import logging
from datetime import datetime, timezone
from typing import Dict, Any

from sqlmodel import Session

from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH
from app.worker.locking import acquire_job_lock, release_job_lock
from app.core.database import get_session_context
from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    BackgroundJobPriority,
    ImportJobSubtype,
)

logger = logging.getLogger(__name__)


@broker.task(
    task_name="import.nexorious_coordinator",
    queue=QUEUE_HIGH,
)
async def import_nexorious_coordinator(job_id: str) -> Dict[str, Any]:
    """
    Coordinate Nexorious JSON import by creating child jobs.

    This task:
    1. Parses the JSON data from the parent job
    2. Creates a child Job for each game and wishlist item
    3. Enqueues the item processing task for each child
    4. Clears the raw import data from the parent
    5. Exits immediately (does not wait for children)

    Args:
        job_id: The parent Job ID

    Returns:
        Dictionary with coordination statistics.
    """
    logger.info(f"Starting Nexorious import coordinator (job: {job_id})")

    async with get_session_context() as session:
        # Try to acquire advisory lock
        if not acquire_job_lock(session, job_id):
            logger.info(f"Job {job_id} already being processed")
            return {"status": "skipped", "reason": "duplicate_execution"}

        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            release_job_lock(session, job_id)
            return {"status": "error", "error": "Job not found"}

        try:
            # Update job status
            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            # Get import data
            result_summary = job.get_result_summary()
            import_data = result_summary.get("_import_data", {})

            if not import_data:
                raise ValueError("No import data found in job")

            games = import_data.get("games", [])
            wishlist = import_data.get("wishlist", [])

            children_created = 0

            # Create child jobs for games
            for game_data in games:
                child = _create_child_job(
                    session=session,
                    parent_job=job,
                    item_data=game_data,
                    subtype=ImportJobSubtype.LIBRARY_IMPORT,
                )
                children_created += 1

                # Enqueue child task
                from app.worker.tasks.import_export.import_nexorious_item import (
                    import_nexorious_item,
                )
                await import_nexorious_item.kiq(child.id)

            # Create child jobs for wishlist
            for wishlist_data in wishlist:
                child = _create_child_job(
                    session=session,
                    parent_job=job,
                    item_data=wishlist_data,
                    subtype=ImportJobSubtype.WISHLIST_IMPORT,
                )
                children_created += 1

                # Enqueue child task
                from app.worker.tasks.import_export.import_nexorious_item import (
                    import_nexorious_item,
                )
                await import_nexorious_item.kiq(child.id)

            # Clear import data and update parent
            result_summary.pop("_import_data", None)
            result_summary["children_created"] = children_created
            result_summary["games_count"] = len(games)
            result_summary["wishlist_count"] = len(wishlist)
            job.set_result_summary(result_summary)

            session.add(job)
            session.commit()

            logger.info(
                f"Coordinator completed for job {job_id}: "
                f"created {children_created} child jobs "
                f"({len(games)} games, {len(wishlist)} wishlist)"
            )

            return {
                "status": "success",
                "children_created": children_created,
                "games_count": len(games),
                "wishlist_count": len(wishlist),
            }

        except Exception as e:
            logger.error(f"Coordinator failed for job {job_id}: {e}")
            session.rollback()
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()
            return {"status": "error", "error": str(e)}
        finally:
            release_job_lock(session, job_id)


def _create_child_job(
    session: Session,
    parent_job: Job,
    item_data: Dict[str, Any],
    subtype: ImportJobSubtype,
) -> Job:
    """Create a child job for a single game or wishlist item."""
    child = Job(
        user_id=parent_job.user_id,
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        import_subtype=subtype,
        status=BackgroundJobStatus.PENDING,
        priority=BackgroundJobPriority.HIGH,
        parent_job_id=parent_job.id,
    )

    # Store item data for processing
    child.set_result_summary({
        "_item_data": item_data,
        "title": item_data.get("title"),
        "igdb_id": item_data.get("igdb_id"),
    })

    session.add(child)
    session.commit()
    session.refresh(child)

    return child
```

**Step 4: Update __init__.py**

In `backend/app/worker/tasks/import_export/__init__.py`:

```python
"""Import/export tasks for background processing."""

from app.worker.tasks.import_export.import_nexorious import import_nexorious_json
from app.worker.tasks.import_export.import_nexorious_coordinator import (
    import_nexorious_coordinator,
)
from app.worker.tasks.import_export.export import export_collection

__all__ = [
    "import_nexorious_json",
    "import_nexorious_coordinator",
    "export_collection",
]
```

**Step 5: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/worker/test_import_nexorious_coordinator.py -v`
Expected: PASS (may need to mock the kiq call)

**Step 6: Commit**

```bash
git add backend/app/worker/tasks/import_export/
git commit -m "feat(worker): add coordinator task for fan-out import"
```

---

## Task 10: Create Item Processing Task

**Files:**
- Create: `backend/app/worker/tasks/import_export/import_nexorious_item.py`
- Modify: `backend/app/worker/tasks/import_export/__init__.py`

**Step 1: Write the test**

Create `backend/app/tests/worker/test_import_nexorious_item.py`:

```python
import pytest
from unittest.mock import AsyncMock, patch
from sqlmodel import Session, select

from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ImportJobSubtype,
)
from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item


@pytest.mark.asyncio
async def test_item_task_processes_game(session: Session):
    """Test that item task processes a game import."""
    # Create parent job
    parent = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        status=BackgroundJobStatus.PROCESSING,
    )
    session.add(parent)
    session.commit()
    session.refresh(parent)

    # Create child job
    child = Job(
        user_id="test-user",
        job_type=BackgroundJobType.IMPORT,
        source=BackgroundJobSource.NEXORIOUS,
        import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
        status=BackgroundJobStatus.PENDING,
        parent_job_id=parent.id,
    )
    child.set_result_summary({
        "_item_data": {
            "title": "Test Game",
            "igdb_id": 12345,
            "play_status": "completed",
        },
        "title": "Test Game",
        "igdb_id": 12345,
    })
    session.add(child)
    session.commit()
    session.refresh(child)

    # Mock IGDB service
    with patch("app.services.game_service.GameService") as mock_game_service:
        mock_instance = mock_game_service.return_value
        mock_instance.create_or_update_game_from_igdb = AsyncMock()

        result = await import_nexorious_item(child.id)

    assert result["status"] in ["success", "already_in_collection"]

    # Verify child status updated
    session.refresh(child)
    assert child.status in [BackgroundJobStatus.COMPLETED, BackgroundJobStatus.FAILED]
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/worker/test_import_nexorious_item.py -v`
Expected: FAIL (module doesn't exist)

**Step 3: Create item processing task**

Create `backend/app/worker/tasks/import_export/import_nexorious_item.py`:

```python
"""Nexorious JSON item import task.

Processes a single game or wishlist item from a Nexorious export.
Called by the coordinator task for parallel processing.
"""

import logging
from datetime import datetime, timezone
from typing import Dict, Any

from sqlmodel import Session, select

from app.worker.broker import broker
from app.worker.queues import QUEUE_HIGH
from app.worker.locking import acquire_job_lock, release_job_lock
from app.core.database import get_session_context
from app.models.job import Job, BackgroundJobStatus, ImportJobSubtype
from app.models.game import Game
from app.models.user_game import UserGame
from app.models.wishlist import Wishlist
from app.services.igdb import IGDBService
from app.services.game_service import GameService

# Import helper functions from original import task
from app.worker.tasks.import_export.import_nexorious import (
    _process_nexorious_game,
    _process_wishlist_item,
)

logger = logging.getLogger(__name__)


@broker.task(
    task_name="import.nexorious_item",
    queue=QUEUE_HIGH,
)
async def import_nexorious_item(job_id: str) -> Dict[str, Any]:
    """
    Process a single game or wishlist item from Nexorious export.

    This task:
    1. Fetches the child job and extracts item data
    2. Processes the item (IGDB lookup, create UserGame/Wishlist)
    3. Updates child job status
    4. Checks if all siblings are complete and finalizes parent if so

    Args:
        job_id: The child Job ID

    Returns:
        Dictionary with processing result.
    """
    logger.debug(f"Processing Nexorious item (job: {job_id})")

    async with get_session_context() as session:
        # Try to acquire advisory lock
        if not acquire_job_lock(session, job_id):
            logger.info(f"Job {job_id} already being processed")
            return {"status": "skipped", "reason": "duplicate_execution"}

        job = session.get(Job, job_id)
        if not job:
            logger.error(f"Job {job_id} not found")
            release_job_lock(session, job_id)
            return {"status": "error", "error": "Job not found"}

        try:
            # Update job status
            job.status = BackgroundJobStatus.PROCESSING
            job.started_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            # Get item data
            result_summary = job.get_result_summary()
            item_data = result_summary.get("_item_data", {})

            if not item_data:
                raise ValueError("No item data found in job")

            # Create services
            igdb_service = IGDBService()
            game_service = GameService(session, igdb_service)

            # Process based on subtype
            if job.import_subtype == ImportJobSubtype.LIBRARY_IMPORT:
                result = await _process_nexorious_game(
                    session=session,
                    game_service=game_service,
                    user_id=job.user_id,
                    game_data=item_data,
                )
            elif job.import_subtype == ImportJobSubtype.WISHLIST_IMPORT:
                result = await _process_wishlist_item(
                    session=session,
                    game_service=game_service,
                    user_id=job.user_id,
                    wishlist_data=item_data,
                )
            else:
                raise ValueError(f"Unknown import subtype: {job.import_subtype}")

            # Update job status based on result
            result_summary.pop("_item_data", None)
            result_summary["result"] = result

            if result in ["imported", "already_in_collection", "already_exists"]:
                job.status = BackgroundJobStatus.COMPLETED
            else:
                job.status = BackgroundJobStatus.FAILED
                job.error_message = f"Import result: {result}"

            job.set_result_summary(result_summary)
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            # Check if parent should be finalized
            if job.parent_job_id:
                await _check_and_finalize_parent(session, job.parent_job_id)

            logger.debug(f"Item job {job_id} completed with result: {result}")
            return {"status": "success", "result": result}

        except Exception as e:
            logger.error(f"Item job {job_id} failed: {e}")
            session.rollback()
            job.status = BackgroundJobStatus.FAILED
            job.error_message = str(e)[:2000]
            job.completed_at = datetime.now(timezone.utc)
            session.add(job)
            session.commit()

            # Still check parent finalization even on failure
            if job.parent_job_id:
                await _check_and_finalize_parent(session, job.parent_job_id)

            return {"status": "error", "error": str(e)}
        finally:
            release_job_lock(session, job_id)


async def _check_and_finalize_parent(session: Session, parent_job_id: str) -> None:
    """
    Check if all sibling jobs are complete and finalize parent if so.

    Called by each child after reaching terminal state.
    Multiple children may call this simultaneously - that's safe
    because setting COMPLETED twice is idempotent.
    """
    from sqlalchemy import func

    # Count non-terminal children
    non_terminal_count = session.exec(
        select(func.count(Job.id))
        .where(Job.parent_job_id == parent_job_id)
        .where(
            Job.status.not_in([
                BackgroundJobStatus.COMPLETED,
                BackgroundJobStatus.FAILED,
                BackgroundJobStatus.CANCELLED,
            ])
        )
    ).one()

    if non_terminal_count == 0:
        # All children are terminal - finalize parent
        parent = session.get(Job, parent_job_id)
        if parent and parent.status != BackgroundJobStatus.COMPLETED:
            parent.status = BackgroundJobStatus.COMPLETED
            parent.completed_at = datetime.now(timezone.utc)
            session.add(parent)
            session.commit()
            logger.info(f"Parent job {parent_job_id} finalized (all children complete)")
```

**Step 4: Update __init__.py**

In `backend/app/worker/tasks/import_export/__init__.py`:

```python
"""Import/export tasks for background processing."""

from app.worker.tasks.import_export.import_nexorious import import_nexorious_json
from app.worker.tasks.import_export.import_nexorious_coordinator import (
    import_nexorious_coordinator,
)
from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item
from app.worker.tasks.import_export.export import export_collection

__all__ = [
    "import_nexorious_json",
    "import_nexorious_coordinator",
    "import_nexorious_item",
    "export_collection",
]
```

**Step 5: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/worker/test_import_nexorious_item.py -v`
Expected: PASS

**Step 6: Commit**

```bash
git add backend/app/worker/tasks/import_export/
git commit -m "feat(worker): add item processing task for fan-out import"
```

---

## Task 11: Update Import Endpoint to Use Coordinator

**Files:**
- Modify: `backend/app/api/import_endpoints.py`

**Step 1: Write the test**

Add to `backend/app/tests/api/test_import_endpoints.py`:

```python
@pytest.mark.asyncio
async def test_import_nexorious_enqueues_coordinator(client, auth_headers, mocker):
    """Test that importing Nexorious JSON enqueues the coordinator task."""
    # Mock the coordinator task
    mock_kiq = mocker.patch(
        "app.api.import_endpoints.import_nexorious_coordinator.kiq",
        new_callable=AsyncMock,
    )
    mock_kiq.return_value.task_id = "test-task-id"

    # Create test file
    test_data = {
        "export_version": "1.2",
        "games": [{"title": "Test", "igdb_id": 123}],
    }
    files = {"file": ("export.json", json.dumps(test_data), "application/json")}

    response = client.post("/import/nexorious", files=files, headers=auth_headers)

    assert response.status_code == 200
    mock_kiq.assert_called_once()
```

**Step 2: Run test to verify it fails**

Run: `cd backend && uv run pytest app/tests/api/test_import_endpoints.py::test_import_nexorious_enqueues_coordinator -v`
Expected: FAIL (old task is enqueued)

**Step 3: Update import endpoint**

In `backend/app/api/import_endpoints.py`, change the import:

```python
from ..worker.tasks.import_export import (
    import_nexorious_coordinator,  # Changed from import_nexorious_json
)
```

And update the task enqueue call (around line 207):

```python
    # Enqueue the coordinator task (changed from import_nexorious_json)
    task_result = await import_nexorious_coordinator.kiq(job.id)
```

**Step 4: Run test to verify it passes**

Run: `cd backend && uv run pytest app/tests/api/test_import_endpoints.py::test_import_nexorious_enqueues_coordinator -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/app/api/import_endpoints.py backend/app/tests/api/test_import_endpoints.py
git commit -m "feat(api): use coordinator task for Nexorious import"
```

---

## Task 12: Delete Old Monolithic Import Task

**Files:**
- Delete: `backend/app/worker/tasks/import_export/import_nexorious.py` (most of it)

**Step 1: Verify new tasks work**

Run: `cd backend && uv run pytest app/tests/worker/ -v`
Expected: All tests pass

**Step 2: Keep helper functions, remove task**

The file `import_nexorious.py` contains helper functions used by the item task:
- `_process_nexorious_game`
- `_process_wishlist_item`
- `_import_platforms`
- `_import_tags`
- `_map_play_status`
- `_map_ownership_status`
- `_parse_rating`
- `_parse_date`

Rename the file and keep only helpers:

```bash
mv backend/app/worker/tasks/import_export/import_nexorious.py \
   backend/app/worker/tasks/import_export/import_nexorious_helpers.py
```

Edit `import_nexorious_helpers.py` to remove the `import_nexorious_json` task function and its decorator. Keep all helper functions.

**Step 3: Update imports in item task**

In `backend/app/worker/tasks/import_export/import_nexorious_item.py`, update:

```python
from app.worker.tasks.import_export.import_nexorious_helpers import (
    _process_nexorious_game,
    _process_wishlist_item,
)
```

**Step 4: Update __init__.py**

In `backend/app/worker/tasks/import_export/__init__.py`:

```python
"""Import/export tasks for background processing."""

from app.worker.tasks.import_export.import_nexorious_coordinator import (
    import_nexorious_coordinator,
)
from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item
from app.worker.tasks.import_export.export import export_collection

__all__ = [
    "import_nexorious_coordinator",
    "import_nexorious_item",
    "export_collection",
]
```

**Step 5: Run all backend tests**

Run: `cd backend && uv run pytest -v`
Expected: All tests pass

**Step 6: Commit**

```bash
git add backend/app/worker/tasks/import_export/
git commit -m "refactor(worker): extract helpers, remove monolithic import task"
```

---

## Task 13: Add Frontend API for Job Children

**Files:**
- Modify: `frontend/src/api/jobs.ts`
- Modify: `frontend/src/types/jobs.ts`

**Step 1: Write the test**

Add to `frontend/src/api/jobs.test.ts`:

```typescript
describe('getJobChildren', () => {
  it('fetches children for a parent job', async () => {
    const mockChildren = [
      { id: 'child-1', status: 'completed', /* ... */ },
      { id: 'child-2', status: 'processing', /* ... */ },
    ];

    mockApi.get.mockResolvedValueOnce(mockChildren);

    const children = await getJobChildren('parent-id');

    expect(mockApi.get).toHaveBeenCalledWith('/jobs/parent-id/children', {
      params: {},
    });
    expect(children).toHaveLength(2);
  });

  it('filters by status', async () => {
    mockApi.get.mockResolvedValueOnce([]);

    await getJobChildren('parent-id', { status: JobStatus.FAILED });

    expect(mockApi.get).toHaveBeenCalledWith('/jobs/parent-id/children', {
      params: { status: 'failed' },
    });
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm run test -- --run src/api/jobs.test.ts`
Expected: FAIL (getJobChildren doesn't exist)

**Step 3: Add types**

In `frontend/src/types/jobs.ts`, add:

```typescript
export interface JobChildrenFilters {
  status?: JobStatus;
  limit?: number;
  offset?: number;
}
```

**Step 4: Add API function**

In `frontend/src/api/jobs.ts`, add:

```typescript
import type { JobChildrenFilters } from '@/types';

/**
 * Get child jobs for a parent job.
 */
export async function getJobChildren(
  jobId: string,
  filters?: JobChildrenFilters
): Promise<Job[]> {
  const params: Record<string, string | number> = {};

  if (filters?.status) params.status = filters.status;
  if (filters?.limit) params.limit = filters.limit;
  if (filters?.offset) params.offset = filters.offset;

  const response = await api.get<JobApiResponse[]>(`/jobs/${jobId}/children`, {
    params,
  });

  return response.map(transformJob);
}
```

**Step 5: Run test to verify it passes**

Run: `cd frontend && npm run test -- --run src/api/jobs.test.ts`
Expected: PASS

**Step 6: Commit**

```bash
git add frontend/src/api/jobs.ts frontend/src/api/jobs.test.ts frontend/src/types/jobs.ts
git commit -m "feat(frontend): add API for fetching job children"
```

---

## Task 14: Add useJobChildren Hook

**Files:**
- Modify: `frontend/src/hooks/use-jobs.ts`
- Modify: `frontend/src/hooks/use-jobs.test.ts`

**Step 1: Write the test**

Add to `frontend/src/hooks/use-jobs.test.ts`:

```typescript
describe('useJobChildren', () => {
  it('fetches children for a job', async () => {
    const mockChildren = [/* mock Job objects */];
    vi.mocked(jobsApi.getJobChildren).mockResolvedValueOnce(mockChildren);

    const { result } = renderHook(() => useJobChildren('parent-id'));

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockChildren);
    expect(jobsApi.getJobChildren).toHaveBeenCalledWith('parent-id', undefined);
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm run test -- --run src/hooks/use-jobs.test.ts`
Expected: FAIL (useJobChildren doesn't exist)

**Step 3: Add hook**

In `frontend/src/hooks/use-jobs.ts`, add:

```typescript
import { getJobChildren } from '@/api/jobs';
import type { JobChildrenFilters } from '@/types';

/**
 * Hook to fetch child jobs for a parent job.
 */
export function useJobChildren(jobId: string, filters?: JobChildrenFilters) {
  return useQuery({
    queryKey: ['jobs', jobId, 'children', filters],
    queryFn: () => getJobChildren(jobId, filters),
    enabled: !!jobId,
    refetchInterval: (query) => {
      // Refetch while there are non-terminal children
      const data = query.state.data;
      if (!data) return false;
      const hasActiveChildren = data.some((child) => !child.isTerminal);
      return hasActiveChildren ? 3000 : false;
    },
  });
}
```

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm run test -- --run src/hooks/use-jobs.test.ts`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/hooks/use-jobs.ts frontend/src/hooks/use-jobs.test.ts
git commit -m "feat(frontend): add useJobChildren hook"
```

---

## Task 15: Add Job Children List Component

**Files:**
- Create: `frontend/src/components/jobs/job-children-list.tsx`
- Create: `frontend/src/components/jobs/job-children-list.test.tsx`

**Step 1: Write the test**

Create `frontend/src/components/jobs/job-children-list.test.tsx`:

```typescript
import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { JobChildrenList } from './job-children-list';
import { useJobChildren } from '@/hooks/use-jobs';
import { JobStatus } from '@/types';

vi.mock('@/hooks/use-jobs');

describe('JobChildrenList', () => {
  it('renders children with status indicators', () => {
    vi.mocked(useJobChildren).mockReturnValue({
      data: [
        { id: '1', status: JobStatus.COMPLETED, resultSummary: { title: 'Game 1' } },
        { id: '2', status: JobStatus.FAILED, resultSummary: { title: 'Game 2' }, errorMessage: 'Error' },
        { id: '3', status: JobStatus.PROCESSING, resultSummary: { title: 'Game 3' } },
      ],
      isLoading: false,
      isError: false,
    } as any);

    render(<JobChildrenList jobId="parent-id" />);

    expect(screen.getByText('Game 1')).toBeInTheDocument();
    expect(screen.getByText('Game 2')).toBeInTheDocument();
    expect(screen.getByText('Game 3')).toBeInTheDocument();
  });

  it('shows loading state', () => {
    vi.mocked(useJobChildren).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    } as any);

    render(<JobChildrenList jobId="parent-id" />);

    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd frontend && npm run test -- --run src/components/jobs/job-children-list.test.tsx`
Expected: FAIL (component doesn't exist)

**Step 3: Create component**

Create `frontend/src/components/jobs/job-children-list.tsx`:

```tsx
'use client';

import { useState } from 'react';
import { useJobChildren } from '@/hooks/use-jobs';
import { JobStatus, getJobStatusLabel, getJobStatusVariant } from '@/types';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { CheckCircle2, XCircle, Loader2, Clock } from 'lucide-react';

interface JobChildrenListProps {
  jobId: string;
}

export function JobChildrenList({ jobId }: JobChildrenListProps) {
  const [statusFilter, setStatusFilter] = useState<JobStatus | 'all'>('all');

  const { data: children, isLoading, isError } = useJobChildren(
    jobId,
    statusFilter !== 'all' ? { status: statusFilter } : undefined
  );

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <span className="ml-2 text-muted-foreground">Loading children...</span>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="text-destructive py-4">
        Failed to load child jobs
      </div>
    );
  }

  const statusIcon = (status: JobStatus) => {
    switch (status) {
      case JobStatus.COMPLETED:
        return <CheckCircle2 className="h-4 w-4 text-green-500" />;
      case JobStatus.FAILED:
        return <XCircle className="h-4 w-4 text-destructive" />;
      case JobStatus.PROCESSING:
        return <Loader2 className="h-4 w-4 animate-spin text-blue-500" />;
      default:
        return <Clock className="h-4 w-4 text-muted-foreground" />;
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="font-medium">Child Jobs</h3>
        <Select
          value={statusFilter}
          onValueChange={(v) => setStatusFilter(v as JobStatus | 'all')}
        >
          <SelectTrigger className="w-[150px]">
            <SelectValue placeholder="Filter by status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All</SelectItem>
            <SelectItem value={JobStatus.COMPLETED}>Completed</SelectItem>
            <SelectItem value={JobStatus.FAILED}>Failed</SelectItem>
            <SelectItem value={JobStatus.PROCESSING}>Processing</SelectItem>
            <SelectItem value={JobStatus.PENDING}>Pending</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2 max-h-96 overflow-y-auto">
        {children?.map((child) => (
          <div
            key={child.id}
            className="flex items-center justify-between p-3 rounded-lg border"
          >
            <div className="flex items-center gap-3">
              {statusIcon(child.status)}
              <span className="font-medium">
                {(child.resultSummary as { title?: string })?.title || 'Unknown'}
              </span>
            </div>
            <div className="flex items-center gap-2">
              {child.errorMessage && (
                <span className="text-sm text-destructive truncate max-w-[200px]">
                  {child.errorMessage}
                </span>
              )}
              <Badge variant={getJobStatusVariant(child.status)}>
                {getJobStatusLabel(child.status)}
              </Badge>
            </div>
          </div>
        ))}

        {children?.length === 0 && (
          <div className="text-center py-4 text-muted-foreground">
            No child jobs found
          </div>
        )}
      </div>
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

Run: `cd frontend && npm run test -- --run src/components/jobs/job-children-list.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/components/jobs/job-children-list.tsx frontend/src/components/jobs/job-children-list.test.tsx
git commit -m "feat(frontend): add JobChildrenList component"
```

---

## Task 16: Integrate Job Children into Job Detail Page

**Files:**
- Modify: `frontend/src/app/(dashboard)/jobs/[id]/page.tsx` (or wherever job detail is)

**Step 1: Identify the job detail page location**

Find the job detail page component (likely in `src/app` or `src/pages`).

**Step 2: Import and use JobChildrenList**

Add the component to show children when the job has them:

```tsx
import { JobChildrenList } from '@/components/jobs/job-children-list';

// In the component, after the main job info:
{/* Show children for parent jobs */}
{job && job.progressTotal > 0 && (
  <Card>
    <CardHeader>
      <CardTitle>Import Progress</CardTitle>
    </CardHeader>
    <CardContent>
      <JobChildrenList jobId={job.id} />
    </CardContent>
  </Card>
)}
```

**Step 3: Run frontend tests**

Run: `cd frontend && npm run test`
Expected: All tests pass

**Step 4: Run frontend type check**

Run: `cd frontend && npm run check`
Expected: No type errors

**Step 5: Commit**

```bash
git add frontend/src/app/
git commit -m "feat(frontend): show job children in detail view"
```

---

## Task 17: Run Full Test Suite and Fix Issues

**Files:**
- Various files may need fixes

**Step 1: Run backend tests**

Run: `cd backend && uv run pytest --cov=app --cov-report=term-missing`
Expected: All tests pass, >80% coverage

**Step 2: Run frontend tests**

Run: `cd frontend && npm run test`
Expected: All tests pass

**Step 3: Run type checks**

Run: `cd backend && uv run pyrefly check`
Run: `cd frontend && npm run check`
Expected: No type errors

**Step 4: Fix any issues found**

Address any failing tests or type errors.

**Step 5: Commit fixes**

```bash
git add .
git commit -m "fix: address test failures and type errors"
```

---

## Task 18: Final Verification and Cleanup

**Step 1: Verify migration works on fresh database**

```bash
cd backend
rm -f *.db  # Remove SQLite database if using
uv run alembic upgrade head
uv run pytest -v
```

**Step 2: Manual testing (optional)**

Start the backend and frontend, upload a Nexorious JSON export, verify:
- Coordinator task creates child jobs
- Children process in parallel
- Progress aggregates correctly
- Cancellation cascades
- Children visible in job detail

**Step 3: Create final commit with any cleanup**

```bash
git add .
git commit -m "chore: final cleanup for fan-out import"
```

# Job/Task System Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor the background job system from PostgreSQL-based TaskIQ broker to NATS JetStream with simplified Job + JobItem data model.

**Architecture:** Replace taskiq-pg with taskiq-nats using PullBasedJetStreamBroker. Simplify data model by replacing Job parent-child pattern and ReviewItem with unified JobItem. Use subject-based routing for priority queues. Remove advisory locking and startup recovery (JetStream handles this).

**Tech Stack:** TaskIQ, taskiq-nats, NATS JetStream, SQLModel, Alembic, FastAPI

---

## Phase 1: Infrastructure Setup

### Task 1: Add NATS to Docker Compose

**Files:**
- Modify: `docker-compose.yml`

**Step 1: Add NATS service to docker-compose.yml**

Add after the `db` service:

```yaml
  nats:
    image: nats:2.10-alpine
    ports:
      - "4222:4222"
      - "8222:8222"
    volumes:
      - nats_data:/data
    command:
      - "--jetstream"
      - "--store_dir=/data"
    healthcheck:
      test: ["CMD", "nats-server", "--help"]
      interval: 5s
      timeout: 5s
      retries: 5
```

Add `nats_data` to the volumes section.

Update `worker` service to depend on `nats` instead of just `db`.

Add `NATS_URL=nats://nats:4222` to worker environment.

**Step 2: Verify compose config is valid**

Run: `podman-compose config`
Expected: Valid YAML output with nats service

**Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "infra: add NATS JetStream service to docker-compose"
```

---

### Task 2: Update Python Dependencies

**Files:**
- Modify: `backend/pyproject.toml`

**Step 1: Update dependencies**

Remove `taskiq-pg` and add `taskiq-nats`:

In `[project.dependencies]`, replace:
```
"taskiq-pg>=0.1.0",
```

With:
```
"taskiq-nats>=0.5.0",
```

**Step 2: Sync dependencies**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv sync`
Expected: Dependencies installed successfully

**Step 3: Commit**

```bash
git add backend/pyproject.toml backend/uv.lock
git commit -m "deps: replace taskiq-pg with taskiq-nats"
```

---

### Task 3: Add NATS_URL to Settings

**Files:**
- Modify: `backend/app/core/config.py`

**Step 1: Add NATS_URL setting**

Add to the Settings class:

```python
NATS_URL: str = "nats://localhost:4222"
```

**Step 2: Verify settings load**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.core.config import settings; print(settings.NATS_URL)"`
Expected: `nats://localhost:4222`

**Step 3: Commit**

```bash
git add backend/app/core/config.py
git commit -m "config: add NATS_URL setting"
```

---

## Phase 2: Data Model Changes

### Task 4: Create JobItem Model and Simplify Job Model

**Files:**
- Modify: `backend/app/models/job.py`

**Step 1: Add JobItemStatus enum**

Add after BackgroundJobPriority:

```python
class JobItemStatus(str, Enum):
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    PENDING_REVIEW = "pending_review"
    SKIPPED = "skipped"
    FAILED = "failed"
```

**Step 2: Create JobItem model**

Add after the Job class:

```python
class JobItem(SQLModel, table=True):
    __tablename__ = "job_items"

    id: str = Field(default_factory=lambda: str(uuid.uuid4()), primary_key=True)
    job_id: str = Field(foreign_key="jobs.id", index=True)
    user_id: str = Field(foreign_key="users.id", index=True)

    # Item identification
    item_key: str = Field(index=True)
    source_title: str
    source_metadata_json: str = Field(default="{}")

    # Processing outcome
    status: JobItemStatus = Field(default=JobItemStatus.PENDING, index=True)
    result_json: str = Field(default="{}")
    error_message: str | None = None

    # Review fields (when status=PENDING_REVIEW)
    igdb_candidates_json: str = Field(default="[]")
    resolved_igdb_id: int | None = None
    match_confidence: float | None = None

    # Timestamps
    created_at: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    processed_at: datetime | None = None
    resolved_at: datetime | None = None

    # Relationships
    job: "Job" = Relationship(back_populates="items")

    __table_args__ = (
        UniqueConstraint("job_id", "item_key", name="uq_job_item_key"),
    )
```

**Step 3: Simplify Job model**

Remove these fields from Job:
- `progress_current`
- `progress_total`
- `successful_items`
- `failed_items`
- `result_summary_json`
- `error_log_json`
- `processed_item_ids_json`
- `failed_item_ids_json`
- `parent_job_id`
- `import_subtype`
- `taskiq_task_id`

Remove these relationships:
- `children`
- `parent`
- `review_items`

Add new relationship:
```python
items: list["JobItem"] = Relationship(back_populates="job")
```

Remove these statuses from BackgroundJobStatus:
- `AWAITING_REVIEW`
- `READY`
- `FINALIZING`

**Step 4: Remove ReviewItem model entirely**

Delete the entire ReviewItem class and ReviewItemStatus enum.

**Step 5: Update imports**

Add to imports at top:
```python
from sqlalchemy import UniqueConstraint
```

**Step 6: Commit**

```bash
git add backend/app/models/job.py
git commit -m "models: add JobItem, simplify Job, remove ReviewItem"
```

---

### Task 5: Create Database Migration

**Files:**
- Create: `backend/app/alembic/versions/XXXX_job_task_system_refactor.py` (auto-generated)

**Step 1: Generate migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic revision --autogenerate -m "job task system refactor"`
Expected: New migration file created

**Step 2: Review migration**

Read the generated migration file and verify it:
- Creates `job_items` table
- Removes columns from `jobs` table
- Drops `review_items` table

**Step 3: Apply migration**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run alembic upgrade head`
Expected: Migration applied successfully

**Step 4: Commit**

```bash
git add backend/app/alembic/versions/
git commit -m "migration: job task system refactor"
```

---

## Phase 3: Broker Configuration

### Task 6: Replace Broker with NATS JetStream

**Files:**
- Modify: `backend/app/worker/broker.py`

**Step 1: Rewrite broker.py**

Replace entire contents with:

```python
"""NATS JetStream broker configuration for TaskIQ."""

from taskiq_nats import PullBasedJetStreamBroker

from app.core.config import settings

broker = PullBasedJetStreamBroker(
    servers=[settings.NATS_URL],
    stream_name="nexorious_tasks",
    durable="nexorious_workers",
)
```

**Step 2: Verify broker imports**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run python -c "from app.worker.broker import broker; print(broker)"`
Expected: PullBasedJetStreamBroker instance printed

**Step 3: Commit**

```bash
git add backend/app/worker/broker.py
git commit -m "broker: switch to NATS JetStream PullBasedJetStreamBroker"
```

---

### Task 7: Update Queue Configuration for Subject-Based Routing

**Files:**
- Modify: `backend/app/worker/queues.py`

**Step 1: Update queue constants**

Replace contents with:

```python
"""Queue configuration for NATS subject-based routing."""

# Priority subjects for NATS JetStream
# High priority: user-initiated tasks (manual sync, imports, exports)
# Low priority: automated tasks (scheduled syncs, maintenance)

SUBJECT_HIGH_IMPORT = "tasks.high.import"
SUBJECT_HIGH_SYNC = "tasks.high.sync"
SUBJECT_HIGH_EXPORT = "tasks.high.export"

SUBJECT_LOW_IMPORT = "tasks.low.import"
SUBJECT_LOW_SYNC = "tasks.low.sync"
SUBJECT_LOW_MAINTENANCE = "tasks.low.maintenance"

# Legacy compatibility (will be removed after full migration)
QUEUE_HIGH = "high"
QUEUE_LOW = "low"
```

**Step 2: Commit**

```bash
git add backend/app/worker/queues.py
git commit -m "queues: add NATS subject constants for priority routing"
```

---

## Phase 4: Remove Legacy Code

### Task 8: Remove Advisory Locking

**Files:**
- Remove: `backend/app/worker/locking.py`

**Step 1: Delete locking.py**

Run: `rm backend/app/worker/locking.py`

**Step 2: Find and remove imports**

Search for imports of locking module and remove them from any files that import it.

**Step 3: Commit**

```bash
git add -A
git commit -m "cleanup: remove advisory locking (JetStream handles this)"
```

---

### Task 9: Remove Startup Recovery

**Files:**
- Remove: `backend/app/worker/startup.py`

**Step 1: Delete startup.py**

Run: `rm backend/app/worker/startup.py`

**Step 2: Remove startup event registration**

Check `backend/app/main.py` and remove any registration of the startup recovery hook.

**Step 3: Commit**

```bash
git add -A
git commit -m "cleanup: remove startup recovery (JetStream handles this)"
```

---

### Task 10: Remove Coordinator Pattern Files

**Files:**
- Remove: `backend/app/worker/tasks/import_export/import_nexorious_coordinator.py`
- Remove: `backend/app/worker/tasks/import_export/import_nexorious_item.py`

**Step 1: Delete coordinator files**

Run:
```bash
rm backend/app/worker/tasks/import_export/import_nexorious_coordinator.py
rm backend/app/worker/tasks/import_export/import_nexorious_item.py
```

**Step 2: Commit**

```bash
git add -A
git commit -m "cleanup: remove coordinator pattern files"
```

---

### Task 11: Remove Batch Job Service

**Files:**
- Remove: `backend/app/services/batch_job_service.py`

**Step 1: Delete batch_job_service.py**

Run: `rm backend/app/services/batch_job_service.py`

**Step 2: Find and remove imports**

Search for imports of batch_job_service and remove them.

**Step 3: Commit**

```bash
git add -A
git commit -m "cleanup: remove batch job service (progress derived from JobItem)"
```

---

## Phase 5: Job Schemas and API

### Task 12: Create JobItem Schemas

**Files:**
- Create: `backend/app/schemas/job_item.py`

**Step 1: Create job_item.py schemas**

```python
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
```

**Step 2: Commit**

```bash
git add backend/app/schemas/job_item.py
git commit -m "schemas: add JobItem schemas"
```

---

### Task 13: Update Job Schemas with Progress

**Files:**
- Modify: `backend/app/schemas/job.py`

**Step 1: Add JobProgress schema**

Add to job.py:

```python
class JobProgress(BaseModel):
    """Progress counts by status."""

    pending: int = 0
    processing: int = 0
    completed: int = 0
    pending_review: int = 0
    skipped: int = 0
    failed: int = 0

    @computed_field
    @property
    def total(self) -> int:
        return (
            self.pending
            + self.processing
            + self.completed
            + self.pending_review
            + self.skipped
            + self.failed
        )

    @computed_field
    @property
    def percent(self) -> int:
        if self.total == 0:
            return 0
        done = self.completed + self.pending_review + self.skipped + self.failed
        return int((done / self.total) * 100)
```

**Step 2: Update JobResponse to include progress**

Modify JobResponse to include:
```python
progress: JobProgress
total_items: int
```

Remove fields that no longer exist:
- `progress_current`
- `progress_total`
- `successful_items`
- `failed_items`
- `parent_job_id`
- etc.

**Step 3: Commit**

```bash
git add backend/app/schemas/job.py
git commit -m "schemas: add JobProgress, update JobResponse"
```

---

### Task 14: Create Job Service with Status Derivation

**Files:**
- Create: `backend/app/services/job_service.py`

**Step 1: Create job_service.py**

```python
"""Service for job operations with derived status."""

from sqlmodel import Session, select, func

from app.models.job import Job, JobItem, JobItemStatus, BackgroundJobStatus
from app.schemas.job import JobProgress


def get_job_progress(session: Session, job_id: str) -> JobProgress:
    """Get progress counts for a job from its items."""
    counts = session.exec(
        select(JobItem.status, func.count())
        .where(JobItem.job_id == job_id)
        .group_by(JobItem.status)
    ).all()

    status_counts = {status: count for status, count in counts}

    return JobProgress(
        pending=status_counts.get(JobItemStatus.PENDING, 0),
        processing=status_counts.get(JobItemStatus.PROCESSING, 0),
        completed=status_counts.get(JobItemStatus.COMPLETED, 0),
        pending_review=status_counts.get(JobItemStatus.PENDING_REVIEW, 0),
        skipped=status_counts.get(JobItemStatus.SKIPPED, 0),
        failed=status_counts.get(JobItemStatus.FAILED, 0),
    )


def get_derived_job_status(session: Session, job: Job) -> BackgroundJobStatus:
    """Derive job status from its items."""
    # Explicit statuses take precedence
    if job.status in (BackgroundJobStatus.FAILED, BackgroundJobStatus.CANCELLED):
        return job.status

    progress = get_job_progress(session, job.id)

    if progress.total == 0:
        return BackgroundJobStatus.PENDING

    if progress.processing > 0 or progress.pending > 0:
        return BackgroundJobStatus.PROCESSING

    return BackgroundJobStatus.COMPLETED
```

**Step 2: Commit**

```bash
git add backend/app/services/job_service.py
git commit -m "services: add job_service with status derivation"
```

---

### Task 15: Update Jobs API

**Files:**
- Modify: `backend/app/api/jobs.py`

**Step 1: Update imports**

Add:
```python
from app.services.job_service import get_job_progress, get_derived_job_status
from app.schemas.job_item import JobItemListResponse, JobItemResponse
```

**Step 2: Update list jobs endpoint**

Modify to use derived status and progress.

**Step 3: Add GET /jobs/{job_id}/items endpoint**

```python
@router.get("/{job_id}/items", response_model=JobItemListResponse)
async def list_job_items(
    job_id: str,
    status: JobItemStatus | None = None,
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """List items for a job with optional status filter."""
    # Verify job belongs to user
    job = session.get(Job, job_id)
    if not job or job.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job not found")

    query = select(JobItem).where(JobItem.job_id == job_id)
    if status:
        query = query.where(JobItem.status == status)

    # Count total
    count_query = select(func.count()).select_from(query.subquery())
    total = session.exec(count_query).one()

    # Paginate
    query = query.offset((page - 1) * page_size).limit(page_size)
    items = session.exec(query).all()

    return JobItemListResponse(
        items=[JobItemResponse.model_validate(item) for item in items],
        total=total,
        page=page,
        page_size=page_size,
    )
```

**Step 4: Remove /jobs/{job_id}/children endpoint**

Delete the children endpoint (no longer needed).

**Step 5: Commit**

```bash
git add backend/app/api/jobs.py
git commit -m "api: update jobs API with derived status, add /items endpoint"
```

---

### Task 16: Create Job Items API

**Files:**
- Create: `backend/app/api/job_items.py`

**Step 1: Create job_items.py**

```python
"""API endpoints for job items (replaces review API)."""

from datetime import datetime, timezone

from fastapi import APIRouter, Depends, HTTPException
from sqlmodel import Session

from app.api.deps import get_current_user, get_session
from app.models.job import JobItem, JobItemStatus
from app.models.user import User
from app.schemas.job_item import (
    JobItemDetailResponse,
    ResolveJobItemRequest,
    SkipJobItemRequest,
)

router = APIRouter(prefix="/job-items", tags=["job-items"])


@router.get("/{item_id}", response_model=JobItemDetailResponse)
async def get_job_item(
    item_id: str,
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """Get a job item by ID."""
    item = session.get(JobItem, item_id)
    if not item or item.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job item not found")
    return JobItemDetailResponse.model_validate(item)


@router.post("/{item_id}/resolve", response_model=JobItemDetailResponse)
async def resolve_job_item(
    item_id: str,
    request: ResolveJobItemRequest,
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """Resolve a job item to an IGDB game."""
    item = session.get(JobItem, item_id)
    if not item or item.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job item not found")

    if item.status != JobItemStatus.PENDING_REVIEW:
        raise HTTPException(status_code=400, detail="Item is not pending review")

    item.resolved_igdb_id = request.igdb_id
    item.status = JobItemStatus.COMPLETED
    item.resolved_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(item)

    return JobItemDetailResponse.model_validate(item)


@router.post("/{item_id}/skip", response_model=JobItemDetailResponse)
async def skip_job_item(
    item_id: str,
    request: SkipJobItemRequest,
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_user),
):
    """Skip a job item."""
    item = session.get(JobItem, item_id)
    if not item or item.user_id != current_user.id:
        raise HTTPException(status_code=404, detail="Job item not found")

    if item.status != JobItemStatus.PENDING_REVIEW:
        raise HTTPException(status_code=400, detail="Item is not pending review")

    item.status = JobItemStatus.SKIPPED
    item.resolved_at = datetime.now(timezone.utc)
    if request.reason:
        item.result_json = f'{{"skip_reason": "{request.reason}"}}'
    session.commit()
    session.refresh(item)

    return JobItemDetailResponse.model_validate(item)
```

**Step 2: Register router in main.py**

Add to `backend/app/main.py`:
```python
from app.api.job_items import router as job_items_router
app.include_router(job_items_router, prefix="/api")
```

**Step 3: Commit**

```bash
git add backend/app/api/job_items.py backend/app/main.py
git commit -m "api: add job-items API (replaces review API)"
```

---

### Task 17: Remove Review API

**Files:**
- Remove: `backend/app/api/review.py`
- Modify: `backend/app/main.py`

**Step 1: Delete review.py**

Run: `rm backend/app/api/review.py`

**Step 2: Remove router registration from main.py**

Remove the review router import and registration.

**Step 3: Remove review schemas**

Run: `rm backend/app/schemas/review.py`

**Step 4: Commit**

```bash
git add -A
git commit -m "cleanup: remove review API (replaced by job-items)"
```

---

## Phase 6: Task Implementations

### Task 18: Create Generic Import Task

**Files:**
- Create: `backend/app/worker/tasks/import_export/process_import_item.py`

**Step 1: Create process_import_item.py**

```python
"""Generic import item processor task."""

import json
from datetime import datetime, timezone

from sqlmodel import Session

from app.core.database import get_sync_session
from app.models.job import JobItem, JobItemStatus, BackgroundJobPriority
from app.services.matching_service import MatchingService
from app.worker.broker import broker
from app.worker.queues import SUBJECT_HIGH_IMPORT, SUBJECT_LOW_IMPORT

AUTO_MATCH_CONFIDENCE_THRESHOLD = 0.85


@broker.task(task_name=SUBJECT_HIGH_IMPORT)
async def process_import_item_high(job_item_id: str) -> dict:
    """Process high-priority import item."""
    return await _process_import_item(job_item_id)


@broker.task(task_name=SUBJECT_LOW_IMPORT)
async def process_import_item_low(job_item_id: str) -> dict:
    """Process low-priority import item."""
    return await _process_import_item(job_item_id)


async def _process_import_item(job_item_id: str) -> dict:
    """Process a single import item."""
    with get_sync_session() as session:
        job_item = session.get(JobItem, job_item_id)
        if not job_item:
            return {"status": "not_found"}

        # Idempotency: skip if already processed
        if job_item.status not in (JobItemStatus.PENDING, JobItemStatus.PROCESSING):
            return {"status": "already_processed", "item_status": job_item.status}

        job_item.status = JobItemStatus.PROCESSING
        session.commit()

        try:
            result = await _match_game(session, job_item)
            job_item.status = result["status"]
            job_item.result_json = json.dumps(result)
            job_item.processed_at = datetime.now(timezone.utc)

            if result.get("candidates"):
                job_item.igdb_candidates_json = json.dumps(result["candidates"])
            if result.get("confidence"):
                job_item.match_confidence = result["confidence"]
            if result.get("igdb_id"):
                job_item.resolved_igdb_id = result["igdb_id"]

        except Exception as e:
            job_item.status = JobItemStatus.FAILED
            job_item.error_message = str(e)

        session.commit()
        return {"status": job_item.status}


async def _match_game(session: Session, job_item: JobItem) -> dict:
    """Match a game to IGDB."""
    matching_service = MatchingService(session)

    candidates = await matching_service.search_igdb(job_item.source_title)

    if not candidates:
        return {
            "status": JobItemStatus.PENDING_REVIEW,
            "candidates": [],
            "reason": "no_matches",
        }

    best_match = candidates[0]
    confidence = matching_service.calculate_confidence(
        job_item.source_title, best_match
    )

    if confidence >= AUTO_MATCH_CONFIDENCE_THRESHOLD:
        return {
            "status": JobItemStatus.COMPLETED,
            "igdb_id": best_match["id"],
            "confidence": confidence,
            "action": "auto_matched",
        }

    return {
        "status": JobItemStatus.PENDING_REVIEW,
        "candidates": candidates[:5],
        "confidence": confidence,
        "reason": "low_confidence",
    }


async def enqueue_import_task(job_item_id: str, priority: BackgroundJobPriority):
    """Enqueue import task with appropriate priority."""
    if priority == BackgroundJobPriority.HIGH:
        await process_import_item_high.kiq(job_item_id)
    else:
        await process_import_item_low.kiq(job_item_id)
```

**Step 2: Commit**

```bash
git add backend/app/worker/tasks/import_export/process_import_item.py
git commit -m "tasks: add generic import item processor"
```

---

### Task 19: Update Import Endpoint for Fan-Out

**Files:**
- Modify: `backend/app/api/import_endpoints.py`

**Step 1: Update nexorious import endpoint**

The endpoint should:
1. Parse the JSON file
2. Create a Job record with total_items count
3. Create JobItems in bulk (all PENDING)
4. Enqueue one task per JobItem
5. Return job_id immediately

Key changes:
- Remove coordinator task call
- Create JobItems directly in the endpoint
- Use `enqueue_import_task` for each item

**Step 2: Commit**

```bash
git add backend/app/api/import_endpoints.py
git commit -m "api: update import endpoint for direct fan-out"
```

---

### Task 20: Update Steam Sync Task

**Files:**
- Modify: `backend/app/worker/tasks/sync/steam.py`

**Step 1: Update to use JobItem pattern**

The sync task should:
1. Create Job with total_items
2. Create JobItems for each game
3. Enqueue tasks for each JobItem
4. Remove inline processing (let tasks handle it)

**Step 2: Commit**

```bash
git add backend/app/worker/tasks/sync/steam.py
git commit -m "tasks: update steam sync to use JobItem pattern"
```

---

### Task 21: Update Export Task

**Files:**
- Modify: `backend/app/worker/tasks/import_export/export.py`

**Step 1: Update export task**

Export doesn't need JobItems (single operation). Update to:
- Use new broker
- Update subject/queue
- Remove any references to old patterns

**Step 2: Commit**

```bash
git add backend/app/worker/tasks/import_export/export.py
git commit -m "tasks: update export task for new broker"
```

---

### Task 22: Update Maintenance Tasks

**Files:**
- Modify: `backend/app/worker/tasks/maintenance/cleanup_stale_batch_jobs.py`
- Modify: `backend/app/worker/tasks/maintenance/cleanup_results.py`
- Modify: `backend/app/worker/tasks/maintenance/cleanup_exports.py`
- Modify: `backend/app/worker/tasks/maintenance/cleanup_sessions.py`

**Step 1: Update each maintenance task**

For each task:
- Update broker import
- Update subject to `SUBJECT_LOW_MAINTENANCE`
- Remove any references to old job patterns

**Step 2: Commit**

```bash
git add backend/app/worker/tasks/maintenance/
git commit -m "tasks: update maintenance tasks for new broker"
```

---

## Phase 7: Cleanup and Testing

### Task 23: Remove Unused Imports and Dead Code

**Files:**
- Various files that may have stale imports

**Step 1: Run type checker to find issues**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check`

**Step 2: Fix all import errors and type errors**

**Step 3: Commit**

```bash
git add -A
git commit -m "cleanup: remove unused imports and dead code"
```

---

### Task 24: Update Tests

**Files:**
- Modify: `backend/app/tests/` (various test files)

**Step 1: Update job model tests**

Update tests to use JobItem instead of ReviewItem and child jobs.

**Step 2: Update API tests**

- Update job API tests
- Replace review API tests with job-items API tests

**Step 3: Run tests**

Run: `cd /home/abo/workspace/home/nexorious/backend && uv run pytest`

**Step 4: Fix failing tests**

**Step 5: Commit**

```bash
git add backend/app/tests/
git commit -m "tests: update tests for new job/task system"
```

---

### Task 25: Integration Test with NATS

**Step 1: Start services**

Run: `podman-compose up -d`

**Step 2: Verify NATS is running**

Run: `curl http://localhost:8222/healthz`
Expected: `{"status":"ok"}`

**Step 3: Run a test import**

Use the API to trigger an import and verify:
- Job is created
- JobItems are created
- Tasks are enqueued to NATS
- Workers process tasks
- Progress updates correctly

**Step 4: Document any issues found**

---

### Task 26: Final Cleanup

**Step 1: Remove any remaining references to old patterns**

Search for:
- `ReviewItem`
- `parent_job_id`
- `taskiq-pg`
- `advisory_lock`

**Step 2: Update any documentation**

**Step 3: Final commit**

```bash
git add -A
git commit -m "cleanup: final cleanup for job/task system refactor"
```

---

## Summary

**Files Created:**
- `backend/app/schemas/job_item.py`
- `backend/app/services/job_service.py`
- `backend/app/api/job_items.py`
- `backend/app/worker/tasks/import_export/process_import_item.py`

**Files Removed:**
- `backend/app/worker/locking.py`
- `backend/app/worker/startup.py`
- `backend/app/worker/tasks/import_export/import_nexorious_coordinator.py`
- `backend/app/worker/tasks/import_export/import_nexorious_item.py`
- `backend/app/services/batch_job_service.py`
- `backend/app/api/review.py`
- `backend/app/schemas/review.py`

**Files Modified:**
- `docker-compose.yml`
- `backend/pyproject.toml`
- `backend/app/core/config.py`
- `backend/app/models/job.py`
- `backend/app/worker/broker.py`
- `backend/app/worker/queues.py`
- `backend/app/schemas/job.py`
- `backend/app/api/jobs.py`
- `backend/app/api/import_endpoints.py`
- `backend/app/main.py`
- `backend/app/worker/tasks/sync/steam.py`
- `backend/app/worker/tasks/import_export/export.py`
- `backend/app/worker/tasks/maintenance/*.py`

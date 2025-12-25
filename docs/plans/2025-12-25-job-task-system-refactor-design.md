# Job/Task System Refactor Design

## Overview

Refactor the background job and task system to use Valkey (Redis) as the message broker while simplifying the data model. This addresses issues with orphaned tasks after restarts and removes the need for DIY advisory locking.

## Goals

1. **Reliability**: Tasks survive restarts, no orphaned jobs
2. **Simplicity**: Remove custom locking and recovery code
3. **Clarity**: Unified data model (Job + JobItem replaces Job + Child Jobs + ReviewItem)
4. **Idempotency**: All tasks safe to re-run

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      PostgreSQL                              │
│  ┌─────────┐       ┌──────────┐                             │
│  │   Job   │ 1───* │ JobItem  │                             │
│  └─────────┘       └──────────┘                             │
│  - job metadata    - per-item result                        │
│  - total_items     - review data (when needed)              │
│  - status          - processing outcome                     │
└─────────────────────────────────────────────────────────────┘
                           │
                           │ tasks read/write
                           ▼
┌─────────────────────────────────────────────────────────────┐
│               TaskIQ + Valkey (Redis Streams)                │
│  - RedisStreamBroker (acknowledgment-based)                 │
│  - Consumer groups (one worker per task)                    │
│  - PEL + XCLAIM (automatic orphan recovery)                 │
│  - Idempotent tasks (re-run safe)                          │
└─────────────────────────────────────────────────────────────┘
```

## Data Model

### Job (Simplified)

```python
class Job(SQLModel, table=True):
    id: str  # UUID
    user_id: str  # FK

    job_type: BackgroundJobType   # SYNC, IMPORT, EXPORT
    source: BackgroundJobSource   # STEAM, EPIC, GOG, CSV, NEXORIOUS, etc.
    status: BackgroundJobStatus   # PENDING, PROCESSING, COMPLETED, FAILED, CANCELLED
    priority: BackgroundJobPriority  # HIGH, LOW

    total_items: int
    file_path: Optional[str]      # Export-specific
    error_message: Optional[str]  # Job-level failures only

    created_at: datetime
    started_at: Optional[datetime]
    completed_at: Optional[datetime]
```

**Removed fields:**
- `progress_current`, `progress_total` (derived from JobItem counts)
- `successful_items`, `failed_items` (derived from JobItem counts)
- `result_summary_json`, `error_log_json` (moved to JobItem)
- `processed_item_ids_json`, `failed_item_ids_json` (JobItem is source of truth)
- `parent_job_id`, `children`, `parent` (no parent-child pattern)
- `import_subtype` (context in JobItem if needed)
- `taskiq_task_id` (not needed with Redis Streams)

**Removed statuses:**
- `AWAITING_REVIEW` (derived: has JobItems with `status=PENDING_REVIEW`)
- `READY`, `FINALIZING` (simplified to PROCESSING until complete)

### JobItem (New - replaces Child Jobs + ReviewItem)

```python
class JobItemStatus(str, Enum):
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    PENDING_REVIEW = "pending_review"
    SKIPPED = "skipped"
    FAILED = "failed"

class JobItem(SQLModel, table=True):
    id: str  # UUID
    job_id: str  # FK -> Job
    user_id: str  # FK -> User

    # Item identification
    item_key: str               # Unique: Steam app ID, CSV row hash, etc.
    source_title: str           # Display title from source
    source_metadata_json: str   # Platform ID, release year, cover URL, etc.

    # Processing outcome
    status: JobItemStatus
    result_json: str            # {action, igdb_id, reason, ...}
    error_message: Optional[str]

    # Review fields (when status=PENDING_REVIEW)
    igdb_candidates_json: str
    resolved_igdb_id: Optional[int]
    match_confidence: Optional[float]

    # Timestamps
    created_at: datetime
    processed_at: Optional[datetime]
    resolved_at: Optional[datetime]

    __table_args__ = (
        UniqueConstraint("job_id", "item_key", name="uq_job_item_key"),
    )
```

## Job Status Derivation

Job status is always computed from JobItem states, never stored:

```python
def get_job_status(session: Session, job_id: str) -> BackgroundJobStatus:
    job = session.get(Job, job_id)

    if job.status == BackgroundJobStatus.FAILED:
        return BackgroundJobStatus.FAILED

    counts = session.exec(
        select(JobItem.status, func.count())
        .where(JobItem.job_id == job_id)
        .group_by(JobItem.status)
    ).all()

    status_counts = dict(counts)
    total = sum(status_counts.values())

    if total == 0:
        return BackgroundJobStatus.PENDING

    pending = status_counts.get(JobItemStatus.PENDING, 0)
    processing = status_counts.get(JobItemStatus.PROCESSING, 0)

    if processing > 0 or pending > 0:
        return BackgroundJobStatus.PROCESSING

    return BackgroundJobStatus.COMPLETED
```

## Task Flow

### Import/Sync Flow

```
1. API endpoint:
   - Parses input (JSON file, Steam API response, etc.)
   - Creates Job with total_items count
   - Creates JobItems in bulk (all PENDING)
   - Enqueues one task per JobItem
   - Returns job_id immediately

2. Each task (parallel across workers):
   - Fetches its JobItem
   - Skips if already processed (idempotency)
   - Processes item (IGDB match, create UserGame, etc.)
   - Updates JobItem status + result
   - Done

3. Job completion:
   - No explicit finalize step
   - Status derived from JobItem counts
   - Frontend queries for progress
```

### Task Definition

```python
@broker.task(task_name="import.process_item", queue=QUEUE_HIGH)
async def process_import_item(job_item_id: str) -> dict:
    async with get_session() as session:
        job_item = session.get(JobItem, job_item_id)
        if not job_item:
            return {"status": "not_found"}

        # Idempotency: skip if already processed
        if job_item.status not in (JobItemStatus.PENDING, JobItemStatus.PROCESSING):
            return {"status": "already_processed"}

        job_item.status = JobItemStatus.PROCESSING
        session.commit()

        try:
            result = await process_game(job_item)
            job_item.status = result.status
            job_item.result_json = result.to_json()
            job_item.processed_at = datetime.now(timezone.utc)
        except Exception as e:
            job_item.status = JobItemStatus.FAILED
            job_item.error_message = str(e)

        session.commit()
        return {"status": job_item.status}
```

### Task Categories

| Category | Job Record | JobItems | Result Backend |
|----------|------------|----------|----------------|
| Item-processing (Import, Sync) | Yes | Yes | No |
| Single-operation (Export, Backup) | Yes | No | No |
| System maintenance (Vacuum, Cleanup) | No | No | Yes (7-day TTL) |

## Broker Configuration

```python
from taskiq_redis import RedisStreamBroker, RedisResultBackend

broker = RedisStreamBroker(
    url="redis://valkey:6379",
    queue_name="nexorious_tasks",
    consumer_group="nexorious_workers",
)

# Result backend for system/maintenance tasks only
broker.with_result_backend(RedisResultBackend(url="redis://valkey:6379"))
```

## Startup Recovery

Handles edge cases where Valkey loses data or tasks were in-flight during shutdown:

```python
import asyncio
from redis.asyncio import Redis
from redis.exceptions import ConnectionError

async def wait_for_valkey(url: str, max_retries: int = 30, delay: float = 1.0) -> bool:
    redis = Redis.from_url(url)

    for attempt in range(max_retries):
        try:
            await redis.ping()
            await redis.close()
            return True
        except ConnectionError:
            logger.info(f"Waiting for Valkey... (attempt {attempt + 1}/{max_retries})")
            await asyncio.sleep(delay)

    await redis.close()
    return False

@app.on_event("startup")
async def recover_incomplete_jobs():
    valkey_url = settings.VALKEY_URL

    if not await wait_for_valkey(valkey_url):
        logger.error("Valkey not available after retries, skipping recovery")
        return

    async with get_session() as session:
        incomplete_items = session.exec(
            select(JobItem)
            .where(JobItem.status.in_([JobItemStatus.PENDING, JobItemStatus.PROCESSING]))
        ).all()

        for item in incomplete_items:
            item.status = JobItemStatus.PENDING
            await process_import_item.kiq(item.id)

        session.commit()
        logger.info(f"Re-queued {len(incomplete_items)} incomplete job items")
```

## API Changes

### Jobs API

| Endpoint | Change |
|----------|--------|
| `GET /jobs/` | Returns jobs with derived status + progress counts |
| `GET /jobs/{id}` | Returns job with derived status + JobItem summary |
| `GET /jobs/{id}/items` | New - paginated JobItems for a job |
| `POST /jobs/{id}/cancel` | Marks job as CANCELLED |
| `DELETE /jobs/{id}` | Deletes job + cascades to JobItems |

### Review API (Simplified)

| Current | New |
|---------|-----|
| `GET /review-items/` | `GET /jobs/{id}/items?status=pending_review` |
| `GET /review-items/{id}` | `GET /job-items/{id}` |
| `POST /review-items/{id}/resolve` | `POST /job-items/{id}/resolve` |
| `POST /review-items/{id}/skip` | `POST /job-items/{id}/skip` |

### Response Shape

```python
class JobProgress(BaseModel):
    pending: int
    processing: int
    completed: int
    pending_review: int
    skipped: int
    failed: int

    @property
    def percent(self) -> int:
        total = sum([self.pending, self.processing, self.completed,
                     self.pending_review, self.skipped, self.failed])
        if total == 0:
            return 0
        done = self.completed + self.pending_review + self.skipped + self.failed
        return int((done / total) * 100)

class JobResponse(BaseModel):
    id: str
    job_type: BackgroundJobType
    source: BackgroundJobSource
    status: BackgroundJobStatus  # Derived
    total_items: int
    progress: JobProgress
    created_at: datetime
    started_at: Optional[datetime]
    completed_at: Optional[datetime]
```

## Infrastructure

### Docker/Podman Compose

```yaml
services:
  valkey:
    image: valkey/valkey:8-alpine
    ports:
      - "6379:6379"
    volumes:
      - valkey_data:/data
    command: valkey-server --appendonly yes

volumes:
  valkey_data:
```

### Python Dependencies

| Remove | Add |
|--------|-----|
| `taskiq-pg` | `taskiq-redis` |

### Environment Variables

| Variable | Example |
|----------|---------|
| `VALKEY_URL` | `redis://valkey:6379` |

## Files to Remove

| File | Reason |
|------|--------|
| `app/worker/locking.py` | Advisory locks no longer needed |
| `app/worker/startup.py` | Replaced by new recovery function |
| `app/worker/tasks/import_export/import_nexorious_coordinator.py` | No coordinator pattern |
| `app/worker/tasks/import_export/import_nexorious_item.py` | Replaced by generic processor |
| `app/services/batch_job_service.py` | Progress aggregation not needed |

## Files to Modify

| File | Changes |
|------|---------|
| `app/worker/broker.py` | Switch to `taskiq-redis` |
| `app/models/job.py` | Simplify Job, remove ReviewItem, add JobItem |
| `app/api/jobs.py` | Derived status, new `/items` endpoint |
| `app/api/import_endpoints.py` | Fan-out in API, create JobItems |
| `app/worker/tasks/sync/steam.py` | Use JobItem pattern |
| `app/worker/tasks/import_export/export.py` | Job-only (no JobItems) |
| `app/worker/tasks/maintenance/*.py` | No Job record, use result backend |

## Database Migration

Since we're resetting the database:
- Create new `job_items` table
- Create simplified `jobs` table (no parent-child columns)
- Remove `review_items` table

## Key Decisions Summary

| Topic | Decision |
|-------|----------|
| Task framework | Keep TaskIQ, swap broker to Redis Streams |
| Message broker | Valkey with `taskiq-redis` RedisStreamBroker |
| Job tracking | PostgreSQL remains source of truth |
| Data model | Job + JobItem (replaces Job + Child Jobs + ReviewItem) |
| Fan-out pattern | API creates JobItems and enqueues tasks (no coordinator) |
| Task idempotency | Required - re-runs are safe |
| Locking | Redis Streams consumer groups (remove DIY locks) |
| Orphan recovery | Redis Streams PEL + startup recovery function |
| Job status | Always derived from JobItem counts |
| Result backend | Keep for system/maintenance tasks only |

# Job/Task System Refactor Design

## Overview

Refactor the background job and task system to use NATS JetStream as the message broker while simplifying the data model. JetStream's durable consumers with acknowledgment-based delivery eliminate the need for startup recovery code and DIY advisory locking.

## Goals

1. **Reliability**: JetStream handles message persistence and redelivery automatically
2. **Simplicity**: Remove custom locking, recovery code, and startup tasks
3. **Clarity**: Unified data model (Job + JobItem replaces Job + Child Jobs + ReviewItem)
4. **Priority Queues**: Subject-based routing for high/low priority tasks

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
│                 TaskIQ + NATS JetStream                      │
│  - PullBasedJetStreamBroker (acknowledgment-based)          │
│  - Durable consumers (automatic redelivery on failure)      │
│  - Subject-based routing (tasks.high.*, tasks.low.*)        │
│  - No startup recovery needed (JetStream handles it)        │
└─────────────────────────────────────────────────────────────┘
```

## Priority Queue Design

NATS subject hierarchy enables priority routing without separate infrastructure:

```
Subject Pattern:
  tasks.high.import.*    - High priority import tasks
  tasks.high.sync.*      - High priority sync tasks
  tasks.low.import.*     - Low priority import tasks
  tasks.low.maintenance  - Low priority maintenance tasks

Worker Subscription:
  Worker subscribes to "tasks.>" (all tasks)
  Weighted polling: check high-priority first, then low-priority
```

### Weighted Polling Implementation

```python
class PriorityAwareWorker:
    """Single worker that prioritizes high-priority tasks."""

    def __init__(self, js: JetStreamContext):
        self.js = js
        self.high_consumer = None
        self.low_consumer = None

    async def setup(self):
        # Create separate consumers for each priority
        self.high_consumer = await self.js.pull_subscribe(
            "tasks.high.>",
            durable="nexorious_high",
            config=ConsumerConfig(ack_wait=300),  # 5 min ack timeout
        )
        self.low_consumer = await self.js.pull_subscribe(
            "tasks.low.>",
            durable="nexorious_low",
            config=ConsumerConfig(ack_wait=300),
        )

    async def fetch_next_task(self) -> Optional[Msg]:
        # Always try high priority first
        try:
            msgs = await self.high_consumer.fetch(1, timeout=0.1)
            if msgs:
                return msgs[0]
        except TimeoutError:
            pass

        # Fall back to low priority
        try:
            msgs = await self.low_consumer.fetch(1, timeout=1.0)
            if msgs:
                return msgs[0]
        except TimeoutError:
            pass

        return None
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
- `taskiq_task_id` (not needed with JetStream durable consumers)

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
   - Enqueues one task per JobItem to appropriate priority subject
   - Returns job_id immediately

2. Each task (parallel across workers):
   - Fetches its JobItem
   - Skips if already processed (idempotency)
   - Processes item (IGDB match, create UserGame, etc.)
   - Updates JobItem status + result
   - Acknowledges message (JetStream removes from queue)
   - If task fails without ack, JetStream auto-redelivers

3. Job completion:
   - No explicit finalize step
   - Status derived from JobItem counts
   - Frontend queries for progress
```

### Task Definition

```python
from taskiq_nats import PullBasedJetStreamBroker

broker = PullBasedJetStreamBroker(
    servers=["nats://nats:4222"],
    stream_name="nexorious_tasks",
)

@broker.task(task_name="tasks.high.import.process_item")
async def process_import_item_high(job_item_id: str) -> dict:
    return await _process_import_item(job_item_id)

@broker.task(task_name="tasks.low.import.process_item")
async def process_import_item_low(job_item_id: str) -> dict:
    return await _process_import_item(job_item_id)

async def _process_import_item(job_item_id: str) -> dict:
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

### Enqueueing with Priority

```python
async def enqueue_import_task(job_item_id: str, priority: BackgroundJobPriority):
    if priority == BackgroundJobPriority.HIGH:
        await process_import_item_high.kiq(job_item_id)
    else:
        await process_import_item_low.kiq(job_item_id)
```

### Task Categories

| Category | Job Record | JobItems | Result Backend |
|----------|------------|----------|----------------|
| Item-processing (Import, Sync) | Yes | Yes | No |
| Single-operation (Export, Backup) | Yes | No | No |
| System maintenance (Vacuum, Cleanup) | No | No | Yes (7-day TTL) |

## Broker Configuration

```python
from taskiq_nats import PullBasedJetStreamBroker

broker = PullBasedJetStreamBroker(
    servers=["nats://nats:4222"],
    stream_name="nexorious_tasks",
    durable="nexorious_workers",
    stream_config={
        "subjects": ["tasks.>"],
        "retention": "work_queue",  # Delete messages after ack
        "storage": "file",          # Persist to disk
        "max_age": 86400 * 7,       # 7 day retention for unprocessed
    },
    consumer_config={
        "ack_wait": 300,            # 5 minute ack timeout
        "max_deliver": 3,           # Retry failed tasks up to 3 times
    },
)
```

### Result Backend (NATS KV Store)

For maintenance tasks that need queryable results, use NATS KV Store instead of Object Store. KV Store is designed for small key-value pairs with TTL support:

```python
from nats.js.kv import KeyValue

async def setup_result_store(js: JetStreamContext) -> KeyValue:
    """Create KV bucket for task results with 7-day TTL."""
    return await js.create_key_value(
        bucket="task_results",
        ttl=86400 * 7,  # 7 days
    )

async def store_task_result(kv: KeyValue, task_id: str, result: dict):
    """Store task result (auto-expires after TTL)."""
    await kv.put(f"task:{task_id}", json.dumps(result).encode())

async def get_task_result(kv: KeyValue, task_id: str) -> Optional[dict]:
    """Retrieve task result if it exists."""
    try:
        entry = await kv.get(f"task:{task_id}")
        return json.loads(entry.value.decode())
    except KeyNotFoundError:
        return None
```

Note: `taskiq-nats` doesn't have a built-in KV result backend, so maintenance tasks store results directly via the NATS KV API rather than through TaskIQ's result backend abstraction.

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
  nats:
    image: nats:2.10-alpine
    ports:
      - "4222:4222"   # Client connections
      - "8222:8222"   # HTTP monitoring
    volumes:
      - nats_data:/data
    command:
      - "--jetstream"
      - "--store_dir=/data"

volumes:
  nats_data:
```

### Python Dependencies

| Remove | Add |
|--------|-----|
| `taskiq-pg` | `taskiq-nats` |

### Environment Variables

| Variable | Example |
|----------|---------|
| `NATS_URL` | `nats://nats:4222` |

## Files to Remove

| File | Reason |
|------|--------|
| `app/worker/locking.py` | Advisory locks no longer needed |
| `app/worker/startup.py` | JetStream handles recovery automatically |
| `app/worker/tasks/import_export/import_nexorious_coordinator.py` | No coordinator pattern |
| `app/worker/tasks/import_export/import_nexorious_item.py` | Replaced by generic processor |
| `app/services/batch_job_service.py` | Progress aggregation not needed |

## Files to Modify

| File | Changes |
|------|---------|
| `app/worker/broker.py` | Switch to `taskiq-nats` PullBasedJetStreamBroker |
| `app/models/job.py` | Simplify Job, remove ReviewItem, add JobItem |
| `app/api/jobs.py` | Derived status, new `/items` endpoint |
| `app/api/import_endpoints.py` | Fan-out in API, create JobItems |
| `app/worker/tasks/sync/steam.py` | Use JobItem pattern |
| `app/worker/tasks/import_export/export.py` | Job-only (no JobItems) |
| `app/worker/tasks/maintenance/*.py` | No Job record, use result backend |
| `app/main.py` | Remove startup recovery hook |

## Database Migration

Since we're resetting the database:
- Create new `job_items` table
- Create simplified `jobs` table (no parent-child columns)
- Remove `review_items` table

## Key Decisions Summary

| Topic | Decision |
|-------|----------|
| Task framework | Keep TaskIQ, swap broker to NATS JetStream |
| Message broker | NATS with `taskiq-nats` PullBasedJetStreamBroker |
| Priority queues | Subject-based routing (`tasks.high.*`, `tasks.low.*`) |
| Worker model | Single worker type with weighted polling (high first) |
| Job tracking | PostgreSQL remains source of truth |
| Data model | Job + JobItem (replaces Job + Child Jobs + ReviewItem) |
| Fan-out pattern | API creates JobItems and enqueues tasks (no coordinator) |
| Task idempotency | Required - re-runs are safe |
| Orphan recovery | JetStream durable consumers (no startup recovery needed) |
| Job status | Always derived from JobItem counts |
| Result backend | NATS KV Store for system/maintenance tasks (7-day TTL) |

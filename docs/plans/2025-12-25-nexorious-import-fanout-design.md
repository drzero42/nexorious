# Nexorious Import Fan-Out Design

## Overview

Redesign the Nexorious JSON import to use parallel processing. Instead of a single task processing all games serially, a coordinator task fans out individual game/wishlist imports to multiple workers.

**Primary goal:** Parallelism for speed - multiple workers processing games simultaneously to reduce total import time.

## Design Decisions

| Decision | Choice |
|----------|--------|
| Tracking mechanism | `parent_job_id` foreign key (no separate batch_id) |
| Coordinator behavior | Separate task that creates parent job, fans out children, exits immediately |
| Child job model | Full Job records (reuses existing model, ReviewItem works unchanged) |
| UI presentation | Parent only in job list, children in detail view |
| Cancellation | Cancel all children regardless of state |
| Progress tracking | Real-time aggregation (query on read) |
| Parent completion | Last child to complete checks siblings, updates parent |
| Error handling | Parent COMPLETED even if some children failed |
| Scope | Both games AND wishlist items fan out as separate jobs |
| Job type distinction | Use `import_subtype` (LIBRARY_IMPORT vs WISHLIST_IMPORT) |

## Database Schema Changes

### Job Model Additions

```python
class Job(SQLModel, table=True):
    # ... existing fields ...

    # New field for parent-child relationship
    parent_job_id: UUID | None = Field(
        default=None,
        foreign_key="job.id",
        index=True,
    )

    # Relationship to children
    children: list["Job"] = Relationship(
        back_populates="parent",
        sa_relationship_kwargs={"cascade": "all, delete-orphan"},
    )
    parent: "Job" | None = Relationship(
        back_populates="children",
        sa_relationship_kwargs={"remote_side": "Job.id"},
    )
```

### ImportJobSubtype Enum

```python
class ImportJobSubtype(str, Enum):
    LIBRARY_IMPORT = "library_import"
    WISHLIST_IMPORT = "wishlist_import"  # New value
    AUTO_MATCH = "auto_match"
    # ... existing values ...
```

### Migration

```python
def upgrade():
    op.add_column(
        'job',
        sa.Column('parent_job_id', sa.UUID(), nullable=True)
    )

    op.create_foreign_key(
        'fk_job_parent_job_id',
        'job', 'job',
        ['parent_job_id'], ['id'],
        ondelete='CASCADE'
    )

    op.create_index('ix_job_parent_job_id', 'job', ['parent_job_id'])

def downgrade():
    op.drop_index('ix_job_parent_job_id')
    op.drop_constraint('fk_job_parent_job_id', 'job')
    op.drop_column('job', 'parent_job_id')
```

## Task Structure

### 1. Coordinator Task (new)

```python
@broker.task(task_name="import.nexorious_coordinator", queue=QUEUE_HIGH)
async def import_nexorious_coordinator(job_id: str) -> dict:
    """
    - Fetches parent job, extracts JSON from result_summary["_import_data"]
    - Parses games array and wishlist array
    - Creates child Job records (status=PENDING, parent_job_id=job_id)
      - Games: import_subtype=LIBRARY_IMPORT
      - Wishlist: import_subtype=WISHLIST_IMPORT
    - Stores item data in each child's result_summary (title, igdb_id, metadata)
    - Enqueues import_nexorious_item task for each child
    - Clears _import_data from parent (no longer needed)
    - Sets parent progress_total = number of children created
    - Exits (does NOT wait for children)
    """
```

### 2. Item Processing Task (new)

```python
@broker.task(task_name="import.nexorious_item", queue=QUEUE_HIGH)
async def import_nexorious_item(job_id: str) -> dict:
    """
    - Fetches child job, extracts item data from result_summary
    - Sets status=PROCESSING
    - Based on import_subtype:
      - LIBRARY_IMPORT: imports game (IGDB lookup, create UserGame, platforms, tags)
      - WISHLIST_IMPORT: imports wishlist item (IGDB lookup, create Wishlist)
    - Sets status=COMPLETED or FAILED
    - Checks if all siblings are terminal
      - If yes, updates parent status to COMPLETED
    """
```

### 3. Endpoint Changes

```python
# POST /import/nexorious
# - Creates parent Job (status=PENDING, no parent_job_id)
# - Stores JSON in result_summary["_import_data"]
# - Enqueues import_nexorious_coordinator (replaces old import_nexorious_json)
```

## Child Completion and Parent Finalization

```python
async def _check_and_finalize_parent(session: Session, parent_job_id: UUID):
    """
    Called by each child after reaching terminal state.
    Checks if all siblings are done, finalizes parent if so.
    """
    result = await session.exec(
        select(func.count(Job.id))
        .where(Job.parent_job_id == parent_job_id)
        .where(Job.status.not_in([
            BackgroundJobStatus.COMPLETED,
            BackgroundJobStatus.FAILED,
            BackgroundJobStatus.CANCELLED,
        ]))
    )
    pending_count = result.one()

    if pending_count == 0:
        parent = await session.get(Job, parent_job_id)
        parent.status = BackgroundJobStatus.COMPLETED
        parent.completed_at = datetime.now(UTC)
        await session.commit()
```

**Race condition note:** Multiple children completing simultaneously may all see `pending_count == 0`. This is safe - setting COMPLETED twice is idempotent.

## Progress Aggregation (Query on Read)

```python
async def get_job_with_progress(
    session: Session,
    job_id: UUID
) -> JobWithProgress:
    """
    Fetches job and computes progress from children if it's a parent job.
    """
    job = await session.get(Job, job_id)

    result = await session.exec(
        select(
            func.count(Job.id).label("total"),
            func.sum(case((Job.status == BackgroundJobStatus.COMPLETED, 1), else_=0)).label("completed"),
            func.sum(case((Job.status == BackgroundJobStatus.FAILED, 1), else_=0)).label("failed"),
            func.sum(case((Job.status == BackgroundJobStatus.CANCELLED, 1), else_=0)).label("cancelled"),
            func.sum(case((Job.status == BackgroundJobStatus.PROCESSING, 1), else_=0)).label("processing"),
        )
        .where(Job.parent_job_id == job_id)
    )
    counts = result.one()

    if counts.total > 0:
        # Parent job - return aggregated progress
        return JobWithProgress(
            job=job,
            progress_total=counts.total,
            progress_current=counts.completed + counts.failed + counts.cancelled,
            successful_items=counts.completed,
            failed_items=counts.failed,
            cancelled_items=counts.cancelled,
            processing_items=counts.processing,
        )
    else:
        # Regular job or child job - use stored values
        return JobWithProgress(
            job=job,
            progress_total=job.progress_total,
            progress_current=job.progress_current,
            successful_items=job.successful_items,
            failed_items=job.failed_items,
        )
```

## API Changes

### New Endpoint

```python
@router.get("/{job_id}/children", response_model=list[JobResponse])
async def get_job_children(
    job_id: UUID,
    session: Session,
    current_user: User,
    status: BackgroundJobStatus | None = None,
    limit: int = 50,
    offset: int = 0,
) -> list[JobResponse]:
    """Returns child jobs for a parent job with pagination and optional status filtering."""
```

### Modified Endpoints

```python
# GET /jobs/
# Now excludes child jobs: WHERE parent_job_id IS NULL

# POST /jobs/{job_id}/cancel
# If job has children, also cancels all non-terminal children

# DELETE /jobs/{job_id}
# CASCADE delete handles children automatically via FK relationship
```

## Frontend UI Changes

### Job List View

Child jobs hidden - only parent jobs shown in main list.

```
┌─────────────────────────────────────────────────────────────┐
│ Jobs                                                        │
├─────────────────────────────────────────────────────────────┤
│ ▶ Nexorious Import          Processing    245/500 (49%)    │
│   Steam Sync                 Completed     2 min ago        │
│   Epic Sync                  Completed     5 min ago        │
└─────────────────────────────────────────────────────────────┘
```

### Job Detail View

Shows aggregated progress and child jobs list.

```
┌─────────────────────────────────────────────────────────────┐
│ Nexorious Import                              [Cancel]      │
├─────────────────────────────────────────────────────────────┤
│ Status: Processing                                          │
│ Progress: 245/500 games (49%)                               │
│ ├─ Completed: 242                                           │
│ ├─ Failed: 3                                                │
│ └─ Processing: 5                                            │
│                                                             │
│ Started: 2 minutes ago                                      │
├─────────────────────────────────────────────────────────────┤
│ Child Jobs                              [Filter: All ▼]     │
├─────────────────────────────────────────────────────────────┤
│ ✓ The Witcher 3              Completed                      │
│ ✓ Cyberpunk 2077             Completed                      │
│ ✗ Some Obscure Game          Failed - IGDB not found        │
│ ◐ Elden Ring                 Processing                     │
│ ○ Hollow Knight              Pending                        │
│ ...                          (paginated)                    │
└─────────────────────────────────────────────────────────────┘
```

Key UI elements:
- Parent job shows aggregated progress bar
- Child jobs shown in expandable/scrollable list within detail view
- Filter dropdown: All / Completed / Failed / Processing / Pending
- Failed children show error message
- Pagination for large imports

## File Changes Summary

### Backend - Modify

- `app/models/job.py` - Add `parent_job_id` field and relationship
- `app/api/jobs.py` - Add `/children` endpoint, filter main list, update cancel logic
- `app/api/import_endpoints.py` - Update to enqueue coordinator task
- `app/services/batch_job_service.py` - Add progress aggregation method

### Backend - Create

- `app/worker/tasks/import_export/import_nexorious_coordinator.py` - Coordinator task
- `app/worker/tasks/import_export/import_nexorious_item.py` - Item processing task

### Backend - Delete

- `app/worker/tasks/import_export/import_nexorious.py` - Old monolithic task

### Frontend - Modify

- Job detail component - Add children list with pagination and filtering

### Migration

- New alembic migration for `parent_job_id` column

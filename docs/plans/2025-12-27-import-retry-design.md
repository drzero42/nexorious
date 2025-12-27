# Import Retry Mechanism Design

## Overview

When a JSON import runs, some items may fail due to transient issues (IGDB API timeouts, rate limiting, network hiccups). This design adds retry capabilities to recover from these failures.

### Features

1. **Automatic retry** — After all items in a job complete their first attempt, any failed items are automatically retried once. The job remains in "Processing" state during this retry phase, so users see it happening.

2. **Manual retry** — After the job completes, users can retry failed items from the UI:
   - "Retry All" button in the Failed section header
   - Individual "Retry" button on each failed item row

3. **Fresh start on retry** — When an item is retried, its error message is cleared and status resets to PENDING. No retry history is tracked.

4. **Unlimited manual retries** — Users can retry as many times as they want.

## Backend Changes

### Database & Models

Add a field to the Job model to track automatic retry state:

```python
auto_retry_done: bool = Field(default=False)
```

This boolean indicates whether the automatic retry has already been performed for a job.

**Migration:** Add `auto_retry_done` column with default `False`. Existing jobs unaffected.

### New Endpoints

#### POST /jobs/{job_id}/retry-failed

Retries all failed items in a job.

**Behavior:**
- Resets all FAILED items in the job to PENDING status
- Clears error_message, resets processed_at to null
- Re-enqueues each item to the task queue
- Returns: job with updated progress

**Validation:**
- Job must belong to current user
- Job must be in terminal state (COMPLETED, FAILED)
- Must have at least one FAILED item

#### POST /job-items/{item_id}/retry

Retries a single failed item.

**Behavior:**
- Resets the FAILED item to PENDING status
- Clears error_message, resets processed_at to null
- Re-enqueues the item to the task queue
- Returns: updated job item

**Validation:**
- Item must belong to current user
- Parent job must be in terminal state (COMPLETED, FAILED)
- Item must be in FAILED status

### Automatic Retry Logic

In `process_import_item.py`, modify the completion check that runs when all items are processed:

1. Check if `auto_retry_done` is `False`
2. Count FAILED items
3. If any failed items exist:
   - Set `auto_retry_done = True` on the job
   - Reset all failed items to PENDING (clear error_message, reset processed_at)
   - Re-enqueue each failed item to the task queue
   - Job stays in PROCESSING state
4. If `auto_retry_done` is `True` OR no failed items:
   - Mark job COMPLETED
   - Set `completed_at`

### Service Layer

Add to `job_service.py`:

```python
async def retry_failed_items(job_id: UUID, user_id: UUID) -> Job:
    """Reset all failed items to pending and re-enqueue them."""
    ...

async def retry_job_item(item_id: UUID, user_id: UUID) -> JobItem:
    """Reset a single failed item to pending and re-enqueue it."""
    ...
```

## Frontend Changes

### Location: Job Details Page — Failed Section

The Failed section already exists as an expandable accordion.

#### Failed Section Header

- Add "Retry All" button next to the item count
- Button disabled while any retry is in progress
- Example: `Failed (3) [Retry All]`

#### Failed Item Rows

- Add "Retry" button to each failed item row
- Button disabled while that item is being retried
- Show error message as before

### Optimistic Updates

When retry is triggered:

1. Items immediately move from Failed section to Processing
2. Failed count decreases, Processing count increases
3. Polling continues to track actual progress
4. Items land in Completed or back in Failed when done

### API Integration

- "Retry All" → `POST /jobs/{job_id}/retry-failed`
- Individual "Retry" → `POST /job-items/{item_id}/retry`
- Both trigger a refetch of job progress after the mutation

### Button States

- Default: enabled, shows "Retry" / "Retry All"
- Loading: disabled, shows spinner
- After success: item moves out of Failed section (optimistic)

## Error Handling & Edge Cases

### Edge Cases

1. **User triggers manual retry while automatic retry is running**
   - Not possible — job is still PROCESSING, manual retry endpoints require terminal state

2. **User triggers "Retry All" while individual retry is in progress**
   - Backend handles gracefully — only resets items currently in FAILED status
   - Items already reset to PENDING are skipped

3. **Concurrent manual retries on same item**
   - First request resets item to PENDING
   - Second request returns 400 (item not in FAILED status)

4. **All items fail on automatic retry**
   - Job completes with COMPLETED status (not FAILED)
   - Consistent with current behavior — partial success is still COMPLETED
   - Users can manually retry from there

5. **Job cancelled during automatic retry**
   - Existing cancellation logic applies
   - In-flight items complete, job marked CANCELLED
   - No further retries

### Error Responses

| Scenario | Status | Message |
|----------|--------|---------|
| Job not found | 404 | "Job not found" |
| Item not found | 404 | "Job item not found" |
| Job not terminal | 400 | "Job must be completed to retry items" |
| Item not failed | 400 | "Item is not in failed status" |
| No failed items to retry | 400 | "No failed items to retry" |

## Files to Change

### Backend

| File | Change |
|------|--------|
| `app/models/job.py` | Add `auto_retry_done: bool` field to Job model |
| `app/api/job_endpoints.py` | Add `POST /jobs/{job_id}/retry-failed` endpoint |
| `app/api/job_item_endpoints.py` | Add `POST /job-items/{item_id}/retry` endpoint |
| `app/services/job_service.py` | Add `retry_failed_items()` and `retry_job_item()` methods |
| `app/worker/tasks/import_export/process_import_item.py` | Add automatic retry logic in completion check |
| Alembic migration | Add `auto_retry_done` column |

### Frontend

| File | Change |
|------|--------|
| `src/api/jobs.ts` | Add `retryFailedItems()` and `retryJobItem()` API functions |
| Failed section component | Add "Retry All" button to header |
| Job item row component | Add "Retry" button to failed items |
| React Query hooks | Add mutations for retry endpoints with optimistic updates |

### Tests

- Backend: Test retry endpoints, automatic retry logic, edge cases
- Frontend: Test button states, optimistic updates, error handling

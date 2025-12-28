# Fix Sync Job Completion After Resolving Items

## Problem

When performing a Steam sync, after resolving all matches that need user input, the sync job keeps showing "Syncing..." but never completes. The frontend shows the sync as still running even though all items have been resolved.

### Root Cause

Two bugs in `backend/app/api/job_items.py`:

1. **`resolve_job_item` endpoint** (lines 35-56):
   - Marks item `COMPLETED` directly without re-queuing the worker task
   - The game is never actually imported (worker Flow B never runs)
   - Job completion check is never called

2. **`skip_job_item` endpoint** (lines 59-81):
   - Marks item `SKIPPED` correctly
   - But never calls the job completion check

The worker task `process_sync_item` has "Flow B" logic that correctly handles `resolved_igdb_id` - it just never gets invoked.

## Solution

### For `resolve_job_item`:
1. Set `resolved_igdb_id` on the item
2. Reset item status to `PENDING` (so worker picks it up)
3. Re-queue the `process_sync_item` worker task
4. Worker runs Flow B → imports game → marks `COMPLETED` → checks job completion

### For `skip_job_item`:
1. Mark item as `SKIPPED` (current behavior)
2. **Add**: Call `_check_and_update_job_completion()` to transition job if all items are now terminal

This matches the pattern used in the existing `retry_job_item` endpoint.

## Implementation

### File: `backend/app/api/job_items.py`

**New imports:**
```python
from ..worker.tasks.sync.process_item import _check_and_update_job_completion
from ..worker.broker import enqueue_task
```

**Changes to `resolve_job_item`:**
- Set `resolved_igdb_id` on item
- Reset status to `PENDING`
- Remove `resolved_at` (worker sets `processed_at`)
- Re-queue `process_sync_item` task

**Changes to `skip_job_item`:**
- After commit, call `_check_and_update_job_completion(session, item.job_id)`

### Tests

Update `backend/app/tests/test_job_items_api.py`:
1. Verify `resolve` re-queues worker task (mock enqueue)
2. Verify `skip` triggers job completion check
3. Verify job transitions to `COMPLETED` when last item is skipped

## Verification

After fix:
1. Start a Steam sync with items that need review
2. Resolve or skip all `PENDING_REVIEW` items
3. Job should transition to `COMPLETED`
4. Frontend should show sync as complete (not "Syncing...")

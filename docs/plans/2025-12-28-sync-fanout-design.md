# Sync Fan-out Design

This document describes the refactoring of the sync system to use per-game tasks for parallel processing.

## Overview

The current Steam sync processes games sequentially in a single monolithic task. This design refactors sync to fan-out individual games as separate tasks, enabling parallel processing while respecting IGDB rate limits via the distributed rate limiter.

## Architecture

### Two-Phase Processing

**Phase 1: Fan-out Task (`dispatch_sync_items`)**
- Fetches user's game library via source adapter (Steam, Epic, etc.)
- Creates a JobItem for each game (streaming insert)
- Dispatches a processing task for each JobItem immediately after creation
- Job status remains PROCESSING until all items complete
- Completes quickly - only doing external API call + DB inserts + task dispatches

**Phase 2: Per-Game Worker Tasks (`process_sync_item`)**
- Each task processes a single JobItem
- Uses distributed rate limiter for IGDB API coordination
- After processing, checks if all siblings are done and marks Job COMPLETED
- Updates `last_synced_at` only when Job completes

### Source Adapter Abstraction

Generic sync processing with source-specific adapters:

```
Source Adapter (Steam, Epic, GOG, etc.)
    | returns: List[ExternalGame]
    |   - external_id: str (e.g., Steam AppID)
    |   - title: str
    |   - platform_id: str (e.g., "pc-windows")
    |   - storefront_id: str (e.g., "steam")
    |   - metadata: dict (playtime, etc.)
    v
Generic Fan-out Task
    | creates JobItems with standardized format
    v
Generic Worker Task
    | processes using storefront_id/external_id
    | checks UserGamePlatform by storefront_id + external_id
    | creates associations with correct platform_id + storefront_id
```

The Job's `source` field (BackgroundJobSource enum) tracks the origin for UI filtering.

## Worker Task Flows

### Flow A: New Item (no `resolved_igdb_id`)

1. Check if already synced (external_id exists in UserGamePlatform) -> COMPLETED
2. Check if ignored -> SKIPPED
3. Call MatchingService (IGDB API) -> based on result:
   - High confidence match -> import/link -> COMPLETED
   - Low confidence / multiple candidates -> PENDING_REVIEW (task exits)
   - No match -> PENDING_REVIEW (task exits)

### Flow B: Reviewed Item (has `resolved_igdb_id`)

1. Check if already synced -> COMPLETED
2. Check if game exists in user's collection (UserGame with game_id = resolved_igdb_id):
   - Yes -> Add platform association to existing UserGame -> COMPLETED
   - No -> Create UserGame + add platform association -> COMPLETED

### Status Mapping

| Outcome | JobItem Status |
|---------|----------------|
| Already synced | COMPLETED |
| Ignored by user | SKIPPED |
| Matched + auto-linked | COMPLETED |
| Matched + needs review | PENDING_REVIEW |
| No match found | PENDING_REVIEW |
| Error during processing | FAILED |

### PENDING_REVIEW Flow

PENDING_REVIEW items wait for user input:

1. Task processes item -> determines it needs review -> sets PENDING_REVIEW -> task exits
2. User selects IGDB match via UI -> API updates `resolved_igdb_id` -> sets status to PENDING -> dispatches new task
3. Task processes item again -> follows Flow B -> COMPLETED

## Queue Simplification

### Current State

Multiple subjects per task type with duplicate task functions:
- `tasks.high.import`, `tasks.high.sync`, `tasks.high.export`
- `tasks.low.import`, `tasks.low.sync`, `tasks.low.maintenance`

### New State

Two priority-based subjects:

```python
SUBJECT_HIGH = "tasks.high"
SUBJECT_LOW = "tasks.low"
```

Tasks have unique names, priority determined at dispatch time:

```python
@broker.task(task_name="import.process_item")
async def process_import_item(job_item_id: str) -> dict:
    ...

@broker.task(task_name="sync.dispatch")
async def dispatch_sync_items(job_id: str, user_id: str, source: str) -> dict:
    ...

@broker.task(task_name="sync.process_item")
async def process_sync_item(job_item_id: str) -> dict:
    ...
```

Dispatch helper:

```python
async def enqueue_task(task_func, *args, priority: BackgroundJobPriority):
    subject = SUBJECT_HIGH if priority == BackgroundJobPriority.HIGH else SUBJECT_LOW
    await task_func.kicker().with_labels(subject=subject).kiq(*args)
```

## Data Structures

### ExternalGame Dataclass

```python
@dataclass
class ExternalGame:
    external_id: str        # e.g., "12345" (Steam AppID)
    title: str              # Game name from source
    platform_id: str        # e.g., "pc-windows"
    storefront_id: str      # e.g., "steam"
    metadata: dict          # Source-specific data (playtime, etc.)
```

### SyncSourceAdapter Protocol

```python
class SyncSourceAdapter(Protocol):
    source: BackgroundJobSource  # STEAM, EPIC, GOG, etc.

    async def fetch_games(self, user: User) -> List[ExternalGame]:
        """Fetch all games from external source for user."""
        ...

    def get_credentials(self, user: User) -> Optional[dict]:
        """Extract credentials from user preferences."""
        ...

    def is_configured(self, user: User) -> bool:
        """Check if user has valid credentials for this source."""
        ...
```

### JobItem source_metadata_json Format

```json
{
  "external_id": "12345",
  "platform_id": "pc-windows",
  "storefront_id": "steam",
  "metadata": {
    "playtime_minutes": 120
  }
}
```

## Job Completion

The `_check_and_update_job_completion` function is extended to update `last_synced_at`:

```python
def _check_and_update_job_completion(session: Session, job_id: str) -> tuple[bool, list[str]]:
    # ... existing logic: count pending/processing items
    # ... existing logic: auto-retry failed items if not done

    # When job completes:
    job.status = BackgroundJobStatus.COMPLETED
    job.completed_at = datetime.now(timezone.utc)
    session.add(job)

    # If this is a SYNC job, update last_synced_at on the sync config
    if job.job_type == BackgroundJobType.SYNC:
        _update_sync_config_timestamp(session, job.user_id, job.source)

    session.commit()
    return True, []
```

## File Structure

### New Files

```
backend/app/worker/tasks/sync/
├── __init__.py
├── adapters/
│   ├── __init__.py
│   ├── base.py              # ExternalGame dataclass, SyncSourceAdapter protocol
│   └── steam.py             # SteamSyncAdapter
├── dispatch.py              # dispatch_sync_items task (fan-out)
└── process_item.py          # process_sync_item task (worker)
```

### Modified Files

```
backend/app/worker/queues.py                                   # Simplify to SUBJECT_HIGH, SUBJECT_LOW
backend/app/worker/tasks/import_export/process_import_item.py  # Single task function, priority at dispatch
backend/app/worker/tasks/import_export/export.py               # Update dispatch to use priority helper
backend/app/worker/tasks/import_export/import_nexorious.py     # Update dispatch calls
backend/app/worker/tasks/sync/check_pending.py                 # Update to dispatch new fan-out task
```

### Removed Files

```
backend/app/worker/tasks/sync/steam.py    # Replaced by generic dispatch + adapter
```

## Benefits

- **Parallel processing** - Multiple games processed concurrently, limited by distributed rate limiter
- **Immediate progress** - Users see items appearing as they're dispatched (streaming)
- **Resilient** - Partial failures don't lose work; each item is independent
- **Extensible** - Easy to add Epic/GOG adapters following the same pattern
- **Simpler queues** - Two priority levels instead of per-task-type subjects

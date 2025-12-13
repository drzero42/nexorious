# Sync, Import, and Export Task Separation Design

**Date:** 2025-12-13
**Status:** Draft

## Overview

This design separates background tasks into distinct categories: **sync** (recurring platform updates), **import** (one-off data ingestion), and **export** (data backup). It builds on the [Background Task System Design](2025-12-13-background-task-system-design.md) and unifies the job model across all task types.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Queue structure | Two priority queues (high/low) | User-initiated tasks don't block on scheduled tasks |
| Priority model | User-initiated = high, automated = low | Users expect responsive feedback on manual actions |
| Review queue | Single unified queue | Simpler UX; games from any source reviewed in one place |
| Matching service | Shared across all imports/syncs | DRY; consistent matching logic regardless of source |
| Nexorious import | Non-interactive | Trusted source with exact IGDB IDs |
| Darkadia import | Interactive with review | Title-based matching requires user verification |
| Sync conflicts | Flag removals for review | User stays in control of their collection |
| Real-time updates | WebSocket for all operations | Consistent UX across imports and syncs |
| Job visibility | Single unified job list | One place to see all task history |
| Sync configuration | Centralized settings page | Easy to manage all platforms in one view |

## Task Architecture

### Priority Queues

Two taskiq queues, both using PostgreSQL:

| Queue | Priority | Purpose |
|-------|----------|---------|
| `high` | User-initiated | Manual sync, imports, manual exports |
| `low` | Automated | Scheduled syncs, scheduled exports, maintenance |

### Task Categories

1. **Sync Tasks** - Update user's collection from external platforms
   - Steam sync (uses Steam AppID for fast DB lookup)
   - Future: Epic Games, GOG, etc. (title-based matching)
   - Can be user-initiated (high priority) or scheduled (low priority)

2. **Import Tasks** - One-off data ingestion
   - Nexorious JSON import (non-interactive, trusted IGDB IDs)
   - Darkadia CSV import (interactive, needs title-based matching)
   - Always user-initiated (high priority)

3. **Export Tasks** - Data backup/portability
   - Manual exports (high priority)
   - Scheduled backup exports (low priority, future)

4. **Maintenance Tasks** - System housekeeping
   - Cleanup old task results
   - Cleanup expired export files
   - Always low priority

## Matching System

### Matching Service

Shared logic used by both syncs and imports to resolve games to IGDB IDs.

### Matching Flow (in order)

1. **IGDB ID present in source data** - Matched immediately (Nexorious JSON exports)
2. **Platform-specific ID lookup** - Check Nexorious DB for existing game with that ID:
   - Steam AppID - If game exists in DB, use its IGDB ID - Matched
   - Future: Epic Game ID, GOG ID, etc.
3. **Title-based IGDB search** - Query IGDB API with title (+ optional metadata like release year, developer)
   - High confidence match - Auto-match
   - Multiple candidates or low confidence - Queue for user review

### Review Queue

- Single unified queue at `/review`
- Shows all unmatched games from any source (imports, syncs)
- Each item displays:
  - Game title from source
  - Source context (e.g., "Steam Sync", "Darkadia Import")
  - IGDB search interface with candidates
  - Skip/Match actions
- User decisions save immediately (WebSocket updates)

### Sync Conflict Handling

- When a game disappears from platform library (refund, lost license)
- Flagged in review queue as "removal detected"
- User decides: keep in collection or remove

## Sync System

### User Configuration

- Centralized at `/settings/sync`
- Per-platform settings in a table view:
  - Platform name (Steam, Epic, etc.)
  - Connection status (connected/not connected)
  - Sync frequency dropdown: "Manual only", "Hourly", "Daily", "Weekly"
  - Last sync timestamp
  - "Sync Now" button (triggers high-priority task)

### Sync Behavior

- **Auto-add mode** vs **review mode**: Configurable per-user
  - Auto-add: Matched games added directly to collection
  - Review mode: Matched games queued for user confirmation before adding

### Sync Task Flow

1. Scheduler checks which users need syncing (based on frequency + last sync time)
2. Enqueues individual sync tasks per user/platform (low priority queue)
3. Sync task pulls library from platform API
4. For each game:
   - Run through matching service
   - If matched + auto-add enabled - Add to collection
   - If matched + review mode - Queue for review
   - If unmatched - Queue for review
   - If removal detected - Queue for review
5. Update last sync timestamp
6. Send WebSocket notification if user is online

### Manual "Sync Now"

- Same flow but uses high-priority queue
- User sees real-time progress via WebSocket

## Import System

### Import Types

| Source | Interactive | Matching Approach |
|--------|-------------|-------------------|
| Nexorious JSON | No | Trusted IGDB IDs, direct import |
| Darkadia CSV | Yes | Title-based IGDB search + user review |
| Steam (initial) | Yes | Steam AppID DB lookup - title search - user review |

### Nexorious JSON Import Flow

1. User uploads JSON export file
2. Task validates schema and `export_version`
3. For each game:
   - Look up IGDB ID in DB (may need to fetch metadata if game not cached)
   - Restore user data (play status, rating, notes, tags, platforms)
4. Import completes, show summary
5. No review needed - fully automated

### Darkadia/Steam Import Flow

1. User uploads file / initiates Steam import
2. Task parses source data
3. For each game:
   - Run through matching service
   - Matched - Ready for import
   - Unmatched - Queue for user review
4. If any unmatched: Job status - `awaiting_review`
5. User resolves unmatched games in review queue
6. User confirms final import
7. Matched games added to collection
8. WebSocket updates throughout

### Import Job States

- `pending` - Job created, not started
- `processing` - Parsing and matching in progress
- `awaiting_review` - Has unmatched games needing user input
- `ready` - All games matched, awaiting user confirmation
- `finalizing` - Adding games to collection
- `completed` - Done
- `failed` - Error occurred
- `cancelled` - User cancelled

## Job Management & History

### Unified Job List

- Single page at `/jobs`
- Shows all task types: imports, exports, syncs
- Filterable by:
  - Type (import, export, sync)
  - Status (pending, processing, completed, failed)
  - Source (Steam, Darkadia, Nexorious, etc.)

### Job List Display

| Column | Example |
|--------|---------|
| Type | Import / Export / Sync |
| Source | Steam, Darkadia, Nexorious |
| Status | Completed / Processing / Failed / Awaiting Review |
| Started | 2025-12-13 10:30 |
| Duration | 2m 34s |
| Result | "Added 42 games" / "Exported 2.4 MB" |
| Actions | View Details / Download / Cancel / Retry |

### Job Detail View

- Full progress history
- For imports/syncs: breakdown of matched/unmatched/skipped games
- For exports: download link (if completed)
- Error details (if failed)
- Link to review queue (if `awaiting_review`)

### Job Retention

- Completed jobs: Keep for 30 days
- Failed jobs: Keep for 7 days (user can retry or delete)
- Export files: Keep for 7 days (per existing export design)

## Real-Time Updates (WebSocket)

### Connection Architecture

- Single WebSocket connection per authenticated user
- Connects when user is on relevant pages (jobs, review, import status)
- Auto-reconnect with exponential backoff

### Event Types (Backend to Frontend)

| Event | Payload | When |
|-------|---------|------|
| `job_created` | Job ID, type, source | New job starts |
| `job_progress` | Job ID, processed/total, current item | During processing |
| `job_status_change` | Job ID, old status, new status | State transitions |
| `job_completed` | Job ID, summary stats | Task finishes |
| `job_failed` | Job ID, error message | Error occurs |
| `review_item_added` | Game title, source, job ID | New item needs review |
| `review_item_resolved` | Item ID, resolution | User matched/skipped |

### Event Types (Frontend to Backend)

| Event | Payload | When |
|-------|---------|------|
| `subscribe_job` | Job ID | Start watching specific job |
| `unsubscribe_job` | Job ID | Stop watching job |
| `resolve_review_item` | Item ID, IGDB ID or "skip" | User makes matching decision |
| `confirm_import` | Job ID | User confirms final import |
| `cancel_job` | Job ID | User cancels in-progress job |

### Fallback

- If WebSocket unavailable, frontend polls `/api/jobs/{id}` every 3 seconds
- Graceful degradation, same UX just slightly delayed

## Project Structure

```
backend/app/worker/
├── __init__.py
├── broker.py                    # Broker config with two queues
├── queues.py                    # Queue definitions (high, low)
├── schedules.py                 # Cron schedule definitions
└── tasks/
    ├── __init__.py
    ├── sync/
    │   ├── __init__.py
    │   ├── steam.py             # Steam sync task
    │   ├── base.py              # Shared sync logic
    │   └── scheduler.py         # Fan-out: check who needs syncing
    ├── import_export/
    │   ├── __init__.py
    │   ├── import_nexorious.py  # Nexorious JSON import
    │   ├── import_darkadia.py   # Darkadia CSV import
    │   ├── import_steam.py      # Steam initial import
    │   └── export.py            # Export tasks (JSON/CSV)
    └── maintenance/
        ├── __init__.py
        ├── cleanup_results.py   # Old task result cleanup
        └── cleanup_exports.py   # Expired export file cleanup

backend/app/services/
├── matching/
│   ├── __init__.py
│   ├── service.py               # Shared matching service
│   ├── igdb_lookup.py           # IGDB title search
│   └── platform_lookup.py       # Steam AppID / Epic ID lookups
└── ... (existing services)
```

### Queue Configuration

```python
# app/worker/queues.py
QUEUE_HIGH = "high"    # User-initiated tasks
QUEUE_LOW = "low"      # Scheduled/automated tasks
```

## Database Models

### Job Model (unified)

```python
class JobType(str, Enum):
    SYNC = "sync"
    IMPORT = "import"
    EXPORT = "export"

class JobSource(str, Enum):
    STEAM = "steam"
    EPIC = "epic"          # Future
    DARKADIA = "darkadia"
    NEXORIOUS = "nexorious"
    SYSTEM = "system"      # Maintenance tasks

class JobStatus(str, Enum):
    PENDING = "pending"
    PROCESSING = "processing"
    AWAITING_REVIEW = "awaiting_review"
    READY = "ready"
    FINALIZING = "finalizing"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"

class Job(SQLModel, table=True):
    id: UUID
    user_id: UUID
    job_type: JobType
    source: JobSource
    status: JobStatus
    priority: str                  # "high" or "low"
    progress_current: int          # Items processed
    progress_total: int            # Total items
    result_summary: Optional[dict] # Stats on completion
    error_message: Optional[str]
    file_path: Optional[str]       # For exports
    created_at: datetime
    started_at: Optional[datetime]
    completed_at: Optional[datetime]
```

### Review Item Model

```python
class ReviewItemStatus(str, Enum):
    PENDING = "pending"
    MATCHED = "matched"
    SKIPPED = "skipped"
    REMOVAL = "removal"           # Sync detected game removed from platform

class ReviewItem(SQLModel, table=True):
    id: UUID
    job_id: UUID                  # Link to originating job
    user_id: UUID
    status: ReviewItemStatus
    source_title: str             # Title from source
    source_metadata: dict         # Platform ID, release year, etc.
    igdb_candidates: list[dict]   # Search results for user to pick from
    resolved_igdb_id: Optional[int]
    created_at: datetime
    resolved_at: Optional[datetime]
```

### User Sync Config Model

```python
class SyncFrequency(str, Enum):
    MANUAL = "manual"
    HOURLY = "hourly"
    DAILY = "daily"
    WEEKLY = "weekly"

class UserSyncConfig(SQLModel, table=True):
    id: UUID
    user_id: UUID
    platform: str                 # "steam", "epic", etc.
    frequency: SyncFrequency
    auto_add: bool                # True = add matched games automatically
    last_synced_at: Optional[datetime]
    enabled: bool
```

## API Endpoints

### Job Management

```
GET    /api/jobs                     # List user's jobs (filterable)
GET    /api/jobs/{job_id}            # Job details
POST   /api/jobs/{job_id}/cancel     # Cancel in-progress job
DELETE /api/jobs/{job_id}            # Delete job record
```

### Review Queue

```
GET    /api/review                   # List pending review items
GET    /api/review/{item_id}         # Item details with IGDB candidates
POST   /api/review/{item_id}/match   # Match to IGDB ID
POST   /api/review/{item_id}/skip    # Skip this game
POST   /api/review/{item_id}/keep    # Keep game (for removal detections)
POST   /api/review/{item_id}/remove  # Remove game (for removal detections)
POST   /api/jobs/{job_id}/confirm    # Confirm import after review complete
```

### Sync

```
GET    /api/sync/config              # Get user's sync settings
PUT    /api/sync/config/{platform}   # Update platform sync settings
POST   /api/sync/{platform}          # Trigger manual sync (high priority)
```

### Import

```
POST   /api/import/nexorious         # Upload Nexorious JSON
POST   /api/import/darkadia          # Upload Darkadia CSV
POST   /api/import/steam             # Initiate Steam import
```

### Export (existing design)

```
POST   /api/export/collection/json   # Start collection JSON export
POST   /api/export/collection/csv    # Start collection CSV export
POST   /api/export/wishlist/json     # Start wishlist JSON export
POST   /api/export/wishlist/csv      # Start wishlist CSV export
GET    /api/export/{job_id}/download # Download completed export
```

### WebSocket

```
WS     /api/ws/jobs                  # Real-time job updates
```

## Frontend Pages

### Routes

| Route | Purpose |
|-------|---------|
| `/settings/sync` | Sync configuration (platform list, frequencies) |
| `/jobs` | Unified job history list |
| `/jobs/{job_id}` | Job detail view with progress |
| `/review` | Unified review queue for unmatched games |
| `/import` | Import landing page (choose source) |
| `/import/nexorious` | Nexorious JSON upload |
| `/import/darkadia` | Darkadia CSV upload |
| `/import/steam` | Steam import flow |

### Components

```
frontend/src/lib/components/
├── jobs/
│   ├── JobList.svelte           # Filterable job table
│   ├── JobCard.svelte           # Individual job summary
│   ├── JobProgress.svelte       # Real-time progress bar
│   └── JobDetail.svelte         # Full job details
├── review/
│   ├── ReviewQueue.svelte       # List of items needing review
│   ├── ReviewItem.svelte        # Single game to match
│   ├── IGDBSearch.svelte        # Search interface with candidates
│   └── RemovalReview.svelte     # Special UI for removal detections
├── sync/
│   ├── SyncConfigTable.svelte   # Platform settings table
│   └── SyncButton.svelte        # Manual "Sync Now" with status
└── import/
    ├── FileUpload.svelte        # Drag-drop file upload
    ├── ImportProgress.svelte    # Real-time import status
    └── ImportSummary.svelte     # Results after completion
```

### Stores

```typescript
// stores/jobs.ts
export const jobsStore = writable<Job[]>([]);
export const activeJobStore = writable<Job | null>(null);

// stores/review.ts
export const reviewItemsStore = writable<ReviewItem[]>([]);
export const reviewCountStore = derived(reviewItemsStore, items =>
    items.filter(i => i.status === 'pending').length
);

// stores/websocket.ts
export const wsConnectionStore = writable<WebSocket | null>(null);
export const wsStatusStore = writable<'connected' | 'disconnected' | 'reconnecting'>('disconnected');
```

## Differences from Existing Designs

### vs. Background Task System Design

| Aspect | Original | This Design |
|--------|----------|-------------|
| Queue structure | Single queue implied | Two priority queues (high/low) |
| Task organization | `sync.py`, `maintenance.py`, `reports.py` | `sync/`, `import_export/`, `maintenance/` folders |
| Sync tasks | `sync_steam_library` basic example | Full sync system with user config, auto-add vs review mode |

### vs. Steam Import Flowchart

| Aspect | Original | This Design |
|--------|----------|-------------|
| Review queue | Per-import-job review flow | Unified review queue across all jobs |
| Matching logic | Inline in Steam import | Extracted to shared matching service |
| Job model | Import-specific states | Unified `Job` model for all task types |

### vs. Export Design

| Aspect | Original | This Design |
|--------|----------|-------------|
| Job model | Separate `ExportJob` | Unified `Job` model with `job_type=export` |
| Job list | Export-specific list | Part of unified job history |
| Priority | Not specified | High (manual) or low (scheduled) |

## Migration Notes

- Existing `ExportJob` model - migrate to unified `Job` model
- Existing `BatchSession` - replaced by `Job` + `ReviewItem`
- Steam import flow - refactor to use shared matching service

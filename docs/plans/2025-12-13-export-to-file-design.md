# Export to File Feature Design

**Date:** 2025-12-13
**Status:** Approved

## Overview

Add manual export functionality to Nexorious, allowing users to back up their game collection and wishlist data. Exports are processed as background jobs and saved to disk for download.

## Goals

- **Primary use case:** Data backup and portability
- **Secondary use case:** Enable external backup tools to pick up exported files from disk

## Scope

**In scope:**
- Manual exports triggered via UI
- Collection and wishlist exports (separate)
- JSON format (for re-import) and CSV format (for portability)
- Per-user exports
- Admin system-wide exports (all users)
- Background job processing with download links

**Out of scope (deferred):**
- Scheduled/automated exports
- Re-import functionality (design JSON to support it, but don't build UI)
- Filtered exports (always export everything)

## Data Schemas

### JSON Export (for re-import into Nexorious)

Lean format containing only user-specific data. IGDB metadata is not included since it can be re-fetched.

```json
{
  "export_version": "1.0",
  "exported_at": "2025-12-13T10:30:00Z",
  "user": { "id": "uuid", "username": "johndoe" },
  "games": [
    {
      "igdb_id": 12345,
      "ownership_status": "owned",
      "play_status": "completed",
      "personal_rating": 4.5,
      "is_loved": true,
      "personal_notes": "Rich text notes...",
      "hours_played": 42,
      "acquired_date": "2024-06-15",
      "platforms": [
        {
          "platform_id": "uuid",
          "storefront_id": "uuid",
          "store_game_id": "123",
          "store_url": "https://...",
          "is_available": true,
          "original_platform_name": null
        }
      ],
      "tags": ["tag-uuid-1", "tag-uuid-2"],
      "created_at": "2024-01-15T08:00:00Z",
      "updated_at": "2024-06-20T14:30:00Z"
    }
  ],
  "tags": [
    { "id": "uuid", "name": "RPG", "color": "#ff5500", "description": "Role-playing games" }
  ],
  "wishlist": [
    { "igdb_id": 67890, "created_at": "2024-03-10T12:00:00Z" }
  ]
}
```

### CSV Export (for portability)

Fully denormalized with all IGDB metadata included. Self-contained and human-readable.

| Column | Example |
|--------|---------|
| igdb_id | 12345 |
| title | The Witcher 3 |
| genre | RPG |
| developer | CD Projekt Red |
| publisher | CD Projekt |
| release_date | 2015-05-19 |
| description | Open world RPG... |
| hltb_main | 50 |
| hltb_extra | 100 |
| hltb_completionist | 180 |
| ownership_status | owned |
| play_status | completed |
| personal_rating | 4.5 |
| is_loved | true |
| hours_played | 42 |
| acquired_date | 2024-06-15 |
| personal_notes | Great game... |
| platforms | Steam (Windows), GOG (Windows) |
| tags | RPG, Favorite |
| created_at | 2024-01-15T08:00:00Z |
| updated_at | 2024-06-20T14:30:00Z |

### Wishlist CSV

| Column | Example |
|--------|---------|
| igdb_id | 67890 |
| title | Elden Ring |
| genre | Action RPG |
| developer | FromSoftware |
| publisher | Bandai Namco |
| release_date | 2022-02-25 |
| description | ... |
| hltb_main | 55 |
| hltb_extra | 98 |
| hltb_completionist | 133 |
| created_at | 2024-03-10T12:00:00Z |

## Backend Architecture

### API Endpoints

**User endpoints:**
```
POST /api/export/collection/json    → Start collection JSON export job
POST /api/export/collection/csv     → Start collection CSV export job
POST /api/export/wishlist/json      → Start wishlist JSON export job
POST /api/export/wishlist/csv       → Start wishlist CSV export job

GET  /api/export/jobs               → List user's export jobs
GET  /api/export/jobs/{job_id}      → Get job status
GET  /api/export/jobs/{job_id}/download → Download completed export file

DELETE /api/export/jobs/{job_id}    → Cancel/delete export job
```

**Admin endpoints (system-wide backup):**
```
POST /api/admin/export/all-users    → Export all users' data
GET  /api/admin/export/jobs         → List all export jobs
```

### Export Job Model

```python
class ExportJobStatus(str, Enum):
    PENDING = "pending"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    EXPIRED = "expired"

class ExportType(str, Enum):
    COLLECTION = "collection"
    WISHLIST = "wishlist"
    ALL_USERS = "all_users"

class ExportFormat(str, Enum):
    JSON = "json"
    CSV = "csv"

class ExportJob(SQLModel, table=True):
    id: str                           # UUID
    user_id: Optional[str]            # Owner (null for admin exports)
    export_type: ExportType
    format: ExportFormat
    status: ExportJobStatus
    file_path: Optional[str]          # Path to generated file on disk
    file_size: Optional[int]          # Size in bytes
    error_message: Optional[str]
    created_at: datetime
    completed_at: Optional[datetime]
    expires_at: Optional[datetime]    # Auto-cleanup time
```

### File Storage

**Configuration:**
- `EXPORT_PATH` env var (default: `./storage/exports/`)
- `EXPORT_RETENTION_DAYS` env var (default: 7)

**Filename format:**
- User exports: `nexorious_{username}_{export_type}_{timestamp}.{json|csv}`
- Admin exports: `nexorious_all_users_{timestamp}.{json|csv}`

**Examples:**
- `nexorious_johndoe_collection_2025-12-13T103000Z.json`
- `nexorious_johndoe_wishlist_2025-12-13T103000Z.csv`
- `nexorious_all_users_2025-12-13T103000Z.json`

### Background Job Processing

- Use FastAPI's `BackgroundTasks` for async execution
- Job writes file to disk, updates `ExportJob` status when complete
- No external task queue needed (Celery/Redis)

### File Cleanup

- Exports expire after `EXPORT_RETENTION_DAYS`
- Cleanup runs on app startup + daily periodic task
- Deletes expired files from disk and marks jobs as `expired`

### Error Handling

- Job catches exceptions, sets `status=failed` with `error_message`
- Users can retry failed jobs or delete them

### Security

- Users can only access their own exports
- Download endpoint verifies `job.user_id == current_user.id`
- Admin exports require admin role

## Frontend UI

### Location

Settings/Account page (`/settings`)

### Layout

```
┌─────────────────────────────────────────────────┐
│ Export Your Data                                │
├─────────────────────────────────────────────────┤
│                                                 │
│ Collection                                      │
│ ┌─────────────┐  ┌─────────────┐               │
│ │ Export JSON │  │ Export CSV  │               │
│ └─────────────┘  └─────────────┘               │
│                                                 │
│ Wishlist                                        │
│ ┌─────────────┐  ┌─────────────┐               │
│ │ Export JSON │  │ Export CSV  │               │
│ └─────────────┘  └─────────────┘               │
│                                                 │
├─────────────────────────────────────────────────┤
│ Recent Exports                                  │
│ ┌───────────────────────────────────────────┐  │
│ │ nexorious_john_collection_...json         │  │
│ │ ✓ Completed · 2.4 MB · [Download] [Delete]│  │
│ ├───────────────────────────────────────────┤  │
│ │ nexorious_john_wishlist_...csv            │  │
│ │ ✗ Failed: Database error · [Retry] [Delete]  │
│ ├───────────────────────────────────────────┤  │
│ │ nexorious_john_collection_...csv          │  │
│ │ ⟳ Processing...                [Cancel]   │  │
│ └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

### Behavior

1. User clicks export button → POST to start job → button shows spinner
2. Job appears in "Recent Exports" list
3. Frontend polls `GET /api/export/jobs/{id}` every 2-3 seconds
4. When complete → "Download" and "Delete" buttons appear
5. Failed jobs show error message with "Retry" and "Delete" options
6. Processing jobs show "Cancel" option

### Admin Panel

Separate section in admin settings:
- "Export All Users" button (JSON and CSV options)
- Job list showing system-wide exports

## Implementation Notes

### CSV Generation
- Use Python's built-in `csv` module (no extra dependencies)
- Stream rows to file to handle large collections efficiently

### JSON Generation
- Build dict in memory, serialize with `json.dumps(indent=2)`
- For very large exports, consider streaming with `ijson` (defer unless needed)

### Polling vs SSE
- Start with polling (simpler)
- Could add Server-Sent Events later if polling becomes problematic

## Future Considerations (Out of Scope)

- **Scheduled exports:** Cron-based automatic exports to disk
- **Re-import UI:** Import from JSON backup to restore collection
- **Filtered exports:** Export subset based on tags, platform, play status
- **Additional formats:** Excel (XLSX), Darkadia-compatible CSV

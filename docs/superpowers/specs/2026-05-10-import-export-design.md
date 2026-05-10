# Import & Export — Design Spec

## Overview

Implements the remaining Phase 3 import/export endpoints for the nexorious Go port. Three handler endpoints (`POST /api/import/nexorious`, `POST /api/export/json`, `POST /api/export/csv`) plus a download endpoint (`GET /api/export/:id/download`). All are JWT-required. Import uses fan-out (one worker task per game); export uses a single task per job.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/import/nexorious` | Upload nexorious JSON export file, create import job |
| `POST` | `/api/export/json` | Start JSON export job |
| `POST` | `/api/export/csv` | Start CSV export job |
| `GET` | `/api/export/:id/download` | Download completed export file |

## Nexorious JSON Format (v1.2)

The canonical exchange format. Export produces it; import consumes it. Matches the Python version exactly.

### Top-Level Structure

```json
{
  "export_version": "1.2",
  "export_date": "2026-05-10T12:00:00Z",
  "user_id": "abc-123",
  "total_games": 42,
  "total_wishlist": 0,
  "export_stats": {
    "by_status": { "playing": 5, "completed": 20, "backlog": 17 },
    "by_platform": { "pc": 30, "playstation": 12 },
    "total_hours": 1234,
    "rated_count": 25,
    "loved_count": 8
  },
  "games": [ "...see below..." ],
  "wishlist": []
}
```

### Game Object

```json
{
  "igdb_id": 1942,
  "title": "The Witcher 3: Wild Hunt",
  "release_year": 2015,
  "play_status": "completed",
  "personal_rating": 9,
  "is_loved": true,
  "hours_played": 120,
  "personal_notes": "Best RPG ever",
  "platforms": [
    {
      "platform_id": "pc",
      "platform_name": "PC",
      "storefront_id": "steam",
      "storefront_name": "Steam",
      "store_game_id": "292030",
      "store_url": null,
      "is_available": true,
      "hours_played": 120,
      "ownership_status": "owned",
      "acquired_date": "2015-05-19"
    }
  ],
  "tags": [
    { "name": "RPG", "color": "#ff5500" }
  ],
  "created_at": "2024-01-15T10:00:00Z",
  "updated_at": "2024-06-01T14:30:00Z"
}
```

### Wishlist

The Go port has no wishlist table. Export includes `wishlist` as an empty array for format compatibility. Import silently skips any `wishlist` entries.

## Import (`POST /api/import/nexorious`)

### Handler (`internal/api/import.go`)

Accepts `multipart/form-data` with a `file` field.

**Validation sequence:**

1. Check content type (`application/json`, `text/json`, `application/octet-stream` accepted)
2. Read file body; reject if >50 MB (413)
3. Parse JSON; reject if malformed (400)
4. Validate top-level structure: must be an object with a `games` array (400)
5. Reject if `games` is empty (400)
6. Check for active nexorious import for this user (409 Conflict)

**Job creation:**

1. Create a `Job` row: `job_type="import"`, `source="nexorious"`, `status="pending"`, `priority="high"`, `total_items=len(games)`
2. Create one `JobItem` per game entry:
   - `item_key`: `"igdb_{igdb_id}"` if the game has an `igdb_id`, otherwise `"game_{index}"`
   - `source_title`: game's `title` field (fallback `"Game {index}"`)
   - `source_metadata`: `{"item_type": "game", "data": <full game object>}`
   - `status`: `"pending"`
3. Enqueue one `pending_task` per JobItem: `task_type="import_item"`, payload `{"job_item_id": "..."}`, priority matches job

**Response:**

```json
{
  "job_id": "uuid",
  "source": "nexorious",
  "status": "pending",
  "message": "Import job created. Processing 42 games.",
  "total_items": 42
}
```

### Worker Task — `import_item` (`internal/worker/tasks/import_item.go`)

Registered on the worker pool as `"import_item"`. Receives a `pending_task` whose payload contains `{"job_item_id": "..."}`.

**Processing sequence:**

1. Load the `JobItem` by ID; extract game data from `source_metadata.data`
2. **Upsert game:** Use `igdb_id` as the game's PK (the `games.id` column is the IGDB ID). If the game already exists, update metadata fields only if the existing values are null (don't overwrite richer data from a previous IGDB fetch). If no `igdb_id`, skip — mark item `failed` with `"missing igdb_id"`.
3. **Create UserGame:** Insert with the user's data (`play_status`, `personal_rating`, `is_loved`, `hours_played`, `personal_notes`, timestamps). The export format's `personal_rating` is a float; truncate to `int32` for the Go model (e.g. `9.5` → `9`). If the user already has a `user_game` for this `game_id`, skip — mark item `completed` with a result noting `"already_exists": true` (idempotent).
4. **Platforms:** For each platform entry in the game data:
   - Look up `platform_id` (slug) in the `platforms` table. If not found, skip this platform (log warning).
   - Look up `storefront_id` (slug) in the `storefronts` table. If not found, set storefront to null.
   - Insert a `UserGamePlatform` row with the matched slugs and remaining fields (`store_game_id`, `store_url`, `is_available`, `hours_played`, `ownership_status`, `acquired_date`).
5. **Tags:** For each tag entry in the game data:
   - Find or create a `Tag` row for this user by `name` (case-insensitive match). If creating, use the provided `color`.
   - Insert a `UserGameTag` association.
6. Mark JobItem `completed` with result `{"game_id": <id>, "user_game_id": "<id>", "is_new_addition": true}`.
7. **Job completion check:** Count remaining `pending` items for this job. If zero, determine final job status:
   - All items `completed` → job status `"completed"`
   - Any items `failed` → job status `"completed_with_errors"`

**Error handling per item:**
- Game upsert fails → mark item `failed`, don't stop other items
- Platform slug not found → skip that platform association, log warning, don't fail the item
- Tag creation fails → mark item `failed`
- Any panic → recovered by worker pool, item marked `failed`

## Export

### JSON Export (`POST /api/export/json`)

**Handler** (`internal/api/export.go`):

1. Count user's games; reject with 400 if zero
2. Create a `Job` row: `job_type="export"`, `source="nexorious"`, `status="pending"`, `total_items=count`
3. Enqueue a single `pending_task`: `task_type="export_json"`, payload `{"job_id": "..."}`
4. Return `{job_id, status, message, estimated_items}`

**Worker task** (`internal/worker/tasks/export.go`):

1. Load the Job; update status to `"processing"`, set `started_at`
2. Query all user games with Bun relations: `.Relation("Game").Relation("Platforms").Relation("Tags").Relation("Tags.Tag")`
3. Build `NexoriousExportData`:
   - `export_version`: `"1.2"`
   - `export_date`: now (UTC)
   - `user_id`: from job
   - `total_games`: count
   - `total_wishlist`: `0` (no wishlist in Go port)
   - `export_stats`: computed from the data (by_status, by_platform, total_hours, rated_count, loved_count)
   - `games`: each UserGame mapped to the export game object format
   - `wishlist`: empty array
4. Write JSON to `{StoragePath}/exports/{user_id}_{timestamp}.json`
5. Update Job: set `file_path`, `status="completed"`, `completed_at`

### CSV Export (`POST /api/export/csv`)

Same handler flow but enqueues `task_type="export_csv"`.

**Worker task** flattens each game into a row:

| Column | Source |
|--------|--------|
| `title` | game.Title |
| `igdb_id` | game.ID |
| `play_status` | user_game.PlayStatus |
| `personal_rating` | user_game.PersonalRating |
| `is_loved` | user_game.IsLoved |
| `hours_played` | user_game.HoursPlayed |
| `personal_notes` | user_game.PersonalNotes |
| `platforms` | semicolon-joined platform slugs |
| `tags` | semicolon-joined tag names |
| `release_year` | year from game.ReleaseDate |
| `created_at` | user_game.CreatedAt (RFC 3339) |
| `updated_at` | user_game.UpdatedAt (RFC 3339) |

Writes to `{StoragePath}/exports/{user_id}_{timestamp}.csv`. Updates Job same as JSON export.

CSV is not re-importable.

### Download (`GET /api/export/:id/download`)

1. Load Job by ID; return 404 if not found
2. Verify `job.user_id == current_user.id` — return 404 if mismatch (don't leak existence)
3. Verify `job.job_type == "export"` — return 400 if not
4. Verify `job.status == "completed"` — return 400 with current status if not
5. Verify `job.file_path` is set — return 500 if not
6. Verify file exists on disk — return 410 Gone if not
7. Check expiration: if `completed_at` + 24 hours < now, delete file and return 410 Gone
8. Serve file with:
   - JSON: `Content-Type: application/json`, filename `nexorious_export_{timestamp}.json`
   - CSV: `Content-Type: text/csv`, filename `nexorious_export_{timestamp}.csv`

### File Lifecycle

Export files stored in `{StoragePath}/exports/`. This directory is **not** exposed via any static route — the existing `/static/cover_art/*` route is scoped to `{StoragePath}/cover_art/` only. Export files are accessible exclusively through the download handler, which enforces ownership.

Files retained for 24 hours. The existing `CleanupExports` scheduler job handles deletion. The exports directory is created on first export if it doesn't exist.

## File Structure

### New Files

| File | Purpose |
|------|---------|
| `internal/api/import.go` | `ImportHandler` with `POST /api/import/nexorious` |
| `internal/api/import_test.go` | Handler tests |
| `internal/api/export.go` | `ExportHandler` with JSON/CSV export + download |
| `internal/api/export_test.go` | Handler tests |
| `internal/worker/tasks/import_item.go` | `import_item` task handler |
| `internal/worker/tasks/export.go` | `export_json` and `export_csv` task handlers |
| `internal/worker/tasks/import_item_test.go` | Task tests |
| `internal/worker/tasks/export_test.go` | Task tests |

The `internal/worker/tasks/` directory is new — establishes the pattern for Phase 4 sync tasks.

### Handler Dependencies

| Handler | Dependencies |
|---------|-------------|
| `ImportHandler` | `*bun.DB`, `*worker.Pool` |
| `ExportHandler` | `*bun.DB`, `*worker.Pool`, `*config.Config` |

Same dependency injection pattern as existing handlers (e.g. `JobsHandler`).

### Router Wiring

Routes added to `internal/api/router.go` inside the JWT-required API zone:

```go
importGroup := api.Group("/import")
{
    importGroup.POST("/nexorious", imh.HandleImportNexorious)
}

exportGroup := api.Group("/export")
{
    exportGroup.POST("/json", exh.HandleExportJSON)
    exportGroup.POST("/csv", exh.HandleExportCSV)
    exportGroup.GET("/:id/download", exh.HandleDownload)
}
```

### Slumber Collection

New requests in `slumber.yaml`:
- `import/nexorious` — POST with file upload
- `export/json` — POST
- `export/csv` — POST
- `export/download` — GET with job ID

All with JWT bearer auth following the existing pattern.

## Error Handling Summary

### Import Handler
| Condition | Response |
|-----------|----------|
| Invalid content type | 400 |
| File too large (>50 MB) | 413 |
| Invalid JSON | 400 |
| Missing/empty `games` array | 400 |
| Active import already running | 409 |

### Import Task (per item)
| Condition | Behavior |
|-----------|----------|
| Missing `igdb_id` | Item marked `failed` |
| Game upsert DB error | Item marked `failed` |
| User already has this game | Item marked `completed` (idempotent, `already_exists: true`) |
| Platform slug not found | Skip platform, log warning, don't fail item |
| Tag creation error | Item marked `failed` |

### Export Handler
| Condition | Response |
|-----------|----------|
| No games to export | 400 |

### Download Handler
| Condition | Response |
|-----------|----------|
| Job not found / not owned | 404 |
| Not an export job | 400 |
| Not completed | 400 |
| File path not set | 500 |
| File missing from disk | 410 |
| File expired (>24h) | 410 |

## Testing

Tests use testcontainers-go (real PostgreSQL) following the existing test patterns:
- **Import handler:** invalid file, invalid JSON, missing fields, empty games, conflict with active import, successful creation
- **Import task:** game upsert, duplicate game (idempotent), platform resolution, tag creation, job completion detection
- **Export handler:** empty collection rejection, successful job creation
- **Export task:** JSON output format validation, CSV column correctness
- **Download:** ownership check, status checks, file expiration, missing file

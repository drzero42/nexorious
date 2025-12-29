# Backup/Restore System Design

## Overview

Full system backup and restore functionality for disaster recovery, migration, and data safety. Admin-only feature.

## Data Scope

### What gets backed up

**Database** - Full PostgreSQL dump via `pg_dump`:
- All user accounts, sessions, preferences
- Game library with metadata, ratings, notes, playtime
- Tags and game-tag associations
- Wishlists
- Platform/storefront configurations
- Sync configurations and job history
- Ignored external games

**Static files:**
- `storage/cover_art/` - All downloaded game covers
- `static/logos/` - Platform and storefront logos

### Archive format

`.tar.gz` containing:

```
backup-2025-01-15T020000Z/
├── manifest.json
├── database.sql
├── cover_art/
│   └── *.jpg, *.png, ...
└── logos/
    └── platforms/...
    └── storefronts/...
```

## Manifest Structure

```json
{
  "version": 1,
  "created_at": "2025-01-15T02:00:00Z",
  "app_version": "1.0.0",
  "alembic_revision": "f7326f86754b",
  "backup_type": "scheduled",
  "database": {
    "file": "database.sql",
    "size_bytes": 1048576,
    "checksum": "sha256:abc123..."
  },
  "files": {
    "cover_art": {
      "count": 150,
      "total_size_bytes": 52428800,
      "checksum": "sha256:def456..."
    },
    "logos": {
      "count": 24,
      "total_size_bytes": 102400,
      "checksum": "sha256:ghi789..."
    }
  },
  "stats": {
    "users": 3,
    "games": 450,
    "tags": 12
  }
}
```

Fields:
- **version**: Manifest schema version (for future-proofing)
- **alembic_revision**: Used to determine if migrations are needed after restore
- **backup_type**: `scheduled`, `manual`, or `pre_restore`
- **checksums**: SHA256 of database dump and tar of each file directory (for upload validation)
- **stats**: Quick reference without extracting the full backup

## Configuration

### Backup configuration model

```python
class BackupConfig:
    schedule: str  # "manual", "daily", "weekly", or cron expression
    schedule_time: str  # Time of day for daily/weekly (e.g., "02:00")
    schedule_day: int | None  # Day of week for weekly (0=Monday, 6=Sunday)
    retention_mode: str  # "days" or "count"
    retention_value: int  # Number of days or number of backups to keep
    backup_path: str  # Default: "storage/backups"
```

### Retention rules

| Backup type | Retention |
|-------------|-----------|
| Scheduled backups | User-configured retention (days or count) |
| Manual backups ("Backup Now") | No auto-deletion, user must delete manually |
| Backup-before-restore | 7 days, auto-deleted |

Retention cleanup runs after each successful scheduled backup.

### Scheduling

- Uses existing task queue scheduler
- Scheduled task calls the same backup service as "Backup Now"
- Config changes update/cancel the scheduled task accordingly

## Backup Process

Creating a backup (triggered manually or by schedule):

1. **Create Job** - Register a new Job record with type `BACKUP`, status `pending`

2. **Database dump** - Execute `pg_dump` to a temporary file:
   ```bash
   pg_dump --format=plain --no-owner --no-acl DATABASE_URL > /tmp/backup-xxx/database.sql
   ```

3. **Copy static files** - Copy `cover_art/` and `logos/` directories to temp location

4. **Generate manifest** - Calculate checksums, gather stats, write `manifest.json`

5. **Create archive** - Compress everything into `.tar.gz`:
   ```
   backup-2025-01-15T020000Z.tar.gz
   ```

6. **Move to backup directory** - Atomic move from temp to configured `backup_path`

7. **Run retention cleanup** - Delete old backups per retention rules (scheduled backups only)

8. **Update Job** - Mark as `completed` with file path and size

### Error handling

- If any step fails, clean up temp files and mark Job as `failed` with error message
- Database dump runs with a timeout to prevent hanging

## Restore Process

### Restoring from a server backup

1. **Validate backup** - Check manifest exists, verify schema version is present

2. **Create pre-restore backup** - Automatic safety backup of current state (7-day retention)
   - Skip this step if restoring from a `pre_restore` type backup

3. **Preserve admin session** - Save current admin's `User` and `UserSession` records

4. **Extract archive** - Unpack to temporary directory

5. **Stop accepting requests** - Set application to maintenance mode (reject non-admin API calls)

6. **Restore database**:
   - Drop all tables
   - Execute `psql < database.sql`
   - Run Alembic migrations if backup is from older schema version

7. **Restore admin session**:
   - If admin's user ID doesn't conflict with restored data, re-insert their session record
   - If conflict exists, admin gets logged out

8. **Restore static files** - Replace `cover_art/` and `logos/` directories

9. **Exit maintenance mode** - Clear caches, reinitialize connections, return to normal operation

10. **Mark complete** - Log success

### Restoring from uploaded backup

Same process, but with additional validation before step 2:
- Verify all checksums from manifest match actual file contents
- Reject if checksums don't match (file corrupted or tampered)

### Schema compatibility

Backups from older schema versions are supported:
- Manifest contains `alembic_revision`
- After restoring database, run Alembic migrations to bring schema to current version
- If migrations fail, system stays in maintenance mode for manual recovery

### Error handling

- If restore fails after database drop, system remains in maintenance mode
- Admin receives clear error message: "Restore failed at step X. A backup of your previous state was saved at `backup-pre-restore-2025-01-15T...`. Use the restore endpoint to recover."
- No automatic rollback - admin stays in control

## API Endpoints

### Backup operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/admin/backups` | Create backup now (returns Job ID) |
| `GET` | `/api/admin/backups` | List available backups |
| `GET` | `/api/admin/backups/{id}/download` | Download backup file |
| `DELETE` | `/api/admin/backups/{id}` | Delete a backup |

### Restore operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/admin/backups/{id}/restore` | Restore from server backup |
| `POST` | `/api/admin/backups/restore/upload` | Restore from uploaded file |

### Configuration

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/admin/backups/config` | Get schedule/retention settings |
| `PUT` | `/api/admin/backups/config` | Update schedule/retention settings |

### Response for listing backups

```json
{
  "backups": [
    {
      "id": "backup-2025-01-15T020000Z",
      "created_at": "2025-01-15T02:00:00Z",
      "type": "scheduled",
      "size_bytes": 53477376,
      "stats": { "users": 3, "games": 450, "tags": 12 }
    }
  ]
}
```

## Frontend UI

### Backup/Restore page (admin settings area)

**Configuration section:**
- Schedule dropdown: "Manual", "Daily", "Weekly"
- Time picker (for daily/weekly)
- Day picker (for weekly)
- Retention mode: Radio or dropdown - "Keep backups for X days" or "Keep last X backups"
- Single number input for retention value
- Backup path display
- "Save Configuration" button

**Actions section:**
- "Backup Now" button - triggers manual backup, shows progress

**Backups list:**
- Table showing: filename, created date, type (scheduled/manual/pre-restore), size, stats summary
- Actions per row: Download, Delete, Restore
- Type badge with different colors (blue for scheduled, gray for manual, orange for pre-restore)

**Restore confirmation dialog:**
- Prominent warning: "This will completely replace all current data"
- Shows backup details (date, stats)
- Checkbox: "I understand this action cannot be undone"
- "Cancel" and "Restore" buttons (Restore disabled until checkbox checked)

**Upload restore:**
- Dropzone or file picker for `.tar.gz` upload
- Shows validation status after upload
- Same confirmation dialog flow

## Error Handling & Edge Cases

### Backup errors

- `pg_dump` fails → Job marked failed with error, temp files cleaned up
- Disk full → Job fails with "insufficient disk space" message
- Backup path not writable → Fail fast with clear error before starting

### Restore errors

- Invalid/corrupted archive → Reject before any changes, clear error message
- Checksum mismatch (uploads) → Reject with "file corrupted or modified" message
- Migration fails after restore → System in maintenance mode, admin instructed to restore from pre-restore backup
- Admin user ID conflict in restored data → Admin gets logged out (session not preserved)

### Concurrent operations

- Only one backup or restore can run at a time
- Attempting a second backup/restore while one is in progress returns 409 Conflict

### Maintenance mode during restore

- All non-admin API calls return 503 Service Unavailable with message "System restore in progress"
- Frontend shows maintenance banner if it receives this response
- Mode automatically clears on restore success or stays active on failure (admin must resolve)

## Implementation Notes

### Files Created/Modified

**Backend:**
- `app/core/config.py` - Added `backup_path` setting
- `app/models/backup_config.py` - BackupConfig model with schedule and retention enums
- `app/schemas/backup.py` - API request/response schemas
- `app/services/backup_service.py` - Core backup/restore logic with pg_dump/psql integration
- `app/middleware/maintenance.py` - Maintenance mode middleware for blocking requests during restore
- `app/api/backup_endpoints.py` - Admin-only API endpoints (8 endpoints total)
- `app/worker/tasks/maintenance/backup_scheduled.py` - Scheduled backup task (hourly cron check)
- `app/alembic/versions/f40c6a59ff67_add_backup_config_table.py` - Database migration

**Tests:**
- `app/tests/test_backup_config_model.py` - Model unit tests
- `app/tests/test_backup_service.py` - Service unit tests (validation, retention, checksums)
- `app/tests/test_backup_endpoints.py` - API integration tests (27 tests)

### Dependencies

No new dependencies required - uses stdlib `tarfile`, `hashlib`, `subprocess`, and existing PostgreSQL tools (`pg_dump`, `psql`).

### Database Migration

Migration creates `backup_config` table for storing schedule/retention settings:
- `id` - Always 1 (singleton)
- `schedule` - ENUM: manual, daily, weekly
- `schedule_time` - HH:MM format (default: 02:00)
- `schedule_day` - 0-6 for weekly schedule
- `retention_mode` - ENUM: days, count
- `retention_value` - Number of days or count
- `updated_at` - Timestamp

### API Endpoints Implemented

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/admin/backups/config` | Get backup configuration |
| `PUT` | `/api/admin/backups/config` | Update backup configuration |
| `POST` | `/api/admin/backups` | Create manual backup |
| `GET` | `/api/admin/backups` | List available backups |
| `GET` | `/api/admin/backups/{id}/download` | Download backup file |
| `DELETE` | `/api/admin/backups/{id}` | Delete a backup |
| `POST` | `/api/admin/backups/{id}/restore` | Restore from server backup |
| `POST` | `/api/admin/backups/restore/upload` | Restore from uploaded file |

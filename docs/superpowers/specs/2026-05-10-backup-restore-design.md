# Backup & Restore Design Spec

Completes Phase 3 of the Go port. Implements full backup/restore with scheduled backups, admin endpoints, setup-time restore, and maintenance mode middleware.

## Tooling Dependency: pg_dump / psql

Backup and restore depend on PostgreSQL client tools (`pg_dump` for export, `psql` for import). These are **not bundled** with the Go binary — they must be present on the host system.

### Availability Check

At startup, probe for both tools via `exec.LookPath("pg_dump")` and `exec.LookPath("psql")`. Store results as package-level booleans in a new `internal/backup` package. Check once at startup, cache for the process lifetime.

### Health Endpoint Change

`GET /health` gains a `backup_available` field:

```json
{
  "status": "ok",
  "igdb_configured": true,
  "backup_available": true
}
```

`backup_available` is `true` only when **both** `pg_dump` and `psql` are found.

### Endpoint Behaviour When Unavailable

- Backup-create endpoints (`POST /api/admin/backups`) return **503** when `pg_dump` is missing.
- Restore endpoints (`POST /api/admin/backups/:id/restore`, `POST /api/admin/backups/restore/upload`, `POST /api/auth/setup/restore`) return **503** when `psql` is missing.
- List, download, delete, and config endpoints work regardless — they only touch files and the database.

Error response format:

```json
{ "error": "pg_dump is not available on this system. Install PostgreSQL client tools to enable backups." }
```

### Frontend Change

When the user navigates to the backup/restore admin page, the UI reads `backup_available` from the health or status response. If `false`, show a persistent banner:

> **Backup and restore are unavailable** because PostgreSQL client tools (pg_dump/psql) are not installed on the server.

Disable the "Create Backup", "Restore", and "Upload & Restore" buttons. List and download remain functional (existing archives can still be managed).

The setup page restore option (`POST /api/auth/setup/restore`) should also check and show a similar message if `psql` is unavailable.

---

## Archive Format

`.tar.gz` archive, identical structure to the Python version:

```
backup-{id}.tar.gz
└── backup-{id}/
    ├── manifest.json
    ├── database.sql
    └── cover_art/
```

### manifest.json

```json
{
  "version": 1,
  "created_at": "2026-05-10T02:00:00Z",
  "app_version": "0.1.0",
  "migration_version": "20260503000001",
  "backup_type": "manual",
  "database_file": "database.sql",
  "database_size_bytes": 123456,
  "database_checksum": "sha256:...",
  "cover_art_count": 42,
  "cover_art_size_bytes": 8388608,
  "cover_art_checksum": "sha256:...",
  "stats_users": 1,
  "stats_games": 150,
  "stats_tags": 12
}
```

Fields:
- `version` — manifest schema version (always `1` for now)
- `app_version` — Go binary version (injected via `-ldflags` at build time)
- `migration_version` — the highest applied migration version in the database at backup time (read from `schema_migrations` table)
- `backup_type` — `"manual"` | `"scheduled"` | `"pre_restore"`
- `database_checksum`, `cover_art_checksum` — SHA-256 hex digests
- `stats_*` — row counts for display in the backup list UI

### database.sql

Full `pg_dump` output in plain SQL format. Contains **all data** — passwords, credentials, session tokens, sync configs, everything. Generated with:

```
pg_dump --format=plain --no-owner --no-acl \
  --host=... --port=... --username=... --dbname=... \
  --file=database.sql
```

Connection params parsed from `DATABASE_URL`. Timeout: 300 seconds.

### cover_art/

Complete copy of `{STORAGE_PATH}/cover_art/`. If the directory doesn't exist or is empty, an empty `cover_art/` directory is included.

Platform/storefront logos are embedded in the binary (in `ui/public/logos/`) and are **not** included in backup archives.

---

## Backup Service

New package: `internal/backup/service.go`. Stateless service struct holding config references (backup path, storage path, database URL). Injected into handlers via the router.

A `sync.Mutex` on the service guards all restore and backup-create operations. `RestoreBackup`, `RestoreFromUpload`, and `CreateBackup` acquire the lock at entry and return **409 Conflict** (`{"error": "A backup or restore operation is already in progress"}`) if the lock is held. This prevents concurrent restores from destroying the database and concurrent backup-creates from colliding on temp directories and archive naming.

### BackupService Methods

**`CheckTools()`** — called at startup, sets package-level `pgDumpAvailable` and `psqlAvailable` booleans. Exported via `PgDumpAvailable()` and `PsqlAvailable()` accessors.

**`CreateBackup(backupType string) (string, error)`** — creates a backup archive:
1. Generate backup ID: `backup-{YYYYMMDD-HHmmss}`
2. Create temp directory
3. Run `pg_dump` → `database.sql`
4. Copy `cover_art/` directory
5. Query DB stats (user count, game count, tag count)
6. Read migration version from `schema_migrations`
7. Write `manifest.json`
8. Create `.tar.gz` archive in the configured backup path
9. Clean up temp directory
10. Return backup ID

**`ListBackups() ([]BackupInfo, error)`** — glob `backup-*.tar.gz` in backup directory, read each manifest, return sorted by `created_at` descending.

**`GetBackupPath(backupID string) string`** — returns full path for a backup archive.

**`ValidateArchive(archivePath string, verifyChecksums bool) (*Manifest, error)`** — opens archive, reads manifest, checks `database.sql` exists, optionally verifies SHA-256 checksums. Additionally:
- Rejects archives with an unknown `manifest.version` (> current supported version).
- Rejects archives whose `migration_version` is higher than the highest migration known to the running binary. This prevents restoring a backup created by a newer app version whose schema the current binary cannot understand. Error: `"Backup was created by a newer version of Nexorious (migration %s). This binary only supports up to migration %s. Upgrade before restoring."`

**`DeleteBackup(backupID string) error`** — removes the `.tar.gz` file.

**`RestoreBackup(backupID string, skipPreRestore bool) error`** — see Restore Behaviour below.

**`RestoreFromUpload(archivePath string) error`** — validates uploaded archive, moves to backup dir, then delegates to `RestoreBackup`.

**`ApplyRetention(config BackupConfig) error`** — deletes backups exceeding retention policy. Called after each successful backup creation. Supports two modes:
- `days` — delete backups older than `retention_value` days
- `count` — keep only the most recent `retention_value` backups
- Pre-restore backups older than 7 days are always cleaned up regardless of retention mode

---

## Restore Behaviour

### Steps

1. **Set maintenance mode** — `middleware.SetMaintenanceMode(true)`
2. **Shut down worker pool and scheduler** — `pool.Shutdown()`, `scheduler.Stop()`
3. **Create pre-restore backup** — unless `skipPreRestore` is true (setup restore or restoring a pre-restore backup)
4. **Close Bun DB pool** — `db.Close()`
5. **Terminate other DB connections** — `psql --command="SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '...' AND pid <> pg_backend_pid();"`
6. **Drop and recreate schema** — `psql --command="DROP SCHEMA public CASCADE; CREATE SCHEMA public;"`
7. **Restore database** — `psql --file=database.sql`
8. **Restore cover art** — replace `{STORAGE_PATH}/cover_art/` with archive contents
9. **Re-create Bun DB pool** — open new connection via `ReconnectDB`; update `DBHolder`
10. **Create fresh Worker Pool and Scheduler** — new instances with the new `*bun.DB`; old instances were shut down in step 2 and are discarded
11. **Re-initialize Migrator** — checks pending migrations against the restored DB
12. **Clear maintenance mode** — `middleware.SetMaintenanceMode(false)`
13. **App state machine takes over:**
    - If restored DB is behind current migrations → state becomes `NeedsMigration` → migration UI appears → user triggers migrations → state transitions to `Ready` → workers + scheduler start
    - If restored DB is current → state goes straight to `Ready` → restart workers + scheduler immediately

**No `os.Exit`** — the process stays alive and the existing state machine handles the transition.

### Rollback on Failure

If any step from 5–8 fails (terminate connections, drop schema, psql restore, or cover art copy), the restore handler attempts an automatic rollback:

1. Log the original error at ERROR level.
2. If a pre-restore backup was created (step 3), attempt to restore it using the same steps 5–8 with `skipPreRestore=true` (no infinite recursion).
3. If the rollback restore succeeds → continue to steps 9–13 as normal. The database is back to its pre-attempt state. Log a WARN: `"Restore of <backup-id> failed: <error>. Successfully rolled back to pre-restore backup."`
4. If the rollback restore also fails → the database is in an unrecoverable state. The handler:
   - Logs FATAL: `"Restore failed AND rollback failed. Database is in an inconsistent state. Manual intervention required. Original error: <err1>. Rollback error: <err2>. Pre-restore backup is at: <path>."`
   - Leaves maintenance mode **on** (prevents the app from serving requests against a broken DB).
   - Sets the app state to `DBUnavailable` so the middleware redirects all requests to `/db-error`.
   - The `/db-error` page (which bypasses all middleware gates) is already visible. Its auto-reload will show the user a meaningful error. The pre-restore backup path is included in the log so an operator can manually restore via `psql`.
   - Returns the error to the HTTP caller (the admin who triggered the restore).

If no pre-restore backup exists (setup restore or restoring a pre-restore backup), there is nothing to roll back to — the handler logs FATAL with the same detail and transitions to `DBUnavailable` + maintenance mode.

### Setup Restore

`POST /api/auth/setup/restore` — special case:
- No JWT required (no users exist yet)
- Returns 403 if any user already exists
- Returns 503 if `psql` is unavailable
- Accepts `.tar.gz` file upload (max 2 GB)
- Validates archive (including checksum verification)
- Skips pre-restore backup (database is empty)
- After restore, user is redirected to login with their restored credentials (or migration UI if the backup is older)

---

## Maintenance Mode Middleware

New file: `internal/middleware/maintenance.go`

```go
var (
    mu              sync.RWMutex
    maintenanceMode bool
)

func SetMaintenanceMode(enabled bool) {
    mu.Lock()
    defer mu.Unlock()
    maintenanceMode = enabled
}

func IsMaintenanceMode() bool {
    mu.RLock()
    defer mu.RUnlock()
    return maintenanceMode
}
```

Echo middleware checks `IsMaintenanceMode()` on every request. Allowed during maintenance:
- `GET /health`
- `GET|POST|DELETE /api/admin/backups/*`
- `GET /api/auth/me`

All other requests receive:

```json
{ "error": "Service Unavailable", "detail": "Restore in progress", "maintenance_mode": true }
```

HTTP status: **503**.

The middleware sits inside the app-state middleware (only runs once state is `Ready`).

---

## Endpoints

### Admin Backup Endpoints (JWT + AdminMiddleware)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/admin/backups/config` | Get backup schedule config |
| `PUT` | `/api/admin/backups/config` | Update schedule; rebuild gocron job |
| `GET` | `/api/admin/backups` | List backups (reads manifests from disk) |
| `POST` | `/api/admin/backups` | Create manual backup (503 if no pg_dump) |
| `DELETE` | `/api/admin/backups/:id` | Delete backup archive |
| `GET` | `/api/admin/backups/:id/download` | Download backup archive |
| `POST` | `/api/admin/backups/:id/restore` | Restore from stored backup (503 if no psql) |
| `POST` | `/api/admin/backups/restore/upload` | Upload and restore external archive (503 if no psql) |

### Setup Restore Endpoint (no JWT)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/auth/setup/restore` | Restore during initial setup (403 if users exist, 503 if no psql) |

### Request/Response Schemas

**GET /api/admin/backups/config**
```json
{
  "schedule_cron": "0 2 * * *",
  "retention_mode": "days",
  "retention_value": 30
}
```

**PUT /api/admin/backups/config**
```json
{
  "schedule_cron": "0 2 * * *",
  "retention_mode": "days",
  "retention_value": 30
}
```

**POST /api/admin/backups** (create)
```json
{
  "backup_id": "backup-20260510-020000",
  "message": "Backup created successfully"
}
```

**GET /api/admin/backups** (list)
```json
{
  "backups": [
    {
      "id": "backup-20260510-020000",
      "created_at": "2026-05-10T02:00:00Z",
      "backup_type": "manual",
      "size_bytes": 1234567,
      "stats": {
        "users": 1,
        "games": 150,
        "tags": 12
      }
    }
  ],
  "total": 1
}
```

**POST /api/admin/backups/:id/restore**

Request:
```json
{ "confirm": true }
```

Response:
```json
{
  "success": true,
  "message": "Restore completed from: backup-20260510-020000. All sessions have been cleared — please log in again."
}
```

**POST /api/admin/backups/restore/upload**

Multipart form upload with a `.tar.gz` file. Maximum upload size: **2 GB**. Same response as restore.

**POST /api/auth/setup/restore**

Multipart form upload with a `.tar.gz` file. Maximum upload size: **2 GB**.
```json
{
  "success": true,
  "message": "Backup restored successfully. Please log in with your restored credentials."
}
```

---

## Scheduled Backups

### backup_config Table

Already exists in the schema — singleton table (id=1) with default values:

```sql
CREATE TABLE backup_config (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    schedule_cron   TEXT NOT NULL DEFAULT '',
    retention_mode  TEXT NOT NULL DEFAULT 'days',
    retention_value INTEGER NOT NULL DEFAULT 30,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Seeded with `schedule_cron = '0 2 * * *'` (daily at 2 AM UTC).

### Scheduler Integration

In `Scheduler.Start()`:
1. Read `backup_config` row
2. If `schedule_cron` is non-empty **and** `pg_dump` is available, register a gocron job
3. If `pg_dump` is not available, log a warning and skip backup job registration
4. The scheduled job calls `BackupService.CreateBackup("scheduled")` then `ApplyRetention(config)`

When `PUT /api/admin/backups/config` is called:
1. Update the `backup_config` row
2. Remove existing backup gocron job (if any)
3. Re-register with new cron expression (or don't, if cron is empty or pg_dump unavailable)

### Retention

After each backup (manual or scheduled), `ApplyRetention` runs:
- `retention_mode = "days"` → delete backups older than `retention_value` days
- `retention_mode = "count"` → keep the most recent `retention_value` backups, delete the rest
- Pre-restore backups (`backup_type = "pre_restore"`) older than 7 days are always cleaned up

---

## Router Wiring

The admin backup group needs to be added to `internal/api/router.go`:

```go
// Admin zone — JWT + admin required
admin := api.Group("", auth.AdminMiddleware())
backupHandler := NewBackupHandler(db, backupService, migrator, pool, scheduler)

adminBackups := admin.Group("/admin/backups")
adminBackups.GET("/config", backupHandler.HandleGetConfig)
adminBackups.PUT("/config", backupHandler.HandleUpdateConfig)
adminBackups.GET("", backupHandler.HandleListBackups)
adminBackups.POST("", backupHandler.HandleCreateBackup)
adminBackups.DELETE("/:id", backupHandler.HandleDeleteBackup)
adminBackups.GET("/:id/download", backupHandler.HandleDownloadBackup)
adminBackups.POST("/:id/restore", backupHandler.HandleRestore)
adminBackups.POST("/restore/upload", backupHandler.HandleRestoreUpload)
```

The setup restore route is already registered in the setup zone (bypasses JWT):

```go
setup.POST("/api/auth/setup/restore", backupHandler.HandleSetupRestore)
```

The maintenance mode middleware is added inside the app-state middleware chain, after the `Ready` state check.

Gate 1/2/3 in the router must also allow `/api/admin/backups/*` through during maintenance mode (the maintenance middleware itself handles the 503 for non-allowed paths once state is `Ready`).

---

## Handler Dependencies

`BackupHandler` needs access to several components for restore orchestration:

- `*DBHolder` — shared `atomic.Pointer[bun.DB]` wrapper owned by `main.go`; all handlers read through this so the pool reference stays current after restore
- `*backup.Service` — the backup service itself
- `*migrate.Migrator` — for re-initialization after restore
- `*worker.Pool` — for shutdown during restore (old instance is discarded; a new one is created)
- `*scheduler.Scheduler` — for shutdown during restore (old instance is discarded; a new one is created)
- A `ReconnectDB func() (*bun.DB, error)` callback — provided by `main.go` at wiring time, encapsulates the logic to create a fresh Bun connection from the config. The handler calls this after psql restore completes, then calls `DBHolder.Set(newDB)` to update the shared reference
- A `RebuildPoolAndScheduler func(db *bun.DB) (*worker.Pool, *scheduler.Scheduler, error)` callback — provided by `main.go`; constructs fresh worker pool and scheduler instances with the new DB connection

This is a wider dependency set than other handlers, but restore is inherently an orchestration operation that touches the entire system.

---

## Slumber Collection

Add requests to `slumber.yaml` under a new `admin/backups/` folder:

- `GET /api/admin/backups/config` (admin JWT)
- `PUT /api/admin/backups/config` (admin JWT, body)
- `GET /api/admin/backups` (admin JWT)
- `POST /api/admin/backups` (admin JWT)
- `DELETE /api/admin/backups/:id` (admin JWT)
- `GET /api/admin/backups/:id/download` (admin JWT)
- `POST /api/admin/backups/:id/restore` (admin JWT, body with `confirm: true`)
- `POST /api/admin/backups/restore/upload` (admin JWT, multipart file upload)

---

## Go Port Spec Updates Required

The main go-port design spec (`2026-05-03-go-port-design.md`) must be updated:

1. **Backup Archive Format section** — add note that `pg_dump`/`psql` are runtime dependencies, not bundled
2. **Health endpoint** — document `backup_available` field
3. **Restore Behaviour section** — replace `os.Exit(0)` with in-process restart; add migration re-check step
4. **Backup Config section** — note that `schedule_cron` is the Go port's approach (replaces Python's `schedule`/`schedule_time`/`schedule_day` enum model)
5. **Phase 3 checklist** — mark backup/restore as designed

---

## File Map

| File | Action |
|------|--------|
| `internal/backup/service.go` | New — backup service (create, list, delete, validate, restore, retention) |
| `internal/backup/tools.go` | New — pg_dump/psql availability check, exec wrappers |
| `internal/middleware/maintenance.go` | New — maintenance mode flag + middleware |
| `internal/api/backup.go` | New — admin backup handlers |
| `internal/api/backup_test.go` | New — handler integration tests |
| `internal/api/router.go` | Modified — add admin backup routes, maintenance middleware, setup restore route |
| `internal/api/setup.go` | Modified — add `HandleSetupRestore` (delegates to backup service; registered in setup route zone but lives alongside other setup handlers) |
| `internal/scheduler/scheduler.go` | Modified — add scheduled backup job, config rebuild |
| `internal/db/models/models.go` | Modified — add `BackupConfig` Bun model |
| `slumber.yaml` | Modified — add backup admin requests |
| `docs/superpowers/specs/2026-05-03-go-port-design.md` | Modified — update backup/restore sections per changes above |

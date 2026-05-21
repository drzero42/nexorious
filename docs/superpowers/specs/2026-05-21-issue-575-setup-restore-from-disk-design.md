# Allow restore from on-disk backups during setup

**Issue:** [#575](https://github.com/drzero42/nexorious/issues/575)
**Date:** 2026-05-21
**Milestone:** 0.1.0

## Problem

The setup page (`ui/setup/index.html`) lets a first-time user either create an admin account or restore from a backup. The restore path today only accepts a file upload via `POST /api/auth/setup/restore`.

A realistic deployment scenario: the operator runs Nexorious in a container with `BACKUP_PATH` mounted from a network share that already contains backup archives. To restore, the operator currently has to download an archive locally and re-upload it through the browser — even though the file is already on a disk the server can read.

## Goal

During setup (when no users exist yet), surface the backup archives that live in `BACKUP_PATH` and let the user restore from any of them, while keeping the existing upload path as a fallback.

## Approach

Add two new public endpoints in the setup zone:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/auth/setup/backups` | List candidate backup files in `BACKUP_PATH` |
| `POST` | `/api/auth/setup/restore/disk` | Restore from a named on-disk backup |

The setup page's restore view grows a "Backups found on disk" list above the existing upload zone. The on-disk restore goes through a new `Service.RestoreFromArchive(path, opts)` method — a sibling of `Service.RestoreFromUpload` that runs the same `ValidateArchive` + `doRestore` sequence but does **not** rename the input. Reusing `RestoreFromUpload` directly would `os.Rename` the operator's curated on-disk backup to a new timestamped name inside `BACKUP_PATH`, destroying the original filename; `RestoreFromArchive` preserves the input file so subsequent restores can use it again.

No new config, no new env vars, no schema migration. Pure additive change.

### Why this layout, not a tabs/toggle UI

The list-first-with-upload-below layout means a user with on-disk backups sees them immediately and a user without them is naturally pushed to upload. Tabs or a radio toggle add a click and hide one path at a time; the alternatives don't earn that friction when the typical case is "pick the most recent backup."

### Why a new `restore/disk` endpoint, not overloading `restore`

`POST /api/auth/setup/restore` accepts multipart with a `file` field today. Overloading it to also accept JSON with a `filename` field would branch the handler on input shape and obscure the API surface. A second endpoint with a JSON body is cleaner to read, easier to test, and keeps `restore` semantically equivalent to "restore from this uploaded body".

## API contract

### `GET /api/auth/setup/backups`

**Authorization:** none required; rejects with 403 if any user already exists (same gate as `HandleSetupRestore`).

**Response 200:**

```json
{
  "backups": [
    {
      "filename": "nexorious-backup-20260520-093015.tar.gz",
      "size_bytes": 12582912,
      "mtime": "2026-05-20T09:30:15Z",
      "restorable": true,
      "manifest": {
        "created_at": "2026-05-20T09:30:15Z",
        "app_version": "0.0.42",
        "migration_version": "20260518120000",
        "backup_type": "scheduled",
        "stats": { "users": 2, "games": 1843, "tags": 27 }
      }
    },
    {
      "filename": "weird.tar.gz",
      "size_bytes": 4096,
      "mtime": "2026-05-19T11:02:00Z",
      "restorable": false,
      "reason": "unreadable manifest"
    },
    {
      "filename": "nexorious-backup-20260521-000001.tar.gz",
      "size_bytes": 13107200,
      "mtime": "2026-05-21T00:00:01Z",
      "restorable": false,
      "reason": "requires Nexorious migration ≥ 20260601000000 (this binary supports up to 20260518120000)"
    }
  ]
}
```

- Sorted newest-first by `mtime`.
- Empty array if the dir is empty, unreadable, or doesn't exist (not an error).
- `manifest` is omitted when not readable.
- `reason` is omitted when `restorable` is `true`.

**Response 403** when a user already exists:

```json
{ "error": "restore during setup is only available when no users exist" }
```

### `POST /api/auth/setup/restore/disk`

**Request body:**

```json
{ "filename": "nexorious-backup-20260520-093015.tar.gz" }
```

**Response codes:**

| Status | Body | When |
|---|---|---|
| 200 | `{ "success": true, "message": "Backup restored successfully. Please log in with your restored credentials." }` | Restore completed |
| 400 | `{ "error": "filename is required" }` | Missing/empty `filename` |
| 400 | `{ "error": "invalid filename" }` | Contains `/`, `\`, `..`, resolves outside `BACKUP_PATH`, or is a symlink |
| 403 | `{ "error": "restore during setup is only available when no users exist" }` | A user already exists |
| 404 | `{ "error": "backup not found" }` | File doesn't exist on disk |
| 409 | `{ "error": "A backup or restore operation is already in progress" }` | `ErrOperationInProgress` from the service |
| 500 | `{ "error": "restore failed: ..." }` | Any other restore error, including manifest/version/checksum validation failures |
| 503 | `{ "error": "psql is not available on this system. Install PostgreSQL client tools to enable restore." }` | Matches existing upload handler |

Success response shape matches `HandleSetupRestore` so the frontend can treat both paths uniformly.

## Backend changes

### `internal/backup/service.go`

New type and method:

```go
type ArchiveInfo struct {
    Filename   string     // base name only, e.g. "nexorious-backup-20260520-093015.tar.gz"
    SizeBytes  int64      // file size on disk
    ModTime    time.Time  // file mtime
    Manifest   *Manifest  // nil if manifest couldn't be read
    Restorable bool       // true only if archive validated end-to-end against this binary
    Reason     string     // human-readable reason when Restorable=false; empty when Restorable=true
}

// ListAvailableArchives scans the configured backup directory (top-level only)
// for *.tar.gz files and returns metadata for each. Files are returned
// regardless of whether they validate, so callers can show non-restorable
// files in the UI with an explanation. Sorted newest mtime first.
//
// Returns an empty slice (not an error) when the directory is empty,
// unreadable, or doesn't exist — listing is a best-effort discovery operation.
func (s *Service) ListAvailableArchives(ctx context.Context, maxMigrationVersion string) ([]ArchiveInfo, error)
```

Implementation:

1. `os.ReadDir(s.backupPath)`; on `ErrNotExist` / permission error, log at debug and return `nil, nil`.
2. For each entry: skip directories, symlinks, sockets, and non-regular files. Skip names not ending in `.tar.gz`.
3. `os.Stat` to capture size and mtime.
4. Try to read manifest via existing `readManifestFromArchive` helper. On error → `Restorable=false, Reason="unreadable manifest"`, no manifest.
5. On manifest read success, run the same migration-version check `ValidateArchive` does at [internal/backup/service.go:208-213](../../internal/backup/service.go#L208-L213). On mismatch → `Restorable=false`, reason matches the existing error string. Otherwise `Restorable=true`.
6. Sort descending by `ModTime`. Return.

Listing intentionally does **not** verify checksums — too slow for an interactive list, and `RestoreFromArchive` (and `RestoreFromUpload`) re-validate everything (including checksums) at restore time.

### `internal/api/backup.go`

Two new handlers plus a small shared helper:

```go
// requireNoUsers returns nil if the setup gate is open (no users yet),
// or an Echo-ready *echo.HTTPError to short-circuit with 403.
func (h *BackupHandler) requireNoUsers(c *echo.Context) error
```

Factor the existing user-count check at [internal/api/backup.go:337-340](../../internal/api/backup.go#L337-L340) out of `HandleSetupRestore` into this helper and reuse it from all three setup handlers.

```go
// HandleSetupListBackups handles GET /api/auth/setup/backups.
func (h *BackupHandler) HandleSetupListBackups(c *echo.Context) error
```

- `requireNoUsers` → return early if set.
- Resolve `maxMigrationVersion` via `h.callbacks.MaxMigration` (same source `makeRestoreOpts` uses).
- `infos, err := h.svc.ListAvailableArchives(ctx, maxMigrationVersion)`.
- Map each `ArchiveInfo` to the JSON response shape above; serialize.

The response's `manifest` object includes only the fields shown in the example: `created_at`, `app_version`, `migration_version`, `backup_type`, and a `stats` block with `users`, `games`, `tags`. Internal fields (checksums, byte sizes, database/cover-art filenames, manifest schema version) are not exposed — the UI doesn't need them and they bloat the response on dirs with many archives.

```go
// HandleSetupRestoreFromDisk handles POST /api/auth/setup/restore/disk.
func (h *BackupHandler) HandleSetupRestoreFromDisk(c *echo.Context) error
```

- `psql` availability check (same as `HandleSetupRestore`).
- `requireNoUsers` → return early if set.
- Bind JSON body to `{ Filename string }`.
- Validate `Filename`:
  1. Empty → 400 `filename is required`.
  2. `strings.ContainsAny(filename, `/\`)` or contains `..` or `filepath.Base(filename) != filename` → 400 `invalid filename`.
  3. `fullPath := filepath.Join(backupPath, filename)`; verify `filepath.Dir(fullPath) == filepath.Clean(backupPath)` → 400 if not.
  4. `os.Lstat(fullPath)`: if `ErrNotExist` → 404; if symlink (`mode&os.ModeSymlink != 0`) → 400; if not a regular file → 400.
- `opts := h.makeRestoreOpts(true)` (skip pre-restore, same as upload path).
- `h.svc.RestoreFromArchive(fullPath, opts)`. Map errors using the same pattern as the upload path:
  - `ErrOperationInProgress` → 409.
  - Anything else → 500 with body `{"error": "restore failed"}`. Full error detail goes to `slog.Error` for operators; the body is kept static so the unauthenticated endpoint doesn't leak internal paths.
- On success: 200 with the same success body `HandleSetupRestore` returns.

The on-disk file is **never renamed, moved, or deleted** by the restore — `RestoreFromArchive` is the new, non-mutating sibling of `RestoreFromUpload` that exists specifically so the operator's archive is preserved. Validation errors (corrupt manifest, future migration version, checksum mismatch) fall into the 500 bucket; finer-grained error classification (a `backup.ErrInvalidArchive` sentinel and a 422 mapping) is a separate change that should be applied to both setup-restore handlers at once if pursued — not in this PR's scope.

### `internal/api/router.go`

Register both routes alongside the existing setup-restore line ([internal/api/router.go:182-187](../../internal/api/router.go#L182-L187)):

```go
e.GET("/api/auth/setup/backups", h.HandleSetupListBackups)
e.POST("/api/auth/setup/restore/disk", h.HandleSetupRestoreFromDisk)
```

Both bypass JWT middleware (already the case for the setup-zone block).

## Frontend changes

[ui/setup/index.html](../../ui/setup/index.html) is plain HTML + vanilla JS (the React SPA isn't embedded at setup time). Match that style — no build step, no new dependencies.

### Behavior

On entering the restore view (when the user clicks "Restore from backup instead"):

1. Issue `GET /api/auth/setup/backups`. Show a "Looking for backups on disk…" placeholder while loading.
2. If `backups` array is empty (or fetch fails): skip the list section, render the existing upload zone alone, with a hint "No backups found on disk — upload one instead."
3. Otherwise render the list above the upload zone, separated by a subhead "Or upload a backup file".

### List item layout

```
┌─────────────────────────────────────────────────────────────────┐
│  nexorious-backup-20260520-093015.tar.gz       [ Restore ]      │
│  2026-05-20 09:30 · 12.0 MB · scheduled                         │
│  1,843 games · 2 users · 27 tags · v0.0.42                      │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  weird.tar.gz                                  [ Restore ] ◯    │
│  2026-05-19 11:02 · 4 KB                                        │
│  ⚠ unreadable manifest                                          │
└─────────────────────────────────────────────────────────────────┘
```

- Restorable items: button enabled, manifest stats line shown.
- Non-restorable items: button disabled, reason line in warning color, no manifest stats.
- Click `Restore` → confirm dialog ("Restore from `<filename>`? This will replace any existing data.") → `POST /api/auth/setup/restore/disk` with `{filename}`.
- On 200: redirect to `/login` (same as upload path).
- During restore: replace the list/upload area with a "Restoring…" state. Errors surface in the existing `restore-err` alert div.

Upload zone behavior is unchanged; it just moves below the list when the list is non-empty.

## Security

This endpoint is unauthenticated during setup (no users exist yet), so the only real attack surface is path traversal. The filename validation chain above is layered defense:

1. Reject obviously malicious characters (`/`, `\`, `..`).
2. `filepath.Base(filename) != filename` catches anything that survived step 1.
3. After `filepath.Join`, verify the resolved parent dir matches `BACKUP_PATH`.
4. `os.Lstat` reject symlinks — prevents a planted link from escaping the dir.
5. Reject non-regular files (directories, devices, sockets).

Only after all five pass does the path reach `RestoreFromArchive`. There's no in-process TOCTOU concern: `RestoreFromArchive` re-validates the manifest, version, and checksums when it opens the archive, so even a file swap between `Lstat` and open would be caught before the database is touched. Hardlinks at the top level of `BACKUP_PATH` are not detected by `os.Lstat` — the trust model assumes the directory is writable only by the nexorious process.

## Testing

Per the project's testing policy ([CLAUDE.md](../../CLAUDE.md)), write tests for security-sensitive logic and non-obvious behavior; skip tests on thin wrappers and tautologies.

### Service layer

**`TestListAvailableArchives`** (table-driven, single test function):

- Non-existent dir → empty slice, no error.
- Mixed-contents dir containing: a valid current-version archive, a valid older-version archive, an archive with a future migration version, a foreign `.tar.gz` (random bytes), a non-`.tar.gz` file, and a subdirectory.
- Asserts: only top-level `.tar.gz` files appear, `Restorable` and `Reason` are correct for each, results are sorted newest-first by mtime.

Uses `t.TempDir()` — no shared DB needed for this method.

### Handler layer

**`TestSetupZoneRejectsWhenUsersExist`**: insert a user, hit `GET /api/auth/setup/backups` and `POST /api/auth/setup/restore/disk`, assert both return 403. Tests the shared `requireNoUsers` gate once.

**`TestHandleSetupRestoreFromDiskValidatesFilename`** (table-driven):

| Input | Expected |
|---|---|
| `""` | 400 |
| `"../etc/passwd"` | 400 |
| `"sub/file.tar.gz"` | 400 |
| `"file\\name.tar.gz"` | 400 (Windows-style separator) |
| valid name not present in dir | 404 |
| valid name pointing to a symlink in dir | 400 |

Both handler tests use the existing shared test DB.

### Out of test scope

- `ListBackups` handler happy path (thin wrapper over service method).
- `RestoreFromDisk` happy path end-to-end (duplicates the existing setup-upload restore test; the new logic is filename validation, covered above).
- `RestoreFromDisk` 422 archive-validation mapping (same error classifier as the upload path, already covered).
- Empty-list listing (tautological).

### Manual verification (frontend)

The setup page has no automated test infra. Smoke-test:

- Dir with one valid backup → list shows it, Restore button enabled.
- Dir with mixed valid/invalid/future-version backups → all shown, only valid ones restorable.
- Empty dir → list section absent, upload zone visible with the "no backups found" hint.
- Restore-from-disk happy path → lands on `/login`.
- Upload path still works after the layout changes.

## Edge cases

- **File deleted between list and restore** → `RestoreFromArchive` fails at open; 500 with body `"restore failed"`. User can refresh and try again. Not worth a dedicated 404 race-check.
- **File mutated between list and restore** → checksums in the manifest are re-verified at restore time; mismatch aborts cleanly.
- **Very large backup dir (1000+ files)** → opening every archive to read manifest could be slow. Acceptable for v1 (typical case is <50); can add an `(filename, mtime, size)` cache later if it becomes a real problem.
- **`BACKUP_PATH` is the empty string** → treat as "no backup dir configured"; return empty list, no 500.
- **Race with the backup scheduler** → `RestoreFromArchive` returns `ErrOperationInProgress` → mapped to 409, same as the upload path.
- **Case sensitivity** → `*.tar.gz` matched case-sensitively. Matches Nexorious's own naming convention (always lowercase); on case-insensitive filesystems the on-disk casing is what's returned, and `.TAR.GZ` would be skipped. Acceptable.

## Out of scope

- Exposing the on-disk listing in the admin panel (post-setup). The issue is specifically about setup; deferring to a follow-up if needed.
- Recursing into subdirectories of `BACKUP_PATH`.
- Caching manifest reads across requests.
- Resumable / streamed listing for very large dirs.
- Adding authentication or rate-limiting to the new endpoints — they're behind the same setup-zone gate as the existing handler, which is the right boundary for setup operations.

## Slumber collection

Per [CLAUDE.md](../../CLAUDE.md), each new route gets an entry in [slumber.yaml](../../slumber.yaml):

- `GET /api/auth/setup/backups` — under the existing `setup/` folder, no `authentication` block (setup zone).
- `POST /api/auth/setup/restore/disk` — same folder, no `authentication` block, JSON body with `filename`.

Run `slumber collection` after editing to confirm the collection loads.

## Verification

- `go test ./...` — all backend tests pass.
- `golangci-lint run` — zero findings.
- `slumber collection` — loads without error.
- Manual setup-page smoke per the list above.

## References

- Issue: [#575](https://github.com/drzero42/nexorious/issues/575)
- Existing setup-restore handler: [internal/api/backup.go:328-388](../../internal/api/backup.go#L328-L388)
- Existing archive validation: [internal/backup/service.go:196-220](../../internal/backup/service.go#L196-L220)
- Setup page: [ui/setup/index.html](../../ui/setup/index.html)
- Backup config: [internal/config/config.go:63](../../internal/config/config.go#L63)

# Setup Restore from On-Disk Backups — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** During setup (no users yet), let the operator pick from backup archives already in `BACKUP_PATH` and restore one of them, while the existing upload path keeps working.

**Architecture:** New `Service.ListAvailableArchives` method scans `BACKUP_PATH` (top-level, `*.tar.gz` only) and classifies each file as restorable / not. Two new public setup-zone endpoints — `GET /api/auth/setup/backups` (list) and `POST /api/auth/setup/restore/disk` (restore by filename) — sit alongside the existing `POST /api/auth/setup/restore`. Disk-restore reuses `Service.RestoreFromUpload(path, opts)`. The setup HTML grows a list section above the existing upload zone.

**Tech Stack:** Go 1.25 + Bun + Echo v5 (backend). Vanilla HTML/JS for the setup page (embedded via `ui/ui.go`).

**Spec:** [docs/superpowers/specs/2026-05-21-issue-575-setup-restore-from-disk-design.md](../specs/2026-05-21-issue-575-setup-restore-from-disk-design.md)

**Branch:** `issue-575-setup-restore-from-disk` (already created on `main`).

---

## File Map

| File | What changes |
|---|---|
| `internal/backup/service.go` | Add `ArchiveInfo` struct, `ListAvailableArchives` method, and exported `BackupPath()` getter |
| `internal/backup/service_test.go` | Add `TestListAvailableArchives` (table-driven) |
| `internal/api/backup.go` | Add `requireNoUsers` helper; refactor `HandleSetupRestore` to use it; add `HandleSetupListBackups` and `HandleSetupRestoreFromDisk` |
| `internal/api/backup_test.go` | Add `TestHandleSetupRestoreFromDisk_FilenameValidation` and `TestSetupZoneRejectsWhenUsersExist` |
| `internal/api/router.go` | Register two new routes |
| `ui/setup/index.html` | Extend the restore view with on-disk listing above the upload zone |
| `slumber.yaml` | Add `list_setup_backups` and `setup_restore_disk` to the `bootstrap` folder |

The state-middleware gate at [internal/api/router.go:99](../../internal/api/router.go#L99) already allows `strings.HasPrefix(path, "/api/auth/setup")` through — no gate change needed.

---

## Task 1: Add `ArchiveInfo` struct and skeleton `ListAvailableArchives` (TDD: empty/missing dir)

**Files:**
- Modify: `internal/backup/service.go`
- Modify: `internal/backup/service_test.go` (extend)

- [ ] **Step 1: Write the failing test**

Append to [internal/backup/service_test.go](../../internal/backup/service_test.go):

```go
// ---------------------------------------------------------------------------
// ListAvailableArchives
// ---------------------------------------------------------------------------

func TestListAvailableArchives_NonExistentDir(t *testing.T) {
	// Backup dir that doesn't exist must produce empty result, not error.
	svc := backup.NewService(nil, "", "/nonexistent/path/that/does/not/exist", "", "0.1.0")
	infos, err := svc.ListAvailableArchives(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAvailableArchives: unexpected error: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected empty list, got %d entries", len(infos))
	}
}

func TestListAvailableArchives_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	svc := backup.NewService(nil, "", dir, "", "0.1.0")
	infos, err := svc.ListAvailableArchives(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAvailableArchives: unexpected error: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected empty list, got %d entries", len(infos))
	}
}
```

- [ ] **Step 2: Run test to verify it fails (compile error)**

```bash
go test ./internal/backup/... -run TestListAvailableArchives -v
```

Expected: compile error — `svc.ListAvailableArchives undefined`.

- [ ] **Step 3: Add the `ArchiveInfo` type and skeleton method to `internal/backup/service.go`**

Append after `ValidateArchive` (which ends around line 248). Add the `context` import if not already present (it should be — check existing imports first):

```go
// ArchiveInfo summarizes one candidate backup archive found in the backup
// directory. Files that fail to validate end-to-end (corrupt manifest,
// migration version newer than this binary supports, etc.) are still returned
// with Restorable=false and a human-readable Reason so the UI can show them.
type ArchiveInfo struct {
	Filename   string    `json:"filename"`     // base name only
	SizeBytes  int64     `json:"size_bytes"`
	ModTime    time.Time `json:"mtime"`
	Manifest   *Manifest `json:"manifest,omitempty"`
	Restorable bool      `json:"restorable"`
	Reason     string    `json:"reason,omitempty"`
}

// BackupPath returns the configured backup directory path. Exposed so handlers
// can safely resolve a user-supplied filename to a full path under it.
func (s *Service) BackupPath() string {
	return s.backupPath
}

// ListAvailableArchives scans the configured backup directory (top-level only)
// for *.tar.gz files and returns metadata for each. Files appear regardless of
// whether they validate so callers can show non-restorable files with an
// explanation. Sorted newest mtime first.
//
// Returns an empty slice (not an error) when the directory is empty,
// unreadable, or doesn't exist — listing is best-effort discovery.
func (s *Service) ListAvailableArchives(ctx context.Context, maxMigrationVersion string) ([]ArchiveInfo, error) {
	return nil, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/backup/... -run TestListAvailableArchives -v
```

Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/backup/service.go internal/backup/service_test.go
git commit -m "$(cat <<'EOF'
feat(backup): add ListAvailableArchives skeleton

Adds ArchiveInfo type, BackupPath getter, and a no-op
ListAvailableArchives method. Empty/missing backup directory returns
an empty slice without erroring.

Refs #575

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Implement full `ListAvailableArchives` logic (TDD: mixed-types case)

**Files:**
- Modify: `internal/backup/service.go`
- Modify: `internal/backup/service_test.go`

- [ ] **Step 1: Write the failing table-driven test**

Append to `internal/backup/service_test.go`:

```go
// writeValidManifestArchive writes a .tar.gz containing both a manifest.json
// and a stub database.sql so the listing predicate (which checks for the
// latter) treats it as restorable. migrationVersion sets the manifest's
// MigrationVersion field; backupType sets the BackupType field.
func writeValidManifestArchive(t *testing.T, dir, filename, migrationVersion, backupType string) string {
	t.Helper()
	manifest := backup.Manifest{
		Version:          backup.ManifestVersion,
		CreatedAt:        time.Now().UTC(),
		AppVersion:       "0.0.42",
		MigrationVersion: migrationVersion,
		BackupType:       backupType,
		DatabaseFile:     "database.sql",
		StatsUsers:       2,
		StatsGames:       1843,
		StatsTags:        27,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	writeFile := func(name string, body []byte) {
		hdr := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header for %s: %v", name, err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("write tar body for %s: %v", name, err)
		}
	}
	writeFile("manifest.json", data)
	writeFile("database.sql", []byte("-- stub"))

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return path
}

func TestListAvailableArchives_MixedContents(t *testing.T) {
	dir := t.TempDir()

	// 1. Valid archive at the current migration version (restorable).
	validPath := writeValidManifestArchive(t, dir, "nexorious-backup-20260520-093015.tar.gz", "20260518120000", "scheduled")

	// 2. Valid archive at a newer migration version (not restorable; version-incompatible).
	futurePath := writeValidManifestArchive(t, dir, "nexorious-backup-20260521-000001.tar.gz", "20260601000000", "scheduled")

	// 3. Foreign .tar.gz with random bytes (no readable manifest).
	weirdPath := createMinimalTarGz(t, dir, "not-manifest.txt", "garbage")
	// Rename so its filename doesn't collide with the helper default.
	if err := os.Rename(weirdPath, filepath.Join(dir, "weird.tar.gz")); err != nil {
		t.Fatalf("rename foreign archive: %v", err)
	}

	// 4. Non-.tar.gz file (must be skipped entirely).
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write notes.txt: %v", err)
	}

	// 5. Subdirectory containing a .tar.gz (must NOT recurse).
	subDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	writeValidManifestArchive(t, subDir, "should-be-invisible.tar.gz", "20260518120000", "manual")

	// Set mtimes so we can assert sort order: valid newest, weird middle, future oldest.
	now := time.Now()
	mustChtime(t, futurePath, now.Add(-2*time.Hour))
	mustChtime(t, filepath.Join(dir, "weird.tar.gz"), now.Add(-1*time.Hour))
	mustChtime(t, validPath, now)

	svc := backup.NewService(nil, "", dir, "", "0.1.0")
	infos, err := svc.ListAvailableArchives(context.Background(), "20260518120000")
	if err != nil {
		t.Fatalf("ListAvailableArchives: unexpected error: %v", err)
	}

	// Expect exactly 3 entries: the two .tar.gz archives and the foreign tar.gz.
	// notes.txt is filtered by extension; the subdir's tar.gz is filtered by no-recurse.
	if len(infos) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(infos), infos)
	}

	// Sort order: newest first.
	if infos[0].Filename != "nexorious-backup-20260520-093015.tar.gz" {
		t.Errorf("entry[0] = %s, want valid current-version archive", infos[0].Filename)
	}
	if infos[1].Filename != "weird.tar.gz" {
		t.Errorf("entry[1] = %s, want weird.tar.gz", infos[1].Filename)
	}
	if infos[2].Filename != "nexorious-backup-20260521-000001.tar.gz" {
		t.Errorf("entry[2] = %s, want future-version archive", infos[2].Filename)
	}

	// Per-entry assertions.
	if !infos[0].Restorable {
		t.Errorf("entry[0] expected Restorable=true, got false (reason=%q)", infos[0].Reason)
	}
	if infos[0].Manifest == nil || infos[0].Manifest.BackupType != "scheduled" {
		t.Errorf("entry[0] manifest not parsed correctly: %+v", infos[0].Manifest)
	}

	if infos[1].Restorable {
		t.Error("entry[1] (weird.tar.gz) expected Restorable=false")
	}
	if infos[1].Reason != "unreadable manifest" {
		t.Errorf("entry[1] reason = %q, want 'unreadable manifest'", infos[1].Reason)
	}
	if infos[1].Manifest != nil {
		t.Errorf("entry[1] expected nil manifest, got %+v", infos[1].Manifest)
	}

	if infos[2].Restorable {
		t.Error("entry[2] (future-version) expected Restorable=false")
	}
	if !strings.Contains(infos[2].Reason, "newer version") || !strings.Contains(infos[2].Reason, "20260601000000") {
		t.Errorf("entry[2] reason = %q, expected mention of newer migration version", infos[2].Reason)
	}
}

// mustChtime sets a file's mtime; fatal on error.
func mustChtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}
```

Add the new imports to the test file if missing: `encoding/json`, `strings`. (`archive/tar`, `compress/gzip`, `time`, `os`, `path/filepath` are already imported.)

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/backup/... -run TestListAvailableArchives_MixedContents -v
```

Expected: FAIL — "expected 3 entries, got 0".

- [ ] **Step 3: Replace the skeleton `ListAvailableArchives` body in `internal/backup/service.go`**

Replace the `return nil, nil` body with:

```go
func (s *Service) ListAvailableArchives(ctx context.Context, maxMigrationVersion string) ([]ArchiveInfo, error) {
	if s.backupPath == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.backupPath)
	if err != nil {
		// Missing dir / permission error is not fatal — listing is best-effort.
		slog.Debug("ListAvailableArchives: ReadDir failed", "path", s.backupPath, "err", err)
		return nil, nil
	}

	infos := make([]ArchiveInfo, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".tar.gz") {
			continue
		}
		fullPath := filepath.Join(s.backupPath, name)
		fi, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		// Only regular files. Skip symlinks, sockets, devices.
		if !fi.Mode().IsRegular() {
			continue
		}

		info := ArchiveInfo{
			Filename:  name,
			SizeBytes: fi.Size(),
			ModTime:   fi.ModTime().UTC(),
		}

		manifest, mErr := readManifestFromArchive(fullPath)
		switch {
		case mErr != nil:
			info.Restorable = false
			info.Reason = "unreadable manifest"
		case manifest.Version > MaxManifestVersion:
			info.Restorable = false
			info.Reason = fmt.Sprintf("unknown manifest version %d (max supported: %d)", manifest.Version, MaxManifestVersion)
			info.Manifest = manifest
		case maxMigrationVersion != "" && manifest.MigrationVersion > maxMigrationVersion:
			info.Restorable = false
			info.Reason = fmt.Sprintf(
				"backup was created by a newer version of Nexorious (migration %s); this binary only supports up to migration %s — upgrade before restoring",
				manifest.MigrationVersion, maxMigrationVersion,
			)
			info.Manifest = manifest
		default:
			// Final restorability check: database.sql must be present in the
			// archive. Mirrors the assertion ValidateArchive makes at restore
			// time, so a Restorable=true entry is a real promise.
			found, fErr := archiveContainsFile(fullPath, "database.sql")
			if fErr != nil || !found {
				info.Restorable = false
				info.Reason = "archive is missing database.sql"
			} else {
				info.Restorable = true
			}
			info.Manifest = manifest
		}

		infos = append(infos, info)
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ModTime.After(infos[j].ModTime)
	})
	return infos, nil
}
```

Add the `sort` import to `internal/backup/service.go` if not already imported.

- [ ] **Step 4: Run all backup tests to verify nothing else broke**

```bash
go test ./internal/backup/... -v
```

Expected: all tests PASS, including the existing `TestListBackups` and the new `TestListAvailableArchives_*`.

- [ ] **Step 5: Commit**

```bash
git add internal/backup/service.go internal/backup/service_test.go
git commit -m "$(cat <<'EOF'
feat(backup): implement ListAvailableArchives classifier

Scans backup dir top-level for *.tar.gz files. Returns valid,
version-incompatible, and unparseable archives all with appropriate
Restorable/Reason fields. Results sorted newest mtime first.

Refs #575

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Refactor `requireNoUsers` out of `HandleSetupRestore`

**Files:**
- Modify: `internal/api/backup.go`

- [ ] **Step 1: Add the helper method and refactor the existing handler**

In `internal/api/backup.go`, insert the helper above `HandleSetupRestore` (around line 327):

```go
// requireNoUsers enforces the setup-mode gate: any of the setup-zone restore
// handlers must reject with 403 if the users table is non-empty. Returns nil
// to continue; returns a non-nil error already sent to the client to
// short-circuit the handler.
func (h *BackupHandler) requireNoUsers(c *echo.Context) error {
	count, err := h.db.NewSelect().TableExpr("users").Count(c.Request().Context())
	if err == nil && count > 0 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "restore during setup is only available when no users exist"})
	}
	return nil
}
```

Then update [internal/api/backup.go:336-340](../../internal/api/backup.go#L336-L340) — replace the inline user-count check in `HandleSetupRestore`:

```go
	// Check that no users exist (setup mode only)
	count, err := h.db.NewSelect().TableExpr("users").Count(c.Request().Context())
	if err == nil && count > 0 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "restore during setup is only available when no users exist"})
	}
```

with:

```go
	if err := h.requireNoUsers(c); err != nil {
		return err
	}
```

- [ ] **Step 2: Run all api tests to confirm nothing broke**

```bash
go test ./internal/api/... -v
```

Expected: all tests PASS (the existing setup-restore behavior is unchanged).

- [ ] **Step 3: Commit**

```bash
git add internal/api/backup.go
git commit -m "$(cat <<'EOF'
refactor(api): extract requireNoUsers helper from HandleSetupRestore

Will be reused by upcoming setup-zone handlers (#575) so the
no-users gate has a single source of truth.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Implement `HandleSetupListBackups` and register the route

**Files:**
- Modify: `internal/api/backup.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Add the handler**

Append to `internal/api/backup.go` (after `HandleSetupRestore`):

```go
// HandleSetupListBackups lists candidate backup archives in the configured
// backup directory during initial setup (GET /api/auth/setup/backups).
func (h *BackupHandler) HandleSetupListBackups(c *echo.Context) error {
	if err := h.requireNoUsers(c); err != nil {
		return err
	}

	maxMigration := ""
	if h.callbacks != nil {
		maxMigration = h.callbacks.MaxMigration
	}

	infos, err := h.svc.ListAvailableArchives(c.Request().Context(), maxMigration)
	if err != nil {
		slog.Error("setup list backups failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list backups"})
	}

	type manifestDTO struct {
		CreatedAt        time.Time `json:"created_at"`
		AppVersion       string    `json:"app_version"`
		MigrationVersion string    `json:"migration_version"`
		BackupType       string    `json:"backup_type"`
		Stats            struct {
			Users int `json:"users"`
			Games int `json:"games"`
			Tags  int `json:"tags"`
		} `json:"stats"`
	}
	type entryDTO struct {
		Filename   string       `json:"filename"`
		SizeBytes  int64        `json:"size_bytes"`
		ModTime    time.Time    `json:"mtime"`
		Restorable bool         `json:"restorable"`
		Reason     string       `json:"reason,omitempty"`
		Manifest   *manifestDTO `json:"manifest,omitempty"`
	}

	out := make([]entryDTO, 0, len(infos))
	for _, info := range infos {
		e := entryDTO{
			Filename:   info.Filename,
			SizeBytes:  info.SizeBytes,
			ModTime:    info.ModTime,
			Restorable: info.Restorable,
			Reason:     info.Reason,
		}
		if info.Manifest != nil {
			m := &manifestDTO{
				CreatedAt:        info.Manifest.CreatedAt,
				AppVersion:       info.Manifest.AppVersion,
				MigrationVersion: info.Manifest.MigrationVersion,
				BackupType:       info.Manifest.BackupType,
			}
			m.Stats.Users = info.Manifest.StatsUsers
			m.Stats.Games = info.Manifest.StatsGames
			m.Stats.Tags = info.Manifest.StatsTags
			e.Manifest = m
		}
		out = append(out, e)
	}

	return c.JSON(http.StatusOK, map[string]any{"backups": out})
}
```

- [ ] **Step 2: Register the route in `internal/api/router.go`**

After [internal/api/router.go:187](../../internal/api/router.go#L187) (the existing `e.POST("/api/auth/setup/restore", bh.HandleSetupRestore)`), add:

```go
	e.GET("/api/auth/setup/backups", bh.HandleSetupListBackups)
```

- [ ] **Step 3: Sanity build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run api tests (existing ones must still pass)**

```bash
go test ./internal/api/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/backup.go internal/api/router.go
git commit -m "$(cat <<'EOF'
feat(api): add GET /api/auth/setup/backups

Lists candidate restore archives from BACKUP_PATH during setup. Each
entry includes filename, size, mtime, a Restorable flag, and (when
the manifest reads cleanly) a compact manifest projection with
created_at, versions, type, and stats.

Refs #575

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Implement `HandleSetupRestoreFromDisk` with filename-validation tests (TDD)

**Files:**
- Modify: `internal/api/backup.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/backup_test.go`

- [ ] **Step 1: Write the failing filename-validation test**

Append to `internal/api/backup_test.go`:

```go
// ---------------------------------------------------------------------------
// HandleSetupRestoreFromDisk
// ---------------------------------------------------------------------------

func TestHandleSetupRestoreFromDisk_FilenameValidation(t *testing.T) {
	truncateAllTables(t)

	// Set up a backup dir with one valid-looking filename present (used by the
	// symlink case — points to a target we create alongside it) and one absent
	// (for the 404 case).
	backupDir := t.TempDir()
	realFile := filepath.Join(backupDir, "real.tar.gz")
	if err := os.WriteFile(realFile, []byte("not-a-real-archive"), 0o644); err != nil {
		t.Fatalf("write real.tar.gz: %v", err)
	}
	symlinkName := "linked.tar.gz"
	if err := os.Symlink(realFile, filepath.Join(backupDir, symlinkName)); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	svc := backup.NewService(testDB, "", backupDir, "", "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)

	cases := []struct {
		name     string
		filename string
		wantCode int
		wantErr  string // substring
	}{
		{"empty", "", http.StatusBadRequest, "filename is required"},
		{"forward-slash", "../etc/passwd", http.StatusBadRequest, "invalid filename"},
		{"subdir", "sub/file.tar.gz", http.StatusBadRequest, "invalid filename"},
		{"backslash", `bad\name.tar.gz`, http.StatusBadRequest, "invalid filename"},
		{"dotdot-bare", "..", http.StatusBadRequest, "invalid filename"},
		{"not-in-dir", "nope.tar.gz", http.StatusNotFound, "backup not found"},
		{"symlink", symlinkName, http.StatusBadRequest, "invalid filename"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"filename": tc.filename})
			req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/restore/disk", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantErr) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tc.wantErr)
			}
		})
	}
}
```

Add to the test file's imports if missing: `os`, `path/filepath`, `strings`. (`bytes`, `encoding/json`, `net/http`, `net/http/httptest` are already imported.)

- [ ] **Step 2: Run the test to verify it fails (compile error — route not registered)**

```bash
go test ./internal/api/... -run TestHandleSetupRestoreFromDisk_FilenameValidation -v
```

Expected: every subtest FAILs with 404 (Echo returns 404 for an unregistered route) — not the expected status codes.

- [ ] **Step 3: Add the handler in `internal/api/backup.go`**

Append after `HandleSetupListBackups`:

```go
// HandleSetupRestoreFromDisk restores from a backup that already exists in the
// configured backup directory (POST /api/auth/setup/restore/disk). Body:
//
//	{ "filename": "nexorious-backup-20260520-093015.tar.gz" }
//
// Only top-level regular files inside the configured BACKUP_PATH are
// accepted. Symlinks are rejected. The file is never deleted after restore.
func (h *BackupHandler) HandleSetupRestoreFromDisk(c *echo.Context) error {
	if !backup.PsqlAvailable() {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "psql is not available on this system. Install PostgreSQL client tools to enable restore.",
		})
	}

	if err := h.requireNoUsers(c); err != nil {
		return err
	}

	var body struct {
		Filename string `json:"filename"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
	}

	filename := strings.TrimSpace(body.Filename)
	if filename == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "filename is required"})
	}

	// Layered path-traversal defense. None of these checks alone is enough.
	if strings.ContainsAny(filename, `/\`) || strings.Contains(filename, "..") || filepath.Base(filename) != filename {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}

	backupDir := h.svc.BackupPath()
	fullPath := filepath.Join(backupDir, filename)
	if filepath.Dir(fullPath) != filepath.Clean(backupDir) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}

	fi, err := os.Lstat(fullPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "backup not found"})
		}
		slog.Error("setup restore-from-disk lstat failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to inspect backup file"})
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}

	opts := h.makeRestoreOpts(true)
	if _, err := h.svc.RestoreFromUpload(fullPath, opts); err != nil {
		if errors.Is(err, backup.ErrOperationInProgress) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "A backup or restore operation is already in progress"})
		}
		slog.Error("setup restore-from-disk failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("restore failed: %v", err)})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "Backup restored successfully. Please log in with your restored credentials.",
	})
}
```

(`errors`, `fs`, `os`, `path/filepath`, `slog`, `fmt`, `strings`, `backup`, `http` are already imported in `internal/api/backup.go` per the imports block.)

- [ ] **Step 4: Register the route in `internal/api/router.go`**

After the GET line added in Task 4, add:

```go
	e.POST("/api/auth/setup/restore/disk", bh.HandleSetupRestoreFromDisk)
```

So the setup section ends up like:

```go
	bh := NewBackupHandler(backupSvc, db, restoreCallbacks)
	e.POST("/api/auth/setup/restore", bh.HandleSetupRestore)
	e.GET("/api/auth/setup/backups", bh.HandleSetupListBackups)
	e.POST("/api/auth/setup/restore/disk", bh.HandleSetupRestoreFromDisk)
```

- [ ] **Step 5: Run the validation test to verify it passes**

```bash
go test ./internal/api/... -run TestHandleSetupRestoreFromDisk_FilenameValidation -v
```

Expected: all 7 subtests PASS.

- [ ] **Step 6: Run the full api test suite to confirm no regressions**

```bash
go test ./internal/api/... -v
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/backup.go internal/api/router.go internal/api/backup_test.go
git commit -m "$(cat <<'EOF'
feat(api): add POST /api/auth/setup/restore/disk

Restores a backup that already exists in BACKUP_PATH during setup.
Layered filename validation (no /, \, .., must be a top-level regular
file under the configured dir, symlinks rejected) before handing off
to Service.RestoreFromUpload.

Refs #575

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Add the setup-zone gate test (both new handlers reject 403 when users exist)

**Files:**
- Modify: `internal/api/backup_test.go`

- [ ] **Step 1: Write the test**

Append to `internal/api/backup_test.go`:

```go
// TestSetupZoneRejectsWhenUsersExist asserts that both new setup-zone restore
// handlers — list and disk-restore — return 403 once a user exists. The
// shared requireNoUsers helper has a single test here for both routes.
func TestSetupZoneRejectsWhenUsersExist(t *testing.T) {
	truncateAllTables(t)

	backupDir := t.TempDir()
	svc := backup.NewService(testDB, "", backupDir, "", "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)

	// Create one admin so the gate is closed.
	_, _ = setupAdminUser(t, testDB, e, "setup-gate")

	// GET /api/auth/setup/backups → 403
	{
		req := httptest.NewRequest(http.MethodGet, "/api/auth/setup/backups", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET /backups: status = %d, want 403: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "no users exist") {
			t.Errorf("GET /backups: body missing expected error message: %s", rec.Body.String())
		}
	}

	// POST /api/auth/setup/restore/disk → 403
	{
		body, _ := json.Marshal(map[string]string{"filename": "anything.tar.gz"})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/restore/disk", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST /restore/disk: status = %d, want 403: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "no users exist") {
			t.Errorf("POST /restore/disk: body missing expected error message: %s", rec.Body.String())
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

```bash
go test ./internal/api/... -run TestSetupZoneRejectsWhenUsersExist -v
```

Expected: PASS.

- [ ] **Step 3: Run the full api test suite**

```bash
go test ./internal/api/... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/api/backup_test.go
git commit -m "$(cat <<'EOF'
test(api): assert setup-zone backup handlers reject 403 with users present

Refs #575

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Frontend — extend the restore view in `ui/setup/index.html`

**Files:**
- Modify: `ui/setup/index.html`

No automated tests — vanilla HTML page with no test infra. Manual smoke covered in Task 10.

- [ ] **Step 1: Modify the restore view's body**

Replace the contents of `<div id="restore-view" style="display:none">` ([ui/setup/index.html:45-69](../../ui/setup/index.html#L45-L69)) with:

```html
    <div id="restore-view" style="display:none">
      <div class="brand-row">
        <img src="/logo.svg" alt="">
        <h1 class="card-title">Restore from Backup</h1>
      </div>
      <p class="card-description">Pick a backup found on disk, or upload one.</p>
      <div class="alert-destructive" id="restore-err"></div>

      <div id="ondisk-section">
        <p id="ondisk-loading" class="card-description" style="margin:8px 0">Looking for backups on disk…</p>
        <ul id="ondisk-list" class="ondisk-list" style="display:none; list-style:none; padding:0; margin:0 0 12px 0"></ul>
        <p id="ondisk-empty" class="card-description" style="display:none; margin:8px 0">No backups found on disk — upload one instead.</p>
        <p id="ondisk-or" class="card-description" style="display:none; margin:12px 0 8px 0"><strong>Or upload a backup file</strong></p>
      </div>

      <input type="file" id="file-input" accept=".tar.gz" style="display:none">
      <div id="drop-zone" class="drop-zone" onclick="document.getElementById('file-input').click()">
        <div class="drop-zone-icon">📦</div>
        <strong>Click to select a backup file</strong>
        <span>.tar.gz files only</span>
      </div>
      <div id="file-info" class="file-info" style="display:none">
        <div class="file-info-text">
          <span class="file-info-name" id="file-name"></span>
          <span class="file-info-size" id="file-size"></span>
        </div>
        <button type="button" class="clear-btn" onclick="clearFile()" title="Remove">✕</button>
      </div>
      <button type="button" class="btn btn-primary" id="restore-btn" onclick="doRestore()" disabled>Restore</button>
      <div class="footer-row">
        <button type="button" class="btn-link" onclick="showAdmin()">Cancel — create a new account instead</button>
      </div>
    </div>
```

- [ ] **Step 2: Update the `showRestore` function and add the on-disk loader**

Replace the existing `showRestore` function ([ui/setup/index.html:73-77](../../ui/setup/index.html#L73-L77)) with:

```javascript
    function showRestore() {
      document.getElementById('admin-view').style.display = 'none';
      document.getElementById('restore-view').style.display = '';
      clearError('restore-err');
      loadOnDiskBackups();
    }

    // Format a UTC ISO date as a local "YYYY-MM-DD HH:MM" string.
    function fmtDate(iso) {
      const d = new Date(iso);
      if (isNaN(d.getTime())) return iso;
      const pad = n => String(n).padStart(2, '0');
      return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate())
        + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
    }

    function fmtCount(n) {
      return new Intl.NumberFormat().format(n);
    }

    let _ondiskLoaded = false;
    async function loadOnDiskBackups() {
      if (_ondiskLoaded) return;
      _ondiskLoaded = true;
      const loading = document.getElementById('ondisk-loading');
      const list = document.getElementById('ondisk-list');
      const empty = document.getElementById('ondisk-empty');
      const orLine = document.getElementById('ondisk-or');

      let backups = [];
      try {
        const res = await fetch('/api/auth/setup/backups');
        if (res.ok) {
          const data = await res.json();
          backups = Array.isArray(data.backups) ? data.backups : [];
        }
      } catch {
        // Fetch failure — fall through to "no backups" state.
      }

      loading.style.display = 'none';

      if (backups.length === 0) {
        empty.style.display = '';
        return;
      }

      backups.forEach(b => {
        const li = document.createElement('li');
        li.style.cssText = 'border:1px solid var(--border, #444); border-radius:6px; padding:10px 12px; margin-bottom:8px; display:flex; flex-direction:column; gap:4px';

        const header = document.createElement('div');
        header.style.cssText = 'display:flex; justify-content:space-between; align-items:center; gap:8px';
        const name = document.createElement('strong');
        name.textContent = b.filename;
        name.style.cssText = 'word-break:break-all';
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn btn-primary';
        btn.textContent = 'Restore';
        btn.style.cssText = 'flex-shrink:0';
        btn.disabled = !b.restorable;
        btn.onclick = () => restoreFromDisk(b.filename);
        header.appendChild(name);
        header.appendChild(btn);
        li.appendChild(header);

        const meta = document.createElement('div');
        meta.style.cssText = 'font-size:0.85em; opacity:0.75';
        const parts = [];
        if (b.manifest && b.manifest.created_at) parts.push(fmtDate(b.manifest.created_at));
        else parts.push(fmtDate(b.mtime));
        parts.push(formatBytes(b.size_bytes));
        if (b.manifest && b.manifest.backup_type) parts.push(b.manifest.backup_type);
        meta.textContent = parts.join(' · ');
        li.appendChild(meta);

        if (b.manifest && b.manifest.stats) {
          const stats = document.createElement('div');
          stats.style.cssText = 'font-size:0.85em; opacity:0.75';
          const s = b.manifest.stats;
          const v = b.manifest.app_version ? ' · v' + b.manifest.app_version : '';
          stats.textContent = fmtCount(s.games) + ' games · ' + fmtCount(s.users) + ' users · ' + fmtCount(s.tags) + ' tags' + v;
          li.appendChild(stats);
        }

        if (!b.restorable && b.reason) {
          const reason = document.createElement('div');
          reason.style.cssText = 'font-size:0.85em; color:var(--destructive, #f87171)';
          reason.textContent = '⚠ ' + b.reason;
          li.appendChild(reason);
        }

        list.appendChild(li);
      });

      list.style.display = '';
      orLine.style.display = '';
    }

    async function restoreFromDisk(filename) {
      if (!confirm('Restore from ' + filename + '? This will replace any existing data.')) return;
      clearError('restore-err');
      const buttons = document.querySelectorAll('#ondisk-list button, #restore-btn');
      buttons.forEach(b => b.disabled = true);
      try {
        const res = await fetch('/api/auth/setup/restore/disk', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ filename }),
        });
        if (res.ok) {
          window.location.href = '/login';
          return;
        }
        const body = await res.json().catch(() => ({}));
        showError('restore-err', body.error || 'Restore failed. Please try again.');
        buttons.forEach(b => { b.disabled = false; });
      } catch {
        showError('restore-err', 'Restore failed. Please try again.');
        buttons.forEach(b => { b.disabled = false; });
      }
    }
```

- [ ] **Step 3: Build the binary and smoke-test the page**

```bash
make build
```

Expected: build succeeds.

Manual smoke:

```bash
# Empty backup dir
rm -rf /tmp/nex-backups && mkdir -p /tmp/nex-backups
BACKUP_PATH=/tmp/nex-backups DATABASE_URL=... SECRET_KEY=... ./nexorious
# Visit http://localhost:8000/setup → click "Restore from backup instead"
# Expect: "Looking for backups on disk…" then "No backups found on disk — upload one instead."
# Upload zone still works.
```

For the populated-dir case, drop a previously-created `nexorious-backup-*.tar.gz` into `/tmp/nex-backups` and reload — list should show it with a working Restore button.

- [ ] **Step 4: Commit**

```bash
git add ui/setup/index.html
git commit -m "$(cat <<'EOF'
feat(setup): list on-disk backups in the restore view

Adds a section above the existing upload zone that lists candidate
backups from BACKUP_PATH (loaded from GET /api/auth/setup/backups).
Each item shows date, size, type, stats, and a Restore button that
calls POST /api/auth/setup/restore/disk. Empty/error states fall back
to upload-only.

Refs #575

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Add the new routes to `slumber.yaml`

**Files:**
- Modify: `slumber.yaml`

The existing `POST /api/auth/setup/restore` upload route is also missing from slumber.yaml — that's a pre-existing gap and out of scope here.

- [ ] **Step 1: Add entries inside the `bootstrap` folder**

Find the existing `create_admin` block ([slumber.yaml:32-40](../../slumber.yaml#L32-L40)) and append immediately after it (still inside `bootstrap.requests`):

```yaml
      list_setup_backups:
        name: List Setup Backups
        method: GET
        url: "{{base_url}}/api/auth/setup/backups"

      setup_restore_disk:
        name: Setup Restore from Disk
        method: POST
        url: "{{base_url}}/api/auth/setup/restore/disk"
        body:
          type: json
          data:
            filename: "{{response('list_setup_backups', trigger='no_history') | jsonpath('$.backups[*].filename', mode='array') | select()}}"
```

- [ ] **Step 2: Verify the collection loads**

```bash
slumber collection
```

Expected: command exits 0 (collection parses without error).

- [ ] **Step 3: Commit**

```bash
git add slumber.yaml
git commit -m "$(cat <<'EOF'
chore(slumber): add setup-zone backup discovery and disk-restore requests

Refs #575

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Final verification

- [ ] **Step 1: Full Go test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests PASS.

- [ ] **Step 2: Lint**

```bash
golangci-lint run
```

Expected: zero findings.

- [ ] **Step 3: Manual setup-page smoke (with three scenarios)**

Start with a fresh database (no users) so the setup gate is open.

```bash
make build
export DATABASE_URL=postgres://...
export SECRET_KEY=$(openssl rand -hex 32)
export BACKUP_PATH=/tmp/nex-backups
mkdir -p "$BACKUP_PATH"
```

**Scenario A — empty dir:** `rm -f $BACKUP_PATH/*.tar.gz`, run `./nexorious`, visit `/setup`, click "Restore from backup instead". Expect: list section absent, upload zone visible with the "No backups found on disk — upload one instead." hint.

**Scenario B — mixed contents:**
- Drop one valid `nexorious-backup-*.tar.gz` into `$BACKUP_PATH` (you can create one by running Nexorious with a real DB and using the admin "Create backup" flow once, then copying the archive out).
- Drop a `weird.tar.gz` with garbage bytes (`echo garbage > $BACKUP_PATH/weird.tar.gz`).
- Restart Nexorious, visit `/setup`, click restore. Expect: both files listed, valid one Restorable, `weird.tar.gz` disabled with "⚠ unreadable manifest".

**Scenario C — happy path:** click Restore on a valid entry, confirm the dialog, expect redirect to `/login`, and verify you can log in with the restored credentials.

**Scenario D — upload path regression:** repeat with an empty backup dir, upload a backup via the existing zone, expect redirect to `/login` and successful login.

- [ ] **Step 4: Push the branch and open a PR**

```bash
git push -u origin issue-575-setup-restore-from-disk
```

Then open a PR per CLAUDE.md's "Creating pull requests" guidelines. PR title format follows Conventional Commits (release-please parses the squash-merge title): `feat(setup): restore from on-disk backups during initial setup`.

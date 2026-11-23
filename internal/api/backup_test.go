package api_test

// Tests for backup API handlers.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/backup"
	"github.com/drzero42/nexorious/internal/migrate"
)

// newTestEchoBackup creates an Echo instance with a real backup service.
func newTestEchoBackup(t *testing.T, db *bun.DB, svc *backup.Service) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(testEncrypter, cfg, m, db, "", nil, svc, nil, "dev", "unknown", nil)
}

// newTestEchoBackupWithCallbacks is newTestEchoBackup with restore callbacks
// wired (so the restore compatibility window — MinMigration/MaxMigration — is
// active in the handler under test).
func newTestEchoBackupWithCallbacks(t *testing.T, db *bun.DB, svc *backup.Service, cb *api.RestoreCallbacks) interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
} {
	t.Helper()
	cfg := testCfg()
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(testEncrypter, cfg, m, db, "", nil, svc, cb, "dev", "unknown", nil)
}

// writeManifestArchive writes a minimal .tar.gz containing just a manifest.json
// with the given migration_version. Enough to drive ValidateArchive's
// version-compatibility checks (which run before the database.sql/checksum
// checks).
func writeManifestArchive(t *testing.T, dir, filename, migrationVersion string) {
	t.Helper()
	manifest := `{"version":1,"created_at":"2026-01-01T00:00:00Z","app_version":"0.15.0","migration_version":"` + migrationVersion + `","backup_type":"manual","database_file":"database.sql"}`
	f, err := os.Create(filepath.Join(dir, filename))
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	body := []byte(manifest)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "backup-x/manifest.json", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}

// TestHandleSetupRestoreFromDisk_PreV0171ShowsMessage is the regression test for
// the swallowed-message bug: a pre-v0.17.1 backup must be rejected with a clear,
// actionable 400 (not a generic "restore failed"), and the database must be left
// untouched so setup can be retried.
func TestHandleSetupRestoreFromDisk_PreV0171ShowsMessage(t *testing.T) {
	backup.CheckTools()
	if !backup.PsqlAvailable() {
		t.Skip("psql not available — setup restore-from-disk handler returns 503 before validation")
	}
	truncateAllTables(t) // no users → requireNoUsers gate open

	backupDir := t.TempDir()
	writeManifestArchive(t, backupDir, "old.tar.gz", "20260601000001") // below the floor

	svc := backup.NewService(testDB, "", backupDir, "", "0.90.0")
	cb := &api.RestoreCallbacks{
		MaxMigration: "20260620000001",
		MinMigration: "20260612000001",
	}
	e := newTestEchoBackupWithCallbacks(t, testDB, svc, cb)

	body, _ := json.Marshal(map[string]string{"filename": "old.tar.gz"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/restore/disk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "v0.17.1") {
		t.Errorf("body should explain the v0.17.1 requirement, got: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "restore failed") {
		t.Errorf("body should not be the generic 'restore failed', got: %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// HandleGetConfig
// ---------------------------------------------------------------------------

func TestHandleGetConfig_Success(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupAdminUser(t, testDB, e, "backup-getcfg")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/backups/config", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["schedule"]; !ok {
		t.Error("response missing 'schedule' field")
	}
}

// ---------------------------------------------------------------------------
// HandleUpdateConfig
// ---------------------------------------------------------------------------

func TestHandleUpdateConfig_InvalidSchedule(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupAdminUser(t, testDB, e, "backup-ucfg-sch")

	body, _ := json.Marshal(map[string]any{
		"schedule":        "biweekly",
		"schedule_time":   "02:00",
		"schedule_day":    0,
		"retention_mode":  "days",
		"retention_value": 7,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/backups/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUpdateConfig_InvalidRetentionMode(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupAdminUser(t, testDB, e, "backup-ucfg-ret")

	body, _ := json.Marshal(map[string]any{
		"schedule":        "daily",
		"schedule_time":   "02:00",
		"schedule_day":    0,
		"retention_mode":  "all_time",
		"retention_value": 7,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/backups/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUpdateConfig_InvalidRetentionValue(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupAdminUser(t, testDB, e, "backup-ucfg-retval")

	body, _ := json.Marshal(map[string]any{
		"schedule":        "daily",
		"schedule_time":   "02:00",
		"schedule_day":    0,
		"retention_mode":  "days",
		"retention_value": 0,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/backups/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUpdateConfig_InvalidCronTime(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupAdminUser(t, testDB, e, "backup-ucfg-cron")

	body, _ := json.Marshal(map[string]any{
		"schedule":        "weekly",
		"schedule_time":   "invalid",
		"schedule_day":    0,
		"retention_mode":  "days",
		"retention_value": 7,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/backups/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateConfig_Success(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupAdminUser(t, testDB, e, "backup-ucfg-ok")

	body, _ := json.Marshal(map[string]any{
		"schedule":        "daily",
		"schedule_time":   "03:00",
		"schedule_day":    0,
		"retention_mode":  "count",
		"retention_value": 5,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/backups/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// HandleListBackups
// ---------------------------------------------------------------------------

func TestHandleListBackups_EmptyList(t *testing.T) {
	truncateAllTables(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)
	_, tok := setupAdminUser(t, testDB, e, "backup-list-empty")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/backups", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["total"] != float64(0) {
		t.Errorf("expected total=0, got %v", body["total"])
	}
}

// ---------------------------------------------------------------------------
// HandleDeleteBackup
// ---------------------------------------------------------------------------

func TestHandleDeleteBackup_NotFound(t *testing.T) {
	truncateAllTables(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)
	_, tok := setupAdminUser(t, testDB, e, "backup-del-notfound")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/backups/nonexistent-backup", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// HandleDownloadBackup
// ---------------------------------------------------------------------------

func TestHandleDownloadBackup_NotFound(t *testing.T) {
	truncateAllTables(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)
	_, tok := setupAdminUser(t, testDB, e, "backup-dl-notfound")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/backups/nonexistent/download", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// HandleRestore
// ---------------------------------------------------------------------------

func TestHandleRestore_MissingConfirm(t *testing.T) {
	backup.CheckTools()
	if !backup.PsqlAvailable() {
		t.Skip("psql not available — handler returns 503 before checking confirm")
	}

	truncateAllTables(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)
	_, tok := setupAdminUser(t, testDB, e, "backup-restore-noconfirm")

	body, _ := json.Marshal(map[string]any{"confirm": false})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/backups/backup-123/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRestore_PsqlUnavailable(t *testing.T) {
	backup.CheckTools()
	if backup.PsqlAvailable() {
		t.Skip("psql is available — cannot test unavailable branch")
	}

	truncateAllTables(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)
	_, tok := setupAdminUser(t, testDB, e, "backup-restore-nopsql")

	body, _ := json.Marshal(map[string]any{"confirm": true})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/backups/backup-123/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// HandleRestoreUpload
// ---------------------------------------------------------------------------

func TestHandleRestoreUpload_MissingFile(t *testing.T) {
	backup.CheckTools()
	if !backup.PsqlAvailable() {
		t.Skip("psql not available")
	}

	truncateAllTables(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)
	_, tok := setupAdminUser(t, testDB, e, "backup-upload-nofile")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/backups/restore/upload", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// HandleCreateBackup
// ---------------------------------------------------------------------------

func TestHandleCreateBackup_PgDumpUnavailable(t *testing.T) {
	backup.CheckTools()
	if backup.PgDumpAvailable() {
		t.Skip("pg_dump is available — cannot test unavailable branch")
	}

	truncateAllTables(t)
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	svc := backup.NewService(nil, "", backupDir, storageDir, "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)
	_, tok := setupAdminUser(t, testDB, e, "backup-create-nopgdump")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/backups", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: tok})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// HandleSetupRestoreFromDisk
// ---------------------------------------------------------------------------

func TestHandleSetupRestoreFromDisk_FilenameValidation(t *testing.T) {
	backup.CheckTools()
	if !backup.PsqlAvailable() {
		t.Skip("psql not available — handler returns 503 before filename validation")
	}

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
		{"nul-byte", "real\x00.tar.gz", http.StatusBadRequest, "invalid filename"},
		{"trailing-whitespace", "real.tar.gz ", http.StatusBadRequest, "invalid filename"},
		{"leading-whitespace", " real.tar.gz", http.StatusBadRequest, "invalid filename"},
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

// TestHandleSetupListBackups_RejectsWhenUsersExist asserts the list endpoint's
// shared requireNoUsers gate. Always runs — no psql dependency.
func TestHandleSetupListBackups_RejectsWhenUsersExist(t *testing.T) {
	truncateAllTables(t)

	backupDir := t.TempDir()
	svc := backup.NewService(testDB, "", backupDir, "", "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)

	// Create one admin so the gate is closed.
	_, _ = setupAdminUser(t, testDB, e, "setup-gate-list")

	req := httptest.NewRequest(http.MethodGet, "/api/auth/setup/backups", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("GET /backups: status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	assertOnlyRefusal(t, rec, "no users exist")
}

// TestHandleSetupRestoreFromDisk_RejectsWhenUsersExist asserts the disk-restore
// endpoint's shared requireNoUsers gate. Skipped without psql because the
// handler runs PsqlAvailable() before requireNoUsers and would 503 first.
func TestHandleSetupRestoreFromDisk_RejectsWhenUsersExist(t *testing.T) {
	backup.CheckTools()
	if !backup.PsqlAvailable() {
		t.Skip("psql not available — disk-restore handler returns 503 before requireNoUsers")
	}

	truncateAllTables(t)

	backupDir := t.TempDir()
	svc := backup.NewService(testDB, "", backupDir, "", "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)

	// Create one admin so the gate is closed.
	_, _ = setupAdminUser(t, testDB, e, "setup-gate-disk")

	body, _ := json.Marshal(map[string]string{"filename": "anything.tar.gz"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/restore/disk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST /restore/disk: status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	assertOnlyRefusal(t, rec, "no users exist")
}

// TestHandleSetupRestore_RejectsWhenUsersExist asserts the upload-restore
// endpoint's requireNoUsers gate stops the handler: an unauthenticated caller
// must not reach the restore once a user exists.
func TestHandleSetupRestore_RejectsWhenUsersExist(t *testing.T) {
	backup.CheckTools()
	if !backup.PsqlAvailable() {
		t.Skip("psql not available — upload-restore handler returns 503 before requireNoUsers")
	}

	truncateAllTables(t)

	svc := backup.NewService(testDB, "", t.TempDir(), "", "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)
	_, _ = setupAdminUser(t, testDB, e, "setup-gate-upload")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, setupUploadRequest(t))
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST /restore: status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	assertOnlyRefusal(t, rec, "no users exist")
}

// TestSetupRestoreGate_FailsClosedWhenUserCountFails asserts that when the
// users count cannot be read, every setup-zone restore endpoint refuses
// instead of treating the instance as not yet set up. The table is empty, so
// only the failed query (cancelled request context) can cause the refusal.
func TestSetupRestoreGate_FailsClosedWhenUserCountFails(t *testing.T) {
	backup.CheckTools()
	truncateAllTables(t)

	svc := backup.NewService(testDB, "", t.TempDir(), "", "0.1.0")
	e := newTestEchoBackup(t, testDB, svc)

	diskBody, _ := json.Marshal(map[string]string{"filename": "anything.tar.gz"})
	diskReq := httptest.NewRequest(http.MethodPost, "/api/auth/setup/restore/disk", bytes.NewReader(diskBody))
	diskReq.Header.Set("Content-Type", "application/json")

	cases := []struct {
		name      string
		req       *http.Request
		needsPsql bool
	}{
		{"list", httptest.NewRequest(http.MethodGet, "/api/auth/setup/backups", nil), false},
		{"disk", diskReq, true},
		{"upload", setupUploadRequest(t), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsPsql && !backup.PsqlAvailable() {
				t.Skip("psql not available — handler returns 503 before requireNoUsers")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, tc.req.WithContext(ctx))
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503: %s", rec.Code, rec.Body.String())
			}
			assertOnlyRefusal(t, rec, "could not verify")
		})
	}
}

// setupUploadRequest builds a multipart POST to the setup-zone upload-restore
// endpoint carrying a small non-archive file.
func setupUploadRequest(t *testing.T) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "backup.tar.gz")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte("not an archive")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/restore", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// assertOnlyRefusal asserts the response body is exactly one JSON object whose
// "error" contains want. A second JSON value means the handler kept running
// after the gate wrote its refusal — the status code alone cannot show that,
// because the first WriteHeader wins.
func assertOnlyRefusal(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	dec := json.NewDecoder(rec.Body)
	var got map[string]string
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("decode refusal body: %v", err)
	}
	if !strings.Contains(got["error"], want) {
		t.Errorf("error = %q, want it to contain %q", got["error"], want)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		t.Errorf("handler continued after the refusal; extra body: %s (err=%v)", extra, err)
	}
}

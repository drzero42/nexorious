package api_test

// Tests for backup API handlers.

import (
	"bytes"
	"encoding/json"
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
	return api.New(testEncrypter, cfg, m, db, "", nil, svc, nil)
}

// ---------------------------------------------------------------------------
// HandleGetConfig
// ---------------------------------------------------------------------------

func TestHandleGetConfig_Success(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupAdminUser(t, testDB, e, "backup-getcfg")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/backups/config", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	req.Header.Set("Authorization", "Bearer "+tok)
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
	if !strings.Contains(rec.Body.String(), "no users exist") {
		t.Errorf("GET /backups: body missing expected error message: %s", rec.Body.String())
	}
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
	if !strings.Contains(rec.Body.String(), "no users exist") {
		t.Errorf("POST /restore/disk: body missing expected error message: %s", rec.Body.String())
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runBackup drives newRootCmd with the given args, pre-seeded against srvURL.
// Returns stdout+stderr combined and any execution error.
func runBackup(t *testing.T, srvURL string, args ...string) (string, error) {
	t.Helper()
	seedProfile(t, srvURL)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// sampleBackups returns a standard list payload for reuse across tests.
func sampleBackupsPayload() map[string]any {
	return map[string]any{
		"backups": []map[string]any{
			{
				"id": "bk-1", "created_at": "2026-06-18T00:00:00Z",
				"backup_type": "full", "size_bytes": 1048576,
				"stats": map[string]any{"users": 1, "games": 42, "tags": 5},
			},
			{
				"id": "bk-2", "created_at": "2026-06-17T00:00:00Z",
				"backup_type": "full", "size_bytes": 512,
				"stats": map[string]any{"users": 1, "games": 40, "tags": 5},
			},
		},
		"total": 2,
	}
}

func TestBackupListTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(sampleBackupsPayload())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "list")
	if err != nil {
		t.Fatalf("backup list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "bk-1") || !strings.Contains(out, "bk-2") {
		t.Errorf("table missing ids: %q", out)
	}
	if !strings.Contains(out, "1.0 MiB") {
		t.Errorf("table missing human size: %q", out)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("table missing games count: %q", out)
	}
}

func TestBackupListQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(sampleBackupsPayload())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "list", "-q")
	if err != nil {
		t.Fatalf("backup list -q: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "bk-1" || lines[1] != "bk-2" {
		t.Errorf("quiet ids = %q, want bk-1 and bk-2 on separate lines", out)
	}
}

func TestBackupListEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"backups": []any{}, "total": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "list")
	if err != nil {
		t.Fatalf("backup list empty: %v\n%s", err, out)
	}
	if !strings.Contains(out, "No backups.") {
		t.Errorf("output = %q, want 'No backups.'", out)
	}
}

func TestBackupCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"backup_id": "bk-new", "message": "backup started",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "create")
	if err != nil {
		t.Fatalf("backup create: %v\n%s", err, out)
	}
	if !strings.Contains(out, "bk-new") {
		t.Errorf("output missing backup id: %q", out)
	}
}

func TestBackupCreateQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"backup_id": "bk-q", "message": "backup started",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "create", "-q")
	if err != nil {
		t.Fatalf("backup create -q: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "bk-q" {
		t.Errorf("quiet output = %q, want bare id", out)
	}
}

func TestBackupRmConfirmed(t *testing.T) {
	var deleted bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "rm", "bk-1", "-y")
	if err != nil {
		t.Fatalf("backup rm -y: %v\n%s", err, out)
	}
	if !deleted {
		t.Fatal("DELETE not received")
	}
	if !strings.Contains(out, "removed backup bk-1") {
		t.Errorf("output = %q, want confirmation message", out)
	}
}

func TestBackupRmAborted(t *testing.T) {
	var deleteHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-1", func(w http.ResponseWriter, _ *http.Request) {
		deleteHit = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "rm", "bk-1")
	if err != nil {
		t.Fatalf("backup rm (no -y): %v\n%s", err, out)
	}
	if deleteHit {
		t.Fatal("DELETE must not be sent when aborted")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want 'Aborted.'", out)
	}
}

func TestBackupDownloadToFile(t *testing.T) {
	const payload = "fake-tar-gz-bytes"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-1/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_, _ = io.WriteString(w, payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	destPath := filepath.Join(t.TempDir(), "out.tar.gz")
	out, err := runBackup(t, srv.URL, "backup", "download", "bk-1", "--out", destPath)
	if err != nil {
		t.Fatalf("backup download --out: %v\n%s", err, out)
	}
	if !strings.Contains(out, "downloaded to") || !strings.Contains(out, destPath) {
		t.Errorf("output = %q, want 'downloaded to <path>'", out)
	}
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != payload {
		t.Errorf("file content = %q, want %q", got, payload)
	}
}

func TestBackupDownloadToStdout(t *testing.T) {
	const payload = "fake-tar-gz-bytes"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-2/download", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "download", "bk-2", "--out", "-")
	if err != nil {
		t.Fatalf("backup download --out -: %v\n%s", err, out)
	}
	if out != payload {
		t.Errorf("stdout = %q, want %q", out, payload)
	}
}

func TestBackupDownloadDefaultFilename(t *testing.T) {
	const payload = "bytes"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-3/download", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	out, err := runBackup(t, srv.URL, "backup", "download", "bk-3")
	if err != nil {
		t.Fatalf("backup download default: %v\n%s", err, out)
	}
	expected := "backup-bk-3.tar.gz"
	if !strings.Contains(out, expected) {
		t.Errorf("output = %q, want path to contain %q", out, expected)
	}
	got, err := os.ReadFile(filepath.Join(dir, expected))
	if err != nil {
		t.Fatalf("read default file: %v", err)
	}
	if string(got) != payload {
		t.Errorf("default file content = %q, want %q", got, payload)
	}
}

func TestBackupRestoreByIDConfirmed(t *testing.T) {
	var restored bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-1/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["confirm"] != true {
			t.Errorf("confirm = %v, want true", body["confirm"])
		}
		restored = true
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "restore started"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "restore", "bk-1", "-y")
	if err != nil {
		t.Fatalf("backup restore -y: %v\n%s", err, out)
	}
	if !restored {
		t.Fatal("POST /restore not received")
	}
	if !strings.Contains(out, "bk-1") {
		t.Errorf("output = %q, want backup id", out)
	}
}

func TestBackupRestoreBothIDAndFileErrors(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "backup.tar.gz", "data")
	_, err := runBackup(t, srv.URL, "backup", "restore", "bk-1", "--file", file, "-y")
	if err == nil {
		t.Fatal("expected error when both id and --file are given")
	}
	if !strings.Contains(err.Error(), "specify exactly one") {
		t.Errorf("error = %v, want it to mention 'specify exactly one'", err)
	}
}

func TestBackupRestoreNeitherIDNorFileErrors(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runBackup(t, srv.URL, "backup", "restore", "-y")
	if err == nil {
		t.Fatal("expected error when neither id nor --file given")
	}
	if !strings.Contains(err.Error(), "specify exactly one") {
		t.Errorf("error = %v, want it to mention 'specify exactly one'", err)
	}
}

func TestBackupRestoreUpload(t *testing.T) {
	const archiveContent = "fake-tar-gz"
	var gotBytes []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/restore/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		gotBytes = readMultipartFile(t, r)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "restore started"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "backup.tar.gz", archiveContent)
	out, err := runBackup(t, srv.URL, "backup", "restore", "--file", file, "-y")
	if err != nil {
		t.Fatalf("backup restore --file: %v\n%s", err, out)
	}
	if string(gotBytes) != archiveContent {
		t.Errorf("uploaded bytes = %q, want %q", gotBytes, archiveContent)
	}
	if !strings.Contains(out, "restore initiated") {
		t.Errorf("output = %q, want 'restore initiated'", out)
	}
}

func TestBackupScheduleShow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schedule":        "weekly",
			"schedule_time":   "02:00",
			"schedule_day":    0,
			"retention_mode":  "count",
			"retention_value": 7,
			"updated_at":      "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "schedule")
	if err != nil {
		t.Fatalf("backup schedule: %v\n%s", err, out)
	}
	if !strings.Contains(out, "weekly") || !strings.Contains(out, "02:00") {
		t.Errorf("output missing schedule fields: %q", out)
	}
}

func TestBackupScheduleShowJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schedule":        "daily",
			"schedule_time":   "03:00",
			"schedule_day":    0,
			"retention_mode":  "count",
			"retention_value": 5,
			"updated_at":      "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "schedule", "--json")
	if err != nil {
		t.Fatalf("backup schedule --json: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["schedule"] != "daily" {
		t.Errorf("schedule = %v, want daily", got["schedule"])
	}
}

func TestBackupScheduleSetFrequency(t *testing.T) {
	var getHit bool
	var putBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getHit = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schedule":        "manual",
				"schedule_time":   "00:00",
				"schedule_day":    0,
				"retention_mode":  "count",
				"retention_value": 3,
				"updated_at":      "2026-06-01T00:00:00Z",
			})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schedule":        "weekly",
				"schedule_time":   "00:00",
				"schedule_day":    0,
				"retention_mode":  "count",
				"retention_value": 3,
				"updated_at":      "2026-06-18T00:00:00Z",
			})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "schedule", "set", "--frequency", "weekly")
	if err != nil {
		t.Fatalf("backup schedule set --frequency: %v\n%s", err, out)
	}
	if !getHit {
		t.Error("expected GET /api/admin/backups/config before PUT")
	}
	if putBody["schedule"] != "weekly" {
		t.Errorf("PUT body schedule = %v, want weekly", putBody["schedule"])
	}
	// Unchanged fields must still be sent (we send the full config).
	if putBody["retention_value"] == nil {
		t.Errorf("PUT body missing retention_value: %v", putBody)
	}
	if !strings.Contains(out, "weekly") {
		t.Errorf("output = %q, want 'weekly'", out)
	}
}

func TestBackupScheduleSetRetention(t *testing.T) {
	var putBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schedule":        "weekly",
				"schedule_time":   "02:00",
				"schedule_day":    1,
				"retention_mode":  "count",
				"retention_value": 7,
				"updated_at":      "2026-06-01T00:00:00Z",
			})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schedule":        "weekly",
				"schedule_time":   "02:00",
				"schedule_day":    1,
				"retention_mode":  "days",
				"retention_value": 14,
				"updated_at":      "2026-06-18T00:00:00Z",
			})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "schedule", "set",
		"--retention-mode", "days", "--retention-value", "14")
	if err != nil {
		t.Fatalf("backup schedule set retention: %v\n%s", err, out)
	}
	if putBody["retention_mode"] != "days" {
		t.Errorf("PUT retention_mode = %v, want days", putBody["retention_mode"])
	}
	// JSON numbers decode to float64.
	if putBody["retention_value"] != float64(14) {
		t.Errorf("PUT retention_value = %v, want 14", putBody["retention_value"])
	}
	// Schedule fields must be carried through unchanged (full-struct PUT).
	if putBody["schedule"] != "weekly" {
		t.Errorf("PUT schedule = %v, want weekly (unchanged)", putBody["schedule"])
	}
	if !strings.Contains(out, "days") {
		t.Errorf("output = %q, want 'days'", out)
	}
}

func TestBackupScheduleSetJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schedule": "manual", "schedule_time": "00:00", "schedule_day": 0,
				"retention_mode": "count", "retention_value": 3, "updated_at": "2026-06-01T00:00:00Z",
			})
		case http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schedule": "daily", "schedule_time": "00:00", "schedule_day": 0,
				"retention_mode": "count", "retention_value": 3, "updated_at": "2026-06-18T00:00:00Z",
			})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "schedule", "set", "--frequency", "daily", "--json")
	if err != nil {
		t.Fatalf("backup schedule set --json: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["schedule"] != "daily" {
		t.Errorf("schedule = %v, want daily", got["schedule"])
	}
}

func TestBackupRestoreByIDAborted(t *testing.T) {
	var restoreHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-1/restore", func(w http.ResponseWriter, _ *http.Request) {
		restoreHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runBackup(t, srv.URL, "backup", "restore", "bk-1")
	if err != nil {
		t.Fatalf("backup restore (no -y): %v\n%s", err, out)
	}
	if restoreHit {
		t.Fatal("POST /restore must not be sent when aborted")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want 'Aborted.'", out)
	}
}

func TestBackupRestoreFileAborted(t *testing.T) {
	var uploadHit bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/restore/upload", func(w http.ResponseWriter, _ *http.Request) {
		uploadHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	file := writeTempFile(t, "backup.tar.gz", "data")
	out, err := runBackup(t, srv.URL, "backup", "restore", "--file", file)
	if err != nil {
		t.Fatalf("backup restore --file (no -y): %v\n%s", err, out)
	}
	if uploadHit {
		t.Fatal("upload must not be sent when aborted")
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("output = %q, want 'Aborted.'", out)
	}
}

func TestBackupScheduleSetOnlyChangedFlag(t *testing.T) {
	// Verify that schedule_time is NOT overridden when --time is not passed.
	var putBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schedule":        "manual",
				"schedule_time":   "04:30",
				"schedule_day":    2,
				"retention_mode":  "count",
				"retention_value": 10,
				"updated_at":      "2026-06-01T00:00:00Z",
			})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schedule":        "daily",
				"schedule_time":   "04:30",
				"schedule_day":    2,
				"retention_mode":  "count",
				"retention_value": 10,
				"updated_at":      "2026-06-18T00:00:00Z",
			})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runBackup(t, srv.URL, "backup", "schedule", "set", "--frequency", "daily")
	if err != nil {
		t.Fatalf("backup schedule set: %v", err)
	}
	// The PUT should carry the original schedule_time and schedule_day unchanged.
	if putBody["schedule_time"] != "04:30" {
		t.Errorf("schedule_time = %v, want 04:30 (unchanged)", putBody["schedule_time"])
	}
	if putBody["schedule"] != "daily" {
		t.Errorf("schedule = %v, want daily", putBody["schedule"])
	}
}

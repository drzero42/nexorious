package cliclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetBackupConfig(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schedule":        "daily",
			"schedule_time":   "03:00",
			"schedule_day":    0,
			"retention_mode":  "days",
			"retention_value": 30,
			"updated_at":      "2026-06-01T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	cfg, err := c.GetBackupConfig("k")
	if err != nil {
		t.Fatalf("GetBackupConfig: %v", err)
	}
	if cfg.Schedule != "daily" {
		t.Errorf("Schedule = %q, want daily", cfg.Schedule)
	}
	if cfg.ScheduleTime != "03:00" {
		t.Errorf("ScheduleTime = %q, want 03:00", cfg.ScheduleTime)
	}
	if cfg.RetentionMode != "days" {
		t.Errorf("RetentionMode = %q, want days", cfg.RetentionMode)
	}
	if cfg.RetentionValue != 30 {
		t.Errorf("RetentionValue = %d, want 30", cfg.RetentionValue)
	}
}

func TestUpdateBackupConfig(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["schedule"] != "weekly" {
			t.Errorf("schedule = %v, want weekly", body["schedule"])
		}
		if body["schedule_time"] != "02:30" {
			t.Errorf("schedule_time = %v, want 02:30", body["schedule_time"])
		}
		if body["retention_mode"] != "count" {
			t.Errorf("retention_mode = %v, want count", body["retention_mode"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schedule":        "weekly",
			"schedule_time":   "02:30",
			"schedule_day":    1,
			"retention_mode":  "count",
			"retention_value": 7,
			"updated_at":      "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	in := BackupConfig{
		Schedule:       "weekly",
		ScheduleTime:   "02:30",
		ScheduleDay:    1,
		RetentionMode:  "count",
		RetentionValue: 7,
	}
	out, err := c.UpdateBackupConfig("k", in)
	if err != nil {
		t.Fatalf("UpdateBackupConfig: %v", err)
	}
	if out.Schedule != "weekly" {
		t.Errorf("Schedule = %q, want weekly", out.Schedule)
	}
	if out.RetentionValue != 7 {
		t.Errorf("RetentionValue = %d, want 7", out.RetentionValue)
	}
}

func TestListBackups(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"backups": []map[string]any{
				{
					"id":          "bk-1",
					"created_at":  "2026-06-01T03:00:00Z",
					"backup_type": "manual",
					"size_bytes":  int64(1048576),
					"stats": map[string]any{
						"users": 1,
						"games": 42,
						"tags":  5,
					},
				},
				{
					"id":          "bk-2",
					"created_at":  "2026-06-08T03:00:00Z",
					"backup_type": "scheduled",
					"size_bytes":  int64(2097152),
					"stats": map[string]any{
						"users": 1,
						"games": 50,
						"tags":  6,
					},
				},
			},
			"total": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	backups, err := c.ListBackups("k")
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("len(backups) = %d, want 2", len(backups))
	}
	if backups[0].ID != "bk-1" || backups[0].BackupType != "manual" {
		t.Errorf("backups[0] = %+v", backups[0])
	}
	if backups[0].Stats.Games != 42 || backups[0].Stats.Tags != 5 {
		t.Errorf("backups[0].Stats = %+v", backups[0].Stats)
	}
	if backups[1].ID != "bk-2" || backups[1].SizeBytes != 2097152 {
		t.Errorf("backups[1] = %+v", backups[1])
	}
}

func TestCreateBackup(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"backup_id": "bk-new",
			"message":   "Backup created successfully",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	res, err := c.CreateBackup("k")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if res.BackupID != "bk-new" {
		t.Errorf("BackupID = %q, want bk-new", res.BackupID)
	}
	if res.Message != "Backup created successfully" {
		t.Errorf("Message = %q", res.Message)
	}
}

func TestCreateBackup_conflict(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "A backup or restore operation is already in progress"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	_, err := c.CreateBackup("k")
	if err == nil {
		t.Fatal("expected error on 409, got nil")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Errorf("error = %v, want 409", err)
	}
}

func TestDeleteBackup(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-del", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.DeleteBackup("k", "bk-del"); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
}

func TestDownloadBackup(t *testing.T) {
	const payload = "\x1f\x8b\x00mock-tar-gz-bytes"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-dl/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "wrong method", http.StatusMethodNotAllowed)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer k")
		}
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write([]byte(payload))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	var buf bytes.Buffer
	if err := c.DownloadBackup("k", "bk-dl", &buf); err != nil {
		t.Fatalf("DownloadBackup: %v", err)
	}
	if buf.String() != payload {
		t.Errorf("body = %q, want %q", buf.String(), payload)
	}
}

func TestDownloadBackup_nonOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-missing/download", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "backup file not found"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	var buf bytes.Buffer
	err := c.DownloadBackup("k", "bk-missing", &buf)
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want 404", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no bytes written on error, got %d", buf.Len())
	}
}

func TestRestoreBackup(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/bk-restore/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["confirm"] != true {
			t.Errorf("confirm = %v, want true", body["confirm"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "Restore completed from: bk-restore. All sessions have been cleared — please log in again.",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.RestoreBackup("k", "bk-restore"); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
}

func TestRestoreBackupUpload(t *testing.T) {
	payload := []byte("\x1f\x8b\x00this-is-a-fake-archive")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/backups/restore/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		got, _ := parseMultipartFile(t, r)
		if string(got) != string(payload) {
			t.Errorf("file bytes mismatch: got %q, want %q", got, payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "Restore completed from upload. All sessions have been cleared — please log in again.",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.RestoreBackupUpload("k", "backup.tar.gz", payload); err != nil {
		t.Fatalf("RestoreBackupUpload: %v", err)
	}
}

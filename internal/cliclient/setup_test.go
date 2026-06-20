package cliclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupListBackups(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/backups", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization header = %q; want empty (setup zone is unauthenticated)", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backups":[{"filename":"b.tar.gz","size_bytes":2048,"mtime":"2026-06-20T09:30:15Z","restorable":true,"manifest":{"app_version":"0.90.0","migration_version":"v0.90.0","backup_type":"manual","stats":{"users":1,"games":42,"tags":3}}}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	entries, err := New(srv.URL).SetupListBackups()
	if err != nil {
		t.Fatalf("SetupListBackups: %v", err)
	}
	if len(entries) != 1 || entries[0].Filename != "b.tar.gz" || entries[0].SizeBytes != 2048 || !entries[0].Restorable {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Manifest == nil || entries[0].Manifest.Stats.Games != 42 {
		t.Fatalf("manifest = %+v", entries[0].Manifest)
	}
}

func TestSetupListBackupsForbidden(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/backups", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"restore during setup is only available when no users exist"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := New(srv.URL).SetupListBackups()
	if err == nil || !strings.Contains(err.Error(), "no users exist") {
		t.Fatalf("err = %v; want forbidden message", err)
	}
}

func TestSetupRestoreFromDisk(t *testing.T) {
	var gotBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/restore/disk", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization header = %q; want empty", got)
		}
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"message":"Backup restored successfully."}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := New(srv.URL).SetupRestoreFromDisk("b.tar.gz"); err != nil {
		t.Fatalf("SetupRestoreFromDisk: %v", err)
	}
	if !strings.Contains(gotBody, `"filename":"b.tar.gz"`) {
		t.Fatalf("body = %q; want filename field", gotBody)
	}
}

func TestSetupRestoreUpload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/setup/restore", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization header = %q; want empty", got)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("Content-Type = %q; want multipart", r.Header.Get("Content-Type"))
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = f.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := New(srv.URL).SetupRestoreUpload("b.tar.gz", strings.NewReader("ARCHIVE-BYTES")); err != nil {
		t.Fatalf("SetupRestoreUpload: %v", err)
	}
}

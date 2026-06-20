package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMigrateAlreadyReady(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ran := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "ready"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) { ran = true })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "migrate", "--url", srv.URL)
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if ran {
		t.Fatal("migrate must not POST run when already ready")
	}
	if !strings.Contains(out, "No pending migrations.") {
		t.Fatalf("out = %q", out)
	}
}

func TestMigrateRunsAndWaits(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ran := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		state := "needs_migration"
		if ran {
			state = "ready"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"state": state})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "migrate", "--url", srv.URL)
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if !ran || !strings.Contains(out, "Migrations complete.") {
		t.Fatalf("ran=%v out=%q", ran, out)
	}
}

func TestMigrateStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "needs_migration", "pending_count": 2})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runNexctl(t, "", "migrate", "status", "--url", srv.URL)
	if err != nil {
		t.Fatalf("migrate status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "needs_migration") {
		t.Fatalf("out = %q", out)
	}
}

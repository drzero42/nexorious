package cliauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func TestPreflightOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := Preflight(&bytes.Buffer{}, cliclient.New(srv.URL), srv.URL); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
}

func TestPreflightRunsMigrations(t *testing.T) {
	ran := false
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "needs_migration"})
	})
	mux.HandleFunc("/api/migrate/run", func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "ready"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	var out bytes.Buffer
	if err := Preflight(&out, cliclient.New(srv.URL), srv.URL); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if !ran {
		t.Fatal("migrations were not triggered")
	}
	if !strings.Contains(out.String(), "Migrations complete.") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestPreflightMigrationFailedSurfacesDetail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "migration_failed"})
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "migration_failed", "error": "migration 003 failed: boom"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	err := Preflight(&bytes.Buffer{}, cliclient.New(srv.URL), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "previously failed") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v; want previously-failed + detail", err)
	}
}

func TestPreflightMigrationFailedStatusUnreachable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "migration_failed"})
	})
	mux.HandleFunc("/api/migrate/status", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	err := Preflight(&bytes.Buffer{}, cliclient.New(srv.URL), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "fetching the failure detail also failed") {
		t.Fatalf("err = %v; want status-fetch-failure surfaced", err)
	}
}

func TestReportSetupResultCreated(t *testing.T) {
	var out bytes.Buffer
	err := ReportSetupResult(&out, "admin", &cliclient.SetupResult{StatusCode: http.StatusCreated})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if !strings.Contains(out.String(), `Admin user "admin" created.`) {
		t.Fatalf("out = %q", out.String())
	}
}

func TestReportSetupResultRedirectToMigrate(t *testing.T) {
	err := ReportSetupResult(&bytes.Buffer{}, "admin", &cliclient.SetupResult{StatusCode: http.StatusFound, Location: "/migrate"})
	if err == nil || !strings.Contains(err.Error(), `run "nexctl migrate"`) {
		t.Fatalf("err = %v; want nexctl-migrate hint", err)
	}
}

func TestReportSetupResultForbidden(t *testing.T) {
	err := ReportSetupResult(&bytes.Buffer{}, "admin", &cliclient.SetupResult{StatusCode: http.StatusForbidden})
	if err == nil || !strings.Contains(err.Error(), "already complete") {
		t.Fatalf("err = %v; want already-complete", err)
	}
}

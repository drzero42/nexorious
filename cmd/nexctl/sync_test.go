package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// syncConfigsHandler returns a handler that serves the standard two-storefront
// config list used across sync tests.
func syncConfigsHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	lastSynced := "2026-06-01T12:00:00Z"
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"configs": []map[string]any{
				{
					"id":             "cfg-1",
					"storefront":     "steam",
					"frequency":      "daily",
					"last_synced_at": lastSynced,
					"is_configured":  true,
					"created_at":     "2026-01-01T00:00:00Z",
					"updated_at":     "2026-06-01T00:00:00Z",
				},
				{
					"id":             "cfg-2",
					"storefront":     "gog",
					"frequency":      "weekly",
					"last_synced_at": nil,
					"is_configured":  false,
					"created_at":     "2026-01-01T00:00:00Z",
					"updated_at":     "2026-01-01T00:00:00Z",
				},
			},
			"total": 2,
		})
	}
}

func TestSyncStatusTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/config", syncConfigsHandler(t))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync status: %v\n%s", err, out.String())
	}

	got := out.String()
	// Header row
	if !strings.Contains(got, "STOREFRONT") || !strings.Contains(got, "CONFIGURED") ||
		!strings.Contains(got, "FREQUENCY") || !strings.Contains(got, "LAST-SYNCED") {
		t.Errorf("missing header columns: %s", got)
	}
	// steam row: configured yes, daily, date
	if !strings.Contains(got, "steam") || !strings.Contains(got, "yes") || !strings.Contains(got, "daily") {
		t.Errorf("steam row missing or wrong: %s", got)
	}
	// gog row: configured no, weekly, never (nil last_synced_at)
	if !strings.Contains(got, "gog") || !strings.Contains(got, "no") || !strings.Contains(got, "never") {
		t.Errorf("gog row missing 'never': %s", got)
	}
}

func TestSyncStatusQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/config", syncConfigsHandler(t))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-q", "sync", "status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync status -q: %v\n%s", err, out.String())
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("quiet: expected 2 lines, got %d: %q", len(lines), out.String())
	}
	if lines[0] != "steam" || lines[1] != "gog" {
		t.Errorf("quiet lines = %v", lines)
	}
}

func TestSyncStatusStorefront(t *testing.T) {
	jobID := "job-42"
	lastSynced := "2026-06-01T12:00:00Z"

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/config", syncConfigsHandler(t))
	mux.HandleFunc("/api/sync/steam/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"storefront":          "steam",
			"is_syncing":          true,
			"last_synced_at":      lastSynced,
			"active_job_id":       jobID,
			"external_game_count": 314,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "status", "steam"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync status steam: %v\n%s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "steam") {
		t.Errorf("storefront missing: %s", got)
	}
	if !strings.Contains(got, "yes") {
		t.Errorf("syncing=yes missing: %s", got)
	}
	if !strings.Contains(got, jobID) {
		t.Errorf("active job id %q missing: %s", jobID, got)
	}
	if !strings.Contains(got, "314") {
		t.Errorf("external game count missing: %s", got)
	}
}

func TestSyncStatusUnknownStorefront(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/config", syncConfigsHandler(t))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "status", "bogus"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown storefront, got nil\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "unknown storefront") {
		t.Errorf("error should mention 'unknown storefront': %v", err)
	}
	if !strings.Contains(err.Error(), "steam") || !strings.Contains(err.Error(), "gog") {
		t.Errorf("error should list valid storefronts: %v", err)
	}
}

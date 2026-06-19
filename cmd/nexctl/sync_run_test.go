package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncConfigShow(t *testing.T) {
	lastSynced := "2026-06-01T12:00:00Z"
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "gog")
	mux.HandleFunc("/api/sync/config/gog", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":             "cfg-gog",
			"storefront":     "gog",
			"frequency":      "weekly",
			"last_synced_at": lastSynced,
			"is_configured":  true,
			"created_at":     "2026-01-01T00:00:00Z",
			"updated_at":     "2026-06-01T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"sync", "config", "gog"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync config gog: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "weekly") {
		t.Errorf("output missing frequency 'weekly': %s", got)
	}
	if !strings.Contains(got, "gog") {
		t.Errorf("output missing storefront 'gog': %s", got)
	}
}

func TestSyncConfigUpdate(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "gog")
	mux.HandleFunc("/api/sync/config/gog", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["frequency"] != "weekly" {
			t.Errorf("frequency = %v, want weekly", body["frequency"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":             "cfg-gog",
			"storefront":     "gog",
			"frequency":      "weekly",
			"last_synced_at": nil,
			"is_configured":  true,
			"created_at":     "2026-01-01T00:00:00Z",
			"updated_at":     "2026-06-18T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"sync", "config", "gog", "--frequency", "weekly"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync config gog --frequency weekly: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "weekly") {
		t.Errorf("output missing 'weekly': %s", got)
	}
}

func TestSyncRun(t *testing.T) {
	jobID := "job-999"
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	mux.HandleFunc("/api/sync/steam", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message":    "sync started",
			"job_id":     jobID,
			"storefront": "steam",
			"status":     "pending",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"sync", "run", "steam"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync run steam: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, jobID) {
		t.Errorf("output missing job id %q: %s", jobID, got)
	}
}

func TestSyncResetConfirmed(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	var deleted bool
	mux.HandleFunc("/api/sync/steam/data", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "reset", "steam", "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync reset steam -y: %v\n%s", err, out.String())
	}
	if !deleted {
		t.Fatal("DELETE /api/sync/steam/data not received")
	}
	if !strings.Contains(out.String(), "reset steam sync data") {
		t.Errorf("output = %q, want confirmation message", out.String())
	}
}

func TestSyncResetAborted(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	var deleteHit bool
	mux.HandleFunc("/api/sync/steam/data", func(w http.ResponseWriter, _ *http.Request) {
		deleteHit = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("")) // off-TTY: Confirm sees no input → aborts
	root.SetArgs([]string{"sync", "reset", "steam"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync reset (no -y): unexpected error: %v\n%s", err, out.String())
	}
	if deleteHit {
		t.Fatal("DELETE must not be sent when aborted")
	}
	if !strings.Contains(out.String(), "Aborted.") {
		t.Errorf("output = %q, want 'Aborted.'", out.String())
	}
}

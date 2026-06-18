package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveSyncConfigs registers the GET /api/sync/config handler that
// resolveStorefront relies on, advertising the given storefront slugs.
func serveSyncConfigs(mux *http.ServeMux, slugs ...string) {
	mux.HandleFunc("/api/sync/config", func(w http.ResponseWriter, _ *http.Request) {
		configs := make([]map[string]any, len(slugs))
		for i, s := range slugs {
			configs[i] = map[string]any{
				"id": "sc-" + s, "storefront": s, "frequency": "daily", "is_configured": true,
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"configs": configs, "total": len(configs)})
	})
}

func TestSyncConnectSteam(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	mux.HandleFunc("/api/sync/steam/connection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["steam_id"] != "76561198000000001" {
			t.Errorf("steam_id = %v", body["steam_id"])
		}
		if body["web_api_key"] != "DEADBEEF" {
			t.Errorf("web_api_key = %v", body["web_api_key"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"steam_username": "bob"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("")) // non-TTY: no prompting
	root.SetArgs([]string{"sync", "connect", "steam", "--steam-id", "76561198000000001", "--api-key", "DEADBEEF"})
	if err := root.Execute(); err != nil {
		t.Fatalf("connect: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "bob") {
		t.Fatalf("output = %q, want it to mention the username", out.String())
	}
}

func TestSyncConnectPSN(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "playstation-store")
	mux.HandleFunc("/api/sync/playstation-store/connection", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["npsso_token"] != "tok-123" {
			t.Errorf("npsso_token = %v", body["npsso_token"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"online_id": "psnuser"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"sync", "connect", "playstation-store", "--npsso", "tok-123"})
	if err := root.Execute(); err != nil {
		t.Fatalf("connect psn: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "psnuser") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSyncConnectSteamInvalid(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	mux.HandleFunc("/api/sync/steam/connection", func(w http.ResponseWriter, _ *http.Request) {
		// Steam reports bad credentials as HTTP 200 with valid:false.
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": "invalid_api_key"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"sync", "connect", "steam", "--steam-id", "76561198000000001", "--api-key", "BADKEY"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error for valid:false, got nil\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "invalid_api_key") {
		t.Fatalf("error = %v, want it to surface invalid_api_key", err)
	}
	if strings.Contains(out.String(), "connected") {
		t.Fatalf("must not print success on rejection: %q", out.String())
	}
}

func TestSyncConnectMissingFlagOffTTY(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("")) // non-TTY: must error, not hang
	root.SetArgs([]string{"sync", "connect", "steam", "--steam-id", "76561198000000001"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error for missing --api-key, got nil\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "--api-key") {
		t.Fatalf("error = %v, want it to mention --api-key", err)
	}
}

func TestSyncDisconnect(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	var deleted bool
	mux.HandleFunc("/api/sync/steam/connection", func(w http.ResponseWriter, r *http.Request) {
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
	root.SetArgs([]string{"sync", "disconnect", "steam", "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("disconnect: %v\n%s", err, out.String())
	}
	if !deleted {
		t.Fatal("DELETE not received")
	}
	if !strings.Contains(out.String(), "disconnected steam") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSyncDisconnectAbortsWithoutYes(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	var deleted bool
	mux.HandleFunc("/api/sync/steam/connection", func(w http.ResponseWriter, _ *http.Request) {
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
	root.SetIn(strings.NewReader("")) // non-TTY, no -y: must abort
	root.SetArgs([]string{"sync", "disconnect", "steam"})
	if err := root.Execute(); err != nil {
		t.Fatalf("disconnect abort: unexpected error %v", err)
	}
	if deleted {
		t.Fatal("DELETE must not be sent when not confirmed")
	}
	if !strings.Contains(out.String(), "Aborted.") {
		t.Fatalf("output = %q, want Aborted.", out.String())
	}
}

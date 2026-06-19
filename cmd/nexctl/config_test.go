package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runConfig drives newRootCmd with the given args, pre-seeded against srvURL.
// Returns stdout+stderr combined and any execution error.
func runConfig(t *testing.T, srvURL string, args ...string) (string, error) {
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

func TestConfigGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"deal_region": "us"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfig(t, srv.URL, "config", "get")
	if err != nil {
		t.Fatalf("config get: %v\n%s", err, out)
	}
	if !strings.Contains(out, "deal_region: us") {
		t.Errorf("output = %q, want 'deal_region: us'", out)
	}
}

func TestConfigGetJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"deal_region": "eu"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfig(t, srv.URL, "config", "get", "--json")
	if err != nil {
		t.Fatalf("config get --json: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["deal_region"] != "eu" {
		t.Errorf("deal_region = %v, want eu", got["deal_region"])
	}
}

func TestConfigSet(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"deal_region": "eu"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfig(t, srv.URL, "config", "set", "--deal-region", "eu")
	if err != nil {
		t.Fatalf("config set: %v\n%s", err, out)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/api/settings" {
		t.Errorf("path = %s, want /api/settings", gotPath)
	}
	if gotBody["deal_region"] != "eu" {
		t.Errorf("body deal_region = %v, want eu", gotBody["deal_region"])
	}
	if !strings.Contains(out, "deal_region: eu") {
		t.Errorf("output = %q, want 'deal_region: eu'", out)
	}
}

func TestConfigSetJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"deal_region": "us"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := runConfig(t, srv.URL, "config", "set", "--deal-region", "us", "--json")
	if err != nil {
		t.Fatalf("config set --json: %v\n%s", err, out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["deal_region"] != "us" {
		t.Errorf("deal_region = %v, want us", got["deal_region"])
	}
}

func TestConfigSetMissingFlag(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runConfig(t, srv.URL, "config", "set")
	if err == nil {
		t.Fatal("expected error when --deal-region is not provided")
	}
	if !strings.Contains(err.Error(), "deal-region") {
		t.Errorf("error = %v, want mention of 'deal-region'", err)
	}
}

func TestConfigSetServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid deal_region"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := runConfig(t, srv.URL, "config", "set", "--deal-region", "zz")
	if err == nil {
		t.Fatal("expected error from 422 response")
	}
	if !strings.Contains(err.Error(), "invalid deal_region") {
		t.Errorf("error = %v, want 'invalid deal_region' in message", err)
	}
}

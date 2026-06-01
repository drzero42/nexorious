package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

// seedProfile writes a logged-in config pointing at srvURL; callers set
// XDG_CONFIG_HOME to a temp dir first.
func seedProfile(t *testing.T, srvURL string) {
	t.Helper()
	cfg := &clicfg.Config{}
	cfg.SetProfile("default", clicfg.Profile{
		URL: srvURL, Username: "alice", KeyName: "cli@host", KeyID: "self-key", Key: "nxr_secret",
	})
	if err := clicfg.Save(cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// runCmd executes the root command with args and returns combined output.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestGenerateNotLoggedIn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := runCmd(t, "api-key", "generate", "--name", "x"); err == nil {
		t.Fatal("expected error when not logged in")
	}
}

func TestGenerateHappyPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var gotBody map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet { // dup-name check
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"k9","name":"ci","scopes":"write","key":"nxr_rawkey","created_at":"2026-01-01T00:00:00Z","expires_at":null}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "generate", "--name", "ci")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "nxr_rawkey") {
		t.Fatalf("output missing raw key: %s", out)
	}
	if !strings.Contains(out, "k9") || !strings.Contains(out, "never") {
		t.Fatalf("output missing id/expiry: %s", out)
	}
	if gotBody["scopes"] != "write" {
		t.Fatalf("default scopes = %q, want write", gotBody["scopes"])
	}
}

func TestGenerateInvalidScopes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seedProfile(t, "http://unused.example")
	out, err := runCmd(t, "api-key", "generate", "--name", "x", "--scopes", "admin")
	if err == nil {
		t.Fatalf("expected scopes validation error, output: %s", out)
	}
}

func TestGenerateMissingName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seedProfile(t, "http://unused.example")
	if _, err := runCmd(t, "api-key", "generate"); err == nil {
		t.Fatal("expected error when --name omitted")
	}
}

func TestGenerateDuplicateNameWarns(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"k1","name":"ci","scopes":"write","last_used_at":null,"created_at":"2026-01-01T00:00:00Z","expires_at":null}]`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"k2","name":"ci","scopes":"write","key":"nxr_rawkey","created_at":"2026-01-01T00:00:00Z","expires_at":null}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "generate", "--name", "ci")
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "already exists") {
		t.Fatalf("expected dup-name warning, got: %s", out)
	}
	if !strings.Contains(out, "nxr_rawkey") {
		t.Fatalf("key should still be created: %s", out)
	}
}

func TestGenerateServerError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "expires_at must be RFC3339"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	out, err := runCmd(t, "api-key", "generate", "--name", "x", "--expires-at", "nope")
	if err == nil {
		t.Fatalf("expected server error, output: %s", out)
	}
	if !strings.Contains(err.Error(), "expires_at must be RFC3339") {
		t.Fatalf("error = %v, want it to surface the server message", err)
	}
}

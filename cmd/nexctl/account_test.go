package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func accountTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAccountLoginWritesConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := accountTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"account", "login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\n%s", err, out.String())
	}

	cfg, _ := clicfg.Load()
	p, ok := cfg.CurrentProfile()
	if !ok || p.Key != "nxr_minted" || p.URL != srv.URL {
		t.Fatalf("stored profile = %+v ok=%v", p, ok)
	}
}

func TestAccountWhoami(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := accountTestServer(t)
	seed := &clicfg.Config{}
	seed.SetProfile("default", clicfg.Profile{URL: srv.URL, Username: "alice", Key: "nxr_x", KeyID: "key-1"})
	if err := clicfg.Save(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"account", "whoami"})
	if err := root.Execute(); err != nil {
		t.Fatalf("whoami: %v\n%s", err, out.String())
	}
	if got := out.String(); got == "" || !bytes.Contains([]byte(got), []byte("alice")) {
		t.Fatalf("whoami output = %q", got)
	}
}

// TestTopLevelLoginAlias verifies `nexctl login` works identically to
// `nexctl account login`.
func TestTopLevelLoginAlias(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := accountTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("top-level login: %v\n%s", err, out.String())
	}
	cfg, _ := clicfg.Load()
	if p, ok := cfg.CurrentProfile(); !ok || p.Key != "nxr_minted" {
		t.Fatalf("top-level login did not store key: %+v ok=%v", p, ok)
	}
}

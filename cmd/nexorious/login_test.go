package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func loginTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginWritesConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")
	srv := loginTestServer(t)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\noutput: %s", err, out.String())
	}

	cfg, err := clicfg.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := cfg.CurrentProfile()
	if !ok {
		t.Fatal("no current profile after login")
	}
	if p.Key != "nxr_minted" || p.KeyID != "key-1" {
		t.Fatalf("stored profile = %+v", p)
	}
	if p.URL != srv.URL || p.Username != "alice" {
		t.Fatalf("stored profile url/username = %+v", p)
	}
}

// TestLoginRotatesOldKey verifies that re-running login when a key is already
// stored revokes the previous key (via the session cookie) before minting and
// storing the new one.
func TestLoginRotatesOldKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NEXORIOUS_PASSWORD", "pw")

	var revokedID, revokeCookie string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-xyz"})
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		revokedID = r.URL.Path[len("/api/auth/api-keys/"):]
		if ck, _ := r.Cookie("session_id"); ck != nil {
			revokeCookie = ck.Value
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-2", "key": "nxr_new"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Seed an existing logged-in profile with an old key.
	seed := &clicfg.Config{}
	seed.SetProfile("default", clicfg.Profile{
		URL: srv.URL, Username: "alice", KeyName: "cli@host", KeyID: "key-1", Key: "nxr_old",
	})
	if err := clicfg.Save(seed); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"login", "--url", srv.URL, "--username", "alice"})
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\noutput: %s", err, out.String())
	}

	if revokedID != "key-1" {
		t.Fatalf("revoked id = %q, want old key key-1", revokedID)
	}
	if revokeCookie != "sess-xyz" {
		t.Fatalf("rotation revoke used cookie %q, want session sess-xyz", revokeCookie)
	}

	cfg, err := clicfg.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, _ := cfg.CurrentProfile()
	if p.Key != "nxr_new" || p.KeyID != "key-2" {
		t.Fatalf("stored profile not updated to new key: %+v", p)
	}
}

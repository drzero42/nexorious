package cliauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

func TestLoginAndStoreKeyWritesNamedProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-1"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-1", "key": "nxr_minted"})
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := &clicfg.Config{}
	if err := LoginAndStoreKey(&bytes.Buffer{}, cliclient.New(srv.URL), cfg, "work", srv.URL, "alice", "pw"); err != nil {
		t.Fatalf("LoginAndStoreKey: %v", err)
	}
	p, ok := cfg.ProfileNamed("work")
	if !ok || p.Key != "nxr_minted" || p.KeyID != "key-1" {
		t.Fatalf("profile work = %+v ok=%v", p, ok)
	}
}

func TestLoginAndStoreKeyRotatesExistingKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var revoked string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "sess-1"})
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "key-new", "key": "nxr_new"})
	})
	// Revoke of the previously stored key: DELETE /api/auth/api-keys/<id>.
	mux.HandleFunc("/api/auth/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		revoked = strings.TrimPrefix(r.URL.Path, "/api/auth/api-keys/")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := &clicfg.Config{}
	cfg.SetProfile("work", clicfg.Profile{URL: srv.URL, Username: "alice", KeyName: "cli@old", KeyID: "key-old", Key: "nxr_old"})

	if err := LoginAndStoreKey(&bytes.Buffer{}, cliclient.New(srv.URL), cfg, "work", srv.URL, "alice", "pw"); err != nil {
		t.Fatalf("LoginAndStoreKey: %v", err)
	}
	if revoked != "key-old" {
		t.Fatalf("revoked = %q, want key-old (existing key should be rotated out)", revoked)
	}
	p, ok := cfg.ProfileNamed("work")
	if !ok || p.Key != "nxr_new" || p.KeyID != "key-new" {
		t.Fatalf("profile work = %+v ok=%v, want freshly minted key", p, ok)
	}
}

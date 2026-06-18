package cliauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	p, ok := cfg.Profile("work")
	if !ok || p.Key != "nxr_minted" || p.KeyID != "key-1" {
		t.Fatalf("profile work = %+v ok=%v", p, ok)
	}
}

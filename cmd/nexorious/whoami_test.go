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

func TestWhoamiPrintsUser(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer nxr_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "unauthorized"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := &clicfg.Config{}
	cfg.SetProfile("default", clicfg.Profile{URL: srv.URL, Username: "alice", Key: "nxr_secret", KeyID: "k1"})
	if err := clicfg.Save(cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"whoami"})
	if err := root.Execute(); err != nil {
		t.Fatalf("whoami: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "alice") {
		t.Fatalf("output missing username: %q", out.String())
	}
}

func TestWhoamiNotLoggedIn(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"whoami"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error when not logged in")
	}
}

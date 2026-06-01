package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestLogoutRevokesAndClearsKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var revokedID, gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("revoke method = %q, want DELETE", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		revokedID = r.URL.Path[len("/api/auth/api-keys/"):]
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Seed a logged-in config.
	cfg := &clicfg.Config{}
	cfg.SetProfile("default", clicfg.Profile{
		URL: srv.URL, Username: "alice", KeyName: "cli@host", KeyID: "key-1", Key: "nxr_secret",
	})
	if err := clicfg.Save(cfg); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"logout"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logout: %v\n%s", err, out.String())
	}

	if revokedID != "key-1" {
		t.Fatalf("revoked id = %q, want key-1", revokedID)
	}
	if gotAuth != "Bearer nxr_secret" {
		t.Fatalf("auth = %q, want Bearer nxr_secret", gotAuth)
	}

	got, err := clicfg.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, _ := got.CurrentProfile()
	if p.Key != "" || p.KeyID != "" || p.KeyName != "" {
		t.Fatalf("credentials not cleared: %+v", p)
	}
	if p.URL != srv.URL || p.Username != "alice" {
		t.Fatalf("url/username should be retained: %+v", p)
	}
}

func TestLogoutNoStoredKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"logout"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error when not logged in")
	}
}

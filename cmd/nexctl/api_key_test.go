package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestAPIKeyListJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/api-keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "key-1", "name": "cli@host", "scopes": "write", "created_at": "2026-01-01T00:00:00Z"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	seed := &clicfg.Config{}
	seed.SetProfile("default", clicfg.Profile{URL: srv.URL, Key: "nxr_x", KeyID: "key-0"})
	if err := clicfg.Save(seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--json", "account", "api-key", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("\"id\": \"key-1\"")) {
		t.Fatalf("json output = %q", out.String())
	}
}

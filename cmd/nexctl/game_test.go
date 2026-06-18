package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// seedProfile writes a logged-in profile pointing at srvURL and returns nothing.
func seedProfile(t *testing.T, srvURL string) { //nolint:unused // used by Tasks 5–10 subcommand tests
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &clicfg.Config{}
	cfg.SetProfile("default", clicfg.Profile{URL: srvURL, Username: "alice", Key: "k", KeyID: "key-1"})
	if err := clicfg.Save(cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestResolveUserGameRefUniqueTitle(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "hollow" {
			t.Errorf("q = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{"id": "ug-1", "game": map[string]any{"id": 1, "title": "Hollow Knight"}}},
			"total":      1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cmd := newGameCmd()
	cmd.SetOut(&bytes.Buffer{})
	ug, err := resolveUserGameRef(cmd, cliclient.New(srv.URL), "k", "hollow")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ug.ID != "ug-1" {
		t.Fatalf("ug = %+v", ug)
	}
}

func TestResolveUserGameRefAmbiguousOffTTYErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{
				{"id": "ug-1", "game": map[string]any{"id": 1, "title": "Final Fantasy VII"}},
				{"id": "ug-2", "game": map[string]any{"id": 2, "title": "Final Fantasy X"}},
			},
			"total": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cmd := newGameCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(bytes.NewReader(nil)) // non-TTY
	_, err := resolveUserGameRef(cmd, cliclient.New(srv.URL), "k", "final fantasy")
	if err == nil {
		t.Fatal("expected ambiguous-ref error off-TTY")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("ug-1")) {
		t.Fatalf("error should list candidate ids: %v", err)
	}
}

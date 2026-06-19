package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGameListTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("play_status") != "completed" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{
				"id": "ug-1", "play_status": "completed",
				"game": map[string]any{"id": 1, "title": "Hollow Knight"},
			}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "list", "--status", "completed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Hollow Knight")) || !bytes.Contains(out.Bytes(), []byte("ug-1")) {
		t.Fatalf("table = %s", out.String())
	}
}

func TestGameListPoolName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "pool-42", "name": "Backlog"},
			{"id": "pool-99", "name": "Finished"},
		})
	})
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pool") != "pool-42" {
			t.Errorf("pool query = %q, want pool-42", r.URL.Query().Get("pool"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{"id": "ug-1", "game": map[string]any{"title": "X"}}},
			"total":      1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "list", "--pool", "Backlog"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list --pool: %v\n%s", err, out.String())
	}
}

func TestGameListQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{"id": "ug-1", "game": map[string]any{"title": "X"}}},
			"total":      1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-q", "game", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list -q: %v", err)
	}
	if strings.TrimSpace(out.String()) != "ug-1" {
		t.Fatalf("quiet = %q", out.String())
	}
}

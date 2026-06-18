package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGameRmByID(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	var deleted bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/"+id, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "game": map[string]any{"title": "Doomed"}})
		case http.MethodDelete:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-y", "game", "rm", id})
	if err := root.Execute(); err != nil {
		t.Fatalf("rm: %v\n%s", err, out.String())
	}
	if !deleted {
		t.Fatal("delete not called")
	}
}

// TestGameRmFilterCapWarns verifies a --filter selection whose Total exceeds the
// returned page emits a truncation warning to stderr.
func TestGameRmFilterCapWarns(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{"id": "ug-1", "game": map[string]any{"title": "A"}}},
			"total":      500,
		})
	})
	mux.HandleFunc("/api/user-games/ug-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-y", "game", "rm", "--filter", "--filter-wishlist"})
	if err := root.Execute(); err != nil {
		t.Fatalf("rm --filter: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("only the first")) {
		t.Fatalf("expected truncation warning, got: %s", out.String())
	}
}

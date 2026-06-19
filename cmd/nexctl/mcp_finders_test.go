package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func TestFindUserGamesByRefTitleMany(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "halo" {
			t.Errorf("q = %q", r.URL.Query().Get("q"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{
				{"id": "11111111-1111-1111-1111-111111111111", "game": map[string]any{"id": 1, "title": "Halo"}},
				{"id": "22222222-2222-2222-2222-222222222222", "game": map[string]any{"id": 2, "title": "Halo 2"}},
			},
			"total": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := findUserGamesByRef(cliclient.New(srv.URL), "k", "halo")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 matches, got %d", len(got))
	}
}

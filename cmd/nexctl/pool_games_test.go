package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPoolAddBulk(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	var added int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "p-1", "name": "Backlog"}})
	})
	// game refs are UUIDs → resolveUserGameRef does GET /api/user-games/<id>
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": r.URL.Path[len("/api/user-games/"):], "game": map[string]any{"title": "X"}})
	})
	mux.HandleFunc("/api/pools/p-1/games/bulk", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		added = len(b["user_game_ids"].([]any))
		_ = json.NewEncoder(w).Encode(map[string]any{"added": added})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "add", "Backlog", id, "223e4567-e89b-12d3-a456-426614174000"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool add: %v\n%s", err, out.String())
	}
	if added != 2 {
		t.Fatalf("added = %d", added)
	}
}

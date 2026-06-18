package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPoolQueueAddsThenOrders(t *testing.T) {
	const a = "123e4567-e89b-12d3-a456-426614174000"
	const b = "223e4567-e89b-12d3-a456-426614174000"
	var bulkIDs, queueIDs []any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "p-1", "name": "Backlog"}})
	})
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": r.URL.Path[len("/api/user-games/"):], "game": map[string]any{"title": "X"}})
	})
	mux.HandleFunc("/api/pools/p-1/games/bulk", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bulkIDs = body["user_game_ids"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"added": len(bulkIDs)})
	})
	mux.HandleFunc("/api/pools/p-1/queue", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		queueIDs = body["ids"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "queue", "Backlog", a, b})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool queue: %v\n%s", err, out.String())
	}
	if len(bulkIDs) != 2 || len(queueIDs) != 2 || queueIDs[0] != a {
		t.Fatalf("bulk=%v queue=%v", bulkIDs, queueIDs)
	}
}

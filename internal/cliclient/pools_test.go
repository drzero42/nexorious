package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPoolMutations(t *testing.T) {
	var queueIDs []any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "p-9", "name": "New"})
	})
	mux.HandleFunc("/api/pools/p-9/games/bulk", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		_ = json.NewEncoder(w).Encode(map[string]any{"added": len(b["user_game_ids"].([]any))})
	})
	mux.HandleFunc("/api/pools/p-9/queue", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		queueIDs = b["ids"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	p, err := c.CreatePool("k", "New", nil, nil)
	if err != nil || p.ID != "p-9" {
		t.Fatalf("CreatePool: %v %+v", err, p)
	}
	n, err := c.BulkAddPoolGames("k", "p-9", []string{"ug-1", "ug-2"})
	if err != nil || n != 2 {
		t.Fatalf("BulkAddPoolGames: %v %d", err, n)
	}
	if err := c.SetQueue("k", "p-9", []string{"ug-2", "ug-1"}); err != nil {
		t.Fatalf("SetQueue: %v", err)
	}
	if len(queueIDs) != 2 || queueIDs[0] != "ug-2" {
		t.Fatalf("queue order = %v", queueIDs)
	}
}

func TestListAndGetPool(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "p-1", "name": "Backlog", "position": 0, "has_filter": true, "queue_count": 2, "candidate_count": 5},
		})
	})
	mux.HandleFunc("/api/pools/p-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "p-1", "name": "Backlog", "position": 0, "has_filter": false,
			"queue":      []map[string]any{{"id": "ug-1", "game": map[string]any{"title": "A"}}},
			"candidates": []map[string]any{{"id": "ug-2", "game": map[string]any{"title": "B"}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	pools, err := c.ListPools("k")
	if err != nil || len(pools) != 1 || pools[0].Name != "Backlog" || pools[0].QueueCount != 2 {
		t.Fatalf("ListPools: %v %+v", err, pools)
	}
	d, err := c.GetPool("k", "p-1")
	if err != nil || len(d.Queue) != 1 || d.Queue[0].Title() != "A" || len(d.Candidates) != 1 {
		t.Fatalf("GetPool: %v %+v", err, d)
	}
}

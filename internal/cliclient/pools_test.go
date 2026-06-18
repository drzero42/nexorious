package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

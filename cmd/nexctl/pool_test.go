package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPoolListAndShow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "p-1", "name": "Backlog", "position": 0, "has_filter": true, "queue_count": 1, "candidate_count": 2},
		})
	})
	mux.HandleFunc("/api/pools/p-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "p-1", "name": "Backlog", "position": 0,
			"queue":      []map[string]any{{"id": "ug-1", "play_status": "in_progress", "game": map[string]any{"title": "Celeste"}}},
			"candidates": []map[string]any{{"id": "ug-2", "game": map[string]any{"title": "Hades"}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	// list
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool list: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Backlog")) {
		t.Fatalf("list = %s", out.String())
	}

	// show by name → resolves via list, then GET detail
	out.Reset()
	root = newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"pool", "show", "Backlog"})
	if err := root.Execute(); err != nil {
		t.Fatalf("pool show: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Celeste", "Hades", "QUEUE", "CANDIDATES"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("show missing %q: %s", want, out.String())
		}
	}
}

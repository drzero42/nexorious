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

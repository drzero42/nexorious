package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTagListAndCreate(t *testing.T) {
	var created bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			created = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-2", "name": "Co-op"})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "t-1", "name": "RPG", "game_count": 7}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"tag", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tag list: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("RPG")) || !bytes.Contains(out.Bytes(), []byte("7")) {
		t.Fatalf("list = %s", out.String())
	}

	out.Reset()
	root = newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"tag", "create", "Co-op"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tag create: %v", err)
	}
	if !created {
		t.Fatal("create not called")
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGameAcquire(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/platforms/simple-list", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "pc-windows", "display_name": "PC (Windows)"}})
	})
	mux.HandleFunc("/api/user-games/"+id, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "game": map[string]any{"title": "X"}})
	})
	mux.HandleFunc("/api/user-games/"+id+"/move-to-library", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		plats := body["platforms"].([]any)
		first := plats[0].(map[string]any)
		if first["platform"] != "pc-windows" || first["ownership_status"] != "owned" {
			t.Errorf("platforms = %v", plats)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "game": map[string]any{"title": "X"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "acquire", id, "--platform", "pc-windows"})
	if err := root.Execute(); err != nil {
		t.Fatalf("acquire: %v\n%s", err, out.String())
	}
}

func TestGameAcquireRequiresPlatform(t *testing.T) {
	seedProfile(t, "http://unused")
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"game", "acquire", "123e4567-e89b-12d3-a456-426614174000"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected --platform required error")
	}
}

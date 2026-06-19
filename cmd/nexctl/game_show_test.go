package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGameShow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/123e4567-e89b-12d3-a456-426614174000", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "123e4567-e89b-12d3-a456-426614174000", "play_status": "completed",
			"game":      map[string]any{"id": 1, "title": "Hollow Knight"},
			"platforms": []map[string]any{{"id": "p1", "platform": "pc_windows", "hours_played": 30.0}},
			"tags":      []map[string]any{{"id": "t1", "name": "Metroidvania"}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "show", "123e4567-e89b-12d3-a456-426614174000"})
	if err := root.Execute(); err != nil {
		t.Fatalf("show: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Hollow Knight", "completed", "pc_windows", "Metroidvania"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("show missing %q: %s", want, out.String())
		}
	}
}

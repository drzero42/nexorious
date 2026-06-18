package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGameEditStatusAndTags(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	var gotStatus string
	var gotTags []any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/"+id, func(w http.ResponseWriter, r *http.Request) {
		// GET for current tags (tag merge) and for ref resolution.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "game": map[string]any{"title": "X"},
			"tags": []map[string]any{{"id": "t1", "name": "RPG"}},
		})
	})
	mux.HandleFunc("/api/user-games/"+id+"/progress", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		gotStatus, _ = b["play_status"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	})
	mux.HandleFunc("/api/user-games/"+id+"/tags", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		gotTags = b["tags"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "edit", id, "--status", "completed", "--tag", "Favourite", "--untag", "RPG"})
	if err := root.Execute(); err != nil {
		t.Fatalf("edit: %v\n%s", err, out.String())
	}
	if gotStatus != "completed" {
		t.Fatalf("status = %q", gotStatus)
	}
	// current {RPG} + add {Favourite} - remove {RPG} = {Favourite}
	if len(gotTags) != 1 || gotTags[0] != "Favourite" {
		t.Fatalf("tags = %v", gotTags)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGameAddByIGDBID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/games/igdb/2131", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"games": []map[string]any{{"igdb_id": 2131, "title": "Hollow Knight"}}, "total": 1,
		})
	})
	var imported, created bool
	mux.HandleFunc("/api/games/igdb-import", func(w http.ResponseWriter, _ *http.Request) {
		imported = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 2131, "title": "Hollow Knight"})
	})
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		created = true
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["game_id"].(float64) != 2131 || body["play_status"] != "in_progress" {
			t.Errorf("body = %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ug-1", "game": map[string]any{"title": "Hollow Knight"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "add", "--igdb-id", "2131", "--status", "in_progress"})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v\n%s", err, out.String())
	}
	if !imported || !created {
		t.Fatalf("imported=%v created=%v", imported, created)
	}
}

func TestGameAddWishlistPlatformConflict(t *testing.T) {
	seedProfile(t, "http://unused")
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "add", "--igdb-id", "1", "--wishlist", "--platform", "pc_windows"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected --wishlist/--platform conflict error")
	}
}

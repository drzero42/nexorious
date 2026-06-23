package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGameAddByQuery(t *testing.T) {
	mux := http.NewServeMux()
	var searchQuery string
	mux.HandleFunc("/api/games/search/igdb", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		searchQuery, _ = body["query"].(string)
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
	// The query is forwarded verbatim; the backend performs the ID inference.
	root.SetArgs([]string{"game", "add", "igdb:2131", "--status", "in_progress"})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v\n%s", err, out.String())
	}
	if !imported || !created {
		t.Fatalf("imported=%v created=%v", imported, created)
	}
	if searchQuery != "igdb:2131" {
		t.Errorf("expected query forwarded verbatim, got %q", searchQuery)
	}
}

func TestGameAddAmbiguousSuggestsIDForm(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/games/search/igdb", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"games": []map[string]any{
				{"igdb_id": 1, "title": "Doom"},
				{"igdb_id": 2, "title": "Doom Eternal"},
			}, "total": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	// Non-interactive: an ambiguous search must error and point at igdb:<id>.
	root.SetArgs([]string{"game", "add", "doom"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected ambiguity error for multiple matches")
	}
	if !strings.Contains(err.Error(), "igdb:<id>") {
		t.Errorf("expected error to suggest the igdb:<id> form, got %q", err.Error())
	}
}

func TestGameAddWishlistPlatformConflict(t *testing.T) {
	seedProfile(t, "http://unused")
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "add", "anything", "--wishlist", "--platform", "pc_windows"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected --wishlist/--platform conflict error")
	}
}

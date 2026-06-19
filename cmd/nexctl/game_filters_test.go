package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func filtersServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/filter-options", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"genres": ["RPG", "Strategy"],
			"game_modes": ["Single player"],
			"themes": ["Fantasy"],
			"player_perspectives": ["First person"]
		}`))
	})
	mux.HandleFunc("/api/platforms/storefronts/simple-list", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"name": "steam", "display_name": "Steam"},
			{"name": "gog", "display_name": "GOG"}
		]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGameFiltersTable(t *testing.T) {
	srv := filtersServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "filters"})
	if err := root.Execute(); err != nil {
		t.Fatalf("filters: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"Play statuses", "not_started", "in_progress", "replay",
		"Ownership statuses", "owned", "no_longer_owned",
		"Storefronts", "steam", "Steam", "gog",
		"Genres", "RPG", "Strategy",
		"Game modes", "Single player",
		"Themes", "Fantasy",
		"Player perspectives", "First person",
	} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("filters table missing %q:\n%s", want, out.String())
		}
	}
}

func TestGameFiltersJSON(t *testing.T) {
	srv := filtersServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "filters", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("filters --json: %v\n%s", err, out.String())
	}
	var f struct {
		PlayStatuses      []string `json:"play_statuses"`
		OwnershipStatuses []string `json:"ownership_statuses"`
		Storefronts       []struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"storefronts"`
		Genres             []string `json:"genres"`
		GameModes          []string `json:"game_modes"`
		Themes             []string `json:"themes"`
		PlayerPerspectives []string `json:"player_perspectives"`
	}
	if err := json.Unmarshal(out.Bytes(), &f); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if len(f.PlayStatuses) != 8 || f.PlayStatuses[0] != "not_started" {
		t.Fatalf("play_statuses = %v", f.PlayStatuses)
	}
	if len(f.OwnershipStatuses) != 5 || f.OwnershipStatuses[0] != "owned" {
		t.Fatalf("ownership_statuses = %v", f.OwnershipStatuses)
	}
	if len(f.Storefronts) != 2 || f.Storefronts[0].Name != "steam" || f.Storefronts[0].DisplayName != "Steam" {
		t.Fatalf("storefronts = %+v", f.Storefronts)
	}
	if len(f.Genres) != 2 || f.Genres[0] != "RPG" {
		t.Fatalf("genres = %v", f.Genres)
	}
	if len(f.PlayerPerspectives) != 1 || f.PlayerPerspectives[0] != "First person" {
		t.Fatalf("player_perspectives = %v", f.PlayerPerspectives)
	}
}

func TestGameFiltersQuiet(t *testing.T) {
	srv := filtersServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "filters", "-q"})
	if err := root.Execute(); err != nil {
		t.Fatalf("filters -q: %v\n%s", err, out.String())
	}
	// Quiet emits bare values, one per line, with storefront slugs (not display names).
	lines := map[string]bool{}
	for _, l := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n")) {
		lines[string(bytes.TrimSpace(l))] = true
	}
	for _, want := range []string{"not_started", "owned", "steam", "RPG", "Single player", "Fantasy", "First person"} {
		if !lines[want] {
			t.Fatalf("quiet output missing bare value %q:\n%s", want, out.String())
		}
	}
	if lines["Steam"] {
		t.Fatalf("quiet output should use storefront slugs, not display names:\n%s", out.String())
	}
}

package cliclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetFilterOptions(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/filter-options", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"genres": ["RPG", "Strategy"],
			"game_modes": ["Single player"],
			"themes": ["Fantasy"],
			"player_perspectives": ["First person"]
		}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	opts, err := New(srv.URL).GetFilterOptions("nxr_secret")
	if err != nil {
		t.Fatalf("GetFilterOptions: %v", err)
	}
	if gotAuth != "Bearer nxr_secret" {
		t.Fatalf("auth = %q, want Bearer nxr_secret", gotAuth)
	}
	if len(opts.Genres) != 2 || opts.Genres[0] != "RPG" {
		t.Fatalf("genres = %v", opts.Genres)
	}
	if len(opts.GameModes) != 1 || opts.GameModes[0] != "Single player" {
		t.Fatalf("game_modes = %v", opts.GameModes)
	}
	if len(opts.Themes) != 1 || opts.Themes[0] != "Fantasy" {
		t.Fatalf("themes = %v", opts.Themes)
	}
	if len(opts.PlayerPerspectives) != 1 || opts.PlayerPerspectives[0] != "First person" {
		t.Fatalf("player_perspectives = %v", opts.PlayerPerspectives)
	}
}

func TestListStorefronts(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/platforms/storefronts/simple-list", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"name": "steam", "display_name": "Steam"},
			{"name": "gog", "display_name": "GOG"}
		]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	sfs, err := New(srv.URL).ListStorefronts("nxr_secret")
	if err != nil {
		t.Fatalf("ListStorefronts: %v", err)
	}
	if gotAuth != "Bearer nxr_secret" {
		t.Fatalf("auth = %q, want Bearer nxr_secret", gotAuth)
	}
	if len(sfs) != 2 || sfs[0].Name != "steam" || sfs[0].DisplayName != "Steam" {
		t.Fatalf("storefronts = %+v", sfs)
	}
}

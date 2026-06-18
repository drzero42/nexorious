package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSearchIGDB(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/games/search/igdb", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth = %q", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["query"] != "hollow" {
			t.Errorf("query = %v", body["query"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"games": []map[string]any{{"igdb_id": 2131, "title": "Hollow Knight", "release_date": "2017-02-24"}},
			"total": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).SearchIGDB("k", "hollow", 10)
	if err != nil {
		t.Fatalf("SearchIGDB: %v", err)
	}
	if res.Total != 1 || len(res.Games) != 1 || res.Games[0].IgdbID != 2131 || res.Games[0].Title != "Hollow Knight" {
		t.Fatalf("res = %+v", res)
	}
}

func TestImportIGDBGame(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/games/igdb-import", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 2131, "title": "Hollow Knight"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	g, err := New(srv.URL).ImportIGDBGame("k", 2131)
	if err != nil {
		t.Fatalf("ImportIGDBGame: %v", err)
	}
	if g.ID != 2131 || g.Title != "Hollow Knight" {
		t.Fatalf("game = %+v", g)
	}
}

func TestListUserGames(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("play_status") != "completed" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{
				"id": "ug-1", "play_status": "completed", "hours_played": 12.5,
				"game": map[string]any{"id": 2131, "title": "Hollow Knight"},
				"tags": []map[string]any{{"id": "t1", "name": "Metroidvania"}},
			}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).ListUserGames("k", url.Values{"play_status": {"completed"}})
	if err != nil {
		t.Fatalf("ListUserGames: %v", err)
	}
	if res.Total != 1 || res.UserGames[0].Game.Title != "Hollow Knight" || res.UserGames[0].HoursPlayed != 12.5 {
		t.Fatalf("res = %+v", res.UserGames[0])
	}
	if len(res.UserGames[0].Tags) != 1 || res.UserGames[0].Tags[0].Name != "Metroidvania" {
		t.Fatalf("tags = %+v", res.UserGames[0].Tags)
	}
}

func TestGetUserGame(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/ug-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ug-1", "game": map[string]any{"id": 2131, "title": "Hollow Knight"},
			"platforms": []map[string]any{{"id": "p1", "platform": "pc_windows", "hours_played": 12.5}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ug, err := New(srv.URL).GetUserGame("k", "ug-1")
	if err != nil {
		t.Fatalf("GetUserGame: %v", err)
	}
	if ug.ID != "ug-1" || len(ug.Platforms) != 1 || *ug.Platforms[0].Platform != "pc_windows" {
		t.Fatalf("ug = %+v", ug)
	}
}

func TestCreateUserGameAndReplaceTags(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["game_id"].(float64) != 2131 {
			t.Errorf("game_id = %v", body["game_id"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ug-9", "game_id": 2131})
	})
	mux.HandleFunc("/api/user-games/ug-9/tags", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		tags := body["tags"].([]any)
		if len(tags) != 2 {
			t.Errorf("tags = %v", tags)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ug-9", "tags": []map[string]any{{"id": "t1", "name": "RPG"}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	ug, err := c.CreateUserGame("k", CreateUserGameInput{GameID: 2131, PlayStatus: "not_started"})
	if err != nil || ug.ID != "ug-9" {
		t.Fatalf("CreateUserGame: %v %+v", err, ug)
	}
	if _, err := c.ReplaceTags("k", "ug-9", []string{"RPG", "Backlog"}); err != nil {
		t.Fatalf("ReplaceTags: %v", err)
	}
}

func TestDeleteUserGame(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/ug-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	if err := New(srv.URL).DeleteUserGame("k", "ug-1"); err != nil {
		t.Fatalf("DeleteUserGame: %v", err)
	}
}

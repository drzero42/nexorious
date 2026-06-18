package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

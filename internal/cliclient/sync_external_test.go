package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListExternalGames(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/steam/external-games", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":          "eg-1",
				"storefront":  "steam",
				"external_id": "12345",
				"title":       "Portal 2",
				"sync_status": "matched",
				"platforms":   []string{"pc-windows"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	games, err := c.ListExternalGames("k", "steam")
	if err != nil {
		t.Fatalf("ListExternalGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("len(games) = %d, want 1", len(games))
	}
	if games[0].Title != "Portal 2" {
		t.Errorf("Title = %q, want %q", games[0].Title, "Portal 2")
	}
	if games[0].SyncStatus != "matched" {
		t.Errorf("SyncStatus = %q, want %q", games[0].SyncStatus, "matched")
	}
}

func TestRematchExternalGame(t *testing.T) {
	var receivedBodies []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/external-games/eg-42/rematch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		receivedBodies = append(receivedBodies, b)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	// without orphan_action: body must have igdb_id but NOT orphan_action
	if err := c.RematchExternalGame("k", "eg-42", 1234, ""); err != nil {
		t.Fatalf("RematchExternalGame (no orphan): %v", err)
	}
	if len(receivedBodies) != 1 {
		t.Fatalf("expected 1 request, got %d", len(receivedBodies))
	}
	body0 := receivedBodies[0]
	if igdbID, ok := body0["igdb_id"]; !ok || igdbID.(float64) != 1234 {
		t.Errorf("igdb_id = %v, want 1234", igdbID)
	}
	if _, ok := body0["orphan_action"]; ok {
		t.Errorf("orphan_action present but should be absent when empty string")
	}

	// with orphan_action: body must have both
	if err := c.RematchExternalGame("k", "eg-42", 5678, "remove"); err != nil {
		t.Fatalf("RematchExternalGame (with orphan): %v", err)
	}
	if len(receivedBodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(receivedBodies))
	}
	body1 := receivedBodies[1]
	if igdbID, ok := body1["igdb_id"]; !ok || igdbID.(float64) != 5678 {
		t.Errorf("igdb_id = %v, want 5678", igdbID)
	}
	if action, ok := body1["orphan_action"]; !ok || action != "remove" {
		t.Errorf("orphan_action = %v, want \"remove\"", action)
	}
}

func TestRetryFailedExternalGames(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/steam/external-games/retry-failed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.RetryFailedExternalGames("k", "steam"); err != nil {
		t.Fatalf("RetryFailedExternalGames: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestSkipExternalGame(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/ignored/eg-99", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	if err := c.SkipExternalGame("k", "eg-99"); err != nil {
		t.Fatalf("SkipExternalGame: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

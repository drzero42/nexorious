package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListSmells(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "orphan-game", "title": "Orphan game", "tier": "inconsistency", "auto_fixable": false, "count": 2},
			{"id": "wishlisted-yet-owned", "title": "Wishlisted yet owned", "tier": "inconsistency", "auto_fixable": true, "count": 1},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got, err := New(srv.URL).ListSmells("k")
	if err != nil {
		t.Fatalf("ListSmells: %v", err)
	}
	if len(got) != 2 || got[0].ID != "orphan-game" || got[0].Count != 2 || !got[1].AutoFixable {
		t.Fatalf("got = %+v", got)
	}
}

func TestListSmellItems(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/played-but-not-started", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "200" {
			t.Errorf("per_page = %q, want 200", r.URL.Query().Get("per_page"))
		}
		sugg := "in_progress"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"user_game_id": "u1", "game_id": 5, "title": "Halo", "suggested_status": sugg},
			},
			"total": 1, "page": 1, "per_page": 200, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).ListSmellItems("k", "played-but-not-started", 1, 200)
	if err != nil {
		t.Fatalf("ListSmellItems: %v", err)
	}
	if res.Total != 1 || len(res.Items) != 1 || res.Items[0].UserGameID != "u1" ||
		res.Items[0].SuggestedStatus == nil || *res.Items[0].SuggestedStatus != "in_progress" {
		t.Fatalf("res = %+v", res)
	}
}

func TestApplyIgnoreRestoreSmell(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body["user_game_ids"]) != 2 {
			t.Errorf("ids = %v", body["user_game_ids"])
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 2, "skipped": 0})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/ignore", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]int{"ignored": 1})
		case http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]int{"restored": 1})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	ap, err := c.ApplySmell("k", "wishlisted-yet-owned", []string{"a", "b"})
	if err != nil || ap.Applied != 2 {
		t.Fatalf("ApplySmell: %+v err=%v", ap, err)
	}
	ig, err := c.IgnoreSmell("k", "wishlisted-yet-owned", []string{"a"})
	if err != nil || ig != 1 {
		t.Fatalf("IgnoreSmell: %d err=%v", ig, err)
	}
	rs, err := c.RestoreSmell("k", "wishlisted-yet-owned", []string{"a"})
	if err != nil || rs != 1 {
		t.Fatalf("RestoreSmell: %d err=%v", rs, err)
	}
}

func TestListIgnoredSmells(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/orphan-game/ignored", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"user_game_id": "u9", "title": "Tetris", "created_at": "2026-06-22T00:00:00Z"}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).ListIgnoredSmells("k", "orphan-game", 1, 25)
	if err != nil {
		t.Fatalf("ListIgnoredSmells: %v", err)
	}
	if res.Total != 1 || res.Items[0].UserGameID != "u9" || res.Items[0].Title != "Tetris" {
		t.Fatalf("res = %+v", res)
	}
}

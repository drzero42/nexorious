package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// doctorServer stubs the summary + a couple of per-check listings.
func doctorServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells", func(w http.ResponseWriter, r *http.Request) {
		// Exact match only — sub-paths are handled below.
		if r.URL.Path != "/api/library/smells" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "orphan-game", "title": "Orphan game", "description": "d", "tier": "inconsistency", "auto_fixable": false, "count": 2},
			{"id": "wishlisted-yet-owned", "title": "Wishlisted yet owned", "description": "d", "tier": "inconsistency", "auto_fixable": true, "count": 1},
			{"id": "unrated-after-finishing", "title": "Unrated after finishing", "description": "d", "tier": "nudge", "auto_fixable": false, "count": 0},
		})
	})
	mux.HandleFunc("/api/library/smells/orphan-game", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"user_game_id": "u1", "game_id": 1, "title": "Tetris"},
				{"user_game_id": "u2", "game_id": 2, "title": "Pong"},
			},
			"total": 2, "page": 1, "per_page": 200, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDoctorSummaryTable(t *testing.T) {
	srv := doctorServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor: %v\n%s", err, out.String())
	}
	for _, want := range []string{"orphan-game", "Orphan game", "inconsistency", "2", "wishlisted-yet-owned", "yes"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("summary missing %q:\n%s", want, out.String())
		}
	}
}

func TestDoctorSummaryQuietOnlyNonzero(t *testing.T) {
	srv := doctorServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "-q"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor -q: %v", err)
	}
	got := out.String()
	if !bytes.Contains(out.Bytes(), []byte("orphan-game")) ||
		!bytes.Contains(out.Bytes(), []byte("wishlisted-yet-owned")) {
		t.Fatalf("quiet missing nonzero ids:\n%s", got)
	}
	if bytes.Contains(out.Bytes(), []byte("unrated-after-finishing")) {
		t.Fatalf("quiet should omit zero-count checks:\n%s", got)
	}
}

func TestDoctorDetail(t *testing.T) {
	srv := doctorServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "--check", "orphan-game"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --check: %v\n%s", err, out.String())
	}
	for _, want := range []string{"u1", "Tetris", "u2", "Pong"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("detail missing %q:\n%s", want, out.String())
		}
	}
	if !bytes.Contains(out.Bytes(), []byte("-")) {
		t.Fatalf("detail SUGGESTION fallback %q missing:\n%s", "-", out.String())
	}
}

func TestCollectFlaggedIDsPaginates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/multi", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"user_game_id": "a", "game_id": 1, "title": "A"}},
				"total": 2, "page": 1, "per_page": 1, "pages": 2,
			})
		case "2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"user_game_id": "b", "game_id": 2, "title": "B"}},
				"total": 2, "page": 2, "per_page": 1, "pages": 2,
			})
		default:
			t.Errorf("unexpected page %q", page)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ids, err := collectFlaggedIDs(cliclient.New(srv.URL), "k", "multi")
	if err != nil {
		t.Fatalf("collectFlaggedIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("ids = %v, want [a b]", ids)
	}
}

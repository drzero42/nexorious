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

func TestDoctorApplyAll(t *testing.T) {
	var gotIDs []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"user_game_id": "u1", "game_id": 1, "title": "A"},
				{"user_game_id": "u2", "game_id": 2, "title": "B"},
			},
			"total": 2, "page": 1, "per_page": 200, "pages": 1,
		})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotIDs = body["user_game_ids"]
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 2, "skipped": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "apply", "wishlisted-yet-owned", "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("apply: %v\n%s", err, out.String())
	}
	if len(gotIDs) != 2 {
		t.Fatalf("applied ids = %v", gotIDs)
	}
	if !bytes.Contains(out.Bytes(), []byte("Applied 2")) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestDoctorApplyByRef(t *testing.T) {
	var gotIDs []string
	mux := http.NewServeMux()
	// UUID ref → direct GET of the user-game.
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "11111111-1111-1111-1111-111111111111", "game": map[string]any{"id": 1, "title": "Halo"}})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotIDs = body["user_game_ids"]
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 1, "skipped": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "apply", "wishlisted-yet-owned",
		"11111111-1111-1111-1111-111111111111", "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("apply by ref: %v\n%s", err, out.String())
	}
	if len(gotIDs) != 1 || gotIDs[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("applied ids = %v", gotIDs)
	}
	if !bytes.Contains(out.Bytes(), []byte("Applied 1")) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestDoctorApplyNothingFlagged(t *testing.T) {
	var applyCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{}, "total": 0, "page": 1, "per_page": 200, "pages": 0,
		})
	})
	mux.HandleFunc("/api/library/smells/wishlisted-yet-owned/apply", func(w http.ResponseWriter, _ *http.Request) {
		applyCalled = true
		_ = json.NewEncoder(w).Encode(map[string]int{"applied": 0, "skipped": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "apply", "wishlisted-yet-owned", "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("apply: %v\n%s", err, out.String())
	}
	if applyCalled {
		t.Fatal("apply endpoint should not be called when nothing is flagged")
	}
	if !bytes.Contains(out.Bytes(), []byte("Nothing to apply")) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestDoctorIgnoreAndRestore(t *testing.T) {
	var ignoreIDs, restoreIDs []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "22222222-2222-2222-2222-222222222222", "game": map[string]any{"id": 1, "title": "Doom"}})
	})
	mux.HandleFunc("/api/library/smells/orphan-game/ignore", func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.Method {
		case http.MethodPost:
			ignoreIDs = body["user_game_ids"]
			_ = json.NewEncoder(w).Encode(map[string]int{"ignored": 1})
		case http.MethodDelete:
			restoreIDs = body["user_game_ids"]
			_ = json.NewEncoder(w).Encode(map[string]int{"restored": 1})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	ref := "22222222-2222-2222-2222-222222222222"

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "ignore", "orphan-game", ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("ignore: %v\n%s", err, out.String())
	}
	if len(ignoreIDs) != 1 || ignoreIDs[0] != ref {
		t.Fatalf("ignore ids = %v", ignoreIDs)
	}

	out.Reset()
	root = newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "restore", "orphan-game", ref})
	if err := root.Execute(); err != nil {
		t.Fatalf("restore: %v\n%s", err, out.String())
	}
	if len(restoreIDs) != 1 || restoreIDs[0] != ref {
		t.Fatalf("restore ids = %v", restoreIDs)
	}
}

func TestDoctorIgnoredList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/library/smells/orphan-game/ignored", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"user_game_id": "u9", "title": "Myst", "created_at": "2026-06-22T10:00:00Z"}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "ignored", "orphan-game"})
	if err := root.Execute(); err != nil {
		t.Fatalf("ignored: %v\n%s", err, out.String())
	}
	for _, want := range []string{"u9", "Myst"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("ignored list missing %q:\n%s", want, out.String())
		}
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const reviewUUID = "11111111-1111-1111-1111-111111111111"

// serveExternalGames registers GET /api/sync/<sf>/external-games returning one
// needs_review item with the given id and title.
func serveExternalGames(mux *http.ServeMux, sf, id, title string) {
	mux.HandleFunc("/api/sync/"+sf+"/external-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": id, "storefront": sf, "external_id": "ext-1", "title": title, "sync_status": "needs_review"},
		})
	})
}

func TestSyncResolveByID(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	serveExternalGames(mux, "steam", reviewUUID, "Hollow Knight")
	var rematched bool
	mux.HandleFunc("/api/sync/external-games/"+reviewUUID+"/rematch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["igdb_id"] != float64(42) {
			t.Errorf("igdb_id = %v, want 42", body["igdb_id"])
		}
		if _, ok := body["orphan_action"]; ok {
			t.Errorf("orphan_action should be omitted, got %v", body["orphan_action"])
		}
		rematched = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "resolve", "steam", reviewUUID, "--igdb-id", "42"})
	if err := root.Execute(); err != nil {
		t.Fatalf("resolve: %v\n%s", err, out.String())
	}
	if !rematched {
		t.Fatal("rematch not received")
	}
	if !strings.Contains(out.String(), "Hollow Knight") || !strings.Contains(out.String(), "42") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSyncResolveByTitle(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	serveExternalGames(mux, "steam", reviewUUID, "Celeste")
	var rematched bool
	mux.HandleFunc("/api/sync/external-games/"+reviewUUID+"/rematch", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["igdb_id"] != float64(7) {
			t.Errorf("igdb_id = %v, want 7", body["igdb_id"])
		}
		rematched = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "resolve", "steam", "celeste", "--igdb-id", "7"})
	if err := root.Execute(); err != nil {
		t.Fatalf("resolve by title: %v\n%s", err, out.String())
	}
	if !rematched {
		t.Fatal("rematch not received")
	}
}

func TestSyncResolveOrphanAction(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	serveExternalGames(mux, "steam", reviewUUID, "Braid")
	mux.HandleFunc("/api/sync/external-games/"+reviewUUID+"/rematch", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["orphan_action"] != "remove" {
			t.Errorf("orphan_action = %v, want remove", body["orphan_action"])
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "resolve", "steam", reviewUUID, "--igdb-id", "9", "--orphan-action", "remove"})
	if err := root.Execute(); err != nil {
		t.Fatalf("resolve --orphan-action: %v\n%s", err, out.String())
	}
}

func TestSyncResolveMissingIgdbID(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	serveExternalGames(mux, "steam", reviewUUID, "Hades")
	mux.HandleFunc("/api/sync/external-games/"+reviewUUID+"/rematch", func(http.ResponseWriter, *http.Request) {
		t.Error("rematch must not be called without --igdb-id")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "resolve", "steam", reviewUUID})
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error for missing --igdb-id, got nil\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "igdb-id") {
		t.Fatalf("error = %v, want it to mention igdb-id", err)
	}
}

func TestSyncSkip(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	serveExternalGames(mux, "steam", reviewUUID, "Tunic")
	var skipped bool
	mux.HandleFunc("/api/sync/ignored/"+reviewUUID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		skipped = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "skip", "steam", reviewUUID, "-y"})
	if err := root.Execute(); err != nil {
		t.Fatalf("skip: %v\n%s", err, out.String())
	}
	if !skipped {
		t.Fatal("skip not received")
	}
	if !strings.Contains(out.String(), "skipped Tunic") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSyncRetry(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	var retried bool
	mux.HandleFunc("/api/sync/steam/external-games/retry-failed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		retried = true
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "retry", "steam"})
	if err := root.Execute(); err != nil {
		t.Fatalf("retry: %v\n%s", err, out.String())
	}
	if !retried {
		t.Fatal("retry-failed not received")
	}
	if !strings.Contains(out.String(), "re-queued") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSyncReviewOffTTY(t *testing.T) {
	seedProfile(t, "http://example.invalid")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("")) // non-TTY: review must refuse
	root.SetArgs([]string{"sync", "review", "steam"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("expected error for non-interactive review, got nil")
	}
	if !strings.Contains(err.Error(), "interactive") {
		t.Fatalf("error = %v, want it to mention interactive", err)
	}
}

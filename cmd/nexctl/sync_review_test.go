package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
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

func TestRematchErrHint_OnlyForTheOrphan409(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&cliclient.HTTPError{Status: 409, Message: "orphan_action required: game would lose its only storefront link"}, true},
		{&cliclient.HTTPError{Status: 409, Message: "a sync is still working on this game"}, false},
		{&cliclient.HTTPError{Status: 500, Message: "failed to rematch external game"}, false},
		{fmt.Errorf("wrapped: %w", &cliclient.HTTPError{Status: 409, Message: "orphan_action required: x"}), true},
		{errors.New("orphan_action required"), false},
	}
	for _, tc := range cases {
		if got := rematchErrHint(tc.err) != ""; got != tc.want {
			t.Errorf("rematchErrHint(%v): hint=%v, want %v", tc.err, got, tc.want)
		}
	}
}

// An unmatched game (e.g. unskipped, never matched) is resolvable by title
// like one in review (#1338).
func TestSyncResolveUnmatchedGameByTitle(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	mux.HandleFunc("/api/sync/steam/external-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": reviewUUID, "storefront": "steam", "external_id": "ext-1", "title": "Celeste", "sync_status": "unmatched"},
		})
	})
	var rematched bool
	mux.HandleFunc("/api/sync/external-games/"+reviewUUID+"/rematch", func(w http.ResponseWriter, _ *http.Request) {
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
		t.Fatalf("resolve: %v\n%s", err, out.String())
	}
	if !rematched {
		t.Fatal("rematch not received")
	}
}

const editionUUID = "22222222-2222-2222-2222-222222222222"

// serveEditionSets registers the steam list with one "Spider-Man" entry of
// two editions (reviewUUID is the representative) and, with second, another
// single-edition "Spider-Man" entry.
func serveEditionSets(mux *http.ServeMux, second bool) {
	entries := []map[string]any{
		{"id": reviewUUID, "storefront": "steam", "external_id": "100", "title": "Spider-Man", "sync_status": "matched",
			"platforms": []string{"pc-linux", "pc-windows"},
			"editions": []map[string]any{
				{"id": reviewUUID, "external_id": "100", "platforms": []string{"pc-windows"}},
				{"id": editionUUID, "external_id": "200", "platforms": []string{"pc-linux"}},
			}},
	}
	if second {
		entries = append(entries, map[string]any{
			"id": "33333333-3333-3333-3333-333333333333", "storefront": "steam", "external_id": "300", "title": "Spider-Man", "sync_status": "matched",
			"platforms": []string{"pc-windows"},
			"editions":  []map[string]any{{"id": "33333333-3333-3333-3333-333333333333", "external_id": "300", "platforms": []string{"pc-windows"}}},
		})
	}
	mux.HandleFunc("/api/sync/steam/external-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(entries)
	})
}

// runSync runs nexctl with args against a stub that records each decision
// request as "METHOD path?query".
func runSync(t *testing.T, second bool, args ...string) (requests []string, out string, err error) {
	t.Helper()
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	serveEditionSets(mux, second)
	record := func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	}
	mux.HandleFunc("/api/sync/external-games/", record)
	mux.HandleFunc("/api/sync/ignored/", record)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err = root.Execute()
	return requests, buf.String(), err
}

// With --only-this-edition the request goes to the edition's id as given,
// never the entry's, with the option.
func TestSyncOnlyThisEdition_SendsTheEditionsID(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"resolve, edition id", []string{"sync", "resolve", "steam", editionUUID, "--igdb-id", "5", "--only-this-edition"},
			"POST /api/sync/external-games/" + editionUUID + "/rematch?only_this_edition=true"},
		{"resolve, representative's id", []string{"sync", "resolve", "steam", reviewUUID, "--igdb-id", "5", "--only-this-edition"},
			"POST /api/sync/external-games/" + reviewUUID + "/rematch?only_this_edition=true"},
		{"skip, edition id", []string{"sync", "skip", "steam", editionUUID, "-y", "--only-this-edition"},
			"POST /api/sync/ignored/" + editionUUID + "?only_this_edition=true"},
		{"resolve without the option, edition id", []string{"sync", "resolve", "steam", editionUUID, "--igdb-id", "5"},
			"POST /api/sync/external-games/" + editionUUID + "/rematch?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqs, out, err := runSync(t, false, tc.args...)
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if len(reqs) != 1 || reqs[0] != tc.want {
				t.Fatalf("requests = %q, want [%q]", reqs, tc.want)
			}
		})
	}
}

// A title of an entry with more than one edition, or of several entries, is
// refused off a terminal with the option, listing the editions with id,
// storefront id and platforms.
func TestSyncOnlyThisEdition_AmbiguousTitle_ListsTheEditions(t *testing.T) {
	for _, second := range []bool{false, true} {
		for _, args := range [][]string{
			{"sync", "resolve", "steam", "spider-man", "--igdb-id", "5", "--only-this-edition"},
			{"sync", "skip", "steam", "spider-man", "-y", "--only-this-edition"},
		} {
			t.Run(fmt.Sprintf("%s second=%v", args[1], second), func(t *testing.T) {
				reqs, _, err := runSync(t, second, args...)
				if err == nil || len(reqs) != 0 {
					t.Fatalf("err %v, requests %q: want a refusal and no request", err, reqs)
				}
				for _, want := range []string{editionUUID + "  200  pc-linux", reviewUUID + "  100  pc-windows"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error lacks %q:\n%v", want, err)
					}
				}
			})
		}
	}
}

// Without the option, a title of several entries lists the entries with
// enough to tell them apart.
func TestSyncResolve_TitleOfSeveralEntries_ListsThemApart(t *testing.T) {
	_, _, err := runSync(t, true, "sync", "resolve", "steam", "spider-man", "--igdb-id", "5")
	if err == nil {
		t.Fatal("want a refusal")
	}
	for _, want := range []string{reviewUUID + "  100  pc-linux,pc-windows", "33333333-3333-3333-3333-333333333333  300  pc-windows"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

// An older server lists no editions: --only-this-edition is refused, since it
// would act on the whole set.
func TestSyncOnlyThisEdition_OlderServer_Refused(t *testing.T) {
	mux := http.NewServeMux()
	serveSyncConfigs(mux, "steam")
	serveExternalGames(mux, "steam", reviewUUID, "Tunic")
	mux.HandleFunc("/api/sync/ignored/", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a request was sent")
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "skip", "steam", reviewUUID, "-y", "--only-this-edition"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "upgrade") {
		t.Fatalf("err = %v, want the older-server refusal", err)
	}
}

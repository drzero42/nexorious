package clientops

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/cliclient"
)

const (
	repID     = "11111111-1111-1111-1111-111111111111"
	editionID = "22222222-2222-2222-2222-222222222222"
	soloID    = "33333333-3333-3333-3333-333333333333"
)

// externalListServer serves a steam list: one entry of two editions (the
// representative first by id) and one single-edition entry, both titled
// "Spider-Man".
func externalListServer(t *testing.T, withEditions bool) *cliclient.Client {
	t.Helper()
	entries := []map[string]any{
		{"id": repID, "title": "Spider-Man", "external_id": "1", "editions": []map[string]any{
			{"id": repID, "external_id": "1", "platforms": []string{"pc-windows"}},
			{"id": editionID, "external_id": "2", "platforms": []string{"pc-linux"}},
		}},
		{"id": soloID, "title": "Spider-Man", "external_id": "3", "editions": []map[string]any{
			{"id": soloID, "external_id": "3", "platforms": []string{"pc-windows"}},
		}},
	}
	if !withEditions {
		for _, e := range entries {
			delete(e, "editions")
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/steam/external-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(entries)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return cliclient.New(srv.URL)
}

// A non-representative edition's id finds its entry and names that edition.
func TestFindExternalGamesByRef_EditionID(t *testing.T) {
	c := externalListServer(t, true)
	got, err := FindExternalGamesByRef(t.Context(), c, "k", "steam", editionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Game.ID != repID || got[0].Edition == nil || got[0].Edition.ID != editionID {
		t.Fatalf("got %+v, want the entry %s with edition %s", got, repID, editionID)
	}
}

func TestExternalDecisionTarget(t *testing.T) {
	cases := []struct {
		name      string
		ref       string
		only      bool
		wantID    string
		ambiguous bool
	}{
		// Without the option the id goes as given; the server acts on the set.
		{"set, edition id", editionID, false, editionID, false},
		{"set, entry id", repID, false, repID, false},
		{"set, title of two entries", "spider-man", false, "", true},
		// With the option the id is always an edition's, never the entry's
		// unless it is the edition meant.
		{"one, edition id", editionID, true, editionID, false},
		{"one, representative's id", repID, true, repID, false},
		{"one, title of two entries", "Spider-Man", true, "", true},
		{"one, single edition by id", soloID, true, soloID, false},
	}
	c := externalListServer(t, true)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := FindExternalGamesByRef(t.Context(), c, "k", "steam", tc.ref)
			if err != nil {
				t.Fatal(err)
			}
			id, amb, err := ExternalDecisionTarget(m, tc.only)
			if err != nil || id != tc.wantID || amb != tc.ambiguous {
				t.Fatalf("got (%q, %v, %v), want (%q, %v, nil)", id, amb, err, tc.wantID, tc.ambiguous)
			}
		})
	}
}

// A title of one entry of several editions is ambiguous with the option; of
// one edition it is that edition.
func TestExternalDecisionTarget_TitleOfOneEntry(t *testing.T) {
	two := ExternalGameMatch{Game: cliclient.ExternalGame{ID: repID, Editions: []cliclient.ExternalGameEdition{{ID: repID}, {ID: editionID}}}}
	if _, amb, err := ExternalDecisionTarget([]ExternalGameMatch{two}, true); err != nil || !amb {
		t.Errorf("two editions: ambiguous %v, err %v; want ambiguous", amb, err)
	}
	if id, _, _ := ExternalDecisionTarget([]ExternalGameMatch{two}, false); id != repID {
		t.Errorf("two editions without the option: id %q, want the entry's", id)
	}
	one := ExternalGameMatch{Game: cliclient.ExternalGame{ID: soloID, Editions: []cliclient.ExternalGameEdition{{ID: soloID}}}}
	if id, amb, err := ExternalDecisionTarget([]ExternalGameMatch{one}, true); err != nil || amb || id != soloID {
		t.Errorf("one edition: (%q, %v, %v), want %q", id, amb, err, soloID)
	}
}

// An older server lists no editions: the option is refused, since it would
// ignore it and act on the whole set; without it nothing changes.
func TestExternalDecisionTarget_OlderServer(t *testing.T) {
	c := externalListServer(t, false)
	m, err := FindExternalGamesByRef(t.Context(), c, "k", "steam", repID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ExternalDecisionTarget(m, true); !errors.Is(err, ErrNoEditionsListed) {
		t.Errorf("with the option: err %v, want ErrNoEditionsListed", err)
	}
	if id, _, err := ExternalDecisionTarget(m, false); err != nil || id != repID {
		t.Errorf("without the option: (%q, %v), want %q", id, err, repID)
	}
}

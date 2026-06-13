package filter

import (
	"testing"
)

func TestParsePoolFilterPlayStatus(t *testing.T) {
	t.Run("multi-value array parses to slice", func(t *testing.T) {
		pf, err := ParsePoolFilter([]byte(`{"filters":[{"play_status":["backlog","shelved"]}]}`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := pf.Filters[0].PlayStatus
		if len(got) != 2 || got[0] != "backlog" || got[1] != "shelved" {
			t.Fatalf("expected [backlog shelved], got %v", got)
		}
	})

	t.Run("legacy single string parses to one-element slice (back-compat)", func(t *testing.T) {
		pf, err := ParsePoolFilter([]byte(`{"filters":[{"play_status":"completed"}]}`))
		if err != nil {
			t.Fatalf("parse legacy string: %v", err)
		}
		got := pf.Filters[0].PlayStatus
		if len(got) != 1 || got[0] != "completed" {
			t.Fatalf("expected [completed], got %v", got)
		}
	})

	t.Run("absent play_status leaves an empty card facet", func(t *testing.T) {
		pf, err := ParsePoolFilter([]byte(`{"filters":[{"play_status":["x"]},{"genre":["RPG"]}]}`))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(pf.Filters[1].PlayStatus) != 0 {
			t.Fatalf("expected no play_status on second card, got %v", pf.Filters[1].PlayStatus)
		}
	})

	t.Run("unknown keys still rejected", func(t *testing.T) {
		if _, err := ParsePoolFilter([]byte(`{"filters":[{"nope":"x"}]}`)); err == nil {
			t.Fatal("expected error for unknown key")
		}
	})
}

func TestFilterCardHasFacetsPlayStatus(t *testing.T) {
	empty := FilterCard{}
	if empty.HasFacets() {
		t.Fatal("empty card should have no facets")
	}
	withStatus := FilterCard{PlayStatus: []string{"shelved"}}
	if !withStatus.HasFacets() {
		t.Fatal("card with play_status should report a facet")
	}
}

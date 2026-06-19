package usergame

import "testing"

// ---------------------------------------------------------------------------
// ownershipRank
// ---------------------------------------------------------------------------

func TestOwnershipRank(t *testing.T) {
	cases := []struct {
		status string
		want   int
	}{
		{"owned", 4},
		{"borrowed", 3},
		{"rented", 3},
		{"subscription", 2},
		{"no_longer_owned", 1},
		{"unknown_status", 0},
		{"", 0},
	}
	for _, tc := range cases {
		if got := ownershipRank(tc.status); got != tc.want {
			t.Errorf("ownershipRank(%q) = %d, want %d", tc.status, got, tc.want)
		}
	}
	// Verify strict ordering: owned > borrowed > subscription > no_longer_owned > unknown.
	if ownershipRank("owned") <= ownershipRank("borrowed") {
		t.Error("owned must rank higher than borrowed")
	}
	if ownershipRank("borrowed") <= ownershipRank("subscription") {
		t.Error("borrowed must rank higher than subscription")
	}
	if ownershipRank("subscription") <= ownershipRank("no_longer_owned") {
		t.Error("subscription must rank higher than no_longer_owned")
	}
	if ownershipRank("no_longer_owned") <= ownershipRank("") {
		t.Error("no_longer_owned must rank higher than empty/unknown")
	}
}

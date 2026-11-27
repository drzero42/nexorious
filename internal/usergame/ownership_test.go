package usergame

import "testing"

// ---------------------------------------------------------------------------
// OwnershipRank
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
		if got := OwnershipRank(tc.status); got != tc.want {
			t.Errorf("OwnershipRank(%q) = %d, want %d", tc.status, got, tc.want)
		}
	}
	// Verify strict ordering: owned > borrowed > subscription > no_longer_owned > unknown.
	if OwnershipRank("owned") <= OwnershipRank("borrowed") {
		t.Error("owned must rank higher than borrowed")
	}
	if OwnershipRank("borrowed") <= OwnershipRank("subscription") {
		t.Error("borrowed must rank higher than subscription")
	}
	if OwnershipRank("subscription") <= OwnershipRank("no_longer_owned") {
		t.Error("subscription must rank higher than no_longer_owned")
	}
	if OwnershipRank("no_longer_owned") <= OwnershipRank("") {
		t.Error("no_longer_owned must rank higher than empty/unknown")
	}
}

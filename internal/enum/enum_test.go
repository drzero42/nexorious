package enum

import (
	"slices"
	"testing"
)

func TestAllPlayStatuses(t *testing.T) {
	got := AllPlayStatuses()
	want := []string{
		"not_started", "in_progress", "completed", "mastered",
		"dominated", "shelved", "dropped", "replay",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("AllPlayStatuses() = %v, want %v", got, want)
	}
	for _, s := range got {
		if !PlayStatus(s).Valid() {
			t.Fatalf("AllPlayStatuses() returned invalid status %q", s)
		}
	}
}

func TestAllOwnershipStatuses(t *testing.T) {
	got := AllOwnershipStatuses()
	want := []string{"owned", "borrowed", "rented", "subscription", "no_longer_owned"}
	if !slices.Equal(got, want) {
		t.Fatalf("AllOwnershipStatuses() = %v, want %v", got, want)
	}
	for _, s := range got {
		if !OwnershipStatus(s).Valid() {
			t.Fatalf("AllOwnershipStatuses() returned invalid status %q", s)
		}
	}
}

func TestOwnershipStatusRank(t *testing.T) {
	cases := []struct {
		status OwnershipStatus
		want   int
	}{
		{OwnershipOwned, 4},
		{OwnershipBorrowed, 3},
		{OwnershipRented, 3},
		{OwnershipSubscription, 2},
		{OwnershipNoLongerOwned, 1},
		{"unknown_status", 0},
		{"", 0},
	}
	for _, tc := range cases {
		if got := tc.status.Rank(); got != tc.want {
			t.Errorf("OwnershipStatus(%q).Rank() = %d, want %d", tc.status, got, tc.want)
		}
	}
	// Strict ordering: owned > borrowed > subscription > no_longer_owned > unknown.
	order := []OwnershipStatus{OwnershipOwned, OwnershipBorrowed, OwnershipSubscription, OwnershipNoLongerOwned, ""}
	for i := 1; i < len(order); i++ {
		if order[i-1].Rank() <= order[i].Rank() {
			t.Errorf("%q must rank higher than %q", order[i-1], order[i])
		}
	}
}

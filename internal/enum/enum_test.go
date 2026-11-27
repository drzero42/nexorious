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

// The rank's numbers don't matter, only the order: each tier ranks strictly
// above the next, statuses in one tier rank equal, and an unknown or empty
// status ranks 0 (the documented floor).
func TestOwnershipStatusRank(t *testing.T) {
	tiers := [][]OwnershipStatus{
		{OwnershipOwned},
		{OwnershipBorrowed, OwnershipRented},
		{OwnershipSubscription},
		{OwnershipNoLongerOwned},
		{"unknown_status", ""},
	}
	for i, tier := range tiers {
		for _, s := range tier[1:] {
			if s.Rank() != tier[0].Rank() {
				t.Errorf("%q must rank equal to %q", s, tier[0])
			}
		}
		if i > 0 && tiers[i-1][0].Rank() <= tier[0].Rank() {
			t.Errorf("%q must rank higher than %q", tiers[i-1][0], tier[0])
		}
	}
	for _, s := range tiers[len(tiers)-1] {
		if s.Rank() != 0 {
			t.Errorf("OwnershipStatus(%q).Rank() = %d, want 0", s, s.Rank())
		}
	}
}

// Every valid ownership status must have a rank, or a newly added status
// would silently rank as unknown.
func TestEveryOwnershipStatusHasRank(t *testing.T) {
	for _, s := range AllOwnershipStatuses() {
		if OwnershipStatus(s).Rank() <= 0 {
			t.Errorf("OwnershipStatus(%q).Rank() = %d, want > 0", s, OwnershipStatus(s).Rank())
		}
	}
}

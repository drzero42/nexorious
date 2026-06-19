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

package api

import (
	"maps"
	"slices"
	"testing"
)

// itemProgress derives its per-status keys from the Job item statuses; the keys
// are API contract, so a renamed status must not silently rename them.
func TestItemProgressKeys(t *testing.T) {
	got := slices.Sorted(maps.Keys(itemProgress(nil)))
	want := []string{
		"cancelled", "completed", "done", "failed", "pending", "pending_review",
		"percent", "skipped", "total",
	}
	if !slices.Equal(got, want) {
		t.Errorf("progress keys = %v, want %v", got, want)
	}
}

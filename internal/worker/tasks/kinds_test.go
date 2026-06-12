package tasks

import "testing"

// The Darkadia per-item workers were missed by the structured-logging sweep, so
// a 2-stage import of N games emitted 2×N Info "job finished" lines. They must be
// in the quiet per-item set like every other high-fan-out worker.
func TestPerItemJobKinds_IncludesDarkadia(t *testing.T) {
	got := make(map[string]bool)
	for _, k := range PerItemJobKinds() {
		got[k] = true
	}
	for _, want := range []string{DarkadiaMatchArgs{}.Kind(), DarkadiaFinalizeArgs{}.Kind()} {
		if !got[want] {
			t.Errorf("PerItemJobKinds() missing %q", want)
		}
	}
}

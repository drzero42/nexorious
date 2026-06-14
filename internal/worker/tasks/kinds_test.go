package tasks

import "testing"

// The import per-item workers must be in the quiet per-item set like every other
// high-fan-out worker, so a 2-stage import of N games does not emit 2×N Info
// "job finished" lines.
func TestPerItemJobKinds_IncludesImport(t *testing.T) {
	got := make(map[string]bool)
	for _, k := range PerItemJobKinds() {
		got[k] = true
	}
	for _, want := range []string{ImportMatchArgs{}.Kind(), ImportFinalizeArgs{}.Kind()} {
		if !got[want] {
			t.Errorf("PerItemJobKinds() missing %q", want)
		}
	}
}

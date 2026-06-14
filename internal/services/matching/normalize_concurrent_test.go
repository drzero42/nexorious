package matching

import (
	"fmt"
	"sync"
	"testing"
)

// TestNormalizeTitle_Concurrent guards against a data race in the diacritic
// folder. NormalizeTitle is called from concurrent River workers (import_match
// and igdb_match); a transform.Transformer is stateful and not safe for
// concurrent use, so a shared one corrupts under load and panics with
// "slice bounds out of range". This test must pass under `go test -race`.
func TestNormalizeTitle_Concurrent(t *testing.T) {
	// Inputs that exercise the diacritic-folding transformer.
	inputs := []string{
		"ABZÛ", "Ōkami HD", "Pokémon", "Café Crème", "Naïve Façade",
		"Mëtàl Gëàr", "Ångström", "Señor Quesø", "Brütal Legend", "Hôtel Düsk",
	}
	want := make([]string, len(inputs))
	for i, in := range inputs {
		want[i] = NormalizeTitle(in) // single-threaded baseline (trusted)
	}

	const goroutines = 64
	const iterations = 500
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for range goroutines {
		wg.Go(func() {
			for i := range iterations {
				idx := i % len(inputs)
				got := NormalizeTitle(inputs[idx])
				if got != want[idx] {
					errs <- fmt.Errorf("NormalizeTitle(%q) = %q, want %q", inputs[idx], got, want[idx])
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

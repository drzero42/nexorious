package matching

import "testing"

func TestDecide_ConfidentUnambiguous(t *testing.T) {
	cands := []Candidate{
		{ID: 1, Title: "Celeste"},
		{ID: 2, Title: "Some Other Game Entirely"},
	}
	d := Decide("Celeste", cands)
	if !d.Confident {
		t.Fatalf("expected confident, got %+v", d)
	}
	if d.ResolvedID != 1 {
		t.Errorf("resolved id = %d, want 1", d.ResolvedID)
	}
}

func TestDecide_TieIsNotConfident(t *testing.T) {
	cands := []Candidate{
		{ID: 1, Title: "Halo"},
		{ID: 2, Title: "Halo"},
	}
	d := Decide("Halo", cands)
	if d.Confident {
		t.Errorf("tie should not be confident: %+v", d)
	}
}

func TestDecide_LowConfidenceNotConfident(t *testing.T) {
	cands := []Candidate{{ID: 1, Title: "Completely Different Title"}}
	d := Decide("xyzzy", cands)
	if d.Confident {
		t.Errorf("low score should not be confident: %+v", d)
	}
}

func TestDecide_NoCandidates(t *testing.T) {
	d := Decide("anything", nil)
	if d.Confident {
		t.Errorf("no candidates should not be confident: %+v", d)
	}
}

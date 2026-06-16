package matching

// AutoResolveThreshold and TieEpsilon are the auto-resolve gate, shared by the
// sync IGDB-match worker and the import match worker.
const (
	AutoResolveThreshold = 0.85
	TieEpsilon           = 0.01
)

// Candidate is a scored match candidate: a stable integer ID and a display title.
// Callers convert from their own types (e.g. igdb.GameMetadata) before calling Decide.
type Candidate struct {
	ID    int32
	Title string
}

// Decision is the result of scoring candidates against a query title.
type Decision struct {
	BestScore  float64
	SecondBest float64
	ResolvedID int32 // best candidate's ID (0 if no candidates)
	// Confident is true when the best score clears AutoResolveThreshold AND beats
	// the runner-up by more than TieEpsilon — i.e. confident and unambiguous.
	Confident bool
}

// Decide scores candidates against query (titles normalized internally) and
// returns the auto-resolve decision. It does no I/O.
func Decide(query string, candidates []Candidate) Decision {
	nq := NormalizeTitle(query)
	var best, second float64
	var bestID int32
	for _, c := range candidates {
		score := FuzzyConfidence(nq, NormalizeTitle(c.Title))
		if score > best {
			second = best
			best = score
			bestID = c.ID
		} else if score > second {
			second = score
		}
	}
	return Decision{
		BestScore:  best,
		SecondBest: second,
		ResolvedID: bestID,
		Confident:  best >= AutoResolveThreshold && (best-second) > TieEpsilon,
	}
}

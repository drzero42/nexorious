package matching

import (
	"log/slog"

	"github.com/paul-mannino/go-fuzzywuzzy"
)

// Metric functions are indirected through package vars so a panic in the
// third-party library can be contained (see safeMetric) and so tests can
// inject a failing metric. go-fuzzywuzzy's PartialRatio panics with
// "slice bounds out of range" on certain real input pairs (malformed internal
// matching blocks), which previously crashed the igdb_match worker.
var (
	ratioMetric    = fuzzy.Ratio
	partialRatio   = fuzzy.PartialRatio
	tokenSortRatio = func(a, b string) int { return fuzzy.TokenSortRatio(a, b) }
	tokenSetRatio  = func(a, b string) int { return fuzzy.TokenSetRatio(a, b) }
)

// safeMetric runs a scoring metric and contains any panic from it, reporting a
// zero score for that metric instead of unwinding into the caller. A single
// metric that trips a third-party bug must not crash the whole match.
func safeMetric(query, title string, fn func(string, string) int) (score int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("matching: fuzzy metric panicked; scoring 0 for this metric",
				"query", query, "title", title, "panic", r)
			score = 0
		}
	}()
	return fn(query, title)
}

// FuzzyConfidence returns a 0.0–1.0 score using the multi-metric weighted
// approach. Both inputs should be pre-normalized via NormalizeTitle.
//
// Weighted max of: exact×1.0, ratio×1.0, partial×0.88, token_sort×0.7, token_set×0.6
//
// ratio carries full weight (1.0) so near-identical strings (differ by an
// article, number, or "(Classic)") can reach the 0.85 auto-resolve threshold.
// partial carries 0.88 so a Steam title that is a verbatim prefix of an IGDB
// title (e.g. "Tesla Effect" vs "Tesla Effect: A Tex Murphy Adventure") can
// also auto-resolve without needing an exact character-level match.
func FuzzyConfidence(query, title string) float64 {
	if query == title {
		return 1.0
	}

	ratio := float64(safeMetric(query, title, ratioMetric)) / 100.0
	partial := float64(safeMetric(query, title, partialRatio)) / 100.0
	tokenSort := float64(safeMetric(query, title, tokenSortRatio)) / 100.0
	tokenSet := float64(safeMetric(query, title, tokenSetRatio)) / 100.0

	// Weighted max — take the best score across all metrics with decreasing weights
	scores := []float64{
		ratio * 1.0,
		partial * 0.88,
		tokenSort * 0.7,
		tokenSet * 0.6,
	}

	best := 0.0
	for _, s := range scores {
		if s > best {
			best = s
		}
	}
	return best
}

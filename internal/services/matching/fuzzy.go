package matching

import (
	"github.com/paul-mannino/go-fuzzywuzzy"
)

// FuzzyConfidence returns a 0.0–1.0 score using the multi-metric weighted
// approach. Both inputs should be pre-normalized via NormalizeTitle.
//
// Weighted max of: exact×1.0, ratio×0.9, partial×0.8, token_sort×0.7, token_set×0.6
func FuzzyConfidence(query, title string) float64 {
	if query == title {
		return 1.0
	}

	ratio := float64(fuzzy.Ratio(query, title)) / 100.0
	partial := float64(fuzzy.PartialRatio(query, title)) / 100.0
	tokenSort := float64(fuzzy.TokenSortRatio(query, title)) / 100.0
	tokenSet := float64(fuzzy.TokenSetRatio(query, title)) / 100.0

	// Weighted max — take the best score across all metrics with decreasing weights
	scores := []float64{
		ratio * 0.9,
		partial * 0.8,
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

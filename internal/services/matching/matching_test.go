package matching

import "testing"

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// GOTY expansion
		{"The Witcher 3 GOTY", "witcher 3 game of the year"},
		{"goty edition", "game of the year edition"},
		// Trademark symbols
		{"Skyrim™", "skyrim"},
		{"FIFA®", "fifa"},
		// Apostrophes (straight and curly)
		{"Assassin's Creed", "assassins creed"},
		{"It's a game", "its a game"},
		// Colons
		{"Halo: Reach", "halo reach"},
		// Standalone dashes (preserve in-word hyphens)
		{"Spider-Man - Miles Morales", "spider-man miles morales"},
		{"God of War - Ragnarök", "god of war ragnarök"},
		// Year in parentheses
		{"Doom (2016)", "doom"},
		{"Resident Evil 4 (2023)", "resident evil 4"},
		// Whitespace collapse
		{"  Hello   World  ", "hello world"},
		// Combined
		{"The Witcher 3: Wild Hunt - GOTY (2015)", "witcher 3 wild hunt game of the year"},
		// Leading "The" article stripped so "The X" and "X" compare equally
		{"The Blackwell Deception", "blackwell deception"},
		{"Blackwell Deception", "blackwell deception"},
		// "the" inside words must not be affected
		{"Thea: The Awakening", "thea the awakening"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeTitle(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFuzzyConfidence(t *testing.T) {
	tests := []struct {
		query    string
		title    string
		minScore float64
		maxScore float64
	}{
		// Exact match → high score
		{"the witcher 3", "the witcher 3", 0.95, 1.0},
		// Close match → medium-high
		{"witcher 3", "the witcher 3 wild hunt", 0.5, 0.9},
		// Completely different → low
		{"doom", "animal crossing new horizons", 0.0, 0.3},
		// Partial match
		{"spider-man", "marvels spider-man remastered", 0.5, 0.95},
	}
	for _, tt := range tests {
		t.Run(tt.query+"_vs_"+tt.title, func(t *testing.T) {
			score := FuzzyConfidence(tt.query, tt.title)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("FuzzyConfidence(%q, %q) = %f, want [%f, %f]",
					tt.query, tt.title, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

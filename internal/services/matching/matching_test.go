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
		// Trademark symbols replaced with space (not removed), so "Velocity®2X" → "velocity 2x"
		{"Skyrim™", "skyrim"},
		{"FIFA®", "fifa"},
		{"Velocity®2X", "velocity 2x"},
		// Apostrophes (straight and curly)
		{"Assassin's Creed", "assassins creed"},
		{"It's a game", "its a game"},
		// Colons
		{"Halo: Reach", "halo reach"},
		// Standalone dashes (preserve in-word hyphens)
		{"Spider-Man - Miles Morales", "spider-man miles morales"},
		{"God of War - Ragnarök", "god of war ragnarok"},
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
		// (Classic) stripped so "Mafia II (Classic)" scores 1.0 vs "Mafia II"
		{"Mafia II (Classic)", "mafia ii"},
		// (Classic, YYYY) compound form also stripped
		{"Star Wars: Battlefront 2 (Classic, 2005)", "star wars battlefront 2"},
		// Unicode diacritics folded to ASCII
		{"ABZÛ", "abzu"},
		{"Ōkami HD", "okami hd"},
		{"God of War - Ragnarök", "god of war ragnarok"},
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
		// Exact match → 1.0
		{"the witcher 3", "the witcher 3", 1.0, 1.0},
		// Substring match — "witcher 3" appears verbatim in the longer title,
		// so partial=100%×0.88=0.88; this is intentionally allowed to score high.
		{"witcher 3", "the witcher 3 wild hunt", 0.5, 0.95},
		// Completely different → low
		{"doom", "animal crossing new horizons", 0.0, 0.3},
		// Partial match — Steam title is prefix of IGDB title (partial * 0.88)
		{"spider-man", "marvels spider-man remastered", 0.5, 0.95},
		// Near-identical titles (ratio ~ 0.94) should now reach 0.85+ with ratio*1.0
		{"batman arkham city game of the year", "batman arkham city game of the year edition", 0.85, 1.0},
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

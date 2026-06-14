package matching

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	reGOTY            = regexp.MustCompile(`(?i)\bgoty\b`)
	ReTrademark       = regexp.MustCompile(`[™®]`)
	reApostrophes     = regexp.MustCompile(`[''']`)
	reColons          = regexp.MustCompile(`:`)
	reStandaloneDash  = regexp.MustCompile(`\s-\s`)
	reYearInParens    = regexp.MustCompile(`\(\d{4}\)`)
	reClassic         = regexp.MustCompile(`(?i)\(classic(?:,\s*\d{4})?\)`)
	reMultiWhitespace = regexp.MustCompile(`\s+`)
)

// NormalizeTitle applies transformations for comparison purposes only.
// The result is never stored or displayed.
func NormalizeTitle(s string) string {
	s = reGOTY.ReplaceAllString(s, "Game of the Year")
	// Trademark symbols become a space so "Velocity®2X" → "Velocity 2X".
	s = ReTrademark.ReplaceAllString(s, " ")
	s = reApostrophes.ReplaceAllString(s, "")
	s = reColons.ReplaceAllString(s, " ")
	// Standalone dashes only; in-word hyphens are preserved.
	s = reStandaloneDash.ReplaceAllString(s, " ")
	// Strip "(YYYY)", "(Classic)", and "(Classic, YYYY)" suffixes.
	s = reYearInParens.ReplaceAllString(s, "")
	s = reClassic.ReplaceAllString(s, "")
	s = reMultiWhitespace.ReplaceAllString(s, " ")
	s = strings.ToLower(strings.TrimSpace(s))
	// Strip leading "the " so "The X" and "X" compare equally.
	s = strings.TrimPrefix(s, "the ")
	// Fold diacritics to ASCII so "ABZÛ"→"abzu" and "Ōkami"→"okami" match.
	// The transformer is stateful and not safe for concurrent use, and
	// NormalizeTitle runs across concurrent workers (import_match, igdb_match),
	// so build a fresh one per call rather than sharing a package-level instance.
	// The allocation is negligible next to the IGDB lookups this feeds.
	folder := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if folded, _, err := transform.String(folder, s); err == nil {
		s = folded
	}
	return s
}

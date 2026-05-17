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
	reTrademark       = regexp.MustCompile(`[™®]`)
	reApostrophes     = regexp.MustCompile(`[''']`)
	reColons          = regexp.MustCompile(`:`)
	reStandaloneDash  = regexp.MustCompile(`\s-\s`)
	reYearInParens    = regexp.MustCompile(`\(\d{4}\)`)
	reClassic         = regexp.MustCompile(`(?i)\(classic(?:,\s*\d{4})?\)`)
	reMultiWhitespace = regexp.MustCompile(`\s+`)

	// diacriticFolder folds unicode diacritics to ASCII base characters
	// (ō→o, û→u, é→e) so titles like "ABZÛ" and "Ōkami HD" match correctly.
	diacriticFolder = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
)

// NormalizeTitle applies transformations for comparison purposes only.
// The result is never stored or displayed.
func NormalizeTitle(s string) string {
	// 1. Expand GOTY
	s = reGOTY.ReplaceAllString(s, "Game of the Year")
	// 2. Replace trademark symbols with a space so "Velocity®2X" → "Velocity 2X"
	s = reTrademark.ReplaceAllString(s, " ")
	// 3. Remove apostrophes
	s = reApostrophes.ReplaceAllString(s, "")
	// 4. Remove colons
	s = reColons.ReplaceAllString(s, " ")
	// 5. Remove standalone dashes (preserve in-word hyphens)
	s = reStandaloneDash.ReplaceAllString(s, " ")
	// 6. Remove year in parentheses; also strip "(Classic)" and "(Classic, YYYY)"
	s = reYearInParens.ReplaceAllString(s, "")
	s = reClassic.ReplaceAllString(s, "")
	// 7. Collapse whitespace
	s = reMultiWhitespace.ReplaceAllString(s, " ")
	// 8. Lowercase and trim
	s = strings.ToLower(strings.TrimSpace(s))
	// 9. Strip leading "the " so "The X" and "X" compare equally
	s = strings.TrimPrefix(s, "the ")
	// 10. Fold diacritics to ASCII so "ABZÛ"→"abzu" and "Ōkami"→"okami" match
	if folded, _, err := transform.String(diacriticFolder, s); err == nil {
		s = folded
	}
	return s
}

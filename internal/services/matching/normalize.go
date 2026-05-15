package matching

import (
	"regexp"
	"strings"
)

var (
	reGOTY            = regexp.MustCompile(`(?i)\bgoty\b`)
	reTrademark       = regexp.MustCompile(`[™®]`)
	reApostrophes     = regexp.MustCompile(`[''']`)
	reColons          = regexp.MustCompile(`:`)
	reStandaloneDash  = regexp.MustCompile(`\s-\s`)
	reYearInParens    = regexp.MustCompile(`\(\d{4}\)`)
	reMultiWhitespace = regexp.MustCompile(`\s+`)
)

// NormalizeTitle applies transformations for comparison purposes only.
// The result is never stored or displayed.
func NormalizeTitle(s string) string {
	// 1. Expand GOTY
	s = reGOTY.ReplaceAllString(s, "Game of the Year")
	// 2. Remove trademark symbols
	s = reTrademark.ReplaceAllString(s, "")
	// 3. Remove apostrophes
	s = reApostrophes.ReplaceAllString(s, "")
	// 4. Remove colons
	s = reColons.ReplaceAllString(s, " ")
	// 5. Remove standalone dashes (preserve in-word hyphens)
	s = reStandaloneDash.ReplaceAllString(s, " ")
	// 6. Remove year in parentheses
	s = reYearInParens.ReplaceAllString(s, "")
	// 7. Collapse whitespace
	s = reMultiWhitespace.ReplaceAllString(s, " ")
	// 8. Lowercase and trim
	s = strings.ToLower(strings.TrimSpace(s))
	// 9. Strip leading "the " so "The X" and "X" compare equally
	s = strings.TrimPrefix(s, "the ")
	return s
}

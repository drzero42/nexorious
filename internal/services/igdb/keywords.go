// internal/services/igdb/keywords.go
package igdb

import (
	"regexp"
	"slices"
	"strings"

	"github.com/drzero42/nexorious/internal/services/matching"
)

var (
	kwGOTY          = regexp.MustCompile(`(?i)\bgoty\b`)
	kwTelltale      = regexp.MustCompile(`(?i)the telltale series`)
	kwClassic       = regexp.MustCompile(`(?i)\(classic\)`)
	kwColon         = regexp.MustCompile(`:`)
	kwYearInParens  = regexp.MustCompile(`\(\d{4}\)`)
	kwStandaloneOne = regexp.MustCompile(`(?:^|\s)1(?:\s|$)`)
	kwOneExclusions = regexp.MustCompile(`(?i)(episode|chapter|part|vol|volume)\s+1`)
)

type keywordRule struct {
	pattern     *regexp.Regexp
	replacement string
	preCheck    func(query string) bool
}

var keywordRules = []keywordRule{
	{kwGOTY, "Game of the Year", nil},
	{kwTelltale, "", nil},
	{kwClassic, "", nil},
	{kwColon, " ", nil},
	{kwYearInParens, "", nil},
	{kwStandaloneOne, " ", func(query string) bool {
		return !kwOneExclusions.MatchString(query)
	}},
}

// expandQueries generates variant queries based on keyword detection.
// Returns at least the original (trimmed, symbol-sanitized) query. If keywords
// are detected, additional variants are appended.
func expandQueries(query string) []string {
	original := collapseWhitespace(matching.ReTrademark.ReplaceAllString(strings.TrimSpace(query), " "))
	results := []string{original}

	fullTransformed := original
	anyMatch := false

	for _, rule := range keywordRules {
		if rule.preCheck != nil && !rule.preCheck(original) {
			continue
		}
		if rule.pattern.MatchString(original) {
			anyMatch = true
			variant := rule.pattern.ReplaceAllString(original, rule.replacement)
			variant = collapseWhitespace(variant)
			results = append(results, variant)
			fullTransformed = rule.pattern.ReplaceAllString(fullTransformed, rule.replacement)
		}
	}

	// If multiple keywords matched, add the fully-transformed variant
	if anyMatch {
		fullTransformed = collapseWhitespace(fullTransformed)
		if fullTransformed != original && !slices.Contains(results, fullTransformed) {
			results = append(results, fullTransformed)
		}
	}

	return results
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

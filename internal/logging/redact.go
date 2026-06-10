package logging

import "strings"

// Redact returns a safe, non-reversible rendering of a sensitive string for
// logging. Short values (<=4 runes) become asterisks; longer values keep a
// 4-rune prefix as a weak correlation hint followed by a redaction marker.
func Redact(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 4 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:4]) + "…[redacted]"
}

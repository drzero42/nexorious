package logging

import (
	"regexp"
	"strings"
)

// Redact returns a safe, non-reversible rendering of a sensitive string for
// logging. Short values (<=4 runes) become asterisks; longer values keep a
// 4-rune prefix as a weak correlation hint followed by a redaction marker.
//
// It is an available helper, not currently wired into any call site: today's
// secret-handling rule is "log the error and a non-sensitive identifier, never
// the secret", and the ScrubURLQueries choke point below covers credential-
// bearing URLs. Reach for Redact if a value *derived* from sensitive material
// ever genuinely needs to appear in a log line.
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

// urlQueryRe matches an http(s) URL up to its query string — the same policy
// as the trace-side redaction in internal/observability/redact.go (#934).
// Go's *url.Error embeds the full request URL in its message, and outbound
// storefront APIs carry credentials in query params (Steam web_api_key, GOG
// client_secret/refresh_token), so the query must never reach a log line or a
// persisted error message.
var urlQueryRe = regexp.MustCompile(`(https?://[^?\s"']+)\?[^\s"']*`)

// ScrubURLQueries strips the query string from any http(s) URL embedded in
// free text. Text without a URL is returned unchanged (fast path).
func ScrubURLQueries(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	return urlQueryRe.ReplaceAllString(s, "$1")
}

// Package dbutil holds small SQL helper utilities shared across packages.
package dbutil

import "strings"

// likeEscaper escapes the LIKE/ILIKE wildcard metacharacters % and _, plus the
// backslash escape character itself. The backslash replacement must come first
// so already-escaped output is not double-escaped. This relies on PostgreSQL's
// default LIKE escape character, backslash, so no explicit ESCAPE clause is
// needed at the call site.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// EscapeLike escapes LIKE/ILIKE metacharacters in s so that the value matches
// literally rather than being interpreted as a wildcard pattern.
func EscapeLike(s string) string {
	return likeEscaper.Replace(s)
}

// LikeContains returns a LIKE/ILIKE pattern that matches any value containing s
// as a literal substring, with s's wildcard metacharacters escaped so that
// user-supplied % or _ cannot widen the match.
func LikeContains(s string) string {
	return "%" + EscapeLike(s) + "%"
}

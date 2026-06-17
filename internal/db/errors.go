// Package db holds shared database helpers used across the data layer.
package db

import "strings"

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505). pgdriver wraps the error and embeds the code in
// the message, so this matches on the substrings the driver emits.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique_violation")
}

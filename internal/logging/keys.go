// Package logging provides the structured-logging seam: a ctx-reading slog
// handler, canonical attribute keys, an error-category enum, a logging HTTP
// round-tripper, and redaction helpers.
package logging

// Canonical slog attribute keys. Use these constants instead of string
// literals so keys never drift across call sites.
const (
	KeyRequestID      = "request_id"
	KeyJobID          = "job_id"
	KeyRiverJobID     = "river_job_id"
	KeyJobType        = "job_type"
	KeyUserID         = "user_id"
	KeySource         = "source"
	KeyOperation      = "operation"
	KeyExternalGameID = "external_game_id"
	KeyDurationMS     = "duration_ms"
	KeyHost           = "host"
	KeyEndpoint       = "endpoint"
	KeyStatus         = "status"
	KeyRoute          = "route"
	KeyOutcome        = "outcome"
	KeyCategory       = "category"
	KeyErr            = "err"
)

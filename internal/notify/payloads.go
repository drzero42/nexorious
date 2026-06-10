package notify

// Typed event payloads — the single source of truth for the emit↔format
// contract. Every emit site constructs one of these; Format decodes into the
// same type. A renamed field or changed type therefore moves both sides
// together and cannot compile out of sync.

// DiffGame is one entry in a sync.diff payload.
type DiffGame struct {
	Title     string   `json:"title"`
	Platforms []string `json:"platforms"`
}

type SyncCompletedPayload struct {
	Storefront string `json:"storefront"`
	JobID      string `json:"job_id"`
}

type SyncCompletedWithErrorsPayload struct {
	Storefront string `json:"storefront"`
	Failed     int    `json:"failed"`
	JobID      string `json:"job_id"`
}

type SyncFailedPayload struct {
	Storefront string `json:"storefront"`
	Error      string `json:"error"`
	JobID      string `json:"job_id"`
}

// SyncAuthExpiredPayload carries the storefront whose credentials expired. No
// error string: it is always "credentials error", which is not useful to the
// user — the actionable message is "go reconnect this storefront".
type SyncAuthExpiredPayload struct {
	Storefront string `json:"storefront"`
}

type SyncNeedsReviewPayload struct {
	Storefront string `json:"storefront"`
	Count      int    `json:"count"`
	JobID      string `json:"job_id"`
}

type SyncDiffPayload struct {
	// Storefront is the storefront's human-readable display name (e.g. "Steam",
	// "PlayStation Store"), resolved from the storefronts reference table at
	// emit time — unlike the other sync payloads, which carry the raw source
	// slug. Empty when the source is unknown; Format then falls back to the
	// generic "Game library changes" title.
	Storefront string     `json:"storefront"`
	Added      []DiffGame `json:"added"`
	Removed    []DiffGame `json:"removed"`
	JobID      string     `json:"job_id"`
}

type ImportCompletedPayload struct {
	JobID string `json:"job_id"`
}

type ImportFailedPayload struct {
	JobID  string `json:"job_id"`
	Failed int    `json:"failed"`
	Error  string `json:"error"`
}

type ExportCompletedPayload struct {
	JobID    string `json:"job_id"`
	FilePath string `json:"file_path"`
}

type ExportFailedPayload struct {
	JobID string `json:"job_id"`
	Error string `json:"error"`
}

type BackupCompletedPayload struct {
	BackupID string `json:"backup_id"`
}

type BackupFailedPayload struct {
	Error string `json:"error"`
}

// MaintPayload is shared by admin.maintenance.completed and
// admin.maintenance.failed. The numeric fields are a union over what the
// maintenance jobs report (prune/metadata → count; orphaned-items →
// rescued+failed; stale-jobs → count); all optional. Format renders only
// Action and Error.
type MaintPayload struct {
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
	Count   int    `json:"count,omitempty"`
	Rescued int    `json:"rescued,omitempty"`
	Failed  int    `json:"failed,omitempty"`
}

// VersionAvailablePayload announces that a newer release than the running
// build is available on GitHub.
type VersionAvailablePayload struct {
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version"`
	ReleaseURL       string `json:"release_url"`
}

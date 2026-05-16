package models

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// ─── Job ─────────────────────────────────────────────────────────────────────

// Job type constants.
const (
	JobTypeSync            = "sync"
	JobTypeImport          = "import"
	JobTypeExport          = "export"
	JobTypeMetadataRefresh = "metadata_refresh"
)

// Job source constants.
const (
	JobSourceSteam     = "steam"
	JobSourceEpic      = "epic"
	JobSourcePSN       = "psn"
	JobSourceGOG       = "gog"
	JobSourceManual    = "manual"
	JobSourceNexorious = "nexorious"
	JobSourceCSV       = "csv"
	JobSourceSystem    = "system"
)

// Job status constants.
const (
	JobStatusPending             = "pending"
	JobStatusProcessing          = "processing"
	JobStatusCompleted           = "completed"
	JobStatusFailed              = "failed"
	JobStatusCancelled           = "cancelled"
	JobStatusCompletedWithErrors = "completed_with_errors"
)

// Job priority constants.
const (
	JobPriorityLow    = "low"
	JobPriorityNormal = "normal"
	JobPriorityHigh   = "high"
)

type Job struct {
	bun.BaseModel `bun:"table:jobs"`

	ID            string     `bun:"id,pk"                    json:"id"`
	UserID        string     `bun:"user_id,notnull"          json:"user_id"`
	JobType       string     `bun:"job_type,notnull"         json:"job_type"`
	Source        string     `bun:"source,notnull"           json:"source"`
	Status        string     `bun:"status,notnull"           json:"status"`
	Priority      string     `bun:"priority,notnull"         json:"priority"`
	FilePath      *string    `bun:"file_path"                json:"file_path"`
	TotalItems    int        `bun:"total_items,notnull"      json:"total_items"`
	ErrorMessage  *string    `bun:"error_message"            json:"error_message"`
	AutoRetryDone bool       `bun:"auto_retry_done,notnull"  json:"auto_retry_done"`
	CreatedAt     time.Time  `bun:"created_at,notnull"       json:"created_at"`
	StartedAt     *time.Time `bun:"started_at"               json:"started_at"`
	CompletedAt   *time.Time `bun:"completed_at"             json:"completed_at"`
}

// IsActive returns true if the job is pending or processing.
func (j *Job) IsActive() bool {
	return j.Status == JobStatusPending || j.Status == JobStatusProcessing
}

// IsTerminal returns true if the job has reached a terminal state.
func (j *Job) IsTerminal() bool {
	return j.Status == JobStatusCompleted ||
		j.Status == JobStatusFailed ||
		j.Status == JobStatusCancelled ||
		j.Status == JobStatusCompletedWithErrors
}

// DurationSeconds returns elapsed seconds from StartedAt to CompletedAt
// (or now if still running). Returns nil if StartedAt is not set.
func (j *Job) DurationSeconds() *float64 {
	if j.StartedAt == nil {
		return nil
	}
	end := time.Now()
	if j.CompletedAt != nil {
		end = *j.CompletedAt
	}
	d := end.Sub(*j.StartedAt).Seconds()
	return &d
}

// ─── JobItem ─────────────────────────────────────────────────────────────────

// JobItem status constants.
const (
	JobItemStatusPending       = "pending"
	JobItemStatusProcessing    = "processing"
	JobItemStatusCompleted     = "completed"
	JobItemStatusPendingReview = "pending_review"
	JobItemStatusSkipped       = "skipped"
	JobItemStatusFailed        = "failed"
	JobItemStatusIGDBFailed    = "igdb_failed"
)

type JobItem struct {
	bun.BaseModel `bun:"table:job_items"`

	ID              string          `bun:"id,pk"                    json:"id"`
	JobID           string          `bun:"job_id,notnull"           json:"job_id"`
	UserID          string          `bun:"user_id,notnull"          json:"user_id"`
	ItemKey         string          `bun:"item_key,notnull"         json:"item_key"`
	SourceTitle     string          `bun:"source_title,notnull"     json:"source_title"`
	SourceMetadata  json.RawMessage `bun:"source_metadata,notnull"  json:"source_metadata"`
	Status          string          `bun:"status,notnull"           json:"status"`
	Result          json.RawMessage `bun:"result,notnull"           json:"result"`
	ErrorMessage    *string         `bun:"error_message"            json:"error_message"`
	IGDBCandidates  json.RawMessage `bun:"igdb_candidates,notnull"  json:"igdb_candidates"`
	ResolvedIGDBID  *int            `bun:"resolved_igdb_id"         json:"resolved_igdb_id"`
	MatchConfidence *float64        `bun:"match_confidence"         json:"match_confidence"`
	CreatedAt       time.Time       `bun:"created_at,notnull"       json:"created_at"`
	ProcessedAt     *time.Time      `bun:"processed_at"             json:"processed_at"`
	ResolvedAt      *time.Time      `bun:"resolved_at"              json:"resolved_at"`
}

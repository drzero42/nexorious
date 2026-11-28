package models

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/uptrace/bun"
)

// ─── Job ─────────────────────────────────────────────────────────────────────

// Job type constants.
const (
	JobTypeSync             = "sync"
	JobTypeImport           = "import"
	JobTypeExport           = "export"
	JobTypeMetadataRefresh  = "metadata_refresh"
	JobTypeStoreLinkRefresh = "store_link_refresh"
)

// JobTypes returns every Job type.
func JobTypes() []string {
	return []string{
		JobTypeSync, JobTypeImport, JobTypeExport,
		JobTypeMetadataRefresh, JobTypeStoreLinkRefresh,
	}
}

// Job source constants.
const (
	JobSourceSteam            = "steam"
	JobSourceEpicGamesStore   = "epic-games-store"
	JobSourcePlaystationStore = "playstation-store"
	JobSourceGOG              = "gog"
	JobSourceHumbleBundle     = "humble-bundle"
	JobSourceAmazonGames      = "amazon-games"
	JobSourceManual           = "manual"
	JobSourceNexorious        = "nexorious"
	JobSourceVglist           = "vglist"
	JobSourceCSV              = "csv"
	JobSourceSystem           = "system"
)

// JobStatus is a value of jobs.status. The jobs_status_check constraint lists
// every value: a new one needs a migration that widens it.
type JobStatus string

// Job status constants.
const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
)

// JobStatuses returns every Job status.
func JobStatuses() []JobStatus {
	return []JobStatus{
		JobStatusPending, JobStatusProcessing, JobStatusCompleted,
		JobStatusFailed, JobStatusCancelled,
	}
}

// Valid reports whether s is a Job status.
func (s JobStatus) Valid() bool {
	return slices.Contains(JobStatuses(), s)
}

// ActiveJobStatuses returns the statuses of a Job that has not ended.
func ActiveJobStatuses() []JobStatus {
	return []JobStatus{JobStatusPending, JobStatusProcessing}
}

// EndedJobStatuses returns the statuses of a Job that has ended.
func EndedJobStatuses() []JobStatus {
	return []JobStatus{JobStatusCompleted, JobStatusFailed, JobStatusCancelled}
}

// Job priority constants.
const (
	JobPriorityLow    = "low"
	JobPriorityNormal = "normal"
	JobPriorityHigh   = "high"
)

type Job struct {
	bun.BaseModel `bun:"table:jobs"`

	ID               string    `bun:"id,pk"                    json:"id"`
	UserID           string    `bun:"user_id,notnull"          json:"user_id"`
	JobType          string    `bun:"job_type,notnull"         json:"job_type"`
	Source           string    `bun:"source,notnull"           json:"source"`
	Status           JobStatus `bun:"status,notnull"           json:"status"`
	Priority         string    `bun:"priority,notnull"         json:"priority"`
	FilePath         *string   `bun:"file_path"                json:"file_path"`
	TotalItems       int       `bun:"total_items,notnull"      json:"total_items"`
	ErrorMessage     *string   `bun:"error_message"            json:"error_message"`
	DispatchComplete bool      `bun:"dispatch_complete,notnull" json:"-"`
	// RiverJobID is the River job that owns the Job's dispatch while it is not
	// done (internal/joblifecycle); NULL otherwise.
	RiverJobID  *int64     `bun:"river_job_id" json:"-"`
	CreatedAt   time.Time  `bun:"created_at,nullzero,notnull"        json:"created_at"`
	StartedAt   *time.Time `bun:"started_at"               json:"started_at"`
	CompletedAt *time.Time `bun:"completed_at"             json:"completed_at"`
}

// IsActive reports whether the job has not ended (ActiveJobStatuses).
func (j *Job) IsActive() bool {
	return slices.Contains(ActiveJobStatuses(), j.Status)
}

// IsTerminal reports whether the job has ended (EndedJobStatuses).
func (j *Job) IsTerminal() bool {
	return slices.Contains(EndedJobStatuses(), j.Status)
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

// JobItemStatus is a value of job_items.status. The job_items_status_check
// constraint lists every value: a new one needs a migration that widens it.
type JobItemStatus string

// JobItem status constants.
const (
	JobItemStatusPending       JobItemStatus = "pending"
	JobItemStatusCompleted     JobItemStatus = "completed"
	JobItemStatusPendingReview JobItemStatus = "pending_review"
	JobItemStatusSkipped       JobItemStatus = "skipped"
	JobItemStatusFailed        JobItemStatus = "failed"
	JobItemStatusCancelled     JobItemStatus = "cancelled"
)

// JobItemStatuses returns every Job item status.
func JobItemStatuses() []JobItemStatus {
	return []JobItemStatus{
		JobItemStatusPending, JobItemStatusCompleted, JobItemStatusPendingReview,
		JobItemStatusSkipped, JobItemStatusFailed, JobItemStatusCancelled,
	}
}

// Valid reports whether s is a Job item status.
func (s JobItemStatus) Valid() bool {
	return slices.Contains(JobItemStatuses(), s)
}

// OpenJobItemStatuses returns the statuses of a Job item that has not ended:
// a worker (pending) or the user (pending_review) acts next.
func OpenJobItemStatuses() []JobItemStatus {
	return []JobItemStatus{JobItemStatusPending, JobItemStatusPendingReview}
}

type JobItem struct {
	bun.BaseModel `bun:"table:job_items"`

	ID              string          `bun:"id,pk"                    json:"id"`
	JobID           string          `bun:"job_id,notnull"           json:"job_id"`
	UserID          string          `bun:"user_id,notnull"          json:"user_id"`
	ExternalGameID  *string         `bun:"external_game_id"         json:"external_game_id"`
	ItemKey         string          `bun:"item_key,notnull"         json:"item_key"`
	SourceTitle     string          `bun:"source_title,notnull"     json:"source_title"`
	SourceMetadata  json.RawMessage `bun:"source_metadata,notnull"  json:"source_metadata"`
	Status          JobItemStatus   `bun:"status,notnull"           json:"status"`
	Result          json.RawMessage `bun:"result,notnull"           json:"result"`
	ErrorMessage    *string         `bun:"error_message"            json:"error_message"`
	IGDBCandidates  json.RawMessage `bun:"igdb_candidates,notnull"  json:"igdb_candidates"`
	ResolvedIGDBID  *int            `bun:"resolved_igdb_id"         json:"resolved_igdb_id"`
	MatchConfidence *float64        `bun:"match_confidence"         json:"match_confidence"`
	CreatedAt       time.Time       `bun:"created_at,nullzero,notnull"       json:"created_at"`
	ProcessedAt     *time.Time      `bun:"processed_at"             json:"processed_at"`
	ResolvedAt      *time.Time      `bun:"resolved_at"              json:"resolved_at"`
	// RiverJobID is the River job that owns a pending item
	// (internal/joblifecycle); NULL once the item leaves pending.
	RiverJobID *int64 `bun:"river_job_id" json:"-"`
}

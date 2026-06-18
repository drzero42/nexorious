package cliclient

import (
	"net/http"
	"net/url"
)

// JobProgress holds the per-status item counts for a job.
type JobProgress struct {
	Pending       int `json:"pending"`
	Processing    int `json:"processing"`
	Completed     int `json:"completed"`
	PendingReview int `json:"pending_review"`
	Skipped       int `json:"skipped"`
	Failed        int `json:"failed"`
	Total         int `json:"total"`
	Percent       int `json:"percent"`
}

// Job is one import/sync job as returned by GET /api/jobs and GET /api/jobs/:id.
// This is a display subset: user_id is intentionally omitted since the CLI is
// always scoped to the authenticated profile and never renders it.
type Job struct {
	ID              string      `json:"id"`
	JobType         string      `json:"job_type"`
	Source          string      `json:"source"`
	Status          string      `json:"status"`
	Priority        string      `json:"priority"`
	FilePath        *string     `json:"file_path"`
	TotalItems      int         `json:"total_items"`
	ErrorMessage    *string     `json:"error_message"`
	AutoRetryDone   bool        `json:"auto_retry_done"`
	CreatedAt       string      `json:"created_at"`
	StartedAt       *string     `json:"started_at"`
	CompletedAt     *string     `json:"completed_at"`
	IsTerminal      bool        `json:"is_terminal"`
	DurationSeconds *float64    `json:"duration_seconds"`
	Progress        JobProgress `json:"progress"`
}

// JobsPage is the paged response envelope for GET /api/jobs.
type JobsPage struct {
	Jobs       []Job `json:"jobs"`
	Total      int   `json:"total"`
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalPages int   `json:"total_pages"`
}

// JobItem is one item within a job (subset — blob fields omitted).
type JobItem struct {
	ID              string   `json:"id"`
	JobID           string   `json:"job_id"`
	ExternalGameID  *string  `json:"external_game_id"`
	ItemKey         string   `json:"item_key"`
	SourceTitle     string   `json:"source_title"`
	Status          string   `json:"status"`
	ErrorMessage    *string  `json:"error_message"`
	ResolvedIgdbID  *int     `json:"resolved_igdb_id"`
	MatchConfidence *float64 `json:"match_confidence"`
	CreatedAt       string   `json:"created_at"`
	ProcessedAt     *string  `json:"processed_at"`
	ResolvedAt      *string  `json:"resolved_at"`
}

// JobItemsPage is the paged response envelope for GET /api/jobs/:id/items.
type JobItemsPage struct {
	Items      []JobItem `json:"items"`
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	PerPage    int       `json:"per_page"`
	TotalPages int       `json:"total_pages"`
}

// RetryResult is the response from POST /api/jobs/:id/retry-failed.
type RetryResult struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	RetriedCount int    `json:"retried_count"`
}

// ListJobs returns a paged list of jobs filtered by the given query params.
// Supported params: page, per_page, sort_by, sort_order, job_type, source, status.
func (c *Client) ListJobs(key string, params url.Values) (*JobsPage, error) {
	path := "/api/jobs"
	if enc := params.Encode(); enc != "" {
		path += "?" + enc
	}
	var out JobsPage
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetJob fetches a single job by id.
func (c *Client) GetJob(key, id string) (*Job, error) {
	var out Job
	if err := c.doBearer(http.MethodGet, "/api/jobs/"+url.PathEscape(id), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetJobItems returns the paged items for a job, filtered by the given query params.
// Supported params: page, per_page, status.
func (c *Client) GetJobItems(key, id string, params url.Values) (*JobItemsPage, error) {
	path := "/api/jobs/" + url.PathEscape(id) + "/items"
	if enc := params.Encode(); enc != "" {
		path += "?" + enc
	}
	var out JobItemsPage
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RetryFailedJob re-enqueues all failed items for a job.
func (c *Client) RetryFailedJob(key, id string) (*RetryResult, error) {
	var out RetryResult
	if err := c.doBearer(http.MethodPost, "/api/jobs/"+url.PathEscape(id)+"/retry-failed", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelJob cancels a running job.
func (c *Client) CancelJob(key, id string) error {
	return c.doBearer(http.MethodPost, "/api/jobs/"+url.PathEscape(id)+"/cancel", key, nil, nil)
}

package cliclient

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ExportResult is the envelope returned by POST /api/export/json and /api/export/csv.
type ExportResult struct {
	JobID          string `json:"job_id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	EstimatedItems int    `json:"estimated_items"`
}

// TriggerExport enqueues an export job for the given format ("json" or "csv").
// It returns an error immediately for any unsupported format string without
// making a network request.
func (c *Client) TriggerExport(key, format string) (*ExportResult, error) {
	if format != "json" && format != "csv" {
		return nil, fmt.Errorf("unsupported export format %q", format)
	}
	var out ExportResult
	if err := c.doBearer(http.MethodPost, "/api/export/"+format, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadExport streams the completed export file for jobID into w.
// It builds the request directly rather than through doBearer because the
// response body is a raw file stream, not a JSON envelope.
func (c *Client) DownloadExport(key, jobID string, w io.Writer) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/export/"+url.PathEscape(jobID)+"/download", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return httpError(resp)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("stream response: %w", err)
	}
	return nil
}

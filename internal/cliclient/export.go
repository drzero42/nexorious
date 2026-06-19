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

// OpenExportDownload opens the completed export file for jobID.
// It builds the request directly rather than through doBearer because the
// response body is a raw file stream, not a JSON envelope. It returns the
// server-suggested filename (parsed from Content-Disposition, "" when absent)
// and the open body, which the caller must Close. On a non-2xx response the
// body is consumed/closed and a nil body is returned alongside the error.
func (c *Client) OpenExportDownload(key, jobID string) (filename string, body io.ReadCloser, err error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/export/"+url.PathEscape(jobID)+"/download", nil)
	if err != nil {
		return "", nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer func() { _ = resp.Body.Close() }()
		return "", nil, httpError(resp)
	}
	return filenameFromContentDisposition(resp.Header.Get("Content-Disposition")), resp.Body, nil
}

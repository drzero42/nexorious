package cliclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
)

// ImportResult is the response from POST /api/import/nexorious,
// POST /api/import/csv, and POST /api/import/:slug.
type ImportResult struct {
	JobID        string `json:"job_id"`
	Source       string `json:"source"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	TotalItems   int    `json:"total_items"`
	SkippedCount int    `json:"skipped_count"`
	// Auto is set only by POST /api/import/csv's auto mode (no format, no
	// mapping); nil for preset/manual imports and the other import endpoints.
	Auto *CSVAutoResolution `json:"auto,omitempty"`
}

// CSVAutoResolution describes how POST /api/import/csv's auto mode mapped the
// file: a preset matched by signature, or a guessed column mapping.
type CSVAutoResolution struct {
	Mode    string               `json:"mode"`              // "preset" | "guessed"
	Preset  *CSVPreset           `json:"preset,omitempty"`  // set when mode=="preset"
	Mapping *CSVSuggestedMapping `json:"mapping,omitempty"` // set when mode=="guessed"
}

// ImportSource is one registered import source as returned by GET /api/import/sources.
// Mapper is a server-only field and is intentionally omitted.
type ImportSource struct {
	Slug        string   `json:"slug"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Accept      []string `json:"accept"`
	Features    []string `json:"features"`
}

// CSVColumn is one column's name plus a capped set of its distinct non-empty values.
type CSVColumn struct {
	Name              string   `json:"name"`
	DistinctValues    []string `json:"distinct_values"`
	DistinctTruncated bool     `json:"distinct_truncated"`
}

// CSVPreset is one selectable known CSV format for the import dialog.
type CSVPreset struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// CSVSuggestedMapping mirrors the JSON shape of csvmap.SuggestedMapping. It is
// a plain local type so this package does not import internal/services/csvmap.
type CSVSuggestedMapping struct {
	Columns struct {
		Title        string `json:"title"`
		IGDBID       string `json:"igdb_id"`
		Platform     string `json:"platform"`
		Storefront   string `json:"storefront"`
		Rating       string `json:"rating"`
		Notes        string `json:"notes"`
		AcquiredDate string `json:"acquired_date"`
		HoursPlayed  string `json:"hours_played"`
		Tags         string `json:"tags"`
		Loved        string `json:"loved"`
	} `json:"columns"`
	Status struct {
		Column   string            `json:"column"`
		ValueMap map[string]string `json:"value_map"`
	} `json:"status"`
	RatingScale  int  `json:"rating_scale"`
	MergeByTitle bool `json:"merge_by_title"`
}

// CSVInspect is the response from POST /api/import/csv/inspect.
type CSVInspect struct {
	Headers          []string            `json:"headers"`
	RowCount         int                 `json:"row_count"`
	Columns          []CSVColumn         `json:"columns"`
	SuggestedMapping CSVSuggestedMapping `json:"suggested_mapping"`
	Presets          []CSVPreset         `json:"presets"`
	Detected         *CSVPreset          `json:"detected,omitempty"`
}

// doBearerMultipart performs an authenticated multipart/form-data request. It
// writes one file part (field "file", given filename) streamed from body and
// any additional text fields from fields. On a 2xx response a non-nil out is
// decoded from the body (skipped for 204). Non-2xx responses become an
// httpError.
//
// The request body is built with an io.Pipe so the multipart payload is never
// fully buffered in memory: a goroutine writes the parts while http.Client
// reads them incrementally (chunked transfer encoding). This lets large
// uploads (e.g. backup restore) stream straight from a file.
func (c *Client) doBearerMultipart(method, path, key, filename string, body io.Reader, fields map[string]string, out any) error {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	// FormDataContentType() returns the boundary chosen at NewWriter time, so
	// it is stable and safe to read before the goroutine starts writing.
	contentType := mw.FormDataContentType()

	go func() {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			_ = pw.CloseWithError(fmt.Errorf("create form file: %w", err))
			return
		}
		if _, err := io.Copy(fw, body); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("write form file: %w", err))
			return
		}
		for k, v := range fields {
			if err := mw.WriteField(k, v); err != nil {
				_ = pw.CloseWithError(fmt.Errorf("write form field %q: %w", k, err))
				return
			}
		}
		// Close writes the boundary trailer; close the pipe to signal EOF.
		if err := mw.Close(); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("close multipart writer: %w", err))
			return
		}
		_ = pw.Close()
	}()

	req, err := http.NewRequest(method, c.baseURL+path, pr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return httpError(resp)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// ListImportSources returns the registered mapper-based import sources from
// GET /api/import/sources (bare JSON array).
func (c *Client) ListImportSources(key string) ([]ImportSource, error) {
	var out []ImportSource
	if err := c.doBearer(http.MethodGet, "/api/import/sources", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ImportNexorious uploads a Nexorious export file via
// POST /api/import/nexorious and returns the created import job.
func (c *Client) ImportNexorious(key, filename string, data []byte) (*ImportResult, error) {
	var out ImportResult
	if err := c.doBearerMultipart(http.MethodPost, "/api/import/nexorious", key, filename, bytes.NewReader(data), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// InspectCSV uploads a CSV file via POST /api/import/csv/inspect and returns
// the inspection envelope (headers, row count, per-column metadata, and a
// suggested mapping).
func (c *Client) InspectCSV(key, filename string, data []byte) (*CSVInspect, error) {
	var out CSVInspect
	if err := c.doBearerMultipart(http.MethodPost, "/api/import/csv/inspect", key, filename, bytes.NewReader(data), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ImportCSV uploads a CSV file via POST /api/import/csv with an optional
// format preset slug and/or a JSON mapping blob. format is omitted when empty;
// mapping is omitted when nil/empty.
func (c *Client) ImportCSV(key, filename string, data []byte, format string, mapping json.RawMessage) (*ImportResult, error) {
	fields := make(map[string]string)
	if format != "" {
		fields["format"] = format
	}
	if len(mapping) > 0 {
		fields["mapping"] = string(mapping)
	}
	var out ImportResult
	if err := c.doBearerMultipart(http.MethodPost, "/api/import/csv", key, filename, bytes.NewReader(data), fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ImportSource uploads a file for a registered import source via
// POST /api/import/:slug and returns the created import job.
func (c *Client) ImportSource(key, slug, filename string, data []byte) (*ImportResult, error) {
	var out ImportResult
	path := "/api/import/" + url.PathEscape(slug)
	if err := c.doBearerMultipart(http.MethodPost, path, key, filename, bytes.NewReader(data), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

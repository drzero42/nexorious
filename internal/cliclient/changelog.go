package cliclient

import (
	"net/http"
	"net/url"
)

// ChangelogGroup is a titled list of change descriptions.
type ChangelogGroup struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

// ChangelogEntry is one released version's notes.
type ChangelogEntry struct {
	Version string           `json:"version"`
	Date    string           `json:"date"`
	Groups  []ChangelogGroup `json:"groups"`
}

// ChangelogResult is the GET /api/changelog response.
type ChangelogResult struct {
	Available bool             `json:"available"`
	Current   string           `json:"current"`
	LastSeen  string           `json:"last_seen"`
	Markdown  string           `json:"markdown"`
	Entries   []ChangelogEntry `json:"entries"`
}

// GetChangelog fetches the changelog. With rangeAll it requests the full
// history (auto-marks seen server-side); with a non-empty since it requests a
// pure read of entries newer than that version; otherwise it requests the
// since-last diff (auto-marks seen). rangeAll and since are mutually exclusive;
// since takes precedence if both are set.
func (c *Client) GetChangelog(key string, rangeAll bool, since string) (*ChangelogResult, error) {
	q := url.Values{}
	switch {
	case since != "":
		q.Set("since", since)
	case rangeAll:
		q.Set("range", "all")
	}
	path := "/api/changelog"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out ChangelogResult
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

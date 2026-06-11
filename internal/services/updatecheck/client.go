package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
)

// Release is the subset of the GitHub "latest release" API response we need.
// The endpoint returns only the latest stable release (drafts and
// pre-releases excluded).
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Client fetches the latest Nexorious release from the GitHub API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient returns a Client pointed at the real GitHub API.
func NewClient() *Client {
	return NewClientWithBaseURL("https://api.github.com")
}

// NewClientWithBaseURL returns a Client with a custom API base URL (tests).
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second, Transport: logging.NewRoundTripper(nil)},
		baseURL:    baseURL,
	}
}

// FetchLatest returns the latest stable release of drzero42/nexorious.
func (c *Client) FetchLatest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/repos/drzero42/nexorious/releases/latest", nil)
	if err != nil {
		return Release{}, fmt.Errorf("updatecheck: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nexorious-update-check")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("updatecheck: fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("updatecheck: GitHub returned status %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("updatecheck: decode release: %w", err)
	}
	if rel.TagName == "" {
		return Release{}, fmt.Errorf("updatecheck: release response has empty tag_name")
	}
	return rel, nil
}

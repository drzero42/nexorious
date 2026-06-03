// Package cliclient is a thin HTTP client over the Nexorious /api/auth/*,
// /api/migrate/*, and /health endpoints used by the CLI to bootstrap an admin,
// run migrations, and manage an API key.
package cliclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const sessionCookieName = "session_id"

// defaultScopes is the scope requested for CLI-minted keys. The server accepts
// "read" or "write"; the CLI always mints write-scoped keys.
const defaultScopes = "write"

// Client talks to one Nexorious server.
type Client struct {
	baseURL string
	hc      *http.Client
}

// New returns a Client for the given base URL (trailing slash trimmed). The
// client does not follow redirects: a gate's 302 is an observable response, so
// callers (e.g. setup) can read its Location instead of silently chasing it.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

type errorBody struct {
	Message string `json:"message"`
}

// httpError decodes an Echo error response (`{"message":"…"}`) into a readable
// error including the status code.
func httpError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("server returned %d (failed reading body: %w)", resp.StatusCode, err)
	}
	var eb errorBody
	if json.Unmarshal(body, &eb) == nil && eb.Message != "" {
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, eb.Message)
	}
	return fmt.Errorf("server returned %d", resp.StatusCode)
}

// Login posts credentials and returns the raw session_id cookie value. The value
// is read straight off the response (not via a cookie jar) so a Secure-flagged
// cookie issued over http://localhost is still usable for the follow-up calls.
func (c *Client) Login(username, password string) (string, error) {
	payload, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return "", fmt.Errorf("marshal login: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/login", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookieName {
			return ck.Value, nil
		}
	}
	return "", fmt.Errorf("login succeeded but no %s cookie was returned", sessionCookieName)
}

type createAPIKeyResp struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// CreateAPIKey mints a write-scoped key named `name`, authenticating with the
// session cookie. Returns the raw key and its server-side id.
func (c *Client) CreateAPIKey(sessionID, name string) (string, string, error) {
	payload, err := json.Marshal(map[string]string{"name": name, "scopes": defaultScopes})
	if err != nil {
		return "", "", fmt.Errorf("marshal create key: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/api-keys", bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("build create key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID}) //nolint:gosec // outbound request cookie from a CLI client; Secure/HttpOnly are response-cookie attributes that don't apply here

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("create key request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", httpError(resp)
	}
	var out createAPIKeyResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode create key response: %w", err)
	}
	return out.Key, out.ID, nil
}

// APIKey describes one API key as returned by the /api/auth/api-keys endpoints.
// Key is only populated by CreateAPIKeyWithBearer (the raw value is shown once at
// creation); list responses never include it.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     string     `json:"scopes"`
	Key        string     `json:"key,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// ListAPIKeys returns the caller's non-revoked API keys, authenticating with the
// key itself as a Bearer token.
func (c *Client) ListAPIKeys(key string) ([]APIKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/auth/api-keys", nil)
	if err != nil {
		return nil, fmt.Errorf("build list keys request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list keys request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, httpError(resp)
	}
	var out []APIKey
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode list keys response: %w", err)
	}
	return out, nil
}

// CreateAPIKeyWithBearer mints a key authenticating with an existing key as a
// Bearer token (used by `api-key generate`). When expiresAt is non-nil it is sent
// as the request's expires_at (the server validates the RFC3339 format). The
// returned APIKey includes the raw Key, shown exactly once.
func (c *Client) CreateAPIKeyWithBearer(key, name, scopes string, expiresAt *string) (APIKey, error) {
	body := map[string]string{"name": name, "scopes": scopes}
	if expiresAt != nil {
		body["expires_at"] = *expiresAt
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return APIKey{}, fmt.Errorf("marshal create key: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/api-keys", bytes.NewReader(payload))
	if err != nil {
		return APIKey{}, fmt.Errorf("build create key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return APIKey{}, fmt.Errorf("create key request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return APIKey{}, httpError(resp)
	}
	var out APIKey
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return APIKey{}, fmt.Errorf("decode create key response: %w", err)
	}
	return out, nil
}

// revoke issues DELETE /api/auth/api-keys/:id with caller-supplied auth.
func (c *Client) revoke(keyID string, auth func(*http.Request)) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/api/auth/api-keys/"+keyID, nil)
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	auth(req)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("revoke request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return httpError(resp)
	}
	return nil
}

// RevokeAPIKeyWithCookie revokes a key using a session cookie (used during
// login rotation, before the new key exists).
func (c *Client) RevokeAPIKeyWithCookie(sessionID, keyID string) error {
	return c.revoke(keyID, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID}) //nolint:gosec // outbound request cookie from a CLI client; Secure/HttpOnly are response-cookie attributes that don't apply here
	})
}

// RevokeAPIKeyWithBearer revokes a key using the key itself as a Bearer token
// (used by logout).
func (c *Client) RevokeAPIKeyWithBearer(key, keyID string) error {
	return c.revoke(keyID, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+key)
	})
}

type healthResp struct {
	Status string `json:"status"`
}

// Health performs the GET /health preflight and returns the reported status
// ("ok" when the server is ready, otherwise the app-state name such as
// "needs_migration" or "db_unavailable").
func (c *Client) Health() (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return "", fmt.Errorf("build health request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("health request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	var out healthResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode health response: %w", err)
	}
	return out.Status, nil
}

// SetupResult is the interpreted outcome of a setup-admin attempt. The caller
// maps StatusCode (and Location for a 3xx redirect) to a message and exit code.
type SetupResult struct {
	StatusCode int
	Location   string // Location header, set when StatusCode is a 3xx redirect
	Message    string // server {"message":...}, set for 4xx when present
}

// SetupAdmin posts the first-admin credentials to POST /api/auth/setup/admin.
// It returns a SetupResult for any HTTP response (including 3xx/4xx) so the
// caller can map the outcome; it returns a non-nil error only for transport
// failures (e.g. the server is unreachable). Redirects are not followed, so a
// gate's 302 is observable via Location.
func (c *Client) SetupAdmin(username, password string) (*SetupResult, error) {
	payload, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return nil, fmt.Errorf("marshal setup: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/setup/admin", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build setup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("setup request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	res := &SetupResult{StatusCode: resp.StatusCode}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		res.Location = resp.Header.Get("Location")
		return res, nil
	}
	if resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr == nil {
			var eb errorBody
			if json.Unmarshal(body, &eb) == nil {
				res.Message = eb.Message
			}
		}
	}
	return res, nil
}

// Logout drops the throwaway session created during login.
func (c *Client) Logout(sessionID string) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/logout", nil)
	if err != nil {
		return fmt.Errorf("build logout request: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID}) //nolint:gosec // outbound request cookie from a CLI client; Secure/HttpOnly are response-cookie attributes that don't apply here

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("logout request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return httpError(resp)
	}
	return nil
}

type meResp struct {
	Username string `json:"username"`
}

// Me returns the authenticated username for the given API key.
func (c *Client) Me(key string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/auth/me", nil)
	if err != nil {
		return "", fmt.Errorf("build me request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("me request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	var out meResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode me response: %w", err)
	}
	return out.Username, nil
}

// RunMigrations triggers POST /api/migrate/run on the running server, so the
// server's own migrator applies pending migrations and its in-memory state
// transitions to ready. 202 ("migration started"), 400 ("already up to date"),
// and 409 ("in progress") are all treated as success (nil) — the caller then
// polls MigrationStatus to learn the outcome. Other responses return an error.
func (c *Client) RunMigrations() error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/migrate/run", nil)
	if err != nil {
		return fmt.Errorf("build migrate request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusAccepted, http.StatusBadRequest, http.StatusConflict:
		return nil
	default:
		return httpError(resp)
	}
}

type migrationStatusResp struct {
	State        string `json:"state"`
	PendingCount int    `json:"pending_count"`
	Error        string `json:"error"`
}

// MigrationStatus returns the server's migration state from
// GET /api/migrate/status ("needs_migration", "migrating", "ready",
// "migration_failed", or "db_unavailable") along with the server's failure
// detail (the "error" field, populated only in the "migration_failed" state;
// empty otherwise).
func (c *Client) MigrationStatus() (state, detail string, err error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/migrate/status", nil)
	if err != nil {
		return "", "", fmt.Errorf("build status request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", httpError(resp)
	}
	var out migrationStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode status response: %w", err)
	}
	return out.State, out.Error, nil
}

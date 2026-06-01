// Package cliclient is a thin HTTP client over the Nexorious /api/auth/*
// endpoints used by the CLI to bootstrap and manage an API key.
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

// Client talks to one Nexorious server.
type Client struct {
	baseURL string
	hc      *http.Client
}

// New returns a Client for the given base URL (trailing slash trimmed).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 30 * time.Second},
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
	payload, err := json.Marshal(map[string]string{"name": name, "scopes": "write"})
	if err != nil {
		return "", "", fmt.Errorf("marshal create key: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/api-keys", bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("build create key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})

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
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	})
}

// RevokeAPIKeyWithBearer revokes a key using the key itself as a Bearer token
// (used by logout).
func (c *Client) RevokeAPIKeyWithBearer(key, keyID string) error {
	return c.revoke(keyID, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+key)
	})
}

// Logout drops the throwaway session created during login.
func (c *Client) Logout(sessionID string) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/logout", nil)
	if err != nil {
		return fmt.Errorf("build logout request: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})

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

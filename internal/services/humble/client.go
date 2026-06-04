package humble

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL    = "https://www.humblebundle.com"
	requestedByHeader = "hb_android_app"
)

// ErrCredentials is returned when Humble rejects the session cookie (401/403).
// The adapter wraps it into storefrontadapter.ErrCredentials.
var ErrCredentials = errors.New("humble: invalid session cookie")

// Client talks to the Humble Bundle order API using a pasted _simpleauth_sess
// session cookie. It rate-limits requests to 5/sec (matching steam/psn).
type Client struct {
	httpClient *http.Client
	baseURL    string
	limiter    *rate.Limiter
}

// NewClient creates a Humble client with production defaults.
func NewClient() *Client {
	return &Client{
		httpClient: http.DefaultClient,
		baseURL:    defaultBaseURL,
		limiter:    rate.NewLimiter(rate.Every(200*time.Millisecond), 1),
	}
}

// doGet performs a rate-limited authenticated GET and returns the response body.
// A 401/403 maps to ErrCredentials; any other non-200 is a generic error.
func (c *Client) doGet(ctx context.Context, cookie, path string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("humble: rate limiter wait: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("humble: build request: %w", err)
	}
	req.Header.Set("Cookie", "_simpleauth_sess="+cookie)
	req.Header.Set("X-Requested-By", requestedByHeader)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("humble: request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrCredentials
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("humble: request %s: unexpected status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("humble: read %s: %w", path, err)
	}
	return body, nil
}

// Verify confirms the session cookie is valid by hitting the order-list
// endpoint. Returns ErrCredentials on 401/403.
func (c *Client) Verify(ctx context.Context, cookie string) error {
	_, err := c.doGet(ctx, cookie, "/api/v1/user/order")
	return err
}

// ListGamekeys returns the gamekeys for every order owned by the cookie's user.
func (c *Client) ListGamekeys(ctx context.Context, cookie string) ([]string, error) {
	body, err := c.doGet(ctx, cookie, "/api/v1/user/order")
	if err != nil {
		return nil, err
	}
	var orders []struct {
		Gamekey string `json:"gamekey"`
	}
	if err := json.Unmarshal(body, &orders); err != nil {
		return nil, fmt.Errorf("humble: decode gamekeys: %w", err)
	}
	keys := make([]string, 0, len(orders))
	for _, o := range orders {
		if o.Gamekey != "" {
			keys = append(keys, o.Gamekey)
		}
	}
	return keys, nil
}

// GetOrder fetches one order's full detail (with all third-party-key data, which
// the adapter ignores).
func (c *Client) GetOrder(ctx context.Context, cookie, gamekey string) (*Order, error) {
	body, err := c.doGet(ctx, cookie, "/api/v1/order/"+url.PathEscape(gamekey)+"?all_tpkds=true")
	if err != nil {
		return nil, err
	}
	var order Order
	if err := json.Unmarshal(body, &order); err != nil {
		return nil, fmt.Errorf("humble: decode order %s: %w", gamekey, err)
	}
	return &order, nil
}

package gog

import (
	"errors"
	"net/http"

	"github.com/drzero42/nexorious/internal/logging"
)

// ErrGOGAuthExpired is returned when GOG rejects the token as expired.
var ErrGOGAuthExpired = errors.New("gog: auth token expired")

// ErrGOGUnauthorized is returned for other authorization failures.
var ErrGOGUnauthorized = errors.New("gog: unauthorized")

// Client is an HTTP client for the GOG API.
type Client struct {
	httpClient *http.Client
	authBase   string
	tokenBase  string
	embedBase  string
}

// NewClient creates a Client with production GOG endpoints.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Transport: logging.NewRoundTripper(nil)},
		authBase:   defaultAuthBase,
		tokenBase:  defaultTokenBase,
		embedBase:  defaultEmbedBase,
	}
}

// NewClientWithURLs creates a Client with overridden base URLs for testing.
// tokenBase replaces https://auth.gog.com; embedBase replaces https://embed.gog.com.
func NewClientWithURLs(tokenBase, embedBase string) *Client {
	return &Client{
		httpClient: &http.Client{Transport: logging.NewRoundTripper(nil)},
		authBase:   defaultAuthBase,
		tokenBase:  tokenBase,
		embedBase:  embedBase,
	}
}

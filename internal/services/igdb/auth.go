package igdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
)

const (
	defaultTwitchTokenURL = "https://id.twitch.tv/oauth2/token" //nolint:gosec // public Twitch OAuth endpoint URL, not a credential
	tokenExpiryBuffer     = 5 * time.Minute
)

// AuthManager handles Twitch OAuth2 client credentials flow for IGDB.
type AuthManager struct {
	mu           sync.Mutex
	accessToken  string
	expiresAt    time.Time
	clientID     string
	clientSecret string
	httpClient   *http.Client
	tokenURL     string // overridable for testing
}

// NewAuthManager creates a new AuthManager. If preConfiguredToken is non-empty,
// it is used as the initial token value with unknown expiry.
func NewAuthManager(clientID, clientSecret, preConfiguredToken string) *AuthManager {
	am := &AuthManager{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second, Transport: logging.NewRoundTripper(nil)},
		tokenURL:     defaultTwitchTokenURL,
	}
	if preConfiguredToken != "" {
		am.accessToken = preConfiguredToken
		// No expiresAt — unknown expiry, used until 401
	}
	return am
}

// GetAccessToken returns a valid access token, refreshing if needed.
func (am *AuthManager) GetAccessToken(ctx context.Context) (string, error) {
	// Fast path: check without lock
	if am.isTokenValid() {
		return am.accessToken, nil
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	// Double-check after acquiring lock
	if am.isTokenValid() {
		return am.accessToken, nil
	}

	// Fetch new token
	if err := am.fetchToken(ctx); err != nil {
		return "", err
	}
	return am.accessToken, nil
}

// isTokenValid checks if the current token is usable.
func (am *AuthManager) isTokenValid() bool {
	if am.accessToken == "" {
		return false
	}
	// If expiresAt is zero (pre-configured token with unknown expiry), treat as valid
	if am.expiresAt.IsZero() {
		return true
	}
	// Valid if more than 5 minutes remain
	return time.Now().Add(tokenExpiryBuffer).Before(am.expiresAt)
}

// fetchToken requests a new token from Twitch.
func (am *AuthManager) fetchToken(ctx context.Context) error {
	data := url.Values{
		"client_id":     {am.clientID},
		"client_secret": {am.clientSecret},
		"grant_type":    {"client_credentials"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, am.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		// Network/DNS/timeout — NOT an auth error.
		return fmt.Errorf("twitch token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// HTTP error from Twitch — this IS an auth error
		return fmt.Errorf("%w: Twitch returned status %d", ErrTwitchAuth, resp.StatusCode)
	}

	var tokenResp twitchTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("decode Twitch token response: %w", err)
	}

	am.accessToken = tokenResp.AccessToken
	am.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return nil
}

// InvalidateToken clears the cached token, forcing a refresh on next call.
func (am *AuthManager) InvalidateToken() {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.expiresAt = time.Time{}
	am.accessToken = ""
}

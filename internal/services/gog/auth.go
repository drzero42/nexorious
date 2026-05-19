package gog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	clientID     = "46899977096215655"
	clientSecret = "9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"
	redirectURI  = "https://embed.gog.com/on_login_success?origin=client"

	defaultAuthBase  = "https://login.gog.com"
	defaultTokenBase = "https://auth.gog.com"
	defaultEmbedBase = "https://embed.gog.com"
)

// TokenResponse holds the tokens and account identity returned by GOG auth.
type TokenResponse struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	UserID       string
	Username     string
}

type tokenAPIResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	UserID       string `json:"user_id"`
}

type userDataResponse struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// BuildAuthURL returns the GOG OAuth login URL. The user opens this in a
// browser, logs in, and the resulting redirect URL contains an auth code to
// paste back into the application.
func (c *Client) BuildAuthURL() string {
	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"layout":        {"client2"},
	}
	return c.authBase + "/auth?" + params.Encode()
}

// ExchangeCode exchanges a GOG authorization code for access and refresh
// tokens, then fetches the account username from /userData.json.
func (c *Client) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	return c.postToken(ctx, url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	})
}

// RefreshToken exchanges a refresh token for a new access/refresh token pair.
// Always store the returned RefreshToken — GOG may rotate it.
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	return c.postToken(ctx, url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

func (c *Client) postToken(ctx context.Context, form url.Values) (*TokenResponse, error) {
	// GOG's token endpoint uses GET with query parameters, not POST with form body.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.tokenBase+"/token?"+form.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("gog: build token request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gog: token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrGOGAuthExpired
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gog: token HTTP %d", resp.StatusCode)
	}

	var apiResp tokenAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("gog: decode token response: %w", err)
	}

	username, err := c.fetchUsername(ctx, apiResp.AccessToken)
	if err != nil {
		// Non-fatal: proceed with empty username rather than failing the auth.
		username = ""
	}

	return &TokenResponse{
		AccessToken:  apiResp.AccessToken,
		RefreshToken: apiResp.RefreshToken,
		ExpiresIn:    apiResp.ExpiresIn,
		UserID:       apiResp.UserID,
		Username:     username,
	}, nil
}

func (c *Client) fetchUsername(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.embedBase+"/userData.json", nil)
	if err != nil {
		return "", fmt.Errorf("gog: build userData request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gog: userData request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gog: userData HTTP %d", resp.StatusCode)
	}

	var ud userDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&ud); err != nil {
		return "", fmt.Errorf("gog: decode userData: %w", err)
	}
	return ud.Username, nil
}

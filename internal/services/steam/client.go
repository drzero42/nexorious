package steam

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// Client is an HTTP client for the Steam Web API.
type Client struct {
	http           *http.Client
	limiter        *rate.Limiter
	ownedGamesBase string // default "https://api.steampowered.com"
	appDetailsBase string // default "https://store.steampowered.com"
}

// NewClient creates a new Steam API client.
func NewClient() *Client {
	return &Client{
		http:           &http.Client{},
		limiter:        rate.NewLimiter(rate.Every(200*time.Millisecond), 1),
		ownedGamesBase: "https://api.steampowered.com",
		appDetailsBase: "https://store.steampowered.com",
	}
}

// NewClientForTests creates a Steam API client with custom HTTP client, rate limiter,
// and base URLs. Only for use in tests.
func NewClientForTests(httpClient *http.Client, limiter *rate.Limiter, ownedGamesBase, appDetailsBase string) *Client {
	return &Client{
		http:           httpClient,
		limiter:        limiter,
		ownedGamesBase: ownedGamesBase,
		appDetailsBase: appDetailsBase,
	}
}

// SteamPlayerSummary holds basic player info from the Steam API.
type SteamPlayerSummary struct {
	PersonaName              string
	CommunityVisibilityState int
}

// OwnedGame is a game entry from the user's Steam library.
type OwnedGame struct {
	AppID         int
	Title         string
	PlaytimeHours int
}

// Platforms represents per-OS availability from the Steam store appdetails endpoint.
type Platforms struct {
	Windows bool
	Mac     bool
	Linux   bool
}

// GetPlayerSummaries fetches the player summary for the given steamID.
// Returns nil, nil if no player was found for that steamID.
func (c *Client) GetPlayerSummaries(ctx context.Context, apiKey, steamID string) (*SteamPlayerSummary, error) {
	url := fmt.Sprintf(
		"%s/ISteamUser/GetPlayerSummaries/v0002/?key=%s&steamids=%s&format=json",
		c.ownedGamesBase, apiKey, steamID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("steam: failed to create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("steam network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("steam rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam HTTP %d", resp.StatusCode)
	}

	var body struct {
		Response struct {
			Players []struct {
				PersonaName              string `json:"personaname"`
				CommunityVisibilityState int    `json:"communityvisibilitystate"`
			} `json:"players"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("steam decode error: %w", err)
	}
	if len(body.Response.Players) == 0 {
		return nil, nil
	}
	p := body.Response.Players[0]
	return &SteamPlayerSummary{
		PersonaName:              p.PersonaName,
		CommunityVisibilityState: p.CommunityVisibilityState,
	}, nil
}

// GetOwnedGames fetches the full Steam library for the given steamID.
// playtime_forever from the API is in minutes; this method converts it to hours.
func (c *Client) GetOwnedGames(ctx context.Context, apiKey, steamID string) ([]OwnedGame, error) {
	url := fmt.Sprintf(
		"%s/IPlayerService/GetOwnedGames/v0001/?key=%s&steamid=%s&include_appinfo=true&format=json",
		c.ownedGamesBase, apiKey, steamID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("steam: failed to create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("steam network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam HTTP %d", resp.StatusCode)
	}

	var body struct {
		Response struct {
			Games []struct {
				AppID           int    `json:"appid"`
				Name            string `json:"name"`
				PlaytimeForever int    `json:"playtime_forever"`
			} `json:"games"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("steam decode error: %w", err)
	}

	games := make([]OwnedGame, 0, len(body.Response.Games))
	for _, g := range body.Response.Games {
		games = append(games, OwnedGame{
			AppID:         g.AppID,
			Title:         g.Name,
			PlaytimeHours: g.PlaytimeForever / 60,
		})
	}
	return games, nil
}

// GetAppDetailsPlatforms fetches platform availability for the given appID.
// Returns (Platforms{}, error) for non-200, success=false, decode error, or missing key.
// Returns (Platforms{}, nil) when success=true but all platforms are false — caller decides fallback.
func (c *Client) GetAppDetailsPlatforms(ctx context.Context, appID int) (Platforms, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return Platforms{}, err
	}
	url := fmt.Sprintf("%s/api/appdetails?appids=%d&filters=platforms", c.appDetailsBase, appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Platforms{}, fmt.Errorf("steam appdetails: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Platforms{}, fmt.Errorf("steam appdetails network error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Platforms{}, fmt.Errorf("steam appdetails HTTP %d for appid %d", resp.StatusCode, appID)
	}

	var body map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Platforms struct {
				Windows bool `json:"windows"`
				Mac     bool `json:"mac"`
				Linux   bool `json:"linux"`
			} `json:"platforms"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Platforms{}, fmt.Errorf("steam appdetails decode error: %w", err)
	}
	key := fmt.Sprintf("%d", appID)
	entry, ok := body[key]
	if !ok {
		return Platforms{}, fmt.Errorf("steam appdetails: missing key %q in response", key)
	}
	if !entry.Success {
		return Platforms{}, fmt.Errorf("steam appdetails: success=false for appid %d", appID)
	}
	return Platforms{
		Windows: entry.Data.Platforms.Windows,
		Mac:     entry.Data.Platforms.Mac,
		Linux:   entry.Data.Platforms.Linux,
	}, nil
}

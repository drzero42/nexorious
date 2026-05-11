package steam

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Client is an HTTP client for the Steam Web API.
type Client struct {
	http *http.Client
}

// NewClient creates a new Steam API client.
func NewClient() *Client {
	return &Client{http: &http.Client{}}
}

// SteamPlayerSummary is the steam-local type — does NOT import the api package.
type SteamPlayerSummary struct {
	PersonaName              string
	CommunityVisibilityState int
}

// GetPlayerSummaries fetches the player summary for the given steamID.
// Returns nil, nil if no player was found for that steamID.
func (c *Client) GetPlayerSummaries(ctx context.Context, apiKey, steamID string) (*SteamPlayerSummary, error) {
	url := fmt.Sprintf(
		"https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v0002/?key=%s&steamids=%s&format=json",
		apiKey, steamID,
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

// ExternalLibraryEntry is a normalised game entry from an external source.
type ExternalLibraryEntry struct {
	ExternalID      string
	Title           string
	RawPlatform     string
	PlaytimeHours   int
	OwnershipStatus string
	IsSubscription  bool
}

// GetOwnedGames fetches the full Steam library for the given steamID.
// playtime_forever from the API is in minutes; this method converts it to hours.
func (c *Client) GetOwnedGames(ctx context.Context, apiKey, steamID string) ([]ExternalLibraryEntry, error) {
	url := fmt.Sprintf(
		"https://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=%s&steamid=%s&include_appinfo=true&format=json",
		apiKey, steamID,
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

	entries := make([]ExternalLibraryEntry, 0, len(body.Response.Games))
	for _, g := range body.Response.Games {
		entries = append(entries, ExternalLibraryEntry{
			ExternalID:      fmt.Sprintf("%d", g.AppID),
			Title:           g.Name,
			RawPlatform:     "pc-windows",
			PlaytimeHours:   g.PlaytimeForever / 60,
			OwnershipStatus: "owned",
			IsSubscription:  false,
		})
	}
	return entries, nil
}

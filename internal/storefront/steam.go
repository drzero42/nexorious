package storefront

import (
	"context"
	"errors"
	"regexp"

	steamsvc "github.com/drzero42/nexorious/internal/services/steam"
)

// SteamClient is the Steam Web API call connecting verifies a key and id with.
// *steamsvc.Client implements it.
type SteamClient interface {
	GetPlayerSummaries(ctx context.Context, apiKey, steamID string) (*steamsvc.SteamPlayerSummary, error)
}

// The ways ConnectSteam rejects a sign-in. Nothing is stored after any of them.
var (
	// ErrInvalidSteamID: the id is not a SteamID64, or Steam knows no such
	// player.
	ErrInvalidSteamID = errors.New("steam: invalid steam id")
	// ErrInvalidSteamAPIKey: the key is not 32 hex digits.
	ErrInvalidSteamAPIKey = errors.New("steam: invalid web api key")
	// ErrSteamPrivateProfile: the profile is not public, so its library
	// cannot be read.
	ErrSteamPrivateProfile = errors.New("steam: profile is not public")
	// ErrSteamRateLimited: Steam answered 429.
	ErrSteamRateLimited = errors.New("steam: rate limited")
	// ErrSteamUnavailable: Steam could not be reached or gave no usable
	// answer.
	ErrSteamUnavailable = errors.New("steam: unavailable")
)

var (
	steamIDPattern     = regexp.MustCompile(`^7656119[0-9]{10}$`)
	steamAPIKeyPattern = regexp.MustCompile(`^[0-9A-Fa-f]{32}$`)
)

// steamCommunityVisibilityPublic is a public profile's
// communityvisibilitystate.
const steamCommunityVisibilityPublic = 3

// ConnectSteam verifies a Steam id and Web API key against Steam and stores
// them as the user's Steam Connection. It returns the profile's display name.
func (c *Connections) ConnectSteam(ctx context.Context, userID, steamID, webAPIKey string) (string, error) {
	if !steamIDPattern.MatchString(steamID) {
		return "", ErrInvalidSteamID
	}
	if !steamAPIKeyPattern.MatchString(webAPIKey) {
		return "", ErrInvalidSteamAPIKey
	}
	summary, err := c.clients.Steam.GetPlayerSummaries(ctx, webAPIKey, steamID)
	// Only the sentinels are returned: Steam's errors can carry the request
	// URL, and with it the Web API key.
	switch {
	case errors.Is(err, steamsvc.ErrRateLimited):
		return "", ErrSteamRateLimited
	case err != nil:
		return "", ErrSteamUnavailable
	case summary == nil:
		return "", ErrInvalidSteamID
	case summary.CommunityVisibilityState != steamCommunityVisibilityPublic:
		return "", ErrSteamPrivateProfile
	}
	creds := SteamCredentials{WebAPIKey: webAPIKey, SteamID: steamID, DisplayName: summary.PersonaName}
	// Steam accepted the sign-in; storing it does not depend on the caller
	// still waiting.
	if err := c.Store(context.WithoutCancel(ctx), userID, "steam", creds); err != nil {
		return "", err
	}
	return summary.PersonaName, nil
}

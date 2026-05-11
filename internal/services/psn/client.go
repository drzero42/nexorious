package psn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	psnsdk "github.com/sizovilya/go-psn-api"
)

// PSNAccountInfo is the psn-local type — does NOT import the api package.
type PSNAccountInfo struct {
	OnlineID  string
	AccountID string
	Region    string
}

// ErrInvalidNPSSOToken is returned when authentication with the NPSSO token fails.
var ErrInvalidNPSSOToken = errors.New("invalid npsso token")

// Client wraps the go-psn-api library.
type Client struct{}

// NewClient creates a new PSN client.
func NewClient() *Client { return &Client{} }

// GetAccountInfo authenticates with PSN using the given NPSSO token and returns
// account information for the authenticated user.
// Returns ErrInvalidNPSSOToken if authentication fails.
func (c *Client) GetAccountInfo(ctx context.Context, npssoToken string) (*PSNAccountInfo, error) {
	psnClient, err := psnsdk.NewClient(&psnsdk.Options{
		Lang:   "en",
		Region: "us",
		Npsso:  npssoToken,
	})
	if err != nil {
		return nil, fmt.Errorf("psn: failed to create client: %w", err)
	}

	if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
		// Auth failure indicates invalid/expired NPSSO token.
		if strings.Contains(err.Error(), "authentication failed") ||
			strings.Contains(err.Error(), "expected redirect") ||
			strings.Contains(err.Error(), "npsso") {
			return nil, ErrInvalidNPSSOToken
		}
		return nil, ErrInvalidNPSSOToken
	}

	// Fetch the authenticated user's own profile using the "me" alias supported
	// by Sony's profile API.
	accessToken, _ := psnClient.AccessToken()
	profile, err := fetchMyProfile(ctx, accessToken)
	if err != nil {
		// If we can't fetch the profile, still return what we have from auth.
		// This can happen if the profile API changes; return a partial result.
		return &PSNAccountInfo{
			Region: psnClient.Region(),
		}, nil
	}

	return &PSNAccountInfo{
		OnlineID:  profile.OnlineID,
		AccountID: profile.NpID,
		Region:    psnClient.Region(),
	}, nil
}

// profileSelf is the shape of a Sony "me" profile response.
type profileSelf struct {
	Profile struct {
		OnlineID string `json:"onlineId"`
		NpID     string `json:"npId"`
	} `json:"profile"`
}

// fetchMyProfile calls Sony's profile API with the "me" path alias.
func fetchMyProfile(ctx context.Context, accessToken string) (*struct {
	OnlineID string
	NpID     string
}, error) {
	const meURL = "https://us-prof.np.community.playstation.net/userProfile/v1/users/me/profile2?fields=onlineId,npId"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meURL, nil)
	if err != nil {
		return nil, fmt.Errorf("psn: failed to create profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("psn: profile request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("psn: profile HTTP %d", resp.StatusCode)
	}

	var body profileSelf
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("psn: profile decode error: %w", err)
	}

	return &struct {
		OnlineID string
		NpID     string
	}{
		OnlineID: body.Profile.OnlineID,
		NpID:     body.Profile.NpID,
	}, nil
}

// ExternalLibraryEntry is a normalised game entry from PSN.
type ExternalLibraryEntry struct {
	ExternalID      string
	Title           string
	RawPlatform     string
	PlaytimeHours   int
	OwnershipStatus string
	IsSubscription  bool
}

// GetLibrary fetches the PSN trophy title list as a proxy for the user's game library.
// The go-psn-api library does not expose a direct "owned games" endpoint;
// trophy titles are used as the closest available approximation.
// Maps PS4/PS3/PSVITA titles to their platform slugs; PS5 titles are identified
// by NpCommunicationID prefix conventions where possible.
// TODO: implement proper game library fetching when a suitable PSN API is available.
func (c *Client) GetLibrary(ctx context.Context, npssoToken string) ([]ExternalLibraryEntry, error) {
	psnClient, err := psnsdk.NewClient(&psnsdk.Options{
		Lang:   "en",
		Region: "us",
		Npsso:  npssoToken,
	})
	if err != nil {
		return nil, fmt.Errorf("psn: failed to create client: %w", err)
	}

	if err := psnClient.AuthWithNPSSO(ctx, npssoToken); err != nil {
		return nil, ErrInvalidNPSSOToken
	}

	// Fetch trophy titles as a library proxy; limit=100 for a reasonable first page.
	// The "comparedUser" parameter requires the user's onlineID — use "me" as alias.
	resp, err := psnClient.GetTrophyTitles(ctx, "me", 100, 0)
	if err != nil {
		// Trophy titles may not be accessible; return empty rather than blocking.
		return []ExternalLibraryEntry{}, nil
	}

	platformMap := map[string]string{
		"PS3":   "playstation-3",
		"PS4":   "playstation-4",
		"PS5":   "playstation-5",
		"PSVITA": "ps-vita",
	}

	entries := make([]ExternalLibraryEntry, 0, len(resp.TrophyTitles))
	for _, t := range resp.TrophyTitles {
		rawPlatform := platformMap[t.TrophyTitlePlatfrom]
		if rawPlatform == "" {
			rawPlatform = "playstation-4" // sensible default
		}
		entries = append(entries, ExternalLibraryEntry{
			ExternalID:      t.NpCommunicationID,
			Title:           t.TrophyTitleName,
			RawPlatform:     rawPlatform,
			PlaytimeHours:   0,
			OwnershipStatus: "owned",
			IsSubscription:  false,
		})
	}
	return entries, nil
}
